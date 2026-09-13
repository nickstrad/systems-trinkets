package counter

import "context"

// Store is the counter table behind the handler. Incr is the primitive whose
// atomicity INV-COUNTER-01/02 test; everything else is bookkeeping.
type Store interface {
	// Incr adds delta to name and returns the post-increment value, atomically.
	Incr(ctx context.Context, name string, delta int64) (int64, error)
	// Get returns the current value of name, 0 when absent.
	Get(ctx context.Context, name string) (int64, error)
	// Del removes name; it reads 0 afterwards.
	Del(ctx context.Context, name string) error
	// Reset removes every counter (POST /_reset).
	Reset(ctx context.Context) error
	// Ping reports whether the store is reachable (GET /healthz).
	Ping(ctx context.Context) error
	Close() error
}
