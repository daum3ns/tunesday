package store

import (
	"database/sql"
	"errors"
	"time"

	"tunesday/tunesday.fm/internal/db"
)

// User represents an admin or team member.
type User struct {
	ID            string
	Email         string
	PasswordHash  string
	EmailVerified bool
	CreatedAt     time.Time
}

// UserStore handles user persistence.
type UserStore struct {
	db *db.DB
}

// NewUserStore creates a new UserStore.
func NewUserStore(database *db.DB) *UserStore {
	return &UserStore{db: database}
}

// Create inserts a new user.
func (s *UserStore) Create(user *User) error {
	verified := 0
	if user.EmailVerified {
		verified = 1
	}
	_, err := s.db.Exec(
		`INSERT INTO users (id, email, password_hash, email_verified, created_at)
		 VALUES (?, ?, ?, ?, ?)`,
		user.ID, user.Email, user.PasswordHash, verified, user.CreatedAt,
	)
	return err
}

// GetByEmail returns a user by email.
func (s *UserStore) GetByEmail(email string) (*User, error) {
	var user User
	var verified int
	var createdAt string
	err := s.db.QueryRow(
		`SELECT id, email, password_hash, email_verified, created_at FROM users WHERE email = ?`,
		email,
	).Scan(&user.ID, &user.Email, &user.PasswordHash, &verified, &createdAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	user.EmailVerified = verified == 1
	user.CreatedAt = parseTime(createdAt)
	return &user, nil
}

// GetByID returns a user by ID.
func (s *UserStore) GetByID(id string) (*User, error) {
	var user User
	var verified int
	var createdAt string
	err := s.db.QueryRow(
		`SELECT id, email, password_hash, email_verified, created_at FROM users WHERE id = ?`,
		id,
	).Scan(&user.ID, &user.Email, &user.PasswordHash, &verified, &createdAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	user.EmailVerified = verified == 1
	user.CreatedAt = parseTime(createdAt)
	return &user, nil
}

// MarkVerified marks a user's email as verified.
func (s *UserStore) MarkVerified(id string) error {
	_, err := s.db.Exec(`UPDATE users SET email_verified = 1 WHERE id = ?`, id)
	return err
}

func parseTime(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	// SQLite default datetime format: 2006-01-02 15:04:05
	t, err := time.Parse("2006-01-02 15:04:05", s)
	if err == nil {
		return t
	}
	// Fallback to RFC3339
	t, err = time.Parse(time.RFC3339, s)
	if err == nil {
		return t
	}
	return time.Time{}
}

// Exists checks whether a user with the given email exists.
func (s *UserStore) Exists(email string) (bool, error) {
	var count int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM users WHERE email = ?`, email).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}
