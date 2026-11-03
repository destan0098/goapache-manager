package api

import (
	"context"
	"encoding/json"
	"goapache-manager/internal/auth"
	"goapache-manager/internal/helper"
	"io"
	"net/http"
	"time"

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
func (a *ApacheAPI) CreateVHost(
	w http.ResponseWriter,
	r *http.Request,
) {
	user, ok := auth.CurrentUser(r)

	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]any{
			"success": false,
			"error":   "Unauthorized",
		})
		return
	}

	if user.Role != "admin" && user.Role != "operator" {
		writeJSON(w, http.StatusForbidden, map[string]any{
			"success": false,
			"error":   "Permission denied",
		})
		return
	}

	var request struct {
		Domain string `json:"domain"`
	}

	r.Body = http.MaxBytesReader(w, r.Body, 4096)

	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(&request); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"success": false,
			"error":   "Invalid JSON request",
		})
		return
	}

	// Reject additional JSON values or trailing content.
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"success": false,
			"error":   "Unexpected trailing JSON data",
		})
		return
	}

	if request.Domain == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"success": false,
			"error":   "Domain is required",
		})
		return
	}

	ctx, cancel := context.WithTimeout(
		r.Context(),
		35*time.Second,
	)
	defer cancel()

	result, err := a.Helper.CreateVHost(ctx, request.Domain)

	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{
			"success": false,
			"error":   err.Error(),
			"output":  resultOutput(result),
		})
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"success": true,
		"domain":  request.Domain,
		"output":  result.Output,
		"message": "VirtualHost created successfully",
	})
}

func resultOutput(result *helper.Response) string {
	if result == nil {
		return ""
	}

	return result.Output
}
