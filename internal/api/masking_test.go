package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMaskingRequiresAdmin(t *testing.T) {
	mux := http.NewServeMux()
	NewAdminHandler(nil, nil, "test-administrator-token", nil).Register(mux)
	for _, path := range []string{"/api/v1/admin/resources/r/fields", "/api/v1/admin/resources/r/tables", "/api/v1/admin/resources/r/masking"} {
		for _, method := range []string{"GET", "PUT"} {
			if method == "PUT" && !strings.HasSuffix(path, "masking") {
				continue
			}
			req := httptest.NewRequest(method, path, nil)
			req.Header.Set("Authorization", "Bearer ordinary-user-token")
			response := httptest.NewRecorder()
			mux.ServeHTTP(response, req)
			if response.Code != 401 {
				t.Fatalf("%s %s: %d", method, path, response.Code)
			}
		}
	}
	req := httptest.NewRequest("PUT", "/api/v1/admin/resources/r/masking", strings.NewReader(`{"mode":"unknown"}`))
	req.Header.Set("Authorization", "Bearer test-administrator-token")
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, req)
	if response.Code != 400 {
		t.Fatalf("invalid mode accepted: %d", response.Code)
	}
}

func TestMaskingInputValidation(t *testing.T) {
	mux := http.NewServeMux()
	NewAdminHandler(nil, nil, "test-administrator-token", nil).Register(mux)
	for _, tc := range []struct{ method, path, body string }{
		{"GET", "/api/v1/admin/resources/r/fields", ""},
		{"PUT", "/api/v1/admin/resources/r/masking", `{"mode":"partial","keep_prefix":-1}`},
		{"PUT", "/api/v1/admin/resources/r/masking", `{"mode":"partial","keep_suffix":65}`},
	} {
		req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
		req.Header.Set("Authorization", "Bearer test-administrator-token")
		response := httptest.NewRecorder()
		mux.ServeHTTP(response, req)
		if response.Code != 400 {
			t.Fatalf("%s %s: got %d", tc.method, tc.body, response.Code)
		}
	}
}
