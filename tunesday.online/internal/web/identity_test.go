package web

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestSetPasswordThenLogin(t *testing.T) {
	h, database, mailer := setupTestHandler(t)
	defer database.Close()

	fm := newFakeMailer()
	mailer.SendFunc = fm.capture()
	ts := httptest.NewServer(h.Router())
	defer ts.Close()
	server := ts.URL

	admin := registerAndVerify(t, server, fm, "pw@example.com", "originalpw")

	// Set an optional password.
	form := url.Values{}
	form.Set("password", "brand-new-password")
	form.Set("password_confirm", "brand-new-password")
	res := admin.postForm(server, "/account/password", form)
	res.Body.Close()
	if !strings.Contains(res.Request.URL.RawQuery, "ok=") {
		t.Fatalf("set password should flash ok, landed on %s", res.Request.URL)
	}

	user, _ := h.deps.Users.GetByEmail("pw@example.com")
	if user == nil || !strings.HasPrefix(user.PasswordHash, "$2") {
		t.Fatalf("expected bcrypt hash stored, got %q", user.PasswordHash)
	}

	// Fresh client logs in with the new password.
	fresh := newTestUser(t, "fresh")
	form = url.Values{}
	form.Set("email", "pw@example.com")
	form.Set("password", "brand-new-password")
	res = fresh.postForm(server, "/login", form)
	res.Body.Close()
	if res.Request.URL.Path != "/onboarding" {
		t.Fatalf("password login failed, landed on %s", res.Request.URL.Path)
	}

	// Old password no longer works.
	form.Set("password", "originalpw")
	res = fresh.postForm(server, "/login", form)
	body := readBody(t, res)
	if !strings.Contains(body, "Invalid email or password") {
		t.Fatalf("old password should be rejected: %s", body)
	}
}

func TestLoginPageShowsMagicLinkHint(t *testing.T) {
	h, database, mailer := setupTestHandler(t)
	defer database.Close()

	fm := newFakeMailer()
	mailer.SendFunc = fm.capture()
	ts := httptest.NewServer(h.Router())
	defer ts.Close()
	server := ts.URL

	// The login page must always point passwordless members at the magic link.
	fresh := newTestUser(t, "fresh")
	res := fresh.get(server, "/login")
	body := readBody(t, res)
	if !strings.Contains(body, "magic link") || !strings.Contains(body, "/login/link") {
		t.Fatal("login page should hint at the magic link flow")
	}

	// A member created via invitation has no password: password login fails
	// with the generic message and never a crash.
	registerAndVerify(t, server, fm, "owner@example.com", "password123")
	owner := newTestUser(t, "owner")
	_ = owner
	res2 := fresh.postForm(server, "/login", url.Values{
		"email":    {"nobody-yet@example.com"},
		"password": {"whatever"},
	})
	body2 := readBody(t, res2)
	if !strings.Contains(body2, "Invalid email or password") {
		t.Fatalf("expected generic rejection, got %s", body2)
	}
}

func TestLoginLinkSelfService(t *testing.T) {
	h, database, mailer := setupTestHandler(t)
	defer database.Close()

	fm := newFakeMailer()
	mailer.SendFunc = fm.capture()
	ts := httptest.NewServer(h.Router())
	defer ts.Close()
	server := ts.URL

	resetLoginLinkThrottle()

	// Register, invite, accept — the member now has a membership + key.
	admin := registerAndVerify(t, server, fm, "boss2@example.com", "password123")
	res := admin.postMultipart(server, "/teams", map[string]string{
		"team_name": "Link Team", "your_name": "Boss",
	})
	res.Body.Close()
	form := url.Values{}
	form.Set("email", "seeker@example.com")
	form.Set("provider_name", "Seeker")
	rr := admin.postForm(server, "/teams/link-team/members", form)
	rr.Body.Close()
	seeker := newTestUser(t, "seeker")
	acc := seeker.postForm(server, "/invite/"+fm.inviteTokenFor("seeker@example.com"), url.Values{})
	acc.Body.Close()

	// Self-service: request a fresh login link.
	anon := newTestUser(t, "anon")
	form = url.Values{}
	form.Set("email", "seeker@example.com")
	res = anon.postForm(server, "/login/link", form)
	body := readBody(t, res)
	if !strings.Contains(body, "on its way") {
		t.Fatalf("expected neutral confirmation, got %s", body)
	}
	if !strings.Contains(fm.loginLinks["seeker@example.com"], "/join/") {
		t.Fatalf("expected login links email, got %q", fm.loginLinks["seeker@example.com"])
	}

	// The delivered link actually logs in on a clean device.
	links := fm.loginLinks["seeker@example.com"]
	joinToken := strings.TrimPrefix(strings.TrimSpace(firstLineAfter(links, "/join/")), "")
	if joinToken == "" {
		t.Fatal("no join token in email")
	}
	clean := newTestUser(t, "clean")
	res = clean.get(server, "/join/"+joinToken)
	res.Body.Close()
	if res.Request.URL.Path != "/teams/link-team/dashboard" {
		t.Fatalf("magic link login landed on %s", res.Request.URL.Path)
	}

	// Unknown email: same neutral message, no email sent.
	// (Reset the throttle: all test clients share one IP here.)
	resetLoginLinkThrottle()
	unknown := newTestUser(t, "unknown")
	form = url.Values{}
	form.Set("email", "nobody@example.com")
	res = unknown.postForm(server, "/login/link", form)
	body = readBody(t, res)
	if !strings.Contains(body, "on its way") {
		t.Fatalf("unknown email must get the neutral message, got %s", body)
	}
	if _, sent := fm.loginLinks["nobody@example.com"]; sent {
		t.Fatal("no email should go out for unknown addresses")
	}
}

func firstLineAfter(s, marker string) string {
	idx := strings.Index(s, marker)
	if idx == -1 {
		return ""
	}
	rest := s[idx+len("/join/"):]
	for i, r := range rest {
		if r == ' ' || r == '\n' || r == '\r' {
			return rest[:i]
		}
	}
	return rest
}

func readBody(t *testing.T, res *http.Response) string {
	t.Helper()
	defer res.Body.Close()
	var sb strings.Builder
	buf := make([]byte, 32*1024)
	for {
		n, err := res.Body.Read(buf)
		sb.Write(buf[:n])
		if err != nil {
			break
		}
	}
	return sb.String()
}
