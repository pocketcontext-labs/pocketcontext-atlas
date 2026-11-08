// Package appconfig constructs the Atlas runtime on pocketcontext-core.
package appconfig

import (
	"path/filepath"
	"slices"

	"ganigeorgiev.com/pocketbase/pocketbase/core"

	"github.com/pocketcontext-labs/pocketcontext-core/appidentity"
	"github.com/pocketcontext-labs/pocketcontext-core/appserver"
	"github.com/pocketcontext-labs/pocketcontext-core/profiles"

	assets "github.com/pocketcontext-labs/pocketcontext-atlas"
	"github.com/pocketcontext-labs/pocketcontext-atlas/internal/atlaspolicy"
	"github.com/pocketcontext-labs/pocketcontext-atlas/internal/buildinfo"
	"github.com/pocketcontext-labs/pocketcontext-atlas/internal/pocketcontext/server"
)

// Identity is Atlas's validated identity.
var Identity appidentity.Identity = atlaspolicy.Identity

// Runtime is the one validated runtime of this process.
var Runtime = mustRuntime()

// Profiles is this application's private profile registry.
var Profiles *profiles.Store = Runtime.Profiles()

// Hooks returns the Atlas product code at each point the server diverges.
func Hooks() appserver.Hooks {
	return appserver.Hooks{
		Register: func(app core.App, ctx appserver.RegisterContext) error {
			server.RegisterDatabase(app, filepath.Join(ctx.AppDir, "server.json"), ctx.LivePath)
			return nil
		},
	}
}

func mustRuntime() *appserver.Runtime {
	rt, err := appserver.New(appserver.Config{
		Identity: atlaspolicy.Identity, Version: buildinfo.Version, Commit: buildinfo.Commit, Assets: assets.Assets,
		Policy: atlaspolicy.Policy(), Hooks: Hooks(), Text: atlaspolicy.Text,
	})
	if err != nil {
		panic(err)
	}
	return rt
}

// KnownApp reports whether an app id belongs to this application or an earlier
// release of it. An earlier id may be diagnosed and stopped, never adopted.
func KnownApp(app string) bool {
	return app == Identity.AppID || slices.Contains(Identity.LegacyAppIDs, app)
}
