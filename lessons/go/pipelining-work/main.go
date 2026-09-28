package main

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/nickstrad/systems-trinkets/internal/lab"
	"github.com/nickstrad/systems-trinkets/internal/lab/valkey"
	"github.com/nickstrad/systems-trinkets/lessons/go/pipelining-work/core"
)

const (
	prefix    = "trinkets:valkey-pipeline"
	batchSize = 200
	runs      = 5
)

var variants = []struct {
	name string
	op   func(context.Context, core.Store, []string) error
}{
	{"sequential", core.IncrementSequential},
	{"pipeline", core.IncrementPipelined},
}

func main() {
	ctx := context.Background()
	client := valkey.Connect(ctx)
	defer client.Close()

	// Setup, the timed operations, and the invariant check all use this one
	// key list, so they cannot disagree on the key format.
	keys := make([]string, batchSize)
	for i := range keys {
		keys[i] = prefix + ":" + strconv.Itoa(i)
	}

	measurements := lab.NewMeasurements("variant", "run", "batch_size", "elapsed_ms", "actual_sum")
	defer measurements.Close()

	fmt.Printf("Valkey: %s\n", valkey.URL())
	fmt.Printf("batch size: %d counters\n\n", batchSize)

	for _, variant := range variants {
		// One untimed call so connection setup does not land in run 1.
		lab.Check(variant.op(ctx, client, keys))

		for run := 1; run <= runs; run++ {
			reset(ctx, client, keys)
			start := time.Now()
			err := variant.op(ctx, client, keys)
			elapsed := time.Since(start)
			lab.Check(err)

			sum, err := core.Sum(ctx, client, keys)
			lab.Check(err)
			elapsedMS := lab.Ms(elapsed)
			measurements.Write(
				variant.name,
				strconv.Itoa(run),
				strconv.Itoa(batchSize),
				fmt.Sprintf("%.3f", elapsedMS),
				strconv.Itoa(sum),
			)
			fmt.Printf("%-10s run=%d elapsed=%8.3f ms  amortized=%8.3f us/write  sum=%d\n",
				variant.name, run, elapsedMS, elapsedMS*1000/batchSize, sum)

			// Record first, then assert, so a failing run leaves its row in the CSV.
			if sum != batchSize {
				panic(fmt.Sprintf("%s invariant failed: sum=%d expected=%d", variant.name, sum, batchSize))
			}
		}
	}
	reset(ctx, client, keys)
}

// reset deletes every counter so each run starts from zero.
func reset(ctx context.Context, client *redis.Client, keys []string) {
	lab.Check(client.Del(ctx, keys...).Err())
}
