package web

import (
	"embed"
	"html/template"
	"io/fs"
	"net/http"
	"net/url"
	"time"

	"github.com/google/uuid"

	"tunesday/internal/playlist"
	"tunesday/tunesday.online/internal/auth"
	"tunesday/tunesday.online/internal/config"
	"tunesday/tunesday.online/internal/db"
	"tunesday/tunesday.online/internal/email"
	"tunesday/tunesday.online/internal/live"
	"tunesday/tunesday.online/internal/radio"
	"tunesday/tunesday.online/internal/store"
	"tunesday/tunesday.online/internal/stream"
)

//go:embed templates/*.html
var templatesFS embed.FS

//go:embed static/*
var staticFS embed.FS

// Deps bundles the services a web handler needs.
type Deps struct {
	DB            *db.DB
	Users         *store.UserStore
	Verifications *store.VerificationTokenStore
	Sessions      *auth.SessionStore
	Email         *email.Service
	Teams         *store.TeamStore
	Providers     *store.ProviderStore
	Members       *store.TeamMemberStore
	Invitations   *store.InvitationStore
	Tunes         *store.TuneStore
	Ceremonies    *store.CeremonyStore
	PlayStats     *store.PlayStatStore
	Quiz          *store.QuizStore
	Rooms         *live.Manager
	Radio         *radio.Manager
	Streams       stream.Resolver
	YT            playlist.TitleProvider
}

// Handler holds the web handlers and templates.
type Handler struct {
	tmpls      map[string]*template.Template
	cfg        *config.Config
	deps       Deps
	streamGate *concurrencyGate
}

// NewHandler creates a new web handler, parsing embedded templates.
func NewHandler(cfg *config.Config, deps Deps) (*Handler, error) {
	pages := []string{
		"base.html", "landing.html", "register.html", "login.html",
		"verify.html", "message.html", "onboarding.html", "team_new.html",
		"dashboard.html", "providers.html", "members.html", "invite_accept.html",
		"ceremony.html", "import.html", "import_confirm.html", "login_link.html",
		"radio.html", "quiz.html",
	}
	tmpls := make(map[string]*template.Template)
	for _, page := range pages {
		tmpl, err := template.ParseFS(templatesFS, "templates/base.html", "templates/"+page)
		if err != nil {
			return nil, err
		}
		tmpls[page] = tmpl
	}
	return &Handler{tmpls: tmpls, cfg: cfg, deps: deps, streamGate: newConcurrencyGate(4)}, nil
}

// StaticFiles returns an http.Handler for embedded static assets.
func (h *Handler) StaticFiles() http.Handler {
	staticSub, err := fs.Sub(staticFS, "static")
	if err != nil {
		panic(err)
	}
	return http.FileServer(http.FS(staticSub))
}

func (h *Handler) render(w http.ResponseWriter, r *http.Request, page string, data map[string]any) {
	if data == nil {
		data = map[string]any{}
	}
	if _, ok := data["Header"]; !ok {
		data["Header"] = headerASCII()
	}
	if _, ok := data["CurrentUser"]; !ok {
		data["CurrentUser"] = auth.UserFromContext(r.Context())
	}
	if _, ok := data["HasPassword"]; !ok {
		data["HasPassword"] = hasPassword(r)
	}
	if q := r.URL.Query(); len(q) > 0 {
		if msg := q.Get("ok"); msg != "" {
			if _, exists := data["Message"]; !exists {
				data["Message"] = msg
			}
		} else if msg := q.Get("err"); msg != "" {
			if _, exists := data["Error"]; !exists {
				data["Error"] = msg
			}
		}
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

// redirectFlash sends a Post/Redirect/Get with an ok or err message in the query.
func redirectFlash(w http.ResponseWriter, r *http.Request, to, key, msg string) {
	http.Redirect(w, r, to+"?"+key+"="+url.QueryEscape(msg), http.StatusSeeOther)
}

// Landing renders the landing page.
func (h *Handler) Landing(w http.ResponseWriter, r *http.Request) {
	h.render(w, r, "landing.html", map[string]any{
		"Title": "tunesday.online",
	})
}

// RegisterPage renders the registration form.
func (h *Handler) RegisterPage(w http.ResponseWriter, r *http.Request) {
	h.render(w, r, "register.html", map[string]any{
		"Title": "Register",
	})
}

// Register handles registration form submission.
func (h *Handler) Register(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		h.render(w, r, "register.html", flash(map[string]any{"Title": "Register"}, "Invalid form"))
		return
	}

	emailAddr := r.FormValue("email")
	password := r.FormValue("password")
	passwordConfirm := r.FormValue("password_confirm")

	fail := func(msg string) {
		h.render(w, r, "register.html", flash(map[string]any{"Title": "Register"}, msg))
	}

	if emailAddr == "" || password == "" {
		fail("Email and password are required")
		return
	}
	if password != passwordConfirm {
		fail("Passwords do not match")
		return
	}

	exists, err := h.deps.Users.Exists(emailAddr)
	if err != nil {
		fail("Something went wrong")
		return
	}
	if exists {
		fail("An account with this email already exists")
		return
	}

	hash, err := auth.HashPassword(password, h.cfg.BcryptCost)
	if err != nil {
		fail("Something went wrong")
		return
	}

	user := &store.User{
		ID:            uuid.NewString(),
		Email:         emailAddr,
		PasswordHash:  hash,
		EmailVerified: false,
		CreatedAt:     time.Now(),
	}
	if err := h.deps.Users.Create(user); err != nil {
		fail("Could not create account")
		return
	}

	token := &store.VerificationToken{
		ID:        uuid.NewString(),
		UserID:    user.ID,
		Token:     uuid.NewString(),
		Used:      false,
		CreatedAt: time.Now(),
	}
	if err := h.deps.Verifications.Create(token); err != nil {
		fail("Could not create verification token")
		return
	}

	if err := h.deps.Email.SendVerificationEmail(user.Email, token.Token); err != nil {
		fail("Account created, but failed to send verification email: " + err.Error())
		return
	}

	h.render(w, r, "verify.html", map[string]any{
		"Title":   "Verify your email",
		"Message": "Check your inbox for a verification link.",
	})
}

// Verify handles the email verification link.
func (h *Handler) Verify(w http.ResponseWriter, r *http.Request) {
	page := func(title, msg string) {
		h.render(w, r, "verify.html", map[string]any{"Title": title, "Message": msg})
	}

	tokenValue := r.URL.Query().Get("token")
	if tokenValue == "" {
		page("Verification", "Missing verification token")
		return
	}

	token, err := h.deps.Verifications.GetByToken(tokenValue)
	if err != nil {
		page("Verification", "Something went wrong")
		return
	}
	if token == nil || token.Used {
		page("Verification", "Invalid or already used verification token")
		return
	}

	if err := h.deps.Users.MarkVerified(token.UserID); err != nil {
		page("Verification", "Could not verify email")
		return
	}
	if err := h.deps.Verifications.MarkUsed(token.ID); err != nil {
		page("Verification", "Could not mark token as used")
		return
	}

	page("Verification", "Email verified! You can now log in.")
}

// LoginPage renders the login form, carrying any deep-link return target.
func (h *Handler) LoginPage(w http.ResponseWriter, r *http.Request) {
	data := map[string]any{"Title": "Login"}
	if next := auth.SafeNext(r.URL.Query().Get("next")); next != "" {
		data["Next"] = next
	}
	h.render(w, r, "login.html", data)
}

// Login handles login form submission.
func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	fail := func(msg string) {
		h.render(w, r, "login.html", flash(map[string]any{"Title": "Login"}, msg))
	}

	if err := r.ParseForm(); err != nil {
		fail("Invalid form")
		return
	}

	emailAddr := r.FormValue("email")
	password := r.FormValue("password")

	user, err := h.deps.Users.GetByEmail(emailAddr)
	if err != nil {
		fail("Something went wrong")
		return
	}
	if user == nil {
		fail("Invalid email or password")
		return
	}
	if !auth.CheckPassword(password, user.PasswordHash) {
		fail("Invalid email or password")
		return
	}
	if !user.EmailVerified {
		fail("Please verify your email before logging in")
		return
	}

	if err := h.deps.Sessions.SetUserID(w, r, user.ID); err != nil {
		fail("Could not create session")
		return
	}

	if next := auth.SafeNext(r.FormValue("next")); next != "" {
		http.Redirect(w, r, next, http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/onboarding", http.StatusSeeOther)
}

// Logout clears the session.
func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
	_ = h.deps.Sessions.Clear(w, r)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func flash(data map[string]any, msg string) map[string]any {
	data["Error"] = msg
	return data
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
