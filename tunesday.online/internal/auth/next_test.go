package auth

import (
	"net/http/httptest"
	"testing"
)

func TestSafeNext(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"/teams/cool/dashboard", "/teams/cool/dashboard"},
		{"/teams/cool/quiz?x=1", "/teams/cool/quiz?x=1"},
		{"", ""},
		{"dashboard", ""},
		{"//evil.example/x", ""},
		{`/\evil.example`, ""},
		{"https://evil.example", ""},
		{"javascript:alert(1)", ""},
		{"/%2F%2Fevil", "/%2F%2Fevil"},
	}
	for _, c := range cases {
		if got := SafeNext(c.in); got != c.want {
			t.Errorf("SafeNext(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestNextFromRequest(t *testing.T) {
	r := httptest.NewRequest("GET", "https://tunesday.online/teams/x/quiz?play=1", nil)
	if got := NextFromRequest(r); got != "/teams/x/quiz?play=1" {
		t.Fatalf("NextFromRequest: %q", got)
	}
	r = httptest.NewRequest("GET", "https://tunesday.online/login", nil)
	if got := NextFromRequest(r); got != "" {
		t.Fatalf("login itself must not produce a next: %q", got)
	}
}
