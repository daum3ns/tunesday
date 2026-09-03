package web

import (
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"tunesday/tunesday.fm/internal/email"
)

// LoginPageLink renders the "email me my magic link" form.
func (h *Handler) LoginPageLink(w http.ResponseWriter, r *http.Request) {
	h.render(w, r, "login_link.html", map[string]any{
		"Title": "Get your login link",
	})
}

// SendLoginLink emails join links for every active membership of an address.
// No login required: the email inbox itself is the proof of identity.
// There is deliberately no confirmation or rejection signal to outside
// callers — the same neutral message is shown whether or not the address
// belongs to a member, so the page cannot be used to enumerate members.
func (h *Handler) SendLoginLink(w http.ResponseWriter, r *http.Request) {
	page := func(data map[string]any) {
		h.render(w, r, "login_link.html", data)
	}

	if !loginLinkThrottle.allow(clientIP(r)) {
		page(flash(map[string]any{"Title": "Get your login link"}, "Please wait a moment between requests."))
		return
	}

	if err := r.ParseForm(); err != nil {
		page(flash(map[string]any{"Title": "Get your login link"}, "Invalid form"))
		return
	}
	emailAddr := strings.TrimSpace(r.FormValue("email"))
	if emailAddr == "" {
		page(flash(map[string]any{"Title": "Get your login link"}, "Email is required"))
		return
	}

	neutral := map[string]any{
		"Title":   "Get your login link",
		"Message": "If that email belongs to a tunesday.fm member, a login link is on its way.",
	}

	user, err := h.deps.Users.GetByEmail(emailAddr)
	if err != nil {
		page(neutral)
		return
	}
	if user == nil {
		page(neutral)
		return
	}

	memberships, err := h.deps.Members.ListForUser(user.ID)
	if err != nil || len(memberships) == 0 {
		page(neutral)
		return
	}

	links := make([]email.TeamLink, 0, len(memberships))
	for _, m := range memberships {
		links = append(links, email.TeamLink{
			TeamName: m.TeamName,
			URL:      h.cfg.BaseURL + "/join/" + m.MagicToken,
		})
	}
	if err := h.deps.Email.SendLoginLinkEmail(emailAddr, links); err != nil {
		// Still neutral on purpose; log the real reason server-side.
		log.Printf("tunesday.fm: login link email to %s failed: %v", emailAddr, err)
	}
	page(neutral)
}

// loginLinkThrottle is a coarse per-IP rate limit for the link mailer.
type ipThrottle struct {
	mu     sync.Mutex
	last   map[string]time.Time
	minGap time.Duration
}

func newIPThrottle(minGap time.Duration) *ipThrottle {
	return &ipThrottle{last: map[string]time.Time{}, minGap: minGap}
}

func (t *ipThrottle) reset() {
	t.mu.Lock()
	t.last = map[string]time.Time{}
	t.mu.Unlock()
}

func (t *ipThrottle) allow(ip string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	now := time.Now()
	if seen, ok := t.last[ip]; ok && now.Sub(seen) < t.minGap {
		return false
	}
	t.last[ip] = now
	if len(t.last) > 10000 {
		for k, v := range t.last {
			if now.Sub(v) > time.Hour {
				delete(t.last, k)
			}
		}
	}
	return true
}

var loginLinkThrottle = newIPThrottle(15 * time.Second)

// resetLoginLinkThrottle clears the per-IP throttle. Used by tests, which all
// share one loopback IP and run faster than the throttle window.
func resetLoginLinkThrottle() {
	loginLinkThrottle.reset()
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
