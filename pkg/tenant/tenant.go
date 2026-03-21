package tenant

import (
	"context"
	"fmt"
	"strconv"
)

// contextKey is a private type for context keys
type contextKey string

const (
	// TenantIDKey is the context key for tenant ID
	TenantIDKey contextKey = "tenant-id"

	// DefaultTestTenantID is the default tenant ID used in test mode
	DefaultTestTenantID = 0

	// HeaderTenantID is the HTTP header name for tenant ID
	HeaderTenantID = "X-Tenant-ID"

	// MetadataTenantID is the gRPC metadata key for tenant ID
	MetadataTenantID = "tenant-id"
)

// Validator handles tenant ID validation
type Validator struct {
	testMode bool
}

// NewValidator creates a new tenant validator
func NewValidator(testMode bool) *Validator {
	return &Validator{
		testMode: testMode,
	}
}

// ValidateTenantID validates and returns the tenant ID
// In test mode, returns DefaultTestTenantID if no tenant ID is provided
// Otherwise, returns an error if tenant ID is missing
func (v *Validator) ValidateTenantID(tenantIDStr string) (int, error) {
	// If tenant ID is provided, parse and return it
	if tenantIDStr != "" {
		tenantID, err := strconv.Atoi(tenantIDStr)
		if err != nil {
			return 0, fmt.Errorf("invalid tenant-id format: %w", err)
		}
		if tenantID < 0 {
			return 0, fmt.Errorf("tenant-id must be non-negative, got %d", tenantID)
		}
		return tenantID, nil
	}

	// If no tenant ID and test mode is enabled, use default
	if v.testMode {
		return DefaultTestTenantID, nil
	}

	// No tenant ID in production mode - reject
	return 0, fmt.Errorf("tenant-id is required")
}

// WithTenantID adds tenant ID to context
func WithTenantID(ctx context.Context, tenantID int) context.Context {
	return context.WithValue(ctx, TenantIDKey, tenantID)
}

// FromContext extracts tenant ID from context
func FromContext(ctx context.Context) (int, bool) {
	tenantID, ok := ctx.Value(TenantIDKey).(int)
	return tenantID, ok
}

// MustFromContext extracts tenant ID from context or panics
func MustFromContext(ctx context.Context) int {
	tenantID, ok := FromContext(ctx)
	if !ok {
		panic("tenant-id not found in context")
	}
	return tenantID
}
