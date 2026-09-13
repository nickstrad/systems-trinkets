# Counter — HTTP contract

Status: **agreed 2026-09-12** (drafted from the suite-authoring interview; user added `DELETE`).

Every counter SUT — any language, any engine — implements this API. The
harness only ever talks HTTP to it. A counter is a named 64-bit signed
integer that springs into existence at 0 the first time it is touched (the
Valkey `INCR`-on-missing-key semantics; SQL implementations emulate it with an
upsert).

## Endpoints

| Method | Path | Request | Response | Notes |
|--------|------|---------|----------|-------|
| `GET` | `/healthz` | – | `200 {"ok":true}` | Ready to serve; backing store reachable. |
| `POST` | `/_reset` | – | `204` | Test hook: delete **every** counter. `FLUSHALL` / `TRUNCATE` / drop file. Must not be reachable in a production build (env gate or build tag). |
| `POST` | `/counters/{name}/incr` | optional `{"delta": n}` | `200 {"name":"…","value":v}` | Atomically adds `delta` (default 1, must be ≥ 1) and returns the **post-increment** value. |
| `GET` | `/counters/{name}` | – | `200 {"name":"…","value":v}` | Unknown counter reads as `0`. |
| `DELETE` | `/counters/{name}` | – | `204` | Removes that counter only; it reads `0` afterwards. Deleting an unknown counter is still `204`. |

`name` matches `[A-Za-z0-9_.-]{1,64}`.

## Errors

| Status | When |
|--------|------|
| `400` | malformed JSON body; `delta` missing-but-not-omitted, `< 1`, or not an integer; invalid `name` |
| `404` | any other path |
| `405` | wrong method on a known path |
| `500` | backing store failed; body is `{"error":"…"}` |

Error bodies are `{"error":"<message>"}`. The harness treats any non-2xx as
"not counted" — an increment that returned 5xx must **not** have been applied
(or, if it was, that is a lost-update in the other direction and INV-COUNTER-01
will catch it; the contract says: do not return 5xx after applying).

## Semantics the invariants rely on

- `incr` is atomic: two concurrent `incr` calls on the same name both count,
  and the two returned values are distinct.
- The value returned by `incr` is the value **immediately after** that
  increment was applied — not a later read.
- `GET` returns the value as of some point after the request arrived (it may
  lag concurrent increments, but never a value older than the last `incr`
  response *this client* already received).
- `/_reset` and `DELETE` complete before they respond: a `GET` issued after
  the `204` observes `0`. `DELETE` touches only the named counter.

## Example

```
$ curl -s -X POST localhost:8080/counters/page.home/incr
{"name":"page.home","value":1}
$ curl -s -X POST localhost:8080/counters/page.home/incr -d '{"delta":5}'
{"name":"page.home","value":6}
$ curl -s localhost:8080/counters/page.home
{"name":"page.home","value":6}
$ curl -s -X DELETE localhost:8080/counters/page.home -o /dev/null -w '%{http_code}\n'
204
$ curl -s localhost:8080/counters/page.home
{"name":"page.home","value":0}
$ curl -s -X POST localhost:8080/_reset -o /dev/null -w '%{http_code}\n'
204
$ curl -s localhost:8080/counters/page.home
{"name":"page.home","value":0}
```
