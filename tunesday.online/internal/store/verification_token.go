package store

import (
	"database/sql"
	"errors"
	"time"

	"tunesday/tunesday.online/internal/db"
)

// VerificationToken represents an email verification token.
type VerificationToken struct {
	ID        string
	UserID    string
	Token     string
	Used      bool
	CreatedAt time.Time
	UsedAt    *time.Time
}

// VerificationTokenStore handles verification token persistence.
type VerificationTokenStore struct {
	db *db.DB
}

// NewVerificationTokenStore creates a new VerificationTokenStore.
func NewVerificationTokenStore(database *db.DB) *VerificationTokenStore {
	return &VerificationTokenStore{db: database}
}

// Create inserts a new verification token.
func (s *VerificationTokenStore) Create(token *VerificationToken) error {
	used := 0
	if token.Used {
		used = 1
	}
	_, err := s.db.Exec(
		`INSERT INTO verification_tokens (id, user_id, token, used, created_at, used_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		token.ID, token.UserID, token.Token, used, formatTime(token.CreatedAt), formatTimePtr(token.UsedAt),
	)
	return err
}

// GetByToken returns a token by its value.
func (s *VerificationTokenStore) GetByToken(token string) (*VerificationToken, error) {
	var t VerificationToken
	var used int
	var createdAt string
	var usedAt sql.NullString
	err := s.db.QueryRow(
		`SELECT id, user_id, token, used, created_at, used_at FROM verification_tokens WHERE token = ?`,
		token,
	).Scan(&t.ID, &t.UserID, &t.Token, &used, &createdAt, &usedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	t.Used = used == 1
	t.CreatedAt = parseTime(createdAt)
	if usedAt.Valid {
		u := parseTime(usedAt.String)
		t.UsedAt = &u
	}
	return &t, nil
}

// MarkUsed marks a token as used.
func (s *VerificationTokenStore) MarkUsed(id string) error {
	_, err := s.db.Exec(
		`UPDATE verification_tokens SET used = 1, used_at = datetime('now') WHERE id = ?`,
		id,
	)
	return err
}
