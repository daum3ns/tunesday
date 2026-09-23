package web

import (
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// platformSquadSetup builds a two-member team, starts a ceremony, dials both
// attendees and reveals a winner. The caller can then exercise tune adding.
func platformSquadSetup(t *testing.T, server, wsBase string, fm *fakeMailer) (*testUser, *testUser, string) {
	t.Helper()
	admin := registerAndVerify(t, server, fm, "boss@example.com", "password123")
	res := admin.postMultipart(server, "/teams", map[string]string{
		"team_name": "Platform Squad", "your_name": "Boss",
	})
	res.Body.Close()

	form := url.Values{}
	form.Set("email", "member@example.com")
	form.Set("provider_name", "Member")
	rr := admin.postForm(server, "/teams/platform-squad/members", form)
	rr.Body.Close()
	member := newTestUser(t, "member@example.com")
	accept := member.postForm(server, "/invite/"+fm.inviteTokenFor("member@example.com"), url.Values{})
	accept.Body.Close()

	slug := "platform-squad"
	token := startCeremony(t, server, admin, slug)
	aConn := dialRoom(t, wsBase, slug, token, admin, server)
	mConn := dialRoom(t, wsBase, slug, token, member, server)
	revealWinner(t, server, admin, slug, token)
	aConn.Close()
	mConn.Close()
	return admin, member, token
}

func TestCeremonyAddTunePlatformValidation(t *testing.T) {
	h, database, mailer := setupTestHandler(t)
	defer database.Close()

	fm := newFakeMailer()
	mailer.SendFunc = fm.capture()
	ts := httptest.NewServer(h.Router())
	defer ts.Close()
	server := ts.URL
	wsBase := "ws" + strings.TrimPrefix(server, "http")

	admin, member, token := platformSquadSetup(t, server, wsBase, fm)

	team, err := h.deps.Teams.GetBySlug("platform-squad")
	if err != nil || team == nil {
		t.Fatal("platform squad team missing")
	}

	postLink := func(link string) string {
		t.Helper()
		form := url.Values{}
		if link != "" {
			form.Set("link", link)
		}
		res := admin.postForm(server, "/teams/"+team.Slug+"/ceremonies/"+token+"/tune", form)
		defer res.Body.Close()
		return res.Request.URL.Query().Get("err")
	}

	// Unsupported platform denied with a clear hint.
	if got := postLink("https://open.spotify.com/track/abc123"); !strings.Contains(got, "Unsupported link") {
		t.Fatalf("expected unsupported-link flash, got %q", got)
	}

	// Empty form denied with the paste hint.
	if got := postLink(""); !strings.Contains(got, "Paste a YouTube, SoundCloud, or Bandcamp link please.") {
		t.Fatalf("expected paste hint, got %q", got)
	}

	// SoundCloud accepted.
	if got := postLink("https://soundcloud.com/lofi-han/summer-haze"); got != "" {
		t.Fatalf("unexpected err flash on success: %q", got)
	}

	tunes, err := h.deps.Tunes.ListAllByTeam(team.ID)
	if err != nil || len(tunes) != 1 {
		t.Fatalf("expected exactly 1 stored tune, got %d (err %v)", len(tunes), err)
	}
	got := tunes[0]
	if got.Platform != "soundcloud" {
		t.Fatalf("expected platform soundcloud, got %q", got.Platform)
	}
	if got.YouTubeID != "" {
		t.Fatalf("expected empty youtube_id for soundcloud, got %q", got.YouTubeID)
	}
	if got.Link != "https://soundcloud.com/lofi-han/summer-haze" {
		t.Fatalf("expected canonical soundcloud link, got %q", got.Link)
	}
	if got.Title == "" {
		t.Fatal("expected a fetched title")
	}

	// A second ceremony allows the Bandcamp platform too (fresh winner + room).
	token2 := startCeremony(t, server, admin, "platform-squad")
	a2 := dialRoom(t, wsBase, "platform-squad", token2, admin, server)
	m2 := dialRoom(t, wsBase, "platform-squad", token2, member, server)
	revealWinner(t, server, admin, "platform-squad", token2)
	a2.Close()
	m2.Close()

	form := url.Values{}
	form.Set("link", "https://gasolinelollipops.bandcamp.com/track/love-is-free-single")
	res := admin.postForm(server, "/teams/"+team.Slug+"/ceremonies/"+token2+"/tune", form)
	res.Body.Close()
	if got := res.Request.URL.Query().Get("err"); got != "" {
		t.Fatalf("unexpected err flash for bandcamp: %q", got)
	}

	tunes, _ = h.deps.Tunes.ListAllByTeam(team.ID)
	foundBandcamp := false
	for _, tu := range tunes {
		if tu.Platform == "bandcamp" {
			foundBandcamp = true
			if tu.YouTubeID != "" {
				t.Fatalf("expected empty youtube_id for bandcamp, got %q", tu.YouTubeID)
			}
		}
	}
	if !foundBandcamp {
		t.Fatalf("expected a bandcamp tune, got %+v", tunes)
	}
}
