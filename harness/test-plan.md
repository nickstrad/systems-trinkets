# HTTP invariant harness — test plan

Status: **v4 (2026-09-12) — R0–R5 done: the counter is the finished worked
example (contract → suite incl. crash test → Go reference on memory, SQLite,
Valkey, PostgreSQL → a target per engine the harness starts itself → runs
and reports). What remains is the flat backlog in §7, each item tied to the
pattern that first needs it. Current position: see the last entry of §0.** This file is the single source of truth for the harness design **and
the living log of the work**: it is the context that survives a context
clear. It is written so that a fresh session with no context can pick up
from here. Keep it current: when a decision changes, change it here first;
when work starts, is delegated, is reviewed, or lands, append to §0.

Companion docs:

- `harness/AGENTS.md` — how an agent works in this folder: the interview used
  to elicit invariants for a new pattern, how to scaffold a suite, how to run
  and report. Derived from §8 (written 2026-09-12).
- `harness/README.md` — how to run; points here.
- `harness/suites/<pattern>/CONTRACT.md` and `INVARIANTS.md` — per pattern,
  agreed with the user before any test code is written. `counter`: agreed
  2026-09-12 (user added `DELETE /counters/{name}`; kept INV-COUNTER-01..05,
  dropped the two "should" invariants for now).

---

## 0. Work log

Append-only, newest last, one or two lines per entry. The rules are in
§10c ("Working protocol"). A fresh session reads the last few entries, then
the §10a table, and knows exactly where things stand.

| When | Entry |
|------|-------|
| 2026-09-12 | Plan v1 written; Q1–Q6 agreed, Q7 deferred, Q8 Go. |
| 2026-09-12 | Toolkit core built (§10b A–H) and verified against `example-sut/counter --bug none\|lost-update\|drop-reset` (details end of §10). Left **uncommitted**. |
| 2026-09-12 | Plan v2: §9 as-built APIs, §10 breakdown ticked. |
| 2026-09-12 | Plan v3: phases dropped for a flat backlog (§7). Q9 agreed: counter reference fully done on memory/SQLite/Valkey/PostgreSQL as the worked example. Q10 suggested (`impls/<lang>/<pattern>/`). Q5 revised: `cmd` targets in scope. §10a R0–R5 written. |
| 2026-09-12 | §0 log and §10c protocol added: owners per R-item (main: R1, R4; builder-opus: R2; builder-sonnet: R3, R5), reviewer (Opus high, read-only) before every commit, one commit per item, §0 entry at every step. Role files created under `.claude/agents/` (uncommitted, land with R0). Codex mapping recorded in §10c: main → `astra` (controlling agent), Sonnet roles → `luna` high, Opus roles → `sol`. **Next: R0** — waiting on the user's go. |
| 2026-09-12 | Global skill `plan-agent-flow-state` created (~/.agents/skills, symlinked for Claude) generalising §0 + §10a/§10c; rule added: every owner cell names both Claude Code and Codex models so items resume on either platform after a quota limit. §10a cells updated. **Next: R0** — waiting on the user's go. |
| 2026-09-12 | User set this file as the session goal (`/goal`) — taken as the go. R0 started → main. `go mod tidy` (toml, duckdb-go now direct), `CONTRACT.md` example `page:home` → `page.home`, `go build/vet/test ./...` green. Committed `ba222eb`. **Next: R1.** |
| 2026-09-12 | R1 started → main. Files: `example-sut/counter/{counter,memory,bugs,storetest}.go` + tests, `example-sut/cmd/counter/main.go`, `harness/main.go` (Option/InProcess), `suites/counter/counter_test.go` (TestMain), `cmd/harness/templates/suite_test.go.tmpl`. `StoreTest` written here (memory + decorators use it) so R2 only adds engine callers. |
| 2026-09-12 | R1 built by main; done-when verified (in-process suite runs, `-race` green; `--bug lost-update` fails INV-01/02/05, `drop-reset` INV-04, `write-behind` passes 01–05). §9 updated (InProcess built, StoreTest, healthz 503, store error 500). R1 → reviewer dispatched. Gotcha: a stale `counter` on :18080 from the earlier session made every bug mode look identical — check `lsof -iTCP:<port>` before trusting a `--url` run. |
| 2026-09-12 | R1 review: needs fixes — must-fix: root `.gitignore` line `/harness/harness` (from R0) ignored the *package* dir `harness/harness/`, so `ba222eb` is unbuildable from a clean checkout; now `/harness/bin/`, verified with a `git archive` export build. Should: stale `./example-sut/counter` paths in README/AGENTS/target file; unit test for InProcess results paths (`harness/main_test.go`). Nits: sync.Once on WriteBehind.Close, store closed on bad `--bug`, Getwd error. Sent back for re-check. Infra: Docker Desktop started, `infra/valkey` + `infra/postgres` compose up (for R2). |
| 2026-09-12 | R1 reviewed clean, nits only (1 must-fix + 3 should + 3 nits fixed; declined: StoreTest in package counter, `--dsn` ignored for memory, INV-06 forward ref) → committed (sha in `git log`, subject "R1: …"). R5 note from reviewer: `knowledge/harness.md:32` still describes `example-sut/counter` as a server. **Next: R2 ∥ R3.** |
| 2026-09-12 | R1 committed `cfd0f2a`. R2 started → builder-opus; R3 started → builder-sonnet (parallel, disjoint files). Decision for R3: its unit tests spawn `cmd/counter --engine memory` (sqlite does not exist until R2 lands in parallel); "state preserved across restart" is an engine property covered by R2's StoreTest + R4, so R3 tests lifecycle only (start→healthy, kill→refused, restart→healthy with a new pid, Stop→clean exit). §10a R3 row amended. `counter-example-memory.toml` is replaced by `counter-go-memory.toml` in R2. |
| 2026-09-12 | R2 built by builder-opus: suite passes on all four engines by hand-started SUTs, `lost-update` fails INV-01 on all four, history report shows the four runs. Deviations: Postgres upsert is `counters.value + excluded.value` (unqualified is ambiguous in PG); SQLite uses `SetMaxOpenConns(1)` + WAL/busy_timeout via `_pragma=` DSN params; `go mod tidy` needed (pgx pulls `puddle/v2` into go.sum). R2 → reviewer dispatched. R4 doc side done by main: INV-COUNTER-06 row added to `INVARIANTS.md` (status "proposed", wording from the R4 row). |
| 2026-09-12 | R3 built by builder-sonnet: `sut/` (Start/Kill/Stop/Exited + `PID`/`ExitState`/`Err`), `LoadTarget` resolves `cwd` against the target file, Main starts/stops the SUT on every exit path (before the sink closes), `H.Restartable/Restart`, `run.go` untouched (Main's stderr already reaches the terminal). Done-when passed against R2's real sqlite target. Main fixed its own R1 test (`TestInProcessWritesResultsOnlyWhenAsked` counted `results/runs/` entries → flaky with concurrent runs; now checks a fixed run id). R3 → reviewer dispatched. |
| 2026-09-12 | R2 review: needs fixes. Must-fix: sqlite target's `cmd` has no `--dsn`, so every start (incl. `H.Restart`) mints a fresh temp DB → R4 would see 0 after restart. Decision (§9 updated): sqlite `--dsn` default is a **stable** path derived from `--addr` under `os.TempDir()`, not a fresh temp file. Should: README quick start must not hand-start a SUT for a `cmd` target; sqlite doc comment (SetMaxOpenConns(1) is the serialiser). Nits: pg ping timeout + DSN redaction, env-overridable test DSNs. Sent back to builder-opus. Reviewer note for R4: memory target has `cmd` too, so skip on `Engine == "memory"`, not on `Restartable()` alone. |
| 2026-09-12 | R4 test written by main (`suites/counter/crash_test.go`, `TestCrashRestart`): load in a goroutine (Restart may t.Fatal → test goroutine), workers keep hammering through the kill and pause only on a post-kill transport error, kill triggered at ⅓ of the acks, bound `all 2xx ≤ final ≤ all 2xx + errored`; skips on `!Restartable()` and on `memory`. Verified: passes on valkey and postgres (16 in flight at the kill each time); fails on valkey `--bug write-behind` (lost 1617 = acked before restart). sqlite pending R2's DSN fix. R4 → reviewer after R2/R3 land. |
| 2026-09-12 | R2 fixes applied by builder-opus (stable sqlite DSN per `--addr`, README quick start via `--target`, pg ping timeout + DSN redaction, env-overridable test DSNs); builder confirms `TestCrashRestart` now passes on sqlite twice in a row with the same DSN. R2 → reviewer re-check. Lesson (R2 builder): every default a SUT computes at startup must be a pure function of its flags, or a restart-based test silently breaks. |
| 2026-09-12 | R2 re-reviewed: nits only (WAL/-shm files of a killed sqlite SUT accumulate next to the stable DB — recovered on next open, left as is; per-user temp dir on Linux — noted). `TestCrashRestart` passes on sqlite/valkey/postgres via `--target`, skips on memory. R2 → committed (sha in `git log`, subject "R2: …"). |
| 2026-09-12 | R2 committed `b8ca6f8`. R3 review: needs fixes. Must-fix: (1) anything answering `Target.URL` satisfied the health wait — a stale server made the suite test a stranger and `TestCrashRestart` pass vacuously → Main now refuses to start if the URL already answers and fails if the SUT exited during the wait; (2) SIGINT handler installed only after `results.Open`, so ^C during the health wait orphaned the process group → handler moves before `sut.Start`; (3) "stopped on panic" is not achievable (a test panic kills the process from the test goroutine; Main's defers never run) → **decision: drop the claim**, in code comments and here (§6/§9 fixed: partial results survive SIGINT, not a test panic or `-timeout`; the pre-check in (1) reports the orphan on the next run). Should: Restart/SIGINT race, Stop error logged, Proc/Restart concurrency docs, replace the cannot-fail `Restartable` test with a child run of `TestCrashRestart` on sqlite. Sent back to builder-sonnet. |
| 2026-09-12 | builder-sonnet, resumed with the R3 fix list, replied as if relaying to another agent and changed nothing; on the user's instruction it was stopped and **R3's fixes reassigned to builder-opus** (`opus` medium / `sol` medium) with a self-contained brief (built state + findings + decisions). §10a R3 owner cell updated. |
| 2026-09-12 19:14 | **Handoff state (user near quota limit).** Committed: R0 `ba222eb`, R1 `cfd0f2a`, R2 `b8ca6f8`. In the tree, uncommitted: (a) **R3** — `sut/`, `harness/{main,h,target}.go`, `harness/sut_integration_test.go`, `cmd/harness/run.go`, being edited by builder-opus to apply the R3 review findings (the list is the §0 entry two rows up: must-fix 1–3, should 4–8, nits 9–10); if the agent did not survive the clear, its partial edits are in the tree — **re-dispatch reviewer on the diff first** (§10c step 5), then a builder-opus for whatever findings remain; (b) **R4** — `suites/counter/crash_test.go` + `INVARIANTS.md` INV-06 row, complete and verified on sqlite/valkey/postgres (passes) and valkey `--bug write-behind` (fails), not yet reviewed — review after R3 commits, then commit; (c) `harness/main_test.go` fixed-run-id change (main's, lands with R3); (d) this file. Order to finish: R3 review → commit R3 (files in (a)+(c)) → R4 review → commit R4 (files in (b)) → R5 (doc sweep; add the two reviewer notes: `knowledge/harness.md:32` describes `example-sut/counter` as a server, `:52` names the deleted `counter-example-memory.toml`). Infra: Docker Desktop with `infra/{valkey,postgres}` compose up; `lsof -nP -iTCP:8080-8083` should be empty before any run. |
| 2026-09-12 | User (near quota) asked main to stop the agents and finish directly. builder-opus had applied all ten R3 findings; main read the full diff, tidied one comment, and re-ran the verification itself in place of a reviewer re-check: `-race` green, `-count=3 ./sut/ ./harness/` no flakes, sqlite target run starts/restarts/stops the SUT (6/6 incl. `TestCrashRestart`), stale-server refusal and SIGINT-during-health-wait reproductions pass, no orphans. R3 → committed (subject "R3: …"). **Protocol deviation recorded: no independent reviewer re-check on R3's fixes.** |
| 2026-09-12 | R3 committed `a8837ae`. R4 done-when verified by main: `TestCrashRestart` passes on sqlite/valkey/postgres `--bug none` (via `--target`), **fails INV-06 on `--bug write-behind` on all three** (≈1615 acked-before-restart increments lost each time), skips on memory and on `--url`. INV-06 wording is the R4 row's, status "proposed" until the user reads it. R4 → committed (subject "R4: …"), **same deviation: reviewed by main only.** **Next: R5** (doc sweep — see the handoff entry above for the two knowledge/harness.md notes). |
| 2026-09-12 | R4 committed `8827754`. R5 done by main directly: phase wording removed from `AGENTS.md`, `README.md`, `invariants.md.tmpl`, `INVARIANTS.md`; `knowledge/harness.md` rewritten to describe only what exists (incl. the two reviewer notes), `knowledge/architecture.md` harness paragraph and `knowledge/index.md` entry updated; `grep -ri "phase [0-9]" harness/ knowledge/` clean outside this file's history; `go test ./...` green. Plan → v4. R5 → committed. **R0–R5 complete. Next: whichever pattern the user reaches (§8 interview), or a §7 backlog item when a pattern needs it.** |

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
| Q5 | Who starts the server under test and its backing store? | Either. With only `url` in the target file (or `--url`) **the user starts the server** and the harness just talks to it. With `cmd` (+ `cwd`, `env`) the harness starts, kills, restarts and stops it itself (`sut` package, §9) — crash tests need this and skip without it. Backing stores stay manual (a `docker compose` file per engine under `harness/infra/`). | **agreed (cmd part: R3 in §10)** |
| Q6 | Where does it live? | `harness/` at the repo root as its **own Go module** (`systems-trinkets/harness`), so the `trinkets` notes CLI does not inherit test/parquet dependencies. `harness/results/` is gitignored. Also add the built `systems-trinkets` binary to the root `.gitignore`. | **agreed** |
| Q7 | Tie runs back to the `trinkets` notes CLI? | **Deferred.** The notes CLI is changing; revisit once both sides settle. | deferred |
| Q8 | Harness language | Go. | agreed |
| Q9 | How complete is the reference counter? | **Fully done: the Go counter on memory, SQLite, Valkey and PostgreSQL, in `example-sut/counter/`.** Counter is simple enough that finishing it on every engine costs little, and it is the *worked example* of the whole loop — contract → suite → implementation per engine → target files → runs → reports — that later patterns and other languages start from. It is not a yardstick; it is the starting point that shows the harness used in a meaningful way. It doubles as the harness's own test fixture (bug modes per invariant, in-process mode for `go test ./...`). (2026-09-12) | **agreed** |
| Q10 | Where do the user's other implementations live? | Anywhere; a target file only needs `url` (and `cmd`/`cwd` if the harness should start it). Suggested: `impls/<language>/<pattern>/` at the repo root, each its own module, so neither Go module inherits their dependencies. The Go counter reference stays inside the harness module because the suite imports it for in-process runs. | suggested |

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
3. **Crash / recovery** — harness kills the SUT (SIGKILL) mid-workload, restarts it, and re-checks invariants: no double-delivery after restart, leases expire, in-flight claims are recoverable, idempotency keys still bind to the same result. Needs a target with `cmd` (Q5); skips otherwise. Note what a process kill can and cannot probe: the backing store (a separate process, or the OS page cache for SQLite) survives it, so a kill tests the **SUT's** discipline — respond only after commit, one atomic step per request — not the store's durability. Requests in flight at the kill are ambiguous (applied or not), so crash invariants are stated as bounds: `2xx-before-kill ≤ final ≤ 2xx-before-kill + errored`.
4. **Performance** (recorded, not asserted by default) — throughput and latency percentiles under closed-loop and open-loop load. Stored as metrics and compared across runs in DuckDB. Optional per-target `expect` thresholds make them hard failures when wanted.
5. **History checks** (backlog, only if a pattern needs it) — record `(invoke, response)` histories for KV/counter/queue ops and run a linearizability checker (Porcupine) when the pattern claims that level of guarantee.

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
  sut/                    (R3) start / kill / stop a SUT process from a
                          target's cmd; harness.Main and H.Restart use it
  suites/
    counter/
      CONTRACT.md
      INVARIANTS.md
      counter_test.go
    fifo-queue/ ...
  targets/
    counter-go-memory.toml  counter-go-sqlite.toml
    counter-go-valkey.toml  counter-go-postgres.toml   (R2; all with cmd)
  infra/
    valkey.compose.yml  postgres.compose.yml
  queries/                *.sql report templates run with DuckDB
  example-sut/            the reference implementations (Q9): the worked
    counter/              example of a pattern done end to end. Package
      counter.go            counter: Store interface, NewHandler, contract
      memory.go             handler, memory store, bug decorators
      bugs.go
      store/sqlite/         one package per engine (only cmd imports them,
      store/valkey/         so in-process suite runs link only memory)
      store/postgres/
    cmd/counter/            binary: --addr --engine --dsn --bug
  results/                gitignored; results/runs/<run_id>/*.parquet
```

`history/` (Porcupine) is not in the layout; it is added only when a pattern
needs it (§7 backlog).

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
# optional (R3): the harness starts/kills/restarts the SUT itself; crash
# tests need this and skip without it. cwd is relative to this file.
cmd = ["go", "run", "./example-sut/cmd/counter", "--engine", "valkey", "--dsn", "redis://127.0.0.1:6379/1"]
cwd = ".."
env = { }                       # extra KEY = "VALUE" for the process; PORT is not injected, put --addr in cmd
[expect]                       # optional hard performance limits (backlog)
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
| Target files (TOML) | `github.com/BurntSushi/toml` (stdlib has no TOML) | | |

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

- `results.Sink` — `Open(runMeta) (*Sink, error)`, `Test(TestRow)`, `Check(CheckRow)`, `Sample(SampleRow)`, `Metric(MetricRow)`, `Close(ctx) error` (flush + COPY). `harness.Main(m)` opens it and installs a SIGINT/SIGTERM handler so a `^C`-ed run still exports what it has. A **test panic or `go test -timeout` expiry does not**: the process dies from the test goroutine before Main's defers run, and the in-memory tables die with it (the durability option below is the fix if this ever matters).
- Per-test `Flush()` of the appenders (cheap; commits to the in-memory table) so a mid-run `harness report --live` could `ATTACH` later if we ever want it. Not needed yet.
- Durability option (later, if wanted): open the DuckDB on disk at `results/runs/<run_id>/run.duckdb` instead of in-memory; SIGKILL then loses at most the unflushed rows. Costs nothing but a file. Default stays in-memory per the original ask.
- Report tool is the same module: `harness report [--run last|<id>] [--query name]`, `harness sql "<sql>"`, and `harness ui` (only if the CLI is installed: `duckdb -ui` opens the local UI; optional).

Requirements:

- Tests emit rows through an in-memory sink; nothing in a test touches files.
- One directory per run, Parquet files per table; DuckDB queries them with a glob across runs.
- Survive a `^C`-ed run with partial results (flush per test into the in-memory tables; export on SIGINT). A crashed (panicking, timed-out) run loses its results until the on-disk DuckDB option above is taken — as built 2026-09-12.
- `harness report` prints tables from SQL templates; `harness sql` is a passthrough; the DuckDB UI can open the same files.

### Tables (draft)

| table | grain | key columns |
|-------|-------|-------------|
| `runs` | one per run | run_id, started_at, finished_at, pattern, language, engine, label, target_url, harness_git_sha, sut_ref, go_version, host |
| `tests` | one per Go test (incl. subtests) | run_id, test, status (pass/fail/skip), duration_ns, error |
| `checks` | one per invariant evaluation | run_id, test, invariant_id, ok, message, details (JSON as VARCHAR), recorded_at |
| `samples` | one per HTTP request | run_id, test, phase, worker, seq, method, path_template, status, latency_ns, err, started_at |
| `metrics` | free-form numbers | run_id, test, name, value, unit, labels (JSON as VARCHAR), recorded_at |

As built (2026-09-12): `at` is a DuckDB keyword, so the timestamp column on
`checks`/`metrics` is `recorded_at`. JSON columns are stored as `VARCHAR`
(query with `json_extract`) so the Appender needs no JSON type handling and
the parquet stays portable. Setup requests made by `harness.New` (`/_reset`,
`/healthz`) are sampled with `phase = results.PhaseSetup`; `results.Query`
adds a `samples_measured` view without them, which is what `latency.sql`,
`compare.sql` and `history.sql` read. Report templates use DuckDB named
parameters (`$run_id`, `$run_a`, `$run_b`, `$pattern`) bound with
`sql.Named` — no string substitution. Latency percentiles are computed only
in SQL; suites record throughput (`check.Metric`) but not their own
quantiles, so there is one definition of p99.

Canned queries (`queries/`): `summary.sql` (pass/fail per test, latest run), `latency.sql` (p50/p95/p99 per path per run via `quantile_cont`), `checks_failed.sql`, `compare.sql` (two run_ids side by side), `history.sql` (same pattern across all runs, by language × engine).

## 7. Status and backlog

No phases. One table: what is built, what is next, what waits for the
pattern that needs it, what was dropped. The next block of work has its
breakdown in §10.

| Status | Item | Notes |
|--------|------|-------|
| built 2026-09-12 | module, `results` sink → Parquet, `hx`, `load.Closed/ClosedFor/Phase`, `check`, `harness` glue, `cmd/harness run\|report\|sql\|new-suite\|targets`, `queries/*.sql`, `infra/` compose files, `counter` CONTRACT/INVARIANTS/suite (INV-01..05), in-memory `example-sut/counter` with `--bug lost-update\|drop-reset\|slow` | Verified: suite passes on the correct server, fails INV-01/02/05 on `lost-update`, INV-04 on `drop-reset`; reports read from Parquet. **Not yet committed** (R0). |
| built 2026-09-12 (§10a R0–R5) | R0 commit · R1 reference counter as a library + in-process suite runs · R2 SQLite/Valkey/PostgreSQL stores + four target files with `cmd` · R3 `sut` lifecycle, `H.Restart` · R4 INV-COUNTER-06 crash test · R5 doc sweep | Commits `ba222eb`, `cfd0f2a`, `b8ca6f8`, `a8837ae`, `8827754`, R5. The counter is the finished worked example; a new pattern is now "interview → suite → implementations". Deviation: R3's fixes and R4 were reviewed by main only (user near quota), see §0. |
| when a pattern needs it | `load.Open` (fixed arrival rate) | first needed by `rate-limiter` (#15). Ticker-based, ~60 LOC, or vegeta's `Pacer`. |
| when wanted | on-disk DuckDB (`results/runs/<run_id>/run.duckdb`) instead of in-memory | the only way a panicking or `-timeout`-killed run keeps its partial results (§6); today only SIGINT exports. Found by the R3 review 2026-09-12. |
| when wanted | `[expect]` thresholds | post-run SQL in `cmd/harness run` over `samples_measured`, one `checks` row per threshold (`INV-PERF-*`). `--bug slow` then has something to fail. |
| when there are ≥ 3 targets for one pattern | `harness run --all-targets <pattern>` | runs every `targets/<pattern>-*.toml` in sequence, one run each; `history.sql` already reports across them. |
| when a pattern claims it | `history/` + Porcupine linearizability check | counter does not need it (INV-02 covers the ordering claim). |
| deferred (Q7) | `trinkets attempt` hand-off | e.g. `harness run --attempt <id>` appending the run_id to the attempt's lessons. |
| dropped | toxiproxy, testcontainers, rapid, `fifo-queue` as a toolkit milestone | fifo-queue is just the next pattern; it gets a suite the normal way (§8) when the user reaches it. |

## 8. Working agreement (draft `AGENTS.md`)

### Starting a new pattern

1. Read the pattern: `trinkets pattern show <slug>` and its row in `docs/systems-patterns.md` (the `invariants` column is the seed).
2. Run the **invariant interview** with the user (the guide's own questions, applied to this pattern):
   1. What invariant must the system preserve? (list them; each gets an ID)
   2. Which primitive provides each guarantee on this engine? (goes in `INVARIANTS.md`; a failure will point back here)
   3. What happens under concurrent access? (→ which concurrency tests)
   4. What happens if the process crashes between steps? (→ which crash tests; see kind 3 in §4 for what a kill can probe)
   5. How are retries, duplicates, ordering, expiration, recovery handled? (→ contract details: idempotency of endpoints, ack semantics, TTLs)
   6. Which guarantees come from the store vs from application convention? (→ which invariants are "should hold" vs "must hold")
   7. Performance expectations, if any (→ metrics to record, optional `[expect]`)
3. Propose `CONTRACT.md` (endpoints, request/response JSON, error codes, plus `/healthz` and `/_reset`) and `INVARIANTS.md` (table: ID, statement, guaranteeing primitive, test kind, status). Iterate until agreed.
4. `harness new-suite <pattern>` scaffolds the package and docs; write tests kind 1 → 2 → 4 → 3.
5. Implement the reference (or the user's implementation) and run: `harness run <pattern> --target targets/<file>.toml`. Give the reference `--bug` modes that break each invariant and prove the suite catches them.
6. Report with `harness report --run last`; record lessons with `trinkets attempt add`.

### Adding a tool to the toolkit

Add it to the package it belongs to (`hx`, `load`, `check`, `results`, `sut`, `history`); add a unit test against `example-sut`; note it in §5 of this plan.

### Rules

- Never assert timing in an invariant test; assert final state or recorded history.
- Every `check.Invariant` cites an ID from `INVARIANTS.md`.
- Every request goes through `hx` so it is sampled.
- Keep this plan and `AGENTS.md` current; they are the context after a reset.

## 9. Package APIs (as built 2026-09-12; R-items marked "planned")

These signatures are the interface between packages. Change them here first
if they need to change. Deviations from the original draft are marked ★.

```go
// results — row types + sink. Flat structs, UTC times, JSON as string fields
// marshalled by the caller (results.JSON). One collector goroutine owns the
// DuckDB appenders.
package results

type RunRow    struct{ RunID, Pattern, Language, Engine, Label, TargetURL, HarnessSHA, SUTRef, GoVersion, Host string; StartedAt, FinishedAt time.Time }
type TestRow   struct{ RunID, Test, Status, Error string; DurationNS int64 }          // Status: pass|fail|skip
type CheckRow  struct{ RunID, Test, InvariantID, Message, DetailsJSON string; OK bool; At time.Time }
type SampleRow struct{ RunID, Test, Phase, Method, PathTemplate, Err string; Worker, Seq int; Status int; LatencyNS int64; StartedAt time.Time }
type MetricRow struct{ RunID, Test, Name, Unit, LabelsJSON string; Value float64; At time.Time }

type Recorder interface{ Test(TestRow); Check(CheckRow); Sample(SampleRow); Metric(MetricRow) } // ★ hx/check/load record through this, never DuckDB
var  Discard Recorder                              // ★ drops rows
type Buffer struct{ … }                            // ★ in-memory Recorder for unit tests; Snapshot()

func Open(ctx context.Context, dir string, run RunRow) (*Sink, error) // in-memory DuckDB, creates dir
func (s *Sink) Test(TestRow); Check(CheckRow); Sample(SampleRow); Metric(MetricRow) // channel-backed; dropped after Close
func (s *Sink) Flush(ctx) error                   // ★ commit appended rows (harness calls it after every test)
func (s *Sink) Close(ctx context.Context) error   // drain, write run row, COPY every table to <dir>/<table>.parquet
func Query(ctx, root string) (*sql.DB, error)     // ★ in-memory DuckDB with a view per table over root/*/<table>.parquet
func NewRunID() string                            // sortable: 20260912T151504Z-ab3f
func JSON(v any) string                           // ★ marshal for *JSON fields; nil → "{}"
func Head[T any](v []T) any                       // ★ truncate a details slice for reports (20 + count)
const PhaseSetup = "setup"                        // ★ phase of harness.New's reset/healthz samples; Query adds view samples_measured = samples minus it
var  Tables = []string{"runs","tests","checks","samples","metrics"}

// hx — HTTP client bound to a target and a recorder.
package hx
type Client struct{ BaseURL string; HTTP *http.Client; Rec results.Recorder; RunID, Test string }
func New(baseURL string, rec results.Recorder, runID, test string) *Client
func (c *Client) Do(ctx, method, pathTemplate string, pathArgs map[string]string, body any, out any, opts ...Opt) (*Resp, error)
func (c *Client) Get(ctx, tmpl, args, out, opts...) / Post(ctx, tmpl, args, body, out, opts...) / Delete(ctx, tmpl, args, opts...)
func WithTimeout(d) Opt
type Resp struct{ Status int; Latency time.Duration; Body []byte }
func (r *Resp) OK() bool; func Is2xx(status int) bool   // ★ the one definition of "success" (2xx) used everywhere
// All clients share one http.Transport with MaxIdleConnsPerHost=512 so workers never redial mid-run.
// {key} in pathTemplate ← url.PathEscape(pathArgs[key]); body nil|[]byte|JSON; out decoded on 2xx.
// Every Do records exactly one SampleRow (PathTemplate is the template, so reports group by route).
// Non-2xx is NOT an error (tests inspect Status); transport errors / timeouts / cancel are.
// ★ Context labels, set by load and read by Do:
func WithWorker(ctx, id int) / WorkerFrom(ctx)      // Seq is a per-worker counter kept inside the client
func WithPhase(ctx, name string) / PhaseFrom(ctx)
type Counter struct{ Total, OK2xx, Non2xx, Errors atomic.Int64 }   // ★ per-request tallies
func WithCounter(ctx, *Counter) / CounterFrom(ctx)

// load — concurrency shapes. All start workers behind a barrier.
package load
type Worker struct{ ID int; Ctx context.Context; Iter int }   // Ctx = h.Ctx + WithWorker + WithCounter; ★ Iter = nth fn call (samples.seq counts requests)
type Result struct{ Total, OK2xx, Non2xx, Errors int; FnErrors int; Errs []error; Elapsed time.Duration } // ★ HTTP tallies from hx.Counter; FnErrors/Errs (≤100) from fn
func (r Result) Conclusive(h *harness.H) bool   // ★ false (and fails the test) if any request errored: final-state invariants can't be judged
func Closed(h *harness.H, workers, iterationsPerWorker int, fn func(*Worker) error) Result
func ClosedFor(h *harness.H, workers int, d time.Duration, fn func(*Worker) error) Result
func Phase(h *harness.H, name string, fn func())      // swaps h.Ctx for a phase-labelled one for the duration
// backlog (rate-limiter): func Open(h, rate float64, d time.Duration, fn) Result

// check — invariant assertions. Always record a CheckRow; fail the test if !ok.
package check
func Invariant(h *harness.H, id string, ok bool, msg string, details map[string]any)
func Eventually(h *harness.H, id string, timeout time.Duration, cond func() (bool, map[string]any), msg string)
func Metric(h *harness.H, name string, value float64, unit string, labels map[string]any)

// harness — glue used from tests.
package harness
func Main(m *testing.M, opts ...Option) int   // TargetFromEnv; install SIGINT/SIGTERM handler; refuse if Target.URL already answers; start SUT if Target.Cmd (R3, sut.log in the results dir); wait /healthz (fails fast if the SUT exits); open sink under HARNESS_RESULTS or <module>/results/runs/<run_id>; run; stop SUT; close sink (also on SIGINT — NOT on a test panic/-timeout: the process dies from the test goroutine). No target → tests skip, unless InProcess is given.
func InProcess(newHandler func() http.Handler) Option   // built R1: no env target → serve newHandler() on httptest, target = {pattern: cwd name, language: go, engine: memory, label: in-process}; results go to HARNESS_RESULTS if set, else results.Discard (in-process runs are not history). Crash tests skip (no Cmd — enforced by H.Restartable once R3 lands).
func New(t *testing.T) *H     // resets SUT via POST /_reset, checks /healthz (phase "setup"), t.Cleanup records TestRow + Flush
type H struct{ T *testing.T; Client *hx.Client; Target Target; Ctx context.Context }   // run/test identity lives on Client (Rec, RunID, Test)
func (h *H) Get/Post/Delete(...)          // ★ hx wrappers using h.Ctx
func (h *H) Must(resp, err) *hx.Resp      // ★ t.Fatal on error or non-2xx (setup steps)
func (h *H) Fail(msg)                     // ★ t.Error + remembers msg for TestRow.Error (testing.T hides its own)
func (h *H) Reset()
func (h *H) Restartable() bool            // built R3: true when Main started the SUT from Target.Cmd (also true for the memory target — crash tests skip on Engine == "memory" themselves)
func (h *H) Restart()                     // built R3: SIGKILL the SUT process group (t.Fatal if the URL still answers afterwards), start it again, wait /healthz; t.Fatal if it cannot. Call from the test goroutine only (it t.Fatals); not concurrent-safe. Does NOT reset. Requests in flight fail with transport errors — crash tests state their invariant as bounds (§4 kind 3), not via Result.Conclusive.
type Target struct{ Pattern, Language, Engine, URL, Label string; Cmd []string; Cwd string; Env map[string]string; Expect map[string]float64 }  // Cwd relative to the target file; Env merged over os.Environ
func LoadTarget(path) (Target, error); TargetFromEnv() (Target, ok bool, error); TargetEnv(Target) []string  // ★ env round-trip lives here; cmd/harness only calls these
const EnvTarget, EnvURL, EnvPattern, EnvLanguage, EnvEngine, EnvLabel, EnvRunID, EnvResults, EnvSUTRef  // ★ the HARNESS_* names, defined once
func ModuleRoot() string; Path(elem ...string) string; RunsDir() string   // ★ <module>/results/runs

// sut — one SUT process. Built R3. Independent of package harness (harness
// imports sut, not the reverse) so it takes a Spec, not a Target.
package sut
type Spec struct{ Cmd []string; Dir string; Env []string; Stdout, Stderr io.Writer }   // Stdout/Stderr default to a <results dir>/sut.log when run under Main
type Proc struct{ … }
func Start(spec Spec) (*Proc, error)      // exec with SysProcAttr{Setpgid: true}; does not wait for health (caller polls /healthz)
func (p *Proc) Kill() error               // SIGKILL the process group, reap
func (p *Proc) Stop(grace time.Duration) error   // SIGTERM, wait up to grace, then Kill
func (p *Proc) Exited() <-chan struct{}   // closed when the process is gone (lets Main notice a SUT that died on its own)
func (p *Proc) PID() int; ExitState() *os.ProcessState; Err() error   // ★ R3 additions for tests and error messages
// Cwd is resolved in harness.LoadTarget (relative to the target file; empty → the file's dir), not in sut.

// example-sut/counter — the reference counter as a library. Built R1 (memory + bugs + handler); engines R2.
package counter
type Store interface {
    Incr(ctx, name string, delta int64) (int64, error)   // atomic post-increment value; the primitive under test
    Get(ctx, name string) (int64, error)                  // 0 when absent
    Set(ctx, name string, v int64) error                  // ★ exists ONLY so the lost-update bug can be expressed as read+Set; a correct handler never calls it
    Del(ctx, name string) error
    Reset(ctx) error
    Ping(ctx) error                                       // /healthz
    Close() error
}
func NewHandler(s Store) http.Handler      // the contract: routes, name regexp, delta parsing, /healthz → Ping (503 {"ok":false} on error), /_reset → Reset; any store error → 500 {"error"} and a log line (never 2xx before the store returned)
func NewMemory() Store
func StoreTest(t *testing.T, open func() Store)   // ★ conformance test every engine runs (R2 callers): sequential semantics, Incr atomic under 32×200 callers, isolation; wipes and closes the store
// bugs are Store decorators, one per invariant they break:
func LostUpdate(Store) Store               // Incr = Get, yield, Set(old+delta)      → INV-01/02/05
func DropReset(Store) Store                // Reset is a no-op                        → INV-04
func WriteBehind(Store, flush time.Duration) Store   // Incr updates an in-memory shadow and responds; a goroutine flushes shadow → Set every `flush`. Reads come from the shadow, Del/Reset write through, Close flushes. Consistent unless killed → only INV-06 fails
func Slow(Store, d time.Duration) Store    // every op (incl. Ping) sleeps d          → nothing until [expect] exists
// engines, each its own package so cmd is the only importer:
//   store/sqlite.Open(dsn)   modernc.org/sqlite (same driver as the notes CLI; pure Go); WAL, busy_timeout; INSERT … ON CONFLICT DO UPDATE SET value = value + excluded.value RETURNING value
//   store/valkey.Open(dsn)   github.com/valkey-io/valkey-go; INCRBY / GET / DEL / FLUSHDB on the DB index in the DSN
//   store/postgres.Open(dsn) github.com/jackc/pgx/v5 via database/sql; same upsert … RETURNING; TRUNCATE
```

`cmd/counter` (the reference binary, `example-sut/cmd/counter`):

```
counter --addr 127.0.0.1:8080 --engine memory|sqlite|valkey|postgres [--dsn …] [--bug none|lost-update|drop-reset|write-behind|slow]
```

`--dsn` defaults (R2): sqlite → a stable file under `os.TempDir()` named from
`--addr` (so a restart of the same target reopens the same database — R4
depends on this); valkey → `redis://127.0.0.1:6379/1`;
postgres → the `infra/postgres.compose.yml` credentials. The binary composes
`NewHandler(bug(engine))`; there is no engine-specific bug code. As built
(R1): `openStore(engine, dsn)` has only `memory`; the other three names are
a clear "not implemented yet (R2)" error. Bug parameters are fixed in the
binary: `write-behind` flushes every 1 s, `slow` sleeps 20 ms.

`cmd/harness`:

```
harness run <pattern> (--target targets/x.toml | --url http://…)  [-- go test flags]
harness report [--run last|<run_id>] [--query summary|latency|checks|compare] [--format box|md|json]
harness sql "<duckdb sql>"          # tables runs/tests/checks/samples/metrics are pre-registered as views over results/runs/*/
harness new-suite <pattern>         # scaffolds suites/<pattern>/{CONTRACT.md,INVARIANTS.md,<pattern>_test.go} from templates
harness targets                     # lists targets/*.toml
```

As built: `report` also takes `--pattern` (for `history`), `--run first|a,b`,
`--runs-dir`; `sql -` reads stdin; `new-suite`/`targets` take `--dir`.
Queries are read from `<module>/queries/*.sql` at run time (not embedded), so
editing a `.sql` needs no rebuild. `run --target` checks the file's pattern
matches the positional one. Empty states (no runs, no targets) exit 0 with a
hint. `harness run`'s exit code is `go test`'s.

## 10. Work breakdown

### 10a. Next: the counter as the finished worked example (agreed 2026-09-12)

Goal: after this block the counter is done end to end — contract, suite,
one Go implementation on all four engines, a target file per engine, runs
and reports for each — and the harness can start, kill and restart a SUT.
That is the shape every later pattern (and every other language) starts
from. Order: **R0 → R1 → R2 ∥ R3 → R4 → R5**. Owners are the roles in
§10c: **main** (the session), **builder-opus**, **builder-sonnet**,
**reviewer**. Main keeps the items where a wrong call is expensive or the
work is the design itself (R1: the API every later item imports; R4: the
invariant, agreed with the user); everything with a clear spec and a
mechanical done-when is delegated; **everything is reviewed before it is
committed** (§10c), including main's own items. Every Owner and
Reviewed-by cell names both platforms' models (Claude Code / Codex) so an
item can be resumed from either after a quota limit.

| ID | Task | Owner | Reviewed by | Done when |
|----|------|-------|-------------|-----------|
| R0 ✅ | Commit the built toolkit as-is together with this plan and the three `.claude/agents/harness-*.md` role definitions; `go mod tidy` (direct deps are currently marked `// indirect`); fix `CONTRACT.md`'s example (`page:home` violates the agreed name regexp — change the example to `page.home`) | main (this session / astra) | main smell test only (already verified work + a one-line doc fix) | `git status` clean except results; `go test ./...` green |
| R1 ✅ | Reference counter as a library: move `example-sut/counter/main.go` into package `counter` (`Store`, `NewHandler`, `NewMemory`, bug decorators `LostUpdate`/`DropReset`/`WriteBehind`/`Slow`) + `example-sut/cmd/counter` binary with `--engine memory` only; `harness.Main` gains `Option` + `InProcess`; `suites/counter` passes `harness.InProcess(func() http.Handler { return counter.NewHandler(counter.NewMemory()) })`. Unit tests in package `counter` per bug: each decorator breaks the invariant it claims and nothing else (memory store, in-process). | main (this session / astra — the API every later item imports) | reviewer (opus high / sol high) | `go test ./...` **runs** the counter suite (no skip) and passes with `-race`; `harness run counter --url …` against `cmd/counter --bug lost-update` still fails INV-01/02/05 |
| R2 ✅ | Engine stores: `store/sqlite`, `store/valkey`, `store/postgres` (§9 APIs, deps listed there); `cmd/counter --engine … --dsn …`; `/healthz` pings the store; four target files `targets/counter-go-{memory,sqlite,valkey,postgres}.toml` each with `cmd`/`cwd` (Q5) so R3 can start them; `README.md` quick start shows one engine end to end. A shared conformance test in package `counter` (`StoreTest(t, func() Store)`) runs against every engine, skipping valkey/postgres when the store is unreachable. Only R2 may touch `go.mod`/`go.sum`. | builder-opus (opus medium / sol medium — three drivers with different quirks — valkey-go's command builder, pgx via `database/sql`, SQLite WAL/busy_timeout — and upsert semantics that *are* the primitive under test) | reviewer (opus high / sol high) | Suite passes against each engine via `harness run counter --target targets/counter-go-<engine>.toml`; `lost-update` fails INV-01 on every engine; `harness report --query history --pattern counter` shows the four runs side by side |
| R3 ✅ | `sut` package (§9) + `harness.Main` integration: start `Target.Cmd` (cwd relative to the target file, env merged) before the health wait, log to `<results dir>/sut.log`, stop at exit / SIGINT / panic; `H.Restartable`, `H.Restart`; `cmd/harness run` prints "started SUT pid …" and fails fast if the process exits before health. Unit tests spawn `example-sut/cmd/counter --engine memory` (build once in `TestMain`; sqlite lands in parallel with R2, and state-across-restart is an engine property covered by R2's `StoreTest` and R4): start → healthy; kill → connection refused; restart → healthy with a new pid; Stop with grace → clean exit. Files: `sut/*`, `harness/main.go`, `harness/h.go`, `cmd/harness/run.go` only. | first pass: builder-sonnet (sonnet high / luna high); review fixes: builder-opus (opus medium / sol medium — reassigned by the user 2026-09-12, see §0) | reviewer (opus high / sol high — process-group kill, reaping, and the SIGINT paths are where races hide) | tests green with `-race`; `harness run counter --target targets/counter-go-sqlite.toml` starts and stops the SUT itself |
| R4 ✅ | INV-COUNTER-06 in `INVARIANTS.md` (agree wording with the user first): *After the SUT is killed and restarted mid-load, the final value is bounded by `2xx-before-kill ≤ final ≤ 2xx-before-kill + errored`: every acknowledged increment survives, and no increment is acknowledged before it is committed.* Guaranteed by: respond only after the store's atomic write returns (application discipline); the store's own persistence. Kind: crash, must. Test: `ClosedFor` workers on one name; `h.Restart()` once from a goroutine after ~⅓ of the expected requests (count-triggered, not time-triggered); tally 2xx before the kill and transport errors around it; `GET` after restart; skip when `!h.Restartable()`. Counter's crash test lands here rather than on a pattern that does not exist yet. | main (this session / astra — the invariant, agreed with the user) | reviewer (opus high / sol high) | passes on `--engine sqlite\|valkey\|postgres --bug none`; fails INV-06 on `--bug write-behind` (each engine); skips on memory / `--url` targets |
| R5 ✅ | De-phase every doc: this file's remaining "phase" wording, `AGENTS.md` (steps 2.4, 4), `README.md` (targets, new-suite), `knowledge/harness.md` (implemented vs planned), `harness/h.go` Target comments, `target.go` LoadTarget comment, `cmd/harness/templates/invariants.md.tmpl` answer 4, `suites/counter/INVARIANTS.md` answer 4 (now INV-06). Update `knowledge/index.md` if entries change; record lessons with the `update-trinkets-knowledge` skill. | builder-sonnet (sonnet high / luna high — mechanical sweep; must verify every claim against the code as it stands after R1–R4) | reviewer (opus high / sol high — docs are the post-context-clear truth; checks each statement against source) | `grep -ri "phase [0-9]" harness/ knowledge/` finds nothing; `knowledge/harness.md` describes only what exists |

### 10c. Working protocol (roles, review, commits, the log)

Purpose: keep main's context thin — design, decisions, the §0 log, and a
final smell test — and make every commit a reviewed one.

**Roles** — the roles are tool-agnostic; the model behind each depends on
which harness runs the session. Under Claude Code they are project agent
definitions under `.claude/agents/` (frontmatter keys `model`, `effort`,
`tools`; effort is fixed per definition, so one file per role). Under
Codex the same roles map to Codex models as in the last column: the
**controlling agent is `astra`** and owns everything marked main; wherever
this plan says Sonnet use **`luna` at high effort**; wherever it says Opus
use **`sol`** at the effort given. The role's job, tool restriction, brief
and report format do not change.

| Role | Claude Code file | Claude Code frontmatter | Codex model | Job |
|------|------------------|-------------------------|-------------|-----|
| main | this session | — | `astra` (controlling agent) | owns this plan and §0, decisions, R1/R4, briefs, dispatch, final smell test, commits |
| builder-opus | `.claude/agents/harness-builder-opus.md` | `model: opus`, `effort: medium` | `sol`, medium | R2 |
| builder-sonnet | `.claude/agents/harness-builder-sonnet.md` | `model: sonnet`, `effort: high` | `luna`, high | R3, R5 |
| reviewer | `.claude/agents/harness-reviewer.md` | `model: opus`, `effort: high`, `tools: Read, Grep, Glob, Bash` (no edits) | `sol`, high, read-only | reviews every R-item before commit |

The `.claude/agents/harness-*.md` bodies are the role briefs; a Codex
session reads them as prose (ignore the frontmatter) and applies the model
column above.

**One R-item, start to commit:**

1. **Kick-off** — main appends to §0: `R<n> started → <owner>`. For a
   delegated item main writes a self-contained brief: the R-row verbatim,
   the §9 block it implements (copied, not referenced), the file list it may
   touch, "match surrounding style, stdlib first, only the deps §9 lists",
   the done-when line, and the report format below. Builders do not commit
   and do not edit this file; if they need a decision they stop and ask in
   their report.
2. **Build** — builder works in the shared working tree on its file list
   (R2 ∥ R3 are disjoint; only R2 touches `go.mod`). Builder's report:
   files changed, how it verified (commands + one-line outcomes),
   deviations from §9 with reasons, open questions. Nothing else.
3. **Review** — main dispatches **reviewer** with the same brief plus the
   builder's report. Reviewer runs `go build ./... && go vet ./... && go test
   -race ./...` and the item's done-when commands, reads the diff against
   §9 and the done-when, and reports findings ranked must-fix / should / nit,
   each with `file:line` and a concrete failure scenario. Reviewer never
   edits. Must-fix and should findings go back to the *same* builder agent
   (SendMessage — keeps its context) and the reviewer re-checks; loop until
   the reviewer reports clean or nits only. Main's own items (R1, R4) go
   through the same reviewer step.
4. **Land** — main reads only the reviewer's final verdict and the builder's
   report (not the diff, unless a finding needs a decision); runs a smell
   test (`git status`, `git diff --stat`, one done-when command); updates §9
   if the API deviated (plan first, then code is truth); appends to §0:
   `R<n> reviewed clean (<k> findings fixed) → committed <sha>`; commits
   **one commit per R-item** with the item in the subject.
5. **After a context clear** — the new session reads the last §0 entries,
   `git log --oneline -5`, and `git status`. Anything in the tree but not in
   §0 as committed is in-flight: dispatch the reviewer on the diff before
   touching it. Running agents do not survive a clear; their work in the
   tree does.

**§0 entries are mandatory at:** item start, delegation, review verdict,
commit, any decision change (§2/§9 first, then a §0 line pointing at it),
and any blocker or open question for the user.

### 10b. History: the first block (built 2026-09-12)

Order was: **A → B in parallel with C/D/E → F → G**. The main session did A,
B, F, G (cgo sink, the `harness.H` glue every test touches, the invariant
list, integration); subagents got self-contained packages.

| ID | Task | Owner | Inputs | Done when |
|----|------|-------|--------|-----------|
| A ✅ | Module skeleton: `harness/go.mod`, folder layout from §5, root `.gitignore` additions (`harness/results/`, `systems-trinkets` binary), `results` row types **and Sink** (appender collector, SIGINT/panic-safe export, `NewRunID`) | **me** | §6, §9 | `go test ./results/` writes 5 parquet files from an in-memory sink and reads p99 back via `read_parquet`; 100k samples in < 1 s |
| B ✅ | `harness` package: `Main`, `New`, `H`, `Target` (TOML loader, `HARNESS_TARGET`/`HARNESS_URL`), reset/health handshake, `TestRow` on cleanup | **me** | A | A dummy suite against `example-sut` records run + test rows |
| C ✅ | `hx` client: `Do`/`Get`/`Post`/`Delete`, path templates, JSON in/out, timeouts, context values for worker/phase/seq, sample recording through an injected recorder interface (so it does not import DuckDB) | **sonnet** subagent | §9 signatures, recorder interface from A | Unit tests with `httptest.Server`; every request yields exactly one sample; non-2xx not an error; transport error is |
| D ✅ | `load` package: `Closed`, `ClosedFor`, `Phase`, barrier start, `Result` aggregation, `-race` clean | **sonnet** subagent | §9 signatures | Unit tests prove all workers start after the barrier, counts add up, ctx cancel stops workers |
| E ✅ | `example-sut/counter`: Go `net/http` server implementing `suites/counter/CONTRACT.md`, in-memory, flags `--addr`, `--bug none|lost-update|drop-reset|slow`; `lost-update` does a racy read-modify-write with a `runtime.Gosched()` | **sonnet** subagent | CONTRACT.md from F | `go vet`, runs, `curl` smoke; `--bug lost-update` observably loses increments under 32 goroutines |
| F ✅ | `suites/counter/CONTRACT.md` + `INVARIANTS.md` via the §8 interview with the user; then `counter_test.go` (sequential contract test, concurrent increment invariant, reset semantics, perf metrics) | **me** (interview + tests) | B, C, D | Passes against `example-sut --bug none`; fails on `INV-COUNTER-01` against `--bug lost-update` |
| G ✅ | `cmd/harness`: `run`, `report` (canned SQL in `queries/`, views over `results/runs/*/`), `sql`, `new-suite` (templates), `targets`; `check` package | **opus** subagent for CLI + queries; `check` is small enough that I do it in B | A, B | `harness run counter --url …` then `harness report --run last` prints summary + latency tables from parquet; `new-suite foo` compiles |
| H ✅ | `infra/valkey.compose.yml`, `infra/postgres.compose.yml`, `harness/README.md` (how to run; points here) | **sonnet** subagent | §5 | `docker compose -f … up -d` works; README accurate |

Subagent briefs must include: the exact §9 signatures, the recorder interface
from A (so `hx`/`load` never import DuckDB), "match the surrounding code
style, stdlib first, no new deps without listing them", and the done-when
line. I review every subagent result before integrating (F/G depend on it).

**Verification (2026-09-12):** `go test ./...` green (suites skip
without a target); `harness run counter --url …` against `example-sut --bug
none` passes all 4 tests / 23 checks; against `--bug lost-update` fails
INV-COUNTER-01, -02, -05 with named invariants (≈5k of 6.4k increments lost);
against `--bug drop-reset` fails INV-COUNTER-04. `harness report` (`summary`,
`latency`, `checks`, `compare`, `history`), `sql`, `new-suite`, `targets` all
work from the Parquet files. Notes carried forward:

- `--bug slow` exists but nothing asserts on it (perf is recorded only); it
  becomes useful once `[expect]` thresholds exist (§7 backlog).
- `harness run`'s exit code is `go test`'s, so a CI step can gate on it.
- The Appender path handled 6.4k samples/test with no visible overhead;
  the sampling knob in §11 is not needed yet.

## 11. Open questions / risks

- SQLite targets: a single-writer engine will serialize writes; concurrency invariants still hold but throughput will differ. Fine — that's the point of the comparison — but tests must not depend on throughput.
- Lease/lock "no two holders at once" can only be checked from client-observed intervals (request sent → response received); this is a sound but conservative check. Say so in `INVARIANTS.md`.
- `/_reset` in a production-shaped server is a smell; it is gated behind an env var in the SUT or only compiled in test builds. The contract will say so.
- Results volume: `samples` at ~100 bytes/row × 100k rows/run is ~10 MB in memory, ~1–2 MB as Parquet. Sampling (every Nth request) is a knob if it ever matters.
