package api

import (
	"context"
	"encoding/json"
	apachemanager "goapache-manager/internal/apache"
	"goapache-manager/internal/auth"
	"goapache-manager/internal/helper"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
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
func (a *ApacheAPI) EnableVHost(
	w http.ResponseWriter,
	r *http.Request,
) {
	a.setVHostEnabled(w, r, true)
}

func (a *ApacheAPI) DisableVHost(
	w http.ResponseWriter,
	r *http.Request,
) {
	a.setVHostEnabled(w, r, false)
}

func (a *ApacheAPI) setVHostEnabled(
	w http.ResponseWriter,
	r *http.Request,
	enabled bool,
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

	domain := chi.URLParam(r, "domain")

	if domain == "" {
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

	var (
		result *helper.Response
		err    error
		action string
	)

	if enabled {
		action = "enabled"
		result, err = a.Helper.EnableVHost(ctx, domain)
	} else {
		action = "disabled"
		result, err = a.Helper.DisableVHost(ctx, domain)
	}

	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{
			"success": false,
			"error":   err.Error(),
			"output":  resultOutput(result),
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"success": true,
		"domain":  domain,
		"enabled": enabled,
		"message": "VirtualHost " + action + " successfully",
		"output":  resultOutput(result),
	})
}

func (a *ApacheAPI) EditVHost(w http.ResponseWriter, r *http.Request) {
	user, ok := auth.CurrentUser(r.Context())
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	if user.Role != "admin" && user.Role != "operator" {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	domain := chi.URLParam(r, "domain")

	r.Body = http.MaxBytesReader(w, r.Body, 8192)

	var input struct {
		DocumentRoot  string   `json:"document_root"`
		ServerAliases []string `json:"server_aliases"`
	}

	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(&input); err != nil {
		http.Error(w, "Invalid JSON body", http.StatusBadRequest)
		return
	}

	var extra interface{}
	if err := decoder.Decode(&extra); err != io.EOF {
		http.Error(w, "Request must contain only one JSON object", http.StatusBadRequest)
		return
	}

	if strings.TrimSpace(input.DocumentRoot) == "" {
		http.Error(w, "document_root is required", http.StatusBadRequest)
		return
	}

	if len(input.ServerAliases) > 50 {
		http.Error(w, "Too many ServerAliases", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 35*time.Second)
	defer cancel()

	result, err := a.Helper.EditVHost(
		ctx,
		domain,
		input.DocumentRoot,
		input.ServerAliases,
	)

	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"domain":  domain,
		"message": "VirtualHost updated successfully",
		"output":  resultOutput(result),
	})
}
