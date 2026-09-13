package sqlite_test

import (
	"path/filepath"
	"systems-trinkets/examples/counter/internal/storetest"
	"testing"

	"systems-trinkets/examples/counter"
	"systems-trinkets/examples/counter/internal/store/sqlite"
)

func TestStore(t *testing.T) {
	dsn := filepath.Join(t.TempDir(), "counter.db")
	storetest.Run(t, func() counter.Store {
		s, err := sqlite.Open(dsn)
		if err != nil {
			t.Fatalf("Open(%s): %v", dsn, err)
		}
		return s
	})
}
