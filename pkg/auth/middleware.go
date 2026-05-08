package auth

import (
	"context"
	"net/http"
	"strings"

	"github.com/pilhuhn/otel-oql/pkg/tenant"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// Authenticator handles API key authentication
type Authenticator interface {
	AuthenticateAPIKey(apiKey string) (username string, tenantID int, ok bool)
}

const (
	// Error messages
	ErrMissingAuth         = "missing Authorization header"
	ErrInvalidAuthFormat   = "invalid Authorization header format (expected: Bearer <token>)"
	ErrInvalidAPIKey       = "invalid API key"
	ErrMissingMetadata     = "missing metadata"
	ErrMissingAuthMetadata = "missing authorization metadata"
	ErrInvalidAuthMetadata = "invalid authorization format"

	// Default test username
	TestModeUsername = "test-user"
)

// Middleware provides HTTP and gRPC authentication middleware
type Middleware struct {
	auth            Authenticator
	testMode        bool
	tenantValidator *tenant.Validator // Cached for test mode
}

// NewMiddleware creates a new authentication middleware
func NewMiddleware(auth Authenticator, testMode bool) *Middleware {
	var validator *tenant.Validator
	if testMode {
		validator = tenant.NewValidator(true)
	}
	return &Middleware{
		auth:            auth,
		testMode:        testMode,
		tenantValidator: validator,
	}
}

// authenticateBearer extracts and validates a Bearer token
// Returns (username, tenantID, error)
func (m *Middleware) authenticateBearer(authHeader string) (string, int, error) {
	parts := strings.SplitN(authHeader, " ", 2)
	if len(parts) != 2 || parts[0] != "Bearer" {
		return "", 0, &AuthError{Message: ErrInvalidAuthFormat}
	}

	apiKey := parts[1]
	username, tenantID, ok := m.auth.AuthenticateAPIKey(apiKey)
	if !ok {
		return "", 0, &AuthError{Message: ErrInvalidAPIKey}
	}

	return username, tenantID, nil
}

// AuthError represents an authentication error
type AuthError struct {
	Message string
}

func (e *AuthError) Error() string {
	return e.Message
}

// HTTPMiddleware is an HTTP middleware that authenticates API keys and injects tenant ID
func (m *Middleware) HTTPMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var tenantID int
		var username string

		// Check for Authorization header first
		authHeader := r.Header.Get("Authorization")
		if authHeader != "" {
			var err error
			username, tenantID, err = m.authenticateBearer(authHeader)
			if err != nil {
				http.Error(w, err.Error(), http.StatusUnauthorized)
				return
			}
		} else {
			// No Authorization header - check if test mode allows tenant-id header
			if !m.testMode {
				http.Error(w, ErrMissingAuth, http.StatusUnauthorized)
				return
			}

			// In test mode, fall back to tenant-id header
			tenantIDHeader := r.Header.Get(tenant.HeaderTenantID)
			if tenantIDHeader == "" {
				// In test mode with no auth and no tenant-id, use default
				tenantID = tenant.DefaultTestTenantID
				username = TestModeUsername
			} else {
				// Validate tenant-id header using cached validator
				var err error
				tenantID, err = m.tenantValidator.ValidateTenantID(tenantIDHeader)
				if err != nil {
					http.Error(w, err.Error(), http.StatusUnauthorized)
					return
				}
				username = TestModeUsername
			}
		}

		// Add tenant ID and username to context
		ctx := tenant.WithTenantID(r.Context(), tenantID)
		ctx = withUsername(ctx, username)

		// Continue with authenticated request
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// authenticateGRPC performs authentication from gRPC metadata
func (m *Middleware) authenticateGRPC(md metadata.MD) (username string, tenantID int, err error) {
	// Check for authorization header
	apiKeyValues := md.Get("authorization")
	if len(apiKeyValues) > 0 {
		return m.authenticateBearer(apiKeyValues[0])
	}

	// No authorization - check if test mode allows tenant-id metadata
	if !m.testMode {
		return "", 0, &AuthError{Message: ErrMissingAuthMetadata}
	}

	// In test mode, fall back to tenant-id metadata
	tenantIDValues := md.Get(tenant.MetadataTenantID)
	if len(tenantIDValues) == 0 {
		// No tenant-id either, use default
		return TestModeUsername, tenant.DefaultTestTenantID, nil
	}

	// Validate tenant-id metadata using cached validator
	tenantID, err = m.tenantValidator.ValidateTenantID(tenantIDValues[0])
	if err != nil {
		return "", 0, err
	}
	return TestModeUsername, tenantID, nil
}

// GRPCUnaryInterceptor is a gRPC unary interceptor that authenticates API keys
func (m *Middleware) GRPCUnaryInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		var tenantID int
		var username string

		// Extract metadata
		md, ok := metadata.FromIncomingContext(ctx)
		if !ok {
			if !m.testMode {
				return nil, status.Error(codes.Unauthenticated, ErrMissingMetadata)
			}
			// In test mode without metadata, use defaults
			tenantID = tenant.DefaultTestTenantID
			username = TestModeUsername
		} else {
			var err error
			username, tenantID, err = m.authenticateGRPC(md)
			if err != nil {
				return nil, status.Error(codes.Unauthenticated, err.Error())
			}
		}

		// Add tenant ID and username to context
		ctx = tenant.WithTenantID(ctx, tenantID)
		ctx = withUsername(ctx, username)

		// Call handler
		return handler(ctx, req)
	}
}

// GRPCStreamInterceptor is a gRPC stream interceptor that authenticates API keys
func (m *Middleware) GRPCStreamInterceptor() grpc.StreamServerInterceptor {
	return func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		ctx := ss.Context()
		var tenantID int
		var username string

		// Extract metadata
		md, ok := metadata.FromIncomingContext(ctx)
		if !ok {
			if !m.testMode {
				return status.Error(codes.Unauthenticated, ErrMissingMetadata)
			}
			// In test mode without metadata, use defaults
			tenantID = tenant.DefaultTestTenantID
			username = TestModeUsername
		} else {
			var err error
			username, tenantID, err = m.authenticateGRPC(md)
			if err != nil {
				return status.Error(codes.Unauthenticated, err.Error())
			}
		}

		// Add tenant ID and username to context
		ctx = tenant.WithTenantID(ctx, tenantID)
		ctx = withUsername(ctx, username)

		// Wrap stream with authenticated context
		wrappedStream := &wrappedServerStream{
			ServerStream: ss,
			ctx:          ctx,
		}

		return handler(srv, wrappedStream)
	}
}

// wrappedServerStream wraps grpc.ServerStream to override the context
type wrappedServerStream struct {
	grpc.ServerStream
	ctx context.Context
}

// Context returns the wrapped context
func (w *wrappedServerStream) Context() context.Context {
	return w.ctx
}

// Context key for username
type contextKey string

const usernameKey contextKey = "username"

// withUsername adds username to context
func withUsername(ctx context.Context, username string) context.Context {
	return context.WithValue(ctx, usernameKey, username)
}

// UsernameFromContext extracts username from context
func UsernameFromContext(ctx context.Context) (string, bool) {
	username, ok := ctx.Value(usernameKey).(string)
	return username, ok
}
