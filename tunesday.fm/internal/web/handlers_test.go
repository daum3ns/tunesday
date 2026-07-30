package web

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"tunesday/tunesday.fm/internal/auth"
	"tunesday/tunesday.fm/internal/config"
	"tunesday/tunesday.fm/internal/db"
	"tunesday/tunesday.fm/internal/email"
	"tunesday/tunesday.fm/internal/store"
)

func setupTestHandler(t *testing.T) (*Handler, *db.DB, *email.Service) {
	t.Helper()

	cfg := &config.Config{
		BaseURL:       "https://tunesday.fm",
		BcryptCost:    4, // low cost for tests
		SessionSecret: []byte("test-secret-test-secret-test-secret"),
		SessionSecure: false,
		SMTPHost:      "smtp.example.com",
		SMTPPort:      587,
		SMTPUser:      "test",
		SMTPPass:      "test",
		SMTPFrom:      "test@example.com",
	}

	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}

	users := store.NewUserStore(database)
	verifications := store.NewVerificationTokenStore(database)
	sessions := auth.NewSessionStore(cfg.SessionSecret, cfg.SessionSecure)
	mailer := email.NewService(cfg)

	h, err := NewHandler(cfg, users, verifications, sessions, mailer)
	if err != nil {
		t.Fatalf("new handler: %v", err)
	}

	return h, database, mailer
}

func TestRegisterAndVerify(t *testing.T) {
	h, db, mailer := setupTestHandler(t)
	defer db.Close()

	var capturedToken string
	mailer.SendFunc = func(to, subject, body string) error {
		// Extract token from body: "https://tunesday.fm/verify?token=<token>"
		idx := strings.Index(body, "?token=")
		if idx != -1 {
			rest := body[idx+7:]
			// Stop at first whitespace or newline.
			if end := strings.IndexAny(rest, " \t\r\n"); end != -1 {
				rest = rest[:end]
			}
			capturedToken = rest
		}
		return nil
	}

	// Register
	form := url.Values{}
	form.Set("email", "admin@example.com")
	form.Set("password", "password123")
	form.Set("password_confirm", "password123")

	req := httptest.NewRequest(http.MethodPost, "/register", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	h.Register(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rr.Code, rr.Body.String())
	}
	if capturedToken == "" {
		t.Fatal("expected verification token to be captured")
	}

	// Verify
	req = httptest.NewRequest(http.MethodGet, "/verify", nil)
	q := req.URL.Query()
	q.Set("token", capturedToken)
	req.URL.RawQuery = q.Encode()
	rr = httptest.NewRecorder()
	h.Verify(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "verified") {
		t.Fatalf("expected verification success message, got: %s", rr.Body.String())
	}

	// Login
	form = url.Values{}
	form.Set("email", "admin@example.com")
	form.Set("password", "password123")

	req = httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr = httptest.NewRecorder()
	h.Login(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Fatalf("expected redirect after login, got %d: %s", rr.Code, rr.Body.String())
	}
	loc := rr.Header().Get("Location")
	if loc != "/onboarding" {
		t.Fatalf("expected redirect to /onboarding, got %s", loc)
	}
}

func TestLoginUnverified(t *testing.T) {
	h, db, _ := setupTestHandler(t)
	defer db.Close()

	// Create an unverified user directly
	user, err := h.users.GetByEmail("unverified@example.com")
	if err != nil {
		t.Fatalf("get user: %v", err)
	}
	if user != nil {
		t.Fatal("expected user to not exist")
	}

	hash, err := auth.HashPassword("password123", 4)
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}

	if err := h.users.Create(&store.User{
		ID:            "test-user-id",
		Email:         "unverified@example.com",
		PasswordHash:  hash,
		EmailVerified: false,
	}); err != nil {
		t.Fatalf("create user: %v", err)
	}

	form := url.Values{}
	form.Set("email", "unverified@example.com")
	form.Set("password", "password123")

	req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	h.Login(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "verify") {
		t.Fatalf("expected verification required message, got: %s", rr.Body.String())
	}
}

func TestAuthMiddleware(t *testing.T) {
	h, db, _ := setupTestHandler(t)
	defer db.Close()

	hash, err := auth.HashPassword("password123", 4)
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}

	if err := h.users.Create(&store.User{
		ID:            "test-user-id",
		Email:         "verified@example.com",
		PasswordHash:  hash,
		EmailVerified: true,
	}); err != nil {
		t.Fatalf("create user: %v", err)
	}

	// Login to get a session cookie
	form := url.Values{}
	form.Set("email", "verified@example.com")
	form.Set("password", "password123")

	req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	h.Login(rr, req)

	cookies := rr.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatal("expected session cookie after login")
	}

	// Access protected route with cookie
	req = httptest.NewRequest(http.MethodGet, "/onboarding", nil)
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rr = httptest.NewRecorder()

	protected := auth.Middleware(h.sessions, h.users)(http.HandlerFunc(h.Onboarding))
	protected.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200 for authenticated user, got %d: %s", rr.Code, rr.Body.String())
	}

	// Access protected route without cookie
	req = httptest.NewRequest(http.MethodGet, "/onboarding", nil)
	rr = httptest.NewRecorder()
	protected.ServeHTTP(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Fatalf("expected redirect for unauthenticated user, got %d", rr.Code)
	}
}
