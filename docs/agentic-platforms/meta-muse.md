# Meta Muse: a focused agent computer

Research checked 2026-09-29. Start with Meta's September 8 launch account,
[How We Built Safety Into Muse](https://research.meta.ai/blog/security-and-safety-for-ai-agents-our-approach-with-muse).
The following is a design study; no Muse deployment or security claim was tested here.

## Published architecture

| Boundary | Meta's description |
|---|---|
| Runtime | Per-user Linux VM; agent/tools in a `systemd-nspawn` cell, remapped root, separate filesystem, filtered syscalls/capabilities. |
| Privileged services | Outside the cell: safety classifiers, connector workers, credential service `authd`, Sentinel, PostgreSQL, constrained inference/telemetry proxies. |
| IPC | Unix sockets authenticate peers with `SO_PEERCRED` and ACLs. |
| Authority | Sentinel governs connectors and egress; approval travels directly between client and Sentinel and is scope-bound. |
| Credentials | Surrogates replace secrets inside runtime requests; authorized egress inserts real credentials. |
| Network | Forward proxy plus Linux/eBPF controls; destination checks include resolved IPs. Taint tracking removes automatic allowance after sensitive reads or uncertain attribution. |
| Browser | Separate broker; restricted interface, without arbitrary page JavaScript; agent pauses during credential filling/user takeover. |

These are vendor-described boundaries, not independently established guarantees.
[Architecture source](https://research.meta.ai/blog/security-and-safety-for-ai-agents-our-approach-with-muse).

## Additional primary reading and limits

Meta's [launch announcement](https://about.fb.com/news/2026/09/introducing-muse-personal-ai-agent/)
frames the dedicated computer as the agent's persistent home, where user data
and connected credentials live. It distinguishes the launch Secure VM from a
planned Confidential VM intended to exclude provider access. Do not assume
that ordinary VM isolation provides confidentiality from its operator.

The designers' [How We Designed Muse](https://introducing.muse.ai/) adds useful
product constraints: scheduled/event-driven work continues while the app is
closed, memory can be edited, and structured approval controls are deliberately
separate from conversation. For our projects, that motivates durable background
state and an approval API, even when the client is entirely AI-generated.

The requested security article was the most detailed first-party architecture
account found in this pass. The additional first-party pages add product
semantics, not reproducible deployment manifests. No public full runtime source,
VMM choice, fleet scheduler, measured VM density, or kernel-taint implementation
was established by this search. Third-party reimplementations and the older
`facebookresearch/MUSE` word-embedding repository are not Muse runtime evidence.
Do not infer Firecracker merely because other providers use it.

## Why a focused environment is useful here

Our interpretation: specialize the **authority surface**, not just the installed
packages. A useful learner-built agent computer can still compile programs and
edit its workspace, while exposing only a few explicitly authorized ways to
change the outside world. A minimal image by itself does not establish that
property. The object to design is the set of interfaces, identities, mounts,
network paths, and durable records that remain available after untrusted code
is running.

For a local design, treat the environment as two separately owned parts:

- **Job workspace:** replaceable code, input/output files, a bounded process
  lifetime, and no permission to change policy or retrieve long-lived secrets.
- **Supervisor:** owns authorization records, action dispatch, broker identity,
  credential access, and the audit trail. The workspace submits requests; it
  cannot manufacture a trusted decision by modifying its own files.

This is our proposed decomposition, not a copy of Meta's source. The decisive
experiment is to replace the workspace code with a deliberately noncompliant
fixture and check whether the external boundaries still hold. A cooperating
agent is insufficient evidence.

## Linux building blocks worth understanding

The upstream [nspawn settings manual](https://www.freedesktop.org/software/systemd/man/systemd.nspawn.html)
documents private user mappings, capability removal, syscall filters, and
privileged configuration placement. In particular, where a settings file comes
from affects whether access-expanding settings are honored. That suggests a
separate teaching question about who can modify the runtime policy, not just
what the policy says. Confirm options against the installed version before
writing a runnable plan; defaults are not a complete confinement profile.

The Linux [Unix socket manual](https://man7.org/linux/man-pages/man7/unix.7.html)
specifies that `SO_PEERCRED` returns peer credentials captured at connection
establishment. Socket pathname permissions and a peer ACL solve different
parts of access control. A JSON field claiming a worker identity is not a
substitute for the kernel-supplied identity. A long-lived connection also needs
a deliberate revocation policy; credentials at connect time are not a fresh
application authorization check for every operation.

## Lessons added or connected

These are original local experiment proposals, not assertions about undisclosed
Muse behavior. See their complete comparison, invariant, metric, readiness, and
global rank in [ideas.md](../lessons/ideas.md).

| Idea | What it adds beyond the existing backlog |
|---|---|
| `focused-runtime-cell` | A restricted Docker worker with a deliberately small interface to its supervisor; goes beyond observing namespace visibility. |
| `peer-authenticated-tool-broker` | Trusted caller identity and per-worker method authorization, rather than merely accepting a connector name from a request. |
| `scoped-approval-consumption` | Atomic use of an expiring, revocable, request-bound grant; complements durable signals without conflating a notification with permission. |
| `surrogate-credential-broker` | A constrained handle usable only for a matching test action; separates executing an action from handing a worker its credential. |
| `enforced-egress-path` | Whether direct connections can bypass the mediation point; complements the Go broker destination-policy lesson. |
| `data-taint-policy-model` | A small explicit information-flow state machine, including unknown provenance; does not claim kernel-level tracking. |

Existing lifecycle, fencing, idempotency, artifact publication, timers, and audit
lessons supply the surrounding platform mechanics. The
[potential projects](../lessons/ideas.md#potential-projects) section composes
them only after these additions.

## Software readiness and deliberately omitted scope

PostgreSQL, Go, and DuckDB support the approval and
policy models now. Use deterministic fixture actions and synthetic tokens;
no live account integration or LLM API is required to learn the invariants.

The local adaptation uses a restricted Docker worker, a Go peer-identity broker
and network-disabled execution with Unix-socket action requests. It does not
require nspawn, host eBPF, a separately managed Linux VM or KVM. The Docker
API, isolation and broker harnesses are configured in `internal/lab/docker`
(verified on Linux; not yet run on Docker Desktop). A harness passing its own
checks does not prove a project's boundaries; the full project must pass
the denied-operation and restart probes on the actual Docker deployment.

Classifier training, browser automation, payment integrations, confidential
computing/attestation, and a production taint tracker are outside the initial
projects. A future browser project would need its own implementation research
and configured tooling. No toy policy model proves prompt-injection immunity.
