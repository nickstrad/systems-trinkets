// Package valkey backs the counter with Valkey (github.com/valkey-io/valkey-go).
// The primitive under test is INCRBY: the server applies it as one command on
// one key, so concurrent increments cannot lose each other.
package valkey

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/valkey-io/valkey-go"

	"systems-trinkets/harness/example-sut/counter"
)

type store struct{ client valkey.Client }

// Open connects to the Valkey instance named by dsn (redis://host:port/db).
// The DB index in the DSN is the one Reset flushes.
func Open(dsn string) (counter.Store, error) {
	opt, err := valkey.ParseURL(dsn)
	if err != nil {
		return nil, fmt.Errorf("valkey dsn %s: %w", dsn, err)
	}
	client, err := valkey.NewClient(opt)
	if err != nil {
		return nil, fmt.Errorf("valkey connect %s: %w", dsn, err)
	}
	return &store{client: client}, nil
}

func (s *store) Incr(ctx context.Context, name string, delta int64) (int64, error) {
	cmd := s.client.B().Incrby().Key(name).Increment(delta).Build()
	v, err := s.client.Do(ctx, cmd).AsInt64()
	if err != nil {
		return 0, fmt.Errorf("valkey incrby %s: %w", name, err)
	}
	return v, nil
}

func (s *store) Get(ctx context.Context, name string) (int64, error) {
	v, err := s.client.Do(ctx, s.client.B().Get().Key(name).Build()).AsInt64()
	if errors.Is(err, valkey.Nil) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("valkey get %s: %w", name, err)
	}
	return v, nil
}

func (s *store) Set(ctx context.Context, name string, v int64) error {
	cmd := s.client.B().Set().Key(name).Value(strconv.FormatInt(v, 10)).Build()
	if err := s.client.Do(ctx, cmd).Error(); err != nil {
		return fmt.Errorf("valkey set %s: %w", name, err)
	}
	return nil
}

func (s *store) Del(ctx context.Context, name string) error {
	if err := s.client.Do(ctx, s.client.B().Del().Key(name).Build()).Error(); err != nil {
		return fmt.Errorf("valkey del %s: %w", name, err)
	}
	return nil
}

// Reset flushes the selected DB only — never FLUSHALL, which would wipe the
// other DB indexes this Valkey instance serves.
func (s *store) Reset(ctx context.Context) error {
	if err := s.client.Do(ctx, s.client.B().Flushdb().Build()).Error(); err != nil {
		return fmt.Errorf("valkey flushdb: %w", err)
	}
	return nil
}

func (s *store) Ping(ctx context.Context) error {
	if err := s.client.Do(ctx, s.client.B().Ping().Build()).Error(); err != nil {
		return fmt.Errorf("valkey ping: %w", err)
	}
	return nil
}

func (s *store) Close() error {
	s.client.Close()
	return nil
}
