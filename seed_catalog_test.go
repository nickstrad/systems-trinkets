package main

import (
	"net/url"
	"strings"
	"testing"
)

// The independently frozen migration contract, not an expected set derived from
// the current seed, protects against accidental additions, omissions or reorders.
var expectedCurriculumSlugs = strings.Fields(`counter optimistic-concurrency state-machine idempotency-key inbox expiring-reservation fifo-queue lease fencing-tokens task-ownership semaphore delayed-queue retry-lifecycle priority-queue rate-limiter heartbeat leader-election notification-vs-delivery outbox saga durable-event-log consumer-checkpoints consumer-groups incremental-projection snapshots-compaction cache-consistency distributed-id sharded-counter partition-rebalancing`)

func TestCanonicalCurriculumStructure(t *testing.T) {
	if len(seedPatterns) != 29 {
		t.Fatalf("patterns=%d", len(seedPatterns))
	}
	for i, p := range seedPatterns {
		if p.slug != expectedCurriculumSlugs[i] || p.curriculumOrder != i+1 {
			t.Fatalf("position %d: %s order %d", i+1, p.slug, p.curriculumOrder)
		}
		if p.name == "" || p.family == "" || len(p.explanation) < 60 || len(p.useCases) < 20 || len(p.invariants) < 2 || len(p.readings) == 0 {
			t.Fatalf("incomplete content: %s", p.slug)
		}
		for _, r := range p.readings {
			u, err := url.Parse(r.URL)
			if err != nil || u.Scheme != "https" || u.Host == "" || r.Title == "" {
				t.Fatalf("bad reading for %s: %+v", p.slug, r)
			}
		}
		for _, sketch := range []string{p.valkey, p.sqlite, p.postgres} {
			if len(sketch) < 70 {
				t.Fatalf("incomplete sketch for %s", p.slug)
			}
		}
	}
}

func TestReconciledContentMatchesCanonicalDefinitions(t *testing.T) {
	_, db := legacyCatalog(t)
	if _, err := reconcileCatalog(db, false, nil); err != nil {
		t.Fatal(err)
	}
	for i, slug := range expectedCurriculumSlugs {
		p, err := getPattern(db, slug)
		if err != nil {
			t.Fatal(err)
		}
		want := seedPatterns[i]
		gotInv, _ := jsonText(p.Invariants)
		wantInv, _ := jsonText(want.invariants)
		gotRead, _ := jsonText(p.Readings)
		wantRead, _ := jsonText(want.readings)
		if p.Name != want.name || p.Family != want.family || p.Explanation != want.explanation || p.UseCases != want.useCases || gotInv != wantInv || gotRead != wantRead || p.CurriculumOrder == nil || *p.CurriculumOrder != i+1 {
			t.Fatalf("canonical pattern content mismatch %s", slug)
		}
		for _, e := range []struct{ slug, sketch string }{{"valkey", want.valkey}, {"sqlite", want.sqlite}, {"postgres", want.postgres}} {
			var sketch string
			err := db.QueryRow(`SELECT a.primitives FROM approaches a JOIN patterns p ON p.id=a.pattern_id JOIN engines e ON e.id=a.engine_id WHERE p.slug=? AND e.slug=? AND a.title=?`, slug, e.slug, seedApproachTitle).Scan(&sketch)
			if err != nil || sketch != e.sketch {
				t.Fatalf("canonical approach mismatch %s/%s: %v", slug, e.slug, err)
			}
		}
	}
}
