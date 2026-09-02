package store

import (
	"database/sql"
	"errors"

	"tunesday/tunesday.fm/internal/db"
)

// TeamMember links a user to a team and one of its providers.
type TeamMember struct {
	TeamID     string
	UserID     string
	ProviderID int64
	Role       string // "admin" or "member"
	MagicToken string
}

// TeamMemberView is a member joined with user and provider info.
type TeamMemberView struct {
	TeamMember
	Email        string
	ProviderName string
}

// TeamMemberStore handles membership persistence.
type TeamMemberStore struct {
	db *db.DB
}

// NewTeamMemberStore creates a new TeamMemberStore.
func NewTeamMemberStore(database *db.DB) *TeamMemberStore {
	return &TeamMemberStore{db: database}
}

// Create inserts a membership with a fresh magic token.
func (s *TeamMemberStore) Create(m *TeamMember) error {
	if m.MagicToken == "" {
		m.MagicToken = NewToken()
	}
	_, err := s.db.Exec(
		`INSERT INTO team_members (team_id, user_id, provider_id, role, magic_token)
		 VALUES (?, ?, ?, ?, ?)`,
		m.TeamID, m.UserID, m.ProviderID, m.Role, m.MagicToken,
	)
	return err
}

// Get returns a membership by team and user.
func (s *TeamMemberStore) Get(teamID, userID string) (*TeamMember, error) {
	var m TeamMember
	err := s.db.QueryRow(
		`SELECT team_id, user_id, provider_id, role, magic_token
		 FROM team_members WHERE team_id = ? AND user_id = ?`,
		teamID, userID,
	).Scan(&m.TeamID, &m.UserID, &m.ProviderID, &m.Role, &m.MagicToken)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &m, nil
}

// GetByMagicToken returns a membership by its magic link token.
func (s *TeamMemberStore) GetByMagicToken(token string) (*TeamMember, error) {
	var m TeamMember
	err := s.db.QueryRow(
		`SELECT team_id, user_id, provider_id, role, magic_token
		 FROM team_members WHERE magic_token = ?`,
		token,
	).Scan(&m.TeamID, &m.UserID, &m.ProviderID, &m.Role, &m.MagicToken)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &m, nil
}

// ListByTeam returns all members with email and provider name.
func (s *TeamMemberStore) ListByTeam(teamID string) ([]*TeamMemberView, error) {
	rows, err := s.db.Query(
		`SELECT m.team_id, m.user_id, m.provider_id, m.role, m.magic_token,
		        u.email, p.name
		 FROM team_members m
		 JOIN users u ON u.id = m.user_id
		 JOIN providers p ON p.id = m.provider_id
		 WHERE m.team_id = ?
		 ORDER BY p.name COLLATE NOCASE`,
		teamID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*TeamMemberView
	for rows.Next() {
		var v TeamMemberView
		if err := rows.Scan(&v.TeamID, &v.UserID, &v.ProviderID, &v.Role, &v.MagicToken,
			&v.Email, &v.ProviderName); err != nil {
			return nil, err
		}
		out = append(out, &v)
	}
	return out, rows.Err()
}

// UpdateRole sets a member's role.
func (s *TeamMemberStore) UpdateRole(teamID, userID, role string) error {
	_, err := s.db.Exec(
		`UPDATE team_members SET role = ? WHERE team_id = ? AND user_id = ?`,
		role, teamID, userID,
	)
	return err
}

// UpdateProvider reassigns a member to another provider.
func (s *TeamMemberStore) UpdateProvider(teamID, userID string, providerID int64) error {
	_, err := s.db.Exec(
		`UPDATE team_members SET provider_id = ? WHERE team_id = ? AND user_id = ?`,
		providerID, teamID, userID,
	)
	return err
}

// Delete removes a membership.
func (s *TeamMemberStore) Delete(teamID, userID string) error {
	_, err := s.db.Exec(`DELETE FROM team_members WHERE team_id = ? AND user_id = ?`, teamID, userID)
	return err
}

// CountAdmins returns how many admins a team has.
func (s *TeamMemberStore) CountAdmins(teamID string) (int, error) {
	var count int
	err := s.db.QueryRow(
		`SELECT COUNT(*) FROM team_members WHERE team_id = ? AND role = 'admin'`, teamID,
	).Scan(&count)
	return count, err
}
