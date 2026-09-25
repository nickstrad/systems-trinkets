package main

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/redis/go-redis/v9"

	"github.com/nickstrad/systems-trinkets/internal/lab"
)

const (
	profileID = int64(42)
	requests  = 21
	// cacheTTL only needs to outlast the run; nothing expires during it.
	cacheTTL = 30 * time.Second
)

func main() {
	ctx := context.Background()

	pg, err := pgx.Connect(ctx, lab.PostgresURL())
	lab.Check(err)
	defer pg.Close(ctx)

	cacheOpts, err := redis.ParseURL(lab.ValkeyURL())
	lab.Check(err)
	cache := redis.NewClient(cacheOpts)
	defer cache.Close()
	lab.Check(cache.Ping(ctx).Err())

	_, err = pg.Exec(ctx, `
		drop table if exists cache_aside_profiles;

		create table cache_aside_profiles(
			id bigint primary key,
			name text not null
		);
	`)
	lab.Check(err)
	_, err = pg.Exec(ctx,
		`insert into cache_aside_profiles(id, name) values ($1, 'Ada')`,
		profileID,
	)
	lab.Check(err)
	lab.Check(cache.Del(ctx, profileKey(profileID)).Err())

	out := lab.NewMeasurements("request", "source", "latency_us")

	for i := range requests {
		start := time.Now()
		name, source := readProfile(ctx, pg, cache, profileID)
		elapsed := time.Since(start)

		fmt.Printf("request=%02d source=%-8s latency=%v name=%s\n", i, source, elapsed, name)

		out.Write(
			strconv.Itoa(i),
			source,
			strconv.FormatInt(elapsed.Microseconds(), 10),
		)
	}

	out.Close()
}

// readProfile is the cache-aside read path: try the cache, fall back to
// Postgres on a miss, and fill the cache so the next read hits.
func readProfile(
	ctx context.Context,
	pg *pgx.Conn,
	cache *redis.Client,
	id int64,
) (name, source string) {
	key := profileKey(id)
	name, err := cache.Get(ctx, key).Result()
	if err == nil {
		return name, "cache"
	}
	if err != redis.Nil {
		panic(err)
	}
	err = pg.QueryRow(
		ctx,
		`select name from cache_aside_profiles where id = $1`,
		id,
	).Scan(&name)
	lab.Check(err)

	lab.Check(cache.Set(ctx, key, name, cacheTTL).Err())

	return name, "postgres"
}

// profileKey is the one place the cache key format lives; setup and the read
// path must agree on it or invalidation silently stops working.
func profileKey(id int64) string {
	return "profile:" + strconv.FormatInt(id, 10)
}
