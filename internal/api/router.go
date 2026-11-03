package api

import (
	"database/sql"
	"net/http"
	"time"

	"goapache-manager/internal/auth"
	"goapache-manager/internal/helper"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

func NewRouter(db *sql.DB) http.Handler {
	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(45 * time.Second))

	authAPI := &AuthAPI{DB: db}

	apacheAPI := &ApacheAPI{
		Helper: helper.NewClient(
			"/run/goapache/helper.sock",
		),
	}

	r.Get("/api/v1/health", func(
		w http.ResponseWriter,
		r *http.Request,
	) {
		writeJSON(w, 200, map[string]string{
			"status": "ok",
		})
	})

	r.Post("/api/v1/auth/login", authAPI.Login)

	r.Group(func(p chi.Router) {
		p.Use(auth.RequireAuth(db))
		p.Use(CSRF)
		p.Get("/api/v1/apache/vhosts/{domain}", apacheAPI.VHostDetails)
		p.Get("/api/v1/apache/vhosts", apacheAPI.VHosts)
		p.Post("/api/v1/apache/vhosts", apacheAPI.CreateVHost)
		p.Post(
			"/api/v1/apache/vhosts/{domain}/enable",
			apacheAPI.EnableVHost,
		)

		p.Post(
			"/api/v1/apache/vhosts/{domain}/disable",
			apacheAPI.DisableVHost,
		)
		p.Put("/api/v1/apache/vhosts/{domain}", apacheAPI.EditVHost)
		p.Delete("/api/v1/apache/vhosts/{domain}", apacheAPI.DeleteVHost)
		p.Get(
			"/api/v1/apache/vhosts/{domain}/backups",
			apacheAPI.ListVHostBackups,
		)

		p.Post(
			"/api/v1/apache/vhosts/{domain}/backups/{id}/restore",
			apacheAPI.RestoreVHostBackup,
		)

		p.Delete(
			"/api/v1/apache/vhosts/{domain}/backups/{id}",
			apacheAPI.DeleteVHostBackup,
		)
		p.Post("/api/v1/auth/logout", authAPI.Logout)
		p.Get("/api/v1/auth/me", authAPI.Me)

		p.Get("/api/v1/apache/status", apacheAPI.Status)
		p.Post("/api/v1/apache/configtest", apacheAPI.ConfigTest)
		p.Post("/api/v1/apache/reload", apacheAPI.Reload)
		p.Post("/api/v1/apache/restart", apacheAPI.Restart)
	})

	return r
}
