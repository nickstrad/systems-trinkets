// Package core holds the two ways this lesson increments a batch of Valkey
// counters: one round trip per command, or every command in one pipeline.
// Callers own the client, the key set, fixture resets, and reporting.
package core

import (
	"context"
	"strconv"

	"github.com/redis/go-redis/v9"
)

// Store is the slice of go-redis a batch increment and its check need.
// *redis.Client and redis.UniversalClient both satisfy it.
type Store interface {
	Incr(ctx context.Context, key string) *redis.IntCmd
	MGet(ctx context.Context, keys ...string) *redis.SliceCmd
	Pipelined(ctx context.Context, fn func(redis.Pipeliner) error) ([]redis.Cmder, error)
}

// IncrementSequential sends one INCR per key and waits for each reply, so
// the batch costs len(keys) round trips.
func IncrementSequential(ctx context.Context, store Store, keys []string) error {
	for _, key := range keys {
		if err := store.Incr(ctx, key).Err(); err != nil {
			return err
		}
	}
	return nil
}

// IncrementPipelined queues one INCR per key and flushes them in a single
// round trip. A pipeline is not atomic: on error, earlier commands may
// already be applied.
func IncrementPipelined(ctx context.Context, store Store, keys []string) error {
	_, err := store.Pipelined(ctx, func(pipe redis.Pipeliner) error {
		for _, key := range keys {
			pipe.Incr(ctx, key)
		}
		return nil
	})
	return err
}

// Sum reads every counter in one MGET and totals them, so callers verify
// stored state rather than trusting INCR replies. Missing keys count as zero.
func Sum(ctx context.Context, store Store, keys []string) (int, error) {
	values, err := store.MGet(ctx, keys...).Result()
	if err != nil {
		return 0, err
	}
	sum := 0
	for _, value := range values {
		if value == nil {
			continue
		}
		n, err := strconv.Atoi(value.(string))
		if err != nil {
			return 0, err
		}
		sum += n
	}
	return sum, nil
}
