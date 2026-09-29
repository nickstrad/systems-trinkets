# Lesson docs

Follow the repository's learning rules and the [lesson lifecycle](README.md).

## Local runtime boundary

Every lesson idea and complete project must run locally on macOS using Docker
Desktop/Compose and Go. Deno/TypeScript is reserved for k6 tooling; lesson
cores, runners, HTTP adapters and platform components are Go only. DuckDB
analysis and k6 load tooling stay in scope. Run Linux-specific harnesses
inside Docker. Exclude Firecracker and all KVM-dependent tools, nested
virtualization, separately managed Linux VMs, Apple container and alternate
VM runtimes from project paths for now. Containerd/nerdctl is allowed when
useful to a specific lesson, with its Linux engine running locally on the
Mac; Docker remains the backing-service default. Excluded tools may appear in
provider research only. Use ARM64-compatible images on Apple silicon. Keep
unimplemented Docker harnesses blocked until configured; portability alone
does not make a lesson ready.

## ideas.md: one entry per idea, grouped by platform aspect

`ideas.md` is a list of candidate lessons under `##` sections, one per core
aspect of an agentic platform (execution, lifecycle, ownership, capacity,
images, workspaces, container control, access, events, durable execution,
timers, observability) as identified in `docs/agentic-platforms/`. Organize by
the mechanism being learned rather than by vendor. Each section opens with a
sentence naming its abstractions and a Research line linking the notes that
motivate it. Read those notes before adding or revising a section or entry.

Each idea is a `###` heading whose text is its stable kebab-case slug,
followed by:

- the question in bold, plus an "(Added <date> ...)" note when the idea came
  from something other than the initial research pass;
- **Compare:** the behavior to build and its baseline or knob;
- **Invariant:** the observable correctness condition;
- **Measure:** the outcome or metric, with units;
- **Software:** the primary stack, then `Ready` or `Blocked:` naming the
  missing catalog rows. Only `Configured: yes` rows in
  `software/software.md` make an idea eligible for a plan; recheck the
  catalog, the label here is a convenience;
- **Options:** combinations of configured software that could implement the
  idea, separated by `·`, each ending with "(no setup)" or the catalog row
  that needs setup or adding;
- **Builds on:** (optional) an existing lesson or idea whose core this reuses;
- **Plan:** (optional) a link to the guide in `planned/` or `completed/`.
  Entries without a Plan line are unplanned ideas.

There is no ranking, order column, or priority bucket. Put an idea under its
main aspect once and cross-reference from other entries with its slug. Slugs
are stable identifiers. Ideas may need unconfigured software when the blocker
is explicit. When asked for structure only, leave sections with their
description and Research line and no entries. Keep the header short: rules
belong here, history belongs in git.

## projects.md: how ideas combine

`projects.md` holds potential platforms assembled from ideas. It is not a
ranked list and nothing in it is implemented or planned. Each project has:

- **Value** in two or three sentences with a deliberately small first scope;
- **Architecture:** a draw-visual embed from `diagrams/` plus one paragraph
  on the decision points;
- **Components:** a bullet per box naming its role, the idea slugs whose
  mechanism it applies, and the work chunk that wires it in; fixtures say
  "Fixture";
- **Lessons needed:** the minimum set for the first slice in build order
  (readiness and dependencies, not importance);
- **Follow-up lessons:** ideas that deepen the platform later;
- **Work to bring it together:** the integration chunks no lesson covers,
  each with what it connects and a "Done when" condition;
- **Exit criterion:** the end-to-end behavior that finishes the slice.

The file opens with a "Start here" recommendation and a "Shared
prerequisites" section listing configuration chunks (Go clients, harness
launchers) that blocked ideas point to. Finishing a prerequisite flips
catalog rows to `Configured: yes`; describing it does not.

Use the repo-local `.claude/skills/draw-visual/SKILL.md` for project
diagrams; keep Mermaid sources in `diagrams/`, regenerate embeds, and inspect
70/80-column previews. Explain every diagram component in one or two
sentences. Distinguish a runnable model from a deployed isolation/runtime
boundary; configured services alone do not supply the missing integration.
Never call glue implemented because a related lesson exists.

CLI/UI and client presentation are outside the learning scope and assumed to
be fully AI-generated. Their backend contracts, authenticated identities,
authorization and recovery remain in scope.

## Plans and completion

`$create-lesson` turns an existing eligible idea into a working guide in
`planned/`. Without an explicit selection it prefers a ready idea from the
"Lessons needed" list of the project marked "Start here". The learner writes
the base source unless explicitly requesting AI implementation. Move a plan to
`completed/` when the base work is finished, following the completion
evidence rules in README.md; k6 remains optional. Update the idea's Plan line
when present. An empty backlog does not invalidate existing plans or require
retrospective idea entries.
