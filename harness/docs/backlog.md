# Harness backlog

The counter worked example and toolkit are implemented. See
[architecture.md](architecture.md) for current behavior and
[work-log.md](work-log.md) for completed work and review evidence.
These items are conditional, not promised current features.

| Item | Trigger | Intended direction |
|------|---------|--------------------|
| Open-loop load (`load.Open`) | A rate-limiter or another arrival-rate-sensitive pattern | Fixed-rate scheduler; consider a small ticker or a pacing library when needed |
| Persistent DuckDB results | Partial results must survive panic, timeout, or abrupt process death | On-disk database under `artifacts/runs/<run_id>/`; define flush/recovery guarantees |
| `[expect]` performance thresholds | Explicit performance requirements | Post-run checks over measured samples; currently parsed but unenforced |
| Linearizability/history checker | A pattern claims a guarantee needing history analysis | Add a history package and evaluate Porcupine |
| Metadata CLI hand-off | Both tools settle enough to define the integration | Link run IDs to `trinkets attempt`; original Q7 remains deferred |
| Further pattern suites | The user reaches the pattern | Follow [suite-authoring.md](suite-authoring.md), beginning with the interview |

Network fault proxies, automatic container lifecycle, and property-based
sequence generation are not current toolkit milestones. Reconsider them
only with a concrete pattern requirement.
