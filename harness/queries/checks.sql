-- checks: the invariants that failed in one run, with the details the test
-- recorded. Empty output means every check passed. Parameters: $run_id.
SELECT test,
       invariant_id,
       message,
       details
FROM checks
WHERE run_id = $run_id
  AND NOT ok
ORDER BY test, invariant_id;
