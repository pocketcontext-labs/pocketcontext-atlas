package atlaspolicy

import (
	"encoding/json"
	"os"
	"testing"
)

// routeCase is one row of the frozen public route table. The JSON file stays
// byte-identical across the move of the allowlist out of this package.
type routeCase struct {
	Method string `json:"method"`
	Path   string `json:"path"`
	Allow  bool   `json:"allow"`
}

const publicRouteTable = "../appconfig/testdata/public_routes.json"

// routeTableCollections mirrors what the public handler builds from the app: each
// allowed collection by name, plus a stable fake id for sources and pages.
func routeTableCollections() (collections map[string]bool, sources map[string]bool) {
	collections = map[string]bool{"pbc_pages_fixture": true, "pbc_sources_fixture": true}
	for _, name := range publicCollections {
		collections[name] = true
	}
	return collections, map[string]bool{"sources": true, "pbc_sources_fixture": true}
}

func TestPublicRouteTable(t *testing.T) {
	data, err := os.ReadFile(publicRouteTable)
	if err != nil {
		t.Fatal(err)
	}
	var table []routeCase
	if err := json.Unmarshal(data, &table); err != nil {
		t.Fatal(err)
	}
	if len(table) < 400 {
		t.Fatalf("table has %d rows", len(table))
	}
	collections, sources := routeTableCollections()
	allowed := 0
	for _, c := range table {
		if c.Allow {
			allowed++
		}
		if got := publicRoute(c.Method, c.Path, collections, sources); got != c.Allow {
			t.Errorf("%s %s: allowed=%v, want %v", c.Method, c.Path, got, c.Allow)
		}
	}
	if allowed == 0 {
		t.Fatal("table allows nothing")
	}
}
