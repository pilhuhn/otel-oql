package tenant

import (
	"context"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// GRPCUnaryInterceptor is a gRPC unary interceptor that validates and injects tenant ID
func (v *Validator) GRPCUnaryInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		// Extract tenant ID from metadata
		tenantIDStr := ""
		if md, ok := metadata.FromIncomingContext(ctx); ok {
			if values := md.Get(MetadataTenantID); len(values) > 0 {
				tenantIDStr = values[0]
			}
		}

		// Validate tenant ID
		tenantID, err := v.ValidateTenantID(tenantIDStr)
		if err != nil {
			return nil, status.Errorf(codes.Unauthenticated, "invalid tenant: %v", err)
		}

		// Add tenant ID to context
		ctx = WithTenantID(ctx, tenantID)

		// Call the handler
		return handler(ctx, req)
	}
}

// GRPCStreamInterceptor is a gRPC stream interceptor that validates and injects tenant ID
func (v *Validator) GRPCStreamInterceptor() grpc.StreamServerInterceptor {
	return func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		// Extract tenant ID from metadata
		tenantIDStr := ""
		if md, ok := metadata.FromIncomingContext(ss.Context()); ok {
			if values := md.Get(MetadataTenantID); len(values) > 0 {
				tenantIDStr = values[0]
			}
		}

		// Validate tenant ID
		tenantID, err := v.ValidateTenantID(tenantIDStr)
		if err != nil {
			return status.Errorf(codes.Unauthenticated, "invalid tenant: %v", err)
		}

		// Add tenant ID to context
		ctx := WithTenantID(ss.Context(), tenantID)

		// Wrap the stream with new context
		wrappedStream := &wrappedServerStream{
			ServerStream: ss,
			ctx:          ctx,
		}

		// Call the handler
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
