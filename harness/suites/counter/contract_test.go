package counter

import (
	"net/http"
	"systems-trinkets/harness/check"
	"systems-trinkets/harness/suitekit"
	"testing"
)

// TestContract is the sequential kind-1 test: INV-COUNTER-03 and -04.
func TestContract(t *testing.T) {
	h := suitekit.New(t)
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
