package results

import "sync"

// Buffer is an in-memory Recorder for unit tests. Safe for concurrent use.
type Buffer struct {
	mu      sync.Mutex
	Tests   []TestRow
	Checks  []CheckRow
	Samples []SampleRow
	Metrics []MetricRow
}

func (b *Buffer) Test(r TestRow) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.Tests = append(b.Tests, r)
}

func (b *Buffer) Check(r CheckRow) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.Checks = append(b.Checks, r)
}

func (b *Buffer) Sample(r SampleRow) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.Samples = append(b.Samples, r)
}

func (b *Buffer) Metric(r MetricRow) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.Metrics = append(b.Metrics, r)
}

// Snapshot returns copies of the collected rows, safe to read while writers
// are still active.
func (b *Buffer) Snapshot() (tests []TestRow, checks []CheckRow, samples []SampleRow, metrics []MetricRow) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]TestRow(nil), b.Tests...),
		append([]CheckRow(nil), b.Checks...),
		append([]SampleRow(nil), b.Samples...),
		append([]MetricRow(nil), b.Metrics...)
}
