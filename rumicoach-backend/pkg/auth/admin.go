package auth

import (
	"net/http"

	"github.com/rumi/rumi-be/internal/apierror"
)

// AdminMiddleware ensures that requests to admin endpoints are made by a user with admin privileges.
func AdminMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !IsAdmin(r.Context()) {
			apierror.Write(w, http.StatusForbidden, apierror.CodeAdminRequired, "Unauthorized: Admin privileges required")
			return
		}
		next.ServeHTTP(w, r)
	})
}
