package auth

import (
	"context"
	"net/http"

	"tunesday/tunesday.online/internal/store"
)

// Middleware returns an HTTP middleware that requires an authenticated user.
func Middleware(sessions *SessionStore, users *store.UserStore) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			userID := sessions.GetUserID(r)
			if userID == "" {
				http.Redirect(w, r, "/login", http.StatusSeeOther)
				return
			}

			user, err := users.GetByID(userID)
			if err != nil || user == nil {
				http.Redirect(w, r, "/login", http.StatusSeeOther)
				return
			}

			// Sliding session: refresh the cookie expiry on every request,
			// so regular users never have to log in again.
			if err := sessions.SetUserID(w, r, user.ID); err != nil {
				http.Redirect(w, r, "/login", http.StatusSeeOther)
				return
			}

			ctx := WithUser(r.Context(), user)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// UserFromContext returns the authenticated user from context, or nil.
func UserFromContext(ctx context.Context) *store.User {
	if u, ok := ctx.Value(userContextKey).(*store.User); ok {
		return u
	}
	return nil
}

// WithUser stores the user in the context.
func WithUser(ctx context.Context, user *store.User) context.Context {
	return context.WithValue(ctx, userContextKey, user)
}

type contextKey int

const userContextKey contextKey = iota
