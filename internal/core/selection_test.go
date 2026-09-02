package core

import (
	"math/rand"
	"testing"
	"time"
)

func TestSelectProvider_NoActive(t *testing.T) {
	r := rand.New(rand.NewSource(1))
	sel := SelectProvider(nil, nil, "", r)
	if sel.Winner != "" {
		t.Errorf("expected no winner, got %q", sel.Winner)
	}
}

func TestSelectProvider_SingleActive(t *testing.T) {
	r := rand.New(rand.NewSource(1))
	sel := SelectProvider([]string{"alice"}, map[string]int{"alice": 5}, "", r)
	if sel.Winner != "alice" {
		t.Errorf("expected alice, got %q", sel.Winner)
	}
	if len(sel.Active) != 1 || len(sel.Pool) != 1 {
		t.Errorf("expected active and pool of size 1, got active=%d pool=%d", len(sel.Active), len(sel.Pool))
	}
}

func TestSelectProvider_BottomHalf(t *testing.T) {
	active := []string{"alice", "bob", "charlie"}
	counts := map[string]int{"alice": 0, "bob": 5, "charlie": 10}
	r := rand.New(rand.NewSource(1))

	sel := SelectProvider(active, counts, "", r)

	wantPool := []string{"alice", "bob"}
	if !sliceEqual(sel.Pool, wantPool) {
		t.Errorf("expected pool %v, got %v", wantPool, sel.Pool)
	}
	if sel.Winner != "alice" && sel.Winner != "bob" {
		t.Errorf("expected winner in pool, got %q", sel.Winner)
	}
}

func TestSelectProvider_TieAtCutoff(t *testing.T) {
	active := []string{"alice", "bob", "charlie", "dave"}
	counts := map[string]int{"alice": 0, "bob": 1, "charlie": 1, "dave": 10}
	r := rand.New(rand.NewSource(1))

	sel := SelectProvider(active, counts, "", r)

	// activeCount=4, N=2, cutoff=1, ties expand pool to 3
	wantPool := []string{"alice", "bob", "charlie"}
	if !sliceEqual(sel.Pool, wantPool) {
		t.Errorf("expected pool %v, got %v", wantPool, sel.Pool)
	}
	found := false
	for _, name := range wantPool {
		if sel.Winner == name {
			found = true
		}
	}
	if !found {
		t.Errorf("expected winner in pool, got %q", sel.Winner)
	}
}

func TestSelectProvider_LastSubmitterExcluded(t *testing.T) {
	active := []string{"alice", "bob", "charlie"}
	counts := map[string]int{"alice": 0, "bob": 5, "charlie": 10}
	r := rand.New(rand.NewSource(1))

	sel := SelectProvider(active, counts, "alice", r)

	wantPool := []string{"bob"}
	if !sliceEqual(sel.Pool, wantPool) {
		t.Errorf("expected pool %v, got %v", wantPool, sel.Pool)
	}
	if sel.Winner != "bob" {
		t.Errorf("expected bob to win when alice was last submitter, got %q", sel.Winner)
	}
}

func TestSelectProvider_LastSubmitterOnlyPoolMember(t *testing.T) {
	active := []string{"alice"}
	counts := map[string]int{"alice": 5}
	r := rand.New(rand.NewSource(1))

	sel := SelectProvider(active, counts, "alice", r)

	wantPool := []string{"alice"}
	if !sliceEqual(sel.Pool, wantPool) {
		t.Errorf("expected pool %v, got %v", wantPool, sel.Pool)
	}
	if sel.Winner != "alice" {
		t.Errorf("expected alice to win, got %q", sel.Winner)
	}
}

func TestSelectProvider_NilRandomSourcePanics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("expected panic for nil random source")
		}
	}()
	SelectProvider([]string{"alice"}, nil, "", nil)
}

// TestLastSubmitterFromTunesData verifies helper logic indirectly through termui integration.
// The core function expects the caller to determine the last provider.
func TestSelectProvider_LastSubmitterDeterminedByCaller(t *testing.T) {
	active := []string{"alice", "bob", "charlie", "dave", "eve"}
	counts := map[string]int{"alice": 0, "bob": 0, "charlie": 0, "dave": 10, "eve": 10}
	r := rand.New(rand.NewSource(1))

	// Caller determines bob is last, not any other provider. Bob is excluded from the pool.
	sel := SelectProvider(active, counts, "bob", r)

	wantPool := []string{"alice", "charlie"}
	if !sliceEqual(sel.Pool, wantPool) {
		t.Errorf("expected bob to be excluded, pool %v, got %v", wantPool, sel.Pool)
	}
}

func TestSelectProviderFromData(t *testing.T) {
	data := NewData()
	data.Participants["alice"] = 0
	data.Participants["bob"] = 5
	data.Participants["charlie"] = 10
	data.Tunes = []Tune{
		{Provider: "alice", AddedAt: time.Now().Add(-2 * time.Hour)},
		{Provider: "bob", AddedAt: time.Now()},
	}
	r := rand.New(rand.NewSource(1))

	sel := SelectProviderFromData(data, r)

	wantPool := []string{"alice"}
	if !sliceEqual(sel.Pool, wantPool) {
		t.Errorf("expected bob to be treated as last submitter, pool %v, got %v", wantPool, sel.Pool)
	}
	if sel.Winner != "alice" {
		t.Errorf("expected alice to win, got %q", sel.Winner)
	}
}

func sliceEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestComputeProviderPool(t *testing.T) {
	counts := map[string]int{"alice": 0, "bob": 5, "charlie": 10}
	pool := ComputeProviderPool([]string{"alice", "bob", "charlie"}, counts, "")
	if !sliceEqual(pool, []string{"alice", "bob"}) {
		t.Fatalf("expected [alice bob], got %v", pool)
	}

	pool = ComputeProviderPool([]string{"alice", "bob", "charlie"}, counts, "alice")
	if !sliceEqual(pool, []string{"bob"}) {
		t.Fatalf("expected [bob] after excluding last submitter, got %v", pool)
	}

	if got := ComputeProviderPool(nil, nil, ""); got != nil {
		t.Fatalf("expected nil pool for no active providers, got %v", got)
	}
}

func TestPoolSeedIsReproducible(t *testing.T) {
	active := []string{"a", "b", "c", "d", "e"}
	counts := map[string]int{"a": 0, "b": 1, "c": 2, "d": 3, "e": 4}
	pool := ComputeProviderPool(active, counts, "b")

	const seed int64 = 1234567890
	first := pool[rand.New(rand.NewSource(seed)).Intn(len(pool))]
	second := pool[rand.New(rand.NewSource(seed)).Intn(len(pool))]
	if first != second {
		t.Fatalf("same seed must produce same winner, got %q vs %q", first, second)
	}
	if first == "b" {
		t.Fatal("last submitter b must be excluded from pool")
	}
}
