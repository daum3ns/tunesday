package store

import (
	"testing"
	"time"

	"tunesday/tunesday.fm/internal/db"
)

func TestUserStore(t *testing.T) {
	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer database.Close()

	store := NewUserStore(database)

	user := &User{
		ID:            "user-1",
		Email:         "test@example.com",
		PasswordHash:  "hash",
		EmailVerified: false,
		CreatedAt:     time.Now(),
	}

	if err := store.Create(user); err != nil {
		t.Fatalf("create user: %v", err)
	}

	found, err := store.GetByEmail("test@example.com")
	if err != nil {
		t.Fatalf("get by email: %v", err)
	}
	if found == nil {
		t.Fatal("expected user to be found")
	}
	if found.Email != "test@example.com" {
		t.Fatalf("expected email test@example.com, got %s", found.Email)
	}

	missing, err := store.GetByEmail("missing@example.com")
	if err != nil {
		t.Fatalf("get missing user: %v", err)
	}
	if missing != nil {
		t.Fatal("expected missing user to be nil")
	}

	exists, err := store.Exists("test@example.com")
	if err != nil {
		t.Fatalf("exists: %v", err)
	}
	if !exists {
		t.Fatal("expected user to exist")
	}

	if err := store.MarkVerified("user-1"); err != nil {
		t.Fatalf("mark verified: %v", err)
	}

	verified, err := store.GetByID("user-1")
	if err != nil {
		t.Fatalf("get by id: %v", err)
	}
	if verified == nil || !verified.EmailVerified {
		t.Fatal("expected user to be verified")
	}
}
