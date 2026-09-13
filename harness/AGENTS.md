# Working in `harness/`

Read [docs/architecture.md](docs/architecture.md) before changing the harness.
It is the current design authority. Follow
[docs/suite-authoring.md](docs/suite-authoring.md) for the invariant interview,
contract agreement, suite structure, and testing rules. Conditional work is
in [docs/backlog.md](docs/backlog.md).

Update the architecture when design or behavior changes. Append work starts,
decisions, verification, and handoff state to
[docs/work-log.md](docs/work-log.md). Its archived plan preserves historical
assignments and reviews; current user instructions govern new work.

## Ownership and package placement

The user builds correct application logic outside this module. Keep deliberate
faults and bug-only interfaces under this harness (`internal/counterfault`,
`cmd/counter-fault`). Standard target files run the correct application.

- `suitekit`: suite lifecycle, per-test context, targets, health, restart.
- `httpclient`: every suite HTTP request goes here so it is sampled.
- `load`: barrier-started workers and phase labels.
- `check`: invariant evaluations and recorded metrics.
- `process`: child process groups; independent of suitekit.
- `results`: row definitions, collector, Parquet export/query source.
- `artifacts/runs`: generated, ignored run data; never source files.

Keep unit tests beside their source. Add a toolkit feature to the responsible
package and update the architecture. Use the repository
`update-trinkets-knowledge` skill for reusable discoveries or changed behavior
already covered by knowledge notes.

## Verification and references

Run from the repository root:

```sh
go -C cli test ./...
go -C examples/counter test ./...
go -C harness test -race ./...
go -C harness vet ./...
go -C harness build ./...
```

For runnable target, fault, report, and scaffold commands, use
[README.md](README.md). Suite environment variables are defined in
[suitekit/target.go](suitekit/target.go); `HARNESS_HEALTH_TIMEOUT` is defined
in [suitekit/suite_lifecycle.go](suitekit/suite_lifecycle.go).
