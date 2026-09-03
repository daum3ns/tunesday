package store

import (
	"database/sql"
	"errors"

	"tunesday/tunesday.online/internal/db"
)

// Provider is a team's tune provider, identified by name.
type Provider struct {
	ID        int64
	TeamID    string
	Name      string
	Disabled  bool
	TuneCount int
}

// ProviderView joins a provider with its assigned member for display.
type ProviderView struct {
	Provider
	MemberUserID string
	MemberEmail  string
	MemberRole   string
}

// ProviderStore handles provider persistence.
type ProviderStore struct {
	db *db.DB
}

// NewProviderStore creates a new ProviderStore.
func NewProviderStore(database *db.DB) *ProviderStore {
	return &ProviderStore{db: database}
}

// Create inserts a provider and returns it.
func (s *ProviderStore) Create(teamID, name string) (*Provider, error) {
	res, err := s.db.Exec(
		`INSERT INTO providers (team_id, name) VALUES (?, ?)`,
		teamID, name,
	)
	if err != nil {
		return nil, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}
	return &Provider{ID: id, TeamID: teamID, Name: name}, nil
}

// GetByID returns a provider.
func (s *ProviderStore) GetByID(id int64) (*Provider, error) {
	var p Provider
	var disabled int
	err := s.db.QueryRow(
		`SELECT id, team_id, name, disabled, tune_count FROM providers WHERE id = ?`, id,
	).Scan(&p.ID, &p.TeamID, &p.Name, &disabled, &p.TuneCount)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	p.Disabled = disabled == 1
	return &p, nil
}

// GetByName returns a provider by team and exact name.
func (s *ProviderStore) GetByName(teamID, name string) (*Provider, error) {
	var p Provider
	var disabled int
	err := s.db.QueryRow(
		`SELECT id, team_id, name, disabled, tune_count FROM providers WHERE team_id = ? AND name = ?`,
		teamID, name,
	).Scan(&p.ID, &p.TeamID, &p.Name, &disabled, &p.TuneCount)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	p.Disabled = disabled == 1
	return &p, nil
}

// GetOrCreate returns the provider with this name, creating it when missing.
func (s *ProviderStore) GetOrCreate(teamID, name string) (*Provider, error) {
	p, err := s.GetByName(teamID, name)
	if err != nil || p != nil {
		return p, err
	}
	return s.Create(teamID, name)
}

// ListByTeam returns all providers with their assigned member info.
func (s *ProviderStore) ListByTeam(teamID string) ([]*ProviderView, error) {
	rows, err := s.db.Query(
		`SELECT p.id, p.team_id, p.name, p.disabled, p.tune_count,
		        COALESCE(m.user_id, ''), COALESCE(u.email, ''), COALESCE(m.role, '')
		 FROM providers p
		 LEFT JOIN team_members m ON m.provider_id = p.id
		 LEFT JOIN users u ON u.id = m.user_id
		 WHERE p.team_id = ?
		 ORDER BY p.name COLLATE NOCASE`,
		teamID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*ProviderView
	for rows.Next() {
		var v ProviderView
		var disabled int
		if err := rows.Scan(&v.ID, &v.TeamID, &v.Name, &disabled, &v.TuneCount,
			&v.MemberUserID, &v.MemberEmail, &v.MemberRole); err != nil {
			return nil, err
		}
		v.Disabled = disabled == 1
		out = append(out, &v)
	}
	return out, rows.Err()
}

// ListEligibleByTeam returns active providers that are assigned to a member,
// which is the pool eligible for ceremonies.
func (s *ProviderStore) ListEligibleByTeam(teamID string) ([]*Provider, error) {
	rows, err := s.db.Query(
		`SELECT DISTINCT p.id, p.team_id, p.name, p.disabled, p.tune_count
		 FROM providers p JOIN team_members m ON m.provider_id = p.id
		 WHERE p.team_id = ? AND p.disabled = 0
		 ORDER BY p.tune_count, p.name COLLATE NOCASE`,
		teamID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*Provider
	for rows.Next() {
		var p Provider
		var disabled int
		if err := rows.Scan(&p.ID, &p.TeamID, &p.Name, &disabled, &p.TuneCount); err != nil {
			return nil, err
		}
		p.Disabled = disabled == 1
		out = append(out, &p)
	}
	return out, rows.Err()
}

// SetDisabled enables or disables a provider.
func (s *ProviderStore) SetDisabled(id int64, disabled bool) error {
	flag := 0
	if disabled {
		flag = 1
	}
	_, err := s.db.Exec(`UPDATE providers SET disabled = ? WHERE id = ?`, flag, id)
	return err
}

// Rename changes a provider's display name.
func (s *ProviderStore) Rename(id int64, name string) error {
	_, err := s.db.Exec(`UPDATE providers SET name = ? WHERE id = ?`, name, id)
	return err
}

// Delete removes a provider when it has no tunes and no assigned member.
func (s *ProviderStore) Delete(id int64) error {
	var tunes, members int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM tunes WHERE provider_id = ?`, id).Scan(&tunes); err != nil {
		return err
	}
	if tunes > 0 {
		return errors.New("provider has tunes and cannot be deleted; disable it instead")
	}
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM team_members WHERE provider_id = ?`, id).Scan(&members); err != nil {
		return err
	}
	if members > 0 {
		return errors.New("provider has an assigned member; reassign the member first")
	}
	_, err := s.db.Exec(`DELETE FROM providers WHERE id = ?`, id)
	return err
}

// RecalculateCounts rebuilds tune_count for every provider of a team.
func (s *ProviderStore) RecalculateCounts(teamID string) error {
	_, err := s.db.Exec(
		`UPDATE providers SET tune_count = (
			SELECT COUNT(*) FROM tunes WHERE tunes.provider_id = providers.id
		 ) WHERE team_id = ?`,
		teamID,
	)
	return err
}
