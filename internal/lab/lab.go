// Package lab holds the few helpers every lesson repeats: service defaults,
// fail-fast error checking, and the measurements.csv writer. It depends only
// on the standard library so a lesson never compiles a driver it does not use.
package lab

import (
	"os"
	"time"
)

// Local dev credentials for the services in services/index.md. They are not
// secret; every lesson prints and uses them as-is.
const (
	DefaultPostgresURL = "postgres://trinkets:trinkets@localhost:5432/trinkets"
	DefaultValkeyURL   = "redis://localhost:6379"
)

// PostgresURL is DATABASE_URL, or the local default.
func PostgresURL() string { return Env("DATABASE_URL", DefaultPostgresURL) }

// ValkeyURL is CACHE_URL, or the local default. Pass it to redis.ParseURL.
func ValkeyURL() string { return Env("CACHE_URL", DefaultValkeyURL) }

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
