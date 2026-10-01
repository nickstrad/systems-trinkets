# Testing norm for project work: spec tests, invariants, property fuzzing

Applies to project work (shared helpers under `internal/`, harnesses, the
platform components and integration chunks in `docs/lessons/projects.md`),
not to learner-typed lesson cores. The rule is in the root `AGENTS.md`
("Testing norm for project work"); the user set it on 2026-09-30. The first
plan written to it is
[docs/plans/docker-harness-prereqs.md](../docs/plans/docker-harness-prereqs.md).

## The three layers

1. **Spec tests:** one `TestSpec_<Item>_<Behaviour>` per "Done when" line of
   the plan. `go test -list '^TestSpec_' ./...` should read like the plan.
2. **Invariants:** named functions in the package (`invariants.go`) that
   return violations. The production path calls them (refuse to act on a
   violation) and the tests call the same functions.
3. **Property and fuzz tests:** the invariants asserted over generated
   inputs. Write the property once as `func(*rapid.T)`, then:

   ```go
   func TestProp_Translate(t *testing.T) { rapid.Check(t, propTranslate) }
   func FuzzTranslate(f *testing.F)      { f.Fuzz(rapid.MakeFuzz(propTranslate)) }
   ```

   `rapid.Check` gives random structured inputs with shrinking;
   `rapid.MakeFuzz` lets Go's coverage-guided fuzzer drive the same
   generators from bytes. Plain byte or string parsers use a native
   `f.Fuzz(func(t *testing.T, s string) {...})` target directly.

## Verified 2026-09-30 (scratch module, Go 1.26.8, `pgregory.net/rapid` v1.3.0)

A translation function with a planted bug (it compared an uncleaned mount
path with the Docker socket path) and one shared `Invariants` function:

- `rapid.Check` failed after 5 cases and shrank to a single mount with
  source `/var/run/../run/docker.sock`.
- `go test -fuzz '^FuzzTranslateRapid$'` found the same input in under 4 s
  and wrote it to `testdata/fuzz/FuzzTranslateRapid/<hash>`.
- A native target with `(string, int64, bool)` arguments also failed, but on
  the volume name `0docker.sock`: the test's invariant said "source ends with
  `docker.sock`", which is wider than the rule. **An invariant must be as
  exact as the rule it states**, or the fuzzer reports the invariant.
- `rapid` has no transitive dependencies; `go.mod` gains one line.

## Gotchas

- `go test -fuzz` takes exactly one target in one package. A pattern that
  matches two targets fails without fuzzing, so a `make fuzz` target loops
  over `go test -list '^Fuzz' <pkg>` and runs each with
  `-run '^$' -fuzz '^<Target>$' -fuzztime $(FUZZTIME)`.
- Native fuzz arguments are limited to primitives, `string` and `[]byte`.
  Structured inputs come from `rapid.MakeFuzz` or a decoder in the test.
- Files in `testdata/fuzz/<Target>/` are replayed by plain `go test`, so a
  committed failing input is a regression test. Commit them.
- `rapid.Check` writes `testdata/rapid/<Test>/*.fail` on failure. Reproduce
  with the printed `-rapid.failfile` or `-rapid.seed`, then keep the case as
  a fuzz seed or a spec test and leave `testdata/rapid/` out of git.
- `.gitignore` anchoring: a pattern with a slash in the middle
  (`testdata/rapid/`) matches only relative to the `.gitignore`, i.e. at the
  repo root, so `internal/pkg/testdata/rapid/` stayed untracked-visible. Use
  `**/testdata/rapid/`. Check with `git check-ignore -v <path>` or
  `git status --short --ignored`.
- A property test that talks to a daemon or service is slow: gate it and cap
  it (`-rapid.checks=<n>`).

Verified 2026-10-01 in the repo (work item H1): `make test` (`go test ./...`),
`make fuzz` (loops every `Fuzz*` target, one `go test -fuzz` run each, stops
on the first failure; `FUZZTIME` defaults to 10s) and `make check-docker`
(`TRINKETS_DOCKER=1 go test -count=1 ./internal/lab/docker/...`) all exit 0.
Without `TRINKETS_DOCKER=1` the gated tests skip through `requireDocker(t)`.
The `fuzz` loop captures `go list ./...` behind an `|| exit 1` guard and each
`go test -list` output before looping, so a module or package that fails to
list or compile fails the target instead of looking like "no targets".
