// Package redis holds the go-redis helpers Redis lessons repeat. It is the
// Redis twin of lab/valkey: same shape, different server and port. It lives
// beside lab rather than in it so lab stays standard-library only and a
// lesson that never talks to Redis does not compile go-redis. The go-redis
// import is aliased goredis so it does not clash with this package's name.
package redis

import (
	"context"

	goredis "github.com/redis/go-redis/v9"

	"github.com/nickstrad/systems-trinkets/internal/lab"
)

// DefaultURL is the local dev Redis from software/software.md (no auth). It
// is port 6380 so it can run beside Valkey on 6379.
const DefaultURL = "redis://localhost:6380"

// URL is REDIS_URL, or DefaultURL. It is a redis:// URL, not a host:port
// address, so it goes through goredis.ParseURL rather than
// goredis.Options.Addr. It is not CACHE_URL, which lab/valkey reads, so a
// lesson can talk to both servers at once.
func URL() string { return lab.Env("REDIS_URL", DefaultURL) }

// Connect opens a client for URL() and pings it, failing fast on either
// error. The ping matters: goredis.NewClient does not contact the server, so
// without it a stopped Redis only surfaces on the first timed command.
func Connect(ctx context.Context) *goredis.Client {
	opts, err := goredis.ParseURL(URL())
	lab.Check(err)
	client := goredis.NewClient(opts)
	lab.Check(client.Ping(ctx).Err())
	return client
}
