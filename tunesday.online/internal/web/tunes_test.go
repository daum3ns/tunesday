package web

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"tunesday/tunesday.online/internal/store"
)

// manualTuneSquad builds a two-provider team (admin "Owner" + member "Marcel")
// via the real routes and returns both sessions.
func manualTuneSquad(t *testing.T, h *Handler, fm *fakeMailer) (adminCookies, memberCookies []*http.Cookie) {
	t.Helper()
	adminCookies = loginAndVerify(t, h, fm, "owner@example.com", "password123")

	rr := createTeam(t, h, adminCookies, "Manual Tunes", "Owner", nil)
	if rr.Code != http.StatusSeeOther {
		t.Fatalf("team create failed: %d %s", rr.Code, rr.Body.String())
	}

	form := url.Values{}
	form.Set("email", "member@example.com")
	form.Set("provider_name", "Marcel")
	rr = doAuthed(t, h, adminCookies, http.MethodPost, "/teams/manual-tunes/members",
		map[string]string{"slug": "manual-tunes"},
		strings.NewReader(form.Encode()), "application/x-www-form-urlencoded", h.InviteMember)
	if rr.Code != http.StatusSeeOther {
		t.Fatalf("invite failed: %d %s", rr.Code, rr.Body.String())
	}

	token := fm.inviteTokenFor("member@example.com")
	if token == "" {
		t.Fatal("no invitation captured")
	}

	// Acceptance is a public route that creates the member's own session.
	req := httptest.NewRequest(http.MethodPost, "/invite/"+token, strings.NewReader(""))
	req = withParams(req, map[string]string{"token": token})
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr = httptest.NewRecorder()
	h.AcceptInvite(rr, req)
	if rr.Code != http.StatusSeeOther {
		t.Fatalf("accept failed: %d %s", rr.Code, rr.Body.String())
	}
	memberCookies = rr.Result().Cookies()
	return adminCookies, memberCookies
}

func TestManualAddTune(t *testing.T) {
	h, database, mailer := setupTestHandler(t)
	defer database.Close()

	fm := newFakeMailer()
	mailer.SendFunc = fm.capture()
	adminCookies, _ := manualTuneSquad(t, h, fm)

	team, _ := h.deps.Teams.GetBySlug("manual-tunes")
	if team == nil {
		t.Fatal("team missing")
	}
	owner, _ := h.deps.Providers.GetByName(team.ID, "Owner")
	if owner == nil {
		t.Fatal("owner provider missing")
	}

	postTune := func(link, providerID, addedOn string) *httptest.ResponseRecorder {
		t.Helper()
		form := url.Values{}
		if link != "" {
			form.Set("link", link)
		}
		if providerID != "" {
			form.Set("provider_id", providerID)
		}
		if addedOn != "" {
			form.Set("added_on", addedOn)
		}
		return doAuthed(t, h, adminCookies, http.MethodPost, "/teams/manual-tunes/tunes",
			map[string]string{"slug": team.Slug},
			strings.NewReader(form.Encode()), "application/x-www-form-urlencoded", h.ManualAddTune)
	}

	// Unsupported platform denied, nothing stored.
	rr := postTune("https://open.spotify.com/track/abc123", strconv.FormatInt(owner.ID, 10), "")
	loc := rr.Header().Get("Location")
	if !strings.Contains(loc, "Unsupported+link") {
		t.Fatalf("expected unsupported-link flash, got %s", loc)
	}
	tunes, _ := h.deps.Tunes.ListAllByTeam(team.ID)
	if len(tunes) != 0 {
		t.Fatalf("expected no tunes stored, got %d", len(tunes))
	}

	// Empty link denied with paste hint.
	rr = postTune("", strconv.FormatInt(owner.ID, 10), "")
	loc = rr.Header().Get("Location")
	if !strings.Contains(loc, "Paste+a") {
		t.Fatalf("expected paste hint flash, got %s", loc)
	}

	// Missing/invalid provider denied.
	rr = postTune("https://www.youtube.com/watch?v=dQw4w9WgXcQ", "", "")
	loc = rr.Header().Get("Location")
	if !strings.Contains(loc, "Invalid+provider") {
		t.Fatalf("expected invalid-provider flash, got %s", loc)
	}
	rr = postTune("https://www.youtube.com/watch?v=dQw4w9WgXcQ", "999999", "")
	loc = rr.Header().Get("Location")
	if !strings.Contains(loc, "Invalid+provider") {
		t.Fatalf("expected invalid-provider flash for bogus id, got %s", loc)
	}

	// Malformed date denied, nothing stored.
	rr = postTune("https://www.youtube.com/watch?v=dQw4w9WgXcQ", strconv.FormatInt(owner.ID, 10), "not-a-date")
	loc = rr.Header().Get("Location")
	if !strings.Contains(loc, "Invalid+date") {
		t.Fatalf("expected invalid-date flash, got %s", loc)
	}
	if tunes, _ := h.deps.Tunes.ListAllByTeam(team.ID); len(tunes) != 0 {
		t.Fatalf("expected no tunes stored after invalid date, got %d", len(tunes))
	}

	// A valid YouTube link is stored for the chosen provider.
	rr = postTune("https://www.youtube.com/watch?v=dQw4w9WgXcQ", strconv.FormatInt(owner.ID, 10), "")
	loc = rr.Header().Get("Location")
	if !strings.Contains(loc, "ok=") {
		t.Fatalf("expected success, got %s", loc)
	}
	tunes, _ = h.deps.Tunes.ListAllByTeam(team.ID)
	if len(tunes) != 1 {
		t.Fatalf("expected 1 stored tune, got %d", len(tunes))
	}
	got := tunes[0]
	if got.Platform != "youtube" {
		t.Fatalf("expected platform youtube, got %q", got.Platform)
	}
	if got.ProviderID != owner.ID {
		t.Fatalf("expected provider %d, got %d", owner.ID, got.ProviderID)
	}
	if got.Title == "" {
		t.Fatal("expected a fetched title")
	}
	if got.YouTubeID != "dQw4w9WgXcQ" {
		t.Fatalf("expected youtube id, got %q", got.YouTubeID)
	}
	owner, _ = h.deps.Providers.GetByID(owner.ID)
	if owner.TuneCount != 1 {
		t.Fatalf("expected provider tune_count 1, got %d", owner.TuneCount)
	}

	// SoundCloud link accepted too.
	rr = postTune("https://soundcloud.com/lofi-han/summer-haze", strconv.FormatInt(owner.ID, 10), "")
	loc = rr.Header().Get("Location")
	if !strings.Contains(loc, "ok=") {
		t.Fatalf("expected success for soundcloud, got %s", loc)
	}
	tunes, _ = h.deps.Tunes.ListAllByTeam(team.ID)
	if len(tunes) != 2 {
		t.Fatalf("expected 2 stored tunes, got %d", len(tunes))
	}
	sc := tunes[1]
	if sc.Platform != "soundcloud" {
		t.Fatalf("expected platform soundcloud, got %q", sc.Platform)
	}

	// A backdated tune lands on the exact date it was picked.
	rr = postTune("https://gasolinelollipops.bandcamp.com/track/love-is-free-single", strconv.FormatInt(owner.ID, 10), "2026-05-01")
	loc = rr.Header().Get("Location")
	if !strings.Contains(loc, "ok=") {
		t.Fatalf("expected success for backdated bandcamp, got %s", loc)
	}
	tunes, _ = h.deps.Tunes.ListAllByTeam(team.ID)
	if len(tunes) != 3 {
		t.Fatalf("expected 3 stored tunes, got %d", len(tunes))
	}
	var bc *store.TuneView
	for _, tu := range tunes {
		if tu.Platform == "bandcamp" {
			bc = tu
			break
		}
	}
	if bc == nil {
		t.Fatal("backdated bandcamp tune missing")
	}
	if got := bc.AddedAt.Format("2006-01-02"); got != "2026-05-01" {
		t.Fatalf("expected AddedAt 2026-05-01, got %s", got)
	}
}

func TestManualAddTuneMemberDenied(t *testing.T) {
	h, database, mailer := setupTestHandler(t)
	defer database.Close()

	fm := newFakeMailer()
	mailer.SendFunc = fm.capture()
	_, memberCookies := manualTuneSquad(t, h, fm)

	team, _ := h.deps.Teams.GetBySlug("manual-tunes")
	owner, _ := h.deps.Providers.GetByName(team.ID, "Owner")

	form := url.Values{}
	form.Set("link", "https://www.youtube.com/watch?v=dQw4w9WgXcQ")
	form.Set("provider_id", strconv.FormatInt(owner.ID, 10))
	rr := doAuthed(t, h, memberCookies, http.MethodPost, "/teams/manual-tunes/tunes",
		map[string]string{"slug": team.Slug},
		strings.NewReader(form.Encode()), "application/x-www-form-urlencoded", h.ManualAddTune)
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), "Admins only") {
		t.Fatalf("expected admin-only rejection, got %d: %s", rr.Code, rr.Body.String())
	}

	tunes, _ := h.deps.Tunes.ListAllByTeam(team.ID)
	if len(tunes) != 0 {
		t.Fatalf("expected no tune stored by member, got %d", len(tunes))
	}
}

func TestManualAddTuneLetsAdminAttributeToAnyTeamProvider(t *testing.T) {
	h, database, mailer := setupTestHandler(t)
	defer database.Close()

	fm := newFakeMailer()
	mailer.SendFunc = fm.capture()
	adminCookies, _ := manualTuneSquad(t, h, fm)

	team, _ := h.deps.Teams.GetBySlug("manual-tunes")
	marcel, _ := h.deps.Providers.GetByName(team.ID, "Marcel")
	if marcel == nil {
		t.Fatal("marcel provider missing")
	}

	form := url.Values{}
	form.Set("link", "https://music.youtube.com/watch?v=abc123")
	form.Set("provider_id", strconv.FormatInt(marcel.ID, 10))
	rr := doAuthed(t, h, adminCookies, http.MethodPost, "/teams/manual-tunes/tunes",
		map[string]string{"slug": team.Slug},
		strings.NewReader(form.Encode()), "application/x-www-form-urlencoded", h.ManualAddTune)
	if !strings.Contains(rr.Header().Get("Location"), "ok=") {
		t.Fatalf("expected success, got %s", rr.Header().Get("Location"))
	}

	tunes, _ := h.deps.Tunes.ListAllByTeam(team.ID)
	if len(tunes) != 1 || tunes[0].ProviderID != marcel.ID {
		t.Fatalf("expected tune attributed to Marcel, got %+v", tunes)
	}
}
