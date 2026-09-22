package api

import (
	"context"
	"crypto/subtle"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/yogel/db-access-gateway/internal/auth"
	"github.com/yogel/db-access-gateway/internal/authz"
	"github.com/yogel/db-access-gateway/internal/control"
	"github.com/yogel/db-access-gateway/internal/id"
	"github.com/yogel/db-access-gateway/internal/target"
)

type AdminHandler struct {
	store      *control.Store
	registry   *target.Registry
	adminToken string
	webAuth    *WebAuth
}

func NewAdminHandler(store *control.Store, registry *target.Registry, adminToken string, webAuth *WebAuth) *AdminHandler {
	return &AdminHandler{store: store, registry: registry, adminToken: adminToken, webAuth: webAuth}
}

func (h *AdminHandler) Register(mux *http.ServeMux) {
	mux.Handle("GET /api/v1/admin/resources/{id}/tables", h.protect(http.HandlerFunc(h.listMaskingTables)))
	mux.Handle("GET /api/v1/admin/resources/{id}/masking", h.protect(http.HandlerFunc(h.listMasking)))
	mux.Handle("PUT /api/v1/admin/resources/{id}/masking", h.protect(http.HandlerFunc(h.saveMasking)))
	mux.Handle("GET /api/v1/admin/resources/{id}/fields", h.protect(http.HandlerFunc(h.listMaskingFields)))
	mux.Handle("GET /api/v1/admin/users", h.protect(http.HandlerFunc(h.listUsers)))
	mux.Handle("POST /api/v1/admin/users", h.protect(http.HandlerFunc(h.createUser)))
	mux.Handle("PATCH /api/v1/admin/users/{id}", h.protect(http.HandlerFunc(h.updateUser)))
	mux.Handle("GET /api/v1/admin/resources", h.protect(http.HandlerFunc(h.listResources)))
	mux.Handle("POST /api/v1/admin/resources", h.protect(http.HandlerFunc(h.createResource)))
	mux.Handle("POST /api/v1/admin/resources/batch", h.protect(http.HandlerFunc(h.createResourceBatch)))
	mux.Handle("PATCH /api/v1/admin/resources/{id}", h.protect(http.HandlerFunc(h.updateResource)))
	mux.Handle("POST /api/v1/admin/resources/{id}/test", h.protect(http.HandlerFunc(h.testResource)))
	mux.Handle("GET /api/v1/admin/grants", h.protect(http.HandlerFunc(h.listGrants)))
	mux.Handle("POST /api/v1/admin/grants", h.protect(http.HandlerFunc(h.upsertGrant)))
	mux.Handle("PATCH /api/v1/admin/grants/{id}", h.protect(http.HandlerFunc(h.updateGrant)))
	mux.Handle("DELETE /api/v1/admin/grants/{id}", h.protect(http.HandlerFunc(h.revokeGrant)))
	mux.Handle("POST /api/v1/admin/authorize", h.protect(http.HandlerFunc(h.testAuthorization)))
	mux.Handle("GET /api/v1/admin/audits", h.protect(http.HandlerFunc(h.listAudits)))
}

func (h *AdminHandler) protect(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if h.webAuth != nil {
			if principal, err := h.webAuth.Principal(r); err == nil && principal.Role == "admin" {
				next.ServeHTTP(w, r)
				return
			}
		}
		token, err := auth.Bearer(r.Header.Get("Authorization"))
		if err != nil || subtle.ConstantTimeCompare([]byte(token), []byte(h.adminToken)) != 1 {
			w.Header().Set("WWW-Authenticate", `Bearer realm="DB Access Gateway Admin"`)
			writeError(w, http.StatusUnauthorized, "unauthorized", "invalid admin token")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (h *AdminHandler) listUsers(w http.ResponseWriter, r *http.Request) {
	items, err := h.store.ListPrincipals(r.Context())
	respond(w, items, err)
}
func (h *AdminHandler) createUser(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Username    string `json:"username"`
		DisplayName string `json:"display_name"`
		Password    string `json:"password"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	body.Username = strings.TrimSpace(body.Username)
	if body.Username == "" || len(body.Username) > 80 {
		writeError(w, 400, "invalid_user", "username is required and must be at most 80 characters")
		return
	}
	if !auth.ValidatePassword(body.Password) {
		writeError(w, 400, "invalid_password", "password must be at least 8 characters and no more than 72 bytes")
		return
	}
	passwordHash, err := auth.HashPassword(body.Password)
	if err != nil {
		writeError(w, 500, "internal", "password hashing failed")
		return
	}
	item, err := h.store.CreatePrincipal(r.Context(), body.Username, strings.TrimSpace(body.DisplayName), passwordHash)
	if err != nil {
		writeError(w, 409, "user_exists", "username already exists")
		return
	}
	h.adminAudit(r.Context(), "create_user", "principal", item.ID)
	writeJSON(w, 201, item)
}
func (h *AdminHandler) updateUser(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Status   string `json:"status"`
		Password string `json:"password"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	principal, err := h.store.GetPrincipal(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, 404, "not_found", "user not found")
		return
	}
	status := principal.Status
	if body.Status != "" {
		if body.Status != "active" && body.Status != "disabled" {
			writeError(w, 400, "invalid_status", "status must be active or disabled")
			return
		}
		status = body.Status
	}
	var passwordHash *string
	if body.Password != "" {
		if !auth.ValidatePassword(body.Password) {
			writeError(w, 400, "invalid_password", "password must be at least 8 characters and no more than 72 bytes")
			return
		}
		hash, hashErr := auth.HashPassword(body.Password)
		if hashErr != nil {
			writeError(w, 500, "internal", "password hashing failed")
			return
		}
		passwordHash = &hash
	}
	err = h.store.UpdatePrincipal(r.Context(), r.PathValue("id"), status, passwordHash)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, 404, "not_found", "user not found")
		return
	}
	if err != nil {
		writeError(w, 500, "internal", "update failed")
		return
	}
	if passwordHash != nil {
		_ = h.store.DeleteSessionsForPrincipal(r.Context(), r.PathValue("id"))
	}
	h.adminAudit(r.Context(), "update_user", "principal", r.PathValue("id"))
	writeJSON(w, 200, map[string]bool{"ok": true})
}
func (h *AdminHandler) listResources(w http.ResponseWriter, r *http.Request) {
	items, err := h.store.ListResources(r.Context())
	respond(w, items, err)
}

type resourceRequest struct {
	ResourceKey        string `json:"resource_key"`
	DisplayName        string `json:"display_name"`
	Host               string `json:"host"`
	Port               uint16 `json:"port"`
	DatabaseName       string `json:"database_name"`
	Username           string `json:"username"`
	Password           string `json:"password"`
	SecretRef          string `json:"secret_ref"` // Deprecated compatibility path.
	TLSMode            string `json:"tls_mode"`
	MaxRows            uint32 `json:"max_rows"`
	MaxWriteRows       uint32 `json:"max_write_rows"`
	StatementTimeoutMS uint32 `json:"statement_timeout_ms"`
	Enabled            bool   `json:"enabled"`
	Version            uint64 `json:"version"`
}

func validateResource(body resourceRequest, requirePassword bool) (control.Resource, error) {
	if strings.TrimSpace(body.ResourceKey) == "" || strings.TrimSpace(body.Host) == "" || strings.TrimSpace(body.DatabaseName) == "" || strings.TrimSpace(body.Username) == "" {
		return control.Resource{}, errors.New("resource_key, host, database_name and username are required")
	}
	if requirePassword && strings.TrimSpace(body.Password) == "" && strings.TrimSpace(body.SecretRef) == "" {
		return control.Resource{}, errors.New("password is required when creating a resource")
	}
	if body.Password != "" && body.SecretRef != "" {
		return control.Resource{}, errors.New("use password instead of secret_ref")
	}
	if body.Port == 0 {
		body.Port = 3306
	}
	if body.TLSMode == "" {
		body.TLSMode = "required"
	}
	if body.MaxRows == 0 {
		body.MaxRows = 1000
	}
	if body.MaxRows > 1000 {
		return control.Resource{}, errors.New("max_rows cannot exceed 1000")
	}
	if body.MaxWriteRows == 0 {
		body.MaxWriteRows = 100
	}
	if body.MaxWriteRows > 1000 {
		return control.Resource{}, errors.New("max_write_rows cannot exceed 1000")
	}
	if body.StatementTimeoutMS == 0 {
		body.StatementTimeoutMS = 10000
	}
	if body.StatementTimeoutMS > 10000 {
		return control.Resource{}, errors.New("statement_timeout_ms cannot exceed 10000")
	}
	validTLS := body.TLSMode == "required" || body.TLSMode == "preferred" || body.TLSMode == "skip_verify" || body.TLSMode == "disabled"
	if !validTLS {
		return control.Resource{}, errors.New("invalid tls_mode")
	}
	return control.Resource{ResourceKey: strings.TrimSpace(body.ResourceKey), DisplayName: strings.TrimSpace(body.DisplayName), Host: strings.TrimSpace(body.Host), Port: body.Port, DatabaseName: strings.TrimSpace(body.DatabaseName), Username: strings.TrimSpace(body.Username), Password: body.Password, SecretRef: strings.TrimSpace(body.SecretRef), TLSMode: body.TLSMode, MaxRows: body.MaxRows, MaxWriteRows: body.MaxWriteRows, StatementTimeoutMS: body.StatementTimeoutMS, Enabled: body.Enabled, Version: body.Version}, nil
}

func (h *AdminHandler) createResource(w http.ResponseWriter, r *http.Request) {
	var body resourceRequest
	if !decodeJSON(w, r, &body) {
		return
	}
	resource, err := validateResource(body, true)
	if err != nil {
		writeError(w, 400, "invalid_resource", err.Error())
		return
	}
	item, err := h.store.CreateResource(r.Context(), resource)
	if err != nil {
		writeError(w, 409, "resource_exists", "resource_key already exists")
		return
	}
	h.adminAudit(r.Context(), "create_resource", "resource", item.ID)
	writeJSON(w, 201, item)
}

type resourceBatchItem struct {
	ResourceKey  string `json:"resource_key"`
	DisplayName  string `json:"display_name"`
	DatabaseName string `json:"database_name"`
}

type resourceBatchRequest struct {
	Host               string              `json:"host"`
	Port               uint16              `json:"port"`
	Username           string              `json:"username"`
	Password           string              `json:"password"`
	TLSMode            string              `json:"tls_mode"`
	MaxRows            uint32              `json:"max_rows"`
	MaxWriteRows       uint32              `json:"max_write_rows"`
	StatementTimeoutMS uint32              `json:"statement_timeout_ms"`
	Enabled            bool                `json:"enabled"`
	Resources          []resourceBatchItem `json:"resources"`
}

func validateResourceBatch(body resourceBatchRequest) ([]control.Resource, error) {
	if len(body.Resources) == 0 {
		return nil, errors.New("resources must contain at least one database")
	}
	if len(body.Resources) > 50 {
		return nil, errors.New("resources cannot contain more than 50 databases")
	}
	seenKeys := make(map[string]struct{}, len(body.Resources))
	resources := make([]control.Resource, 0, len(body.Resources))
	for _, item := range body.Resources {
		resource, err := validateResource(resourceRequest{
			ResourceKey:        item.ResourceKey,
			DisplayName:        item.DisplayName,
			Host:               body.Host,
			Port:               body.Port,
			DatabaseName:       item.DatabaseName,
			Username:           body.Username,
			Password:           body.Password,
			TLSMode:            body.TLSMode,
			MaxRows:            body.MaxRows,
			MaxWriteRows:       body.MaxWriteRows,
			StatementTimeoutMS: body.StatementTimeoutMS,
			Enabled:            body.Enabled,
		}, true)
		if err != nil {
			return nil, err
		}
		if _, exists := seenKeys[resource.ResourceKey]; exists {
			return nil, fmt.Errorf("duplicate resource_key %q", resource.ResourceKey)
		}
		seenKeys[resource.ResourceKey] = struct{}{}
		resources = append(resources, resource)
	}
	return resources, nil
}

func (h *AdminHandler) createResourceBatch(w http.ResponseWriter, r *http.Request) {
	var body resourceBatchRequest
	if !decodeJSON(w, r, &body) {
		return
	}
	resources, err := validateResourceBatch(body)
	if err != nil {
		writeError(w, 400, "invalid_resource_batch", err.Error())
		return
	}
	items, err := h.store.CreateResources(r.Context(), resources)
	if err != nil {
		writeError(w, 409, "resource_exists", "one or more resource_key values already exist")
		return
	}
	for _, item := range items {
		h.adminAudit(r.Context(), "create_resource", "resource", item.ID)
	}
	writeJSON(w, 201, items)
}

func (h *AdminHandler) updateResource(w http.ResponseWriter, r *http.Request) {
	var body resourceRequest
	if !decodeJSON(w, r, &body) {
		return
	}
	resource, err := validateResource(body, false)
	if err != nil {
		writeError(w, 400, "invalid_resource", err.Error())
		return
	}
	resource.ID = r.PathValue("id")
	item, err := h.store.UpdateResource(r.Context(), resource, body.Version)
	if errors.Is(err, control.ErrConflict) {
		writeError(w, 409, "version_conflict", "resource was changed by another request")
		return
	}
	if err != nil {
		writeError(w, 500, "internal", "resource update failed")
		return
	}
	h.registry.Invalidate(item.ID)
	h.adminAudit(r.Context(), "update_resource", "resource", item.ID)
	writeJSON(w, 200, item)
}
func (h *AdminHandler) testResource(w http.ResponseWriter, r *http.Request) {
	resource, err := h.store.GetResource(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, 404, "not_found", "resource not found")
		return
	}
	h.registry.Invalidate(resource.ID)
	db, err := h.registry.Get(r.Context(), resource)
	if err == nil {
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		err = db.PingContext(ctx)
	}
	if err != nil {
		writeError(w, 422, "connection_failed", err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (h *AdminHandler) listGrants(w http.ResponseWriter, r *http.Request) {
	items, err := h.store.ListGrants(r.Context())
	respond(w, items, err)
}
func (h *AdminHandler) upsertGrant(w http.ResponseWriter, r *http.Request) {
	var body control.Grant
	if !decodeJSON(w, r, &body) {
		return
	}
	if !validateGrant(w, body) {
		return
	}
	item, err := h.store.UpsertGrant(r.Context(), body)
	if err != nil {
		writeError(w, 400, "invalid_grant", "grant references an unknown user or resource")
		return
	}
	h.adminAudit(r.Context(), "upsert_grant", "grant", item.ID)
	writeJSON(w, 201, item)
}

func (h *AdminHandler) updateGrant(w http.ResponseWriter, r *http.Request) {
	var body control.Grant
	if !decodeJSON(w, r, &body) {
		return
	}
	if !validateGrant(w, body) {
		return
	}
	body.ID = r.PathValue("id")
	item, err := h.store.UpdateGrant(r.Context(), body)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, 404, "not_found", "grant not found")
		return
	}
	if err != nil {
		writeError(w, 400, "invalid_grant", "grant conflicts with an existing authorization or references an unknown user or resource")
		return
	}
	h.adminAudit(r.Context(), "update_grant", "grant", item.ID)
	writeJSON(w, 200, item)
}

func validateGrant(w http.ResponseWriter, body control.Grant) bool {
	if _, err := authz.ParseAction(body.Action); err != nil {
		writeError(w, 400, "invalid_action", "action must be schema_read, query_read or query_write")
		return false
	}
	if body.RowLimit != nil && (*body.RowLimit == 0 || *body.RowLimit > 1000) {
		writeError(w, 400, "invalid_constraint", "row_limit must be 1..1000")
		return false
	}
	if body.StatementTimeoutMS != nil && (*body.StatementTimeoutMS < 100 || *body.StatementTimeoutMS > 10000) {
		writeError(w, 400, "invalid_constraint", "statement_timeout_ms must be 100..10000")
		return false
	}
	return true
}
func (h *AdminHandler) revokeGrant(w http.ResponseWriter, r *http.Request) {
	err := h.store.RevokeGrant(r.Context(), r.PathValue("id"))
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, 404, "not_found", "grant not found")
		return
	}
	if err != nil {
		writeError(w, 500, "internal", "revoke failed")
		return
	}
	h.adminAudit(r.Context(), "revoke_grant", "grant", r.PathValue("id"))
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (h *AdminHandler) testAuthorization(w http.ResponseWriter, r *http.Request) {
	var body struct {
		PrincipalID string `json:"principal_id"`
		ResourceID  string `json:"resource_id"`
		Action      string `json:"action"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	action, err := authz.ParseAction(body.Action)
	if err != nil {
		writeError(w, 400, "invalid_action", "invalid action")
		return
	}
	principal, err := h.store.GetPrincipal(r.Context(), body.PrincipalID)
	if err != nil || principal.Status != "active" {
		writeJSON(w, 200, map[string]any{"allowed": false, "reason": "principal is not active"})
		return
	}
	resource, err := h.store.GetResource(r.Context(), body.ResourceID)
	if err != nil || !resource.Enabled {
		writeJSON(w, 200, map[string]any{"allowed": false, "reason": "resource is not enabled"})
		return
	}
	grants, err := h.store.EffectiveGrants(r.Context(), body.PrincipalID, body.ResourceID)
	if err != nil {
		writeError(w, 500, "internal", "authorization lookup failed")
		return
	}
	decision := authz.Evaluate(grants, action)
	writeJSON(w, 200, map[string]any{"allowed": decision.Allowed, "constraints": decision.Constraints})
}
func (h *AdminHandler) listAudits(w http.ResponseWriter, r *http.Request) {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	pageSize, _ := strconv.Atoi(r.URL.Query().Get("page_size"))
	items, err := h.store.ListAudits(r.Context(), page, pageSize)
	respond(w, items, err)
}

func (h *AdminHandler) adminAudit(ctx context.Context, action, objectType, objectID string) {
	_ = h.store.InsertAdminAudit(ctx, id.New(), action, objectType, objectID, "SUCCEEDED")
}
func respond(w http.ResponseWriter, value any, err error) {
	if err != nil {
		writeError(w, 500, "internal", "operation failed")
		return
	}
	writeJSON(w, 200, value)
}
