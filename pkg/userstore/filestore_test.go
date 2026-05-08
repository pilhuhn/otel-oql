package userstore

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFileStore_LoadUsers(t *testing.T) {
	// Create temporary directory
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
alice,key-alice-123
bob,key-bob-456
charlie,key-charlie-789
`
	if err := os.WriteFile(apiKeysFile, []byte(apiKeysData), 0600); err != nil {
		t.Fatalf("Failed to create api-keys.csv: %v", err)
	}

	// Create FileStore
	fs, err := NewFileStore(usersFile, apiKeysFile)
	if err != nil {
		t.Fatalf("Failed to create FileStore: %v", err)
	}

	// Test user-tenant mappings
	tests := []struct {
		username string
		expected int
		exists   bool
	}{
		{"alice", 1, true},
		{"bob", 1, true},
		{"charlie", 2, true},
		{"unknown", 0, false},
	}

	for _, tt := range tests {
		t.Run(tt.username, func(t *testing.T) {
			tenantID, ok := fs.GetTenantID(tt.username)
			if ok != tt.exists {
				t.Errorf("GetTenantID(%s): exists = %v, want %v", tt.username, ok, tt.exists)
			}
			if ok && tenantID != tt.expected {
				t.Errorf("GetTenantID(%s): tenant_id = %d, want %d", tt.username, tenantID, tt.expected)
			}
		})
	}
}

func TestFileStore_AuthenticateAPIKey(t *testing.T) {
	// Create temporary directory
	tmpDir := t.TempDir()

	// Create users.csv
	usersFile := filepath.Join(tmpDir, "users.csv")
	usersData := `username,tenant_id
alice,1
bob,2
`
	if err := os.WriteFile(usersFile, []byte(usersData), 0600); err != nil {
		t.Fatalf("Failed to create users.csv: %v", err)
	}

	// Create api-keys.csv
	apiKeysFile := filepath.Join(tmpDir, "api-keys.csv")
	apiKeysData := `username,api_key
alice,key-alice-123
bob,key-bob-456
`
	if err := os.WriteFile(apiKeysFile, []byte(apiKeysData), 0600); err != nil {
		t.Fatalf("Failed to create api-keys.csv: %v", err)
	}

	// Create FileStore
	fs, err := NewFileStore(usersFile, apiKeysFile)
	if err != nil {
		t.Fatalf("Failed to create FileStore: %v", err)
	}

	// Test API key authentication
	tests := []struct {
		name           string
		apiKey         string
		wantUsername   string
		wantTenantID   int
		wantAuth       bool
	}{
		{
			name:         "Valid API key for alice",
			apiKey:       "key-alice-123",
			wantUsername: "alice",
			wantTenantID: 1,
			wantAuth:     true,
		},
		{
			name:         "Valid API key for bob",
			apiKey:       "key-bob-456",
			wantUsername: "bob",
			wantTenantID: 2,
			wantAuth:     true,
		},
		{
			name:         "Invalid API key",
			apiKey:       "invalid-key",
			wantUsername: "",
			wantTenantID: 0,
			wantAuth:     false,
		},
		{
			name:         "Empty API key",
			apiKey:       "",
			wantUsername: "",
			wantTenantID: 0,
			wantAuth:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			username, tenantID, ok := fs.AuthenticateAPIKey(tt.apiKey)
			if ok != tt.wantAuth {
				t.Errorf("AuthenticateAPIKey(): ok = %v, want %v", ok, tt.wantAuth)
			}
			if username != tt.wantUsername {
				t.Errorf("AuthenticateAPIKey(): username = %s, want %s", username, tt.wantUsername)
			}
			if tenantID != tt.wantTenantID {
				t.Errorf("AuthenticateAPIKey(): tenant_id = %d, want %d", tenantID, tt.wantTenantID)
			}
		})
	}
}

func TestFileStore_InvalidFormat(t *testing.T) {
	// Create temporary directory
	tmpDir := t.TempDir()

	tests := []struct {
		name        string
		usersData   string
		apiKeysData string
		wantError   bool
	}{
		{
			name:        "Invalid tenant_id format",
			usersData:   "alice,invalid\n",
			apiKeysData: "alice,key-123\n",
			wantError:   true,
		},
		{
			name:        "Negative tenant_id",
			usersData:   "alice,-1\n",
			apiKeysData: "alice,key-123\n",
			wantError:   true,
		},
		{
			name:        "Invalid users.csv columns",
			usersData:   "alice\n",
			apiKeysData: "alice,key-123\n",
			wantError:   true,
		},
		{
			name:        "Invalid api-keys.csv columns",
			usersData:   "alice,1\n",
			apiKeysData: "alice\n",
			wantError:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			usersFile := filepath.Join(tmpDir, "users.csv")
			apiKeysFile := filepath.Join(tmpDir, "api-keys.csv")

			if err := os.WriteFile(usersFile, []byte(tt.usersData), 0600); err != nil {
				t.Fatalf("Failed to create users.csv: %v", err)
			}
			if err := os.WriteFile(apiKeysFile, []byte(tt.apiKeysData), 0600); err != nil {
				t.Fatalf("Failed to create api-keys.csv: %v", err)
			}

			_, err := NewFileStore(usersFile, apiKeysFile)
			if (err != nil) != tt.wantError {
				t.Errorf("NewFileStore() error = %v, wantError %v", err, tt.wantError)
			}
		})
	}
}

func TestFileStore_MultipleUsersPerTenant(t *testing.T) {
	// Create temporary directory
	tmpDir := t.TempDir()

	// Create users.csv with multiple users for tenant 1
	usersFile := filepath.Join(tmpDir, "users.csv")
	usersData := `username,tenant_id
alice,1
bob,1
charlie,1
dave,2
`
	if err := os.WriteFile(usersFile, []byte(usersData), 0600); err != nil {
		t.Fatalf("Failed to create users.csv: %v", err)
	}

	// Create api-keys.csv
	apiKeysFile := filepath.Join(tmpDir, "api-keys.csv")
	apiKeysData := `username,api_key
alice,key-alice
bob,key-bob
charlie,key-charlie
dave,key-dave
`
	if err := os.WriteFile(apiKeysFile, []byte(apiKeysData), 0600); err != nil {
		t.Fatalf("Failed to create api-keys.csv: %v", err)
	}

	// Create FileStore
	fs, err := NewFileStore(usersFile, apiKeysFile)
	if err != nil {
		t.Fatalf("Failed to create FileStore: %v", err)
	}

	// Verify all three users map to tenant 1
	for _, username := range []string{"alice", "bob", "charlie"} {
		tenantID, ok := fs.GetTenantID(username)
		if !ok {
			t.Errorf("GetTenantID(%s): user not found", username)
		}
		if tenantID != 1 {
			t.Errorf("GetTenantID(%s): tenant_id = %d, want 1", username, tenantID)
		}
	}

	// Verify dave maps to tenant 2
	tenantID, ok := fs.GetTenantID("dave")
	if !ok {
		t.Errorf("GetTenantID(dave): user not found")
	}
	if tenantID != 2 {
		t.Errorf("GetTenantID(dave): tenant_id = %d, want 2", tenantID)
	}
}
