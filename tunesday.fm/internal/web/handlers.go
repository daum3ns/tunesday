package web

import (
	"embed"
	"html/template"
	"io/fs"
	"net/http"
	"time"

	"github.com/google/uuid"

	"tunesday/tunesday.fm/internal/auth"
	"tunesday/tunesday.fm/internal/config"
	"tunesday/tunesday.fm/internal/email"
	"tunesday/tunesday.fm/internal/store"
)

//go:embed templates/*.html
var templatesFS embed.FS

//go:embed static/*
var staticFS embed.FS

// Handler holds the web handlers and templates.
type Handler struct {
	tmpls         map[string]*template.Template
	cfg           *config.Config
	users         *store.UserStore
	verifications *store.VerificationTokenStore
	sessions      *auth.SessionStore
	email         *email.Service
}

// NewHandler creates a new web handler, parsing embedded templates.
func NewHandler(cfg *config.Config, users *store.UserStore, verifications *store.VerificationTokenStore, sessions *auth.SessionStore, email *email.Service) (*Handler, error) {
	pages := []string{"base.html", "landing.html", "register.html", "login.html", "verify.html"}
	tmpls := make(map[string]*template.Template)
	for _, page := range pages {
		tmpl, err := template.ParseFS(templatesFS, "templates/base.html", "templates/"+page)
		if err != nil {
			return nil, err
		}
		tmpls[page] = tmpl
	}
	return &Handler{
		tmpls:         tmpls,
		cfg:           cfg,
		users:         users,
		verifications: verifications,
		sessions:      sessions,
		email:         email,
	}, nil
}

// StaticFiles returns an http.Handler for embedded static assets.
func (h *Handler) StaticFiles() http.Handler {
	staticSub, err := fs.Sub(staticFS, "static")
	if err != nil {
		panic(err)
	}
	return http.FileServer(http.FS(staticSub))
}

func (h *Handler) render(w http.ResponseWriter, page string, data map[string]any) {
	if data == nil {
		data = map[string]any{}
	}
	if _, ok := data["Header"]; !ok {
		data["Header"] = headerASCII()
	}
	tmpl, ok := h.tmpls[page]
	if !ok {
		http.Error(w, "template not found", http.StatusInternalServerError)
		return
	}
	if err := tmpl.ExecuteTemplate(w, "base.html", data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// Landing renders the landing page.
func (h *Handler) Landing(w http.ResponseWriter, r *http.Request) {
	h.render(w, "landing.html", map[string]any{
		"Title": "tunesday.fm",
	})
}

// RegisterPage renders the registration form.
func (h *Handler) RegisterPage(w http.ResponseWriter, r *http.Request) {
	h.render(w, "register.html", map[string]any{
		"Title": "Register",
	})
}

// Register handles registration form submission.
func (h *Handler) Register(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		h.render(w, "register.html", map[string]any{
			"Title":   "Register",
			"Message": "Invalid form",
		})
		return
	}

	emailAddr := r.FormValue("email")
	password := r.FormValue("password")
	passwordConfirm := r.FormValue("password_confirm")

	if emailAddr == "" || password == "" {
		h.render(w, "register.html", map[string]any{
			"Title":   "Register",
			"Message": "Email and password are required",
		})
		return
	}

	if password != passwordConfirm {
		h.render(w, "register.html", map[string]any{
			"Title":   "Register",
			"Message": "Passwords do not match",
		})
		return
	}

	exists, err := h.users.Exists(emailAddr)
	if err != nil {
		h.render(w, "register.html", map[string]any{
			"Title":   "Register",
			"Message": "Something went wrong",
		})
		return
	}
	if exists {
		h.render(w, "register.html", map[string]any{
			"Title":   "Register",
			"Message": "An account with this email already exists",
		})
		return
	}

	hash, err := auth.HashPassword(password, h.cfg.BcryptCost)
	if err != nil {
		h.render(w, "register.html", map[string]any{
			"Title":   "Register",
			"Message": "Something went wrong",
		})
		return
	}

	user := &store.User{
		ID:            uuid.NewString(),
		Email:         emailAddr,
		PasswordHash:  hash,
		EmailVerified: false,
		CreatedAt:     time.Now(),
	}
	if err := h.users.Create(user); err != nil {
		h.render(w, "register.html", map[string]any{
			"Title":   "Register",
			"Message": "Could not create account",
		})
		return
	}

	token := &store.VerificationToken{
		ID:        uuid.NewString(),
		UserID:    user.ID,
		Token:     uuid.NewString(),
		Used:      false,
		CreatedAt: time.Now(),
	}
	if err := h.verifications.Create(token); err != nil {
		h.render(w, "register.html", map[string]any{
			"Title":   "Register",
			"Message": "Could not create verification token",
		})
		return
	}

	if err := h.email.SendVerificationEmail(user.Email, token.Token); err != nil {
		h.render(w, "register.html", map[string]any{
			"Title":   "Register",
			"Message": "Account created, but failed to send verification email: " + err.Error(),
		})
		return
	}

	h.render(w, "verify.html", map[string]any{
		"Title":   "Verify your email",
		"Message": "Check your inbox for a verification link.",
	})
}

// Verify handles the email verification link.
func (h *Handler) Verify(w http.ResponseWriter, r *http.Request) {
	tokenValue := r.URL.Query().Get("token")
	if tokenValue == "" {
		h.render(w, "verify.html", map[string]any{
			"Title":   "Verification",
			"Message": "Missing verification token",
		})
		return
	}

	token, err := h.verifications.GetByToken(tokenValue)
	if err != nil {
		h.render(w, "verify.html", map[string]any{
			"Title":   "Verification",
			"Message": "Something went wrong",
		})
		return
	}
	if token == nil || token.Used {
		h.render(w, "verify.html", map[string]any{
			"Title":   "Verification",
			"Message": "Invalid or already used verification token",
		})
		return
	}

	if err := h.users.MarkVerified(token.UserID); err != nil {
		h.render(w, "verify.html", map[string]any{
			"Title":   "Verification",
			"Message": "Could not verify email",
		})
		return
	}

	if err := h.verifications.MarkUsed(token.ID); err != nil {
		h.render(w, "verify.html", map[string]any{
			"Title":   "Verification",
			"Message": "Could not mark token as used",
		})
		return
	}

	h.render(w, "verify.html", map[string]any{
		"Title":   "Verification",
		"Message": "Email verified! You can now log in.",
	})
}

// LoginPage renders the login form.
func (h *Handler) LoginPage(w http.ResponseWriter, r *http.Request) {
	h.render(w, "login.html", map[string]any{
		"Title": "Login",
	})
}

// Login handles login form submission.
func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		h.render(w, "login.html", map[string]any{
			"Title":   "Login",
			"Message": "Invalid form",
		})
		return
	}

	emailAddr := r.FormValue("email")
	password := r.FormValue("password")

	user, err := h.users.GetByEmail(emailAddr)
	if err != nil {
		h.render(w, "login.html", map[string]any{
			"Title":   "Login",
			"Message": "Something went wrong",
		})
		return
	}
	if user == nil {
		h.render(w, "login.html", map[string]any{
			"Title":   "Login",
			"Message": "Invalid email or password",
		})
		return
	}

	if !auth.CheckPassword(password, user.PasswordHash) {
		h.render(w, "login.html", map[string]any{
			"Title":   "Login",
			"Message": "Invalid email or password",
		})
		return
	}

	if !user.EmailVerified {
		h.render(w, "login.html", map[string]any{
			"Title":   "Login",
			"Message": "Please verify your email before logging in",
		})
		return
	}

	if err := h.sessions.SetUserID(w, r, user.ID); err != nil {
		h.render(w, "login.html", map[string]any{
			"Title":   "Login",
			"Message": "Could not create session",
		})
		return
	}

	http.Redirect(w, r, "/onboarding", http.StatusSeeOther)
}

// Logout clears the session.
func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
	_ = h.sessions.Clear(w, r)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// Onboarding renders the onboarding page.
func (h *Handler) Onboarding(w http.ResponseWriter, r *http.Request) {
	h.render(w, "landing.html", map[string]any{
		"Title": "Welcome",
	})
}

func headerASCII() string {
	return `██████████████████████████████████████████████████████████████████████████
█▌                                                                      ▐█
█▌                                                                      ▐█
█▌                                                                      ▐█
█▌     ░▀█▀░▀█▀░▀░█▀▀░░░░░░░░░                                          ▐█
█▌     ░░█░░░█░░░░▀▀█░░░░░░░░░                                          ▐█
█▌     ░▀▀▀░░▀░░░░▀▀▀░▀░░▀░░▀░                                          ▐█
█▌     ░█░█░█▀█░█▀█░█▀█░█░█░░░▀█▀░█░█░█▀█░█▀▀░█▀▀░█▀▄░█▀█░█░█░░░█░█     ▐█
█▌     ░█▀█░█▀█░█▀▀░█▀▀░░█░░░░░█░░█░█░█░█░█▀▀░▀▀█░█░█░█▀█░░█░░░░▀░▀     ▐█
█▌     ░▀░▀░▀░▀░▀░░░▀░░░░▀░░░░░▀░░▀▀▀░▀░▀░▀▀▀░▀▀▀░▀▀░░▀░▀░░▀░░░░▀░▀     ▐█
█▌                                                                      ▐█
█▌                                                                      ▐█
█▌                                                                      ▐█
██████████████████████████████████████████████████████████████████████████`
}
