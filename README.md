# redis-term
redis-term is a simple database browser with both a terminal (TUI) and a web UI.

It supports:

- **Redis** — browse keys as a tree, preview string/hash/list/set/zset/stream values, inline cell editing, and raw commands.
- **MongoDB** — browse databases and collections, preview documents as a table with type-preserving inline edits, run queries, and drop collections/databases.
- **MySQL** — browse databases and tables, preview rows as a table with inline cell editing (primary-key columns are protected), run raw SQL, and drop tables/databases.

## Build & run

```
make run    # TUI:  go run ./cmd/redis-term
make web    # Web:  go run ./cmd/redis-term-web -addr :9898
make build  # build redis-term / redis-term-web binaries
```

## Configuration

Connections are read from `~/.redis-term.json` (override with `-config`). It is a
JSON array of connection objects. `kind` selects the backend: empty/`"redis"` for
redis, `"mongo"` for mongodb, `"mysql"` for mysql:

```json
[
  {"name": "local", "host": "127.0.0.1", "port": 6379, "auth": ""},
  {"name": "game-mongo", "kind": "mongo", "uri": "mongodb://user:pass@host:27017/admin"},
  {"name": "game-mysql", "kind": "mysql", "host": "127.0.0.1", "port": 3306, "user": "root", "auth": "pass"}
]
```
