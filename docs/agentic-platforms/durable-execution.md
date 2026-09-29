# Durable execution: Temporal and Restate

**Research date: 2026-09-29.** This is a comparison of the public execution models. Temporal's and Restate's docs describe their interfaces and behavior; they do not imply identical internals or exactly-once external side effects.

## Temporal: workflow history replayed by Workers

The client sends workflow start, signal, query, or result requests over gRPC to the Temporal Service. The Service persists workflow history and state, places Workflow and Activity Tasks on Task Queues, and Workers poll those queues. Workflow code runs in Workers, not inside the Service ([architecture](https://docs.temporal.io/encyclopedia/architecture/how-temporal-works)).

When a Worker receives a Workflow Task, its SDK replays workflow code from the beginning against Event History. Previously completed Activities and timers resolve from history rather than running again; newly generated commands are sent to the Service and recorded as events. Activity code performs I/O and reports success or failure. On retryable Activity failure, the Service can schedule another Activity attempt ([Tasks](https://docs.temporal.io/tasks)).

Workflow code must be deterministic so that replay generates commands compatible with the recorded history ([workflow definition](https://docs.temporal.io/workflow-definition)). Activity side effects may be retried; make them idempotent or use an explicit deduplication key. Temporal documents an “effectively once” experience at the workflow API, while Activity attempts can occur more than once ([Tasks](https://docs.temporal.io/tasks)). Workflow Task failure and Workflow Execution failure have different retry behavior; a configured Workflow Retry Policy creates a new Run with its own history.

Scaling means deploying enough Workers polling the relevant queues and scaling the Service/persistence layer for recorded history and task traffic. Temporal's public docs explain this separation, but a self-hosted single-node dev server is not representative of production cluster capacity or fault tolerance.

## Restate: journaled handlers, objects, and workflows

Restate offers three service shapes: stateless Basic Services, keyed Virtual Objects with state, and Workflows keyed by workflow ID. The docs describe durable execution in each; Virtual Objects serialize writers per key while allowing concurrency across keys and shared readers, and Workflow `run` executes once per ID while signals/queries can interact concurrently ([service types](https://docs.restate.dev/foundations/services)).

Handlers use Restate context operations for durable calls/state, promises, timers, and other actions. The service is deployed as an endpoint and registered with Restate; immutable deployment versions allow new requests to route to new code while existing requests continue on their original deployment ([service types](https://docs.restate.dev/foundations/services)). Restate's Kafka guide says its server durably persists the message, pushes the invocation to a handler, and retries failed requests; for Virtual Objects the Kafka key becomes the object key, effectively producing a queue per key ([Kafka quickstart](https://docs.restate.dev/guides/kafka-quickstart)).

Restate's agent-orchestration docs state that routing decisions, agent calls, and results are recorded in the journal and that recovery skips already persisted decisions ([multi-agent pattern](https://docs.restate.dev/ai/patterns/multi-agent)). The precise on-disk layout, replication behavior, and hosted scheduler internals should not be inferred from the programming model. As with Temporal, an external side effect needs idempotency around crash/ack ambiguity.

## Useful comparison for a lesson

Temporal is **configured** in [`software/software.md`](../../software/software.md): local dev server, Go SDK, gRPC port 7233 and UI 8233; the dev server stores history in its own SQLite volume, not repo PostgreSQL. Restate is **not configured** and is listed as a future compose/reference service. That makes Temporal the runnable platform experiment today and Restate a source-grounded design comparison.

The runnable lesson should compare only PostgreSQL checkpoints with the configured Temporal dev server and Go SDK. Keep the job concrete: reserve a resource, wait on a short timer, then publish a result. Store business state in PostgreSQL; Temporal's local dev server keeps its own workflow history in its SQLite volume. Invariant: business state advances monotonically, and retries cannot duplicate a reservation or publication. Inject a worker restart after an Activity's external write but before completion is acknowledged. Measure attempts, duplicate effects, recovery time, history growth, and queue wait. Keep the timer short and report that the local dev server is not a production cluster.

NATS JetStream or Valkey Streams can be a separate follow-up alternative for comparing queue redelivery and explicit checkpoints; both are configured, but they are outside the core Temporal-versus-PostgreSQL experiment. Restate remains a source-grounded comparison only until configured.

## Evidence versus inference

**Documented:** Temporal client/service/worker boundary, task polling, event history replay, deterministic workflows, and activity retries; Restate service types, keyed write concurrency, durable Kafka ingestion and retry, with direct links above.

**Inference:** Temporal foregrounds replayable workflow code plus separately executed Activities; Restate foregrounds durable handler calls and keyed virtual entities. These are useful teaching contrasts, not claims that one is strictly more durable or that either provides exactly-once effects in arbitrary external systems.
