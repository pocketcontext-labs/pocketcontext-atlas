package appconfig

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"ganigeorgiev.com/pocketbase/pocketbase/core"
	"ganigeorgiev.com/pocketbase/pocketbase/tools/filesystem"
	contextserver "github.com/pocketcontext-labs/pocketcontext-atlas/internal/pocketcontext/server"
	"github.com/pocketcontext-labs/pocketcontext-core/appserver"
)

// Opt-in integration check: ordinary Go tests do not require Node or browsers.
// Build ui/dist and install ui's Playwright Chromium first, then run:
// POCKETCONTEXT_HTTPS_BROWSER_TEST=1 go test -tags "$(cat build-tags.txt)" ./internal/bundle -run TestPublicHTTPSPhoneBrowser -count=1
func TestPublicHTTPSPhoneBrowser(t *testing.T) {
	if os.Getenv("POCKETCONTEXT_HTTPS_BROWSER_TEST") != "1" {
		t.Skip("opt-in Playwright phone browser test")
	}
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	reader := filepath.Join(root, "ui", "dist")
	if _, err := os.Stat(filepath.Join(reader, "index.html")); err != nil {
		t.Fatal("build the reader before this test:", err)
	}
	a := newTestApp(t)
	spaces, err := a.app.FindRecordsByFilter("spaces", "", "", 1, 0)
	if err != nil {
		t.Fatal(err)
	}
	collection, err := a.app.FindCollectionByNameOrId("sources")
	if err != nil {
		t.Fatal(err)
	}
	source := core.NewRecord(collection)
	source.Set("space", spaces[0].Id)
	source.Set("title", "Synthetic phone evidence")
	source.Set("original_name", "phone.txt")
	source.Set("media_type", "text/plain")
	source.Set("revision", 1)
	file, err := filesystem.NewFileFromBytes([]byte("Synthetic phone reader evidence."), "phone.txt")
	if err != nil {
		t.Fatal(err)
	}
	source.Set("original", file)
	if err := a.app.Save(source); err != nil {
		t.Fatal(err)
	}
	contextserver.RegisterDatabase(a.app, filepath.Join(root, "server.json"), filepath.Join(a.app.DataDir(), "data.db"))
	// RegisterDatabase owns read/search connections outside PocketBase's pool.
	// Terminate them before newTestApp resets its database and removes fixtures.
	t.Cleanup(func() {
		if err := a.app.OnTerminate().Trigger(&core.TerminateEvent{App: a.app}); err != nil {
			t.Error(err)
		}
	})
	certServer := httptest.NewTLSServer(http.NotFoundHandler())
	tlsConfig := certServer.TLS.Clone()
	certServer.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	server, err := Runtime.NewTestServer(a.app, appserver.TestServerOptions{Domain: "knowledge.example.com", ReaderDir: reader, Cache: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	address, shutdown, err := server.StartTLS(ctx, "knowledge.example.com", tlsConfig)
	if err != nil {
		t.Fatal("phone test HTTPS listener:", err)
	}
	stop := func() {
		stopCtx, done := context.WithTimeout(context.Background(), 5*time.Second)
		defer done()
		if err := shutdown(stopCtx); err != nil {
			t.Fatal(err)
		}
	}
	code, err := server.IssueCode(a.user.Id, 2*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	commandCtx, done := context.WithTimeout(context.Background(), 90*time.Second)
	defer done()
	command := exec.CommandContext(commandCtx, "node", filepath.Join(root, "tests", "https_reader_browser.mjs"))
	_, port, err := net.SplitHostPort(address)
	if err != nil {
		t.Fatal(err)
	}
	command.Env = append(os.Environ(), "POCKETCONTEXT_SYNTHETIC_SIGNIN=https://knowledge.example.com/bundle/login#"+code, "POCKETCONTEXT_SYNTHETIC_PORT="+port)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("HTTPS phone browser: %v\n%s", err, output)
	}
	t.Log(string(output))
	cancel()
	stop()
}
