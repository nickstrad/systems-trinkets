// Package valkey holds the go-redis helpers Valkey lessons repeat. It lives
// beside lab rather than in it so lab stays standard-library only and a
// lesson that never talks to Valkey does not compile go-redis.
package valkey

import (
	"context"

	"github.com/redis/go-redis/v9"

	"github.com/nickstrad/systems-trinkets/internal/lab"
)

// DefaultURL is the local dev Valkey from services/index.md (no auth).
const DefaultURL = "redis://localhost:6379"

// URL is CACHE_URL, or DefaultURL. It is a redis:// URL, not a host:port
// address, so it goes through redis.ParseURL rather than redis.Options.Addr.
func URL() string { return lab.Env("CACHE_URL", DefaultURL) }

// Connect opens a client for URL() and pings it, failing fast on either
// error. The ping matters: redis.NewClient does not contact the server, so
// without it a stopped Valkey only surfaces on the first timed command.
func Connect(ctx context.Context) *redis.Client {
	opts, err := redis.ParseURL(URL())
	lab.Check(err)
	client := redis.NewClient(opts)
	lab.Check(client.Ping(ctx).Err())
	return client
}
