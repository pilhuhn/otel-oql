# Issue #2: User/Tenant Management Implementation

## Summary

This implementation adds a simple, file-based user/tenant management system to OTEL-OQL. The system uses two CSV files to map users to tenants and authenticate requests via API keys.

## What Was Implemented

### 1. User Store (`pkg/userstore/`)
- **filestore.go**: File-based storage that loads and caches user data from CSV files
- **filestore_test.go**: Comprehensive tests for file loading, validation, and authentication

### 2. Authentication Middleware (`pkg/auth/`)
- **middleware.go**: HTTP and gRPC middleware for API key authentication
- **middleware_test.go**: Tests for authentication flows, including multi-tenant scenarios

### 3. Configuration Updates (`internal/config/`)
- Added `users_file` and `api_keys_file` configuration options
- Support for environment variables: `USERS_FILE`, `API_KEYS_FILE`
- Command-line flags: `--users-file`, `--api-keys-file`
- Default paths: `./users.csv`, `./api-keys.csv`

### 4. Main Service Updates (`cmd/otel-oql/main.go`)
- New `initAuth()` function to initialize user store and auth middleware
- Updated all three run modes (all, ingestion, query) to use auth middleware
- Graceful fallback to tenant-id headers in test mode

### 5. Receiver/API Updates
- **pkg/receiver/grpc.go**: Updated to accept gRPC interceptor instead of validator
- **pkg/receiver/http.go**: Updated to accept HTTP middleware function
- **pkg/api/server.go**: Updated to accept HTTP middleware function

### 6. Example Files
- **users.csv**: Example user-to-tenant mappings (alice, bob → tenant 1; charlie → tenant 2)
- **api-keys.csv**: Example API keys for authentication

### 7. Documentation
- **USER_MANAGEMENT.md**: Complete guide for authentication system usage

## Key Design Decisions

### Simplicity First
- No database required - just two CSV files
- No CLI tool needed - users can edit files directly
- Minimal dependencies (only standard library)

### Backward Compatibility
- Test mode continues to work without user files
- Existing tenant-id header authentication still works in test mode
- No breaking changes for development workflows

### Security Model
- API keys in plaintext (users should protect files with filesystem permissions)
- Bearer token authentication (standard HTTP Authorization header)
- Multi-tenant isolation maintained at application level

## How It Works

### Authentication Flow

```
1. Client sends request with Authorization header
2. Auth middleware extracts Bearer token
3. Lookup username from api-keys.csv
4. Lookup tenant_id from users.csv  
5. Inject tenant_id into request context
6. Continue with existing tenant isolation logic
```

### Test Mode Fallback

```
TEST_MODE=true (default for dev):
- If user files exist: Enforce auth, but allow tenant-id header fallback
- If user files missing: Use old tenant-id header system
- If no auth at all: Default to tenant_id=0
```

## Usage Examples

### Production Mode (with authentication):
```bash
# 1. Create user files
cat > users.csv <<EOF
username,tenant_id
alice,1
bob,1
charlie,2
EOF

cat > api-keys.csv <<EOF
username,api_key
alice,oql_key_alice_abc123
bob,oql_key_bob_def456
charlie,oql_key_charlie_ghi789
EOF

# 2. Start service (auth enabled automatically)
./otel-oql

# 3. Make authenticated requests
curl -H "Authorization: Bearer oql_key_alice_abc123" \
  http://localhost:8080/query \
  -d '{"query": "signal=spans | limit 10"}'
```

### Development Mode (test mode):
```bash
# Start in test mode (no user files needed)
./otel-oql --test-mode

# Old tenant-id header still works
curl -H "tenant-id: 0" http://localhost:8080/query
```

## Testing

Run tests for the new components:

```bash
# User store tests
go test ./pkg/userstore -v

# Auth middleware tests
go test ./pkg/auth -v

# Integration tests
go test ./pkg/integration -v
```

## Files Changed

### New Files:
- `pkg/userstore/filestore.go`
- `pkg/userstore/filestore_test.go`
- `pkg/auth/middleware.go`
- `pkg/auth/middleware_test.go`
- `users.csv`
- `api-keys.csv`
- `USER_MANAGEMENT.md`
- `ISSUE-2-README.md`

### Modified Files:
- `internal/config/config.go` (added users_file, api_keys_file config)
- `cmd/otel-oql/main.go` (added initAuth, updated all run modes)
- `pkg/receiver/grpc.go` (accept interceptor instead of validator)
- `pkg/receiver/http.go` (accept middleware function)
- `pkg/api/server.go` (accept middleware function)

## Next Steps

### Immediate (for this PR):
1. Review code changes
2. Run full test suite
3. Test with example CSV files
4. Update main README.md to reference USER_MANAGEMENT.md

### Future Enhancements (separate issues):
1. API key hashing (SHA256) for better security
2. API key rotation mechanism
3. Web-based user management UI
4. Integration with external auth providers (OAuth2, LDAP)
5. Role-based access control (RBAC)
6. Per-user query quotas
7. Audit logging for authentication events

## Migration Guide

For existing deployments:

**Option 1: Continue without authentication (test mode)**
```bash
# No changes needed - just enable test mode
export TEST_MODE=true
./otel-oql
```

**Option 2: Enable authentication**
```bash
# 1. Create user files (see examples above)
# 2. Start service (test mode off)
./otel-oql

# 3. Update clients to use API keys
# OLD: curl -H "tenant-id: 1" ...
# NEW: curl -H "Authorization: Bearer <api-key>" ...
```

## Support

For questions or issues:
- See USER_MANAGEMENT.md for detailed usage guide
- Check troubleshooting section for common problems
- Review test files for example usage
