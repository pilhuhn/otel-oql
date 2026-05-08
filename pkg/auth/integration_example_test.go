package auth_test

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/pilhuhn/otel-oql/pkg/auth"
	"github.com/pilhuhn/otel-oql/pkg/tenant"
	"github.com/pilhuhn/otel-oql/pkg/userstore"
)

// TestIntegrationExample demonstrates end-to-end authentication flow
func TestIntegrationExample(t *testing.T) {
	// Create temporary directory for test files
	tmpDir := t.TempDir()

	// Create users.csv
	usersFile := filepath.Join(tmpDir, "users.csv")
	usersData := `username,tenant_id
alice,1
bob,1
charlie,2
`
	if err := os.WriteFile(usersFile, []byte(usersData), 0600); err != nil {
		t.Fatalf("Failed to create users.csv: %v", err)
	}

	// Create api-keys.csv
	apiKeysFile := filepath.Join(tmpDir, "api-keys.csv")
	apiKeysData := `username,api_key
alice,oql_key_alice_123456789abcdef
bob,oql_key_bob_987654321fedcba
charlie,oql_key_charlie_abcdef123456
`
	if err := os.WriteFile(apiKeysFile, []byte(apiKeysData), 0600); err != nil {
		t.Fatalf("Failed to create api-keys.csv: %v", err)
	}

	// Initialize user store
	store, err := userstore.NewFileStore(usersFile, apiKeysFile)
	if err != nil {
		t.Fatalf("Failed to create user store: %v", err)
	}

	// Create auth middleware (production mode)
	authMW := auth.NewMiddleware(store, false)

	// Create a test handler that verifies tenant ID
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Extract tenant ID from context
		tenantID, ok := tenant.FromContext(r.Context())
		if !ok {
			http.Error(w, "No tenant ID in context", http.StatusInternalServerError)
			return
		}

		// Extract username from context
		username, ok := auth.UsernameFromContext(r.Context())
		if !ok {
			http.Error(w, "No username in context", http.StatusInternalServerError)
			return
		}

		// Return success with tenant and username
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK: tenant=" + strconv.Itoa(tenantID) + " user=" + username))
	})

	// Wrap handler with auth middleware
	authenticatedHandler := authMW.HTTPMiddleware(handler)

	// Test 1: Alice authenticates successfully (tenant 1)
	t.Run("Alice_Auth_Success", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/test", nil)
		req.Header.Set("Authorization", "Bearer oql_key_alice_123456789abcdef")
		rr := httptest.NewRecorder()

		authenticatedHandler.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("Expected 200, got %d", rr.Code)
		}

		expected := "OK: tenant=1 user=alice"
		if rr.Body.String() != expected {
			t.Errorf("Expected %q, got %q", expected, rr.Body.String())
		}
	})

	// Test 2: Bob authenticates successfully (same tenant as Alice)
	t.Run("Bob_Auth_Success", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/test", nil)
		req.Header.Set("Authorization", "Bearer oql_key_bob_987654321fedcba")
		rr := httptest.NewRecorder()

		authenticatedHandler.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("Expected 200, got %d", rr.Code)
		}

		// Bob should also be in tenant 1
		expected := "OK: tenant=1 user=bob"
		if rr.Body.String() != expected {
			t.Errorf("Expected %q, got %q", expected, rr.Body.String())
		}
	})

	// Test 3: Charlie authenticates successfully (different tenant)
	t.Run("Charlie_Auth_Success", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/test", nil)
		req.Header.Set("Authorization", "Bearer oql_key_charlie_abcdef123456")
		rr := httptest.NewRecorder()

		authenticatedHandler.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("Expected 200, got %d", rr.Code)
		}

		// Charlie should be in tenant 2
		expected := "OK: tenant=2 user=charlie"
		if rr.Body.String() != expected {
			t.Errorf("Expected %q, got %q", expected, rr.Body.String())
		}
	})

	// Test 4: Invalid API key fails
	t.Run("Invalid_API_Key", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/test", nil)
		req.Header.Set("Authorization", "Bearer invalid_key")
		rr := httptest.NewRecorder()

		authenticatedHandler.ServeHTTP(rr, req)

		if rr.Code != http.StatusUnauthorized {
			t.Errorf("Expected 401, got %d", rr.Code)
		}
	})

	// Test 5: Missing Authorization header fails
	t.Run("Missing_Authorization", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/test", nil)
		rr := httptest.NewRecorder()

		authenticatedHandler.ServeHTTP(rr, req)

		if rr.Code != http.StatusUnauthorized {
			t.Errorf("Expected 401, got %d", rr.Code)
		}
	})
}

// TestIntegrationTestMode demonstrates test mode fallback behavior
func TestIntegrationTestMode(t *testing.T) {
	// Create temporary directory for test files
	tmpDir := t.TempDir()

	// Create minimal user files
	usersFile := filepath.Join(tmpDir, "users.csv")
	usersData := `username,tenant_id
testuser,5
`
	if err := os.WriteFile(usersFile, []byte(usersData), 0600); err != nil {
		t.Fatalf("Failed to create users.csv: %v", err)
	}

	apiKeysFile := filepath.Join(tmpDir, "api-keys.csv")
	apiKeysData := `username,api_key
testuser,test_key_123
`
	if err := os.WriteFile(apiKeysFile, []byte(apiKeysData), 0600); err != nil {
		t.Fatalf("Failed to create api-keys.csv: %v", err)
	}

	// Initialize user store
	store, err := userstore.NewFileStore(usersFile, apiKeysFile)
	if err != nil {
		t.Fatalf("Failed to create user store: %v", err)
	}

	// Create auth middleware in TEST MODE
	authMW := auth.NewMiddleware(store, true)

	// Create test handler
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tenantID, ok := tenant.FromContext(r.Context())
		if !ok {
			http.Error(w, "No tenant ID", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK: tenant=" + strconv.Itoa(tenantID)))
	})

	authenticatedHandler := authMW.HTTPMiddleware(handler)

	// Test 1: API key authentication still works
	t.Run("TestMode_API_Key_Works", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/test", nil)
		req.Header.Set("Authorization", "Bearer test_key_123")
		rr := httptest.NewRecorder()

		authenticatedHandler.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("Expected 200, got %d", rr.Code)
		}
	})

	// Test 2: Tenant-id header fallback works in test mode
	t.Run("TestMode_TenantID_Header_Fallback", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/test", nil)
		req.Header.Set("tenant-id", "7")
		rr := httptest.NewRecorder()

		authenticatedHandler.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("Expected 200, got %d", rr.Code)
		}

		expected := "OK: tenant=7"
		if rr.Body.String() != expected {
			t.Errorf("Expected %q, got %q", expected, rr.Body.String())
		}
	})

	// Test 3: No auth at all defaults to tenant 0 in test mode
	t.Run("TestMode_Default_Tenant", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/test", nil)
		rr := httptest.NewRecorder()

		authenticatedHandler.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("Expected 200, got %d", rr.Code)
		}

		expected := "OK: tenant=0"
		if rr.Body.String() != expected {
			t.Errorf("Expected %q, got %q", expected, rr.Body.String())
		}
	})
}
