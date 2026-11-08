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
	},
	BudgetExhausted: "Knowledge request budget exhausted. Retry after the indicated delay.",
	OTPSubject:      "Log in to PocketContext Atlas",
	OTPSenderName:   "PocketContext Atlas",
}

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
