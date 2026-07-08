// cmd/godocstore/serve_http.go
package main

import (
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/creativeyann17/go-docstore"
)

//go:embed ui/index.html
var serveUI embed.FS

// newServeMux builds the debug-UI HTTP API over one store. Documents
// are addressed via the ?id= query parameter (NOT the path) because
// ids are allowed to contain slashes ("user-1/upload-42").
func newServeMux(ds *docstore.Store) http.Handler {
	mux := http.NewServeMux()

	writeErr := func(w http.ResponseWriter, code int, err error) {
		w.WriteHeader(code)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
	}
	writeJSON := func(w http.ResponseWriter, v any) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(v)
	}
	// collection opens the requested collection, optionally with soft
	// delete (for rm?soft=1 — Restore/Purge work without the option).
	collection := func(r *http.Request, soft bool) (*docstore.Collection, error) {
		name := r.URL.Query().Get("c")
		if name == "" {
			return nil, errors.New("missing ?c=<collection>")
		}
		var opts []docstore.Option
		if soft {
			opts = append(opts, docstore.WithSoftDelete())
		}
		return ds.Collection(name, opts...)
	}
	statusFor := func(err error) int {
		switch {
		case errors.Is(err, docstore.ErrNotFound):
			return http.StatusNotFound
		case errors.Is(err, docstore.ErrExists):
			return http.StatusConflict
		default:
			return http.StatusInternalServerError
		}
	}

	mux.HandleFunc("GET /api/collections", func(w http.ResponseWriter, r *http.Request) {
		names, err := ds.Collections()
		if err != nil {
			writeErr(w, 500, err)
			return
		}
		type entry struct {
			Name    string `json:"name"`
			Count   int    `json:"count"`
			Deleted int    `json:"deleted"`
		}
		out := []entry{}
		for _, name := range names {
			// Soft-aware open so Count() means "live documents" even on
			// collections the owning app treats as hard-delete.
			c, err := ds.Collection(name, docstore.WithSoftDelete())
			if err != nil {
				writeErr(w, 500, err)
				return
			}
			n, _ := c.Count()
			del := 0
			c.EachDeleted(func(string, []byte) error { del++; return nil })
			out = append(out, entry{Name: name, Count: n, Deleted: del})
		}
		writeJSON(w, out)
	})

	mux.HandleFunc("GET /api/ids", func(w http.ResponseWriter, r *http.Request) {
		c, err := collection(r, true) // live = not soft-deleted, always
		if err != nil {
			writeErr(w, 400, err)
			return
		}
		if r.URL.Query().Get("deleted") == "1" {
			ids := []string{}
			c.EachDeleted(func(id string, _ []byte) error { ids = append(ids, id); return nil })
			writeJSON(w, ids)
			return
		}
		ids, err := c.IDs()
		if err != nil {
			writeErr(w, 500, err)
			return
		}
		if ids == nil {
			ids = []string{}
		}
		writeJSON(w, ids)
	})

	mux.HandleFunc("GET /api/doc", func(w http.ResponseWriter, r *http.Request) {
		c, err := collection(r, false)
		if err != nil {
			writeErr(w, 400, err)
			return
		}
		id := r.URL.Query().Get("id")
		meta, err := c.Meta(id)
		if err != nil {
			writeErr(w, statusFor(err), err)
			return
		}
		// Meta answers for soft-deleted docs; fetch raw through SQL so
		// the viewer can inspect them too.
		var raw string
		table := "c_" + strings.ReplaceAll(r.URL.Query().Get("c"), "-", "_")
		if err := ds.DB().QueryRow(
			fmt.Sprintf(`SELECT doc FROM %s WHERE id = ?`, table), id).Scan(&raw); err != nil {
			writeErr(w, 500, err)
			return
		}
		writeJSON(w, map[string]any{
			"id":        id,
			"doc":       json.RawMessage(raw),
			"createdAt": meta.CreatedAt,
			"updatedAt": meta.UpdatedAt,
			"deletedAt": meta.DeletedAt,
		})
	})

	mux.HandleFunc("PUT /api/doc", func(w http.ResponseWriter, r *http.Request) {
		c, err := collection(r, false)
		if err != nil {
			writeErr(w, 400, err)
			return
		}
		id := r.URL.Query().Get("id")
		if id == "" {
			writeErr(w, 400, errors.New("missing ?id="))
			return
		}
		body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 8<<20))
		if err != nil {
			writeErr(w, 400, err)
			return
		}
		if !json.Valid(body) {
			writeErr(w, 400, errors.New("body is not valid JSON"))
			return
		}
		if err := c.Put(id, body); err != nil {
			writeErr(w, statusFor(err), err)
			return
		}
		writeJSON(w, map[string]string{"status": "ok"})
	})

	mux.HandleFunc("DELETE /api/doc", func(w http.ResponseWriter, r *http.Request) {
		soft := r.URL.Query().Get("soft") == "1"
		c, err := collection(r, soft)
		if err != nil {
			writeErr(w, 400, err)
			return
		}
		if err := c.Delete(r.URL.Query().Get("id")); err != nil {
			writeErr(w, statusFor(err), err)
			return
		}
		writeJSON(w, map[string]string{"status": "ok"})
	})

	mux.HandleFunc("POST /api/restore", func(w http.ResponseWriter, r *http.Request) {
		c, err := collection(r, false)
		if err != nil {
			writeErr(w, 400, err)
			return
		}
		if err := c.Restore(r.URL.Query().Get("id")); err != nil {
			writeErr(w, statusFor(err), err)
			return
		}
		writeJSON(w, map[string]string{"status": "ok"})
	})

	mux.HandleFunc("POST /api/purge", func(w http.ResponseWriter, r *http.Request) {
		c, err := collection(r, false)
		if err != nil {
			writeErr(w, 400, err)
			return
		}
		if err := c.Purge(r.URL.Query().Get("id")); err != nil {
			writeErr(w, statusFor(err), err)
			return
		}
		writeJSON(w, map[string]string{"status": "ok"})
	})

	// /api/keys powers the Find tab's path autocomplete: no fixed
	// schema exists (documents are free-form JSON), so instead we
	// sample a handful of real documents and walk their key tree.
	// Each stops early once the sample is full — errSampleDone is our
	// own break signal, not a real failure, so it's swallowed below.
	mux.HandleFunc("GET /api/keys", func(w http.ResponseWriter, r *http.Request) {
		c, err := collection(r, true)
		if err != nil {
			writeErr(w, 400, err)
			return
		}
		set := map[string]struct{}{}
		sampled := 0
		errSampleDone := errors.New("sample done")
		err = c.Each(func(id string, raw []byte) error {
			if sampled >= keySampleLimit {
				return errSampleDone
			}
			var v any
			if json.Unmarshal(raw, &v) == nil {
				extractPaths("", v, 0, set)
			}
			sampled++
			return nil
		})
		if err != nil && !errors.Is(err, errSampleDone) {
			writeErr(w, 500, err)
			return
		}
		writeJSON(w, sortedKeys(set))
	})

	mux.HandleFunc("GET /api/find", func(w http.ResponseWriter, r *http.Request) {
		c, err := collection(r, true) // search live documents only
		if err != nil {
			writeErr(w, 400, err)
			return
		}
		q := r.URL.Query()
		limit, _ := strconv.Atoi(q.Get("limit"))
		var value any = q.Get("value")
		if q.Get("string") != "1" {
			if n, err := strconv.ParseFloat(q.Get("value"), 64); err == nil {
				value = n
			}
		}
		docs, err := c.FindPath(q.Get("path"), q.Get("op"), value, limit)
		if err != nil {
			writeErr(w, 400, err)
			return
		}
		type hit struct {
			ID  string          `json:"id"`
			Doc json.RawMessage `json:"doc"`
		}
		out := []hit{}
		for _, d := range docs {
			out = append(out, hit{ID: d.ID, Doc: json.RawMessage(d.Raw)})
		}
		writeJSON(w, out)
	})

	mux.HandleFunc("POST /api/sql", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Query string `json:"query"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeErr(w, 400, err)
			return
		}
		q := strings.TrimSpace(req.Query)
		if !strings.HasPrefix(strings.ToUpper(q), "SELECT") {
			res, err := ds.DB().Exec(q)
			if err != nil {
				writeErr(w, 400, err)
				return
			}
			n, _ := res.RowsAffected()
			writeJSON(w, map[string]any{"rowsAffected": n})
			return
		}
		rows, err := ds.DB().Query(q)
		if err != nil {
			writeErr(w, 400, err)
			return
		}
		defer rows.Close()
		cols, _ := rows.Columns()
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		out := []map[string]any{}
		for rows.Next() {
			if err := rows.Scan(ptrs...); err != nil {
				writeErr(w, 500, err)
				return
			}
			m := map[string]any{}
			for i, col := range cols {
				if b, ok := vals[i].([]byte); ok {
					m[col] = string(b)
				} else {
					m[col] = vals[i]
				}
			}
			out = append(out, m)
		}
		writeJSON(w, out)
	})

	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		b, _ := serveUI.ReadFile("ui/index.html")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(b)
	})

	return mux
}
