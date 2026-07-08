package docstore

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

type doc struct {
	Login string `json:"login"`
	Email string `json:"email"`
	N     int    `json:"n"`
}

func open(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func coll(t *testing.T, s *Store, opts ...Option) *Collection {
	t.Helper()
	c, err := s.Collection("things", opts...)
	if err != nil {
		t.Fatalf("collection: %v", err)
	}
	return c
}

func TestCRUDRoundTrip(t *testing.T) {
	c := coll(t, open(t))

	if err := c.Put("a", doc{Login: "yann", N: 1}); err != nil {
		t.Fatalf("put: %v", err)
	}
	var got doc
	if err := c.Get("a", &got); err != nil || got.Login != "yann" {
		t.Fatalf("get: %v %+v", err, got)
	}
	if err := c.Put("a", doc{Login: "yann", N: 2}); err != nil {
		t.Fatalf("overwrite: %v", err)
	}
	if err := c.Get("a", &got); err != nil || got.N != 2 {
		t.Fatalf("get after overwrite: %v %+v", err, got)
	}
	if err := c.Delete("a"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if err := c.Get("a", &got); !errors.Is(err, ErrNotFound) {
		t.Fatalf("get deleted = %v, want ErrNotFound", err)
	}
	if err := c.Delete("a"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("delete missing = %v, want ErrNotFound", err)
	}
}

func TestPathLikeIDs(t *testing.T) {
	c := coll(t, open(t))
	id := "user-123/upload-456"
	if err := c.Put(id, doc{N: 7}); err != nil {
		t.Fatalf("put: %v", err)
	}
	var got doc
	if err := c.Get(id, &got); err != nil || got.N != 7 {
		t.Fatalf("get: %v %+v", err, got)
	}
	ids, err := c.IDs()
	if err != nil || len(ids) != 1 || ids[0] != id {
		t.Fatalf("ids: %v %v", err, ids)
	}
}

func TestInsertConflicts(t *testing.T) {
	c := coll(t, open(t), WithUniqueIndex("login", "$.login"))

	if err := c.Insert("a", doc{Login: "yann"}); err != nil {
		t.Fatalf("insert: %v", err)
	}
	if err := c.Insert("a", doc{Login: "other"}); !errors.Is(err, ErrExists) {
		t.Fatalf("insert dup id = %v, want ErrExists", err)
	}
	if err := c.Insert("b", doc{Login: "yann"}); !errors.Is(err, ErrExists) {
		t.Fatalf("insert dup login = %v, want ErrExists", err)
	}
}

func TestIndexedLookups(t *testing.T) {
	c := coll(t, open(t),
		WithUniqueIndex("login", "$.login"),
		WithIndex("email", "$.email", true))

	c.Put("a", doc{Login: "yann", Email: "Yann@Example.com"})
	c.Put("b", doc{Login: "bob", Email: "bob@example.com"})

	var got doc
	if err := c.GetBy("login", "bob", &got); err != nil || got.Login != "bob" {
		t.Fatalf("GetBy login: %v %+v", err, got)
	}
	if err := c.GetBy("login", "nope", &got); !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetBy missing = %v, want ErrNotFound", err)
	}

	ok, err := c.ExistsBy("email", "yann@example.COM", "")
	if err != nil || !ok {
		t.Fatalf("ExistsBy nocase = %v %v, want true", ok, err)
	}
	ok, _ = c.ExistsBy("email", "yann@example.com", "a")
	if ok {
		t.Fatal("ExistsBy with exceptID matched the excluded doc")
	}
}

func TestUpdateTransactionalNoLostUpdates(t *testing.T) {
	c := coll(t, open(t))
	if err := c.Put("counter", doc{N: 0}); err != nil {
		t.Fatal(err)
	}

	const workers, rounds = 8, 25
	var wg sync.WaitGroup
	for range workers {
		wg.Go(func() {
			for range rounds {
				err := c.Update("counter", func(raw []byte) ([]byte, error) {
					var d doc
					if err := json.Unmarshal(raw, &d); err != nil {
						return nil, err
					}
					d.N++
					return json.Marshal(d)
				})
				if err != nil {
					t.Errorf("update: %v", err)
					return
				}
			}
		})
	}
	wg.Wait()

	var got doc
	if err := c.Get("counter", &got); err != nil {
		t.Fatal(err)
	}
	if got.N != workers*rounds {
		t.Fatalf("lost updates: N = %d, want %d", got.N, workers*rounds)
	}
}

func TestUpdateMissingAndAbort(t *testing.T) {
	c := coll(t, open(t))
	if err := c.Update("nope", func(raw []byte) ([]byte, error) { return raw, nil }); !errors.Is(err, ErrNotFound) {
		t.Fatalf("update missing = %v, want ErrNotFound", err)
	}

	c.Put("a", doc{N: 1})
	boom := errors.New("boom")
	if err := c.Update("a", func([]byte) ([]byte, error) { return nil, boom }); !errors.Is(err, boom) {
		t.Fatalf("update abort = %v, want boom", err)
	}
	var got doc
	c.Get("a", &got)
	if got.N != 1 {
		t.Fatalf("aborted update mutated doc: %+v", got)
	}
}

func TestPersistenceAcrossReopen(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.db")

	s1, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	c1, _ := s1.Collection("things", WithUniqueIndex("login", "$.login"))
	c1.Put("a", doc{Login: "yann", N: 42})
	s1.Close()

	s2, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()
	c2, err := s2.Collection("things", WithUniqueIndex("login", "$.login"))
	if err != nil {
		t.Fatalf("reopen collection (idempotent DDL): %v", err)
	}
	var got doc
	if err := c2.GetBy("login", "yann", &got); err != nil || got.N != 42 {
		t.Fatalf("reopen get: %v %+v", err, got)
	}
}

func TestEachAndCount(t *testing.T) {
	c := coll(t, open(t))
	c.Put("b", doc{N: 2})
	c.Put("a", doc{N: 1})

	var seen []string
	err := c.Each(func(id string, raw []byte) error {
		seen = append(seen, id)
		return nil
	})
	if err != nil || len(seen) != 2 || seen[0] != "a" || seen[1] != "b" {
		t.Fatalf("each: %v %v", err, seen)
	}
	if n, _ := c.Count(); n != 2 {
		t.Fatalf("count = %d", n)
	}
}

func TestMetaTimestamps(t *testing.T) {
	c := coll(t, open(t))
	before := time.Now().Add(-time.Second)

	c.Put("a", doc{N: 1})
	m1, err := c.Meta("a")
	if err != nil {
		t.Fatal(err)
	}
	if m1.CreatedAt.Before(before) || m1.DeletedAt != nil {
		t.Fatalf("fresh meta wrong: %+v", m1)
	}

	time.Sleep(5 * time.Millisecond)
	c.Put("a", doc{N: 2}) // overwrite must keep created_at, bump updated_at
	m2, _ := c.Meta("a")
	if !m2.CreatedAt.Equal(m1.CreatedAt) {
		t.Fatalf("created_at changed on overwrite: %v -> %v", m1.CreatedAt, m2.CreatedAt)
	}
	if !m2.UpdatedAt.After(m1.UpdatedAt) {
		t.Fatalf("updated_at not bumped: %v -> %v", m1.UpdatedAt, m2.UpdatedAt)
	}

	if _, err := c.Meta("nope"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("meta missing = %v", err)
	}
}

func TestSoftDeleteLifecycle(t *testing.T) {
	c := coll(t, open(t), WithSoftDelete())
	c.Put("a", doc{Login: "yann", N: 1})
	c.Put("b", doc{Login: "bob", N: 2})

	if err := c.Delete("a"); err != nil {
		t.Fatalf("soft delete: %v", err)
	}

	// Hidden from every live read.
	var got doc
	if err := c.Get("a", &got); !errors.Is(err, ErrNotFound) {
		t.Fatalf("get soft-deleted = %v, want ErrNotFound", err)
	}
	if ids, _ := c.IDs(); len(ids) != 1 || ids[0] != "b" {
		t.Fatalf("ids include soft-deleted: %v", ids)
	}
	if n, _ := c.Count(); n != 1 {
		t.Fatalf("count includes soft-deleted: %d", n)
	}
	if err := c.Update("a", func(r []byte) ([]byte, error) { return r, nil }); !errors.Is(err, ErrNotFound) {
		t.Fatalf("update soft-deleted = %v, want ErrNotFound", err)
	}

	// But visible through the dedicated paths.
	m, err := c.Meta("a")
	if err != nil || m.DeletedAt == nil {
		t.Fatalf("meta of soft-deleted: %v %+v", err, m)
	}
	var deleted []string
	c.EachDeleted(func(id string, raw []byte) error { deleted = append(deleted, id); return nil })
	if len(deleted) != 1 || deleted[0] != "a" {
		t.Fatalf("eachDeleted: %v", deleted)
	}

	// Restore brings it back.
	if err := c.Restore("a"); err != nil {
		t.Fatalf("restore: %v", err)
	}
	if err := c.Get("a", &got); err != nil || got.N != 1 {
		t.Fatalf("get after restore: %v %+v", err, got)
	}
	if err := c.Restore("a"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("restore live doc = %v, want ErrNotFound", err)
	}

	// Purge removes for real.
	c.Delete("a")
	if err := c.Purge("a"); err != nil {
		t.Fatalf("purge: %v", err)
	}
	if _, err := c.Meta("a"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("meta after purge = %v, want ErrNotFound", err)
	}
}

func TestPutRevivesSoftDeleted(t *testing.T) {
	c := coll(t, open(t), WithSoftDelete())
	c.Put("a", doc{N: 1})
	c.Delete("a")
	if err := c.Put("a", doc{N: 5}); err != nil {
		t.Fatalf("put over soft-deleted: %v", err)
	}
	var got doc
	if err := c.Get("a", &got); err != nil || got.N != 5 {
		t.Fatalf("revived doc: %v %+v", err, got)
	}
}

func TestFind(t *testing.T) {
	c := coll(t, open(t))
	c.Put("a", doc{Login: "yann", N: 10})
	c.Put("b", doc{Login: "bob", N: 20})
	c.Put("c", doc{Login: "yara", N: 30})

	eq, err := c.Find("$.login", "=", "bob", 0)
	if err != nil || len(eq) != 1 || eq[0].ID != "b" {
		t.Fatalf("find =: %v %v", err, eq)
	}
	gt, err := c.Find("$.n", ">", 15, 0)
	if err != nil || len(gt) != 2 {
		t.Fatalf("find > numeric: %v %v", err, gt)
	}
	like, err := c.Find("$.login", "like", "ya%", 0)
	if err != nil || len(like) != 2 {
		t.Fatalf("find like: %v %v", err, like)
	}
	limited, err := c.Find("$.n", ">", 0, 2)
	if err != nil || len(limited) != 2 {
		t.Fatalf("find limit: %v %v", err, limited)
	}
	if _, err := c.Find("$.n", "DROP", 1, 0); err == nil {
		t.Fatal("invalid op accepted")
	}
}

// TestUpgradeFromEmbeddedSchema simulates a database created by the
// pre-extraction version (mist-drive's embedded docstore: id/doc/
// updated_at only) and asserts the in-place upgrade.
func TestUpgradeFromEmbeddedSchema(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "old.db")

	s1, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	// Hand-create the OLD shape, exactly like the embedded ancestor.
	if _, err := s1.DB().Exec(`CREATE TABLE c_users (id TEXT PRIMARY KEY, doc TEXT NOT NULL, updated_at INTEGER NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	if _, err := s1.DB().Exec(`INSERT INTO c_users (id, doc, updated_at) VALUES ('u1', '{"login":"yann"}', 1700000000000)`); err != nil {
		t.Fatal(err)
	}
	s1.Close()

	s2, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()
	c, err := s2.Collection("users", WithUniqueIndex("login", "$.login"))
	if err != nil {
		t.Fatalf("upgrade collection: %v", err)
	}
	var got doc
	if err := c.GetBy("login", "yann", &got); err != nil {
		t.Fatalf("read after upgrade: %v", err)
	}
	m, err := c.Meta("u1")
	if err != nil {
		t.Fatalf("meta after upgrade: %v", err)
	}
	if m.CreatedAt.UnixMilli() != 1700000000000 {
		t.Fatalf("created_at not backfilled from updated_at: %v", m.CreatedAt.UnixMilli())
	}
}

func TestCollectionsListing(t *testing.T) {
	s := open(t)
	s.Collection("users")
	s.Collection("compress-queue") // '-' maps to '_' in the table name

	names, err := s.Collections()
	if err != nil || len(names) != 2 {
		t.Fatalf("collections: %v %v", err, names)
	}
	if names[0] != "compress_queue" || names[1] != "users" {
		t.Fatalf("unexpected names: %v", names)
	}
}

func TestImportExportRoundTrip(t *testing.T) {
	c := coll(t, open(t))

	// Seed a legacy-style tree: flat file + nested dir.
	src := t.TempDir()
	os.WriteFile(filepath.Join(src, "alice.json"), []byte(`{"login":"alice","n":1}`), 0o600)
	os.MkdirAll(filepath.Join(src, "sub"), 0o755)
	os.WriteFile(filepath.Join(src, "sub", "bob.json"), []byte(`{"login":"bob","n":2}`), 0o600)
	os.WriteFile(filepath.Join(src, "notes.txt"), []byte("ignored"), 0o600)

	n, err := c.ImportDir(src)
	if err != nil || n != 2 {
		t.Fatalf("importDir: %v n=%d", err, n)
	}
	var got doc
	if err := c.Get("sub/bob", &got); err != nil || got.N != 2 {
		t.Fatalf("nested import id: %v %+v", err, got)
	}

	dst := t.TempDir()
	n, err = c.ExportDir(dst)
	if err != nil || n != 2 {
		t.Fatalf("exportDir: %v n=%d", err, n)
	}
	b, err := os.ReadFile(filepath.Join(dst, "sub", "bob.json"))
	if err != nil || !json.Valid(b) {
		t.Fatalf("exported nested file: %v", err)
	}

	// Invalid JSON aborts.
	bad := t.TempDir()
	os.WriteFile(filepath.Join(bad, "broken.json"), []byte("{nope"), 0o600)
	if _, err := c.ImportDir(bad); err == nil {
		t.Fatal("importDir accepted invalid JSON")
	}
}

func TestImportFile(t *testing.T) {
	c := coll(t, open(t))
	src := filepath.Join(t.TempDir(), "carol.json")
	os.WriteFile(src, []byte(`{"login":"carol"}`), 0o600)
	if err := c.ImportFile(src); err != nil {
		t.Fatal(err)
	}
	var got doc
	if err := c.Get("carol", &got); err != nil || got.Login != "carol" {
		t.Fatalf("importFile: %v %+v", err, got)
	}
}
