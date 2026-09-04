package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"tunesday/tunesday.online/internal/db"
)

// Ceremony is one provider-selection event with an audit trail.
type Ceremony struct {
	ID               string
	TeamID           string
	StartedBy        string
	Token            string
	Seed             int64
	Pool             []string
	WinnerProviderID int64 // 0 while unrevealed
	TuneID           int64 // 0 until the winner's tune is registered
	AlgorithmVersion string
	StartedAt        time.Time
	RevealedAt       *time.Time
	CompletedAt      *time.Time
}

// Revealed reports whether the winner has been drawn.
func (c *Ceremony) Revealed() bool { return c.RevealedAt != nil }

// Completed reports whether the winner's tune was registered.
func (c *Ceremony) Completed() bool { return c.CompletedAt != nil }

// CeremonyStore handles ceremony persistence.
type CeremonyStore struct {
	db *db.DB
}

// NewCeremonyStore creates a new CeremonyStore.
func NewCeremonyStore(database *db.DB) *CeremonyStore {
	return &CeremonyStore{db: database}
}

// Create inserts a new open ceremony with its seed and pool recorded upfront.
func (s *CeremonyStore) Create(c *Ceremony) error {
	if c.ID == "" {
		c.ID = newID()
	}
	if c.Token == "" {
		c.Token = NewToken()
	}
	poolJSON, err := json.Marshal(c.Pool)
	if err != nil {
		return err
	}
	if c.AlgorithmVersion == "" {
		c.AlgorithmVersion = "bottom-half-v1"
	}
	_, err = s.db.Exec(
		`INSERT INTO ceremonies (id, team_id, started_by, token, seed, pool_json, winner_provider_id, algorithm_version, started_at)
		 VALUES (?, ?, ?, ?, ?, ?, NULL, ?, ?)`,
		c.ID, c.TeamID, c.StartedBy, c.Token, c.Seed, string(poolJSON), c.AlgorithmVersion, formatTime(time.Now()),
	)
	return err
}

const ceremonyColumns = `id, team_id, started_by, token, seed, pool_json, winner_provider_id, tune_id, algorithm_version, started_at, revealed_at, completed_at`

// GetByToken returns a ceremony by room token.
func (s *CeremonyStore) GetByToken(token string) (*Ceremony, error) {
	return s.get(`SELECT `+ceremonyColumns+` FROM ceremonies WHERE token = ?`, token)
}

// GetByID returns a ceremony by ID.
func (s *CeremonyStore) GetByID(id string) (*Ceremony, error) {
	return s.get(`SELECT `+ceremonyColumns+` FROM ceremonies WHERE id = ?`, id)
}

func (s *CeremonyStore) get(query, arg string) (*Ceremony, error) {
	var (
		c           Ceremony
		poolJSON    string
		winnerID    sql.NullInt64
		tuneID      sql.NullInt64
		seed        sql.NullInt64
		startedAt   sql.NullString
		revealedAt  sql.NullString
		completedAt sql.NullString
		algoVersion sql.NullString
	)
	err := s.db.QueryRow(query, arg).Scan(
		&c.ID, &c.TeamID, &c.StartedBy, &c.Token, &seed, &poolJSON, &winnerID, &tuneID,
		&algoVersion, &startedAt, &revealedAt, &completedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	if seed.Valid {
		c.Seed = seed.Int64
	}
	if err := json.Unmarshal([]byte(poolJSON), &c.Pool); err != nil {
		return nil, err
	}
	if winnerID.Valid {
		c.WinnerProviderID = winnerID.Int64
	}
	if tuneID.Valid {
		c.TuneID = tuneID.Int64
	}
	if algoVersion.Valid {
		c.AlgorithmVersion = algoVersion.String
	}
	if startedAt.Valid {
		c.StartedAt = parseTime(startedAt.String)
	}
	if revealedAt.Valid {
		t := parseTime(revealedAt.String)
		c.RevealedAt = &t
	}
	if completedAt.Valid {
		t := parseTime(completedAt.String)
		c.CompletedAt = &t
	}
	return &c, nil
}

// RecordReveal stores the drawn winner together with the pool and seed that
// were actually in play at reveal time. It only applies while unrevealed,
// making double-reveal impossible; the stored (seed, pool) reproduces the
// winner exactly.
func (s *CeremonyStore) RecordReveal(id string, seed int64, pool []string, winnerProviderID int64) error {
	poolJSON, err := json.Marshal(pool)
	if err != nil {
		return err
	}
	res, err := s.db.Exec(
		`UPDATE ceremonies
		 SET winner_provider_id = ?, revealed_at = ?, seed = ?, pool_json = ?,
		     algorithm_version = 'attendees-random-v1'
		 WHERE id = ? AND revealed_at IS NULL`,
		winnerProviderID, formatTime(time.Now()), seed, string(poolJSON), id,
	)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return errors.New("ceremony already revealed")
	}
	return nil
}

// MarkCompleted records that the winner's tune has been registered, linking
// the ceremony to that tune for a durable history.
func (s *CeremonyStore) MarkCompleted(id string, tuneID int64) error {
	_, err := s.db.Exec(
		`UPDATE ceremonies SET completed_at = ?, tune_id = ? WHERE id = ? AND completed_at IS NULL`,
		formatTime(time.Now()), tuneID, id,
	)
	return err
}

// CeremonyHistoryItem is a compact ceremony row for the dashboard list.
type CeremonyHistoryItem struct {
	Token        string
	StartedAt    time.Time
	Status       string // open | revealed | completed
	WinnerName   string
	TuneTitle    string
	PresentCount int
}

// ListRecentByTeam returns the newest ceremonies for a team with winner/tune/
// attendee info joined, for the dashboard history.
func (s *CeremonyStore) ListRecentByTeam(teamID string, limit int) ([]*CeremonyHistoryItem, error) {
	rows, err := s.db.Query(
		`SELECT c.token, c.started_at, c.revealed_at, c.completed_at,
		        COALESCE(p.name, ''), COALESCE(t.title, ''),
		        (SELECT COUNT(*) FROM ceremony_attendees a WHERE a.ceremony_id = c.id)
		 FROM ceremonies c
		 LEFT JOIN providers p ON p.id = c.winner_provider_id
		 LEFT JOIN tunes t ON t.id = c.tune_id
		 WHERE c.team_id = ?
		 ORDER BY c.started_at DESC LIMIT ?`,
		teamID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*CeremonyHistoryItem
	for rows.Next() {
		var (
			item        CeremonyHistoryItem
			startedAt   sql.NullString
			revealedAt  sql.NullString
			completedAt sql.NullString
		)
		if err := rows.Scan(&item.Token, &startedAt, &revealedAt, &completedAt,
			&item.WinnerName, &item.TuneTitle, &item.PresentCount); err != nil {
			return nil, err
		}
		if startedAt.Valid {
			item.StartedAt = parseTime(startedAt.String)
		}
		item.Status = "open"
		if revealedAt.Valid {
			item.Status = "revealed"
		}
		if completedAt.Valid {
			item.Status = "completed"
		}
		out = append(out, &item)
	}
	return out, rows.Err()
}

// ProviderWinCount is one provider's ceremony win record.
type ProviderWinCount struct {
	ProviderName      string
	Wins              int
	TotalCeremonies   int // how many times they appeared in the pool
}

// WinCounts returns per-provider win counts for completed ceremonies in a team.
func (s *CeremonyStore) WinCounts(teamID string) ([]ProviderWinCount, error) {
	rows, err := s.db.Query(
		`SELECT p.name,
		        SUM(CASE WHEN c.winner_provider_id = p.id THEN 1 ELSE 0 END) AS wins,
		        COUNT(*) AS total
		 FROM ceremonies c, providers p
		 WHERE c.team_id = ? AND c.revealed_at IS NOT NULL
		   AND p.team_id = ?
		 GROUP BY p.id ORDER BY wins DESC`,
		teamID, teamID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []ProviderWinCount
	for rows.Next() {
		var r ProviderWinCount
		if err := rows.Scan(&r.ProviderName, &r.Wins, &r.TotalCeremonies); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// CeremonyStats holds summary ceremony statistics.
type CeremonyStats struct {
	TotalCeremonies int
	AvgAttendance   float64
}

// Stats returns total ceremony count and average attendance for a team.
func (s *CeremonyStore) Stats(teamID string) (CeremonyStats, error) {
	var out CeremonyStats
	err := s.db.QueryRow(
		`SELECT COUNT(*) FROM ceremonies
		 WHERE team_id = ? AND revealed_at IS NOT NULL`, teamID,
	).Scan(&out.TotalCeremonies)
	if err != nil {
		return out, err
	}
	err = s.db.QueryRow(
		`SELECT AVG(cnt) FROM (
			SELECT COUNT(*) AS cnt
			FROM ceremonies c
			JOIN ceremony_attendees ca ON ca.ceremony_id = c.id
			WHERE c.team_id = ? AND c.revealed_at IS NOT NULL
			GROUP BY c.id
		)`, teamID,
	).Scan(&out.AvgAttendance)
	if err != nil && err != sql.ErrNoRows {
		return out, err
	}
	return out, nil
}
func (s *CeremonyStore) AddAttendee(ceremonyID, userID, alias string) error {
	_, err := s.db.Exec(
		`INSERT INTO ceremony_attendees (ceremony_id, user_id, alias) VALUES (?, ?, ?)
		 ON CONFLICT(ceremony_id, user_id) DO NOTHING`,
		ceremonyID, userID, alias,
	)
	return err
}

// Attendee is one participant in a ceremony room.
type Attendee struct {
	UserID string
	Email  string
	Alias  string
}

// ListAttendees returns attendees with their account emails.
func (s *CeremonyStore) ListAttendees(ceremonyID string) ([]*Attendee, error) {
	rows, err := s.db.Query(
		`SELECT a.user_id, a.alias, COALESCE(u.email, '')
		 FROM ceremony_attendees a JOIN users u ON u.id = a.user_id
		 WHERE a.ceremony_id = ? ORDER BY a.joined_at`,
		ceremonyID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*Attendee
	for rows.Next() {
		var a Attendee
		if err := rows.Scan(&a.UserID, &a.Alias, &a.Email); err != nil {
			return nil, err
		}
		out = append(out, &a)
	}
	return out, rows.Err()
}
