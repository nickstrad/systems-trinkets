# Valkey from Go (go-redis): gotchas

Applies when a lesson talks to the local Valkey with
`github.com/redis/go-redis/v9` (see `lessons/go/cache-aside/main.go`). The
documented connection string is `redis://localhost:6379` (no auth), from
`software/software.md`.

- **Connect from the documented URL, not a bare address.**
  `redis.Options{Addr: ...}` wants `host:port`; passing `redis://localhost:6379`
  to it fails. `valkey.Connect(ctx)` (in `internal/lab/valkey`) does
  `redis.ParseURL(valkey.URL())`, `redis.NewClient(opts)`, and a `Ping`, since
  `NewClient` alone never contacts the server. `valkey.URL()` reads
  `CACHE_URL` and defaults to `redis://localhost:6379`, mirroring
  `postgres.URL()` for Postgres.
- **A miss is `redis.Nil`, not an empty string.** `cache.Get(ctx, key).Result()`
  returns `err == redis.Nil` on a missing key; any other error is a real
  failure. `check(err)` cannot express that, so the read path spells it out.
- **Build the key in one helper.** Setup (`Del`) and the read path (`Get`/`Set`)
  must agree on the key format; a hand-copied literal in setup silently stops
  invalidating when the format changes.

Verified 2026-09-25: `lessons/go/cache-aside` ran end to end with `ParseURL` and
the default URL against `make up-valkey`.

## Lease experiments

The code in [lease-reclaim.md](../docs/lessons/planned/lease-reclaim.md) uses `SetNX`
with zero expiry for the baseline and a positive TTL for the lease. Validate
lease TTLs before the call: zero must not silently create permanent ownership.
Renewal compares the owner token and applies `PEXPIRE` in one Lua operation.
Use fresh ownership tokens and check a stale token after another owner wins.

An acquisition reply describes one point in time. Finite leases can expire
between successful replies; success counts alone cannot establish that old
workers stopped using an external resource. The k6 plan therefore uses a
separate non-expiring contention probe for exactly-one-winner checks and
measures availability recovery independently. Fencing is outside this lesson.

Verified 2026-09-28 against the Valkey [SET](https://valkey.io/commands/set/)
and [PEXPIRE](https://valkey.io/commands/pexpire/) documentation and by running
the guide's extracted code against `redis://localhost:6379` (no auth).
Three permanent claims stayed blocked; three leases recovered; wrong-token
and stale-token renewal checks passed. Future k6 behavior remains a plan.
