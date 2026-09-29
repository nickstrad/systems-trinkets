# Lesson lifecycle

```text
ideas.md  ->  planned/<slug>.md  ->  completed/<slug>.md
 candidate     guide to work       base lesson finished
```

Lesson cores, runners and platform components are Go only. Deno/TypeScript
is reserved for k6 tooling. Use Docker for backing services and local container
harnesses; containerd/nerdctl is an allowed configured integration when useful.
KVM-based tools and separately managed Linux VMs are outside scope.

Keep potential projects in the aspect-based tables in [ideas.md](ideas.md),
following [AGENTS.md](AGENTS.md). Empty tables are intentional until ideas are
requested; existing plans remain valid without a matching backlog row.
Each entry has a stable slug,
a concrete question, the mechanism or comparison, required software, and
optional links to [platform research](../agentic-platforms/README.md). Ideas can
need unconfigured software; label the missing prerequisites. The readiness
source of truth is [software/software.md](../../software/software.md), where
`Configured: yes` means ready to use in a lesson. An idea's readiness label is
only a convenience, and must be checked again when planning.

Use `$create-lesson` to choose the eligible unplanned idea with the smallest
global `Order`, or name an idea explicitly. Order ranks platform-building
impact across all aspect tables; it does not override software readiness. It writes one complete working guide in `planned/`: source code
for the learner to type, terminal diagrams, run commands, DuckDB analysis,
correctness checks, and an embedded optional k6 build plan. It updates the
idea's status and link. It does not create the learner's executable files
unless asked. If no idea is eligible, it reports the missing prerequisites;
setting up software is a separate task.

Work through the plan with `glow docs/lessons/planned/<slug>.md`. Implement
its base source under `lessons/go/<slug>/`. A guide's
scratch validation proves that its example runs; it does not mean the learner
has completed the lesson.

When the base implementation, experiment, invariant checks, and DuckDB
analysis are finished, move the same guide to `completed/<slug>.md`. An AI
closing out the work should inspect the source and record the checks actually
run, or the learner's explicit completion report, with a date in the guide.
If verification cannot run, record that limitation instead of claiming a pass.
Unfinished implementation or known failing invariants remain in planned.
Update the matching idea row's status/link when present, the guide's own paths,
and other inbound links.
Do not overwrite a completed guide or create a second copy of the plan.

The HTTP/k6 follow-up is optional and may happen after this move. Its skill
reads the embedded build plan from completed first, then planned. Older lessons
without a guide need no invented historical plan.

To replenish the backlog, ask AI to update ideas.md separately. Use the platform
notes for mechanisms and questions, avoid duplicating existing lessons, and
keep software suggestions visibly distinct from configured prerequisites.
