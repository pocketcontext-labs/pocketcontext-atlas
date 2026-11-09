// Package atlaspolicy holds what makes this server PocketContext Atlas rather
// than another application of pocketcontext-core: its identity, public route
// allowlist, maintenance reads and copy. It carries no runtime state.
package atlaspolicy

import (
	"net/http"
	"strings"

	"ganigeorgiev.com/pocketbase/pocketbase/core"

	"github.com/pocketcontext-labs/pocketcontext-core/appidentity"
	"github.com/pocketcontext-labs/pocketcontext-core/appserver"
	"github.com/pocketcontext-labs/pocketcontext-core/maintenance"
	"github.com/pocketcontext-labs/pocketcontext-core/mcpobserve"
)

// Identity is Atlas's identity. Names of files, variables and headers that older
// binaries, hosted containers and backups rely on must never change here.
var Identity = appidentity.Identity{
	AppID:             "pocketcontext-atlas",
	LegacyAppIDs:      []string{"pocketcontext"},
	DisplayName:       "PocketContext Atlas",
	Command:           "pocketcontext-atlas",
	SkillName:         "pocketcontext-atlas",
	Marker:            ".pocketcontext-atlas",
	RouterMarker:      ".pocketcontext-atlas-router",
	RegistryNamespace: "pocketcontext-atlas",
	CacheRoot:         "pocketcontext", // holds the working copy, keys and hosted state; never move
	SocketHost:        "pocketcontext.internal",
	SocketDirPrefix:   "pocketcontext",
	DefaultPort:       8096,
	DatabaseFile:      "pocketcontext.db",
	ConfigFile:        "pocketcontext.json",
	EnvPrefix:         "POCKETCONTEXT",
	LocalKeyHeader:    "X-PocketContext-Local-Key",
	LocalAccountEmail: "me@pocketcontext.localhost",
	ReaderStorageKey:  "pocketcontext.reader.auth",
	HookGlobal:        "$pocketcontext",
}

// Text is the copy shown to people.
var Text = appserver.Text{
	AccountName:        "PocketContext",
	LegacyRecreateHint: "Keep using the earlier release for it, or start a new project with `pocketcontext-atlas init --db NEW_PATH` and ingest the sources again",
	OAuthResourceName:  "PocketContext knowledge",
	OAuthConsent: appserver.OAuthConsentText{
		ConnectionsHTML: `<p>These apps can read and change knowledge in every space. Revoke access to disconnect an app.</p>`,
		RequestHTML:     `<p><strong>{{.Name}}</strong> is asking to read and change your knowledge in every space, including originals, citations and published pages.</p><p>This does not grant server administration or shell access.</p>`,
		// Atlas has no connected-apps view in the reader, so ConnectionsPath
		// stays empty and /oauth/connections keeps Core's page in this style.
		Page: appserver.OAuthPage{
			CSS:        oauthPageCSS,
			BrandHTML:  oauthBrandHTML,
			SignInPath: "/#/signin",
		},
	},
	BudgetExhausted: "Knowledge request budget exhausted. Retry after the indicated delay.",
	OTPSubject:      "Log in to PocketContext Atlas",
	OTPSenderName:   "PocketContext Atlas",
}

// The OAuth pages' Content-Security-Policy loads no images or fonts, so the
// logo (ui/src/assets/brand/logo.svg) is inline and the wordmark falls back to
// brand.css's Arial. Colours and type follow ui/src/theme.css and the reader's
// login card. No text is under 16px, so iOS Safari never zooms.
const oauthBrandHTML = `<div class="product-brand"><svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 256 256" width="34" height="34" aria-hidden="true"><path d="M15 10h226v149l-113 87L15 159z" fill="#e9ede2" stroke="#688258" stroke-width="8" stroke-linejoin="round"/><path d="m86 124 30 30 64-70" fill="none" stroke="#2e493e" stroke-width="12" stroke-linecap="round" stroke-linejoin="round"/></svg><span>PocketContext Atlas</span></div>`

const oauthPageCSS = `:root{color-scheme:light}
*{box-sizing:border-box}
[hidden]{display:none!important}
html{background:#fafaf7}
body{margin:0 auto;min-height:100dvh;max-width:408px;padding:35px 24px;display:flex;flex-direction:column;justify-content:center;background:radial-gradient(ellipse at 50% 20%,#35694f0d,transparent 65%);color:#46554b;font:16px/1.6 Inter,ui-sans-serif,-apple-system,BlinkMacSystemFont,"Segoe UI",sans-serif;overflow-wrap:anywhere}
.product-brand{display:flex;align-items:center;gap:10px;margin-bottom:32px;color:#263b32;font:700 20px Arial,sans-serif;letter-spacing:-.6px}
.product-brand svg{display:block;flex-shrink:0}
h1{margin:0 0 20px;color:#25332c;font:400 34px/1.15 Georgia,serif;letter-spacing:-1px}
p{margin:0 0 14px}
p strong{color:#25332c;font-weight:600}
small{display:block;margin:0 0 24px;color:#596b5e;font-size:16px}
code{font:16px ui-monospace,SFMono-Regular,Consolas,monospace;color:#25332c}
button,a{font:inherit;-webkit-tap-highlight-color:transparent;touch-action:manipulation}
button{cursor:pointer}
button:disabled{opacity:.6;cursor:wait}
a{color:#35694f}
#approve,#signin{display:block;width:100%;padding:13px 18px;border:0;border-radius:6px;background:#35694f;color:#f7faf7;font-weight:550;text-align:center;text-decoration:none}
#deny{display:block;margin:6px auto 0;padding:10px 12px;border:0;background:none;color:#526657;text-decoration:underline}
#message{margin:14px 0;color:#a13930;white-space:pre-wrap}
#message:empty{display:none}
#signin+p,#message+p{margin-top:24px}
#message~p{text-align:center}
#connections{margin:0 0 20px;padding:0;list-style:none;border-top:1px solid #dfe4dc}
#connections li{padding:14px 0;border-bottom:1px solid #dfe4dc;color:#25332c}
#reload{align-self:flex-start}
#connections li button,#reload{margin-top:8px;padding:9px 14px;border:1px solid #35694f33;border-radius:6px;background:#f0f2ed;color:#25332c}
button:focus-visible,a:focus-visible{outline:2px solid #35694f;outline-offset:4px}`

// Policy returns the public routes, maintenance reads and observability lists.
func Policy() appserver.Policy {
	return appserver.Policy{
		PublicRoute: PublicRouteFactory,
		Maintenance: maintenance.Policy{
			ReadPosts: map[string]bool{"/api/context/query": true, "/api/context/search": true, "/api/files/token": true},
			ReadGets:  map[string]bool{"/api/context/schema": true},
		},
		Observe:           Observe,
		HostedRequestCost: HostedRequestCost,
	}
}

// Observe lists the diagnostic labels; anything else is reported as unknown.
var Observe = mcpobserve.Policy{
	Tools:     strings.Fields("whoami schema query search search_sources get new_id create update batch list_spaces create_space rename_space archive_space restore_space default_space set_default_space ingest_text ingest_file extract pending original passages get_page lint stage check_run publish stage_home"),
	APIRoutes: strings.Fields(`/api/context/schema /api/context/query /api/context/search /api/spaces/default /api/batch /api/files/token /api/files/sources/{id}/{filename} /api/pocketcontext/runs/{id}/check /api/collections/{collection}/records /api/collections/{collection}/records/{id}`),
}

// HostedRequestCost reserves the server's maximum batch size before any
// operation runs. This bounds multipart and JSON batches equally without
// reparsing their bodies. Inner record handlers are guest-only, so they do not
// charge a second time.
func HostedRequestCost(re *core.RequestEvent) int {
	if re.Request.URL.Path == "/api/batch" {
		return max(1, re.App.Settings().Batch.MaxRequests)
	}
	return 1
}

var publicCollections = []string{"spaces", "settings", "sources", "renditions", "passages", "pages", "ingestion_runs", "page_revisions", "citations", "page_links", "publications", "audit_log"}

// PublicRouteFactory resolves the public collections to their ids once, when a
// public handler is built.
func PublicRouteFactory(app core.App) func(method, path string) bool {
	collections := map[string]bool{}
	sources := map[string]bool{"sources": true}
	for _, name := range publicCollections {
		collections[name] = true
		if c, err := app.FindCollectionByNameOrId(name); err == nil {
			collections[c.Id] = true
			if name == "sources" {
				sources[c.Id] = true
			}
		}
	}
	return func(method, p string) bool { return publicRoute(method, p, collections, sources) }
}

// publicRoute is deliberately an allowlist. A future local API does not become
// public merely because it was registered on the application's router.
func publicRoute(method, p string, collections map[string]bool, sources map[string]bool) bool {
	read := method == http.MethodGet || method == http.MethodHead
	if read && (p == "/" || p == "/api/bundle/auth-mode" || p == "/bundle/login" || p == "/bundle/login.js") {
		return true
	}
	if read && strings.HasPrefix(p, "/assets/") && appserver.ReaderAssetName(strings.TrimPrefix(p, "/assets/")) {
		return true
	}
	if read && (p == "/api/context/schema" || p == "/api/spaces/default" || p == "/api/realtime") {
		return true
	}
	if method == http.MethodPost {
		switch p {
		case "/api/bundle/redeem", "/api/collections/users/auth-with-password", "/api/collections/users/auth-refresh", "/api/context/query", "/api/context/search", "/api/realtime", "/api/files/token":
			return true
		}
	}
	parts := strings.Split(strings.TrimPrefix(p, "/"), "/")
	if len(parts) == 5 && parts[0] == "api" && parts[1] == "pocketcontext" && parts[2] == "runs" && parts[4] == "check" {
		return read
	}
	if len(parts) >= 4 && parts[0] == "api" && parts[1] == "collections" && collections[parts[2]] && parts[3] == "records" {
		if len(parts) == 4 {
			return read || method == http.MethodPost
		}
		if len(parts) == 5 && parts[4] != "" {
			return read || method == http.MethodPatch
		}
	}
	return read && len(parts) == 5 && parts[0] == "api" && parts[1] == "files" && sources[parts[2]] && parts[3] != "" && parts[4] != ""
}
