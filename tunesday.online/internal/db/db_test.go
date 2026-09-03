package db

import (
	"testing"
)

func TestOpen_AppliesMigrations(t *testing.T) {
	db, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer db.Close()

	// Verify a table from the initial migration exists.
	var name string
	err = db.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name='users'").Scan(&name)
	if err != nil {
		t.Fatalf("users table not found: %v", err)
	}
	if name != "users" {
		t.Fatalf("expected users table, got %s", name)
	}

	// Verify migration was recorded.
	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM schema_migrations").Scan(&count)
	if err != nil {
		t.Fatalf("schema_migrations query failed: %v", err)
	}
	if count == 0 {
		t.Fatalf("expected migrations to be recorded")
	}
}
