# Changelog

## v0.2.0

- `FindPath(path, op, value, limit)` — friendlier search: dotted, index-free paths
  (`login`, `loginHistory.at`, no `$.` prefix, no `[0]`). If the first segment is
  an array, EVERY element is searched (not just index 0): array-of-objects paths
  match if any entry has the field, array-of-scalars paths match on membership.
  Used by the CLI `find` command and the serve UI's Find tab and key autocomplete.

- `godocstore serve` — embedded JSON-first debug web UI (single HTML file, no
  node toolchain): collections sidebar with live/deleted counts, filterable id
  list, syntax-highlighted document viewer with created/updated/deleted
  metadata, inline editor (client+server JSON validation, transactional save),
  delete/soft-delete/restore/purge, Find tab (JSON-path queries), SQL tab.
  Binds 127.0.0.1:8391 by default — the UI has no auth.
- Serve read paths are soft-delete aware regardless of collection options
  (live listings always exclude marked documents).
- Fixed: `Collection.Meta()` timestamps are now explicitly UTC (`time.UnixMilli`
  returns local server time by default — a Go quirk). The stored value was
  always a timezone-agnostic epoch-millis integer; only the Go-side conversion
  needed pinning down. The serve UI renders them in the viewer's local time.
- Serve UI polish: fixed a bug where clicking Edit made both the viewer and
  the editor disappear (an ID selector's own `display:none` rule silently
  defeated the JS toggle); Copy now copies the in-progress edit, not the
  last-saved document, while editing; "show deleted" is a real checkbox
  instead of a clickable div; Delete/Soft delete reordered with a visual
  separator so the destructive action reads as distinct; long ids no longer
  crowd the toolbar (id and timestamps are two lines); the id list scrolls
  properly instead of growing the page; the title is clickable (reload);
  Find state (query + results) is remembered per collection instead of
  leaking into whichever collection you switch to next.

## v0.1.0

- Initial release, extracted from mist-drive's embedded document store.
- Library: SQLite-backed JSON document collections ("CRUD by file names"):
  Put/Insert/Get/GetBy/ExistsBy/Delete/Update/Each/IDs/Count, generated-column
  indexes over JSON paths (unique + NOCASE), transactional read-modify-write
  `Update` (BEGIN IMMEDIATE), WAL + busy_timeout for multi-process safety.
- Metadata: created_at/updated_at on every document, `Meta(id)`; in-place
  schema upgrade for databases created by the embedded ancestor.
- Soft delete (opt-in `WithSoftDelete`): Delete marks, reads filter,
  `Restore`/`Purge`/`EachDeleted`.
- `Find(jsonPath, op, value, limit)` — search on JSON content (= != < > like), literal
  SQLite json_extract paths.
- Files bridge: `ImportFile`/`ImportDir` (migrate JSON files → documents),
  `ExportDir` (documents → pretty-printed files).
- Optional per-operation debug logging via `SetLogger(*slog.Logger)`.
- CLI `godocstore`: create/ls/get/put/edit/rm/restore/purge/find/sql/import/export/version.
