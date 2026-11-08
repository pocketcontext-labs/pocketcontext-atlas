package appconfig

import (
	"encoding/json"
	"github.com/pocketcontext-labs/pocketcontext-core/appserver"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"ganigeorgiev.com/pocketbase/pocketbase/core"
)

// rateLimitInfo mirrors the rate metadata body the server returns.
type rateLimitInfo struct {
	Limit             string `json:"limit"`
	RetryAfterSeconds int    `json:"retry_after_seconds"`
	Capacity          int    `json:"capacity"`
	Remaining         int    `json:"remaining"`
}

func assertRateMetadata(t *testing.T, w *httptest.ResponseRecorder, label string, capacity, retry int) {
	t.Helper()
	var body struct {
		Data struct {
			Rate rateLimitInfo `json:"rate_limit"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Data.Rate.Limit != label || body.Data.Rate.Capacity != capacity || body.Data.Rate.RetryAfterSeconds != retry {
		t.Fatalf("metadata: %+v, body %s", body.Data.Rate, w.Body)
	}
	for _, header := range []string{"Retry-After", "RateLimit-Limit", "RateLimit-Remaining", "RateLimit-Reset"} {
		if w.Header().Get(header) == "" {
			t.Errorf("missing %s", header)
		}
	}
}

func TestHostedRateLimitChargesBatchBeforeExecution(t *testing.T) {
	now := time.Now()
	a, server := hostedTestServer(t, &appserver.TestServerOptions{Clock: func() time.Time { return now }})
	h := server.Router()
	// A batch with no requests is rejected by its handler after admission, so a
	// 429 means the middleware budget refused it before any handler ran.
	admitted := 0
	// Freeze time so only admission cost determines the result.
	start := time.Now()
	var rejected bool
	for i := 0; i < 61; i++ {
		r := server.MarkPublic(httptest.NewRequest("POST", "http://knowledge.example.com/api/batch", strings.NewReader(`{"requests":[]}`)))
		r.Header.Set("Content-Type", "application/json")
		token, err := a.user.NewAuthToken()
		if err != nil {
			t.Fatal(err)
		}
		r.Header.Set("Authorization", token)
		r.RemoteAddr = "192.0.2." + strconv.Itoa(i+1) + ":1234"
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code == 429 {
			rejected = true
			assertRateMetadata(t, w, "authenticated_knowledge", 1200, 1)
			if admitted != 60 {
				t.Fatalf("premature limit: %d", admitted)
			}
			break
		}
		admitted++
	}
	if !rejected {
		t.Fatalf("no bounded limit after %s", time.Since(start))
	}
}

func TestHostedRateLimitNestedBatchPreservesCustomRule(t *testing.T) {
	a := newTestApp(t)
	a.user.SetEmail("owner@example.test")
	if err := a.app.Save(a.user); err != nil {
		t.Fatal(err)
	}
	a.app.Settings().RateLimits.Rules = append([]core.RateLimitRule{{Label: "pages:create", Audience: core.RateLimitRuleAudienceAuth, MaxRequests: 1, Duration: 60}}, a.app.Settings().RateLimits.Rules...)
	server, err := Runtime.NewTestServer(a.app, appserver.TestServerOptions{Domain: "knowledge.example.com", Hosted: true, HostedConfig: syntheticHostedConfig(), Cache: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	h := server.Router()
	body := `{"requests":[{"method":"POST","url":"/api/collections/pages/records","body":{"slug":"rate-first","kind":"concept"}},{"method":"POST","url":"/api/collections/pages/records","body":{"slug":"rate-second","kind":"concept"}}]}`
	req := server.MarkPublic(httptest.NewRequest("POST", "http://knowledge.example.com/api/batch", strings.NewReader(body)))
	token, err := a.user.NewAuthToken()
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", token)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != 400 {
		t.Fatalf("want outer400: %d %s", w.Code, w.Body)
	}
	assertRateMetadata(t, w, "pocketbase:pages:create", 1, 60)
	var parsed map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &parsed); err != nil {
		t.Fatal(err)
	}
	inner := parsed["data"].(map[string]any)["requests"].(map[string]any)["1"].(map[string]any)["response"].(map[string]any)
	if inner["status"] != float64(429) || inner["data"].(map[string]any)["rate_limit"] == nil {
		t.Fatalf("inner metadata missing: %v", inner)
	}
	pages, err := a.app.FindAllRecords("pages")
	if err != nil || len(pages) != 0 {
		t.Fatalf("batch rollback: %d pages, %v", len(pages), err)
	}
}
