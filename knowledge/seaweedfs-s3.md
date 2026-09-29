# SeaweedFS through the S3 API: gotchas

Applies to `http://localhost:8333` (access key `trinkets`, secret key
`trinkets-secret`, region `us-east-1`, path-style; see `software/software.md`).
The service is configured; a Go S3 client/helper is not yet configured.

Historical gateway probes (2026-09-27) found:

- `LastModified` has whole-second precision. Age-based garbage collection must
  allow for timestamp truncation as well as the maximum in-flight upload time.
- Missing-object HEAD requests return HTTP 404. Translate the chosen Go SDK's
  typed error explicitly; do not treat every error as absence.
- Batched object deletion is supported. List/batch operations can reduce round
  trips compared with one HEAD or DELETE per key; measure the Go client afresh.
- Repeating bucket creation can return `BucketAlreadyExists` or
  `BucketAlreadyOwnedByYou`. Handle expected existing-bucket outcomes explicitly.

The old implementation and its client-specific timings were removed. These
notes retain service observations, not a claim that a Go client was tested.
Verify timestamp, pagination, error translation and batch behavior when adding
that client. No runnable lesson currently uses S3.
