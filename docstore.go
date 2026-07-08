// Package docstore is a small document store on SQLite: collections of
// JSON documents addressed by string ids — "CRUD by file names", with
// the directory/file mental model (path-like ids such as "uid/upload"
// are fine) but real multi-process safety.
//
// Design rules:
//   - Only stdlib + the pure-Go SQLite driver (modernc.org/sqlite, no cgo).
//   - No app-level cache: SQLite's page cache serves hot reads in
//     microseconds and is never stale across processes.
//   - The concurrency primitive is Update (transactional
//     read-modify-write, BEGIN IMMEDIATE) — it replaces file locks and
//     per-entity mutexes and is correct across processes.
//   - Indexed lookups use SQLite generated columns over JSON paths, so
//     documents stay schemaless while lookups stay O(log n).
//   - Every document carries created_at/updated_at metadata; soft
//     deletion (deleted_at) is opt-in per collection via WithSoftDelete.
package docstore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

var (
	ErrNotFound = errors.New("docstore: not found")
	ErrExists   = errors.New("docstore: already exists")
)

// Store is one SQLite database file holding any number of collections.
type Store struct {
	db  *sql.DB
	log *slog.Logger
}

// SetLogger routes per-operation debug records (op, collection, id,
// duration, error) through the given logger. Nil (the default) means
// silent — callers opt in, typically with their app logger at debug level.
func (s *Store) SetLogger(l *slog.Logger) { s.log = l }

// Open opens (or creates) the database file. WAL journaling for
// concurrent readers + one writer, busy_timeout so competing writers
// queue instead of erroring, txlock=immediate so Update transactions
// take their write lock up front (no deferred-upgrade SQLITE_BUSY).
func Open(path string) (*Store, error) {
	dsn := "file:" + path + "?_txlock=immediate" +
		"&_pragma=journal_mode(WAL)" +
		"&_pragma=busy_timeout(5000)" +
		"&_pragma=synchronous(NORMAL)" +
		"&_pragma=foreign_keys(ON)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, err
	}
	return &Store{db: db}, nil
}

func (s *Store) Close() error { return s.db.Close() }

// DB exposes the underlying handle for consumers that need raw SQL
// (reports, ad-hoc queries). The document tables are named c_<collection>.
func (s *Store) DB() *sql.DB { return s.db }

// Collections lists the collection names present in the database
// (tables matching the c_* naming scheme). Note: '-' in a collection
// name is stored as '_' in the table name, so names round-trip with
// underscores.
func (s *Store) Collections() ([]string, error) {
	rows, err := s.db.Query(
		`SELECT name FROM sqlite_master WHERE type = 'table' AND name LIKE 'c\_%' ESCAPE '\' ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		out = append(out, strings.TrimPrefix(name, "c_"))
	}
	return out, rows.Err()
}

// index describes one generated-column index over a JSON path.
type index struct {
	col      string
	jsonPath string
	unique   bool
	nocase   bool
}

// Option configures a Collection at open time.
type Option func(*Collection)

// WithUniqueIndex adds a UNIQUE index on json_extract(doc, jsonPath),
// exposed as a queryable column (GetBy/ExistsBy). Put/Insert of a
// conflicting document returns ErrExists.
func WithUniqueIndex(col, jsonPath string) Option {
	return func(c *Collection) {
		c.indexes = append(c.indexes, index{col: col, jsonPath: jsonPath, unique: true})
	}
}

// WithIndex adds a non-unique index; nocase makes lookups
// case-insensitive (à la strings.EqualFold).
func WithIndex(col, jsonPath string, nocase bool) Option {
	return func(c *Collection) {
		c.indexes = append(c.indexes, index{col: col, jsonPath: jsonPath, nocase: nocase})
	}
}

// WithSoftDelete makes Delete mark documents (deleted_at) instead of
// removing them; reads filter marked documents out, Restore/Purge
// manage them. CAVEAT with unique indexes: a soft-deleted document
// still holds its unique values, so re-creating "the same" document
// conflicts until purged — prefer hard delete (the default) or no
// unique indexes on soft-delete collections.
func WithSoftDelete() Option {
	return func(c *Collection) { c.softDelete = true }
}

// Collection is a named set of documents — think "directory".
type Collection struct {
	store      *Store
	name       string
	table      string
	indexes    []index
	softDelete bool
}

// Meta carries a document's metadata timestamps. DeletedAt is nil for
// live documents.
type Meta struct {
	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt *time.Time
}

// Doc is one raw document, as returned by Find.
type Doc struct {
	ID  string
	Raw []byte
}

// logOp emits one debug record per store call. The Enabled check keeps
// the cost near zero when debug logging is off.
func (c *Collection) logOp(op, id string, start time.Time, err error) {
	l := c.store.log
	if l == nil || !l.Enabled(context.Background(), slog.LevelDebug) {
		return
	}
	attrs := []any{"op", op, "collection", c.name, "id", id,
		"dur", time.Since(start).Round(time.Microsecond).String()}
	if err != nil {
		attrs = append(attrs, "err", err.Error())
	}
	l.Debug("docstore", attrs...)
}

var nameRe = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)
var colRe = regexp.MustCompile(`^[a-zA-Z0-9_]+$`)

// Collection opens (creating if needed) a collection and applies its
// indexes. Idempotent — and it upgrades tables created by older
// versions of this package in place (adds missing metadata columns).
func (s *Store) Collection(name string, opts ...Option) (*Collection, error) {
	if !nameRe.MatchString(name) {
		return nil, fmt.Errorf("docstore: invalid collection name %q", name)
	}
	c := &Collection{store: s, name: name, table: "c_" + strings.ReplaceAll(name, "-", "_")}
	for _, o := range opts {
		o(c)
	}

	if _, err := s.db.Exec(fmt.Sprintf(
		`CREATE TABLE IF NOT EXISTS %s (
			id TEXT PRIMARY KEY,
			doc TEXT NOT NULL,
			created_at INTEGER NOT NULL,
			updated_at INTEGER NOT NULL,
			deleted_at INTEGER
		)`, c.table)); err != nil {
		return nil, err
	}
	if err := c.upgradeSchema(); err != nil {
		return nil, err
	}

	for _, ix := range c.indexes {
		if !colRe.MatchString(ix.col) {
			return nil, fmt.Errorf("docstore: invalid index column %q", ix.col)
		}
		if err := c.ensureIndex(ix); err != nil {
			return nil, err
		}
	}
	return c, nil
}

// hasColumn reports whether the table already has a column.
func (c *Collection) hasColumn(col string) (bool, error) {
	var n int
	err := c.store.db.QueryRow(fmt.Sprintf(
		`SELECT COUNT(*) FROM pragma_table_xinfo('%s') WHERE name = ?`, c.table), col).Scan(&n)
	return n > 0, err
}

// upgradeSchema brings tables created by the pre-extraction embedded
// version (id, doc, updated_at) up to the current shape. created_at is
// backfilled from updated_at — the best information available.
func (c *Collection) upgradeSchema() error {
	if ok, err := c.hasColumn("created_at"); err != nil {
		return err
	} else if !ok {
		if _, err := c.store.db.Exec(fmt.Sprintf(
			`ALTER TABLE %s ADD COLUMN created_at INTEGER NOT NULL DEFAULT 0`, c.table)); err != nil {
			return err
		}
		if _, err := c.store.db.Exec(fmt.Sprintf(
			`UPDATE %s SET created_at = updated_at WHERE created_at = 0`, c.table)); err != nil {
			return err
		}
	}
	if ok, err := c.hasColumn("deleted_at"); err != nil {
		return err
	} else if !ok {
		if _, err := c.store.db.Exec(fmt.Sprintf(
			`ALTER TABLE %s ADD COLUMN deleted_at INTEGER`, c.table)); err != nil {
			return err
		}
	}
	return nil
}

// ensureIndex adds the generated column (if missing) and its index.
// Generated VIRTUAL columns cost nothing at rest — they're computed
// from doc on demand; the index materializes them for lookups.
func (c *Collection) ensureIndex(ix index) error {
	ok, err := c.hasColumn(ix.col)
	if err != nil {
		return err
	}
	if !ok {
		collate := ""
		if ix.nocase {
			collate = " COLLATE NOCASE"
		}
		if _, err := c.store.db.Exec(fmt.Sprintf(
			`ALTER TABLE %s ADD COLUMN %s TEXT%s GENERATED ALWAYS AS (json_extract(doc, '%s')) VIRTUAL`,
			c.table, ix.col, collate, ix.jsonPath)); err != nil {
			return err
		}
	}
	unique := ""
	if ix.unique {
		unique = "UNIQUE "
	}
	_, err = c.store.db.Exec(fmt.Sprintf(
		`CREATE %sINDEX IF NOT EXISTS ix_%s_%s ON %s(%s)`,
		unique, c.table, ix.col, c.table, ix.col))
	return err
}

// alive is the WHERE fragment hiding soft-deleted documents on
// soft-delete collections; hard-delete collections see every row.
func (c *Collection) alive() string {
	if c.softDelete {
		return " AND deleted_at IS NULL"
	}
	return ""
}

func isUniqueViolation(err error) bool {
	return err != nil && strings.Contains(err.Error(), "UNIQUE constraint failed")
}

func (c *Collection) marshal(doc any) ([]byte, error) {
	if raw, ok := doc.([]byte); ok {
		return raw, nil
	}
	return jsonMarshal(doc)
}

// Insert stores a NEW document; any conflict (id or unique index)
// returns ErrExists — including a conflict with a soft-deleted document,
// which still occupies its id.
func (c *Collection) Insert(id string, doc any) (err error) {
	defer func(start time.Time) { c.logOp("insert", id, start, err) }(time.Now())
	raw, err := c.marshal(doc)
	if err != nil {
		return err
	}
	now := time.Now().UnixMilli()
	_, err = c.store.db.Exec(fmt.Sprintf(
		`INSERT INTO %s (id, doc, created_at, updated_at) VALUES (?, ?, ?, ?)`, c.table),
		id, string(raw), now, now)
	if isUniqueViolation(err) {
		return ErrExists
	}
	return err
}

// Put creates or replaces the document with this id, keeping its
// original created_at. Putting over a soft-deleted document revives it
// (deleted_at cleared). A unique-index conflict with a DIFFERENT
// document still returns ErrExists.
func (c *Collection) Put(id string, doc any) (err error) {
	defer func(start time.Time) { c.logOp("put", id, start, err) }(time.Now())
	raw, err := c.marshal(doc)
	if err != nil {
		return err
	}
	now := time.Now().UnixMilli()
	_, err = c.store.db.Exec(fmt.Sprintf(
		`INSERT INTO %s (id, doc, created_at, updated_at) VALUES (?, ?, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET doc = excluded.doc, updated_at = excluded.updated_at, deleted_at = NULL`,
		c.table), id, string(raw), now, now)
	if isUniqueViolation(err) {
		return ErrExists
	}
	return err
}

// Get unmarshals the document with this id into out.
func (c *Collection) Get(id string, out any) (err error) {
	defer func(start time.Time) { c.logOp("get", id, start, err) }(time.Now())
	var raw string
	err = c.store.db.QueryRow(fmt.Sprintf(
		`SELECT doc FROM %s WHERE id = ?%s`, c.table, c.alive()), id).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	return jsonUnmarshal([]byte(raw), out)
}

// GetRaw returns the raw JSON of the document with this id.
func (c *Collection) GetRaw(id string) (raw []byte, err error) {
	defer func(start time.Time) { c.logOp("getRaw", id, start, err) }(time.Now())
	var s string
	err = c.store.db.QueryRow(fmt.Sprintf(
		`SELECT doc FROM %s WHERE id = ?%s`, c.table, c.alive()), id).Scan(&s)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return []byte(s), nil
}

// Meta returns the document's metadata timestamps. Unlike Get, it also
// answers for soft-deleted documents (that's how you inspect them).
func (c *Collection) Meta(id string) (m Meta, err error) {
	defer func(start time.Time) { c.logOp("meta", id, start, err) }(time.Now())
	var created, updated int64
	var deleted sql.NullInt64
	err = c.store.db.QueryRow(fmt.Sprintf(
		`SELECT created_at, updated_at, deleted_at FROM %s WHERE id = ?`, c.table), id).
		Scan(&created, &updated, &deleted)
	if errors.Is(err, sql.ErrNoRows) {
		return Meta{}, ErrNotFound
	}
	if err != nil {
		return Meta{}, err
	}
	m.CreatedAt = time.UnixMilli(created)
	m.UpdatedAt = time.UnixMilli(updated)
	if deleted.Valid {
		t := time.UnixMilli(deleted.Int64)
		m.DeletedAt = &t
	}
	return m, nil
}

// GetBy looks a document up through an indexed column (see WithIndex /
// WithUniqueIndex). With multiple matches the smallest id wins.
func (c *Collection) GetBy(col string, val any, out any) (err error) {
	defer func(start time.Time) { c.logOp("getBy", fmt.Sprintf("%s=%v", col, val), start, err) }(time.Now())
	if !colRe.MatchString(col) {
		return fmt.Errorf("docstore: invalid column %q", col)
	}
	var raw string
	err = c.store.db.QueryRow(fmt.Sprintf(
		`SELECT doc FROM %s WHERE %s = ?%s ORDER BY id LIMIT 1`, c.table, col, c.alive()), val).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	return jsonUnmarshal([]byte(raw), out)
}

// ExistsBy reports whether any document other than exceptID has this
// value in the indexed column (pass "" to check all documents).
func (c *Collection) ExistsBy(col string, val any, exceptID string) (found bool, err error) {
	defer func(start time.Time) { c.logOp("existsBy", fmt.Sprintf("%s=%v", col, val), start, err) }(time.Now())
	if !colRe.MatchString(col) {
		return false, fmt.Errorf("docstore: invalid column %q", col)
	}
	var one int
	err = c.store.db.QueryRow(fmt.Sprintf(
		`SELECT 1 FROM %s WHERE %s = ? AND id != ?%s LIMIT 1`, c.table, col, c.alive()), val, exceptID).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}

// Find returns documents whose json_extract(doc, jsonPath) matches
// value under op (one of = != < > like). value is used as-is: pass a
// number for numeric JSON fields, a string for text. limit <= 0 means
// no limit.
func (c *Collection) Find(jsonPath, op string, value any, limit int) (out []Doc, err error) {
	defer func(start time.Time) { c.logOp("find", fmt.Sprintf("%s %s %v", jsonPath, op, value), start, err) }(time.Now())
	sqlOp, ok := map[string]string{
		"=": "=", "!=": "!=", "<": "<", ">": ">", "like": "LIKE",
	}[strings.ToLower(op)]
	if !ok {
		return nil, fmt.Errorf("docstore: invalid op %q (want = != < > like)", op)
	}
	q := fmt.Sprintf(`SELECT id, doc FROM %s WHERE json_extract(doc, ?) %s ?%s ORDER BY id`,
		c.table, sqlOp, c.alive())
	if limit > 0 {
		q += fmt.Sprintf(" LIMIT %d", limit)
	}
	rows, err := c.store.db.Query(q, jsonPath, value)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var d Doc
		var raw string
		if err := rows.Scan(&d.ID, &raw); err != nil {
			return nil, err
		}
		d.Raw = []byte(raw)
		out = append(out, d)
	}
	return out, rows.Err()
}

// Delete removes the document — hard by default, or marks deleted_at on
// WithSoftDelete collections. ErrNotFound when there is nothing (live)
// to delete.
func (c *Collection) Delete(id string) (err error) {
	defer func(start time.Time) { c.logOp("delete", id, start, err) }(time.Now())
	var res sql.Result
	if c.softDelete {
		res, err = c.store.db.Exec(fmt.Sprintf(
			`UPDATE %s SET deleted_at = ? WHERE id = ? AND deleted_at IS NULL`, c.table),
			time.Now().UnixMilli(), id)
	} else {
		res, err = c.store.db.Exec(fmt.Sprintf(
			`DELETE FROM %s WHERE id = ?`, c.table), id)
	}
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// Restore clears a soft-deleted document's deleted_at mark. Available
// on any collection (it acts on the column, not the option).
func (c *Collection) Restore(id string) (err error) {
	defer func(start time.Time) { c.logOp("restore", id, start, err) }(time.Now())
	res, err := c.store.db.Exec(fmt.Sprintf(
		`UPDATE %s SET deleted_at = NULL WHERE id = ? AND deleted_at IS NOT NULL`, c.table), id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// Purge removes a document for real, regardless of soft-delete state.
func (c *Collection) Purge(id string) (err error) {
	defer func(start time.Time) { c.logOp("purge", id, start, err) }(time.Now())
	res, err := c.store.db.Exec(fmt.Sprintf(
		`DELETE FROM %s WHERE id = ?`, c.table), id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// IDs lists every (live) document id, sorted.
func (c *Collection) IDs() (out []string, err error) {
	defer func(start time.Time) { c.logOp("ids", "*", start, err) }(time.Now())
	rows, err := c.store.db.Query(fmt.Sprintf(
		`SELECT id FROM %s WHERE 1=1%s ORDER BY id`, c.table, c.alive()))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// Count returns the number of (live) documents.
func (c *Collection) Count() (n int, err error) {
	defer func(start time.Time) { c.logOp("count", "*", start, err) }(time.Now())
	err = c.store.db.QueryRow(fmt.Sprintf(
		`SELECT COUNT(*) FROM %s WHERE 1=1%s`, c.table, c.alive())).Scan(&n)
	return n, err
}

// Each streams every (live) (id, raw document) pair, sorted by id.
// Returning an error from fn stops the iteration and propagates it.
func (c *Collection) Each(fn func(id string, raw []byte) error) (err error) {
	defer func(start time.Time) { c.logOp("each", "*", start, err) }(time.Now())
	return c.each(fn, c.alive())
}

// EachDeleted streams soft-deleted documents only.
func (c *Collection) EachDeleted(fn func(id string, raw []byte) error) (err error) {
	defer func(start time.Time) { c.logOp("eachDeleted", "*", start, err) }(time.Now())
	return c.each(fn, " AND deleted_at IS NOT NULL")
}

func (c *Collection) each(fn func(id string, raw []byte) error, where string) error {
	rows, err := c.store.db.Query(fmt.Sprintf(
		`SELECT id, doc FROM %s WHERE 1=1%s ORDER BY id`, c.table, where))
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id, raw string
		if err := rows.Scan(&id, &raw); err != nil {
			return err
		}
		if err := fn(id, []byte(raw)); err != nil {
			return err
		}
	}
	return rows.Err()
}

// Update runs a transactional read-modify-write on one document: fn
// receives the current raw JSON and returns the replacement. The whole
// sequence holds the database write lock (BEGIN IMMEDIATE via the
// txlock DSN), so concurrent Updates — same process or another one —
// serialize instead of losing writes. fn returning an error aborts.
func (c *Collection) Update(id string, fn func(raw []byte) ([]byte, error)) (err error) {
	defer func(start time.Time) { c.logOp("update", id, start, err) }(time.Now())
	tx, err := c.store.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var raw string
	err = tx.QueryRow(fmt.Sprintf(
		`SELECT doc FROM %s WHERE id = ?%s`, c.table, c.alive()), id).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}

	next, err := fn([]byte(raw))
	if err != nil {
		return err
	}
	if _, err := tx.Exec(fmt.Sprintf(
		`UPDATE %s SET doc = ?, updated_at = ? WHERE id = ?`, c.table),
		string(next), time.Now().UnixMilli(), id); err != nil {
		if isUniqueViolation(err) {
			return ErrExists
		}
		return err
	}
	return tx.Commit()
}
