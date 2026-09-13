package valkey_test

import (
	"context"
	"os"
	"testing"

	"systems-trinkets/harness/example-sut/counter"
	"systems-trinkets/harness/example-sut/counter/store/valkey"
)

// defaultDSN is cmd/counter's default: DB index 1, kept clear of DB 0 so a
// FLUSHDB here never touches anything else. COUNTER_VALKEY_DSN overrides it.
const defaultDSN = "redis://127.0.0.1:6379/1"

func TestStore(t *testing.T) {
	dsn := defaultDSN
	if env := os.Getenv("COUNTER_VALKEY_DSN"); env != "" {
		dsn = env
	}
	counter.StoreTest(t, func() counter.Store {
		s, err := valkey.Open(dsn)
		if err != nil {
			skipUnreachable(t, dsn, err)
		}
		if err := s.Ping(context.Background()); err != nil {
			s.Close()
			skipUnreachable(t, dsn, err)
		}
		return s
	})
}

// skipUnreachable skips rather than fails: this test wants a live Valkey, and
// not having one is a missing container, not a broken store.
func skipUnreachable(t *testing.T, dsn string, err error) {
	t.Helper()
	t.Skipf("skipping: no valkey at %s (start infra/valkey.compose.yml, or set COUNTER_VALKEY_DSN): %v", dsn, err)
}
