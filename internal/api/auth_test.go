package api

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"goapache-manager/internal/database"

	"golang.org/x/crypto/bcrypt"
)

func TestLoginCookieSecureFlag(t *testing.T) {
	cases := []struct {
		name       string
		env        string
		wantSecure bool
	}{
		{"secure by default", "", true},
		{"opt-out for trusted HTTP", "1", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("GOAPACHE_INSECURE_COOKIES", tc.env)

			db, err := database.Open(filepath.Join(t.TempDir(), "test.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()

			hash, err := bcrypt.GenerateFromPassword(
				[]byte("correct-horse-battery"),
				bcrypt.MinCost,
			)
			if err != nil {
				t.Fatal(err)
			}

			if _, err := db.Conn.Exec(`
				INSERT INTO users (username, password_hash, role)
				VALUES ('admin', ?, 'admin')
			`, string(hash)); err != nil {
				t.Fatal(err)
			}

			body := `{"username":"admin","password":"correct-horse-battery"}`
			req := httptest.NewRequest(
				http.MethodPost,
				"/api/v1/auth/login",
				bytes.NewBufferString(body),
			)
			rec := httptest.NewRecorder()

			NewRouter(db.Conn).ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("login status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body)
			}

			cookies := rec.Result().Cookies()
			if len(cookies) != 2 {
				t.Fatalf("got %d cookies, want 2", len(cookies))
			}

			for _, cookie := range cookies {
				if cookie.Secure != tc.wantSecure {
					t.Errorf(
						"cookie %q Secure = %v, want %v",
						cookie.Name,
						cookie.Secure,
						tc.wantSecure,
					)
				}
			}
		})
	}
}
