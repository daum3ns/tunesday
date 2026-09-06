package store

import (
	"testing"
	"time"

	"tunesday/tunesday.online/internal/db"
)

func TestVerificationTokenStore(t *testing.T) {
	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer database.Close()

	userStore := NewUserStore(database)
	if err := userStore.Create(&User{
		ID:            "user-id",
		Email:         "test@example.com",
		PasswordHash:  "hash",
		EmailVerified: false,
		CreatedAt:     time.Now(),
	}); err != nil {
		t.Fatalf("create user: %v", err)
	}

	store := NewVerificationTokenStore(database)

	token := &VerificationToken{
		ID:        "token-id",
		UserID:    "user-id",
		Token:     "secret-token",
		Used:      false,
		CreatedAt: time.Now(),
	}

	if err := store.Create(token); err != nil {
		t.Fatalf("create token: %v", err)
	}

	found, err := store.GetByToken("secret-token")
	if err != nil {
		t.Fatalf("get by token: %v", err)
	}
	if found == nil {
		t.Fatal("expected token to be found")
	}
	if found.Token != "secret-token" {
		t.Fatalf("expected token secret-token, got %s", found.Token)
	}

	if err := store.MarkUsed(found.ID); err != nil {
		t.Fatalf("mark used: %v", err)
	}

	used, err := store.GetByToken("secret-token")
	if err != nil {
		t.Fatalf("get used token: %v", err)
	}
	if used == nil || !used.Used {
		t.Fatal("expected token to be marked used")
	}
}
