package userstore

import (
	"encoding/csv"
	"fmt"
	"os"
	"strconv"
	"sync"
)

// FileStore manages user and API key data from CSV files
type FileStore struct {
	usersFile   string
	apiKeysFile string

	// In-memory cache
	mu           sync.RWMutex
	userTenants  map[string]int    // username -> tenant_id
	apiKeys      map[string]string // api_key -> username
}

// NewFileStore creates a new file-based user store
func NewFileStore(usersFile, apiKeysFile string) (*FileStore, error) {
	fs := &FileStore{
		usersFile:   usersFile,
		apiKeysFile: apiKeysFile,
		userTenants: make(map[string]int),
		apiKeys:     make(map[string]string),
	}

	// Load data from files
	if err := fs.Load(); err != nil {
		return nil, err
	}

	return fs, nil
}

// Load reads data from CSV files into memory
func (fs *FileStore) Load() error {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	// Load users.csv (username,tenant_id)
	if err := fs.loadUsers(); err != nil {
		return fmt.Errorf("failed to load users: %w", err)
	}

	// Load api-keys.csv (username,api_key)
	if err := fs.loadAPIKeys(); err != nil {
		return fmt.Errorf("failed to load API keys: %w", err)
	}

	return nil
}

// loadUsers reads users.csv
func (fs *FileStore) loadUsers() error {
	file, err := os.Open(fs.usersFile)
	if err != nil {
		return fmt.Errorf("failed to open users file %s: %w", fs.usersFile, err)
	}
	defer file.Close()

	reader := csv.NewReader(file)
	records, err := reader.ReadAll()
	if err != nil {
		return fmt.Errorf("failed to read users CSV: %w", err)
	}

	// Clear existing data
	fs.userTenants = make(map[string]int)

	// Parse records (skip header if exists)
	for i, record := range records {
		if len(record) != 2 {
			return fmt.Errorf("invalid users.csv format at line %d: expected 2 columns, got %d", i+1, len(record))
		}

		username := record[0]
		tenantIDStr := record[1]

		// Skip header row if it looks like a header
		if username == "username" && tenantIDStr == "tenant_id" {
			continue
		}

		tenantID, err := strconv.Atoi(tenantIDStr)
		if err != nil {
			return fmt.Errorf("invalid tenant_id at line %d: %w", i+1, err)
		}

		if tenantID < 0 {
			return fmt.Errorf("invalid tenant_id at line %d: must be non-negative", i+1)
		}

		fs.userTenants[username] = tenantID
	}

	return nil
}

// loadAPIKeys reads api-keys.csv
func (fs *FileStore) loadAPIKeys() error {
	file, err := os.Open(fs.apiKeysFile)
	if err != nil {
		return fmt.Errorf("failed to open API keys file %s: %w", fs.apiKeysFile, err)
	}
	defer file.Close()

	reader := csv.NewReader(file)
	records, err := reader.ReadAll()
	if err != nil {
		return fmt.Errorf("failed to read API keys CSV: %w", err)
	}

	// Clear existing data
	fs.apiKeys = make(map[string]string)

	// Parse records (skip header if exists)
	for i, record := range records {
		if len(record) != 2 {
			return fmt.Errorf("invalid api-keys.csv format at line %d: expected 2 columns, got %d", i+1, len(record))
		}

		username := record[0]
		apiKey := record[1]

		// Skip header row if it looks like a header
		if username == "username" && apiKey == "api_key" {
			continue
		}

		fs.apiKeys[apiKey] = username
	}

	return nil
}

// AuthenticateAPIKey validates an API key and returns the username and tenant_id
func (fs *FileStore) AuthenticateAPIKey(apiKey string) (username string, tenantID int, ok bool) {
	fs.mu.RLock()
	defer fs.mu.RUnlock()

	// Lookup username from API key
	username, exists := fs.apiKeys[apiKey]
	if !exists {
		return "", 0, false
	}

	// Lookup tenant_id from username
	tenantID, exists = fs.userTenants[username]
	if !exists {
		return "", 0, false
	}

	return username, tenantID, true
}

// GetTenantID returns the tenant_id for a username
func (fs *FileStore) GetTenantID(username string) (int, bool) {
	fs.mu.RLock()
	defer fs.mu.RUnlock()

	tenantID, ok := fs.userTenants[username]
	return tenantID, ok
}

// Reload reloads data from CSV files
func (fs *FileStore) Reload() error {
	return fs.Load()
}
