package store

import (
	"testing"
	"time"
)

func TestCeremonyHistory(t *testing.T) {
	database := newTestDB(t)
	teams := NewTeamStore(database)
	providers := NewProviderStore(database)
	cers := NewCeremonyStore(database)

	createUser(t, database, "u1", "u1@example.com")
	if err := teams.Create(&Team{ID: "t1", Name: "T", Slug: "t", AdminID: "u1"}); err != nil {
		t.Fatal(err)
	}
	lukas, err := providers.Create("t1", "Lukas")
	if err != nil {
		t.Fatal(err)
	}

	// Open ceremony.
	c1 := &Ceremony{TeamID: "t1", StartedBy: "u1", Seed: 1, Pool: []string{"Lukas"}}
	if err := cers.Create(c1); err != nil {
		t.Fatal(err)
	}

	hist, err := cers.ListRecentByTeam("t1", 5)
	if err != nil || len(hist) != 1 {
		t.Fatalf("history: %v / %d", err, len(hist))
	}
	if hist[0].Status != "open" || hist[0].WinnerName != "" {
		t.Fatalf("unexpected row: %+v", hist[0])
	}
	if hist[0].Token != c1.Token {
		t.Fatalf("token mismatch: %+v", hist[0])
	}

	// Reveal only.
	if err := cers.RecordReveal(c1.ID, 7, []string{"Lukas"}, lukas.ID); err != nil {
		t.Fatal(err)
	}
	hist, _ = cers.ListRecentByTeam("t1", 5)
	if hist[0].Status != "revealed" || hist[0].WinnerName != "Lukas" || hist[0].TuneTitle != "" {
		t.Fatalf("unexpected revealed row: %+v", hist[0])
	}

	// Completed with a linked tune.
	if _, err := database.Exec(
		`INSERT INTO tunes (team_id, title, link, youtube_id, provider_id, added_at)
		 VALUES ('t1', 'Song X', 'https://youtu.be/xxxxxxxxxxx', 'xxxxxxxxxxx', ?, ?)`,
		lukas.ID, formatTime(time.Now()),
	); err != nil {
		t.Fatal(err)
	}
	var tuneID int64
	if err := database.QueryRow(`SELECT id FROM tunes WHERE team_id = 't1'`).Scan(&tuneID); err != nil {
		t.Fatal(err)
	}
	if err := cers.MarkCompleted(c1.ID, tuneID); err != nil {
		t.Fatal(err)
	}
	hist, _ = cers.ListRecentByTeam("t1", 5)
	if hist[0].Status != "completed" || hist[0].TuneTitle != "Song X" {
		t.Fatalf("unexpected completed row: %+v", hist[0])
	}

	// Ordering: newest first, limit respected.
	c2 := &Ceremony{TeamID: "t1", StartedBy: "u1", Pool: []string{"Lukas"}}
	if err := cers.Create(c2); err != nil {
		t.Fatal(err)
	}
	// started_at is same-second; force ordering deterministically.
	if _, err := database.Exec(
		`UPDATE ceremonies SET started_at = '2099-01-01 00:00:00' WHERE id = ?`, c2.ID,
	); err != nil {
		t.Fatal(err)
	}
	hist, err = cers.ListRecentByTeam("t1", 5)
	if err != nil || len(hist) != 2 {
		t.Fatalf("expected 2 rows: %v / %d", err, len(hist))
	}
	if hist[0].Token != c2.Token {
		t.Fatal("expected newest ceremony first")
	}
	hist, _ = cers.ListRecentByTeam("t1", 1)
	if len(hist) != 1 {
		t.Fatalf("limit not respected: %d", len(hist))
	}

	// GetByToken surfaces the linked tune id.
	got, err := cers.GetByToken(c1.Token)
	if err != nil || got == nil || got.TuneID != tuneID {
		t.Fatalf("expected tune link on ceremony row: %+v (%v)", got, err)
	}
}
