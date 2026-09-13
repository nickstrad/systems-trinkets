package counter

import (
	"slices"
	"systems-trinkets/harness/load"
	"systems-trinkets/harness/suitekit"
	"testing"
)

type counter struct {
	Name  string `json:"name"`
	Value int64  `json:"value"`
}

type incrBody struct {
	Delta int `json:"delta"`
}

func name(n string) map[string]string { return map[string]string{"name": n} }

func get(h *suitekit.H, n string) int64 {
	h.T.Helper()
	var c counter
	h.Must(h.Get("/counters/{name}", name(n), &c))
	return c.Value
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
func incrLoad(h *suitekit.H, n string, workers, perWorker int, delta func(w *load.Worker) int) incrRun {
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
