package auth

import (
	"context"
	"database/sql"
	"net/http"
	"time"
)

type User struct {
	ID       int64
	Username string
	Role     string
	CSRF     string
}

type contextKey string

const userContextKey contextKey = "user"

func RequireAuth(db *sql.DB) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(
			w http.ResponseWriter,
			r *http.Request,
		) {
			cookie, err := r.Cookie("goapache_session")

			if err != nil {
				http.Error(w, "Unauthorized", 401)
				return
			}

			var user User

			err = db.QueryRow(`
				SELECT users.id, users.username,
				       users.role, sessions.csrf_token
				FROM sessions
				JOIN users ON users.id = sessions.user_id
				WHERE sessions.token_hash = ?
				  AND sessions.expires_at > ?
				  AND users.active = 1
			`,
				tokenHash(cookie.Value),
				time.Now().UTC(),
			).Scan(
				&user.ID,
				&user.Username,
				&user.Role,
				&user.CSRF,
			)

			if err != nil {
				http.Error(w, "Unauthorized", 401)
				return
			}

			ctx := context.WithValue(
				r.Context(),
				userContextKey,
				user,
			)

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func CurrentUser(r *http.Request) (User, bool) {
	user, ok := r.Context().Value(userContextKey).(User)
	return user, ok
}
