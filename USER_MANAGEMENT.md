# User/Tenant Management

## Overview

OTEL-OQL now supports user/tenant management through a simple file-based authentication system. This allows multiple users to access the system, with each user mapped to a specific tenant for data isolation.

## Architecture

The authentication system uses two CSV files:

1. **users.csv** - Maps usernames to tenant IDs
2. **api-keys.csv** - Maps usernames to API keys

### Authentication Flow

```
Client Request → Extract API Key → Lookup User → Validate → Inject tenant_id → Process Request
```

### Components

- **pkg/userstore/**: File-based user store for loading and caching user data
- **pkg/auth/**: Authentication middleware for HTTP and gRPC
- **Config**: Environment variables and flags for user file paths

## Configuration

### File Locations

By default, the system looks for user files in the current directory:
- `./users.csv`
- `./api-keys.csv`

You can customize the paths using:

**Environment Variables:**
```bash
export USERS_FILE=/path/to/users.csv
export API_KEYS_FILE=/path/to/api-keys.csv
```

**Command-line Flags:**
```bash
./otel-oql --users-file=/path/to/users.csv --api-keys-file=/path/to/api-keys.csv
```

**Config File:**
```yaml
users_file: /path/to/users.csv
api_keys_file: /path/to/api-keys.csv
```

### File Formats

#### users.csv

Format: `username,tenant_id`

```csv
username,tenant_id
alice,1
bob,1
charlie,2
```

**Notes:**
- Multiple users can belong to the same tenant (1:N relationship)
- Tenant IDs must be non-negative integers
- First line (header) is optional and will be skipped if it matches the format

#### api-keys.csv

Format: `username,api_key`

```csv
username,api_key
alice,oql_key_alice_123456789abcdef
bob,oql_key_bob_987654321fedcba
charlie,oql_key_charlie_abcdef123456
```

**Notes:**
- Each username must have exactly one API key
- API keys should be long, random strings (recommended: 32+ characters)
- API keys are stored in plaintext (consider using strong keys and file permissions)

## Usage

### Authenticating Requests

All requests must include an `Authorization` header with a Bearer token:

**HTTP Example:**
```bash
curl -X POST http://localhost:8080/query \
  -H "Authorization: Bearer oql_key_alice_123456789abcdef" \
  -H "Content-Type: application/json" \
  -d '{"query": "signal=spans | limit 10"}'
```

**gRPC Example:**
```go
conn, _ := grpc.Dial("localhost:4317",
    grpc.WithPerRPCCredentials(oauth.NewOauthAccess(&oauth2.Token{
        AccessToken: "oql_key_alice_123456789abcdef",
    })),
)
```

### Test Mode Behavior

When `TEST_MODE=true`, the authentication system provides backward compatibility:

1. **With user files present**: Authentication is enforced, but falls back to `tenant-id` header if no `Authorization` header is provided
2. **Without user files**: System falls back to the old `tenant-id` header-based authentication
3. **Default tenant**: If neither authentication method is provided, defaults to `tenant_id=0`

**Example (test mode without auth):**
```bash
# Still works in test mode
curl -X POST http://localhost:8080/query \
  -H "tenant-id: 0" \
  -H "Content-Type: application/json" \
  -d '{"query": "signal=spans | limit 10"}'
```

## Security Considerations

### File Permissions

Protect your user files with appropriate permissions:

```bash
chmod 600 users.csv api-keys.csv
chown otel-oql:otel-oql users.csv api-keys.csv
```

### API Key Generation

Generate strong, random API keys:

```bash
# Generate a random 32-character key
openssl rand -hex 16

# Or use uuidgen with a prefix
echo "oql_key_$(uuidgen | tr '[:upper:]' '[:lower:]')"
```

### Best Practices

1. **Use HTTPS/TLS**: Always use TLS in production to protect API keys in transit
2. **Rotate keys regularly**: Update API keys periodically
3. **Monitor failed auth attempts**: Check logs for authentication failures
4. **Limit file access**: Only the service user should read user files
5. **Backup user files**: Keep backups of user/tenant mappings

## Examples

### Multi-Tenant Setup

Create users for two teams:

**users.csv:**
```csv
username,tenant_id
team1_alice,100
team1_bob,100
team2_charlie,200
team2_dave,200
```

**api-keys.csv:**
```csv
username,api_key
team1_alice,oql_key_team1_alice_abc123
team1_bob,oql_key_team1_bob_def456
team2_charlie,oql_key_team2_charlie_ghi789
team2_dave,oql_key_team2_dave_jkl012
```

Now `team1_alice` and `team1_bob` can only access data for `tenant_id=100`, while `team2_charlie` and `team2_dave` can only access data for `tenant_id=200`.

### Development Setup

For local development, use test mode:

```bash
# Start service in test mode
./otel-oql --test-mode

# Can use either API keys or tenant-id headers
curl -H "tenant-id: 0" http://localhost:8080/query -d '{"query": "..."}'
```

## Troubleshooting

### "users file not found" Error

**Problem:** Service fails to start with "users file not found" error.

**Solution:**
- Create `users.csv` and `api-keys.csv` in the current directory, OR
- Enable test mode: `--test-mode`, OR
- Specify custom paths: `--users-file=/path/to/users.csv`

### "invalid API key" Error

**Problem:** Requests fail with 401 Unauthorized.

**Solutions:**
1. Verify the API key matches an entry in `api-keys.csv`
2. Check that the username in `api-keys.csv` exists in `users.csv`
3. Ensure Authorization header format is: `Bearer <api-key>`
4. Check for trailing whitespace in CSV files

### Tenant Isolation Issues

**Problem:** User can see data from other tenants.

**Investigation:**
- Verify user's tenant_id in `users.csv`
- Check Pinot queries include `WHERE tenant_id = ?`
- Review application logs for tenant_id in context

## Migration from Old System

### Before (tenant-id headers):
```bash
curl -H "tenant-id: 1" http://localhost:8080/query
```

### After (API key authentication):
```bash
curl -H "Authorization: Bearer oql_key_alice_123" http://localhost:8080/query
```

### Migration Steps:

1. **Create user files** while keeping test mode enabled:
   ```bash
   # users.csv
   admin,0
   
   # api-keys.csv
   admin,oql_key_admin_temporary123
   ```

2. **Test authentication**:
   ```bash
   ./otel-oql --test-mode
   curl -H "Authorization: Bearer oql_key_admin_temporary123" http://localhost:8080/query
   ```

3. **Disable test mode** once verified:
   ```bash
   ./otel-oql  # test mode off by default
   ```

4. **Update clients** to use Authorization headers

## Future Enhancements

Potential improvements for future versions:
- Password-based authentication
- API key rotation without file edits
- Web UI for user management
- Integration with external identity providers (LDAP, OAuth2)
- Role-based access control (RBAC)
- Query quotas per user/tenant
