package dataimport

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"

	"tunesday/internal/core"
	"tunesday/tunesday.online/internal/store"
)

func TestReplaceTeamSwapsDataKeepsStructure(t *testing.T) {
	database := setupDB(t)

	initial := &core.Data{
		Participants: map[string]int{"Alain": 0, "Rolf": 0, "Hussein": 0},
		Tunes: []core.Tune{
			{Name: "old song", Link: "https://youtu.be/aaaaaaaaaaa", ID: "aaaaaaaaaaa", Provider: "Rolf", AddedAt: time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)},
		},
	}
	created, err := CreateTeam(database, CreateTeamInput{
		AdminUserID:       "admin-1",
		TeamName:          "Swap Team",
		Slug:              "swap-team",
		AdminProviderName: "Alain",
		Data:              initial,
	})
	if err != nil {
		t.Fatalf("CreateTeam: %v", err)
	}
	teamID := created.TeamID

	users := store.NewUserStore(database)
	providers := store.NewProviderStore(database)
	members := store.NewTeamMemberStore(database)
	cers := store.NewCeremonyStore(database)
	invites := store.NewInvitationStore(database)

	// Hussein gets a member, so a replace that drops him must keep the provider.
	husseinUser := &store.User{ID: "h1", Email: "h@example.com", PasswordHash: "x", CreatedAt: time.Now()}
	if err := users.Create(husseinUser); err != nil {
		t.Fatal(err)
	}
	hussein, err := providers.GetByName(teamID, "Hussein")
	if err != nil || hussein == nil {
		t.Fatal("Hussein provider missing")
	}
	if err := members.Create(&store.TeamMember{
		TeamID: teamID, UserID: husseinUser.ID, ProviderID: hussein.ID, Role: "member",
	}); err != nil {
		t.Fatal(err)
	}

	// Rolf will vanish from the new file; pin references to his provider first.
	rolf, err := providers.GetByName(teamID, "Rolf")
	if err != nil || rolf == nil {
		t.Fatal("Rolf provider missing")
	}
	cer := &store.Ceremony{
		TeamID: teamID, StartedBy: "admin-1", Seed: 42, Pool: []string{"Rolf"},
	}
	if err := cers.Create(cer); err != nil {
		t.Fatal(err)
	}
	if err := cers.RecordReveal(cer.ID, 42, []string{"Rolf"}, rolf.ID); err != nil {
		t.Fatal(err)
	}
	if err := invites.Create(&store.Invitation{
		TeamID: teamID, Email: "ghost@example.com", ProviderID: rolf.ID,
	}); err != nil {
		t.Fatal(err)
	}

	// Replace: no Rolf, no Hussein in the file; Marcel added and disabled.
	fresh := &core.Data{
		Participants: map[string]int{"Alain": 5, "Marcel": 0},
		Disabled:     map[string]bool{"Marcel": true},
		Tunes: []core.Tune{
			{Name: "new song A", Link: "https://youtu.be/bbbbbbbbbbb", ID: "bbbbbbbbbbb", Provider: "Alain", AddedAt: time.Date(2026, 5, 1, 10, 0, 0, 0, time.UTC)},
			{Name: "new song B", Link: "https://youtu.be/ccccccccccc", ID: "ccccccccccc", Provider: "Alain", AddedAt: time.Date(2026, 5, 8, 10, 0, 0, 0, time.UTC)},
		},
	}

	res, err := ReplaceTeam(database, teamID, fresh)
	if err != nil {
		t.Fatalf("ReplaceTeam: %v", err)
	}

	if res.TunesDeleted != 1 {
		t.Fatalf("expected 1 deleted tune, got %d", res.TunesDeleted)
	}
	if res.TunesImported != 2 {
		t.Fatalf("expected 2 imported tunes, got %d", res.TunesImported)
	}
	if len(res.ProvidersRemoved) != 1 || res.ProvidersRemoved[0] != "Rolf" {
		t.Fatalf("expected Rolf removed, got %v", res.ProvidersRemoved)
	}
	if len(res.ProvidersKept) != 1 || res.ProvidersKept[0] != "Hussein" {
		t.Fatalf("expected Hussein kept, got %v", res.ProvidersKept)
	}
	if res.InvitesRevoked != 1 {
		t.Fatalf("expected 1 invitation revoked, got %d", res.InvitesRevoked)
	}

	// Flags and counts follow the file.
	marcel, _ := providers.GetByName(teamID, "Marcel")
	if marcel == nil || !marcel.Disabled {
		t.Fatalf("expected disabled Marcel, got %+v", marcel)
	}
	alain, _ := providers.GetByName(teamID, "Alain")
	if alain == nil || alain.TuneCount != 2 {
		t.Fatalf("expected Alain count 2, got %+v", alain)
	}
	gone, _ := providers.GetByName(teamID, "Rolf")
	if gone != nil {
		t.Fatal("expected Rolf provider deleted")
	}

	// Structure survived: ceremony keeps its audit trail (pool+seed), winner cleared.
	afterCer, err := cers.GetByToken(cer.Token)
	if err != nil || afterCer == nil {
		t.Fatal("ceremony vanished after replace")
	}
	if afterCer.WinnerProviderID != 0 || !afterCer.Revealed() {
		t.Fatalf("expected winner cleared but revealed_at kept, got %+v", afterCer)
	}
	if len(afterCer.Pool) != 1 || afterCer.Pool[0] != "Rolf" {
		t.Fatalf("expected pool preserved, got %v", afterCer.Pool)
	}

	// Hussein's membership untouched.
	allMembers, _ := members.ListByTeam(teamID)
	if len(allMembers) != 2 {
		t.Fatalf("expected 2 members after replace, got %d", len(allMembers))
	}
}

func TestBuildExportRoundTrip(t *testing.T) {
	database := setupDB(t)

	data := &core.Data{
		Participants: map[string]int{"Alain": 99, "Lukas": 0, "Rolf": 99},
		Disabled:     map[string]bool{"Rolf": true},
		Tunes: []core.Tune{
			{Name: "A", Link: "https://youtu.be/aaaaaaaaaaa", ID: "aaaaaaaaaaa", Provider: "Alain", AddedAt: time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)},
			{Name: "B", Link: "https://youtu.be/bbbbbbbbbbb", ID: "bbbbbbbbbbb", Provider: "Alain", AddedAt: time.Date(2026, 1, 8, 10, 0, 0, 0, time.UTC)},
		},
	}
	created, err := CreateTeam(database, CreateTeamInput{
		AdminUserID: "admin-1", TeamName: "RT", Slug: "rt",
		AdminProviderName: "Alain", Data: data,
	})
	if err != nil {
		t.Fatal(err)
	}

	providers, _ := store.NewProviderStore(database).ListByTeam(created.TeamID)
	tunes, _ := store.NewTuneStore(database).ListAllByTeam(created.TeamID)
	exported := BuildExport(providers, tunes)

	if exported.Participants["Alain"] != 2 {
		t.Fatalf("expected recalculated count 2 for Alain, got %d", exported.Participants["Alain"])
	}
	if _, ok := exported.Participants["Lukas"]; !ok {
		t.Fatal("expected Lukas present with 0")
	}
	if !exported.Disabled["Rolf"] {
		t.Fatal("expected Rolf disabled in export")
	}
	if len(exported.Tunes) != 2 || exported.Tunes[0].Name != "A" {
		t.Fatalf("unexpected exported tunes: %+v", exported.Tunes)
	}

	// The export must parse cleanly back through Parse.
	raw, err := json.Marshal(exported)
	if err != nil {
		t.Fatal(err)
	}
	reimported, err := Parse(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("re-parse export: %v", err)
	}
	if len(reimported.Tunes) != 2 {
		t.Fatalf("expected 2 tunes after round-trip, got %d", len(reimported.Tunes))
	}
}
