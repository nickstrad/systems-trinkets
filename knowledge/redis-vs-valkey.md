# Redis or Valkey for a lesson

Applies when a lesson needs a RESP server (cache, queue, lease, rate limit,
index) and you must pick between the two local ones. Catalog and policy:
"Choosing Redis or Valkey" in `software/software.md`.

## Policy

Pick per lesson by feature set and say why in the lesson or idea. When it does
not matter, default to Redis. Existing completed and planned Valkey lessons
(cache-aside, pipelining-work, lease-reclaim) stay on Valkey.

Rule of thumb: Redis for JSON, search/secondary indexes, vector sets, time
series, Bloom/Cuckoo/CMS/Top-K, or `DELEX`. Valkey for Valkey-specific behavior
(`DELIFEQ`, cluster-mode numbered databases) or the open-source-fork story.

## Setup

| | Redis | Valkey |
|---|---|---|
| Compose | `software/redis.compose.yaml`, `redis:8` | `software/valkey.compose.yaml`, `valkey/valkey:9` |
| URL | `redis://localhost:6380` (no auth) | `redis://localhost:6379` (no auth) |
| Port variable | `REDIS_PORT` | `VALKEY_PORT` |
| Go helper | `internal/lab/redis` (`REDIS_URL`) | `internal/lab/valkey` (`CACHE_URL`) |

Redis is on 6380 so both run at once and the Valkey lessons keep working
unchanged. The `redis` package imports go-redis as `goredis` to avoid the name
clash. The helpers read different env vars on purpose, so one lesson can
connect to both.

## Verified differences (2026-09-29)

Redis 8.10.2 (`redis:8`) against Valkey 9.1.2 (`valkey/valkey:9`), by running
commands with `docker exec ... redis-cli` / `valkey-cli`:

- `MODULE LIST`: Redis lists `bf`, `search`, `timeseries`, `ReJSON`,
  `vectorset`; Valkey lists only `lua`. `JSON.SET` and `VADD` work on Redis and
  are unknown commands on Valkey.
- `SET key val IFEQ old` and `HSETEX ... EX` work on both.
- Compare-and-delete differs: Valkey `DELIFEQ key val`, Redis
  `DELEX key IFEQ val`; each rejects the other's.
- `CONFIG GET notify-keyspace-events` is `xE` on both with `Ex` set.
- `COMMAND COUNT`: Redis 447, Valkey 257.
- `redis:8` has an arm64 manifest, so it runs on Apple silicon.

Not verified here: Valkey is BSD-3 (Linux Foundation); Redis 8 is tri-licensed
RSALv2/SSPLv1/AGPLv3; Valkey 9 has cluster-mode numbered databases and atomic
slot migration.

Also verified: `redis.Connect(ctx)` and `valkey.Connect(ctx)` both pinged
their servers from one scratch program, and `JSON.SET` succeeded through the
Redis client.
