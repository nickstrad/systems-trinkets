// Package hx is the HTTP client every test request goes through, so that every
// request becomes a results.SampleRow.
package hx

import (
	"context"
	"sync/atomic"
)

type ctxKey int

const (
	workerKey ctxKey = iota
	phaseKey
	counterKey
)

// Counter tallies requests made with a context that carries it (see
// WithCounter). Client.Do bumps it once per request: Total always; then
// exactly one of OK2xx, Non2xx (any HTTP response outside 200–299) or Errors
// (no HTTP response: transport error, timeout, context cancelled).
// The load package installs one per Closed/ClosedFor call to build its Result.
type Counter struct {
	Total, OK2xx, Non2xx, Errors atomic.Int64
}

// WithCounter makes requests made with ctx tally into c.
func WithCounter(ctx context.Context, c *Counter) context.Context {
	return context.WithValue(ctx, counterKey, c)
}

// CounterFrom returns the Counter set by WithCounter, or nil.
func CounterFrom(ctx context.Context) *Counter {
	c, _ := ctx.Value(counterKey).(*Counter)
	return c
}

// WithWorker labels requests made with ctx as coming from worker id.
// The load package sets this; sequential test code uses worker 0.
func WithWorker(ctx context.Context, id int) context.Context {
	return context.WithValue(ctx, workerKey, id)
}

// WithPhase labels requests made with ctx with a phase name ("warmup",
// "main", ...). Empty by default.
func WithPhase(ctx context.Context, name string) context.Context {
	return context.WithValue(ctx, phaseKey, name)
}

// WorkerFrom returns the worker id set by WithWorker, or 0.
func WorkerFrom(ctx context.Context) int {
	id, _ := ctx.Value(workerKey).(int)
	return id
}

// PhaseFrom returns the phase set by WithPhase, or "".
func PhaseFrom(ctx context.Context) string {
	name, _ := ctx.Value(phaseKey).(string)
	return name
}
