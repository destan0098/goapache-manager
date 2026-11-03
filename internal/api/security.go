package api

import (
	"crypto/subtle"
	"net/http"
)

func CSRF(next http.Handler) http.Handler {
	return http.HandlerFunc(func(
		w http.ResponseWriter,
		r *http.Request,
	) {
		switch r.Method {
		case http.MethodPost, http.MethodPut,
			http.MethodPatch, http.MethodDelete:

			cookie, err := r.Cookie("goapache_csrf")
			header := r.Header.Get("X-CSRF-Token")

			if err != nil || header == "" ||
				subtle.ConstantTimeCompare(
					[]byte(cookie.Value),
					[]byte(header),
				) != 1 {
				http.Error(w, "CSRF validation failed", 403)
				return
			}
		}

		next.ServeHTTP(w, r)
	})
}
