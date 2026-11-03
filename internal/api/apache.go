package api

import (
	"context"
	"net/http"
	"time"

	"goapache-manager/internal/auth"
	"goapache-manager/internal/helper"

	apachemanager "goapache-manager/internal/apache"
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
func (a *ApacheAPI) VHosts(
	w http.ResponseWriter,
	r *http.Request,
) {
	user, _ := auth.CurrentUser(r)

	if user.Role != "admin" && user.Role != "operator" {
		writeJSON(w, http.StatusForbidden, map[string]string{
			"error": "permission denied",
		})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 35*time.Second)
	defer cancel()

	result, err := a.Helper.Execute(ctx, "apache.vhosts.list")
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{
			"error": err.Error(),
		})
		return
	}

	vhosts := apachemanager.ParseVHosts(result.Output)

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"count":   len(vhosts),
		"vhosts":  vhosts,
	})
}
