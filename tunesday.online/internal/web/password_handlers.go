package web

import (
	"net/http"
	"strings"

	"tunesday/tunesday.online/internal/auth"
)

// SetPassword lets any account holder set or change an optional password.
// There is deliberately no reset flow: the recovery path is always the
// magic link (/login/link), so a forgotten password costs nothing.
func (h *Handler) SetPassword(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r.Context())
	back := "/onboarding"

	if err := r.ParseForm(); err != nil {
		redirectFlash(w, r, back, "err", "Invalid form")
		return
	}
	password := r.FormValue("password")
	confirm := r.FormValue("password_confirm")

	if len(password) < 8 {
		redirectFlash(w, r, back, "err", "Password needs at least 8 characters")
		return
	}
	if password != confirm {
		redirectFlash(w, r, back, "err", "Passwords do not match")
		return
	}

	hash, err := auth.HashPassword(password, h.cfg.BcryptCost)
	if err != nil {
		redirectFlash(w, r, back, "err", "Could not save the password")
		return
	}
	if err := h.deps.Users.SetPasswordHash(user.ID, hash); err != nil {
		redirectFlash(w, r, back, "err", "Could not save the password")
		return
	}
	user.PasswordHash = hash

	redirectFlash(w, r, back, "ok", "Password saved — but your magic link stays the fastest way in.")
}

// hasPassword reports whether the current user has set an optional password.
func hasPassword(r *http.Request) bool {
	user := auth.UserFromContext(r.Context())
	return user != nil && strings.TrimSpace(user.PasswordHash) != ""
}
