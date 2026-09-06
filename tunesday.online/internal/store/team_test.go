package store

import (
	"testing"
	"time"

	"tunesday/tunesday.online/internal/db"
)

func newTestDB(t *testing.T) *db.DB {
	t.Helper()
	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	return database
}

func createUser(t *testing.T, database *db.DB, id, email string) *User {
	t.Helper()
	users := NewUserStore(database)
	u := &User{ID: id, Email: email, PasswordHash: "hash", EmailVerified: true, CreatedAt: time.Now()}
	if err := users.Create(u); err != nil {
		t.Fatalf("create user: %v", err)
	}
	return u
}

func TestGenerateSlug(t *testing.T) {
	database := newTestDB(t)
	teams := NewTeamStore(database)

	got, err := teams.GenerateSlug("USP Dev Team!")
	if err != nil {
		t.Fatalf("GenerateSlug: %v", err)
	}
	if got != "usp-dev-team" {
		t.Fatalf("expected usp-dev-team, got %s", got)
	}

	user := createUser(t, database, "u1", "u1@example.com")
	if err := teams.Create(&Team{ID: "t1", Name: "Other", Slug: "usp-dev-team", AdminID: user.ID}); err != nil {
		t.Fatalf("create colliding team: %v", err)
	}

	got, err = teams.GenerateSlug("USP Dev Team")
	if err != nil {
		t.Fatalf("GenerateSlug with collision: %v", err)
	}
	if got != "usp-dev-team-2" {
		t.Fatalf("expected usp-dev-team-2, got %s", got)
	}
}

func TestListByUser(t *testing.T) {
	database := newTestDB(t)
	teams := NewTeamStore(database)
	providers := NewProviderStore(database)
	members := NewTeamMemberStore(database)

	u1 := createUser(t, database, "u1", "u1@example.com")
	u2 := createUser(t, database, "u2", "u2@example.com")

	if err := teams.Create(&Team{ID: "ta", Name: "A", Slug: "a", AdminID: u1.ID}); err != nil {
		t.Fatal(err)
	}
	if err := teams.Create(&Team{ID: "tb", Name: "B", Slug: "b", AdminID: u1.ID}); err != nil {
		t.Fatal(err)
	}
	pa, err := providers.Create("ta", "Alain")
	if err != nil {
		t.Fatal(err)
	}
	pb, err := providers.Create("tb", "Armin")
	if err != nil {
		t.Fatal(err)
	}

	for _, m := range []*TeamMember{
		{TeamID: "ta", UserID: u1.ID, ProviderID: pa.ID, Role: "admin"},
		{TeamID: "tb", UserID: u1.ID, ProviderID: pb.ID, Role: "member"},
		{TeamID: "ta", UserID: u2.ID, ProviderID: pa.ID, Role: "member"},
	} {
		if err := members.Create(m); err != nil {
			t.Fatalf("create member: %v", err)
		}
	}

	teams1, err := teams.ListByUser(u1.ID)
	if err != nil {
		t.Fatalf("ListByUser: %v", err)
	}
	if len(teams1) != 2 {
		t.Fatalf("expected 2 teams for u1, got %d", len(teams1))
	}

	teams2, err := teams.ListByUser(u2.ID)
	if err != nil {
		t.Fatalf("ListByUser: %v", err)
	}
	if len(teams2) != 1 {
		t.Fatalf("expected 1 team for u2, got %d", len(teams2))
	}
}

func TestMemberAndProviderViews(t *testing.T) {
	database := newTestDB(t)
	teams := NewTeamStore(database)
	providers := NewProviderStore(database)
	members := NewTeamMemberStore(database)

	u := createUser(t, database, "u1", "u1@example.com")
	if err := teams.Create(&Team{ID: "ta", Name: "A", Slug: "a", AdminID: u.ID}); err != nil {
		t.Fatal(err)
	}
	lukas, err := providers.Create("ta", "Lukas")
	if err != nil {
		t.Fatal(err)
	}
	marcel, err := providers.Create("ta", "Marcel")
	if err != nil {
		t.Fatal(err)
	}
	if err := providers.SetDisabled(marcel.ID, true); err != nil {
		t.Fatal(err)
	}

	if err := members.Create(&TeamMember{TeamID: "ta", UserID: u.ID, ProviderID: lukas.ID, Role: "admin"}); err != nil {
		t.Fatal(err)
	}

	view, err := providers.ListByTeam("ta")
	if err != nil {
		t.Fatalf("ListByTeam: %v", err)
	}
	if len(view) != 2 {
		t.Fatalf("expected 2 providers, got %d", len(view))
	}
	if view[0].Name != "Lukas" || view[0].MemberEmail != "u1@example.com" {
		t.Fatalf("unexpected view: %+v", view[0])
	}
	if view[1].Name != "Marcel" || view[1].MemberEmail != "" {
		t.Fatalf("unexpected view: %+v", view[1])
	}

	// Only assigned + active providers are eligible.
	eligible, err := providers.ListEligibleByTeam("ta")
	if err != nil {
		t.Fatalf("ListEligibleByTeam: %v", err)
	}
	if len(eligible) != 1 || eligible[0].Name != "Lukas" {
		t.Fatalf("expected only Lukas eligible, got %v", providerNames(eligible))
	}

	// Magic token lookup.
	m, err := members.GetByMagicToken("nope")
	if err != nil || m != nil {
		t.Fatalf("expected no member for unknown token: %v", err)
	}
	all, err := members.ListByTeam("ta")
	if err != nil || len(all) != 1 {
		t.Fatalf("ListByTeam members: %v / %d", err, len(all))
	}
	byToken, err := members.GetByMagicToken(all[0].MagicToken)
	if err != nil || byToken == nil || byToken.UserID != "u1" {
		t.Fatalf("GetByMagicToken failed: %v", err)
	}

	// Count recalculation.
	if _, err := database.Exec(
		`INSERT INTO tunes (team_id, title, link, youtube_id, provider_id, added_at)
		 VALUES ('ta', 'x', 'https://youtu.be/aaaaaaaaaaa', 'aaaaaaaaaaa', ?, '2026-01-01 10:00:00')`,
		lukas.ID,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(
		`INSERT INTO tunes (team_id, title, link, youtube_id, provider_id, added_at)
		 VALUES ('ta', 'y', 'https://youtu.be/bbbbbbbbbbb', 'bbbbbbbbbbb', ?, '2026-02-01 10:00:00')`,
		lukas.ID,
	); err != nil {
		t.Fatal(err)
	}
	if err := providers.RecalculateCounts("ta"); err != nil {
		t.Fatal(err)
	}
	p, err := providers.GetByID(lukas.ID)
	if err != nil || p.TuneCount != 2 {
		t.Fatalf("expected tune_count 2, got %+v (%v)", p, err)
	}

	// Delete rules.
	if err := providers.Delete(lukas.ID); err == nil {
		t.Fatal("expected delete to fail for provider with tunes/member")
	}
	if err := providers.Delete(marcel.ID); err != nil {
		t.Fatalf("expected unassigned provider delete to work: %v", err)
	}
}

func providerNames(list []*Provider) []string {
	var out []string
	for _, p := range list {
		out = append(out, p.Name)
	}
	return out
}
