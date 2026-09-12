// Package counter tests any SUT implementing suites/counter/CONTRACT.md
// against the invariants in INVARIANTS.md.
package counter

import (
	"net/http"
	"os"
	"slices"
	"testing"

	"systems-trinkets/harness/check"
	"systems-trinkets/harness/harness"
	"systems-trinkets/harness/load"
	"systems-trinkets/harness/results"
)

func TestMain(m *testing.M) { os.Exit(harness.Main(m)) }

type counter struct {
	Name  string `json:"name"`
	Value int64  `json:"value"`
}

type incrBody struct {
	Delta int `json:"delta"`
}

func name(n string) map[string]string { return map[string]string{"name": n} }

func get(h *harness.H, n string) int64 {
	h.T.Helper()
	var c counter
	h.Must(h.Get("/counters/{name}", name(n), &c))
	return c.Value
}

// TestContract is the sequential kind-1 test: INV-COUNTER-03 and -04.
func TestContract(t *testing.T) {
	h := harness.New(t)
	var c counter

	got := get(h, "fresh")
	check.Invariant(h, "INV-COUNTER-03", got == 0, "unknown counter reads 0", map[string]any{"got": got})

	h.Must(h.Post("/counters/{name}/incr", name("a"), nil, &c))
	check.Invariant(h, "INV-COUNTER-03", c.Name == "a" && c.Value == 1, "first incr (default delta) returns 1", map[string]any{"got": c})

	h.Must(h.Post("/counters/{name}/incr", name("a"), incrBody{5}, &c))
	check.Invariant(h, "INV-COUNTER-03", c.Value == 6, "incr delta=5 returns previous+5", map[string]any{"got": c.Value, "want": 6})
	got = get(h, "a")
	check.Invariant(h, "INV-COUNTER-03", got == 6, "GET equals last incr response", map[string]any{"got": got, "want": 6})
	got = get(h, "b")
	check.Invariant(h, "INV-COUNTER-05", got == 0, "incrementing a leaves b at 0", map[string]any{"got": got})

	for _, bad := range []any{incrBody{0}, incrBody{-3}, map[string]any{"delta": "x"}, []byte(`{"delta":`)} {
		resp, err := h.Post("/counters/{name}/incr", name("a"), bad, nil)
		if err != nil {
			t.Fatal(err)
		}
		check.Invariant(h, "INV-COUNTER-03", resp.Status == http.StatusBadRequest, "bad delta is 400", map[string]any{"body": string(resp.Body), "status": resp.Status, "sent": bad})
	}
	resp, err := h.Post("/counters/{name}/incr", name("no spaces!"), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	check.Invariant(h, "INV-COUNTER-03", resp.Status == http.StatusBadRequest, "bad name is 400", map[string]any{"status": resp.Status})
	got = get(h, "a")
	check.Invariant(h, "INV-COUNTER-03", got == 6, "rejected requests do not change the value", map[string]any{"got": got, "want": 6})

	// DELETE removes one name only.
	h.Must(h.Post("/counters/{name}/incr", name("b"), nil, nil))
	resp = h.Must(h.Delete("/counters/{name}", name("a")))
	check.Invariant(h, "INV-COUNTER-04", resp.Status == http.StatusNoContent, "DELETE returns 204", map[string]any{"status": resp.Status})
	got = get(h, "a")
	check.Invariant(h, "INV-COUNTER-04", got == 0, "deleted counter reads 0", map[string]any{"got": got})
	got = get(h, "b")
	check.Invariant(h, "INV-COUNTER-04", got == 1, "DELETE a leaves b unchanged", map[string]any{"got": got, "want": 1})
	resp = h.Must(h.Delete("/counters/{name}", name("never")))
	check.Invariant(h, "INV-COUNTER-04", resp.Status == http.StatusNoContent, "DELETE of unknown counter is 204", map[string]any{"status": resp.Status})

	// Reset wipes everything and the next incr starts from 0 again.
	h.Must(h.Post("/counters/{name}/incr", name("a"), incrBody{7}, nil))
	resp = h.Must(h.Post("/_reset", nil, nil, nil))
	check.Invariant(h, "INV-COUNTER-04", resp.Status == http.StatusNoContent, "reset returns 204", map[string]any{"status": resp.Status})
	a, b := get(h, "a"), get(h, "b")
	check.Invariant(h, "INV-COUNTER-04", a == 0 && b == 0, "all counters read 0 after reset", map[string]any{"a": a, "b": b})
	h.Must(h.Post("/counters/{name}/incr", name("a"), incrBody{3}, &c))
	check.Invariant(h, "INV-COUNTER-04", c.Value == 3, "incr after reset returns delta", map[string]any{"got": c.Value, "want": 3})
}

// incrRun is what incrLoad observed: every value returned by a 2xx incr and
// the sum of the deltas those 2xx increments carried. Latencies are not
// collected here — every request is already a sample row, and latency.sql
// derives the percentiles from those.
type incrRun struct {
	load.Result
	values  []int64
	applied int64
}

// incrLoad runs workers×perWorker increments on one name behind a barrier.
func incrLoad(h *harness.H, n string, workers, perWorker int, delta func(w *load.Worker) int) incrRun {
	args := name(n)
	values := make([][]int64, workers)
	applied := make([]int64, workers)
	r := load.Closed(h, workers, perWorker, func(w *load.Worker) error {
		var body any
		d := delta(w)
		if d != 1 {
			body = incrBody{d}
		}
		var c counter
		resp, err := h.Client.Post(w.Ctx, "/counters/{name}/incr", args, body, &c)
		if err != nil {
			return err
		}
		if resp.OK() {
			values[w.ID] = append(values[w.ID], c.Value)
			applied[w.ID] += int64(d)
		}
		return nil
	})
	var sum int64
	for _, a := range applied {
		sum += a
	}
	return incrRun{Result: r, values: slices.Concat(values...), applied: sum}
}

// TestIncrementConcurrent is the kind-2 test for INV-COUNTER-01 and -02:
// N workers increment one counter by 1 behind a barrier.
func TestIncrementConcurrent(t *testing.T) {
	h := harness.New(t)
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
	h := harness.New(t)
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
	h := harness.New(t)
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

// permutationGaps walks sorted values against 1..n in lockstep and reports
// the values that are missing and the ones that appear more than once.
func permutationGaps(sorted []int64, n int64) (missing, dup []int64) {
	want := int64(1)
	for i, v := range sorted {
		if i > 0 && sorted[i-1] == v {
			dup = append(dup, v)
			continue
		}
		for ; want < v && want <= n; want++ {
			missing = append(missing, want)
		}
		if want == v {
			want++
		}
	}
	for ; want <= n; want++ {
		missing = append(missing, want)
	}
	return missing, dup
}

func TestPermutationGaps(t *testing.T) {
	cases := []struct {
		sorted       []int64
		n            int64
		missing, dup []int64
	}{
		{[]int64{1, 2, 3, 4}, 4, nil, nil},
		{[]int64{1, 1, 3, 3, 3, 4}, 5, []int64{2, 5}, []int64{1, 3, 3}},
		{[]int64{2, 3, 4, 6}, 4, []int64{1}, nil},
		{nil, 2, []int64{1, 2}, nil},
	}
	for _, c := range cases {
		missing, dup := permutationGaps(c.sorted, c.n)
		if !slices.Equal(missing, c.missing) || !slices.Equal(dup, c.dup) {
			t.Errorf("permutationGaps(%v, %d) = %v, %v; want %v, %v", c.sorted, c.n, missing, dup, c.missing, c.dup)
		}
	}
}
