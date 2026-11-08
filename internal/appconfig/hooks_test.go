package appconfig

import (
	"bytes"
	"encoding/json"
	"io/fs"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"regexp"
	"strings"
	"testing"

	assets "github.com/pocketcontext-labs/pocketcontext-atlas"
)

// Hooks and migrations must not reach the operating system: $os exposes
// process creation ($os.cmd, $os.exec, $os.startProcess), and external tools
// such as sha256sum are missing on stock Windows. File access goes through
// app.newFilesystem(); anything else belongs in registerHookBindings.
func TestHooksStartNoProcesses(t *testing.T) {
	forbidden := regexp.MustCompile(`\$os\b|\bchild_process\b`)
	scanned := 0
	for _, dir := range []string{"pb_hooks", "pb_migrations"} {
		err := fs.WalkDir(assets.Assets, dir, func(name string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(name, ".js") {
				return err
			}
			data, err := assets.Assets.ReadFile(name)
			if err != nil {
				return err
			}
			scanned++
			for i, line := range strings.Split(string(data), "\n") {
				if m := forbidden.FindString(line); m != "" {
					t.Errorf("%s:%d uses %s; hooks must not depend on external tools or OS processes", name, i+1, m)
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	if scanned == 0 {
		t.Fatal("no embedded hooks or migrations scanned")
	}
}

// patternReader yields byte i%251 at offset i, n bytes in total.
// syntheticNote has a BOM, CRLF line endings and multi-byte UTF-8.
const syntheticNote = "\ufeff---\r\ntitle: Synthetic note\r\n---\r\n# Café — budget\r\n\r\nLine with tab\there and emoji 📝.\n"

// The real hooks hash an uploaded original in-process and store the same digest.
func TestSourceOriginalHash(t *testing.T) {
	a := newTestApp(t)
	app, handler, token := a.app, a.handler, a.token

	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	_ = w.WriteField("title", "Synthetic note")
	_ = w.WriteField("original_name", "note.md")
	_ = w.WriteField("media_type", "text/markdown")
	header := textproto.MIMEHeader{}
	header.Set("Content-Disposition", `form-data; name="original"; filename="note.md"`)
	header.Set("Content-Type", "application/octet-stream")
	part, err := w.CreatePart(header)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = part.Write([]byte(syntheticNote))
	_ = w.Close()
	req := httptest.NewRequest(http.MethodPost, "/api/collections/sources/records", &body)
	req.Header.Set("Content-Type", w.FormDataContentType())
	req.Header.Set("Authorization", token)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, req)
	if response.Code != http.StatusOK {
		t.Fatalf("upload: %d %s", response.Code, response.Body)
	}
	var created map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	stored, err := app.FindRecordById("sources", created["id"].(string))
	if err != nil {
		t.Fatal(err)
	}
	const want = "f07b4bcb6d3e4ef1378a96d7975e0b6174d31780dd5435ac8be90493744d18f4"
	if got := stored.GetString("sha256"); got != want {
		t.Fatalf("stored sha256 = %q; want %s", got, want)
	}
}
