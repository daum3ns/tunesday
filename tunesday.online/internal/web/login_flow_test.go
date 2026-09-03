package web

import (
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestLoginReturnsToDeepLink(t *testing.T) {
	h, database, mailer := setupTestHandler(t)
	defer database.Close()

	fm := newFakeMailer()
	mailer.SendFunc = fm.capture()
	ts := httptest.NewServer(h.Router())
	defer ts.Close()
	server := ts.URL

	registerAndVerify(t, server, fm, "deeplink@example.com", "password123")

	// Anonymous visit to a team page: bounced to /login?next=...
	anon := newTestUser(t, "anon")
	res := anon.get(server, "/teams/deep-team/dashboard")
	if q := res.Request.URL.Query().Get("next"); q != "/teams/deep-team/dashboard" {
		t.Fatalf("login redirect lost the target: %s", res.Request.URL)
	}
	// The login page must carry the target through the form.
	body := readBody(t, res)
	if !strings.Contains(body, `name="next" value="/teams/deep-team/dashboard"`) {
		t.Fatal("login form missing next field")
	}

	// Logging in with the form lands on the deep link.
	form := url.Values{}
	form.Set("email", "deeplink@example.com")
	form.Set("password", "password123")
	form.Set("next", "/teams/deep-team/dashboard")
	res = anon.postForm(server, "/login", form)
	res.Body.Close()
	if res.Request.URL.Path != "/teams/deep-team/dashboard" {
		t.Fatalf("post-login landing = %s", res.Request.URL.Path)
	}

	// Hostile next values are ignored, not followed.
	fresh := newTestUser(t, "fresh2")
	form.Set("next", "//evil.example/phish")
	res = fresh.postForm(server, "/login", form)
	res.Body.Close()
	if res.Request.URL.Path != "/onboarding" || strings.Contains(res.Request.URL.String(), "evil") {
		t.Fatalf("hostile next followed: %s", res.Request.URL)
	}
}

func TestMembersPageRosterPointer(t *testing.T) {
	h, database, mailer := setupTestHandler(t)
	defer database.Close()

	fm := newFakeMailer()
	mailer.SendFunc = fm.capture()
	ts := httptest.NewServer(h.Router())
	defer ts.Close()
	server := ts.URL

	admin := registerAndVerify(t, server, fm, "pointer@example.com", "password123")
	res := admin.postMultipart(server, "/teams", map[string]string{
		"team_name": "Point Team", "your_name": "Pilot",
	})
	res.Body.Close()

	// Freshly created team: every seat is filled (the admin's own) — no pointer.
	page := readBody(t, admin.get(server, "/teams/point-team/members"))
	if strings.Contains(page, "still have no member") {
		t.Fatal("pointer shown without empty seats")
	}

	// Add a seat with nobody in it.
	form := url.Values{}
	form.Set("name", "SpareSeat")
	res = admin.postForm(server, "/teams/point-team/providers", form)
	res.Body.Close()

	page = readBody(t, admin.get(server, "/teams/point-team/members"))
	if !strings.Contains(page, "still have no member") || !strings.Contains(page, "SpareSeat") {
		t.Fatal("roster pointer missing for unassigned seat")
	}
	if !strings.Contains(page, ">roster<") {
		t.Fatal("roster tab rename missing in nav")
	}
}
