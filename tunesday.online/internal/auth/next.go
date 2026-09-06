package auth

import (
	"net/http"
	"net/url"
	"strings"
)

// SafeNext returns a usable post-login redirect target from an untrusted
// value: only same-origin absolute paths survive. It rejects scheme URLs,
// protocol-relative "//host" and the "/\host" backslash trick browsers resolve
// as network paths. Anything else becomes "".
func SafeNext(raw string) string {
	if raw == "" || !strings.HasPrefix(raw, "/") {
		return ""
	}
	if strings.HasPrefix(raw, "//") || strings.Contains(raw, `\`) {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil || u.IsAbs() || u.Host != "" {
		return ""
	}
	return raw
}

// NextFromRequest captures the current request target for a login round-trip.
func NextFromRequest(r *http.Request) string {
	target := r.URL.RequestURI()
	if r.URL.Path == "/login" || !strings.HasPrefix(target, "/") {
		return ""
	}
	return SafeNext(target)
}

// RedirectToLogin sends the visitor to the login page, remembering where they
// were headed so Login can bring them back.
func RedirectToLogin(w http.ResponseWriter, r *http.Request) {
	target := "/login"
	if next := NextFromRequest(r); next != "" {
		target += "?next=" + url.QueryEscape(next)
	}
	http.Redirect(w, r, target, http.StatusSeeOther)
}
