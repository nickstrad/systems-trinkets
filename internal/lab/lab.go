// Package lab holds the few helpers every lesson repeats: environment
// defaults, fail-fast error checking, and the measurements.csv writer. It
// depends only on the standard library so a lesson never compiles a driver it
// does not use. Driver-specific helpers, including each service's URL, live
// in the subpackages lab/postgres, lab/valkey and lab/redis.
package lab

import (
	"os"
	"time"
)

// Env returns the environment variable named key, or def when it is unset or
// empty.
func Env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// Check panics on a non-nil error. Lessons fail fast: a setup or service error
// would make every measurement after it meaningless, so it is never reported
// as a result.
func Check(err error) {
	if err != nil {
		panic(err)
	}
}

// Ms converts a duration to fractional milliseconds for CSV output.
func Ms(d time.Duration) float64 {
	return float64(d) / float64(time.Millisecond)
}
