// Package core contains the cache-aside operation, independent of the lesson runner.
package core

import (
	"context"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/redis/go-redis/v9"
)

// Querier accepts a pgx connection or pool. Concurrent callers should use a pool.
type Querier interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

// ReadProfile tries the cache, falls back to Postgres, and fills the cache.
// Clients belong to the caller; this function does no setup or reporting.
func ReadProfile(ctx context.Context, pg Querier, cache *redis.Client, id int64, ttl time.Duration) (name, source string, err error) {
	key := ProfileKey(id)
	name, err = cache.Get(ctx, key).Result()
	if err == nil {
		return name, "cache", nil
	}
	if err != redis.Nil {
		return "", "", err
	}
	if err = pg.QueryRow(ctx, `select name from cache_aside_profiles where id = $1`, id).Scan(&name); err != nil {
		return "", "", err
	}
	if err = cache.Set(ctx, key, name, ttl).Err(); err != nil {
		return "", "", err
	}
	return name, "postgres", nil
}

// ProfileKey is shared by reads and fixture invalidation.
func ProfileKey(id int64) string {
	return "profile:" + strconv.FormatInt(id, 10)
}
