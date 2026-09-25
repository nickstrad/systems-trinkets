# Valkey from Go (go-redis): gotchas

Applies when a lesson talks to the local Valkey with
`github.com/redis/go-redis/v9` (see `lessons/cache-aside/main.go`). The
documented connection string is `redis://localhost:6379` (no auth), from
`services/index.md`.

- **Connect from the documented URL, not a bare address.**
  `redis.Options{Addr: ...}` wants `host:port`; passing `redis://localhost:6379`
  to it fails. Use `redis.ParseURL(lab.ValkeyURL())` then
  `redis.NewClient(opts)`. `lab.ValkeyURL()` (in `internal/lab`) reads
  `CACHE_URL` and defaults to `redis://localhost:6379`, mirroring
  `lab.PostgresURL()` for Postgres.
- **A miss is `redis.Nil`, not an empty string.** `cache.Get(ctx, key).Result()`
  returns `err == redis.Nil` on a missing key; any other error is a real
  failure. `check(err)` cannot express that, so the read path spells it out.
- **Build the key in one helper.** Setup (`Del`) and the read path (`Get`/`Set`)
  must agree on the key format; a hand-copied literal in setup silently stops
  invalidating when the format changes.

Verified 2026-09-25: `lessons/cache-aside` ran end to end with `ParseURL` and
the default URL against `make up-valkey`.
