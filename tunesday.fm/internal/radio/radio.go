// Package radio holds the server-authoritative state of each team's listening
// room: the queue, the current track, and pause/resume math. The web layer
// composes it with the live WebSocket hub; a restart resets the room to idle.
package radio

import (
	"math/rand"
	"sort"
	"sync"
	"time"

	"tunesday/tunesday.fm/internal/live"
)

const (
	ModeOrdered  = "ordered"
	ModeShuffled = "shuffled"

	// minEndedElapsed refuses bogus "ended" reports right after a seek or
	// track change (clients send their own duration math, clocks skew).
	minEndedElapsed = 10 * time.Second
)

// State is a snapshot of a team's radio room.
type State struct {
	Mode      string
	Queue     []int64
	Index     int // -1 = nothing started yet
	StartedAt time.Time
	Paused    bool
	PausedAt  time.Time
}

// Idle reports whether nothing has been started (or the room was reset).
func (s State) Idle() bool { return s.Index < 0 || s.Index >= len(s.Queue) }

// TuneID returns the current tune id, or 0 when idle.
func (s State) TuneID() int64 {
	if s.Idle() {
		return 0
	}
	return s.Queue[s.Index]
}

// Room is one team's radio state plus listener aliases.
type Room struct {
	mu         sync.Mutex
	state      State
	sessionID  string
	aliases    map[string]string
	endedGuard time.Duration
}

func newRoom(guard time.Duration) *Room {
	return &Room{
		state:      State{Mode: ModeOrdered, Index: -1},
		sessionID:  live.Token(),
		aliases:    map[string]string{},
		endedGuard: guard,
	}
}

// SessionID identifies this room incarnation for play-stat attribution.
func (r *Room) SessionID() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.sessionID
}

// Snapshot copies the current state for broadcast payloads.
func (r *Room) Snapshot() State {
	r.mu.Lock()
	defer r.mu.Unlock()
	st := r.state
	st.Queue = append([]int64(nil), r.state.Queue...)
	return st
}

// AliasFor returns the listener's stable alias inside this room.
func (r *Room) AliasFor(userID string) string {
	r.mu.Lock()
	defer r.mu.Unlock()
	if a, ok := r.aliases[userID]; ok {
		return a
	}
	used := map[string]struct{}{}
	for _, a := range r.aliases {
		used[a] = struct{}{}
	}
	a := live.NewAlias(used)
	r.aliases[userID] = a
	return a
}

// Play starts (or resumes) playback. tuneID 0 means "resume, or start the
// queue from the top". ids is the fresh tune catalogue of the team.
// It reports the resulting state so callers can broadcast.
func (r *Room) Play(ids []int64, tuneID int64) State {
	r.mu.Lock()
	defer r.mu.Unlock()

	st := &r.state

	if st.Paused && tuneID == 0 {
		// resume where we left off
		st.StartedAt = st.StartedAt.Add(time.Since(st.PausedAt))
		st.Paused = false
		st.PausedAt = time.Time{}
		return r.snapshotLocked()
	}

	if len(ids) == 0 {
		return r.snapshotLocked()
	}

	if tuneID != 0 {
		if idx := indexOf(st.Queue, tuneID); idx != -1 {
			st.Index = idx
		} else {
			// unknown tune: rebuild around it at the front
			st.Queue = append([]int64{tuneID}, ids...)
			st.Index = 0
		}
	} else if st.Idle() {
		r.rebuildLocked(ids)
		st.Index = 0
	}

	st.StartedAt = time.Now()
	st.Paused = false
	st.PausedAt = time.Time{}
	return r.snapshotLocked()
}

// Pause freezes the position; the room stays on the current track.
func (r *Room) Pause() State {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.state.Idle() || r.state.Paused {
		return r.snapshotLocked()
	}
	r.state.Paused = true
	r.state.PausedAt = time.Now()
	return r.snapshotLocked()
}

// Next advances to the following track, rebuilding and looping at the end.
func (r *Room) Next(ids []int64) State {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.advanceLocked(ids)
	return r.snapshotLocked()
}

// Prev goes back one track, wrapping to the end.
func (r *Room) Prev(ids []int64) State {
	r.mu.Lock()
	defer r.mu.Unlock()
	st := &r.state
	if st.Idle() {
		if len(ids) == 0 {
			return r.snapshotLocked()
		}
		r.rebuildLocked(ids)
		st.Index = len(st.Queue) - 1
	} else {
		st.Index--
		if st.Index < 0 {
			r.rebuildLocked(ids)
			st.Index = len(st.Queue) - 1
		}
	}
	st.StartedAt = time.Now()
	st.Paused = false
	st.PausedAt = time.Time{}
	return r.snapshotLocked()
}

// Ended handles a client's "track finished" report. It only advances when
// the reported tune is actually the current one (dedupes the first-wins race
// among clients) and enough time has passed since the track started.
func (r *Room) Ended(ids []int64, tuneID int64, now time.Time) State {
	r.mu.Lock()
	defer r.mu.Unlock()
	st := &r.state
	if st.Idle() || st.Paused || st.TuneID() != tuneID {
		return r.snapshotLocked()
	}
	if r.endedGuard > 0 && now.Sub(st.StartedAt) < r.endedGuard {
		return r.snapshotLocked()
	}
	r.advanceLocked(ids)
	return r.snapshotLocked()
}

// SetMode switches queue order. The current track moves to the front of the
// re-derived queue so it keeps playing, and everything after it follows in
// the new order.
func (r *Room) SetMode(mode string) State {
	r.mu.Lock()
	defer r.mu.Unlock()
	if mode != ModeOrdered && mode != ModeShuffled {
		return r.snapshotLocked()
	}
	st := &r.state
	if st.Mode == mode {
		return r.snapshotLocked()
	}
	st.Mode = mode
	if !st.Idle() {
		current := st.TuneID()
		ids := append([]int64(nil), st.Queue...)
		if mode == ModeShuffled {
			rand.Shuffle(len(ids), func(i, j int) { ids[i], ids[j] = ids[j], ids[i] })
		} else {
			sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
		}
		rest := make([]int64, 0, len(ids))
		for _, id := range ids {
			if id != current {
				rest = append(rest, id)
			}
		}
		st.Queue = append([]int64{current}, rest...)
		st.Index = 0
	}
	return r.snapshotLocked()
}

// advanceLocked moves to the next track, re-deriving the queue from the
// fresh catalogue whenever the cycle wraps.
func (r *Room) advanceLocked(ids []int64) {
	st := &r.state
	if len(ids) == 0 {
		st.Index = -1
		return
	}
	if st.Idle() {
		r.rebuildLocked(ids)
	} else {
		st.Index++
		if st.Index >= len(st.Queue) {
			r.rebuildLocked(ids) // loop forever; fresh catalogue, new shuffle
		}
	}
	st.StartedAt = time.Now()
	st.Paused = false
	st.PausedAt = time.Time{}
}

// rebuildLocked resets the queue from the catalogue, honouring shuffle mode.
func (r *Room) rebuildLocked(ids []int64) {
	q := append([]int64(nil), ids...)
	if r.state.Mode == ModeShuffled {
		rand.Shuffle(len(q), func(i, j int) { q[i], q[j] = q[j], q[i] })
	}
	r.state.Queue = q
	r.state.Index = 0
}

func (r *Room) snapshotLocked() State {
	st := r.state
	st.Queue = append([]int64(nil), r.state.Queue...)
	return st
}

func indexOf(list []int64, v int64) int {
	for i, id := range list {
		if id == v {
			return i
		}
	}
	return -1
}

// Manager owns one Room per team.
type Manager struct {
	mu         sync.Mutex
	rooms      map[string]*Room
	endedGuard time.Duration
}

// NewManager creates an empty radio manager with production guards.
func NewManager() *Manager {
	return &Manager{rooms: map[string]*Room{}, endedGuard: minEndedElapsed}
}

// NewManagerWithEndedGuard builds a manager with a custom "ended" sanity
// window; tests use zero to skip it.
func NewManagerWithEndedGuard(guard time.Duration) *Manager {
	return &Manager{rooms: map[string]*Room{}, endedGuard: guard}
}

// For returns the team's room, creating it on first use.
func (m *Manager) For(teamID string) *Room {
	m.mu.Lock()
	defer m.mu.Unlock()
	room, ok := m.rooms[teamID]
	if !ok {
		room = newRoom(m.endedGuard)
		m.rooms[teamID] = room
	}
	return room
}
