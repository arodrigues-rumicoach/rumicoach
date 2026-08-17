package auth

import (
	"net/http"
	"strings"

	"github.com/rumi/rumi-be/config"
	"github.com/rumi/rumi-be/internal/apierror"
)

// RegionAllowed enforces data residency: a plane only serves users whose tokens carry
// its own region. Legacy tokens without a region claim are treated as EU (where all
// pre-migration users live). Planes without a configured region (local dev) allow all.
func RegionAllowed(claims *Claims) bool {
	planeRegion := config.AppConfig.RegionCode
	if planeRegion == "" {
		return true
	}
	tokenRegion := claims.Region
	if tokenRegion == "" {
		tokenRegion = "eu"
	}
	return tokenRegion == planeRegion
}

func AuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" || !strings.HasPrefix(authHeader, "Bearer ") {
			apierror.Write(w, http.StatusUnauthorized, apierror.CodeUnauthenticated, "Authorization header missing or invalid")
			return
		}

		tokenString := strings.TrimPrefix(authHeader, "Bearer ")
		claims, err := VerifyToken(tokenString)
		if err != nil {
			apierror.Write(w, http.StatusUnauthorized, apierror.CodeUnauthenticated, "Invalid token")
			return
		}

		if !RegionAllowed(claims) {
			apierror.Write(w, http.StatusForbidden, apierror.CodeWrongRegion, "Wrong region: this account's data is stored in another region")
			return
		}

		ctx := withIdentity(r.Context(), claims.Subject, claims.Email, claims.IsAdmin)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
