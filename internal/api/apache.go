package api

import (
	"context"
	"net/http"
	"time"

	"goapache-manager/internal/auth"
	"goapache-manager/internal/helper"
)

type ApacheAPI struct {
	Helper *helper.Client
}

func (a *ApacheAPI) Status(
	w http.ResponseWriter,
	r *http.Request,
) {
	a.run(w, r, "apache.status")
}

func (a *ApacheAPI) ConfigTest(
	w http.ResponseWriter,
	r *http.Request,
) {
	a.run(w, r, "apache.configtest")
}

func (a *ApacheAPI) Reload(
	w http.ResponseWriter,
	r *http.Request,
) {
	a.run(w, r, "apache.reload")
}

func (a *ApacheAPI) Restart(
	w http.ResponseWriter,
	r *http.Request,
) {
	a.run(w, r, "apache.restart")
}

func (a *ApacheAPI) run(
	w http.ResponseWriter,
	r *http.Request,
	action string,
) {
	user, ok := auth.CurrentUser(r)

	if !ok {
		http.Error(w, "Unauthorized", 401)
		return
	}

	if user.Role != "admin" && user.Role != "operator" {
		http.Error(w, "Forbidden", 403)
		return
	}

	ctx, cancel := context.WithTimeout(
		r.Context(),
		35*time.Second,
	)
	defer cancel()

	result, err := a.Helper.Execute(ctx, action)

	if err != nil {
		writeJSON(w, 502, map[string]any{
			"success": false,
			"error":   err.Error(),
			"output":  result.Output,
		})
		return
	}

	writeJSON(w, 200, result)
}
