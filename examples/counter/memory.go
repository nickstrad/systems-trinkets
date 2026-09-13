package counter

import (
	"context"
	"sync"
)

// memory is the in-process Store: a map guarded by a mutex. The mutex is the
// primitive that makes Incr atomic.
type memory struct {
	mu     sync.Mutex
	values map[string]int64
}

// NewMemory returns an empty in-memory Store.
func NewMemory() Store {
	return &memory{values: map[string]int64{}}
}

func (m *memory) Incr(_ context.Context, name string, delta int64) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.values[name] += delta
	return m.values[name], nil
}

func (m *memory) Get(_ context.Context, name string) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.values[name], nil
}

func (m *memory) Del(_ context.Context, name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.values, name)
	return nil
}

func (m *memory) Reset(context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.values = map[string]int64{}
	return nil
}

func (m *memory) Ping(context.Context) error { return nil }

func (m *memory) Close() error { return nil }
