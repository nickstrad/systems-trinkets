# SQLite from Go (modernc.org/sqlite): gotchas

Applies when a lesson opens SQLite through `database/sql` with the pure-Go
`modernc.org/sqlite` driver (see `lessons/sqlite-wal-lab/main.go`).

- **Per-connection pragmas miss pooled connections.** `busy_timeout` (like most
  pragmas) belongs to one connection. `db.Exec("PRAGMA busy_timeout = 2000")`
  sets it only on whichever pooled connection ran it; a second `db.Conn` gets
  the default 0 and fails at once with `SQLITE_BUSY` instead of waiting. Put
  pragmas in the DSN so the driver runs them on every new connection:
  `file:events.db?_pragma=busy_timeout(2000)&_pragma=journal_mode(WAL)`.
- **WAL leaves sidecar files.** WAL mode creates `<db>-wal` and `<db>-shm`
  next to the database. Deleting only `<db>` between runs can leak state. Give
  each run its own `os.MkdirTemp` directory and `os.RemoveAll` it.
- **Time only the operation.** A fresh connection's first statement also loads
  the schema and compiles SQL. `PrepareContext` before starting the timer, then
  time `stmt.ExecContext`. (Reasoned, not measured separately.)
- **`ExecContext(nil, ...)` does not panic** here (Go 1.26, modernc v1.59), so
  a `nil` context fails silently rather than loudly; pass a real `ctx` anyway.

Verified 2026-09-24 with a scratch program: two `db.Conn` connections showed
`busy_timeout` 2000 and 0 after `db.Exec`, and 2000/2000 with `journal_mode=wal`
when set via the DSN. After the lesson moved pragmas to the DSN, DELETE-mode
writes against an open reader waited ~2026 ms then returned
`database is locked (5) (SQLITE_BUSY)`; WAL-mode writes took ~0.1 ms.
