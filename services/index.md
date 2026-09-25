# Services index

| Service | Definition | Purpose | Connect |
|---|---|---|---|
| PostgreSQL 18 | `postgres.compose.yaml` | Relational store | `postgres://trinkets:trinkets@localhost:5432/trinkets` |
| Valkey 9 | `valkey.compose.yaml` | Redis-compatible cache, queues, pub/sub | `redis://localhost:6379` (no auth) |
| SeaweedFS | `seaweedfs/` | S3-compatible object storage | `http://localhost:8333`, access key `trinkets`, secret key `trinkets-secret`, region `us-east-1`, path-style |

## Port conflicts

Host ports default to the standard ones. If something else already holds a port,
override it with an environment variable when starting the service:

| Variable | Default |
|---|---|
| `POSTGRES_PORT` | 5432 |
| `VALKEY_PORT` | 6379 |
| `SEAWEEDFS_S3_PORT` | 8333 |
| `SEAWEEDFS_MASTER_PORT` | 9333 |
| `SEAWEEDFS_FILER_PORT` | 8888 |

    POSTGRES_PORT=15432 make up-postgres

## postgres.compose.yaml

Single `postgres:18` container. User, password, and database are all `trinkets`.
Data lives in the `trinkets-postgres_postgres-data` volume.

## valkey.compose.yaml

Single `valkey/valkey:9` container with append-only persistence, so keys survive
`down`/`up` until `clean`. No password. Data lives in `trinkets-valkey_valkey-data`.

## seaweedfs/

- `compose.yaml` — runs `weed server` (master, volume, filer, and S3 gateway in one
  process). Ports: 8333 S3 API, 9333 master UI, 8888 filer UI.
- `s3.json` — S3 identities. Access key `trinkets`, secret key `trinkets-secret`,
  full permissions. Use path-style addressing and any region (e.g. `us-east-1`).

Data lives in `trinkets-seaweedfs_seaweedfs-data`.
