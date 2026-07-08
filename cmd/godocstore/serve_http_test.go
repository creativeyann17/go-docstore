// cmd/godocstore/serve_http_test.go
package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/creativeyann17/go-docstore"
)

func serveFixture(t *testing.T) *httptest.Server {
	t.Helper()
	ds, err := docstore.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ds.Close() })
	c, err := ds.Collection("users")
	if err != nil {
		t.Fatal(err)
	}
	c.Put("u1", map[string]any{"login": "yann", "quota": 100})
	c.Put("u2", map[string]any{"login": "bob", "quota": 5})

	srv := httptest.NewServer(newServeMux(ds))
	t.Cleanup(srv.Close)
	return srv
}

func get(t *testing.T, srv *httptest.Server, path string, out any) int {
	t.Helper()
	res, err := http.Get(srv.URL + path)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	json.NewDecoder(res.Body).Decode(out)
	return res.StatusCode
}

func TestServeCollectionsAndIds(t *testing.T) {
	srv := serveFixture(t)

	var colls []struct {
		Name  string `json:"name"`
		Count int    `json:"count"`
	}
	if code := get(t, srv, "/api/collections", &colls); code != 200 {
		t.Fatalf("collections status %d", code)
	}
	if len(colls) != 1 || colls[0].Name != "users" || colls[0].Count != 2 {
		t.Fatalf("collections: %+v", colls)
	}

	var ids []string
	get(t, srv, "/api/ids?c=users", &ids)
	if len(ids) != 2 || ids[0] != "u1" {
		t.Fatalf("ids: %v", ids)
	}
}

func TestServeDocRoundTrip(t *testing.T) {
	srv := serveFixture(t)

	var doc struct {
		ID  string          `json:"id"`
		Doc json.RawMessage `json:"doc"`
	}
	if code := get(t, srv, "/api/doc?c=users&id=u1", &doc); code != 200 {
		t.Fatalf("doc status %d", code)
	}
	if !strings.Contains(string(doc.Doc), "yann") {
		t.Fatalf("doc content: %s", doc.Doc)
	}

	// Edit via PUT, invalid JSON rejected.
	req, _ := http.NewRequest("PUT", srv.URL+"/api/doc?c=users&id=u1", strings.NewReader("{nope"))
	res, _ := http.DefaultClient.Do(req)
	if res.StatusCode != 400 {
		t.Fatalf("invalid JSON accepted: %d", res.StatusCode)
	}
	req, _ = http.NewRequest("PUT", srv.URL+"/api/doc?c=users&id=u1", strings.NewReader(`{"login":"yann","quota":999}`))
	res, _ = http.DefaultClient.Do(req)
	if res.StatusCode != 200 {
		t.Fatalf("PUT failed: %d", res.StatusCode)
	}
	get(t, srv, "/api/doc?c=users&id=u1", &doc)
	if !strings.Contains(string(doc.Doc), "999") {
		t.Fatalf("edit not persisted: %s", doc.Doc)
	}

	if code := get(t, srv, "/api/doc?c=users&id=ghost", &struct{}{}); code != 404 {
		t.Fatalf("missing doc status %d, want 404", code)
	}
}

func TestServeSoftDeleteLifecycle(t *testing.T) {
	srv := serveFixture(t)

	req, _ := http.NewRequest("DELETE", srv.URL+"/api/doc?c=users&id=u2&soft=1", nil)
	if res, _ := http.DefaultClient.Do(req); res.StatusCode != 200 {
		t.Fatalf("soft delete: %d", res.StatusCode)
	}
	var ids []string
	get(t, srv, "/api/ids?c=users", &ids)
	if len(ids) != 1 {
		t.Fatalf("soft-deleted still listed: %v", ids)
	}
	get(t, srv, "/api/ids?c=users&deleted=1", &ids)
	if len(ids) != 1 || ids[0] != "u2" {
		t.Fatalf("deleted list: %v", ids)
	}
	if res, _ := http.Post(srv.URL+"/api/restore?c=users&id=u2", "", nil); res.StatusCode != 200 {
		t.Fatalf("restore: %d", res.StatusCode)
	}
	get(t, srv, "/api/ids?c=users", &ids)
	if len(ids) != 2 {
		t.Fatalf("restore not effective: %v", ids)
	}
}

func TestServeFindAndSQL(t *testing.T) {
	srv := serveFixture(t)

	var hits []struct {
		ID string `json:"id"`
	}
	get(t, srv, "/api/find?c=users&path=$.quota&op=>&value=50&limit=0", &hits)
	if len(hits) != 1 || hits[0].ID != "u1" {
		t.Fatalf("find: %+v", hits)
	}

	res, err := http.Post(srv.URL+"/api/sql", "application/json",
		strings.NewReader(`{"query":"SELECT id FROM c_users ORDER BY id"}`))
	if err != nil {
		t.Fatal(err)
	}
	var rows []map[string]any
	json.NewDecoder(res.Body).Decode(&rows)
	if len(rows) != 2 || rows[0]["id"] != "u1" {
		t.Fatalf("sql rows: %+v", rows)
	}
}

func TestServeUIRoot(t *testing.T) {
	srv := serveFixture(t)
	res, err := http.Get(srv.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != 200 || !strings.Contains(res.Header.Get("Content-Type"), "text/html") {
		t.Fatalf("root: %d %s", res.StatusCode, res.Header.Get("Content-Type"))
	}
}
