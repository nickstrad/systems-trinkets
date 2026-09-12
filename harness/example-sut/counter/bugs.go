package counter

import (
	"context"
	"runtime"
	"sync"
	"time"
)

// Bugs are Store decorators, one per invariant they break, so the suite can
// prove it catches each one on every engine. They are engine-agnostic: the
// bug is always in how the application uses the store, never in the store.

// LostUpdate makes Incr a non-atomic read-modify-write (Get, yield, Set) so
// concurrent increments clobber each other. Breaks INV-COUNTER-01, -02 and
// -05; sequential use still looks correct.
func LostUpdate(s Store) Store { return lostUpdate{s} }

type lostUpdate struct{ Store }

func (l lostUpdate) Incr(ctx context.Context, name string, delta int64) (int64, error) {
	old, err := l.Store.Get(ctx, name)
	if err != nil {
		return 0, err
	}
	// Widen the window between read and write so the race is observable
	// even on a fast in-memory store.
	runtime.Gosched()
	time.Sleep(50 * time.Microsecond)
	v := old + delta
	if err := l.Store.Set(ctx, name, v); err != nil {
		return 0, err
	}
	return v, nil
}

// DropReset makes Reset a no-op that still reports success. Breaks
// INV-COUNTER-04 (and, as a consequence, every test that relies on a clean
// SUT).
func DropReset(s Store) Store { return dropReset{s} }

type dropReset struct{ Store }

func (dropReset) Reset(context.Context) error { return nil }

// WriteBehind acknowledges increments from an in-memory shadow and writes
// the shadow to the underlying store every flush interval. Reads come from
// the shadow too, so every invariant holds while the process is alive; a
// kill loses the unflushed increments that were already acknowledged.
// Breaks only INV-COUNTER-06 (crash), and only on a persistent engine.
func WriteBehind(s Store, flush time.Duration) Store {
	w := &writeBehind{Store: s, shadow: map[string]int64{}, dirty: map[string]bool{}, stop: make(chan struct{}), done: make(chan struct{})}
	go w.loop(flush)
	return w
}

type writeBehind struct {
	Store
	mu     sync.Mutex
	shadow map[string]int64 // values the client has been told; wins over the store
	dirty  map[string]bool  // shadow entries not yet written through
	stop   chan struct{}
	done   chan struct{}
	once   sync.Once // Close is safe to call twice (a deferred Close after a signal-driven one)
}

func (w *writeBehind) loop(flush time.Duration) {
	defer close(w.done)
	t := time.NewTicker(flush)
	defer t.Stop()
	for {
		select {
		case <-t.C:
			w.flushNow()
		case <-w.stop:
			return
		}
	}
}

// flushNow writes every dirty shadow value to the store. Errors are dropped:
// the bug's whole point is that the client was already told "ok".
func (w *writeBehind) flushNow() {
	w.mu.Lock()
	defer w.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	for name := range w.dirty {
		_ = w.Store.Set(ctx, name, w.shadow[name])
	}
	clear(w.dirty)
}

func (w *writeBehind) Incr(ctx context.Context, name string, delta int64) (int64, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	v, ok := w.shadow[name]
	if !ok {
		var err error
		if v, err = w.Store.Get(ctx, name); err != nil {
			return 0, err
		}
	}
	v += delta
	w.shadow[name], w.dirty[name] = v, true
	return v, nil
}

func (w *writeBehind) Get(ctx context.Context, name string) (int64, error) {
	w.mu.Lock()
	v, ok := w.shadow[name]
	w.mu.Unlock()
	if ok {
		return v, nil
	}
	return w.Store.Get(ctx, name)
}

func (w *writeBehind) Set(ctx context.Context, name string, v int64) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.shadow[name], w.dirty[name] = v, true
	return nil
}

func (w *writeBehind) Del(ctx context.Context, name string) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	delete(w.shadow, name)
	delete(w.dirty, name)
	return w.Store.Del(ctx, name)
}

func (w *writeBehind) Reset(ctx context.Context) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	clear(w.shadow)
	clear(w.dirty)
	return w.Store.Reset(ctx)
}

// Close stops the flusher, writes what is pending (a clean shutdown loses
// nothing — only a kill does) and closes the store.
func (w *writeBehind) Close() (err error) {
	w.once.Do(func() {
		close(w.stop)
		<-w.done
		w.flushNow()
		err = w.Store.Close()
	})
	return err
}

// Slow adds d of latency to every store call. Breaks nothing the suite
// asserts today; it gives [expect] thresholds something to fail on.
func Slow(s Store, d time.Duration) Store { return slow{s, d} }

type slow struct {
	Store
	d time.Duration
}

func (s slow) Incr(ctx context.Context, name string, delta int64) (int64, error) {
	time.Sleep(s.d)
	return s.Store.Incr(ctx, name, delta)
}

func (s slow) Get(ctx context.Context, name string) (int64, error) {
	time.Sleep(s.d)
	return s.Store.Get(ctx, name)
}

func (s slow) Set(ctx context.Context, name string, v int64) error {
	time.Sleep(s.d)
	return s.Store.Set(ctx, name, v)
}

func (s slow) Del(ctx context.Context, name string) error {
	time.Sleep(s.d)
	return s.Store.Del(ctx, name)
}

func (s slow) Reset(ctx context.Context) error {
	time.Sleep(s.d)
	return s.Store.Reset(ctx)
}

func (s slow) Ping(ctx context.Context) error {
	time.Sleep(s.d)
	return s.Store.Ping(ctx)
}
