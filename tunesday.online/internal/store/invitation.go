package store

import (
	"database/sql"
	"errors"
	"time"

	"tunesday/tunesday.online/internal/db"
)

// Invitation is a pending or accepted team invitation.
type Invitation struct {
	ID         string
	TeamID     string
	Email      string
	ProviderID int64 // 0 means not pre-assigned
	Token      string
	AcceptedBy string // empty while pending
	CreatedAt  time.Time
}

// Pending reports whether the invitation has not been accepted yet.
func (i *Invitation) Pending() bool { return i.AcceptedBy == "" }

// InvitationStore handles invitation persistence.
type InvitationStore struct {
	db *db.DB
}

// NewInvitationStore creates a new InvitationStore.
func NewInvitationStore(database *db.DB) *InvitationStore {
	return &InvitationStore{db: database}
}

// Create inserts an invitation.
func (s *InvitationStore) Create(inv *Invitation) error {
	if inv.ID == "" {
		inv.ID = newID()
	}
	if inv.Token == "" {
		inv.Token = NewToken()
	}
	var providerID any
	if inv.ProviderID != 0 {
		providerID = inv.ProviderID
	}
	var acceptedBy any
	if inv.AcceptedBy != "" {
		acceptedBy = inv.AcceptedBy
	}
	_, err := s.db.Exec(
		`INSERT INTO invitations (id, team_id, email, provider_id, token, accepted_by, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		inv.ID, inv.TeamID, inv.Email, providerID, inv.Token, acceptedBy, formatTime(time.Now()),
	)
	return err
}

// GetByToken returns an invitation by token.
func (s *InvitationStore) GetByToken(token string) (*Invitation, error) {
	var inv Invitation
	var providerID sql.NullInt64
	var acceptedBy sql.NullString
	var createdAt sql.NullString
	err := s.db.QueryRow(
		`SELECT id, team_id, email, provider_id, token, accepted_by, created_at
		 FROM invitations WHERE token = ?`,
		token,
	).Scan(&inv.ID, &inv.TeamID, &inv.Email, &providerID, &inv.Token, &acceptedBy, &createdAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	if providerID.Valid {
		inv.ProviderID = providerID.Int64
	}
	if acceptedBy.Valid {
		inv.AcceptedBy = acceptedBy.String
	}
	if createdAt.Valid {
		inv.CreatedAt = parseTime(createdAt.String)
	}
	return &inv, nil
}

// MarkAccepted records which user accepted the invitation.
func (s *InvitationStore) MarkAccepted(id, userID string) error {
	_, err := s.db.Exec(`UPDATE invitations SET accepted_by = ? WHERE id = ?`, userID, id)
	return err
}

// Delete removes an invitation.
func (s *InvitationStore) Delete(id string) error {
	_, err := s.db.Exec(`DELETE FROM invitations WHERE id = ?`, id)
	return err
}

// ListPendingByTeam returns open invitations for a team.
func (s *InvitationStore) ListPendingByTeam(teamID string) ([]*Invitation, error) {
	rows, err := s.db.Query(
		`SELECT id, team_id, email, provider_id, token, created_at
		 FROM invitations WHERE team_id = ? AND accepted_by IS NULL
		 ORDER BY created_at`,
		teamID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*Invitation
	for rows.Next() {
		var inv Invitation
		var providerID sql.NullInt64
		var createdAt sql.NullString
		if err := rows.Scan(&inv.ID, &inv.TeamID, &inv.Email, &providerID, &inv.Token, &createdAt); err != nil {
			return nil, err
		}
		if providerID.Valid {
			inv.ProviderID = providerID.Int64
		}
		if createdAt.Valid {
			inv.CreatedAt = parseTime(createdAt.String)
		}
		out = append(out, &inv)
	}
	return out, rows.Err()
}

// HasPendingForEmail reports whether a pending invitation exists for an email in a team.
func (s *InvitationStore) HasPendingForEmail(teamID, email string) (bool, error) {
	var count int
	err := s.db.QueryRow(
		`SELECT COUNT(*) FROM invitations WHERE team_id = ? AND email = ? AND accepted_by IS NULL`,
		teamID, email,
	).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}
