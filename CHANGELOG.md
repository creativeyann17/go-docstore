# Changelog

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
- `Find(jsonPath, op, value, limit)` — search on JSON content (= != < > like).
- Files bridge: `ImportFile`/`ImportDir` (migrate JSON files → documents),
  `ExportDir` (documents → pretty-printed files).
- Optional per-operation debug logging via `SetLogger(*slog.Logger)`.
- CLI `godocstore`: create/ls/get/put/edit/rm/restore/purge/find/sql/import/export/version.
