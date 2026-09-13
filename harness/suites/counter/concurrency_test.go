package counter

import (
	"slices"
	"systems-trinkets/harness/check"
	"systems-trinkets/harness/load"
	"systems-trinkets/harness/results"
	"systems-trinkets/harness/suitekit"
	"testing"
)

// TestIncrementConcurrent is the kind-2 test for INV-COUNTER-01 and -02:
// N workers increment one counter by 1 behind a barrier.
func TestIncrementConcurrent(t *testing.T) {
	h := suitekit.New(t)
	const workers, perWorker = 32, 200
	const want = int64(workers * perWorker)

	r := incrLoad(h, "hot", workers, perWorker, func(*load.Worker) int { return 1 })
	if r.Elapsed > 0 {
		check.Metric(h, "incr_throughput", float64(r.Total)/r.Elapsed.Seconds(), "req/s",
			map[string]any{"workers": workers, "path": "/counters/{name}/incr"})
	}
	if !r.Conclusive(h) {
		return
	}
	got := get(h, "hot")
	check.Invariant(h, "INV-COUNTER-01", got == int64(r.OK2xx),
		"final value equals number of 2xx increments",
		map[string]any{"got": got, "want": r.OK2xx, "sent": r.Total, "non2xx": r.Non2xx, "lost": int64(r.OK2xx) - got})

	slices.Sort(r.values)
	missing, dup := permutationGaps(r.values, want)
	check.Invariant(h, "INV-COUNTER-02", len(missing) == 0 && len(dup) == 0 && int64(len(r.values)) == want,
		"returned values are a permutation of 1..N",
		map[string]any{"n": want, "returned": len(r.values), "missing": results.Head(missing), "duplicates": results.Head(dup)})
}

// TestIncrementDeltaConcurrent is INV-COUNTER-01 with mixed deltas: the final
// value must equal the sum of the deltas of every 2xx increment.
func TestIncrementDeltaConcurrent(t *testing.T) {
	h := suitekit.New(t)
	const workers, perWorker = 16, 100
	deltas := []int{1, 2, 3, 5, 8}
	r := incrLoad(h, "sum", workers, perWorker, func(w *load.Worker) int { return deltas[(w.ID+w.Iter)%len(deltas)] })
	if !r.Conclusive(h) {
		return
	}
	got := get(h, "sum")
	check.Invariant(h, "INV-COUNTER-01", got == r.applied,
		"final value equals sum of deltas of all 2xx increments",
		map[string]any{"got": got, "want": r.applied, "non2xx": r.Non2xx, "lost": r.applied - got})
}

// TestIsolationConcurrent is INV-COUNTER-05: several names hammered at once,
// each must end at exactly its own count.
func TestIsolationConcurrent(t *testing.T) {
	h := suitekit.New(t)
	names := []string{"iso-a", "iso-b", "iso-c", "iso-d"}
	const workers, perWorker = 16, 100 // 4 workers per name
	args := make([]map[string]string, len(names))
	for i, n := range names {
		args[i] = name(n)
	}
	r := load.Closed(h, workers, perWorker, func(w *load.Worker) error {
		_, err := h.Client.Post(w.Ctx, "/counters/{name}/incr", args[w.ID%len(names)], nil, nil)
		return err
	})
	if !r.Conclusive(h) {
		return
	}
	perName := int64(workers / len(names) * perWorker)
	finals := map[string]int64{}
	ok := r.Non2xx == 0
	for _, n := range names {
		finals[n] = get(h, n)
		ok = ok && finals[n] == perName
	}
	check.Invariant(h, "INV-COUNTER-05", ok, "each name ends at exactly its own increment count",
		map[string]any{"got": finals, "want_each": perName, "non2xx": r.Non2xx})
	got := get(h, "iso-untouched")
	check.Invariant(h, "INV-COUNTER-05", got == 0, "untouched name stays 0", map[string]any{"got": got})
}
