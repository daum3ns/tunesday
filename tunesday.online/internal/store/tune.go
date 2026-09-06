package store

import (
	"database/sql"
	"errors"
	"time"

	"tunesday/tunesday.online/internal/db"
)

// Tune is a single tune entry belonging to a team and provider.
type Tune struct {
	ID         int64
	TeamID     string
	Title      string
	Link       string
	YouTubeID  string
	ProviderID int64
	AddedAt    time.Time
}

// TuneView is a tune joined with its provider name.
type TuneView struct {
	Tune
	ProviderName string
}

// TuneStore handles tune persistence.
type TuneStore struct {
	db *db.DB
}

// NewTuneStore creates a new TuneStore.
func NewTuneStore(database *db.DB) *TuneStore {
	return &TuneStore{db: database}
}

// ListRecentByTeam returns the newest tunes for a team.
func (s *TuneStore) ListRecentByTeam(teamID string, limit int) ([]*TuneView, error) {
	rows, err := s.db.Query(
		`SELECT t.id, t.team_id, t.title, t.link, t.youtube_id, t.provider_id, t.added_at, p.name
		 FROM tunes t JOIN providers p ON p.id = t.provider_id
		 WHERE t.team_id = ?
		 ORDER BY t.added_at DESC LIMIT ?`,
		teamID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*TuneView
	for rows.Next() {
		var v TuneView
		var addedAt sql.NullString
		if err := rows.Scan(&v.ID, &v.TeamID, &v.Title, &v.Link, &v.YouTubeID,
			&v.ProviderID, &addedAt, &v.ProviderName); err != nil {
			return nil, err
		}
		if addedAt.Valid {
			v.AddedAt = parseTime(addedAt.String)
		}
		out = append(out, &v)
	}
	return out, rows.Err()
}

// ListAllByTeam returns every tune of a team in chronological order.
func (s *TuneStore) ListAllByTeam(teamID string) ([]*TuneView, error) {
	rows, err := s.db.Query(
		`SELECT t.id, t.team_id, t.title, t.link, t.youtube_id, t.provider_id, t.added_at, p.name
		 FROM tunes t JOIN providers p ON p.id = t.provider_id
		 WHERE t.team_id = ?
		 ORDER BY t.added_at, t.id`,
		teamID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*TuneView
	for rows.Next() {
		var v TuneView
		var addedAt sql.NullString
		if err := rows.Scan(&v.ID, &v.TeamID, &v.Title, &v.Link, &v.YouTubeID,
			&v.ProviderID, &addedAt, &v.ProviderName); err != nil {
			return nil, err
		}
		if addedAt.Valid {
			v.AddedAt = parseTime(addedAt.String)
		}
		out = append(out, &v)
	}
	return out, rows.Err()
}

// GetByID returns a single tune with its provider name.
func (s *TuneStore) GetByID(id int64) (*TuneView, error) {
	var v TuneView
	var addedAt sql.NullString
	err := s.db.QueryRow(
		`SELECT t.id, t.team_id, t.title, t.link, t.youtube_id, t.provider_id, t.added_at, p.name
		 FROM tunes t JOIN providers p ON p.id = t.provider_id WHERE t.id = ?`, id,
	).Scan(&v.ID, &v.TeamID, &v.Title, &v.Link, &v.YouTubeID, &v.ProviderID, &addedAt, &v.ProviderName)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	if addedAt.Valid {
		v.AddedAt = parseTime(addedAt.String)
	}
	return &v, nil
}

// CountByTeam returns how many tunes a team has.
func (s *TuneStore) CountByTeam(teamID string) (int, error) {
	var count int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM tunes WHERE team_id = ?`, teamID).Scan(&count)
	return count, err
}

// CountAddedBetween counts tunes added in the half-open UTC interval
// [start, end). Timestamps are stored in the canonical UTC layout, so
// lexicographic comparison equals chronological order.
func (s *TuneStore) CountAddedBetween(teamID, start, end string) (int, error) {
	var count int
	err := s.db.QueryRow(
		`SELECT COUNT(*) FROM tunes
		 WHERE team_id = ? AND added_at >= ? AND added_at < ?`,
		teamID, start, end,
	).Scan(&count)
	return count, err
}
