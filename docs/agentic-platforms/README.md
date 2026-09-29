# Agentic platform architecture research

Research checked 2026-09-29 against primary documentation, public source, and
engineering posts. These notes inform future lessons and software suggestions;
they do not configure or benchmark the platforms. Links near each claim identify
the evidence. Recheck changing features and pin source revisions before turning
an implementation detail into a lesson dependency.

| Platform notes | Useful mechanisms |
|---|---|
| [Meta Muse](meta-muse.md) | Focused runtime/supervisor design, broker identity, scoped approvals, credential mediation, egress and information-flow lesson candidates |
| [Daytona](daytona.md) | API/runner/guest boundaries, desired-state reconciliation, container versus VM lifecycle, warm-pool claims, images and volumes |
| [E2B](e2b.md) | Firecracker, template restore, lazy memory/disk access, copy-on-write, pause durability, placement and cache locality |
| [Vercel Sandbox](vercel-sandbox.md) | Hive hosts and cells, guest/container boundary, parallel restore, decoded-image cache, durable identity versus sessions |
| [Vercel Workflow](vercel-workflow.md) | World adapters, event history, queues and function invocation, replay, durable timers, step retry boundaries |
| [Inngest](inngest.md) | Handler reinvocation, memoized step results, active-step concurrency, keyed limits and retries |
| [Modal](modal.md) | Autoscaling pools, warm capacity, sandbox isolation choices and compatibility |
| [Temporal and Restate](durable-execution.md) | Deterministic history replay versus journaled handlers, keyed execution, activity retries and external effects |

[daily-linux-lesson-prompt.md](daily-linux-lesson-prompt.md) holds a paste-ready
prompt for a scheduled chat thread that teaches one Linux mechanism per day
from the list above, within the local runtime boundary.

## Turn research into a lesson

Start with one question and put it in [ideas.md](../lessons/ideas.md). The notes'
experiment suggestions are candidates, not already accepted working plans.
Select one comparison, an observable invariant, one or two primary services,
and a bounded failure schedule. Analyze actual local measurements with DuckDB.
Examples worth connecting across platforms:

- **Ownership and reconciliation:** lease expiry restores availability, while
  fencing at the destination prevents a stale owner from applying a write.
- **Prepared state:** warm inventory, immutable templates, and local caches
  trade storage or idle compute for less work on the request path. Separate
  time to readiness from the first useful operation and later cache misses.
- **Durable execution:** a stored step result permits reuse after retry; it does
  not make an external side effect atomic with recording that result.
- **Waiting versus running:** persist due times and release workers, then
  reconcile overdue work after an outage. Limit active work independently from
  queued or sleeping work.

These are synthesis and lesson-design inferences from the linked notes, not
claims that all providers implement the same mechanism.

## Software boundary

[software/software.md](../../software/software.md) is authoritative. PostgreSQL,
Valkey, Redis, SeaweedFS, NATS, etcd, Temporal, and other rows marked yes can support
small local experiments today. Modeling allocation with rows is not running a
VM; fetching chunks is not implementing userfaultfd; object storage is not a
POSIX mount; a local run is not a production scaling benchmark.

Local ideas and complete projects must use Docker Desktop/Compose and Go.
Firecracker and all KVM paths are excluded for now, as are separately managed
VMs and alternate VM runtimes. Containerd/nerdctl is allowed for a specific
lesson benefit with a configured local Linux engine. Provider architecture descriptions remain research,
not dependencies. Docker Engine API, container RPC/PTY tooling and broker
harnesses still need configuration before planning. See the catalog's local
baseline and the replacement ideas in [ideas.md](../lessons/ideas.md).
