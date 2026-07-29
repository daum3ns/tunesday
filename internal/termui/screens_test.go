package termui

import (
	"math/rand"
	"testing"
	"time"

	"tunesday/internal/core"
)

func TestSelectProvider_NoParticipants(t *testing.T) {
	data := core.NewData()
	r := rand.New(rand.NewSource(1))

	sel := selectProvider(data, r)
	if sel.winner != "" {
		t.Errorf("expected no winner, got %q", sel.winner)
	}
}

func TestSelectProvider_AllDisabled(t *testing.T) {
	data := core.NewData()
	data.Participants["alice"] = 0
	data.Disabled = map[string]bool{"alice": true}
	r := rand.New(rand.NewSource(1))

	sel := selectProvider(data, r)
	if sel.winner != "" {
		t.Errorf("expected no winner when all disabled, got %q", sel.winner)
	}
}

func TestSelectProvider_SingleActive(t *testing.T) {
	data := core.NewData()
	data.Participants["alice"] = 5
	r := rand.New(rand.NewSource(1))

	sel := selectProvider(data, r)
	if sel.winner != "alice" {
		t.Errorf("expected alice, got %q", sel.winner)
	}
	if len(sel.active) != 1 || len(sel.pool) != 1 {
		t.Errorf("expected active and pool of size 1, got active=%d pool=%d", len(sel.active), len(sel.pool))
	}
}

func TestSelectProvider_BottomHalf(t *testing.T) {
	data := core.NewData()
	data.Participants["alice"] = 0
	data.Participants["bob"] = 5
	data.Participants["charlie"] = 10
	r := rand.New(rand.NewSource(1))

	sel := selectProvider(data, r)

	wantPool := []string{"alice", "bob"}
	if !sliceEqual(sel.pool, wantPool) {
		t.Errorf("expected pool %v, got %v", wantPool, sel.pool)
	}
	if sel.winner != "alice" && sel.winner != "bob" {
		t.Errorf("expected winner in pool, got %q", sel.winner)
	}
}

func TestSelectProvider_TieAtCutoff(t *testing.T) {
	data := core.NewData()
	data.Participants["alice"] = 0
	data.Participants["bob"] = 1
	data.Participants["charlie"] = 1
	data.Participants["dave"] = 10
	r := rand.New(rand.NewSource(1))

	sel := selectProvider(data, r)

	// activeCount=4, N=2, cutoff=1, ties expand pool to 3
	wantPool := []string{"alice", "bob", "charlie"}
	if !sliceEqual(sel.pool, wantPool) {
		t.Errorf("expected pool %v, got %v", wantPool, sel.pool)
	}
	found := false
	for _, name := range wantPool {
		if sel.winner == name {
			found = true
		}
	}
	if !found {
		t.Errorf("expected winner in pool, got %q", sel.winner)
	}
}

func TestSelectProvider_LastSubmitterExcluded(t *testing.T) {
	data := core.NewData()
	data.Participants["alice"] = 0
	data.Participants["bob"] = 5
	data.Participants["charlie"] = 10
	// Last submitter was alice, who would be in the pool.
	data.Tunes = []core.Tune{{Provider: "alice", AddedAt: time.Now()}}
	r := rand.New(rand.NewSource(1))

	sel := selectProvider(data, r)

	wantPool := []string{"bob"}
	if !sliceEqual(sel.pool, wantPool) {
		t.Errorf("expected pool %v, got %v", wantPool, sel.pool)
	}
	if sel.winner != "bob" {
		t.Errorf("expected bob to win when alice was last submitter, got %q", sel.winner)
	}
}

func TestSelectProvider_SingleActiveLastSubmitter(t *testing.T) {
	data := core.NewData()
	data.Participants["alice"] = 5
	// Last submitter is the only active participant.
	data.Tunes = []core.Tune{{Provider: "alice", AddedAt: time.Now()}}
	r := rand.New(rand.NewSource(1))

	sel := selectProvider(data, r)

	wantPool := []string{"alice"}
	if !sliceEqual(sel.pool, wantPool) {
		t.Errorf("expected pool %v, got %v", wantPool, sel.pool)
	}
	if sel.winner != "alice" {
		t.Errorf("expected alice to win, got %q", sel.winner)
	}
}

func TestSelectProvider_LastSubmitterByAddedAt(t *testing.T) {
	data := core.NewData()
	data.Participants["alice"] = 0
	data.Participants["bob"] = 5
	data.Participants["charlie"] = 10
	// alice is last in the slice, but bob has the latest AddedAt.
	data.Tunes = []core.Tune{
		{Provider: "alice", AddedAt: time.Now().Add(-2 * time.Hour)},
		{Provider: "bob", AddedAt: time.Now()},
	}
	r := rand.New(rand.NewSource(1))

	sel := selectProvider(data, r)

	wantPool := []string{"alice"}
	if !sliceEqual(sel.pool, wantPool) {
		t.Errorf("expected bob to be treated as last submitter, pool %v, got %v", wantPool, sel.pool)
	}
	if sel.winner != "alice" {
		t.Errorf("expected alice to win, got %q", sel.winner)
	}
}

func TestSelectProvider_DisabledIgnored(t *testing.T) {
	data := core.NewData()
	data.Participants["alice"] = 0
	data.Participants["bob"] = 5
	data.Participants["charlie"] = 10
	data.Disabled = map[string]bool{"charlie": true}
	r := rand.New(rand.NewSource(1))

	sel := selectProvider(data, r)

	// activeCount=3, N=2, disabled charlie excluded
	wantPool := []string{"alice", "bob"}
	if !sliceEqual(sel.pool, wantPool) {
		t.Errorf("expected pool %v, got %v", wantPool, sel.pool)
	}
	for _, name := range sel.active {
		if name == "charlie" {
			t.Errorf("disabled participant charlie should not be active")
		}
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
