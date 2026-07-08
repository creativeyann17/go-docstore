# go-docstore

[![CI](https://github.com/creativeyann17/go-docstore/actions/workflows/test-and-release.yml/badge.svg)](https://github.com/creativeyann17/go-docstore/actions/workflows/test-and-release.yml)
[![Release](https://img.shields.io/github/v/release/creativeyann17/go-docstore)](https://github.com/creativeyann17/go-docstore/releases)
[![Go Version](https://img.shields.io/github/go-mod/go-version/creativeyann17/go-docstore)](go.mod)
[![License](https://img.shields.io/github/license/creativeyann17/go-docstore)](LICENSE)
[![Buy Me A Coffee](https://img.shields.io/badge/Buy%20Me%20A%20Coffee-support-orange?logo=buy-me-a-coffee&logoColor=white)](https://buymeacoffee.com/creativeyann17)

A tiny document store on SQLite for Go: collections of JSON documents
addressed by string ids — **"CRUD by file names"**. The mental model of a
directory of JSON files, with what that approach never gives you: real
multi-process safety, transactions, indexes and search.

Born inside [mist-drive](https://github.com/creativeyann17/mist-drive),
which stored users as JSON files with file locks until the locking,
atomic-rename and cache-invalidation code grew into an unlabeled,
untested database. This is that code done right — on top of the most
battle-tested storage engine in existence.

## Features

- **One file, many collections** — pure-Go SQLite (`modernc.org/sqlite`, no cgo)
- **CRUD by id**, path-like ids welcome (`"user-1/upload-42"`)
- **Transactional `Update`** (read-modify-write under the write lock) —
  no lost updates, in-process or across processes
- **Indexed lookups** via SQLite generated columns over JSON paths
  (unique and case-insensitive variants) — documents stay schemaless
- **`Find`** — search on JSON content (`= != < > like`)
- **Metadata for free** — `created_at`/`updated_at` per document, `Meta(id)`
- **Opt-in soft delete** — `Delete` marks, reads filter, `Restore`/`Purge`
- **Files bridge** — `ImportDir`/`ImportFile` migrate a tree of JSON files
  into a collection (ids = relative paths); `ExportDir` writes them back
- **No app-level cache needed** — SQLite's page cache serves reads in
  microseconds and is never stale for a second process
- **Optional debug logging** — one record per operation via `SetLogger`
- **CLI** (`godocstore`) to browse, edit and search any go-docstore database

## Installation

Library:

```sh
go get github.com/creativeyann17/go-docstore
```

CLI:

```sh
go install github.com/creativeyann17/go-docstore/cmd/godocstore@latest
# or grab a prebuilt binary from Releases
```

## CLI usage

```sh
godocstore --db app.db create users uploads        # new empty db + collections
godocstore --db app.db ls                          # collections + counts
godocstore --db app.db ls users                    # document ids
godocstore --db app.db get users u1 --meta         # pretty JSON + timestamps
godocstore --db app.db put users u1 doc.json       # upsert (stdin when no file)
godocstore --db app.db edit users u1               # $EDITOR round-trip, validated
godocstore --db app.db find users '$.login' = yann # search JSON content
godocstore --db app.db find users '$.quota' '>' 1000000 --ids
godocstore --db app.db sql "SELECT id, json_extract(doc,'$.email') FROM c_users"
godocstore --db app.db import users ./legacy-json/ # migrate files → documents
godocstore --db app.db export users ./backup/      # documents → files
godocstore --db app.db rm users u1 --soft          # mark; restore/purge undo it
```

## Library usage

```go
store, _ := docstore.Open("app.db")
defer store.Close()

users, _ := store.Collection("users",
    docstore.WithUniqueIndex("login", "$.login"),
    docstore.WithIndex("email", "$.email", true), // NOCASE
)

users.Put("u1", User{Login: "yann"})              // upsert
var u User
users.Get("u1", &u)                                // read (fresh copy, always)
users.GetBy("login", "yann", &u)                   // indexed lookup

// The concurrency primitive: transactional read-modify-write.
users.Update("u1", func(raw []byte) ([]byte, error) {
    var cur User
    json.Unmarshal(raw, &cur)
    cur.Quota += 50
    return json.Marshal(cur)
})

docs, _ := users.Find("$.quota", ">", 100, 10)     // search
meta, _ := users.Meta("u1")                        // created/updated/deleted
```

See [examples/](examples/) for a runnable walkthrough.

## Semantics worth knowing

- `Put` keeps the original `created_at` and **revives** a soft-deleted
  document; `Insert` fails with `ErrExists` on any conflict.
- **Soft delete + unique indexes don't mix well**: a soft-deleted document
  still holds its unique values, so re-creating "the same" document
  conflicts until purged. Prefer hard delete (the default) on collections
  with unique indexes.
- Databases created by mist-drive's embedded ancestor (schema without
  metadata columns) are **upgraded in place** on first `Collection()` call.
- The store uses WAL: a live database is `app.db` + `-wal` + `-shm`.
  Don't `cp` it while an app is writing — use `VACUUM INTO 'backup.db'`.

## Development

```sh
make test           # unit tests
make build          # bin/godocstore
make build-all      # cross-compiled dist/ + checksums
make install-hooks  # gofmt pre-commit hook
```

Releases: tag `vX.Y.Z` → CI runs tests, builds all platforms, publishes
with notes extracted from [CHANGELOG.md](CHANGELOG.md).

## License

[MIT](LICENSE)
