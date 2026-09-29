# Inngest: durable functions by replaying handler steps

**Research date: 2026-09-29.** Sources below include Inngest's product docs and engineering posts. They describe the public execution contract and selected Cloud internals.

## Request and execution path

An application defines a function with triggers and exposes its SDK handler to Inngest. An event starts a function run. The handler is invoked incrementally: it discovers the next step, executes it, returns its result to Inngest, and the platform persists the result. The SDK then runs the handler again with the event and prior state; successful step IDs are looked up and their values injected, so the callback does not run again. Each step executes as a separate HTTP request to the application's handler ([execution model](https://www.inngest.com/docs/learn/how-functions-are-executed)).

This differs from keeping one process and call stack alive while waiting. The user code is ordinary TypeScript, Python, or Go, and each durable boundary is explicit as `step.run`, sleep, wait-for-event, or another SDK operation. A callback's external side effect can still have an ambiguous outcome if the side effect succeeds but the step result was not durably acknowledged; design the operation to be idempotent ([error handling](https://www.inngest.com/docs/guides/error-handling)).

The execution contract says results are persisted in a managed function state store. The architecture and engineering sources below describe selected queue mechanisms; the public step API alone does not establish the underlying storage or replication design.

## Engineering writeups

Inngest's [Cloud architecture page](https://www.inngest.com/docs/architecture) gives a public control/data path: Event API validates and publishes to Pub/Sub; New Runs matches triggers and applies batching/debounce/singleton rules; ready runs enter queue shards; Executors consume queue items and call the customer's SDK over HTTP or Connect. It also says executor and customer code are separate, and describes separate scheduling, execution, and flow-control layers. The page currently describes AWS Ohio APIs/control plane and Inngest bare-metal in Ashburn for primary scheduling/execution. This is more precise than inferring a generic managed queue from the product overview, though it remains vendor-published architecture documentation.

The Inngest queue team's [September 2026 engineering post](https://www.inngest.com/blog/the-queue-is-the-easy-part) frames fairness and durability as the hard queue problems: a shared multi-tenant queue must avoid starving tenants and retain the promise to run jobs through machine failure. It explains their use of a sorted red-black-tree scheduling structure to pick the earliest due job without scanning all in-flight work. Their [2024 queue series, part I](https://www.inngest.com/blog/building-the-inngest-queue-pt-i-fairness-multi-tenancy) describes shared-nothing workers, per-function queues/virtual queues for concurrency, idempotent enqueue because event streams are at-least-once, and step results saved into run state before scheduling subsequent work. Those posts provide concrete design rationale and a useful lesson comparison: a basic FIFO or one global `SKIP LOCKED` queue does not by itself solve tenant fairness, low-contention scheduling, or durable multi-step execution.

The technical details above are the company's own description; they do not independently establish production performance or all failure guarantees. The docs and posts do not disclose a full persistence schema or replication protocol.

## Failure, flow control, and scale

- A failed `step.run` retries at that step boundary. Earlier successful step results are reused. The documented default is four retries after the initial attempt; retry count and non-retriable errors are configurable ([retries](https://www.inngest.com/docs/guides/error-handling)).
- A handler change can affect long-running executions, so Inngest documents function versioning as part of replay compatibility ([execution model](https://www.inngest.com/docs/learn/how-functions-are-executed)).
- Concurrency limits active step execution, not all open runs. A sleeping or waiting run releases capacity; limits can use keys for best-effort per-tenant fairness and be shared by scope ([concurrency](https://www.inngest.com/docs/guides/concurrency)). This is a useful distinction between queued work and occupied compute.
- Parallel steps can be individually retried, with their results rejoined when the handler resumes ([parallel steps](https://www.inngest.com/docs/guides/step-parallelism)).
- Inngest describes the platform as handling queueing, scaling, concurrency, throttling, and observability, while application code runs on the developer's compute ([overview](https://www.inngest.com/docs)). That is a product contract, not a disclosure of scheduler internals.

## Lesson fit for systems-trinkets

Inngest and its Dev Server are **not configured** in [`software/software.md`](../../software/software.md). A future lesson can reproduce the central idea with configured PostgreSQL, Valkey Streams, and DuckDB: accept an event, persist step results by `(run_id, step_id)`, deliberately fail after a completed side effect, retry, and measure which steps ran again. The invariant is: a successfully recorded step result is reused on retry, while a failed step may execute again; external effects must remain safe under that retry. Compare a naive whole-job retry, explicit step checkpointing, and a Valkey consumer-group claim/retry path. Record attempt counts, duplicate effects, queue wait, and completion latency. This models a documented contract; it should not claim to reproduce Inngest's hosted backend.

Configured pieces already support useful comparisons: PostgreSQL, Valkey Streams, NATS JetStream, and Temporal are marked yes. A near-term lesson could use Temporal for a contrasting durable model and compare its Worker/history replay with hand-built checkpoints. Inngest remains a future software row/setup idea, not a configured dependency.

## Evidence versus inference

**Documented:** handler reinvocation, separate HTTP step executions, memoized completed steps, configurable retries, and active-step concurrency semantics, cited above.

**Inference:** explicit step boundaries make prior progress independently reusable and allow waiting runs to consume little active compute. The complete storage failure protocol and scheduler policy beyond the published mechanisms remain unspecified; the architecture page discloses selected deployment regions, not a complete fleet topology.
