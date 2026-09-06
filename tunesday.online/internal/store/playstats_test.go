package store

import (
	"testing"
	"time"

	"tunesday/tunesday.online/internal/db"
)

func insertTune(t *testing.T, database *db.DB, teamID string, title, ytID string, providerID int64) int64 {
	t.Helper()
	_, err := database.Exec(
		`INSERT INTO tunes (team_id, title, link, youtube_id, provider_id, added_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		teamID, title, "https://youtu.be/"+ytID, ytID, providerID, formatTime(time.Now()),
	)
	if err != nil {
		t.Fatalf("insert tune: %v", err)
	}
	var id int64
	if err := database.QueryRow(`SELECT id FROM tunes WHERE youtube_id = ?`, ytID).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func TestMostPlayed(t *testing.T) {
	database := newTestDB(t)
	teams := NewTeamStore(database)
	providers := NewProviderStore(database)
	ps := NewPlayStatStore(database)

	createUser(t, database, "u1", "u1@example.com")
	if err := teams.Create(&Team{ID: "t1", Name: "T", Slug: "t", AdminID: "u1"}); err != nil {
		t.Fatal(err)
	}
	p1, _ := providers.Create("t1", "Lukas")
	p2, _ := providers.Create("t1", "Marcel")

	// Insert tunes.
	t1 := insertTune(t, database, "t1", "Song A", "aaaaaaaaaaa", p1.ID)
	t2 := insertTune(t, database, "t1", "Song B", "bbbbbbbbbbb", p2.ID)

	// Record plays: Song A = 5, Song B = 2.
	for i := 0; i < 5; i++ {
		ps.Record(&PlayStat{TeamID: "t1", TuneID: t1, UserID: "u1", SessionID: "s", StartedAt: time.Now()})
	}
	for i := 0; i < 2; i++ {
		ps.Record(&PlayStat{TeamID: "t1", TuneID: t2, UserID: "u1", SessionID: "s", StartedAt: time.Now()})
	}

	// Most played all time.
	most, err := ps.MostPlayed("t1", nil, 10)
	if err != nil {
		t.Fatalf("MostPlayed: %v", err)
	}
	if len(most) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(most))
	}
	if most[0].Title != "Song A" || most[0].Count != 5 || most[0].ProviderName != "Lukas" {
		t.Fatalf("unexpected first row: %+v", most[0])
	}
	if most[1].Title != "Song B" || most[1].Count != 2 {
		t.Fatalf("unexpected second row: %+v", most[1])
	}

	// Most played with limit.
	limited, _ := ps.MostPlayed("t1", nil, 1)
	if len(limited) != 1 || limited[0].Title != "Song A" {
		t.Fatalf("limit failed: %+v", limited)
	}

	// Most played with time filter (future = no results).
	future := time.Now().Add(1 * time.Hour)
	filtered, _ := ps.MostPlayed("t1", &future, 10)
	if len(filtered) != 0 {
		t.Fatalf("expected 0 with future filter, got %d", len(filtered))
	}
}

func TestProviderPlayCounts(t *testing.T) {
	database := newTestDB(t)
	teams := NewTeamStore(database)
	providers := NewProviderStore(database)
	ps := NewPlayStatStore(database)

	createUser(t, database, "u1", "u1@example.com")
	teams.Create(&Team{ID: "t1", Name: "T", Slug: "t", AdminID: "u1"})
	p1, _ := providers.Create("t1", "Lukas")
	p2, _ := providers.Create("t1", "Marcel")

	t1 := insertTune(t, database, "t1", "A", "aaaaaaaaaaa", p1.ID)
	t2 := insertTune(t, database, "t1", "B", "bbbbbbbbbbb", p2.ID)

	for i := 0; i < 3; i++ {
		ps.Record(&PlayStat{TeamID: "t1", TuneID: t1, SessionID: "s", StartedAt: time.Now()})
	}
	for i := 0; i < 7; i++ {
		ps.Record(&PlayStat{TeamID: "t1", TuneID: t2, SessionID: "s", StartedAt: time.Now()})
	}

	counts, err := ps.ProviderPlayCounts("t1", nil)
	if err != nil {
		t.Fatalf("ProviderPlayCounts: %v", err)
	}
	if len(counts) != 2 {
		t.Fatalf("expected 2 providers, got %d", len(counts))
	}
	if counts[0].ProviderName != "Marcel" || counts[0].Count != 7 {
		t.Fatalf("expected Marcel 7, got %+v", counts[0])
	}
	if counts[1].ProviderName != "Lukas" || counts[1].Count != 3 {
		t.Fatalf("expected Lukas 3, got %+v", counts[1])
	}
}

func TestTotalPlays(t *testing.T) {
	database := newTestDB(t)
	teams := NewTeamStore(database)
	providers := NewProviderStore(database)
	ps := NewPlayStatStore(database)

	createUser(t, database, "u1", "u1@example.com")
	teams.Create(&Team{ID: "t1", Name: "T", Slug: "t", AdminID: "u1"})
	p, _ := providers.Create("t1", "Lukas")
	t1 := insertTune(t, database, "t1", "A", "aaaaaaaaaaa", p.ID)

	total, _ := ps.TotalPlays("t1")
	if total != 0 {
		t.Fatalf("expected 0, got %d", total)
	}

	ps.Record(&PlayStat{TeamID: "t1", TuneID: t1, SessionID: "s"})
	ps.Record(&PlayStat{TeamID: "t1", TuneID: t1, SessionID: "s"})
	ps.Record(&PlayStat{TeamID: "t1", TuneID: t1, SessionID: "s"})

	total, _ = ps.TotalPlays("t1")
	if total != 3 {
		t.Fatalf("expected 3, got %d", total)
	}
}
