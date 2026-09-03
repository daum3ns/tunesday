package dataimport

import (
	"strings"
	"testing"
	"time"

	"tunesday/internal/core"
	"tunesday/tunesday.online/internal/db"
	"tunesday/tunesday.online/internal/store"
)

func setupDB(t *testing.T) *db.DB {
	t.Helper()
	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	if err := database.QueryRow(`SELECT 1`).Scan(new(int)); err != nil {
		t.Fatalf("db not ready: %v", err)
	}

	// Create an admin user to satisfy foreign keys.
	users := store.NewUserStore(database)
	if err := users.Create(&store.User{
		ID:            "admin-1",
		Email:         "admin@example.com",
		PasswordHash:  "hash",
		EmailVerified: true,
		CreatedAt:     time.Now(),
	}); err != nil {
		t.Fatalf("create admin: %v", err)
	}
	return database
}

func TestCreateTeamWithoutData(t *testing.T) {
	database := setupDB(t)

	res, err := CreateTeam(database, CreateTeamInput{
		AdminUserID:       "admin-1",
		TeamName:          "USP Dev",
		Slug:              "usp-dev",
		AdminProviderName: "Alain",
		Data:              nil,
	})
	if err != nil {
		t.Fatalf("CreateTeam: %v", err)
	}
	if res.TeamID == "" {
		t.Fatal("expected team ID")
	}

	teams := store.NewTeamStore(database)
	team, err := teams.GetBySlug("usp-dev")
	if err != nil || team == nil {
		t.Fatalf("team not found: %v", err)
	}
	if team.Name != "USP Dev" || team.AdminID != "admin-1" {
		t.Fatalf("unexpected team: %+v", team)
	}

	providers := store.NewProviderStore(database)
	p, err := providers.GetByName(team.ID, "Alain")
	if err != nil || p == nil {
		t.Fatalf("admin provider not created: %v", err)
	}
	if p.TuneCount != 0 {
		t.Fatalf("expected 0 tunes, got %d", p.TuneCount)
	}

	members := store.NewTeamMemberStore(database)
	m, err := members.Get(team.ID, "admin-1")
	if err != nil || m == nil {
		t.Fatalf("admin membership not created: %v", err)
	}
	if m.Role != "admin" {
		t.Fatalf("expected admin role, got %s", m.Role)
	}
	if m.MagicToken == "" {
		t.Fatal("expected magic token")
	}
}

func TestCreateTeamWithData(t *testing.T) {
	database := setupDB(t)

	data := &core.Data{
		Participants: map[string]int{
			"Alain":  99, // stale value, must be recalculated
			"Lukas":  0,
			"Marcel": 1,
			"Rolf":   99,
		},
		Disabled: map[string]bool{"Rolf": true},
		Tunes: []core.Tune{
			{Name: "Song A", Link: "https://www.youtube.com/watch?v=aaa1111111a", ID: "aaa1111111a", Provider: "Alain", AddedAt: time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)},
			{Name: "Song B", Link: "https://www.youtube.com/watch?v=bbb2222222b", ID: "bbb2222222b", Provider: "Alain", AddedAt: time.Date(2026, 2, 1, 10, 0, 0, 0, time.UTC)},
			{Name: "Song C", Link: "https://youtu.be/ccc3333333c", ID: "ccc3333333c", Provider: "Marcel", AddedAt: time.Date(2026, 3, 1, 10, 0, 0, 0, time.UTC)},
		},
	}

	res, err := CreateTeam(database, CreateTeamInput{
		AdminUserID:       "admin-1",
		TeamName:          "Import Team",
		Slug:              "import-team",
		AdminProviderName: "Alain",
		Data:              data,
	})
	if err != nil {
		t.Fatalf("CreateTeam: %v", err)
	}
	if res.TunesInserted != 3 {
		t.Fatalf("expected 3 tunes, got %d", res.TunesInserted)
	}

	providers := store.NewProviderStore(database)
	list, err := providers.ListByTeam(res.TeamID)
	if err != nil {
		t.Fatalf("list providers: %v", err)
	}
	if len(list) != 4 {
		t.Fatalf("expected 4 providers, got %d: %v", len(list), providerNames(list))
	}

	counts := map[string]int{}
	disabled := map[string]bool{}
	for _, p := range list {
		counts[p.Name] = p.TuneCount
		disabled[p.Name] = p.Disabled
	}
	if counts["Alain"] != 2 || counts["Marcel"] != 1 || counts["Lukas"] != 0 || counts["Rolf"] != 0 {
		t.Fatalf("unexpected counts: %v", counts)
	}
	if !disabled["Rolf"] || disabled["Alain"] {
		t.Fatalf("unexpected disabled flags: %v", disabled)
	}

	// Admin membership maps to the pre-existing Alain provider.
	members := store.NewTeamMemberStore(database)
	m, err := members.Get(res.TeamID, "admin-1")
	if err != nil || m == nil {
		t.Fatalf("admin membership missing: %v", err)
	}
	alain, err := providers.GetByName(res.TeamID, "Alain")
	if err != nil || alain == nil {
		t.Fatal("Alain provider missing")
	}
	if m.ProviderID != alain.ID {
		t.Fatalf("expected admin on Alain (%d), got %d", alain.ID, m.ProviderID)
	}

	tunes := store.NewTuneStore(database)
	last, err := tunes.LastSubmitterProvider(res.TeamID)
	if err != nil {
		t.Fatalf("last submitter: %v", err)
	}
	if last != "Marcel" {
		t.Fatalf("expected Marcel as latest submitter, got %s", last)
	}
}

func TestParseRejectsBadTunes(t *testing.T) {
	payload := `{"participants":{},"tunes":[{"name":"x","link":"y","provider":""}]}`
	_, err := Parse(strings.NewReader(payload))
	if err == nil || !strings.Contains(err.Error(), "missing provider") {
		t.Fatalf("expected missing provider error, got %v", err)
	}
}

func TestParseAcceptsMinimal(t *testing.T) {
	payload := `{"participants":{"a":1},"tunes":[{"name":"x","link":"https://www.youtube.com/watch?v=abcdefghij1","provider":"a"}]}`
	data, err := Parse(strings.NewReader(payload))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(data.Tunes) != 1 {
		t.Fatalf("expected 1 tune, got %d", len(data.Tunes))
	}
}

func providerNames(list []*store.ProviderView) []string {
	var out []string
	for _, p := range list {
		out = append(out, p.Name)
	}
	return out
}
