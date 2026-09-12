-- compare: two runs side by side in one table. kind='check' rows compare an
-- invariant's outcome (ok / FAIL / ∅ when that run never checked it);
-- kind='latency' rows compare p99 per endpoint, in ms. Parameters: $run_a, $run_b.
WITH chk AS (
    SELECT 'check' AS kind,
           test || ' / ' || invariant_id AS "key",
           CASE bool_and(ok) FILTER (WHERE run_id = $run_a) WHEN true THEN 'ok' WHEN false THEN 'FAIL' END AS a,
           CASE bool_and(ok) FILTER (WHERE run_id = $run_b) WHEN true THEN 'ok' WHEN false THEN 'FAIL' END AS b
    FROM checks
    WHERE run_id IN ($run_a, $run_b)
    GROUP BY test, invariant_id
),
lat AS (
    SELECT 'latency' AS kind,
           method || ' ' || path_template || ' p99_ms' AS "key",
           printf('%.2f', quantile_cont(latency_ns, 0.99) FILTER (WHERE run_id = $run_a) / 1e6) AS a,
           printf('%.2f', quantile_cont(latency_ns, 0.99) FILTER (WHERE run_id = $run_b) / 1e6) AS b
    FROM samples_measured
    WHERE run_id IN ($run_a, $run_b)
    GROUP BY method, path_template
)
SELECT * FROM chk
UNION ALL
SELECT * FROM lat
ORDER BY kind, "key";
