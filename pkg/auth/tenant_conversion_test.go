package auth_test

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/pilhuhn/otel-oql/pkg/auth"
	"github.com/pilhuhn/otel-oql/pkg/tenant"
	"github.com/pilhuhn/otel-oql/pkg/userstore"
)

// TestTenantIDConversionForLargeValues verifies that tenant IDs >= 10 are handled correctly
func TestTenantIDConversionForLargeValues(t *testing.T) {
	tmpDir := t.TempDir()

	// Create users.csv with large tenant IDs
	usersFile := filepath.Join(tmpDir, "users.csv")
	usersData := `username,tenant_id
user10,10
user100,100
user999,999
`
	if err := os.WriteFile(usersFile, []byte(usersData), 0600); err != nil {
		t.Fatalf("Failed to create users.csv: %v", err)
	}

	// Create api-keys.csv
	apiKeysFile := filepath.Join(tmpDir, "api-keys.csv")
	apiKeysData := `username,api_key
user10,key10
user100,key100
user999,key999
`
	if err := os.WriteFile(apiKeysFile, []byte(apiKeysData), 0600); err != nil {
		t.Fatalf("Failed to create api-keys.csv: %v", err)
	}

	// Initialize user store
	store, err := userstore.NewFileStore(usersFile, apiKeysFile)
	if err != nil {
		t.Fatalf("Failed to create user store: %v", err)
	}

	// Create auth middleware
	authMW := auth.NewMiddleware(store, false)

	// Test cases with large tenant IDs
	tests := []struct {
		apiKey         string
		expectedTenant int
		expectedUser   string
	}{
		{"key10", 10, "user10"},
		{"key100", 100, "user100"},
		{"key999", 999, "user999"},
	}

	for _, tt := range tests {
		t.Run("tenant_"+tt.expectedUser, func(t *testing.T) {
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				tenantID, ok := tenant.FromContext(r.Context())
				if !ok {
					t.Error("tenant ID not found in context")
					http.Error(w, "No tenant ID", http.StatusInternalServerError)
					return
				}

				if tenantID != tt.expectedTenant {
					t.Errorf("tenant ID = %d, want %d", tenantID, tt.expectedTenant)
				}

				username, ok := auth.UsernameFromContext(r.Context())
				if !ok {
					t.Error("username not found in context")
					http.Error(w, "No username", http.StatusInternalServerError)
					return
				}

				if username != tt.expectedUser {
					t.Errorf("username = %s, want %s", username, tt.expectedUser)
				}

				w.WriteHeader(http.StatusOK)
			})

			req := httptest.NewRequest("GET", "/test", nil)
			req.Header.Set("Authorization", "Bearer "+tt.apiKey)
			rr := httptest.NewRecorder()

			authMW.HTTPMiddleware(handler).ServeHTTP(rr, req)

			if rr.Code != http.StatusOK {
				t.Errorf("status code = %d, want %d", rr.Code, http.StatusOK)
			}
		})
	}
}

// TestTestModeTenantIDConversionForLargeValues verifies test mode with large tenant IDs
func TestTestModeTenantIDConversionForLargeValues(t *testing.T) {
	tmpDir := t.TempDir()

	// Create minimal user files
	usersFile := filepath.Join(tmpDir, "users.csv")
	usersData := `username,tenant_id
testuser,50
`
	if err := os.WriteFile(usersFile, []byte(usersData), 0600); err != nil {
		t.Fatalf("Failed to create users.csv: %v", err)
	}

	apiKeysFile := filepath.Join(tmpDir, "api-keys.csv")
	apiKeysData := `username,api_key
testuser,testkey
`
	if err := os.WriteFile(apiKeysFile, []byte(apiKeysData), 0600); err != nil {
		t.Fatalf("Failed to create api-keys.csv: %v", err)
	}

	store, err := userstore.NewFileStore(usersFile, apiKeysFile)
	if err != nil {
		t.Fatalf("Failed to create user store: %v", err)
	}

	authMW := auth.NewMiddleware(store, true) // Test mode

	// Test with tenant-id header using large value
	t.Run("tenant_header_large_value", func(t *testing.T) {
		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			tenantID, ok := tenant.FromContext(r.Context())
			if !ok {
				t.Error("tenant ID not found in context")
				http.Error(w, "No tenant ID", http.StatusInternalServerError)
				return
			}

			expectedTenant := 42
			if tenantID != expectedTenant {
				t.Errorf("tenant ID = %d, want %d", tenantID, expectedTenant)
			}

			w.WriteHeader(http.StatusOK)
		})

		req := httptest.NewRequest("GET", "/test", nil)
		req.Header.Set("tenant-id", "42")
		rr := httptest.NewRecorder()

		authMW.HTTPMiddleware(handler).ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("status code = %d, want %d", rr.Code, http.StatusOK)
		}
	})
}
