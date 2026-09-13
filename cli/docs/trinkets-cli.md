# trinkets — CLI and schema

`trinkets` is a small Go + SQLite CLI for keeping notes on the patterns in
[systems-patterns.md](../../docs/systems-patterns.md): what each pattern is, how each
storage engine could build it, and what happened when I tried.

From `cli/`:

```sh
go build -o bin/trinkets .
./bin/trinkets seed
./bin/trinkets matrix
```

The database is `./trinkets.db` unless `--db PATH` or `$TRINKETS_DB` says
otherwise. It is created on first use, so there is no init step.

## Schema

Four tables. The first two are the axes, the third is the writing, the fourth is
the doing.

```
patterns ──┬─< approaches >─┬── engines
           │                │
           └─< attempts >───┘
                  │
                  └─ optional link to the approach being tried
```

| table | columns |
| --- | --- |
| `patterns` | `slug` UNIQUE, `name`, `family`, `explanation`, `use_cases`, `invariants` JSON, `readings` JSON, `notes`, nullable `curriculum_order` |
| `engines` | `slug` UNIQUE, `name`, `notes` |
| `approaches` | `pattern_id`, `engine_id`, `title`, `primitives`, `writeup` |
| `attempts` | `pattern_id`, `engine_id`, `approach_id?`, `title`, `status`, `lessons` |

Every row also has `id`, `created_at`, and `updated_at`.

`family` groups patterns for listing (`queueing`, `coordination`,
`rate-limiting`, …). `explanation` says what the pattern is in plain words, a
few sentences at most. `use_cases` names the problems it solves and where it
shows up. `invariants` is question 1 of the guide: what must the system
preserve? A pattern usually has more than one, so it is a JSON array of
strings. `readings` is a JSON array of `{"title", "url"}` objects. Both columns
are CHECK-constrained to be JSON arrays, so SQLite's JSON functions apply
directly:

```sql
SELECT p.slug, i.value FROM patterns p, json_each(p.invariants) i;
SELECT slug FROM patterns WHERE json_array_length(readings) = 0;
```

On the command line the lists are repeatable flags: `--invariant "..."` and
`--reading "Title - URL"`. Passing either on `edit` replaces the whole list. `primitives` is question 2: which primitive actually provides that
guarantee — it is the cell the `matrix` view prints. `status` on an attempt is
CHECK-enforced: `planned`, `in_progress`, `done`, `abandoned`.

Several approaches per (pattern, engine) is the normal case — a naive claim and
a lease-based claim on the same SQLite queue are two rows, not an edit. The
`title` is UNIQUE per pair.

The link from an attempt to an approach is a composite foreign key:

```sql
FOREIGN KEY (approach_id, pattern_id, engine_id)
  REFERENCES approaches (id, pattern_id, engine_id)
```

so an attempt can only cite an approach written for the same pattern and engine.
SQLite skips the check when `approach_id` is NULL, which is what makes the link
optional at no cost.

`curriculum_order` is a nullable positive integer. Seeded curriculum rows use
positions 1 through 29; user-created patterns may remain NULL. Its partial
unique index permits many NULL values but prevents two non-NULL rows from
sharing a position. Pattern lists and the matrix sort numbered rows first in
ascending order, then NULL-order rows by family and slug. JSON and tabular
output include the order (JSON uses `null` and tables use `-` when unset).

## Commands

Every resource takes the same five verbs. Patterns and engines are addressed by
slug, approaches and attempts by id. `--json` works on every `list` and `show`.
On `edit`, only the flags you pass are written.

```
trinkets pattern   list|show|add|edit|rm
trinkets engine    list|show|add|edit|rm
trinkets approach  list|show|add|edit|rm
trinkets attempt   list|show|add|edit|rm
trinkets matrix
trinkets seed [--update [--prune [--dry-run]]]
```

### Reading

```sh
trinkets pattern list --family queueing
trinkets pattern show fifo-queue          # + its approaches and attempts
trinkets pattern add --slug local-job --name "Local job" --family queueing
trinkets pattern edit local-job --curriculum-order 30
trinkets pattern edit local-job --clear-curriculum-order
trinkets approach list --engine postgres
trinkets attempt list --status in_progress
trinkets matrix                           # the core map, as stored
trinkets matrix --counts                  # coverage instead of primitives
```

### Writing

```sh
trinkets approach add --pattern fifo-queue --engine postgres \
  --title "serialized FIFO claim" \
  --primitives "Lock queue_head, then claim the minimum eligible sequence in one transaction" \
  --writeup "The shared queue-head lock serializes claim decisions; completion order may differ."

trinkets attempt add --pattern fifo-queue --engine postgres \
  --title "go worker pool" --approach 15 --status in_progress

trinkets attempt edit 1 --status done \
  --lessons "Fell over at 32 workers; the claim query was the bottleneck, not the lock."
```

Everything lives in this repo, so an attempt records no path or ref — find the
code by the pattern slug. Prose goes in as an argument; a heredoc works for
anything long:

```sh
trinkets approach edit 15 --writeup "$(cat <<'EOF'
...several paragraphs...
EOF
)"
```

### Deleting

Removing a pattern or engine that still has approaches or attempts needs
`--force`, and reports what went with it. Removing an approach unlinks any
attempts that cite it and keeps them: the record of having tried something
outlives the writeup of how.

### Seeding

`trinkets seed` loads the three engines, the 29 patterns, and one `core map
sketch` approach per pattern/engine cell from `cli/seed_catalog.go`. It inserts
missing rows and leaves existing seed-owned fields alone. `--update` refreshes
seed-owned fields and primitives while preserving pattern notes, authored writeups,
attempts, IDs, and creation timestamps. Neither mode removes or renames rows.

`trinkets seed --update --prune` is the explicit curriculum reconciliation. It
applies the seven named legacy-to-current renames, inserts new exercises,
refreshes canonical content, removes the nine named retired slugs, and verifies
29 patterns, three engines, and 87 core approaches in one content transaction.
The reconciliation retains authored approaches on unaffected patterns and
lists their IDs in `preserved_authored_approaches`; it blocks on unknown
patterns/engines, a same-slug rename collision, attempts or authored content on
affected rows, or non-core approaches on affected rows. Preserved non-core
work on an unaffected pattern is retained and reported; affected authored work
requires a deliberate resolution.

Add `--dry-run` to that exact combination for a deterministic JSON preview:

```sh
trinkets --db /path/to/copy.db seed --update --prune --dry-run
```

Dry-run copies the source through SQLite's read-only backup API, runs schema
migrations and planning only on the temporary copy, and leaves the source
bytes and rows unchanged. Insert actions have `id: 0` because planning has not
allocated a row yet; refresh, rename, and removal actions show their existing
IDs. Apply rebuilds the report under its writer transaction, so a previous
preview is never trusted after the source changes. A failed apply rolls back
all content changes; the schema migration may remain complete.

`--prune` requires `--update`, and `--dry-run` is accepted only with both. A
plain `seed` or `seed --update` cannot resurrect a retired standalone slug
because those definitions are no longer in the executable catalog.

### Curriculum order flags

On `pattern add` and `pattern edit`, use `--curriculum-order N` to set a
positive position or `--clear-curriculum-order` to set it to NULL. The flags
are mutually exclusive. Omitting both on edit preserves the current value; an
edit with no fields still fails. CLI validation and the SQLite check/index both
enforce positive values and uniqueness among non-NULL orders.

## v1 scope

Deliberately absent: tags, search, an `$EDITOR` round-trip, links between
patterns, and export back to Markdown. The schema uses idempotent startup
migrations, including nullable curriculum order and its partial unique index;
catalog reconciliation remains an explicit seed mode.
