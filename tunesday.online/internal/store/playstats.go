package store

import (
	"time"

	"tunesday/tunesday.online/internal/db"
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

// PlayCountRow is one entry in a "most played" ranking.
type PlayCountRow struct {
	TuneID       int64
	Title        string
	ProviderName string
	Count        int
}

// ProviderPlayCount aggregates play counts per provider.
type ProviderPlayCount struct {
	ProviderName string
	Count        int
}

// MostPlayed returns the top tunes by play count for a team.
// If since is non-nil, only plays after that time are counted.
func (s *PlayStatStore) MostPlayed(teamID string, since *time.Time, limit int) ([]PlayCountRow, error) {
	query := `SELECT t.id, t.title, COALESCE(p.name, ''), COUNT(*) AS cnt
		FROM play_stats ps
		JOIN tunes t ON t.id = ps.tune_id
		LEFT JOIN providers p ON p.id = t.provider_id
		WHERE ps.team_id = ?`
	args := []any{teamID}
	if since != nil {
		query += ` AND ps.started_at >= ?`
		args = append(args, formatTime(*since))
	}
	query += ` GROUP BY ps.tune_id ORDER BY cnt DESC LIMIT ?`
	args = append(args, limit)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []PlayCountRow
	for rows.Next() {
		var r PlayCountRow
		if err := rows.Scan(&r.TuneID, &r.Title, &r.ProviderName, &r.Count); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// ProviderPlayCounts returns play counts grouped by provider for a team.
func (s *PlayStatStore) ProviderPlayCounts(teamID string, since *time.Time) ([]ProviderPlayCount, error) {
	query := `SELECT COALESCE(p.name, ''), COUNT(*) AS cnt
		FROM play_stats ps
		JOIN tunes t ON t.id = ps.tune_id
		LEFT JOIN providers p ON p.id = t.provider_id
		WHERE ps.team_id = ?`
	args := []any{teamID}
	if since != nil {
		query += ` AND ps.started_at >= ?`
		args = append(args, formatTime(*since))
	}
	query += ` GROUP BY t.provider_id ORDER BY cnt DESC`

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []ProviderPlayCount
	for rows.Next() {
		var r ProviderPlayCount
		if err := rows.Scan(&r.ProviderName, &r.Count); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// TotalPlays returns the total number of play events for a team.
func (s *PlayStatStore) TotalPlays(teamID string) (int, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM play_stats WHERE team_id = ?`, teamID).Scan(&n)
	return n, err
}
