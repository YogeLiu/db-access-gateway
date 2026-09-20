package api

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/yogel/db-access-gateway/internal/auth"
	"github.com/yogel/db-access-gateway/internal/control"
	"github.com/yogel/db-access-gateway/internal/id"
)

const (
	sessionCookieName = "dbag_session"
	sessionTTL        = 8 * time.Hour
)

type WebAuth struct {
	store        *control.Store
	tokenPepper  string
	cookieSecure bool
}

func NewWebAuth(store *control.Store, tokenPepper string) *WebAuth {
	return &WebAuth{store: store, tokenPepper: tokenPepper}
}

func (a *WebAuth) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/auth/login", a.login)
	mux.HandleFunc("POST /api/v1/auth/logout", a.logout)
	mux.Handle("GET /api/v1/auth/me", a.RequireAny(http.HandlerFunc(a.me)))
	mux.Handle("POST /api/v1/auth/password", a.RequireAny(http.HandlerFunc(a.changePassword)))
}

func (a *WebAuth) principal(r *http.Request) (control.Principal, [32]byte, error) {
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil || strings.TrimSpace(cookie.Value) == "" {
		return control.Principal{}, [32]byte{}, errors.New("session missing")
	}
	digest := auth.Digest(a.tokenPepper, cookie.Value)
	principal, err := a.store.PrincipalBySessionDigest(r.Context(), digest)
	if err != nil {
		return control.Principal{}, [32]byte{}, errors.New("session invalid")
	}
	return principal, digest, nil
}

func (a *WebAuth) Principal(r *http.Request) (control.Principal, error) {
	principal, _, err := a.principal(r)
	return principal, err
}

func (a *WebAuth) RequireRole(role string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		principal, err := a.Principal(r)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "unauthorized", "login required")
			return
		}
		if principal.Role != role {
			writeError(w, http.StatusForbidden, "forbidden", "insufficient role")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (a *WebAuth) RequireAny(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := a.Principal(r); err != nil {
			writeError(w, http.StatusUnauthorized, "unauthorized", "login required")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (a *WebAuth) login(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	username := strings.TrimSpace(body.Username)
	principal, err := a.store.GetPrincipalByUsername(r.Context(), username)
	if err != nil || principal.Status != "active" || !auth.CheckPassword(principal.PasswordHash, body.Password) {
		writeError(w, http.StatusUnauthorized, "invalid_credentials", "账号或密码错误")
		return
	}
	if err := a.startSession(w, r.Context(), principal.ID); err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "登录会话创建失败")
		return
	}
	writeJSON(w, http.StatusOK, principal)
}

func (a *WebAuth) logout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(sessionCookieName); err == nil && cookie.Value != "" {
		_ = a.store.DeleteSession(r.Context(), auth.Digest(a.tokenPepper, cookie.Value))
	}
	a.clearSessionCookie(w)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (a *WebAuth) me(w http.ResponseWriter, r *http.Request) {
	principal, err := a.Principal(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "login required")
		return
	}
	writeJSON(w, http.StatusOK, principal)
}

func (a *WebAuth) changePassword(w http.ResponseWriter, r *http.Request) {
	principal, _, err := a.principal(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "login required")
		return
	}
	var body struct {
		CurrentPassword string `json:"current_password"`
		NewPassword     string `json:"new_password"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	if !auth.CheckPassword(principal.PasswordHash, body.CurrentPassword) {
		writeError(w, http.StatusBadRequest, "invalid_password", "当前密码错误")
		return
	}
	if !auth.ValidatePassword(body.NewPassword) {
		writeError(w, http.StatusBadRequest, "invalid_password", "新密码至少 8 个字符且不超过 72 个字节")
		return
	}
	hash, err := auth.HashPassword(body.NewPassword)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "密码更新失败")
		return
	}
	if err := a.store.UpdatePassword(r.Context(), principal.ID, hash); err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "密码更新失败")
		return
	}
	if err := a.store.DeleteSessionsForPrincipal(r.Context(), principal.ID); err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "登录会话更新失败")
		return
	}
	if err := a.startSession(w, r.Context(), principal.ID); err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "登录会话创建失败")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (a *WebAuth) startSession(w http.ResponseWriter, ctx context.Context, principalID string) error {
	plain, _, err := auth.NewToken()
	if err != nil {
		return err
	}
	expiresAt := time.Now().UTC().Add(sessionTTL)
	if err := a.store.InsertSession(ctx, id.New(), principalID, auth.Digest(a.tokenPepper, plain), expiresAt); err != nil {
		return err
	}
	http.SetCookie(w, &http.Cookie{Name: sessionCookieName, Value: plain, Path: "/", Expires: expiresAt,
		MaxAge: int(sessionTTL.Seconds()), HttpOnly: true, Secure: a.cookieSecure, SameSite: http.SameSiteLaxMode})
	return nil
}

func (a *WebAuth) clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{Name: sessionCookieName, Value: "", Path: "/", MaxAge: -1,
		HttpOnly: true, Secure: a.cookieSecure, SameSite: http.SameSiteLaxMode})
}
