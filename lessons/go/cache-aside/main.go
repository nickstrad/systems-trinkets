package main

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/nickstrad/systems-trinkets/internal/lab"
	"github.com/nickstrad/systems-trinkets/internal/lab/postgres"
	"github.com/nickstrad/systems-trinkets/internal/lab/valkey"
	"github.com/nickstrad/systems-trinkets/lessons/go/cache-aside/core"
)

const (
	profileID = int64(42)
	requests  = 21
	// cacheTTL only needs to outlast the run; nothing expires during it.
	cacheTTL = 30 * time.Second
)

func main() {
	ctx := context.Background()

	pg := postgres.Connect(ctx)
	defer pg.Close(ctx)

	cache := valkey.Connect(ctx)
	defer cache.Close()

	_, err := pg.Exec(ctx, `
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
	lab.Check(cache.Del(ctx, core.ProfileKey(profileID)).Err())

	out := lab.NewMeasurements("request", "source", "latency_us")

	for i := range requests {
		start := time.Now()
		name, source, err := core.ReadProfile(ctx, pg, cache, profileID, cacheTTL)
		lab.Check(err)
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
