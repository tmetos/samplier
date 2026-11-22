package auth

import (
	"encoding/json"
	"net/http"
	"strings"
)

// Middleware returns an HTTP middleware enforcing bearer-token auth.
// Requests without a valid token receive 401. With enabled=false the
// middleware is a pass-through (local development).
func (v *Verifier) Middleware(enabled bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !enabled {
				// Dev mode: attach an anonymous principal so handlers can rely
				// on claims always being present.
				next.ServeHTTP(w, r.WithContext(WithClaims(r.Context(), &Claims{
					Subject: "anonymous", DisplayName: "Development User",
				})))
				return
			}

			token, err := BearerToken(r.Header.Get("Authorization"))
			if err != nil {
				unauthorized(w, "missing bearer token")
				return
			}
			claims, err := v.Verify(r.Context(), token)
			if err != nil {
				if err == ErrExpired {
					unauthorized(w, "token expired")
					return
				}
				unauthorized(w, "invalid token")
				return
			}
			next.ServeHTTP(w, r.WithContext(WithClaims(r.Context(), claims)))
		})
	}
}

// RequireRole returns middleware allowing only users with the given realm role.
func RequireRole(role string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims, ok := ClaimsFrom(r.Context())
			if !ok || !HasRole(claims, role) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusForbidden)
				_ = json.NewEncoder(w).Encode(map[string]string{
					"error": "requires role " + role,
				})
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func unauthorized(w http.ResponseWriter, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("WWW-Authenticate", `Bearer realm="samplier"`)
	w.WriteHeader(http.StatusUnauthorized)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

// hasBearerPrefix is a small helper used by tests and callers that need to
// probe for a token without validating it.
func hasBearerPrefix(header string) bool {
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(header)), "bearer ")
}
