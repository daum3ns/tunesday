package store

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"

	"tunesday/tunesday.online/internal/db"
)

// Team represents a tunesday.online team.
type Team struct {
	ID              string
	Name            string
	Slug            string
	AdminID         string
	CreatedAt       time.Time
	Timezone        string
	TunesdayWeekday int // time.Weekday (Sunday=0); default 2 (Tuesday)
}

// TeamStore handles team persistence.
type TeamStore struct {
	db *db.DB
}

// NewTeamStore creates a new TeamStore.
func NewTeamStore(database *db.DB) *TeamStore {
	return &TeamStore{db: database}
}

// Create inserts a new team.
func (s *TeamStore) Create(team *Team) error {
	if team.ID == "" {
		team.ID = newID()
	}
	if team.Timezone == "" {
		team.Timezone = "UTC"
	}
	if team.TunesdayWeekday == 0 && team.Timezone != "" {
		team.TunesdayWeekday = int(time.Tuesday)
	}
	_, err := s.db.Exec(
		`INSERT INTO teams (id, name, slug, admin_id, created_at, timezone, tunesday_weekday)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		team.ID, team.Name, team.Slug, team.AdminID, formatTime(time.Now()),
		team.Timezone, team.TunesdayWeekday,
	)
	return err
}

// GetBySlug returns a team by URL slug.
func (s *TeamStore) GetBySlug(slug string) (*Team, error) {
	return s.get(`SELECT id, name, slug, admin_id, created_at, timezone, tunesday_weekday FROM teams WHERE slug = ?`, slug)
}

// GetByID returns a team by ID.
func (s *TeamStore) GetByID(id string) (*Team, error) {
	return s.get(`SELECT id, name, slug, admin_id, created_at, timezone, tunesday_weekday FROM teams WHERE id = ?`, id)
}

func (s *TeamStore) get(query, arg string) (*Team, error) {
	var t Team
	var createdAt sql.NullString
	var timezone sql.NullString
	var weekday sql.NullInt64
	err := s.db.QueryRow(query, arg).Scan(&t.ID, &t.Name, &t.Slug, &t.AdminID, &createdAt, &timezone, &weekday)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	t.Timezone = "UTC"
	if timezone.Valid && timezone.String != "" {
		t.Timezone = timezone.String
	}
	t.TunesdayWeekday = int(time.Tuesday)
	if weekday.Valid {
		t.TunesdayWeekday = int(weekday.Int64)
	}
	if createdAt.Valid {
		t.CreatedAt = parseTime(createdAt.String)
	}
	return &t, nil
}

// ListByUser returns all teams where the user is a member.
func (s *TeamStore) ListByUser(userID string) ([]*Team, error) {
	rows, err := s.db.Query(
		`SELECT t.id, t.name, t.slug, t.admin_id, t.created_at, t.timezone, t.tunesday_weekday
		 FROM teams t JOIN team_members m ON m.team_id = t.id
		 WHERE m.user_id = ? ORDER BY t.created_at`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var teams []*Team
	for rows.Next() {
		var t Team
		var createdAt sql.NullString
		var timezone sql.NullString
		var weekday sql.NullInt64
		if err := rows.Scan(&t.ID, &t.Name, &t.Slug, &t.AdminID, &createdAt, &timezone, &weekday); err != nil {
			return nil, err
		}
		t.Timezone = timezone.String
		if !timezone.Valid || timezone.String == "" {
			t.Timezone = "UTC"
		}
		t.TunesdayWeekday = int(weekday.Int64)
		if !weekday.Valid {
			t.TunesdayWeekday = int(time.Tuesday)
		}
		if createdAt.Valid {
			t.CreatedAt = parseTime(createdAt.String)
		}
		teams = append(teams, &t)
	}
	return teams, rows.Err()
}

// ListAll returns every team, ordered by creation date (newest first).
func (s *TeamStore) ListAll() ([]*Team, error) {
	rows, err := s.db.Query(`SELECT id, name, slug, admin_id, created_at, timezone, tunesday_weekday FROM teams ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var teams []*Team
	for rows.Next() {
		var t Team
		var createdAt sql.NullString
		var timezone sql.NullString
		var weekday sql.NullInt64
		if err := rows.Scan(&t.ID, &t.Name, &t.Slug, &t.AdminID, &createdAt, &timezone, &weekday); err != nil {
			return nil, err
		}
		t.Timezone = timezone.String
		if !timezone.Valid || timezone.String == "" {
			t.Timezone = "UTC"
		}
		t.TunesdayWeekday = int(weekday.Int64)
		if !weekday.Valid {
			t.TunesdayWeekday = int(time.Tuesday)
		}
		if createdAt.Valid {
			t.CreatedAt = parseTime(createdAt.String)
		}
		teams = append(teams, &t)
	}
	return teams, rows.Err()
}

// UpdateTunesdaySettings sets a team's timezone and Tunesday weekday.
func (s *TeamStore) UpdateTunesdaySettings(teamID, timezone string, weekday int) error {
	_, err := s.db.Exec(
		`UPDATE teams SET timezone = ?, tunesday_weekday = ? WHERE id = ?`,
		timezone, weekday, teamID,
	)
	return err
}

// Location returns the team's parsed timezone, falling back to UTC if unset
// or invalid.
func (t *Team) Location() *time.Location {
	loc, err := time.LoadLocation(t.Timezone)
	if err != nil {
		return time.UTC
	}
	return loc
}

// IsTunesday reports whether it is the team's Tunesday in its own timezone,
// evaluated at the given instant.
func (t *Team) IsTunesday(at time.Time) bool {
	return at.In(t.Location()).Weekday() == time.Weekday(t.TunesdayWeekday)
}

// SlugExists reports whether a slug is already taken.
func (s *TeamStore) SlugExists(slug string) (bool, error) {
	var count int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM teams WHERE slug = ?`, slug).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// GenerateSlug turns a team name into a URL-safe unique slug.
// It lowercases, replaces non-alphanumeric runs with hyphens, and appends
// -2, -3, ... on collision.
func (s *TeamStore) GenerateSlug(name string) (string, error) {
	base := slugify(name)
	if base == "" {
		base = "team"
	}

	slug := base
	for i := 2; ; i++ {
		exists, err := s.SlugExists(slug)
		if err != nil {
			return "", err
		}
		if !exists {
			return slug, nil
		}
		slug = fmt.Sprintf("%s-%d", base, i)
	}
}

func slugify(name string) string {
	var b strings.Builder
	prevHyphen := false
	for _, r := range strings.ToLower(strings.TrimSpace(name)) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			prevHyphen = false
			continue
		}
		if !prevHyphen && b.Len() > 0 {
			b.WriteByte('-')
			prevHyphen = true
		}
	}
	return strings.Trim(b.String(), "-")
}
