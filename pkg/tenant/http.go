package tenant

import (
	"net/http"
)

// HTTPMiddleware is an HTTP middleware that validates and injects tenant ID
func (v *Validator) HTTPMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Extract tenant ID from header
		tenantIDStr := r.Header.Get(HeaderTenantID)

		// Validate tenant ID
		tenantID, err := v.ValidateTenantID(tenantIDStr)
		if err != nil {
			http.Error(w, err.Error(), http.StatusUnauthorized)
			return
		}

		// Add tenant ID to context
		ctx := WithTenantID(r.Context(), tenantID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
