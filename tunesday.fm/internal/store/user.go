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
		user.ID, user.Email, user.PasswordHash, verified, formatTime(user.CreatedAt),
	)
	return err
}

// GetByEmail returns a user by email.
func (s *UserStore) GetByEmail(email string) (*User, error) {
	return s.get(`SELECT id, email, password_hash, email_verified, created_at FROM users WHERE email = ?`, email)
}

// GetByID returns a user by ID.
func (s *UserStore) GetByID(id string) (*User, error) {
	return s.get(`SELECT id, email, password_hash, email_verified, created_at FROM users WHERE id = ?`, id)
}

func (s *UserStore) get(query string, arg string) (*User, error) {
	var user User
	var verified int
	var createdAt sql.NullString
	err := s.db.QueryRow(query, arg).Scan(&user.ID, &user.Email, &user.PasswordHash, &verified, &createdAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	user.EmailVerified = verified == 1
	if createdAt.Valid {
		user.CreatedAt = parseTime(createdAt.String)
	}
	return &user, nil
}

// GetOrCreateByEmail returns the user with this email, creating a verified
// passwordless account if none exists yet (used for invitation acceptance).
func (s *UserStore) GetOrCreateByEmail(email string) (*User, error) {
	user, err := s.GetByEmail(email)
	if err != nil || user != nil {
		return user, err
	}
	user = &User{
		ID:            newID(),
		Email:         email,
		PasswordHash:  "",
		EmailVerified: true,
		CreatedAt:     time.Now(),
	}
	if err := s.Create(user); err != nil {
		return nil, err
	}
	return user, nil
}

// MarkVerified marks a user's email as verified.
func (s *UserStore) MarkVerified(id string) error {
	_, err := s.db.Exec(`UPDATE users SET email_verified = 1 WHERE id = ?`, id)
	return err
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
