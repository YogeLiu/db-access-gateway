package api

import (
	"database/sql"
	"net/http"
	"strings"
	"time"

	"github.com/yogel/db-access-gateway/internal/auth"
	"github.com/yogel/db-access-gateway/internal/control"
	"github.com/yogel/db-access-gateway/internal/id"
)

type UserHandler struct {
	store       *control.Store
	webAuth     *WebAuth
	tokenPepper string
}

func NewUserHandler(store *control.Store, webAuth *WebAuth, tokenPepper string) *UserHandler {
	return &UserHandler{store: store, webAuth: webAuth, tokenPepper: tokenPepper}
}

func (h *UserHandler) Register(mux *http.ServeMux) {
	mux.Handle("GET /api/v1/me/access", h.webAuth.RequireRole("user", http.HandlerFunc(h.listAccess)))
	mux.Handle("GET /api/v1/me/tokens", h.webAuth.RequireRole("user", http.HandlerFunc(h.listTokens)))
	mux.Handle("GET /api/v1/me/tokens/{id}/secret", h.webAuth.RequireRole("user", http.HandlerFunc(h.tokenSecret)))
	mux.Handle("POST /api/v1/me/tokens", h.webAuth.RequireRole("user", http.HandlerFunc(h.createToken)))
	mux.Handle("DELETE /api/v1/me/tokens/{id}", h.webAuth.RequireRole("user", http.HandlerFunc(h.deleteToken)))
}

func (h *UserHandler) listAccess(w http.ResponseWriter, r *http.Request) {
	principal, err := h.webAuth.Principal(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "login required")
		return
	}
	items, err := h.store.ListPrincipalAccess(r.Context(), principal.ID)
	respond(w, map[string]any{"items": items}, err)
}

func (h *UserHandler) listTokens(w http.ResponseWriter, r *http.Request) {
	principal, err := h.webAuth.Principal(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "login required")
		return
	}
	items, err := h.store.ListTokens(r.Context(), principal.ID)
	respond(w, map[string]any{"items": items}, err)
}

func (h *UserHandler) createToken(w http.ResponseWriter, r *http.Request) {
	principal, err := h.webAuth.Principal(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "login required")
		return
	}
	var body struct {
		Name          string `json:"name"`
		ExpiresInDays *int   `json:"expires_in_days"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	name := strings.TrimSpace(body.Name)
	if name == "" {
		name = "MCP access token"
	}
	if len([]rune(name)) > 100 {
		writeError(w, http.StatusBadRequest, "invalid_name", "token name must be at most 100 characters")
		return
	}
	plain, prefix, err := auth.NewToken()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "token generation failed")
		return
	}
	var expires *time.Time
	if body.ExpiresInDays != nil {
		if *body.ExpiresInDays < 1 || *body.ExpiresInDays > 365 {
			writeError(w, http.StatusBadRequest, "invalid_expiry", "expires_in_days must be 1..365")
			return
		}
		value := time.Now().UTC().Add(time.Duration(*body.ExpiresInDays) * 24 * time.Hour)
		expires = &value
	}
	tokenID := id.New()
	ciphertext, err := auth.SealToken(h.tokenPepper, tokenID, principal.ID, plain)
	if err != nil {
		writeError(w, 500, "internal", "token encryption failed")
		return
	}
	if err := h.store.InsertToken(r.Context(), tokenID, principal.ID, name, prefix, auth.Digest(h.tokenPepper, plain), expires, ciphertext); err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "token creation failed")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusCreated, map[string]any{"id": tokenID, "name": name, "token": plain, "prefix": prefix, "expires_at": expires})
}

func (h *UserHandler) tokenSecret(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	principal, err := h.webAuth.Principal(r)
	if err != nil {
		writeError(w, 401, "unauthorized", "login required")
		return
	}
	tokenID := r.PathValue("id")
	sealed, err := h.store.TokenCiphertext(r.Context(), tokenID, principal.ID)
	if err == sql.ErrNoRows {
		writeError(w, 404, "not_found", "Token 不存在或已失效")
		return
	}
	if err != nil {
		writeError(w, 500, "internal", "无法加载 Token")
		return
	}
	if len(sealed) == 0 {
		writeError(w, 409, "legacy_token", "旧 Token 未保存可恢复原文，请新建 Token 以自动生成配置；旧 Token 仍可正常使用。")
		return
	}
	plain, err := auth.OpenToken(h.tokenPepper, tokenID, principal.ID, sealed)
	if err != nil {
		writeError(w, 500, "internal", "无法解密 Token，请检查服务器密钥配置")
		return
	}
	writeJSON(w, 200, map[string]string{"token": plain})
}

func (h *UserHandler) deleteToken(w http.ResponseWriter, r *http.Request) {
	principal, err := h.webAuth.Principal(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "login required")
		return
	}
	err = h.store.DeleteTokenForPrincipal(r.Context(), r.PathValue("id"), principal.ID)
	if err == sql.ErrNoRows {
		writeError(w, http.StatusNotFound, "not_found", "token not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "token deletion failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
