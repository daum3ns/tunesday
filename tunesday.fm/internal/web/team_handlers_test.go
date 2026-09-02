package web

import (
	"bytes"
	"context"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"tunesday/tunesday.fm/internal/auth"
)

// withParams injects a chi RouteContext so handlers can read URL params.
func withParams(r *http.Request, params map[string]string) *http.Request {
	rc := chi.NewRouteContext()
	for k, v := range params {
		rc.URLParams.Add(k, v)
	}
	return r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rc))
}

type inviteRecord struct {
	to  string
	url string
}

type fakeMailer struct {
	verifications map[string]string
	invites       map[string]*inviteRecord
}

func newFakeMailer() *fakeMailer {
	return &fakeMailer{
		verifications: map[string]string{},
		invites:       map[string]*inviteRecord{},
	}
}

func (f *fakeMailer) capture() func(to, subject, body string) error {
	return func(to, subject, body string) error {
		if strings.HasPrefix(subject, "Verify") {
			if idx := strings.Index(body, "?token="); idx != -1 {
				rest := body[idx+7:]
				if end := strings.IndexAny(rest, " \t\r\n"); end != -1 {
					rest = rest[:end]
				}
				f.verifications[to] = rest
			}
			return nil
		}
		if strings.HasPrefix(subject, "You've been invited") {
			u := ""
			for _, line := range strings.Split(body, "\n") {
				line = strings.TrimSpace(line)
				if strings.HasPrefix(line, "http") {
					u = line
					break
				}
			}
			f.invites[to] = &inviteRecord{to: to, url: u}
		}
		return nil
	}
}

func (f *fakeMailer) inviteTokenFor(email string) string {
	rec := f.invites[email]
	if rec == nil {
		return ""
	}
	idx := strings.LastIndex(rec.url, "/invite/")
	if idx == -1 {
		return ""
	}
	return strings.TrimSpace(rec.url[idx+len("/invite/"):])
}

// loginAndVerify registers a user, verifies the email via the captured link,
// logs in, and returns the session cookies.
func loginAndVerify(t *testing.T, h *Handler, fm *fakeMailer, emailAddr, password string) []*http.Cookie {
	t.Helper()

	form := url.Values{}
	form.Set("email", emailAddr)
	form.Set("password", password)
	form.Set("password_confirm", password)

	req := httptest.NewRequest(http.MethodPost, "/register", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	h.Register(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("register failed: %s", rr.Body.String())
	}

	token := fm.verifications[emailAddr]
	if token == "" {
		t.Fatal("no verification token captured")
	}

	req = httptest.NewRequest(http.MethodGet, "/verify", nil)
	q := req.URL.Query()
	q.Set("token", token)
	req.URL.RawQuery = q.Encode()
	rr = httptest.NewRecorder()
	h.Verify(rr, req)
	if !strings.Contains(rr.Body.String(), "verified") {
		t.Fatalf("verify failed: %s", rr.Body.String())
	}

	lf := url.Values{}
	lf.Set("email", emailAddr)
	lf.Set("password", password)

	req = httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(lf.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr = httptest.NewRecorder()
	h.Login(rr, req)
	if rr.Code != http.StatusSeeOther {
		t.Fatalf("login failed: %d %s", rr.Code, rr.Body.String())
	}
	return rr.Result().Cookies()
}

func multipartBody(fields map[string]string, fileName string, fileContent []byte) (io.Reader, string) {
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	for k, v := range fields {
		_ = w.WriteField(k, v)
	}
	if fileName != "" {
		fw, _ := w.CreateFormFile("tunesday_json", fileName)
		_, _ = fw.Write(fileContent)
	}
	_ = w.Close()
	return &buf, w.FormDataContentType()
}

func doAuthed(t *testing.T, h *Handler, cookies []*http.Cookie, method, path string, params map[string]string, body io.Reader, contentType string, handler http.HandlerFunc) *httptest.ResponseRecorder {
	t.Helper()
	var req *http.Request
	if body == nil {
		req = httptest.NewRequest(method, path, nil)
	} else {
		req = httptest.NewRequest(method, path, body)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	req = withParams(req, params)
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rr := httptest.NewRecorder()
	authed := auth.Middleware(h.deps.Sessions, h.deps.Users)(handler)
	authed.ServeHTTP(rr, req)
	return rr
}

func createTeam(t *testing.T, h *Handler, cookies []*http.Cookie, teamName, yourName string, payload []byte) *httptest.ResponseRecorder {
	t.Helper()
	fields := map[string]string{"team_name": teamName, "your_name": yourName}
	var fileName string
	if payload != nil {
		fileName = "tunesday.json"
	}
	body, ct := multipartBody(fields, fileName, payload)
	return doAuthed(t, h, cookies, http.MethodPost, "/teams", nil, body, ct, h.CreateTeam)
}

func TestTeamCreationAndDashboardFlow(t *testing.T) {
	h, database, mailer := setupTestHandler(t)
	defer database.Close()

	fm := newFakeMailer()
	mailer.SendFunc = fm.capture()

	cookies := loginAndVerify(t, h, fm, "admin@example.com", "password123")

	rr := createTeam(t, h, cookies, "USP Dev Team", "Alain", nil)
	if rr.Code != http.StatusSeeOther {
		t.Fatalf("expected redirect, got %d: %s", rr.Code, rr.Body.String())
	}
	loc := rr.Header().Get("Location")
	if !strings.HasPrefix(loc, "/teams/usp-dev-team/dashboard") {
		t.Fatalf("unexpected redirect %s", loc)
	}

	rr = doAuthed(t, h, cookies, http.MethodGet, "/teams/usp-dev-team/dashboard",
		map[string]string{"slug": "usp-dev-team"}, nil, "", h.Dashboard)
	if rr.Code != http.StatusOK {
		t.Fatalf("dashboard failed: %d %s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "USP Dev Team") {
		t.Fatal("dashboard missing team name")
	}

	rr = doAuthed(t, h, cookies, http.MethodGet, "/teams/usp-dev-team/providers",
		map[string]string{"slug": "usp-dev-team"}, nil, "", h.ProvidersPage)
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), "Alain") {
		t.Fatalf("providers page failed: %d", rr.Code)
	}

	rr = doAuthed(t, h, cookies, http.MethodGet, "/teams/usp-dev-team/members",
		map[string]string{"slug": "usp-dev-team"}, nil, "", h.MembersPage)
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), "admin@example.com") {
		t.Fatalf("members page failed: %d", rr.Code)
	}

	rr = doAuthed(t, h, cookies, http.MethodGet, "/onboarding",
		nil, nil, "", h.Onboarding)
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), "USP Dev Team") {
		t.Fatalf("onboarding page failed: %d", rr.Code)
	}
}

func TestTeamCreationWithJSONImport(t *testing.T) {
	h, database, mailer := setupTestHandler(t)
	defer database.Close()

	fm := newFakeMailer()
	mailer.SendFunc = fm.capture()
	cookies := loginAndVerify(t, h, fm, "admin2@example.com", "password123")

	payload := []byte(`{
      "participants": {"Alain": 7, "Lukas": 4},
      "disabled": {"Lukas": true},
      "tunes": [
        {"name": "Song A", "link": "https://www.youtube.com/watch?v=aaa1111111a", "id": "aaa1111111a", "provider": "Alain", "added_at": "2026-01-01T10:00:00Z"}
      ]
    }`)

	rr := createTeam(t, h, cookies, "Import Squad", "Alain", payload)
	if rr.Code != http.StatusSeeOther {
		t.Fatalf("expected redirect, got %d: %s", rr.Code, rr.Body.String())
	}

	team, _ := h.deps.Teams.GetBySlug("import-squad")
	if team == nil {
		t.Fatal("team missing")
	}

	providers, err := h.deps.Providers.ListByTeam(team.ID)
	if err != nil {
		t.Fatalf("providers: %v", err)
	}
	if len(providers) != 2 {
		t.Fatalf("expected 2 providers, got %d", len(providers))
	}

	members, _ := h.deps.Members.ListByTeam(team.ID)
	if len(members) != 1 || members[0].ProviderName != "Alain" {
		t.Fatalf("expected admin assigned to Alain, got %+v", members)
	}

	eligible, _ := h.deps.Providers.ListEligibleByTeam(team.ID)
	if len(eligible) != 1 || eligible[0].Name != "Alain" {
		t.Fatalf("expected only Alain eligible (Lukas disabled + unassigned), got %+v", eligible)
	}
}

func TestInviteAcceptAndMagicLink(t *testing.T) {
	h, database, mailer := setupTestHandler(t)
	defer database.Close()

	fm := newFakeMailer()
	mailer.SendFunc = fm.capture()
	adminCookies := loginAndVerify(t, h, fm, "owner@example.com", "password123")

	rr := createTeam(t, h, adminCookies, "Rhythm Rebels", "Owner", nil)
	if rr.Code != http.StatusSeeOther {
		t.Fatalf("team create failed: %d %s", rr.Code, rr.Body.String())
	}

	team, _ := h.deps.Teams.GetBySlug("rhythm-rebels")
	if team == nil {
		t.Fatal("team missing")
	}

	// Admin invites a member with a provider assigned.
	form := url.Values{}
	form.Set("email", "member@example.com")
	form.Set("provider_name", "Marcel")

	rr = doAuthed(t, h, adminCookies, http.MethodPost, "/teams/rhythm-rebels/members",
		map[string]string{"slug": "rhythm-rebels"},
		strings.NewReader(form.Encode()), "application/x-www-form-urlencoded", h.InviteMember)
	if rr.Code != http.StatusSeeOther {
		t.Fatalf("invite failed: %d %s", rr.Code, rr.Body.String())
	}

	token := fm.inviteTokenFor("member@example.com")
	if token == "" {
		t.Fatalf("no invitation captured: %+v", fm.invites)
	}

	// Member views the invitation (no session) and accepts.
	req := httptest.NewRequest(http.MethodGet, "/invite/"+token, nil)
	req = withParams(req, map[string]string{"token": token})
	rr = httptest.NewRecorder()
	h.AcceptInvitePage(rr, req)
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), "Marcel") {
		t.Fatalf("accept page missing provider: %d %s", rr.Code, rr.Body.String())
	}

	req = httptest.NewRequest(http.MethodPost, "/invite/"+token, strings.NewReader(""))
	req = withParams(req, map[string]string{"token": token})
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr = httptest.NewRecorder()
	h.AcceptInvite(rr, req)
	if rr.Code != http.StatusSeeOther {
		t.Fatalf("accept failed: %d %s", rr.Code, rr.Body.String())
	}
	memberCookies := rr.Result().Cookies()
	if len(memberCookies) == 0 {
		t.Fatal("expected session cookie after accepting invitation")
	}

	user, _ := h.deps.Users.GetByEmail("member@example.com")
	if user == nil {
		t.Fatal("member user not created")
	}
	member, _ := h.deps.Members.Get(team.ID, user.ID)
	if member == nil {
		t.Fatal("member membership not created")
	}

	// Magic link login.
	members, _ := h.deps.Members.ListByTeam(team.ID)
	var magicToken string
	for _, m := range members {
		if m.Email == "member@example.com" {
			magicToken = m.MagicToken
		}
	}
	if magicToken == "" {
		t.Fatal("no magic token found")
	}

	req = httptest.NewRequest(http.MethodGet, "/join/"+magicToken, nil)
	req = withParams(req, map[string]string{"token": magicToken})
	rr = httptest.NewRecorder()
	h.JoinMagicLink(rr, req)
	if rr.Code != http.StatusSeeOther {
		t.Fatalf("magic link failed: %d %s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Header().Get("Location"), "rhythm-rebels") {
		t.Fatalf("magic link should redirect to the team, got %s", rr.Header().Get("Location"))
	}

	// Member (not admin) cannot invite others.
	form = url.Values{}
	form.Set("email", "someone@example.com")
	rr = doAuthed(t, h, memberCookies, http.MethodPost, "/teams/rhythm-rebels/members",
		map[string]string{"slug": "rhythm-rebels"},
		strings.NewReader(form.Encode()), "application/x-www-form-urlencoded", h.InviteMember)
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), "Admins only") {
		t.Fatalf("expected admin-only rejection, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestInviteMemberChoosesProvider(t *testing.T) {
	h, database, mailer := setupTestHandler(t)
	defer database.Close()

	fm := newFakeMailer()
	mailer.SendFunc = fm.capture()
	adminCookies := loginAndVerify(t, h, fm, "owner2@example.com", "password123")

	rr := createTeam(t, h, adminCookies, "Free Choosers", "Owner", nil)
	if rr.Code != http.StatusSeeOther {
		t.Fatalf("team create failed: %s", rr.Body.String())
	}

	form := url.Values{}
	form.Set("email", "chooser@example.com")
	rr = doAuthed(t, h, adminCookies, http.MethodPost, "/teams/free-choosers/members",
		map[string]string{"slug": "free-choosers"},
		strings.NewReader(form.Encode()), "application/x-www-form-urlencoded", h.InviteMember)
	if rr.Code != http.StatusSeeOther {
		t.Fatalf("invite failed: %d %s", rr.Code, rr.Body.String())
	}

	token := fm.inviteTokenFor("chooser@example.com")
	if token == "" {
		t.Fatal("no invitation captured")
	}

	// Accept page should offer unassigned providers (none yet) and a name input.
	req := httptest.NewRequest(http.MethodGet, "/invite/"+token, nil)
	req = withParams(req, map[string]string{"token": token})
	rr = httptest.NewRecorder()
	h.AcceptInvitePage(rr, req)
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), "provider_name") {
		t.Fatalf("expected provider name input on accept page: %d %s", rr.Code, rr.Body.String())
	}

	form = url.Values{}
	form.Set("provider_name", "Zoe")
	req = httptest.NewRequest(http.MethodPost, "/invite/"+token, strings.NewReader(form.Encode()))
	req = withParams(req, map[string]string{"token": token})
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr = httptest.NewRecorder()
	h.AcceptInvite(rr, req)
	if rr.Code != http.StatusSeeOther {
		t.Fatalf("accept failed: %d %s", rr.Code, rr.Body.String())
	}

	team, _ := h.deps.Teams.GetBySlug("free-choosers")
	user, _ := h.deps.Users.GetByEmail("chooser@example.com")
	member, _ := h.deps.Members.Get(team.ID, user.ID)
	if member == nil {
		t.Fatal("membership missing")
	}
	p, _ := h.deps.Providers.GetByID(member.ProviderID)
	if p == nil || p.Name != "Zoe" {
		t.Fatalf("expected provider Zoe, got %+v", p)
	}
}

func TestProviderManagement(t *testing.T) {
	h, database, mailer := setupTestHandler(t)
	defer database.Close()

	fm := newFakeMailer()
	mailer.SendFunc = fm.capture()
	cookies := loginAndVerify(t, h, fm, "prov@example.com", "password123")

	rr := createTeam(t, h, cookies, "Provider Pals", "Pat", nil)
	if rr.Code != http.StatusSeeOther {
		t.Fatalf("team create failed")
	}
	team, _ := h.deps.Teams.GetBySlug("provider-pals")
	if team == nil {
		t.Fatal("team missing")
	}

	// Add a provider.
	form := url.Values{}
	form.Set("name", "Robin")
	rr = doAuthed(t, h, cookies, http.MethodPost, "/teams/provider-pals/providers",
		map[string]string{"slug": "provider-pals"},
		strings.NewReader(form.Encode()), "application/x-www-form-urlencoded", h.AddProvider)
	if rr.Code != http.StatusSeeOther {
		t.Fatalf("add provider failed: %d %s", rr.Code, rr.Body.String())
	}

	robin, _ := h.deps.Providers.GetByName(team.ID, "Robin")
	if robin == nil {
		t.Fatal("provider Robin not created")
	}

	// Duplicate add is rejected.
	rr = doAuthed(t, h, cookies, http.MethodPost, "/teams/provider-pals/providers",
		map[string]string{"slug": "provider-pals"},
		strings.NewReader(form.Encode()), "application/x-www-form-urlencoded", h.AddProvider)
	if rr.Code == http.StatusSeeOther && strings.Contains(rr.Header().Get("Location"), "ok=") {
		t.Fatal("expected duplicate provider rejection")
	}

	// Disable it.
	rr = doAuthed(t, h, cookies, http.MethodPost, "/teams/provider-pals/providers/toggle",
		map[string]string{"slug": "provider-pals", "id": strconv.FormatInt(robin.ID, 10)},
		nil, "", h.ToggleProvider)
	if rr.Code != http.StatusSeeOther {
		t.Fatalf("toggle failed: %d", rr.Code)
	}
	p, _ := h.deps.Providers.GetByID(robin.ID)
	if p == nil || !p.Disabled {
		t.Fatal("expected Robin disabled")
	}

	// Delete works when unassigned and without tunes.
	rr = doAuthed(t, h, cookies, http.MethodPost, "/teams/provider-pals/providers/delete",
		map[string]string{"slug": "provider-pals", "id": strconv.FormatInt(robin.ID, 10)},
		nil, "", h.DeleteProvider)
	if rr.Code != http.StatusSeeOther {
		t.Fatalf("delete failed: %d", rr.Code)
	}
	p, _ = h.deps.Providers.GetByID(robin.ID)
	if p != nil {
		t.Fatal("expected Robin deleted")
	}

	// Deleting Pat (assigned to admin) fails.
	pat, _ := h.deps.Providers.GetByName(team.ID, "Pat")
	if pat == nil {
		t.Fatal("Pat missing")
	}
	rr = doAuthed(t, h, cookies, http.MethodPost, "/teams/provider-pals/providers/delete",
		map[string]string{"slug": "provider-pals", "id": strconv.FormatInt(pat.ID, 10)},
		nil, "", h.DeleteProvider)
	if !strings.Contains(rr.Header().Get("Location"), "err=") {
		t.Fatalf("expected delete error for assigned provider, got %s", rr.Header().Get("Location"))
	}
}
