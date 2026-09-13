package postgres_test

import (
	"context"
	"os"
	"testing"

	"systems-trinkets/harness/example-sut/counter"
	"systems-trinkets/harness/example-sut/counter/store/postgres"
)

// defaultDSN is cmd/counter's default: the infra/postgres.compose.yml
// database. COUNTER_POSTGRES_DSN overrides it.
const defaultDSN = "postgres://trinkets:trinkets@127.0.0.1:5432/trinkets?sslmode=disable"

func TestStore(t *testing.T) {
	dsn := defaultDSN
	if env := os.Getenv("COUNTER_POSTGRES_DSN"); env != "" {
		dsn = env
	}
	counter.StoreTest(t, func() counter.Store {
		s, err := postgres.Open(dsn)
		if err != nil {
			skipUnreachable(t, err)
		}
		if err := s.Ping(context.Background()); err != nil {
			s.Close()
			skipUnreachable(t, err)
		}
		return s
	})
}

// skipUnreachable skips rather than fails: this test wants a live PostgreSQL,
// and not having one is a missing container, not a broken store. The DSN is
// not echoed — Open's error already carries a redacted copy.
func skipUnreachable(t *testing.T, err error) {
	t.Helper()
	t.Skipf("skipping: no postgres reachable (start infra/postgres.compose.yml, or set COUNTER_POSTGRES_DSN): %v", err)
}
