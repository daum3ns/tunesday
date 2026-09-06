package auth

import (
	"net/http"
	"time"

	"github.com/gorilla/sessions"
)

const (
	sessionName = "tunesday_session"
	userIDKey   = "user_id"
)

// SessionStore wraps gorilla/sessions store.
type SessionStore struct {
	store *sessions.CookieStore
}

// NewSessionStore creates a session store with the given cookie lifetime.
// A zero lifetime falls back to 30 days.
func NewSessionStore(secret []byte, secure bool, lifetime time.Duration) *SessionStore {
	if lifetime <= 0 {
		lifetime = 30 * 24 * time.Hour
	}
	store := sessions.NewCookieStore(secret)
	store.Options.Path = "/"
	store.Options.HttpOnly = true
	store.Options.Secure = secure
	store.Options.SameSite = http.SameSiteLaxMode
	store.Options.MaxAge = int(lifetime.Seconds())
	return &SessionStore{store: store}
}

// GetUserID returns the logged-in user's ID from the session, or empty.
func (s *SessionStore) GetUserID(r *http.Request) string {
	session, err := s.store.Get(r, sessionName)
	if err != nil {
		return ""
	}
	if id, ok := session.Values[userIDKey].(string); ok {
		return id
	}
	return ""
}

// SetUserID stores the user's ID in the session.
func (s *SessionStore) SetUserID(w http.ResponseWriter, r *http.Request, userID string) error {
	session, err := s.store.Get(r, sessionName)
	if err != nil {
		return err
	}
	session.Values[userIDKey] = userID
	return session.Save(r, w)
}

// Clear removes the user from the session.
func (s *SessionStore) Clear(w http.ResponseWriter, r *http.Request) error {
	session, err := s.store.Get(r, sessionName)
	if err != nil {
		return err
	}
	delete(session.Values, userIDKey)
	return session.Save(r, w)
}
