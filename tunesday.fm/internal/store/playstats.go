package store

import (
	"time"

	"tunesday/tunesday.fm/internal/db"
)

// PlayStat is one radio track-start event.
type PlayStat struct {
	TeamID    string
	TuneID    int64
	UserID    string // who triggered the start; empty for system/loop
	SessionID string
	StartedAt time.Time
}

// PlayStatStore records and aggregates radio plays.
type PlayStatStore struct {
	db *db.DB
}

// NewPlayStatStore creates a new PlayStatStore.
func NewPlayStatStore(database *db.DB) *PlayStatStore {
	return &PlayStatStore{db: database}
}

// Record logs a track start. A missing user is stored as NULL.
func (s *PlayStatStore) Record(stat *PlayStat) error {
	var userID any
	if stat.UserID != "" {
		userID = stat.UserID
	}
	at := stat.StartedAt
	if at.IsZero() {
		at = time.Now()
	}
	_, err := s.db.Exec(
		`INSERT INTO play_stats (team_id, tune_id, user_id, room_session_id, started_at)
		 VALUES (?, ?, ?, ?, ?)`,
		stat.TeamID, stat.TuneID, userID, stat.SessionID, formatTime(at),
	)
	return err
}

// CountByTune returns how many times a tune has been played.
func (s *PlayStatStore) CountByTune(tuneID int64) (int, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM play_stats WHERE tune_id = ?`, tuneID).Scan(&n)
	return n, err
}
