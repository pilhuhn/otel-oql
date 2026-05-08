package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/pilhuhn/otel-oql/pkg/tenant"
)

// mockAuthenticator is a mock implementation of Authenticator for testing
type mockAuthenticator struct {
	validKeys map[string]struct {
		username string
		tenantID int
	}
}

func (m *mockAuthenticator) AuthenticateAPIKey(apiKey string) (string, int, bool) {
	creds, ok := m.validKeys[apiKey]
	if !ok {
		return "", 0, false
	}
	return creds.username, creds.tenantID, true
}

func TestHTTPMiddleware_ValidAPIKey(t *testing.T) {
	// Setup mock authenticator
	auth := &mockAuthenticator{
		validKeys: map[string]struct {
			username string
			tenantID int
		}{
			"valid-key-123": {"alice", 1},
		},
	}

	middleware := NewMiddleware(auth, false)

	// Create test handler
	called := false
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true

		// Verify tenant ID in context
		tenantID, ok := tenant.FromContext(r.Context())
		if !ok {
			t.Error("tenant ID not found in context")
		}
		if tenantID != 1 {
			t.Errorf("tenant ID = %d, want 1", tenantID)
		}

		// Verify username in context
		username, ok := UsernameFromContext(r.Context())
		if !ok {
			t.Error("username not found in context")
		}
		if username != "alice" {
			t.Errorf("username = %s, want alice", username)
		}

		w.WriteHeader(http.StatusOK)
	})

	// Create request with valid API key
	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer valid-key-123")

	// Execute middleware
	rr := httptest.NewRecorder()
	middleware.HTTPMiddleware(handler).ServeHTTP(rr, req)

	// Verify handler was called
	if !called {
		t.Error("handler was not called")
	}

	// Verify response
	if rr.Code != http.StatusOK {
		t.Errorf("status code = %d, want %d", rr.Code, http.StatusOK)
	}
}

func TestHTTPMiddleware_InvalidAPIKey(t *testing.T) {
	// Setup mock authenticator
	auth := &mockAuthenticator{
		validKeys: map[string]struct {
			username string
			tenantID int
		}{
			"valid-key-123": {"alice", 1},
		},
	}

	middleware := NewMiddleware(auth, false)

	// Create test handler
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called for invalid API key")
	})

	// Create request with invalid API key
	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer invalid-key")

	// Execute middleware
	rr := httptest.NewRecorder()
	middleware.HTTPMiddleware(handler).ServeHTTP(rr, req)

	// Verify unauthorized response
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("status code = %d, want %d", rr.Code, http.StatusUnauthorized)
	}
}

func TestHTTPMiddleware_MissingAuthHeader(t *testing.T) {
	auth := &mockAuthenticator{validKeys: make(map[string]struct {
		username string
		tenantID int
	})}
	middleware := NewMiddleware(auth, false)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called without auth header")
	})

	req := httptest.NewRequest("GET", "/test", nil)
	rr := httptest.NewRecorder()

	middleware.HTTPMiddleware(handler).ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("status code = %d, want %d", rr.Code, http.StatusUnauthorized)
	}
}

func TestHTTPMiddleware_InvalidAuthFormat(t *testing.T) {
	auth := &mockAuthenticator{validKeys: make(map[string]struct {
		username string
		tenantID int
	})}
	middleware := NewMiddleware(auth, false)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called with invalid auth format")
	})

	tests := []string{
		"valid-key-123",         // Missing "Bearer" prefix
		"Basic dXNlcjpwYXNz",    // Wrong auth type
		"Bearer",                // Missing key
		"Bearer key1 key2",      // Extra parts (should still work, but tests the split)
	}

	for _, authHeader := range tests {
		req := httptest.NewRequest("GET", "/test", nil)
		req.Header.Set("Authorization", authHeader)
		rr := httptest.NewRecorder()

		middleware.HTTPMiddleware(handler).ServeHTTP(rr, req)

		if rr.Code != http.StatusUnauthorized {
			t.Errorf("auth header %q: status code = %d, want %d", authHeader, rr.Code, http.StatusUnauthorized)
		}
	}
}

func TestHTTPMiddleware_TestMode(t *testing.T) {
	auth := &mockAuthenticator{validKeys: make(map[string]struct {
		username string
		tenantID int
	})}
	middleware := NewMiddleware(auth, true) // Test mode enabled

	called := false
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	// In test mode, request with tenant-id header should bypass auth
	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("tenant-id", "5")
	rr := httptest.NewRecorder()

	middleware.HTTPMiddleware(handler).ServeHTTP(rr, req)

	// Handler should be called
	if !called {
		t.Error("handler was not called in test mode")
	}

	// Should succeed
	if rr.Code != http.StatusOK {
		t.Errorf("status code = %d, want %d", rr.Code, http.StatusOK)
	}
}

func TestHTTPMiddleware_MultipleTenants(t *testing.T) {
	// Setup mock authenticator with multiple users/tenants
	auth := &mockAuthenticator{
		validKeys: map[string]struct {
			username string
			tenantID int
		}{
			"alice-key": {"alice", 1},
			"bob-key":   {"bob", 1}, // Same tenant as alice
			"carol-key": {"carol", 2},
		},
	}

	middleware := NewMiddleware(auth, false)

	tests := []struct {
		name           string
		apiKey         string
		wantUsername   string
		wantTenantID   int
		wantStatusCode int
	}{
		{"alice (tenant 1)", "alice-key", "alice", 1, http.StatusOK},
		{"bob (tenant 1)", "bob-key", "bob", 1, http.StatusOK},
		{"carol (tenant 2)", "carol-key", "carol", 2, http.StatusOK},
		{"invalid key", "invalid", "", 0, http.StatusUnauthorized},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Verify tenant ID
				tenantID, ok := tenant.FromContext(r.Context())
				if !ok {
					t.Error("tenant ID not found in context")
				}
				if tenantID != tt.wantTenantID {
					t.Errorf("tenant ID = %d, want %d", tenantID, tt.wantTenantID)
				}

				// Verify username
				username, ok := UsernameFromContext(r.Context())
				if !ok {
					t.Error("username not found in context")
				}
				if username != tt.wantUsername {
					t.Errorf("username = %s, want %s", username, tt.wantUsername)
				}

				w.WriteHeader(http.StatusOK)
			})

			req := httptest.NewRequest("GET", "/test", nil)
			req.Header.Set("Authorization", "Bearer "+tt.apiKey)
			rr := httptest.NewRecorder()

			middleware.HTTPMiddleware(handler).ServeHTTP(rr, req)

			if rr.Code != tt.wantStatusCode {
				t.Errorf("status code = %d, want %d", rr.Code, tt.wantStatusCode)
			}
		})
	}
}

func TestUsernameFromContext(t *testing.T) {
	ctx := context.Background()

	// Test without username
	_, ok := UsernameFromContext(ctx)
	if ok {
		t.Error("expected no username in empty context")
	}

	// Test with username
	ctx = withUsername(ctx, "alice")
	username, ok := UsernameFromContext(ctx)
	if !ok {
		t.Error("expected username in context")
	}
	if username != "alice" {
		t.Errorf("username = %s, want alice", username)
	}
}
