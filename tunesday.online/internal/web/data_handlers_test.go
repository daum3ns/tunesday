package web

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"tunesday/internal/core"
)

func TestExportImportReplaceFlow(t *testing.T) {
	h, database, mailer := setupTestHandler(t)
	defer database.Close()

	fm := newFakeMailer()
	mailer.SendFunc = fm.capture()
	cookies := loginAndVerify(t, h, fm, "data@example.com", "password123")

	payload := []byte(`{
      "participants": {"Alain": 1, "Rolf": 0},
      "tunes": [
        {"name": "Original", "link": "https://youtu.be/aaaaaaaaaaa", "id": "aaaaaaaaaaa", "provider": "Alain", "added_at": "2026-01-01T10:00:00Z"}
      ]
    }`)
	rr := createTeam(t, h, cookies, "Data Team", "Alain", payload)
	if rr.Code != http.StatusSeeOther {
		t.Fatalf("create team failed: %d", rr.Code)
	}
	team, _ := h.deps.Teams.GetBySlug("data-team")

	// Export.
	rr = doAuthed(t, h, cookies, http.MethodGet, "/teams/data-team/export",
		map[string]string{"slug": "data-team"}, nil, "", h.ExportTeam)
	if rr.Code != http.StatusOK {
		t.Fatalf("export status %d", rr.Code)
	}
	if cd := rr.Header().Get("Content-Disposition"); !strings.Contains(cd, "tunesday-data-team.json") {
		t.Fatalf("bad content disposition: %s", cd)
	}
	var exported core.Data
	if err := json.Unmarshal(rr.Body.Bytes(), &exported); err != nil {
		t.Fatalf("exported json invalid: %v", err)
	}
	if exported.Participants["Alain"] != 1 || len(exported.Tunes) != 1 {
		t.Fatalf("unexpected export: %+v", exported)
	}

	// Import preview shows the destructive warning.
	replacement := []byte(`{
      "participants": {"Newcomer": 0},
      "tunes": [
        {"name": "Fresh", "link": "https://youtu.be/bbbbbbbbbbb", "id": "bbbbbbbbbbb", "provider": "Newcomer", "added_at": "2026-06-01T10:00:00Z"},
        {"name": "Second", "link": "https://youtu.be/ccccccccccc", "id": "ccccccccccc", "provider": "Newcomer", "added_at": "2026-06-02T10:00:00Z"}
      ]
    }`)
	body, ct := multipartBody(map[string]string{}, "tunesday.json", replacement)
	rr = doAuthed(t, h, cookies, http.MethodPost, "/teams/data-team/import",
		map[string]string{"slug": "data-team"}, body, ct, h.ImportPreview)
	if rr.Code != http.StatusOK {
		t.Fatalf("preview status %d: %s", rr.Code, rr.Body.String())
	}
	page := rr.Body.String()
	if !strings.Contains(page, "Confirm: replace all team data") {
		t.Fatal("missing warning headline")
	}
	if !strings.Contains(page, "1 deleted") || !strings.Contains(page, "2 imported") {
		t.Fatalf("warning lacks tune counts: %s", page)
	}
	if !strings.Contains(page, "Rolf") {
		t.Fatalf("warning should list Rolf as removed: %s", page)
	}
	// Alain is kept because the admin is assigned to him.
	if !strings.Contains(page, "Alain") || !strings.Contains(page, "kept despite") {
		t.Fatalf("warning should mention kept Alain: %s", page)
	}

	// Confirm the replace.
	confirm := url.Values{}
	confirm.Set("payload_b64", base64.StdEncoding.EncodeToString(replacement))
	rr = doAuthed(t, h, cookies, http.MethodPost, "/teams/data-team/import/confirm",
		map[string]string{"slug": "data-team"},
		strings.NewReader(confirm.Encode()), "application/x-www-form-urlencoded", h.ImportConfirm)
	if rr.Code != http.StatusSeeOther {
		t.Fatalf("confirm status %d: %s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Header().Get("Location"), "ok=Data+replaced") {
		t.Fatalf("expected success flash, got %s", rr.Header().Get("Location"))
	}

	tunes, _ := h.deps.Tunes.CountByTeam(team.ID)
	if tunes != 2 {
		t.Fatalf("expected 2 tunes after replace, got %d", tunes)
	}
	rolf, _ := h.deps.Providers.GetByName(team.ID, "Rolf")
	if rolf != nil {
		t.Fatal("Rolf should be gone after replace")
	}
	newcomer, _ := h.deps.Providers.GetByName(team.ID, "Newcomer")
	if newcomer == nil || newcomer.TuneCount != 2 {
		t.Fatalf("expected Newcomer with 2 tunes, got %+v", newcomer)
	}
	// Admin's membership survived on the kept Alain provider.
	members, _ := h.deps.Members.ListByTeam(team.ID)
	if len(members) != 1 || members[0].ProviderName != "Alain" {
		t.Fatalf("admin membership should survive on Alain, got %+v", members)
	}
}

func TestImportConfirmRejectsCorruptedAndForeign(t *testing.T) {
	h, database, mailer := setupTestHandler(t)
	defer database.Close()

	fm := newFakeMailer()
	mailer.SendFunc = fm.capture()
	cookies := loginAndVerify(t, h, fm, "x@example.com", "password123")

	rr := createTeam(t, h, cookies, "Guard Team", "Guard", nil)
	if rr.Code != http.StatusSeeOther {
		t.Fatalf("create failed %d", rr.Code)
	}

	// Corrupted base64 is rejected.
	form := url.Values{}
	form.Set("payload_b64", "!!!not base64!!!")
	rr = doAuthed(t, h, cookies, http.MethodPost, "/teams/guard-team/import/confirm",
		map[string]string{"slug": "guard-team"},
		strings.NewReader(form.Encode()), "application/x-www-form-urlencoded", h.ImportConfirm)
	if rr.Code != http.StatusSeeOther || !strings.Contains(rr.Header().Get("Location"), "err=") {
		t.Fatalf("expected error flash for bad base64, got %d %s", rr.Code, rr.Header().Get("Location"))
	}

	// Valid base64 of garbage JSON is rejected.
	form = url.Values{}
	form.Set("payload_b64", base64.StdEncoding.EncodeToString([]byte(`{"participants":`)))
	rr = doAuthed(t, h, cookies, http.MethodPost, "/teams/guard-team/import/confirm",
		map[string]string{"slug": "guard-team"},
		strings.NewReader(form.Encode()), "application/x-www-form-urlencoded", h.ImportConfirm)
	if rr.Code != http.StatusSeeOther || !strings.Contains(rr.Header().Get("Location"), "err=") {
		t.Fatalf("expected error flash for bad json, got %d", rr.Code)
	}

	// A plain member cannot reach import at all.
	invite := url.Values{}
	invite.Set("email", "viewer@example.com")
	invite.Set("provider_name", "Viewer")
	rr = doAuthed(t, h, cookies, http.MethodPost, "/teams/guard-team/members",
		map[string]string{"slug": "guard-team"},
		strings.NewReader(invite.Encode()), "application/x-www-form-urlencoded", h.InviteMember)
	if rr.Code != http.StatusSeeOther {
		t.Fatalf("invite failed: %d", rr.Code)
	}
	memberCookies := acceptInviteAnonymously(t, h, fm.inviteTokenFor("viewer@example.com"))

	form = url.Values{}
	form.Set("payload_b64", base64.StdEncoding.EncodeToString([]byte(`{}`)))
	rr = doAuthed(t, h, memberCookies, http.MethodPost, "/teams/guard-team/import/confirm",
		map[string]string{"slug": "guard-team"},
		strings.NewReader(form.Encode()), "application/x-www-form-urlencoded", h.ImportConfirm)
	if !strings.Contains(rr.Body.String(), "Admins only") {
		t.Fatalf("member should be rejected from import: %s", rr.Body.String())
	}
}

// acceptInviteAnonymously accepts an invitation without a session and returns
// the cookies the member receives.
func acceptInviteAnonymously(t *testing.T, h *Handler, token string) []*http.Cookie {
	t.Helper()
	if token == "" {
		t.Fatal("no invite token captured")
	}
	req := httptest.NewRequest(http.MethodPost, "/invite/"+token, strings.NewReader(""))
	req = withParams(req, map[string]string{"token": token})
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	h.AcceptInvite(rr, req)
	if rr.Code != http.StatusSeeOther {
		t.Fatalf("accept failed: %d %s", rr.Code, rr.Body.String())
	}
	return rr.Result().Cookies()
}
