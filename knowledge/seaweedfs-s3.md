# SeaweedFS through the S3 API: gotchas

Applies when a lesson talks to the local SeaweedFS S3 gateway at
`http://localhost:8333` (access key `trinkets`, secret key `trinkets-secret`,
region `us-east-1`, path-style; see `software/software.md`). From Deno use
`lessons/deno/lab/seaweedfs.ts`; no Go lesson uses S3 yet.

- **`LastModified` is whole seconds.** Two PUTs 250 ms apart both listed as
  `…:34.000Z`; the client clock read `…:34.311Z` at the first PUT, so the
  value is truncated, not rounded. Any age-based rule (a reconciler's grace
  window, a "delete objects older than" sweep) sees objects up to a second
  older than they are. A safe grace window is the longest in-flight time
  plus one second: in `cross-store-failure`, uploads in flight for 0.5 s were
  all deleted with a 1 s window and none with 2 s.
- **`HeadObject` on a missing key throws `NotFound` (HTTP 404).** Check
  `err.name === "NotFound"`; anything else is a real failure. `exists()` in
  the Deno helper wraps this.
- **`DeleteObjects` (batch of up to 1000 keys) works.** 2002 keys deleted in
  two requests in about 300 ms; one request per key would have been 2002
  round trips.
- **Throughput from one Deno process, concurrency 16:** about 0.2 ms per PUT
  (2000 small objects in 380 ms); `ListObjectsV2` returned 2002 keys in 20 ms
  (3 pages of 1000). Listing is roughly 10 µs per object, a HEAD is a full
  round trip per object, which is why a reconciler that scans everything in
  bulk can beat one that checks only a few hundred in-doubt keys one at a
  time (`lessons/deno/cross-store-failure/analyze.sql`, cost table).
- **Bucket create is not idempotent.** A second `CreateBucket` fails with
  `BucketAlreadyExists` or `BucketAlreadyOwnedByYou`; `ensureBucket()` treats
  both as success so lessons rerun.

Verified 2026-09-27 against `make up-seaweedfs` with `@aws-sdk/client-s3`
3.1141 under Deno 2.7.14 (a probe script for the timestamp, HEAD, and
throughput numbers; the lesson run for the grace and cost results).
