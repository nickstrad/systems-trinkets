-- latency: per test and endpoint, how the SUT answered. Setup traffic (the
-- /_reset + /healthz handshake) is excluded. Parameters: $run_id.
SELECT test,
       method,
       path_template,
       count(*)                                          AS n,
       count(*) FILTER (WHERE status BETWEEN 200 AND 299) AS ok_2xx,
       count(*) FILTER (WHERE status >= 300
                           OR (status > 0 AND status < 200)) AS non_2xx,
       count(*) FILTER (WHERE err <> '')                  AS errors,
       quantile_cont(latency_ns, 0.50) / 1e6              AS p50_ms,
       quantile_cont(latency_ns, 0.95) / 1e6              AS p95_ms,
       quantile_cont(latency_ns, 0.99) / 1e6              AS p99_ms,
       max(latency_ns) / 1e6                              AS max_ms
FROM samples_measured
WHERE run_id = $run_id
GROUP BY test, method, path_template
ORDER BY test, path_template;
