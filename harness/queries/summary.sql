-- summary: one row per test in one run — outcome, duration, how much it
-- recorded. Parameters: $run_id.
WITH t AS (
    SELECT * FROM tests WHERE run_id = $run_id
),
c AS (
    SELECT test,
           count(*)                        AS checks,
           count(*) FILTER (WHERE NOT ok)  AS checks_failed
    FROM checks
    WHERE run_id = $run_id
    GROUP BY test
),
s AS (
    SELECT test, count(*) AS samples
    FROM samples
    WHERE run_id = $run_id
    GROUP BY test
)
SELECT t.test                       AS test,
       t.status                     AS status,
       t.duration_ns / 1e6          AS duration_ms,
       coalesce(c.checks, 0)        AS checks,
       coalesce(c.checks_failed, 0) AS checks_failed,
       coalesce(s.samples, 0)       AS samples,
       t.error                      AS error
FROM t
LEFT JOIN c ON c.test = t.test
LEFT JOIN s ON s.test = t.test
ORDER BY t.test;
