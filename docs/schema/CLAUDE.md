# Schema reference maintenance

Keep `index.html` and `erd.html` current whenever `db.go` schema SQL or migrations, store-layer semantics, foreign-key behavior, indexes, or CLI deletion behavior changes.

Before updating either page, verify the change against the implemented SQL and the relevant store/command code. Keep the pages as a concise, offline-readable two-page reference: `index.html` is the complete column reference and `erd.html` is the relationship diagram and constraint summary.
