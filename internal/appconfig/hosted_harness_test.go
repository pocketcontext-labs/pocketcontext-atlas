package appconfig

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"ganigeorgiev.com/pocketbase/pocketbase/core"
	"github.com/pocketcontext-labs/pocketcontext-core/appserver"
)

func syntheticHostedConfig() *appserver.HostedConfig {
	return &appserver.HostedConfig{OwnerEmail: "owner@example.test", From: "login@example.test", SMTP: core.SMTPConfig{Enabled: true, Host: "smtp.example.test", Port: 465, TLS: true, Username: "resend", Password: "synthetic-smtp-secret"}}
}

// hostedTestServer composes the real hosted request path around Atlas's hooks.
func hostedTestServer(t *testing.T, opts *appserver.TestServerOptions) (*testApp, *appserver.TestServer) {
	t.Helper()
	a := newTestApp(t)
	a.user.SetEmail("owner@example.test")
	if err := a.app.Save(a.user); err != nil {
		t.Fatal(err)
	}
	options := appserver.TestServerOptions{}
	if opts != nil {
		options = *opts
	}
	options.Domain, options.Hosted, options.HostedConfig = "knowledge.example.com", true, syntheticHostedConfig()
	if options.Cache == "" {
		options.Cache = t.TempDir()
	}
	server, err := Runtime.NewTestServer(a.app, options)
	if err != nil {
		t.Fatal(err)
	}
	return a, server
}

func hostedRequest(h http.Handler, method, path, body string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, "http://knowledge.example.com"+path, strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	r.RemoteAddr = "10.0.0.2:12345"
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

// Product routes on the hosted boundary: the public allowlist itself is pinned
// by the frozen table; this proves it is the policy the handler actually uses.
func TestHostedProductRoutesBoundary(t *testing.T) {
	_, server := hostedTestServer(t, nil)
	h := server.Public()
	for _, tc := range []struct {
		method, path, body string
		status             int
	}{
		{"GET", "/api/bundle/auth-mode", "", 200}, {"GET", "/up", "", 200},
		{"POST", "/api/files/token", "{}", 401},
		{"POST", "/api/batch", `{"requests":[]}`, 404},
		{"POST", "/api/collections/users/auth-with-password", `{"identity":"owner@example.test","password":"x"}`, 404},
		{"POST", "/api/bundle/redeem", `{"code":"x"}`, 404}, {"GET", "/bundle/login", "", 404},
		{"POST", "/api/bundle/account", `{}`, 404}, {"GET", "/_/", "", 404},
	} {
		if w := hostedRequest(h, tc.method, tc.path, tc.body); w.Code != tc.status {
			t.Errorf("%s %s: %d %s", tc.method, tc.path, w.Code, w.Body)
		}
	}
}
