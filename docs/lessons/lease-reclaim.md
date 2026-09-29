# Reclaim abandoned work with a lease

Go · Valkey · Availability · about 20 minutes.

When an owner stops renewing, can another worker claim its resource?
Compare `no-expiry` with `lease`. Adapted from the
[shared lesson](https://chatgpt.com/share/6abaf7b2-4668-83ea-8288-de0293c993a5).

## Operation and measurement paths

```text
 Owner A                               Contender B
    |                                      |
    | Acquire: SET key A NX [PX ttl]        | Acquire: SET key B NX
    | Renew: compare token + extend TTL     |          [PX ttl]
    |         in one Lua operation         |
    +-----------------+--------------------+
                      v
              +----------------+
              |     Valkey     |
              | resource -> A  |
              +----------------+
                      |
              replies to runner
                      v
                 main.go
         checks + reclaim timer
                      |
                      v
              measurements.csv
                      |
                      v
             DuckDB / analyze.sql
```

```text
                  final renewal; A stops here
                              |<--- reclaim_ms --->|
                              t0                   |
 no-expiry:  A owns -----------+--------------------+---- A owns
 lease:      A owns -----------+---- TTL expires ---+---- B owns
                              |                   ^
 contender:                   fail  fail  fail ... success
                              |                        |
                          timer starts           timer stops

 no-expiry: stop observing at 900 ms; reclamation is unobserved.
 lease:     expect roughly 500 ms + polling/request overhead.
```

## Create these files

```sh
mkdir -p lessons/go/lease-reclaim/core
```

```text
lessons/go/lease-reclaim/
  core/core.go     acquire and renew operations
  main.go          fixtures, interruption, polling, checks, CSV
  analyze.sql      availability comparison
```

### lessons/go/lease-reclaim/core/core.go

```go
package core

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

type Mode string

const (
	NoExpiry Mode = "no-expiry"
	Lease    Mode = "lease"
)

func ResourceKey(resource string) string {
	return "trinkets:lease-reclaim:resource:" + resource
}

func expiry(mode Mode, ttl time.Duration) (time.Duration, error) {
	switch mode {
	case NoExpiry:
		return 0, nil
	case Lease:
		if ttl >= time.Millisecond && ttl%time.Millisecond == 0 {
			return ttl, nil
		}
		return 0, fmt.Errorf("lease TTL must be positive whole milliseconds")
	default:
		return 0, fmt.Errorf("unknown mode %q", mode)
	}
}

// Acquire returns false, nil for ordinary contention.
// Use a fresh owner token for each ownership attempt, not a reusable worker ID.
// A true result describes this operation; it does not promise future ownership.
func Acquire(ctx context.Context, client *redis.Client,
	resource, owner string, mode Mode, ttl time.Duration,
) (bool, error) {
	duration, err := expiry(mode, ttl)
	if err != nil {
		return false, err
	}
	return client.SetNX(ctx, ResourceKey(resource), owner, duration).Result()
}

// Renew returns false, nil when the token no longer owns the resource.
// Checking the token and changing expiry must be one atomic operation.
// The no-expiry variant checks ownership without adding an expiration.
func Renew(ctx context.Context, client *redis.Client,
	resource, owner string, mode Mode, ttl time.Duration,
) (bool, error) {
	duration, err := expiry(mode, ttl)
	if err != nil {
		return false, err
	}
	const script = `
if redis.call("GET", KEYS[1]) ~= ARGV[1] then return 0 end
if tonumber(ARGV[2]) == 0 then return 1 end
return redis.call("PEXPIRE", KEYS[1], ARGV[2])
`
	n, err := client.Eval(ctx, script, []string{ResourceKey(resource)},
		owner, duration.Milliseconds()).Int()
	return n == 1, err
}
```

### lessons/go/lease-reclaim/main.go

```go
package main

import (
	"context"
	"crypto/rand"
	"fmt"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/nickstrad/systems-trinkets/internal/lab"
	"github.com/nickstrad/systems-trinkets/internal/lab/valkey"
	"github.com/nickstrad/systems-trinkets/lessons/go/lease-reclaim/core"
)

const (
	leaseTTL          = 500 * time.Millisecond
	renewEvery        = 150 * time.Millisecond
	pollEvery         = 25 * time.Millisecond
	observationWindow = 900 * time.Millisecond
	runs              = 3
)

func main() {
	client := valkey.Connect(context.Background())
	defer client.Close()
	measurements := lab.NewMeasurements(
		"variant", "run", "resource", "ttl_ms", "window_ms", "observed_ms",
		"outcome", "reclaim_ms", "attempts", "acquired", "invariant_ok",
	)
	defer measurements.Close()
	fmt.Println("Valkey:", valkey.URL())

	for _, mode := range []core.Mode{core.NoExpiry, core.Lease} {
		for run := 1; run <= runs; run++ {
			trial(client, measurements, mode, run)
		}
	}
}

func trial(client *redis.Client, measurements *lab.Measurements,
	mode core.Mode, run int,
) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	resource := rand.Text() // Private to this trial, including parallel runs.
	owner, contender := rand.Text(), rand.Text()
	key := core.ResourceKey(resource)
	defer func() {
		cleanup, stop := context.WithTimeout(context.Background(), 2*time.Second)
		defer stop()
		lab.Check(client.Del(cleanup, key).Err())
	}()

	ok, err := core.Acquire(ctx, client, resource, owner, mode, leaseTTL)
	lab.Check(err)
	require(ok, "initial owner must acquire")

	// Three healthy renewals are outside the reclaim timer.
	for range 3 {
		time.Sleep(renewEvery)
		ok, err = core.Acquire(ctx, client, resource, contender, mode, leaseTTL)
		lab.Check(err)
		require(!ok, "contender must be blocked while ownership is live")
		ok, err = core.Renew(ctx, client, resource, contender, mode, leaseTTL)
		lab.Check(err)
		require(!ok, "wrong token must not renew")
		ok, err = core.Renew(ctx, client, resource, owner, mode, leaseTTL)
		lab.Check(err)
		require(ok, "owner must renew before interruption")
	}

	// Simulate abandonment: A makes no more calls while B polls.
	start := time.Now()
	attempts, acquired := 0, false
	for time.Since(start) < observationWindow {
		attempts++
		acquired, err = core.Acquire(ctx, client, resource, contender, mode, leaseTTL)
		lab.Check(err)
		if acquired {
			break
		}
		time.Sleep(pollEvery)
	}
	observed := time.Since(start)
	reclaimMS, outcome := "", "still_blocked"
	if acquired {
		reclaimMS = fmt.Sprintf("%.3f", lab.Ms(observed))
		outcome = "reclaimed"
	} else if mode == core.Lease {
		outcome = "not_reclaimed"
	}
	invariantOK := acquired == (mode == core.Lease)

	if acquired {
		// A resumes after B won: A must not extend B's lease.
		ok, err = core.Renew(ctx, client, resource, owner, mode, leaseTTL)
		lab.Check(err)
		invariantOK = invariantOK && !ok
		actualOwner, err := client.Get(ctx, key).Result()
		lab.Check(err)
		invariantOK = invariantOK && actualOwner == contender
	}

	// Blank reclaim_ms means unobserved, not zero or negative latency.
	measurements.Write(string(mode), strconv.Itoa(run), resource,
		strconv.FormatInt(leaseTTL.Milliseconds(), 10),
		strconv.FormatInt(observationWindow.Milliseconds(), 10),
		fmt.Sprintf("%.3f", lab.Ms(observed)), outcome, reclaimMS,
		strconv.Itoa(attempts), strconv.FormatBool(acquired),
		strconv.FormatBool(invariantOK))
	fmt.Printf("%-9s run=%d %-13s observed=%.1f ms invariant=%t\n",
		mode, run, outcome, lab.Ms(observed), invariantOK)
	require(invariantOK, "variant outcome or ownership check failed")
}

func require(ok bool, message string) {
	if !ok {
		panic(message)
	}
}
```

### lessons/go/lease-reclaim/analyze.sql

```sql
create table measurements as
from read_csv('measurements.csv', types = {'reclaim_ms': 'DOUBLE'});

.print 'Every row must pass; blank reclaim_ms means unobserved'
select variant, run, outcome, acquired, invariant_ok,
       reclaim_ms, round(observed_ms, 1) as observed_ms
from measurements
order by variant, run;

.print 'Compare recovery counts before comparing latency'
select variant, count(*) as trials,
       count(*) filter (where acquired) as reclaimed,
       count(*) filter (where not acquired) as unobserved,
       count(*) filter (where not invariant_ok) as failures,
       round(median(reclaim_ms), 1) as p50_reclaim_ms
from measurements
group by all
order by variant;

.print 'Question: does expiry restore availability within the window?'
select
  count(*) filter (where variant = 'no-expiry' and acquired)
    as no_expiry_reclaims,
  count(*) filter (where variant = 'lease' and acquired)
    as lease_reclaims,
  round(median(reclaim_ms) filter (where variant = 'lease'), 1)
    as lease_p50_reclaim_ms
from measurements;
```

## Run

From the repository root, after typing the three files:

```sh
make up-valkey
make lab-lease-reclaim
make analyze-lease-reclaim
```

Requires Go, Docker, and the DuckDB CLI; no new dependencies. The helper uses
`redis://localhost:6379` with no authentication. Each trial deletes only its
own random key, including on an ordinary panic. A forced kill can leave a
`no-expiry` key behind.

Expect zero `no_expiry_reclaims`, three `lease_reclaims`, and zero failures.
`lease_p50_reclaim_ms` should be near 500 ms. Set `leaseTTL` to 250 ms and
repeat; keep `renewEvery` below the TTL and `observationWindow` above it.

The timer starts after the last renewal reply and includes remaining TTL,
polling, requests, and scheduling. It excludes setup, healthy renewals,
post-reclaim checks, CSV, and cleanup. `observed_ms` can exceed the nominal
window by a poll/request delay. Unobserved reclamation is NULL in DuckDB.
This is a simulated abandoned client; external sandbox writes and fencing,
service crashes, replication, and failover are outside the experiment.

## k6 build plan

Build this after the base lesson has been typed and run. The goal is the
same availability comparison under concurrent acquisition attempts.

### Files and operation contract

Create `perf/main.go`, `perf/k6.ts`, `perf/analyze.sql`, and a short
`perf/README.md`. Reuse `internal/lab/perf`, `scripts/perf/run.ts`,
`scripts/perf/workload.ts`, and the shared analysis; call `core.Acquire`,
`core.Renew`, and `core.ResourceKey` directly.

- `POST /trials?variant=lease`: create a bounded private trial with a fresh
  owner token and acquire its resource. Return trial ID and owner token.
  This fixture call is excluded from operation latency.
- `POST /operation?variant=lease&trial=<id>&action=acquire&owner=<token>`:
  one `core.Acquire` call. Return `{"variant":"lease","acquired":false}`
  for contention with HTTP 200. Renewal uses `action=renew` and returns
  `{"variant":"lease","renewed":true}`. Success means the local Valkey
  operation took effect, without any replication or disk durability claim;
  it does not mean a sandbox was provisioned or its work completed.
- `POST /trials/<id>/abandon`: perform one final owner renewal, then record
  a monotonic start time on the server immediately after its reply. No
  owner renewals occur during recovery polling. This is setup traffic.
- The first successful contender acquisition after abandonment also
  returns `reclaim_ms`, measured against that server clock. Later successes
  do not produce a second recovery sample for the same trial.
- `POST /trials/<id>/finish`: after outstanding calls finish, read stored
  ownership for the final check, finalize one summary, and delete only this
  trial's key and live metadata. On an unreclaimed baseline, check that the
  stored token is still the original owner and `PTTL` is -1.
- Reject unknown variants/actions/trials, empty or oversized tokens, and
  cross-variant trial IDs with 400/404. Dependency errors are 5xx, never
  normal contention. Use bounds on trial count and request duration.

Use both `no-expiry` and `lease` in every run. Each iteration chooses a
variant round-robin and owns one trial; a new server adds a random namespace
to every resource. Tokens are unique per claimant and acquisition attempt.
Retries in the same polling attempt can keep the same token. Never accept
an arbitrary storage key from a request or reset a shared fixture in a
measured handler.

### Workload phases

```text
 new trial -> A owns -> B/C/D attempt while A renews
                            |
                      all must lose
                            v
                 final renewal -> abandon A
                            |
              batch B/C/D acquire every 25 ms
                            |
             +--------------+---------------+
             |                              |
        no-expiry                         lease
        no recovery                 first recovery sample
             |                              |
        window ends                 stale A renewal fails
             +--------------+---------------+
                            v
                finish -> read state -> cleanup
```

1. Warm connections with disposable fixtures before recording samples.
2. Create the trial, then renew A while a batch of three distinct
   contenders tries to acquire. All contenders must lose while A's
   ownership is live; a wrong-token renewal must return false. Avoid a
   long healthy phase where TTL expiry would make this assertion ambiguous.
3. Call abandon, stop A's renewals, and poll with concurrent batches at
   `POLL_MS=25` until first success or `WINDOW_MS=1500`. A batch uses the
   shared `operationParams` tags. Stop sending new polls after success;
   wait for the current batch before finalization.
4. For `lease`, record exactly one recovery and try A's stale renewal: it
   must fail. For `no-expiry`, observe the entire window with no recovery.
   Read actual storage in finish, not just the HTTP success counter.
5. Finish and clean the trial even when an assertion fails. Retain bounded
   aggregate counters for `/stats`. Drain outstanding iterations before
   teardown, then check the per-variant counts and that active trials are
   zero. Adapter shutdown deletes its remaining owned keys.

Finite leases can expire between two successful replies. Do not assert
that different clients can never receive success at different times. Add a
separate contention probe during setup: race eight unique tokens against
one empty `no-expiry` key, assert exactly one winner, and read back that
winner's token. Never expire or reset that key until the batch finishes.
This tests atomic acquisition. It does not prove an expired worker stopped
writing to an external resource; that would need fencing.

### Metrics, settings, and checks

`GET /health` reports `LEASE_TTL_MS` (500), `POLL_MS` (25), `WINDOW_MS`
(1500), `CONTENDERS` (3), `MAX_TRIALS` (2000 total), and the maximum active
trials (32). Reject a window no larger than TTL. Bound lease TTL to
100–1000 ms and contender count to 1–8 for this lab. Stop creating trials
at the cap and report that run limit explicitly.

`GET /stats` exposes, per variant: `started`, `finished`, `reclaimed`,
`unobserved`, `unexpected_errors`, `stale_renew_attempts`,
`stale_renew_accepted`, `ownership_check_failures`, and `active`.
Also expose the contention probe's `expected_winners=1`, `actual_winners`,
and stored-token match. Synchronize instrumentation separately from Valkey
operations so a server mutex does not serialize the acquisition race.

Record `reclaim_ms` as a Trend once per recovered trial, and recovery as a
Rate once per finished trial, both tagged by variant. Keep `name=operation`
and `variant` on measured HTTP calls; setup/finish/health/stats use other
names. Track HTTP errors separately from expected `acquired=false` replies.
No synthetic zero or timeout value enters the reclaim Trend.

In `perf/analyze.sql`, reproduce the base table's `trials`, `reclaimed`,
`unobserved`, `failures`, and `p50_reclaim_ms` by variant. Get counts from
`domain.json` and latency from the custom metric; show HTTP p95 separately.
The question table must still show `no_expiry_reclaims`, `lease_reclaims`,
and `lease_p50_reclaim_ms`. Print effective TTL/window settings alongside
the comparison. A baseline with no recovery has NULL latency and a nonzero
unobserved count, not a fast latency.

Acceptance criteria after draining:

- Both variants appear. `started = finished = reclaimed + unobserved` and
  `active = 0` per variant; no dependency errors or ownership failures.
- `no-expiry`: `reclaimed = 0`; `lease`: `reclaimed = finished`.
- Every recovered lease triggers one stale renewal attempt; none succeeds.
- The separate contention probe has one winner whose token matches storage.
- All correctness checks pass; no dropped iterations in smoke/default load.
  Use an illustrative HTTP p95 budget of 250 ms for the short operation
  calls. The 1500 ms recovery window is a separate domain budget.

### Runs to build and execute later

Default smoke: one VU for 10 seconds. Load: five trial iterations/second
for 30 seconds, at most 32 VUs. `RATE` counts trials, not individual polling
requests. Both runs include both variants. Compare TTL 500 vs 250 ms with
all other settings fixed; expect shorter recovery, without assuming exact
halving because polling and request time remain.

These commands become available only after the adapter/workload exists:

```sh
make lab-k6-lease-reclaim
make lab-k6-lease-reclaim PROFILE=load RATE=5 DURATION_S=30 LEASE_TTL_MS=500
make lab-k6-lease-reclaim PROFILE=load RATE=5 DURATION_S=30 LEASE_TTL_MS=250
make analyze-k6-lease-reclaim
```

Before building, read the completed source and its actual measurements.
Then invoke:

```text
$add-basic-k6-testing lessons/go/lease-reclaim
Use the k6 build plan in docs/lessons/lease-reclaim.md.
```
