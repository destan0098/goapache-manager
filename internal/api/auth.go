package api

import (
	"database/sql"
	"encoding/json"
	"goapache-manager/internal/auth"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

type AuthAPI struct {
	DB *sql.DB

	// InsecureCookies disables the Secure flag on session cookies.
	// Only enable it (GOAPACHE_INSECURE_COOKIES=1) when the panel is
	// reached over plain HTTP on a trusted local network.
	InsecureCookies bool
}

// insecureCookies reports whether the GOAPACHE_INSECURE_COOKIES
// environment variable explicitly enables non-Secure cookies.
func insecureCookies() bool {
	value := strings.ToLower(strings.TrimSpace(
		os.Getenv("GOAPACHE_INSECURE_COOKIES"),
	))

	return value == "1" || value == "true" || value == "yes"
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func (a *AuthAPI) Login(
	w http.ResponseWriter,
	r *http.Request,
) {
	var input loginRequest

	r.Body = http.MaxBytesReader(w, r.Body, 8192)

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		http.Error(w, "Invalid request", 400)
		return
	}

	input.Username = strings.TrimSpace(input.Username)

	if input.Username == "" || input.Password == "" {
		http.Error(w, "Invalid credentials", 401)
		return
	}

	var id int64
	var hash string

	err := a.DB.QueryRow(`
		SELECT id, password_hash
		FROM users
		WHERE username = ? AND active = 1
	`, input.Username).Scan(&id, &hash)

	if err != nil || !auth.VerifyPassword(hash, input.Password) {
		http.Error(w, "Invalid credentials", 401)
		return
	}

	session, err := auth.CreateSession(a.DB, id)

	if err != nil {
		log.Printf("ERROR: CreateSession failed for user_id=%d: %v", id, err)

		http.Error(w, "Could not create session", http.StatusInternalServerError)
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "goapache_session",
		Value:    session.Token,
		Path:     "/",
		HttpOnly: true,
		Secure:   !a.InsecureCookies,
		SameSite: http.SameSiteStrictMode,
		Expires:  session.ExpiresAt,
	})

	http.SetCookie(w, &http.Cookie{
		Name:     "goapache_csrf",
		Value:    session.CSRFToken,
		Path:     "/",
		HttpOnly: false,
		Secure:   !a.InsecureCookies,
		SameSite: http.SameSiteStrictMode,
		Expires:  session.ExpiresAt,
	})

	writeJSON(w, http.StatusOK, map[string]any{
		"success": true,
		"csrf":    session.CSRFToken,
	})
}

func (a *AuthAPI) Logout(
	w http.ResponseWriter,
	r *http.Request,
) {
	cookie, err := r.Cookie("goapache_session")

	if err == nil {
		_ = auth.DeleteSession(a.DB, cookie.Value)
	}

	for _, name := range []string{
		"goapache_session",
		"goapache_csrf",
	} {
		http.SetCookie(w, &http.Cookie{
			Name:     name,
			Value:    "",
			Path:     "/",
			MaxAge:   -1,
			HttpOnly: name == "goapache_session",
			Secure:   !a.InsecureCookies,
			SameSite: http.SameSiteStrictMode,
		})
	}

	writeJSON(w, 200, map[string]bool{"success": true})
}

func (a *AuthAPI) Me(
	w http.ResponseWriter,
	r *http.Request,
) {
	user, ok := auth.CurrentUser(r)

	if !ok {
		http.Error(w, "Unauthorized", 401)
		return
	}

	writeJSON(w, 200, map[string]any{
		"id":       user.ID,
		"username": user.Username,
		"role":     user.Role,
	})
}

func writeJSON(
	w http.ResponseWriter,
	status int,
	data any,
) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

var _ = time.Second
