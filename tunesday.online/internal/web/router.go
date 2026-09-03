package web

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"tunesday/tunesday.online/internal/auth"
)

// Router builds the complete HTTP route set.
func (h *Handler) Router() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.RequestID)

	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	r.Handle("/static/*", http.StripPrefix("/static/", h.StaticFiles()))

	// Public auth
	r.Get("/", h.Landing)
	r.Get("/register", h.RegisterPage)
	r.Post("/register", h.Register)
	r.Get("/verify", h.Verify)
	r.Get("/login", h.LoginPage)
	r.Post("/login", h.Login)
	r.Get("/login/link", h.LoginPageLink)
	r.Post("/login/link", h.SendLoginLink)

	// Public magic links (they create the session themselves)
	r.Get("/invite/{token}", h.AcceptInvitePage)
	r.Post("/invite/{token}", h.AcceptInvite)
	r.Get("/join/{token}", h.JoinMagicLink)

	// Authenticated (admin or member)
	r.Group(func(r chi.Router) {
		r.Use(auth.Middleware(h.deps.Sessions, h.deps.Users))
		r.Post("/logout", h.Logout)
		r.Post("/account/password", h.SetPassword)

		r.Get("/onboarding", h.Onboarding)
		r.Get("/teams/new", h.NewTeamForm)
		r.Post("/teams", h.CreateTeam)

		r.Get("/teams/{slug}/dashboard", h.Dashboard)
		r.Get("/teams/{slug}/providers", h.ProvidersPage)
		r.Get("/teams/{slug}/members", h.MembersPage)

		// The Ceremony
		r.Get("/teams/{slug}/ceremonies/{token}", h.CeremonyPage)
		r.Get("/teams/{slug}/ceremonies/{token}/host", h.CeremonyHostPage)
		r.Get("/teams/{slug}/ceremonies/{token}/ws", h.CeremonyWS)
		r.Post("/teams/{slug}/ceremonies", h.StartCeremony)
		r.Post("/teams/{slug}/ceremonies/{token}/reveal", h.CeremonyReveal)
		r.Post("/teams/{slug}/ceremonies/{token}/tune", h.CeremonyAddTune)

		// The Quiz
		r.Get("/teams/{slug}/quiz", h.QuizPage)
		r.Post("/teams/{slug}/quiz/result", h.QuizResult)

		// The Radio Room
		r.Get("/teams/{slug}/radio", h.RadioPage)
		r.Get("/teams/{slug}/radio/ws", h.RadioWS)
		r.Post("/teams/{slug}/radio/play", h.radioControl(radioPlay))
		r.Post("/teams/{slug}/radio/pause", h.radioControl(radioPause))
		r.Post("/teams/{slug}/radio/next", h.radioControl(radioNext))
		r.Post("/teams/{slug}/radio/prev", h.radioControl(radioPrev))
		r.Post("/teams/{slug}/radio/ended", h.radioControl(radioEnded))
		r.Post("/teams/{slug}/radio/mode", h.radioControl(radioMode))

		// Data: export and destructive replace (admin-only inside handlers)
		r.Get("/teams/{slug}/export", h.ExportTeam)
		r.Get("/teams/{slug}/import", h.ImportPage)
		r.Post("/teams/{slug}/import", h.ImportPreview)
		r.Post("/teams/{slug}/import/confirm", h.ImportConfirm)

		// Admin-only mutations (role checks happen inside the handlers)
		r.Post("/teams/{slug}/providers", h.AddProvider)
		r.Post("/teams/{slug}/providers/{id}/rename", h.RenameProvider)
		r.Post("/teams/{slug}/providers/{id}/toggle", h.ToggleProvider)
		r.Post("/teams/{slug}/providers/{id}/delete", h.DeleteProvider)
		r.Post("/teams/{slug}/members", h.InviteMember)
		r.Post("/teams/{slug}/members/{user}/role", h.SetMemberRole)
		r.Post("/teams/{slug}/members/{user}/provider", h.SetMemberProvider)
		r.Post("/teams/{slug}/members/{user}/remove", h.RemoveMember)
	})

	return r
}
