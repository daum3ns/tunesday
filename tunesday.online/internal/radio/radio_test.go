package radio

import (
	"testing"
	"time"
)

func TestPlayFromIdle(t *testing.T) {
	r := newRoom(minEndedElapsed)
	st := r.Play([]int64{3, 1, 2}, 0)
	if st.Idle() {
		t.Fatal("expected playback to start")
	}
	if st.TuneID() != 3 {
		t.Fatalf("expected first tune 3, got %d", st.TuneID())
	}
	if st.Paused {
		t.Fatal("must not start paused")
	}
}

func TestPlaySpecificTune(t *testing.T) {
	r := newRoom(minEndedElapsed)
	r.Play([]int64{1, 2, 3}, 0)
	st := r.Play([]int64{1, 2, 3}, 2)
	if st.TuneID() != 2 {
		t.Fatalf("expected tune 2, got %d", st.TuneID())
	}
	// unknown tune id gets queued at the front
	st = r.Play([]int64{1, 2, 3}, 99)
	if st.TuneID() != 99 {
		t.Fatalf("expected unknown tune 99 to start, got %d", st.TuneID())
	}
}

func TestPauseResumeMath(t *testing.T) {
	r := newRoom(minEndedElapsed)
	ids := []int64{1, 2, 3}
	r.Play(ids, 0)

	// fabricate: track started 100s ago, paused 60s in
	r.mu.Lock()
	now := time.Now()
	r.state.StartedAt = now.Add(-100 * time.Second)
	r.state.Paused = true
	r.state.PausedAt = now.Add(-40 * time.Second)
	r.mu.Unlock()

	st := r.Play(ids, 0) // resume
	if st.Paused {
		t.Fatal("resume should clear paused")
	}
	elapsed := time.Since(st.StartedAt)
	if elapsed < 60*time.Second || elapsed > 61*time.Second {
		t.Fatalf("resume must preserve ~60s of playback, got %v", elapsed)
	}
}

func TestNextLoopsForever(t *testing.T) {
	r := newRoom(minEndedElapsed)
	ids := []int64{1, 2}
	st := r.Play(ids, 0)
	if st.Index != 0 {
		t.Fatal("expected index 0")
	}
	st = r.Next(ids)
	if st.TuneID() != 2 {
		t.Fatalf("expected tune 2, got %d", st.TuneID())
	}
	// wrap: queue rebuilt, back to first
	st = r.Next(ids)
	if st.Index != 0 || st.TuneID() != 1 {
		t.Fatalf("expected loop to tune 1 at index 0, got %d/%d", st.Index, st.TuneID())
	}
	// fresh catalogue is picked up when the cycle wraps
	st = r.Next([]int64{7}) // still mid-old-cycle
	if st.TuneID() != 2 {
		t.Fatalf("old queue order must play out, got %d", st.TuneID())
	}
	st = r.Next([]int64{7}) // wraps -> rebuild from fresh catalogue
	if st.TuneID() != 7 {
		t.Fatalf("expected rebuilt queue with tune 7, got %d", st.TuneID())
	}
}

func TestPrevWrapsBack(t *testing.T) {
	r := newRoom(minEndedElapsed)
	ids := []int64{1, 2, 3}
	r.Play(ids, 0)
	st := r.Prev(ids) // from first backwards wraps to last
	if st.TuneID() != 3 {
		t.Fatalf("expected wrap to tune 3, got %d", st.TuneID())
	}
	st = r.Prev(ids)
	if st.TuneID() != 2 {
		t.Fatalf("expected tune 2, got %d", st.TuneID())
	}
}

func TestEndedGuards(t *testing.T) {
	r := newRoom(minEndedElapsed)
	ids := []int64{1, 2, 3}
	r.Play(ids, 0)

	now := time.Now()

	// too soon after start: ignored
	if st := r.Ended(ids, 1, now); st.TuneID() != 1 {
		t.Fatal("ended must be ignored within the guard window")
	}
	// wrong tune: ignored
	if st := r.Ended(ids, 5, now.Add(time.Minute)); st.TuneID() != 1 {
		t.Fatal("ended for a non-current tune must be ignored")
	}
	// paused: ignored
	r.Pause()
	if st := r.Ended(ids, 1, now.Add(time.Minute)); st.TuneID() != 1 {
		t.Fatal("ended must be ignored while paused")
	}
	r.Play(ids, 0) // resume

	// valid: advances
	st := r.Ended(ids, 1, now.Add(time.Minute))
	if st.TuneID() != 2 {
		t.Fatalf("expected advance to tune 2, got %d", st.TuneID())
	}
	// race: second client reports the old tune — no double advance
	st = r.Ended(ids, 1, now.Add(61*time.Second))
	if st.TuneID() != 2 {
		t.Fatalf("stale ended report must not advance again, got %d", st.TuneID())
	}
}

func TestSetModeKeepsCurrentUpFront(t *testing.T) {
	r := newRoom(minEndedElapsed)
	ids := []int64{1, 2, 3, 4}
	r.Play(ids, 3)
	st := r.SetMode(ModeShuffled)
	if st.Mode != ModeShuffled {
		t.Fatalf("mode not set: %+v", st)
	}
	if st.TuneID() != 3 {
		t.Fatalf("current tune must survive mode change, got %d", st.TuneID())
	}
	if len(st.Queue) != 4 || st.Queue[0] != 3 {
		t.Fatalf("queue must start with current tune: %v", st.Queue)
	}
	seen := map[int64]bool{}
	for _, id := range st.Queue {
		seen[id] = true
	}
	if len(seen) != 4 {
		t.Fatalf("mode change must not drop/duplicate tunes: %v", st.Queue)
	}
}

func TestShuffledLoopRebuild(t *testing.T) {
	r := newRoom(minEndedElapsed)
	r.SetMode(ModeShuffled)
	ids := []int64{1, 2, 3}
	st := r.Play(ids, 0)
	if len(st.Queue) != 3 {
		t.Fatal("queue built")
	}
	// run full cycle; each wrap must rebuild from catalogue
	for i := 0; i < 3; i++ {
		st = r.Next(ids)
	}
	if st.Index != 0 {
		t.Fatalf("after full cycle expect fresh index 0, got %d", st.Index)
	}
	if len(st.Queue) != 3 {
		t.Fatalf("queue length must persist, got %v", st.Queue)
	}
}

func TestAliasesStableAndDistinct(t *testing.T) {
	r := newRoom(minEndedElapsed)
	a1 := r.AliasFor("u1")
	again := r.AliasFor("u1")
	a2 := r.AliasFor("u2")
	if a1 != again {
		t.Fatal("alias must be stable per user")
	}
	if a1 == a2 {
		t.Fatal("aliases must be distinct")
	}
	if a1 == "" {
		t.Fatal("alias must not be empty")
	}
}
