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

func TestWinCounts(t *testing.T) {
	database := newTestDB(t)
	teams := NewTeamStore(database)
	providers := NewProviderStore(database)
	cers := NewCeremonyStore(database)

	createUser(t, database, "u1", "u1@example.com")
	teams.Create(&Team{ID: "t1", Name: "T", Slug: "t", AdminID: "u1"})
	p1, _ := providers.Create("t1", "Lukas")
	p2, _ := providers.Create("t1", "Marcel")

	// Ceremony 1: Lukas wins.
	c1 := &Ceremony{TeamID: "t1", StartedBy: "u1", Pool: []string{"Lukas", "Marcel"}}
	cers.Create(c1)
	cers.RecordReveal(c1.ID, 1, []string{"Lukas", "Marcel"}, p1.ID)

	// Ceremony 2: Marcel wins.
	c2 := &Ceremony{TeamID: "t1", StartedBy: "u1", Pool: []string{"Lukas", "Marcel"}}
	cers.Create(c2)
	cers.RecordReveal(c2.ID, 2, []string{"Lukas", "Marcel"}, p2.ID)

	// Ceremony 3: Lukas wins again.
	c3 := &Ceremony{TeamID: "t1", StartedBy: "u1", Pool: []string{"Lukas", "Marcel"}}
	cers.Create(c3)
	cers.RecordReveal(c3.ID, 3, []string{"Lukas", "Marcel"}, p1.ID)

	wins, err := cers.WinCounts("t1")
	if err != nil {
		t.Fatalf("WinCounts: %v", err)
	}
	if len(wins) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(wins))
	}
	if wins[0].ProviderName != "Lukas" || wins[0].Wins != 2 || wins[0].TotalCeremonies != 3 {
		t.Fatalf("unexpected Lukas row: %+v", wins[0])
	}
	if wins[1].ProviderName != "Marcel" || wins[1].Wins != 1 || wins[1].TotalCeremonies != 3 {
		t.Fatalf("unexpected Marcel row: %+v", wins[1])
	}
}

func TestCeremonyStats(t *testing.T) {
	database := newTestDB(t)
	teams := NewTeamStore(database)
	providers := NewProviderStore(database)
	cers := NewCeremonyStore(database)

	createUser(t, database, "u1", "u1@example.com")
	createUser(t, database, "u2", "u2@example.com")
	teams.Create(&Team{ID: "t1", Name: "T", Slug: "t", AdminID: "u1"})
	p1, _ := providers.Create("t1", "Lukas")

	// Ceremony 1 with 2 attendees.
	c1 := &Ceremony{TeamID: "t1", StartedBy: "u1", Pool: []string{"Lukas"}}
	cers.Create(c1)
	cers.RecordReveal(c1.ID, 1, []string{"Lukas"}, p1.ID)
	cers.AddAttendee(c1.ID, "u1", "Alpha")
	cers.AddAttendee(c1.ID, "u2", "Bravo")

	// Ceremony 2 with 1 attendee.
	c2 := &Ceremony{TeamID: "t1", StartedBy: "u1", Pool: []string{"Lukas"}}
	cers.Create(c2)
	cers.RecordReveal(c2.ID, 2, []string{"Lukas"}, p1.ID)
	cers.AddAttendee(c2.ID, "u1", "Charlie")

	stats, err := cers.Stats("t1")
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}
	if stats.TotalCeremonies != 2 {
		t.Fatalf("expected 2 ceremonies, got %d", stats.TotalCeremonies)
	}
	if stats.AvgAttendance != 1.5 {
		t.Fatalf("expected avg 1.5, got %f", stats.AvgAttendance)
	}
}
