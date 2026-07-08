package docstore

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// This file is the files ↔ collection bridge: the migration path for
// apps that stored documents as JSON files ("CRUD by file names",
// literally), and the export path for anyone who wants their documents
// back as plain files.

// ImportFile stores one JSON file as a document; the id is the file
// name without its .json extension. Invalid JSON is rejected.
func (c *Collection) ImportFile(path string) (err error) {
	defer func(start time.Time) { c.logOp("importFile", path, start, err) }(time.Now())
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if !json.Valid(raw) {
		return fmt.Errorf("docstore: %s is not valid JSON", path)
	}
	id := strings.TrimSuffix(filepath.Base(path), ".json")
	return c.Put(id, raw)
}

// ImportDir walks dir recursively and stores every *.json file; ids are
// the slash-separated relative paths without the .json extension (so
// "users/id1.json" becomes id "users/id1"). Returns the number of
// imported documents; the first invalid file aborts with an error.
func (c *Collection) ImportDir(dir string) (n int, err error) {
	defer func(start time.Time) { c.logOp("importDir", dir, start, err) }(time.Now())
	err = filepath.WalkDir(dir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() || !strings.HasSuffix(d.Name(), ".json") {
			return nil
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if !json.Valid(raw) {
			return fmt.Errorf("docstore: %s is not valid JSON", path)
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		id := strings.TrimSuffix(filepath.ToSlash(rel), ".json")
		if err := c.Put(id, raw); err != nil {
			return fmt.Errorf("docstore: import %s: %w", path, err)
		}
		n++
		return nil
	})
	return n, err
}

// ExportDir writes every (live) document to dir as <id>.json (pretty
// printed); path-like ids become subdirectories. Returns the number of
// exported documents. Ids that would escape dir are rejected.
func (c *Collection) ExportDir(dir string) (n int, err error) {
	defer func(start time.Time) { c.logOp("exportDir", dir, start, err) }(time.Now())
	err = c.Each(func(id string, raw []byte) error {
		dst := filepath.Join(dir, filepath.FromSlash(id)+".json")
		rel, err := filepath.Rel(dir, dst)
		if err != nil || strings.HasPrefix(rel, "..") {
			return fmt.Errorf("docstore: id %q escapes export dir", id)
		}
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		var pretty []byte
		var v any
		if json.Unmarshal(raw, &v) == nil {
			pretty, _ = json.MarshalIndent(v, "", "  ")
		} else {
			pretty = raw
		}
		if err := os.WriteFile(dst, append(pretty, '\n'), 0o600); err != nil {
			return err
		}
		n++
		return nil
	})
	return n, err
}
