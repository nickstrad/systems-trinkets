# HTTP invariant harness — test plan

Status: **v1 (2026-09-12) — decisions Q1–Q6 agreed, Q7 deferred; ready to build Phase 1 per §10.** This file is the single source of truth
for the harness design. It is written so that a fresh session with no context
can pick up from here. Keep it current: when a decision changes, change it here
first.

Companion docs (to be created once this plan is agreed):

- `harness/AGENTS.md` — how an agent works in this folder: the interview used
  to elicit invariants for a new pattern, how to scaffold a suite, how to run
  and report. Section 8 below is its draft.
- `harness/suites/<pattern>/CONTRACT.md` and `INVARIANTS.md` — per pattern,
  agreed with the user before any test code is written.

---

## 1. Goal

A small, isolated Go toolkit under `harness/` that tests **invariants** of tiny
HTTP servers implementing the systems patterns in `docs/systems-patterns.md`
(counter, idempotency key, FIFO queue, lease, lock, rate limiter, …). The user
implements each pattern in several languages and against several engines
(Valkey, SQLite, PostgreSQL); the harness verifies every implementation the same
way and records results so they can be compared with DuckDB instead of read
off the terminal.

Non-goals: a general load-testing product, a CI system, a mocking framework.
It should be easy to add a suite in an afternoon and easy to extend the toolkit
when a pattern needs something new.

Guiding question from the guide, applied to testing: *what primitive is
actually providing the guarantee?* — every invariant test should name the
guarantee it is probing, so a failure says which primitive (or which
application convention) let it through.

## 2. Decisions (agreed 2026-09-12)

| # | Question | Decision | Status |
|---|----------|----------|--------|
| Q1 | Is a suite keyed by **pattern** or by **pattern × language**? | Per **pattern**. Language and engine are dimensions of a *target* (the server under test), not of the suite. One suite + one HTTP contract verifies every implementation, which is what makes cross-language/engine comparison in DuckDB meaningful. Per-target overrides (skip/expect) handle engine-specific semantics. | **agreed** |
| Q2 | Runner: `go test` or a custom binary? | `go test`. Each suite is a Go test package; a shared results sink is opened in `TestMain` and flushed at exit. Keeps `-run`, `-count`, `-race`, `-timeout`, `-v` for free and the user practices idiomatic Go testing. A thin `harness` CLI wraps `go test` for convenience but is not required. | **agreed** |
| Q3 | Results pipeline: DuckDB Go bindings (cgo) vs JSONL + `duckdb` CLI vs pure-Go Parquet writer | **`github.com/duckdb/duckdb-go/v2` for both write and read.** The Appender into an in-memory DuckDB *is* the in-memory sink; `COPY … TO parquet` *is* the flush; the report tool reads the same files with the same driver. One dependency, no CLI install, no second Parquet implementation. Verified on this machine: cold build 4.7 s, incremental 0.06 s, ~49–60 MB binary, 200k rows appended in 44 ms. Needs only clang (present) and `CGO_ENABLED=1` (present). Details §6. **Escape hatch:** if cgo turns out to be painful in practice, swap the writer for `parquet-go` (§6 option C) — row structs are kept flat so this stays a small change. | **agreed (try it)** |
| Q4 | Every server under test must expose `GET /healthz` and `POST /_reset`? | Yes. Two tiny endpoints keep the harness free of per-engine reset code and make every implementation language-agnostic to the harness. `/_reset` wipes the pattern's state (FLUSHALL / TRUNCATE / delete file) — it is a test hook, documented as such in each `CONTRACT.md`. | **agreed (try it)** |
| Q5 | Who starts the server under test and its backing store? | Phase 1: **the user starts the server** (and Valkey/Postgres if used) and the harness only needs its URL, from the target file or `--url`. Phase 2: an optional `cmd` in the target file lets the harness start/kill/restart the server itself, which crash tests require. Backing stores stay manual (a `docker compose` file per engine is provided under `harness/infra/`). | **agreed** |
| Q6 | Where does it live? | `harness/` at the repo root as its **own Go module** (`systems-trinkets/harness`), so the `trinkets` notes CLI does not inherit test/parquet dependencies. `harness/results/` is gitignored. Also add the built `systems-trinkets` binary to the root `.gitignore`. | **agreed** |
| Q7 | Tie runs back to the `trinkets` notes CLI? | **Deferred to the end of Phase 3.** The notes CLI is changing; revisit once both sides settle. | deferred |
| Q8 | Harness language | Go. | agreed |

**Terminology:** "SUT" = *server under test* — the little HTTP server you write for a pattern (in any language, on any engine). The harness never links to it; it only sends it HTTP requests.

## 3. Vocabulary

- **Pattern** — a system behavior from the guide (`counter`, `fifo-queue`, `lease` …). Slugs match `seed.go` in the notes CLI.
- **Suite** — the Go test package for one pattern: `harness/suites/<pattern>/`.
- **Contract** — the HTTP API a SUT must implement for that pattern (`CONTRACT.md`).
- **Invariant** — a named, numbered property (`INV-COUNTER-01`) with the primitive expected to guarantee it and how it is tested.
- **Target** — one concrete SUT: `(pattern, language, engine, url[, cmd])`, described in `harness/targets/<pattern>-<lang>-<engine>.toml`.
- **Run** — one execution of one suite against one target, identified by `run_id`. Produces one results directory.
- **Check** — one evaluation of one invariant inside one test. A test may check many invariants; an invariant may be checked by many tests.
- **Sample** — one HTTP request/response observed by the harness client (latency, status, error).

## 4. Kinds of tests

Every suite is expected to grow through these kinds, roughly in order.

1. **Contract / sequential** — one client, deterministic sequences. Does the API do what `CONTRACT.md` says? (Cheap, catches most bugs first.)
2. **Concurrency invariants** — N goroutines, released together on a barrier, hammering the SUT; then invariants are checked on the **final state** and on the **recorded history**, never on timing. Examples: final counter == number of `2xx` increments; every job id appears in exactly one worker's claimed set; at no instant do two holders believe they own the same lease (checked from request/response intervals); rate limiter admitted ≤ N in any window (checked from admitted timestamps).
3. **Crash / recovery** (phase 2) — harness kills the SUT (SIGKILL) mid-workload, restarts it, and re-checks invariants: no double-delivery after restart, leases expire, in-flight claims are recoverable, idempotency keys still bind to the same result.
4. **Performance** (recorded, not asserted by default) — throughput and latency percentiles under closed-loop and open-loop load. Stored as metrics and compared across runs in DuckDB. Optional per-target `expect` thresholds make them hard failures when wanted.
5. **History checks** (phase 3) — record `(invoke, response)` histories for KV/counter/queue ops and run a linearizability checker (Porcupine) when the pattern claims that level of guarantee.

Principle: a failing **invariant** test must mean the SUT is wrong, not that the harness was unlucky. Performance is reported; correctness is asserted.

## 5. Layout

```
harness/
  test-plan.md            this file
  AGENTS.md               working agreement for agents (draft in §8)
  go.mod                  module systems-trinkets/harness
  cmd/harness/            CLI: run | report | sql | new-suite | targets
  hx/                     HTTP client: JSON helpers, timeouts, path templates,
                          every request recorded as a sample
  load/                   concurrency: Closed (N workers), Open (fixed rate),
                          Barrier, Phases, Deadline helpers
  check/                  invariant assertions -> check rows + t.Errorf
  results/                Run/Test/Check/Sample/Metric row types, in-memory
                          Sink, flush to Parquet, run_id, results dir layout
  sut/                    (phase 2) start / health-wait / kill / restart / reset
  history/                (phase 3) op history recording for Porcupine
  suites/
    counter/
      CONTRACT.md
      INVARIANTS.md
      counter_test.go
    fifo-queue/ ...
  targets/
    counter-go-valkey.toml
  infra/
    valkey.compose.yml  postgres.compose.yml
  queries/                *.sql report templates run with DuckDB
  example-sut/            tiny Go reference server(s) for the harness's own
                          tests, with --bug flags that break invariants on
                          purpose so we can prove the tests have teeth
  results/                gitignored; results/runs/<run_id>/*.parquet
```

### Suite skeleton

```go
// suites/counter/counter_test.go
package counter

func TestMain(m *testing.M) { os.Exit(harness.Main(m)) }   // opens sink, reads target, flushes

func TestIncrementConcurrent(t *testing.T) {
    h := harness.New(t)                    // client bound to target, resets SUT, records test row on cleanup
    const workers, perWorker = 32, 200
    ok := load.Closed(h, workers, perWorker, func(w *load.Worker) error {
        return h.Post("/counter/incr", nil, nil)   // 2xx counted by hx automatically
    })
    got := h.GetJSON("/counter").Value
    check.Invariant(h, "INV-COUNTER-01", got == ok.Success2xx,
        "final value equals number of successful increments",
        map[string]any{"got": got, "want": ok.Success2xx})
}
```

### Target file

```toml
# targets/counter-go-valkey.toml
pattern  = "counter"
language = "go"
engine   = "valkey"
url      = "http://127.0.0.1:8080"
label    = "v1 INCR"            # free text, shows up in reports
# phase 2:
# cmd = ["go", "run", "./cmd/counter"]
# cwd = "../impls/go/counter"
# env = { VALKEY_URL = "redis://127.0.0.1:6379" }
[expect]                       # optional hard performance limits
# p99_ms = 20
```

## 5b. Toolkit: what to adopt vs hand-roll (researched 2026-09-12)

| Need | Day one | Later | Skip |
|------|---------|-------|------|
| Closed-loop concurrency (N workers) | hand-rolled `load.Closed` on `golang.org/x/sync/errgroup` + `semaphore`; a `Barrier` so all workers fire at once | | `rakyll/hey` (CLI only) |
| Open-loop load (fixed arrival rate, needed for rate limiters) | | `github.com/tsenart/vegeta/v12/lib` `Pacer` (`ConstantPacer`, `SinePacer`) — or a small ticker-based `load.Open` if that is all we need | |
| Latency | raw per-request samples → Parquet; quantiles in DuckDB (`quantile_cont`) | `HdrHistogram/hdrhistogram-go` only if sample volume becomes a problem | |
| Process kill/restart (crash tests) | `os/exec` + `SysProcAttr{Setpgid: true}` + `SIGKILL`/`SIGTERM` in `sut/` | `Shopify/toxiproxy` to fault the SUT↔store link (latency, resets) rather than the process | |
| Backing stores | `docker compose` files under `infra/`, brought up once and left running | `testcontainers-go` (`modules/valkey`, `modules/postgres`) if we ever want hermetic one-shot runs | |
| Test orchestration | `testing`: `TestMain`, `t.Cleanup`, `-race`, `-count`, `-shuffle`, `-json` | | `testing/synctest` (GA in Go 1.25) — virtual time, useless over real network I/O |
| Linearizability | | `anishathalye/porcupine` (`Operation`, `Model`, `CheckOperations`, HTML `Visualize`; used by etcd) for counter/KV/queue histories | |
| Generated op sequences w/ shrinking | | `pgregory.net/rapid` state-machine testing (generics, shrinking) for adversarial sequences | `leanovate/gopter` (older), `testing.F` (inputs, not sequences) |
| Jepsen-style framework | hand-roll (that is this harness) | | `maelstrom` (stdin/stdout toy protocol), Jepsen/Elle (Clojure) |

Takeaway: day one needs only the stdlib, `x/sync`, and a Parquet path (§6). Everything else is additive and slots into an existing package.

## 6. Results pipeline (Q3)

### Options considered (researched and run on this machine, 2026-09-12)

| Option | Facts | Verdict |
|--------|-------|---------|
| **A. `github.com/duckdb/duckdb-go/v2`** (official driver; `marcboeker/go-duckdb` moved to the DuckDB org at v2.5.0) | Latest `v2.10505.0` (2026-07-22) bundles DuckDB 1.5.5. cgo, but prebuilt static lib for darwin/arm64 ships as a Go module (`duckdb-go-bindings/lib/darwin-arm64`) — `go build` just works with Xcode CLT. Parquet + JSON extensions bundled, work offline. Appender: 200k rows in 43.5 ms into in-memory DB; `COPY (SELECT …) TO 'x.parquet' (FORMAT PARQUET, COMPRESSION ZSTD)` → 482 KB for 200k rows; `quantile_cont` over `read_parquet()` verified. | **Adopt** |
| B. Harness writes JSONL; `duckdb` CLI (`brew install duckdb`, 1.5.5) does `read_ndjson_auto` → `COPY … TO parquet` | Works (nested objects → STRUCT, ISO timestamps inferred). But: CLI not installed here, JSONL is fat/slow at 100k+ rows, harness and report would depend on an external binary and a second schema. | Keep only as an optional `--dump-jsonl` debug output |
| C. Pure-Go `github.com/parquet-go/parquet-go` (v0.32.0, 2026-08-12, pre-1.0) or `apache/arrow-go` pqarrow (v18.8.0) | parquet-go's `parquet.WriteFile[T]` from struct tags is a one-liner; arrow-go needs hand-built schemas. Neither can *query* — you still need DuckDB somewhere. | Fallback if we ever need `CGO_ENABLED=0` (cross-compile/distroless). Keep row structs flat so the swap is ~10 lines. |

Driver gotchas to bake into `results/`:

- Appender is **not goroutine-safe** and needs the native connection (`duckdb.NewConnector` → `Connect(ctx)`), not a pooled `database/sql` handle. So: workers send rows on a channel; one collector goroutine owns all appenders.
- `TIMESTAMP` round-trips as UTC micros. Store `time.Time` in UTC always (or epoch micros as `BIGINT`).
- JSON columns (`details`, `labels`) must be scanned into `any`/`duckdb.Composite`, not `string`. Write them via `encoding/json` marshal.
- Close everything (appender → conn → db) or it leaks. `Flush()` then `Close()` on appenders before `COPY`.
- Don't use `-tags duckdb_arrow` (heavy, not concurrency-safe); we don't need Arrow.

### Design

```
test goroutines ──rows──▶ chan ──▶ collector goroutine ──▶ Appender (in-memory DuckDB, 5 tables)
                                                                 │
                       TestMain exit / SIGINT / panic ───────────▶ COPY each table TO results/runs/<run_id>/<table>.parquet
                                                                 │
                                  harness report ──▶ read_parquet('results/runs/*/<table>.parquet') via the same driver
```

- `results.Sink` — `Open(runMeta) (*Sink, error)`, `Test(TestRow)`, `Check(CheckRow)`, `Sample(SampleRow)`, `Metric(MetricRow)`, `Close(ctx) error` (flush + COPY). `harness.Main(m)` opens it, installs a SIGINT handler and a deferred recover so a `^C`-ed or panicking run still exports what it has.
- Per-test `Flush()` of the appenders (cheap; commits to the in-memory table) so a mid-run `harness report --live` could `ATTACH` later if we ever want it. Not needed in phase 1.
- Durability option (later, if wanted): open the DuckDB on disk at `results/runs/<run_id>/run.duckdb` instead of in-memory; SIGKILL then loses at most the unflushed rows. Costs nothing but a file. Default stays in-memory per the original ask.
- Report tool is the same module: `harness report [--run last|<id>] [--query name]`, `harness sql "<sql>"`, and `harness ui` (only if the CLI is installed: `duckdb -ui` opens the local UI; optional).

Requirements:

- Tests emit rows through an in-memory sink; nothing in a test touches files.
- One directory per run, Parquet files per table; DuckDB queries them with a glob across runs.
- Survive a crashed or `^C`-ed run with partial results (flush per test, not only at exit).
- `harness report` prints tables from SQL templates; `harness sql` is a passthrough; the DuckDB UI can open the same files.

### Tables (draft)

| table | grain | key columns |
|-------|-------|-------------|
| `runs` | one per run | run_id, started_at, finished_at, pattern, language, engine, label, target_url, harness_git_sha, sut_ref, go_version, host |
| `tests` | one per Go test (incl. subtests) | run_id, test, status (pass/fail/skip), duration_ns, error |
| `checks` | one per invariant evaluation | run_id, test, invariant_id, ok, message, details (JSON) |
| `samples` | one per HTTP request | run_id, test, phase, worker, seq, method, path_template, status, latency_ns, err, started_at |
| `metrics` | free-form numbers | run_id, test, name, value, unit, labels (JSON) |

Canned queries (`queries/`): `summary.sql` (pass/fail per test, latest run), `latency.sql` (p50/p95/p99 per path per run via `quantile_cont`), `checks_failed.sql`, `compare.sql` (two run_ids side by side), `history.sql` (same pattern across all runs, by language × engine).

## 7. Phases

- **Phase 0 (done 2026-09-12)** — this plan; Q1–Q6 agreed, Q7 deferred.
- **Phase 1 — skeleton with teeth** (breakdown and owners in §10): module, `hx`, `load.Closed` + barrier, `check`, `results` + Parquet flush, `cmd/harness run|report|sql|new-suite`, the `counter` suite, and `example-sut/counter` (in-memory Go server with `--bug lost-update`). Done when: the suite passes against the correct server, fails with a named invariant against the buggy one, and `harness report` shows both runs from Parquet.
- **Phase 2 — second pattern + crashes**: `fifo-queue` suite (exactly-once claim, ordering, ack/retry), `load.Open`, `sut` lifecycle, first crash test. Done when a kill-mid-claim test passes against a correct SUT.
- **Phase 3 — depth**: `history` + Porcupine for counter/KV; `compare` reports; `[expect]` thresholds; `trinkets attempt` hand-off; more suites as the user reaches them.

## 8. Working agreement (draft `AGENTS.md`)

### Starting a new pattern

1. Read the pattern: `trinkets pattern show <slug>` and its row in `docs/systems-patterns.md` (the `invariants` column is the seed).
2. Run the **invariant interview** with the user (the guide's own questions, applied to this pattern):
   1. What invariant must the system preserve? (list them; each gets an ID)
   2. Which primitive provides each guarantee on this engine? (goes in `INVARIANTS.md`; a failure will point back here)
   3. What happens under concurrent access? (→ which concurrency tests)
   4. What happens if the process crashes between steps? (→ which crash tests, phase 2)
   5. How are retries, duplicates, ordering, expiration, recovery handled? (→ contract details: idempotency of endpoints, ack semantics, TTLs)
   6. Which guarantees come from the store vs from application convention? (→ which invariants are "should hold" vs "must hold")
   7. Performance expectations, if any (→ metrics to record, optional `[expect]`)
3. Propose `CONTRACT.md` (endpoints, request/response JSON, error codes, plus `/healthz` and `/_reset`) and `INVARIANTS.md` (table: ID, statement, guaranteeing primitive, test kind, status). Iterate until agreed.
4. `harness new-suite <pattern>` scaffolds the package and docs; write tests kind 1 → 2 → 4 → 3.
5. Run against `example-sut` if one exists, then the user's implementation: `harness run <pattern> --target targets/<file>.toml`.
6. Report with `harness report --run last`; record lessons with `trinkets attempt add`.

### Adding a tool to the toolkit

Add it to the package it belongs to (`hx`, `load`, `check`, `results`, `sut`, `history`); add a unit test against `example-sut`; note it in §5 of this plan.

### Rules

- Never assert timing in an invariant test; assert final state or recorded history.
- Every `check.Invariant` cites an ID from `INVARIANTS.md`.
- Every request goes through `hx` so it is sampled.
- Keep this plan and `AGENTS.md` current; they are the context after a reset.

## 9. Package APIs (Phase 1 contract between packages)

These signatures are the interface subagents build against. Change them here
first if they need to change.

```go
// results — row types + sink. Flat structs, UTC times, JSON as string fields
// marshalled by the caller. One collector goroutine owns the DuckDB appenders.
package results

type RunRow    struct{ RunID, Pattern, Language, Engine, Label, TargetURL, HarnessSHA, SUTRef, GoVersion, Host string; StartedAt, FinishedAt time.Time }
type TestRow   struct{ RunID, Test, Status, Error string; DurationNS int64 }          // Status: pass|fail|skip
type CheckRow  struct{ RunID, Test, InvariantID, Message, DetailsJSON string; OK bool; At time.Time }
type SampleRow struct{ RunID, Test, Phase, Method, PathTemplate, Err string; Worker, Seq int; Status int; LatencyNS int64; StartedAt time.Time }
type MetricRow struct{ RunID, Test, Name, Unit, LabelsJSON string; Value float64; At time.Time }

func Open(ctx context.Context, dir string, run RunRow) (*Sink, error) // in-memory DuckDB, creates dir
func (s *Sink) Test(TestRow); Check(CheckRow); Sample(SampleRow); Metric(MetricRow) // non-blocking, channel-backed
func (s *Sink) Close(ctx context.Context) error   // drain, finish run row, COPY every table to <dir>/<table>.parquet
func NewRunID() string                            // sortable: 20260912T151504Z-<4 random chars>

// hx — HTTP client bound to a target and a sink.
package hx
type Client struct{ BaseURL string; HTTP *http.Client; /* sink + run/test identity */ }
func (c *Client) Do(ctx, method, pathTemplate string, pathArgs map[string]string, body any, out any, opts ...Opt) (*Resp, error)
func (c *Client) Get/Post/Delete(...)              // thin wrappers on Do
type Resp struct{ Status int; Latency time.Duration; Body []byte }
// Every Do records a SampleRow (worker/phase/seq come from load via context values).
// Non-2xx is NOT an error by default (tests inspect Status); transport errors are.

// load — concurrency shapes. All start workers behind a barrier.
package load
type Worker struct{ ID int; Ctx context.Context; Seq func() int }
type Result struct{ Total, OK2xx, Non2xx, Errors int; Elapsed time.Duration; Errs []error }
func Closed(h *harness.H, workers, iterationsPerWorker int, fn func(*Worker) error) Result
func ClosedFor(h *harness.H, workers int, d time.Duration, fn func(*Worker) error) Result
func Phase(h *harness.H, name string, fn func())      // labels samples with a phase name
// phase 2: func Open(h, rate float64, d time.Duration, fn) Result

// check — invariant assertions. Always record a CheckRow; fail the test if !ok.
package check
func Invariant(h *harness.H, id string, ok bool, msg string, details map[string]any)
func Eventually(h *harness.H, id string, timeout time.Duration, cond func() (bool, map[string]any), msg string)
func Metric(h *harness.H, name string, value float64, unit string, labels map[string]any)

// harness — glue used from tests.
package harness
func Main(m *testing.M) int                        // parse target (HARNESS_TARGET file or HARNESS_URL), open sink, run, close sink; handles SIGINT + panic
func New(t *testing.T) *H                          // resets SUT via POST /_reset, waits /healthz, registers t.Cleanup that records TestRow
type H struct{ T *testing.T; Client *hx.Client; Target Target; /* run/test identity, sink */ }
type Target struct{ Pattern, Language, Engine, URL, Label string; Cmd []string; Cwd string; Env map[string]string; Expect map[string]float64 }
```

`cmd/harness`:

```
harness run <pattern> (--target targets/x.toml | --url http://…)  [-- go test flags]
harness report [--run last|<run_id>] [--query summary|latency|checks|compare] [--format box|md|json]
harness sql "<duckdb sql>"          # tables runs/tests/checks/samples/metrics are pre-registered as views over results/runs/*/
harness new-suite <pattern>         # scaffolds suites/<pattern>/{CONTRACT.md,INVARIANTS.md,<pattern>_test.go} from templates
harness targets                     # lists targets/*.toml
```

## 10. Phase 1 work breakdown and delegation

Order matters: **A → B in parallel with C/D/E → F → G**. I (the main
session) do A, B, F, G myself; they are the pieces where a wrong decision is
expensive (cgo sink, the `harness.H` glue every test touches, the invariant
list, and integration). Subagents get well-specified, self-contained packages.

| ID | Task | Owner | Inputs | Done when |
|----|------|-------|--------|-----------|
| A | Module skeleton: `harness/go.mod`, folder layout from §5, root `.gitignore` additions (`harness/results/`, `systems-trinkets` binary), `results` row types **and Sink** (appender collector, SIGINT/panic-safe export, `NewRunID`) | **me** | §6, §9 | `go test ./results/` writes 5 parquet files from an in-memory sink and reads p99 back via `read_parquet`; 100k samples in < 1 s |
| B | `harness` package: `Main`, `New`, `H`, `Target` (TOML loader, `HARNESS_TARGET`/`HARNESS_URL`), reset/health handshake, `TestRow` on cleanup | **me** | A | A dummy suite against `example-sut` records run + test rows |
| C | `hx` client: `Do`/`Get`/`Post`/`Delete`, path templates, JSON in/out, timeouts, context values for worker/phase/seq, sample recording through an injected recorder interface (so it does not import DuckDB) | **sonnet** subagent | §9 signatures, recorder interface from A | Unit tests with `httptest.Server`; every request yields exactly one sample; non-2xx not an error; transport error is |
| D | `load` package: `Closed`, `ClosedFor`, `Phase`, barrier start, `Result` aggregation, `-race` clean | **sonnet** subagent | §9 signatures | Unit tests prove all workers start after the barrier, counts add up, ctx cancel stops workers |
| E | `example-sut/counter`: Go `net/http` server implementing `suites/counter/CONTRACT.md`, in-memory, flags `--addr`, `--bug none|lost-update|drop-reset|slow`; `lost-update` does a racy read-modify-write with a `runtime.Gosched()` | **sonnet** subagent | CONTRACT.md from F | `go vet`, runs, `curl` smoke; `--bug lost-update` observably loses increments under 32 goroutines |
| F | `suites/counter/CONTRACT.md` + `INVARIANTS.md` via the §8 interview with the user; then `counter_test.go` (sequential contract test, concurrent increment invariant, reset semantics, perf metrics) | **me** (interview + tests) | B, C, D | Passes against `example-sut --bug none`; fails on `INV-COUNTER-01` against `--bug lost-update` |
| G | `cmd/harness`: `run`, `report` (canned SQL in `queries/`, views over `results/runs/*/`), `sql`, `new-suite` (templates), `targets`; `check` package | **opus** subagent for CLI + queries; `check` is small enough that I do it in B | A, B | `harness run counter --url …` then `harness report --run last` prints summary + latency tables from parquet; `new-suite foo` compiles |
| H | `infra/valkey.compose.yml`, `infra/postgres.compose.yml`, `harness/README.md` (how to run; points here) | **sonnet** subagent | §5 | `docker compose -f … up -d` works; README accurate |

Subagent briefs must include: the exact §9 signatures, the recorder interface
from A (so `hx`/`load` never import DuckDB), "match the surrounding code
style, stdlib first, no new deps without listing them", and the done-when
line. I review every subagent result before integrating (F/G depend on it).

Phase 2 and 3 get their own breakdown here when Phase 1 is done.

## 11. Open questions / risks

- SQLite targets: a single-writer engine will serialize writes; concurrency invariants still hold but throughput will differ. Fine — that's the point of the comparison — but tests must not depend on throughput.
- Lease/lock "no two holders at once" can only be checked from client-observed intervals (request sent → response received); this is a sound but conservative check. Say so in `INVARIANTS.md`.
- `/_reset` in a production-shaped server is a smell; it is gated behind an env var in the SUT or only compiled in test builds. The contract will say so.
- Results volume: `samples` at ~100 bytes/row × 100k rows/run is ~10 MB in memory, ~1–2 MB as Parquet. Sampling (every Nth request) is a knob if it ever matters.
