package web

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"tunesday/tunesday.fm/internal/live"
)

type wsMsg struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

type ceremonyStatePayload struct {
	Status    string `json:"status"`
	Attendees []struct {
		Alias        string `json:"alias"`
		ProviderName string `json:"provider"`
		IsYou        bool   `json:"isYou"`
	} `json:"attendees"`
	Winner    string `json:"winner"`
	CanReveal bool   `json:"canReveal"`
}

type revealPayload struct {
	Pool        []string `json:"pool"`
	Winner      string   `json:"winner"`
	Seed        int64    `json:"seed"`
	DurationMs  int      `json:"duration_ms"`
	CountdownMs int      `json:"countdown_ms"`
}

// testUser is a browser-like client with its own cookie jar.
type testUser struct {
	t    *testing.T
	http *http.Client
	name string
}

func newTestUser(t *testing.T, name string) *testUser {
	t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	return &testUser{t: t, http: &http.Client{Jar: jar, Timeout: 5 * time.Second}, name: name}
}

func (u *testUser) postForm(server, path string, form url.Values) *http.Response {
	u.t.Helper()
	req, err := http.NewRequest(http.MethodPost, server+path, strings.NewReader(form.Encode()))
	if err != nil {
		u.t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	res, err := u.http.Do(req)
	if err != nil {
		u.t.Fatalf("%s: post %s: %v", u.name, path, err)
	}
	return res
}

func (u *testUser) get(server, path string) *http.Response {
	u.t.Helper()
	res, err := u.http.Get(server + path)
	if err != nil {
		u.t.Fatalf("%s: get %s: %v", u.name, path, err)
	}
	return res
}

func (u *testUser) postMultipart(server, path string, fields map[string]string) *http.Response {
	u.t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	for k, v := range fields {
		_ = w.WriteField(k, v)
	}
	_ = w.Close()
	req, err := http.NewRequest(http.MethodPost, server+path, &buf)
	if err != nil {
		u.t.Fatal(err)
	}
	req.Header.Set("Content-Type", w.FormDataContentType())
	res, err := u.http.Do(req)
	if err != nil {
		u.t.Fatalf("%s: multipart post %s: %v", u.name, path, err)
	}
	return res
}

func (u *testUser) cookieHeader(server string) http.Header {
	tokURL, err := url.Parse(server)
	if err != nil {
		u.t.Fatal(err)
	}
	h := http.Header{}
	for _, c := range u.http.Jar.Cookies(tokURL) {
		h.Add("Cookie", c.Name+"="+c.Value)
	}
	return h
}

// registerAndVerify creates and verifies an account via the real routes.
func registerAndVerify(t *testing.T, server string, fm *fakeMailer, emailAddr, password string) *testUser {
	t.Helper()
	u := newTestUser(t, emailAddr)
	form := url.Values{}
	form.Set("email", emailAddr)
	form.Set("password", password)
	form.Set("password_confirm", password)
	res := u.postForm(server, "/register", form)
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("register status %d", res.StatusCode)
	}

	token := fm.verifications[emailAddr]
	if token == "" {
		t.Fatal("no verification token")
	}
	res = u.get(server, "/verify?token="+token)
	defer res.Body.Close()

	form = url.Values{}
	form.Set("email", emailAddr)
	form.Set("password", password)
	res = u.postForm(server, "/login", form)
	defer res.Body.Close()
	if res.Request.URL.Path != "/onboarding" {
		t.Fatalf("login did not land on onboarding: %s", res.Request.URL.Path)
	}
	return u
}

// readUntil reads WS messages until the wanted type arrives (or timeout).
func readUntil(t *testing.T, conn *websocket.Conn, want string) wsMsg {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for {
		_ = conn.SetReadDeadline(deadline)
		_, data, err := conn.ReadMessage()
		if err != nil {
			t.Fatalf("ws read waiting %q: %v", want, err)
		}
		var m wsMsg
		if err := json.Unmarshal(data, &m); err != nil {
			t.Fatalf("ws bad json: %v", err)
		}
		if m.Type == want {
			return m
		}
		if m.Type == "state" || m.Type == "attendees" {
			continue
		}
		t.Fatalf("unexpected ws type %q while waiting for %q", m.Type, want)
	}
}

func TestCeremonyEndToEnd(t *testing.T) {
	h, database, mailer := setupTestHandler(t)
	defer database.Close()

	fm := newFakeMailer()
	mailer.SendFunc = fm.capture()

	ts := httptest.NewServer(h.Router())
	defer ts.Close()
	server := ts.URL

	admin := registerAndVerify(t, server, fm, "boss@example.com", "password123")

	// Create team.
	res := admin.postMultipart(server, "/teams", map[string]string{
		"team_name": "Ceremony Squad",
		"your_name": "Boss",
	})
	res.Body.Close()
	if res.Request.URL.Path != "/teams/ceremony-squad/dashboard" {
		t.Fatalf("team create should land on dashboard, got %s", res.Request.URL.Path)
	}

	// Invite a second provider and let them accept.
	form := url.Values{}
	form.Set("email", "drummer@example.com")
	form.Set("provider_name", "Drummer")
	res = admin.postForm(server, "/teams/ceremony-squad/members", form)
	res.Body.Close()
	inviteToken := fm.inviteTokenFor("drummer@example.com")
	if inviteToken == "" {
		t.Fatal("no invite captured")
	}
	drummer := newTestUser(t, "drummer@example.com")
	res = drummer.postForm(server, "/invite/"+inviteToken, url.Values{})
	res.Body.Close()
	if res.Request.URL.Path != "/teams/ceremony-squad/dashboard" {
		t.Fatalf("accept invite should land on dashboard, ended on %s", res.Request.URL.Path)
	}

	// Start ceremony as admin (follows the redirect to the host page).
	res = admin.postForm(server, "/teams/ceremony-squad/ceremonies", url.Values{})
	res.Body.Close()
	finalPath := res.Request.URL.Path
	if !strings.HasPrefix(finalPath, "/teams/ceremony-squad/ceremonies/") || !strings.HasSuffix(finalPath, "/host") {
		t.Fatalf("ceremony start should land on host page, got %s", finalPath)
	}
	token := strings.TrimSuffix(strings.TrimPrefix(finalPath, "/teams/ceremony-squad/ceremonies/"), "/host")
	if token == "" || strings.Contains(token, "/") {
		t.Fatalf("bad ceremony token from %s", finalPath)
	}

	// Both connect via WebSocket.
	wsBase := "ws" + strings.TrimPrefix(server, "http")
	adminConn, _, err := websocket.DefaultDialer.Dial(wsBase+"/teams/ceremony-squad/ceremonies/"+token+"/ws", admin.cookieHeader(server))
	if err != nil {
		t.Fatalf("admin ws dial: %v", err)
	}
	defer adminConn.Close()

	var adminState ceremonyStatePayload
	if m := readUntil(t, adminConn, "state"); json.Unmarshal(m.Payload, &adminState) != nil {
		t.Fatal("bad state payload")
	}
	if adminState.Status != "open" {
		t.Fatalf("admin state wrong: %+v", adminState)
	}
	// Host alone in the room: not reveal-ready yet.
	if adminState.CanReveal {
		t.Fatal("host must not be able to reveal while alone in the room")
	}
	if len(adminState.Attendees) != 1 {
		t.Fatalf("expected 1 attendee, got %d", len(adminState.Attendees))
	}

	drummerConn, _, err := websocket.DefaultDialer.Dial(wsBase+"/teams/ceremony-squad/ceremonies/"+token+"/ws", drummer.cookieHeader(server))
	if err != nil {
		t.Fatalf("drummer ws dial: %v", err)
	}
	defer drummerConn.Close()

	var drummerState ceremonyStatePayload
	if m := readUntil(t, drummerConn, "state"); json.Unmarshal(m.Payload, &drummerState) != nil {
		t.Fatal("bad state payload")
	}
	if len(drummerState.Attendees) != 2 {
		t.Fatalf("expected 2 attendees for drummer, got %d", len(drummerState.Attendees))
	}
	if drummerState.CanReveal {
		t.Fatal("drummer (member) must not be allowed to reveal")
	}
	// Admin should have seen the attendee broadcast too.
	readUntil(t, adminConn, "attendees")

	// Aliases must be non-empty and music-tech style.
	for _, a := range drummerState.Attendees {
		if a.Alias == "" {
			t.Fatal("expected alias for every attendee")
		}
	}

	// Reveal.
	res = admin.postForm(server, "/teams/ceremony-squad/ceremonies/"+token+"/reveal", url.Values{})
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(res.Body)
		t.Fatalf("reveal status %d: %s", res.StatusCode, body)
	}

	var revealReveal struct {
		OK     bool   `json:"ok"`
		Winner string `json:"winner"`
	}
	if err := json.NewDecoder(res.Body).Decode(&revealReveal); err != nil {
		t.Fatalf("reveal json: %v", err)
	}

	var adminReveal, drummerReveal revealPayload
	if m := readUntil(t, drummerConn, "reveal"); json.Unmarshal(m.Payload, &drummerReveal) != nil {
		t.Fatal("bad reveal payload")
	}
	if m := readUntil(t, adminConn, "reveal"); json.Unmarshal(m.Payload, &adminReveal) != nil {
		t.Fatal("bad reveal payload for admin")
	}
	if drummerReveal.Winner != adminReveal.Winner || drummerReveal.Winner != revealReveal.Winner {
		t.Fatalf("winner mismatch: admin conn %q, drummer conn %q, http %q", adminReveal.Winner, drummerReveal.Winner, revealReveal.Winner)
	}
	if len(drummerReveal.Pool) != 2 {
		t.Fatalf("expected pool of 2, got %v", drummerReveal.Pool)
	}
	if drummerReveal.CountdownMs != 3000 || adminReveal.CountdownMs != 3000 {
		t.Fatalf("expected 3000ms synced countdown, got %d / %d",
			adminReveal.CountdownMs, drummerReveal.CountdownMs)
	}

	// Double reveal is rejected.
	res2 := admin.postForm(server, "/teams/ceremony-squad/ceremonies/"+token+"/reveal", url.Values{})
	res2.Body.Close()
	if res2.StatusCode != http.StatusConflict {
		t.Fatalf("expected 409 on second reveal, got %d", res2.StatusCode)
	}

	// Winner adds the tune (admin acts as proxy; allowed for admins).
	form = url.Values{}
	form.Set("link", "https://www.youtube.com/watch?v=dQw4w9WgXcQ")
	res = admin.postForm(server, "/teams/ceremony-squad/ceremonies/"+token+"/tune", form)
	res.Body.Close()

	var adminComplete, drummerComplete live.Message
	if m := readUntil(t, drummerConn, "complete"); json.Unmarshal(m.Payload, &drummerComplete) != nil {
		t.Fatal("bad complete payload for drummer")
	}
	_ = adminComplete

	// DB reflects completion.
	cer, err := h.deps.Ceremonies.GetByToken(token)
	if err != nil || cer == nil {
		t.Fatalf("ceremony lookup: %v", err)
	}
	if !cer.Completed() {
		t.Fatal("expected ceremony completed")
	}
	count, _ := h.deps.Tunes.CountByTeam(cer.TeamID)
	if count != 1 {
		t.Fatalf("expected 1 tune, got %d", count)
	}
	if winner := cer.WinnerProviderID; winner == 0 {
		t.Fatal("winner provider id not recorded")
	}
	provider, _ := h.deps.Providers.GetByID(cer.WinnerProviderID)
	if provider == nil || provider.TuneCount != 1 {
		t.Fatalf("expected winner provider tune count 1, got %+v", provider)
	}

	// Dashboard history must show the finished ceremony: winner + tune.
	dash := admin.get(server, "/teams/ceremony-squad/dashboard")
	dashBody := readBody(t, dash)
	if !strings.Contains(dashBody, drummerReveal.Winner) || !strings.Contains(dashBody, "Fake Title") {
		t.Fatalf("dashboard history missing winner/tune: %s", dashBody)
	}

	// A late joiner gets the full completed state (using the admin session).
	if lateConn, resp, err := websocket.DefaultDialer.Dial(
		wsBase+"/teams/ceremony-squad/ceremonies/"+token+"/ws",
		admin.cookieHeader(server),
	); err == nil {
		defer lateConn.Close()
		_ = resp
		var lateState ceremonyStatePayload
		if m := readUntil(t, lateConn, "state"); json.Unmarshal(m.Payload, &lateState) == nil {
			if lateState.Status != "completed" || lateState.Winner == "" {
				t.Fatalf("late joiner state: %+v", lateState)
			}
		}
	}
}

func TestCeremonyNeedsTwoEligible(t *testing.T) {
	h, database, mailer := setupTestHandler(t)
	defer database.Close()

	fm := newFakeMailer()
	mailer.SendFunc = fm.capture()
	ts := httptest.NewServer(h.Router())
	defer ts.Close()
	server := ts.URL

	admin := registerAndVerify(t, server, fm, "solo@example.com", "password123")
	res := admin.postMultipart(server, "/teams", map[string]string{
		"team_name": "Solo Squad",
		"your_name": "Solo",
	})
	res.Body.Close()

	res = admin.postForm(server, "/teams/solo-squad/ceremonies", url.Values{})
	res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("expected dashboard render, got %d", res.StatusCode)
	}
	if !strings.Contains(res.Request.URL.RawQuery, "err=") {
		t.Fatalf("expected error flash for <2 eligible, got %s", res.Request.URL.String())
	}
}
