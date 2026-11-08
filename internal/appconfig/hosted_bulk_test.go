package appconfig

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

// MCP dispatches to the ordinary authenticated router after OAuth validation;
// /api/batch deliberately remains unavailable on the public HTTP allowlist.
func TestHostedBulkConsecutiveBatches(t *testing.T) {
	a, server := hostedTestServer(t, nil)
	token, err := a.user.NewAuthToken()
	if err != nil {
		t.Fatal(err)
	}
	raw := server.Router()
	for batch := range 6 {
		requests := make([]any, 20)
		for i := range requests {
			requests[i] = map[string]any{"method": "POST", "url": "/api/collections/pages/records", "body": map[string]any{"slug": fmt.Sprintf("bulk-%d-%d", batch, i), "kind": "concept"}}
		}
		body, _ := json.Marshal(map[string]any{"requests": requests})
		req := httptest.NewRequest("POST", "https://knowledge.example.com/api/batch", bytes.NewReader(body))
		req.RemoteAddr = "192.0.2.1:0"
		req = server.MarkPublic(req)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", token)
		rec := httptest.NewRecorder()
		raw.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("batch %d: %d %s", batch+1, rec.Code, rec.Body)
		}
	}
	pages, err := a.app.FindAllRecords("pages")
	if err != nil || len(pages) != 120 {
		t.Fatalf("pages=%d err=%v", len(pages), err)
	}
	if rec := hostedRequest(server.Public(), "POST", "/api/batch", `{"requests":[]}`); rec.Code != 404 {
		t.Fatalf("public batch status=%d", rec.Code)
	}
	req := httptest.NewRequest("POST", "https://knowledge.example.com/api/collections/pages/records", bytes.NewBufferString(`{"slug":"anonymous-write","kind":"concept"}`))
	req = server.MarkPublic(req)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	raw.ServeHTTP(rec, req)
	if rec.Code < 400 {
		t.Fatalf("anonymous write accepted: %d", rec.Code)
	}
}
