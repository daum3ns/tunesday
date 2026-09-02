package store

import "github.com/google/uuid"

// newID generates a random identifier for entities.
func newID() string {
	return uuid.NewString()
}

// NewToken generates a URL-safe random token for links.
func NewToken() string {
	return uuid.NewString()
}
