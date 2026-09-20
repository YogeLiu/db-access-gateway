package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestTokenSecretRequiresWebSession(t *testing.T) {
	mux := http.NewServeMux()
	NewUserHandler(nil, NewWebAuth(nil, "test-pepper"), "test-pepper").Register(mux)
	for _, bearer := range []string{"", "Bearer ordinary-access-token"} {
		req := httptest.NewRequest("GET", "/api/v1/me/tokens/token-id/secret", nil)
		req.Header.Set("Authorization", bearer)
		response := httptest.NewRecorder()
		mux.ServeHTTP(response, req)
		if response.Code != http.StatusUnauthorized {
			t.Fatalf("expected login required, got %d", response.Code)
		}
	}
}
