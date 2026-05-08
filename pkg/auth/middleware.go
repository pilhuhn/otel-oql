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

// Middleware provides HTTP and gRPC authentication middleware
type Middleware struct {
	auth          Authenticator
	testMode      bool
	tenantFallback bool // If true, fall back to tenant-id header/metadata in test mode
}

// NewMiddleware creates a new authentication middleware
func NewMiddleware(auth Authenticator, testMode bool) *Middleware {
	return &Middleware{
		auth:     auth,
		testMode: testMode,
	}
}

// HTTPMiddleware is an HTTP middleware that authenticates API keys and injects tenant ID
func (m *Middleware) HTTPMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var tenantID int
		var username string

		// Check for Authorization header first
		authHeader := r.Header.Get("Authorization")
		if authHeader != "" {
			// Parse "Bearer <api-key>"
			parts := strings.SplitN(authHeader, " ", 2)
			if len(parts) != 2 || parts[0] != "Bearer" {
				http.Error(w, "invalid Authorization header format (expected: Bearer <api-key>)", http.StatusUnauthorized)
				return
			}

			apiKey := parts[1]

			// Authenticate API key
			var ok bool
			username, tenantID, ok = m.auth.AuthenticateAPIKey(apiKey)
			if !ok {
				http.Error(w, "invalid API key", http.StatusUnauthorized)
				return
			}
		} else {
			// No Authorization header - check if test mode allows tenant-id header
			if !m.testMode {
				http.Error(w, "missing Authorization header", http.StatusUnauthorized)
				return
			}

			// In test mode, fall back to tenant-id header
			tenantIDHeader := r.Header.Get(tenant.HeaderTenantID)
			if tenantIDHeader == "" {
				// In test mode with no auth and no tenant-id, use default
				tenantID = tenant.DefaultTestTenantID
				username = "test-user"
			} else {
				// Validate tenant-id header
				validator := tenant.NewValidator(true)
				var err error
				tenantID, err = validator.ValidateTenantID(tenantIDHeader)
				if err != nil {
					http.Error(w, err.Error(), http.StatusUnauthorized)
					return
				}
				username = "test-user"
			}
		}

		// Add tenant ID and username to context
		ctx := tenant.WithTenantID(r.Context(), tenantID)
		ctx = withUsername(ctx, username)

		// Continue with authenticated request
		next.ServeHTTP(w, r.WithContext(ctx))
	})
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
				return nil, status.Error(codes.Unauthenticated, "missing metadata")
			}
			// In test mode without metadata, use defaults
			tenantID = tenant.DefaultTestTenantID
			username = "test-user"
		} else {
			// Check for authorization header
			apiKeyValues := md.Get("authorization")
			if len(apiKeyValues) > 0 {
				// Parse "Bearer <api-key>"
				authValue := apiKeyValues[0]
				parts := strings.SplitN(authValue, " ", 2)
				if len(parts) != 2 || parts[0] != "Bearer" {
					return nil, status.Error(codes.Unauthenticated, "invalid authorization format")
				}

				apiKey := parts[1]

				// Authenticate API key
				var authOk bool
				username, tenantID, authOk = m.auth.AuthenticateAPIKey(apiKey)
				if !authOk {
					return nil, status.Error(codes.Unauthenticated, "invalid API key")
				}
			} else {
				// No authorization - check if test mode allows tenant-id metadata
				if !m.testMode {
					return nil, status.Error(codes.Unauthenticated, "missing authorization metadata")
				}

				// In test mode, fall back to tenant-id metadata
				tenantIDValues := md.Get(tenant.MetadataTenantID)
				if len(tenantIDValues) == 0 {
					// No tenant-id either, use default
					tenantID = tenant.DefaultTestTenantID
					username = "test-user"
				} else {
					// Validate tenant-id metadata
					validator := tenant.NewValidator(true)
					var err error
					tenantID, err = validator.ValidateTenantID(tenantIDValues[0])
					if err != nil {
						return nil, status.Errorf(codes.Unauthenticated, "invalid tenant: %v", err)
					}
					username = "test-user"
				}
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
				return status.Error(codes.Unauthenticated, "missing metadata")
			}
			// In test mode without metadata, use defaults
			tenantID = tenant.DefaultTestTenantID
			username = "test-user"
		} else {
			// Check for authorization header
			apiKeyValues := md.Get("authorization")
			if len(apiKeyValues) > 0 {
				// Parse "Bearer <api-key>"
				authValue := apiKeyValues[0]
				parts := strings.SplitN(authValue, " ", 2)
				if len(parts) != 2 || parts[0] != "Bearer" {
					return status.Error(codes.Unauthenticated, "invalid authorization format")
				}

				apiKey := parts[1]

				// Authenticate API key
				var authOk bool
				username, tenantID, authOk = m.auth.AuthenticateAPIKey(apiKey)
				if !authOk {
					return status.Error(codes.Unauthenticated, "invalid API key")
				}
			} else {
				// No authorization - check if test mode allows tenant-id metadata
				if !m.testMode {
					return status.Error(codes.Unauthenticated, "missing authorization metadata")
				}

				// In test mode, fall back to tenant-id metadata
				tenantIDValues := md.Get(tenant.MetadataTenantID)
				if len(tenantIDValues) == 0 {
					// No tenant-id either, use default
					tenantID = tenant.DefaultTestTenantID
					username = "test-user"
				} else {
					// Validate tenant-id metadata
					validator := tenant.NewValidator(true)
					var err error
					tenantID, err = validator.ValidateTenantID(tenantIDValues[0])
					if err != nil {
						return status.Errorf(codes.Unauthenticated, "invalid tenant: %v", err)
					}
					username = "test-user"
				}
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
