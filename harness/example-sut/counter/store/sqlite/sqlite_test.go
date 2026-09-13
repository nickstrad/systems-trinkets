package sqlite_test

import (
	"path/filepath"
	"testing"

	"systems-trinkets/harness/example-sut/counter"
	"systems-trinkets/harness/example-sut/counter/store/sqlite"
)

func TestStore(t *testing.T) {
	dsn := filepath.Join(t.TempDir(), "counter.db")
	counter.StoreTest(t, func() counter.Store {
		s, err := sqlite.Open(dsn)
		if err != nil {
			t.Fatalf("Open(%s): %v", dsn, err)
		}
		return s
	})
}
