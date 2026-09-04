package radio

import (
	"testing"
)

func TestAliasForStable(t *testing.T) {
	r := newRoom()
	a1 := r.AliasFor("u1")
	a2 := r.AliasFor("u1")
	if a1 != a2 {
		t.Fatalf("alias must be stable, got %q and %q", a1, a2)
	}
	if a1 == "" {
		t.Fatal("alias must not be empty")
	}
}

func TestAliasesDistinct(t *testing.T) {
	r := newRoom()
	a1 := r.AliasFor("u1")
	a2 := r.AliasFor("u2")
	if a1 == a2 {
		t.Fatalf("aliases must be distinct, both got %q", a1)
	}
}

func TestSetNowPlayingBasic(t *testing.T) {
	r := newRoom()
	r.SetNowPlaying("u1", 42)
	snap := r.NowPlayingSnapshot()
	if snap["u1"] != 42 {
		t.Fatalf("expected tune 42, got %d", snap["u1"])
	}
}

func TestSetNowPlayingZeroClears(t *testing.T) {
	r := newRoom()
	r.SetNowPlaying("u1", 42)
	r.SetNowPlaying("u1", 0)
	snap := r.NowPlayingSnapshot()
	if _, ok := snap["u1"]; ok {
		t.Fatal("tuneID 0 should remove entry from snapshot")
	}
}

func TestNowPlayingSnapshotIsolation(t *testing.T) {
	r := newRoom()
	r.SetNowPlaying("u1", 10)
	snap := r.NowPlayingSnapshot()
	snap["u1"] = 99
	snap2 := r.NowPlayingSnapshot()
	if snap2["u1"] != 10 {
		t.Fatalf("mutating snapshot must not affect room, got %d", snap2["u1"])
	}
}

func TestManagerForCreatesOnDemand(t *testing.T) {
	m := NewManager()
	r1 := m.For("team-a")
	r2 := m.For("team-a")
	r3 := m.For("team-b")
	if r1 != r2 {
		t.Fatal("same team must return same room")
	}
	if r1 == r3 {
		t.Fatal("different teams must get different rooms")
	}
}
