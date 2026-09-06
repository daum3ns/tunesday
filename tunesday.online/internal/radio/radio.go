// Package radio holds the per-user "now playing" state for each team's
// listening room. The queue lives entirely on the client; the server only
// tracks who is listening and what tune each person is hearing.
package radio

import (
	"sync"

	"tunesday/tunesday.online/internal/live"
)

// Room tracks per-user now-playing info and stable listener aliases.
type Room struct {
	mu         sync.Mutex
	aliases    map[string]string // userID → alias
	nowPlaying map[string]int64  // userID → tuneID (0 = idle)
}

func newRoom() *Room {
	return &Room{
		aliases:    map[string]string{},
		nowPlaying: map[string]int64{},
	}
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

// SetNowPlaying records what tune the user is currently hearing.
// Pass tuneID 0 to clear (user stopped playing).
func (r *Room) SetNowPlaying(userID string, tuneID int64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if tuneID == 0 {
		delete(r.nowPlaying, userID)
	} else {
		r.nowPlaying[userID] = tuneID
	}
}

// NowPlayingSnapshot returns a copy of the current now-playing map.
func (r *Room) NowPlayingSnapshot() map[string]int64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	snap := make(map[string]int64, len(r.nowPlaying))
	for k, v := range r.nowPlaying {
		snap[k] = v
	}
	return snap
}

// Manager owns one Room per team.
type Manager struct {
	mu    sync.Mutex
	rooms map[string]*Room
}

// NewManager creates an empty radio manager.
func NewManager() *Manager {
	return &Manager{rooms: map[string]*Room{}}
}

// For returns the team's room, creating it on first use.
func (m *Manager) For(teamID string) *Room {
	m.mu.Lock()
	defer m.mu.Unlock()
	room, ok := m.rooms[teamID]
	if !ok {
		room = newRoom()
		m.rooms[teamID] = room
	}
	return room
}
