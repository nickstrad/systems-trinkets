-- history: every recorded run of one pattern, oldest first, so language and
-- engine can be compared over time. p99_ms is the p99 of all measured
-- requests (setup handshake excluded); see latency.sql for per-endpoint
-- numbers. Parameters: $pattern.
WITH r AS (
    SELECT * FROM runs WHERE pattern = $pattern
),
t AS (
    SELECT run_id,
           count(*)                                AS tests,
           count(*) FILTER (WHERE status = 'fail') AS failed
    FROM tests GROUP BY run_id
),
c AS (
    SELECT run_id, count(*) FILTER (WHERE NOT ok) AS checks_failed
    FROM checks GROUP BY run_id
),
s AS (
    SELECT run_id, quantile_cont(latency_ns, 0.99) / 1e6 AS p99_ms
    FROM samples_measured
    GROUP BY run_id
)
SELECT r.run_id                     AS run_id,
       r.started_at                 AS started_at,
       r.language                   AS language,
       r.engine                     AS engine,
       r.label                      AS label,
       coalesce(t.tests, 0)         AS tests,
       coalesce(t.failed, 0)        AS failed,
       coalesce(c.checks_failed, 0) AS checks_failed,
       s.p99_ms                     AS p99_ms
FROM r
LEFT JOIN t ON t.run_id = r.run_id
LEFT JOIN c ON c.run_id = r.run_id
LEFT JOIN s ON s.run_id = r.run_id
ORDER BY r.started_at;
