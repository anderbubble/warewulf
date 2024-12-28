package api

import (
	"context"
	"errors"
	"net/http"

	"github.com/warewulf/warewulf/internal/pkg/config"
)

type ctxKey string
const (
	userKey ctxKey = "user"
)

func AuthMiddleware(auth *config.Authentication) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if auth != nil {
				username, password, ok := r.BasicAuth()
				if !ok {
					w.Header().Set("WWW-Authenticate", `Basic realm="Restricted"`)
					http.Error(w, "Unauthorized", http.StatusUnauthorized)
					return
				}

				user, err := auth.Authenticate(username, password)
				if err != nil {
					w.Header().Set("WWW-Authenticate", `Basic realm="Restricted"`)
					http.Error(w, "Unauthorized", http.StatusUnauthorized)
					return
				}
				ctx := context.WithValue(r.Context(), userKey, user)
				next.ServeHTTP(w, r.WithContext(ctx))
			} else {
				next.ServeHTTP(w, r)
			}
		})
	}
}

func ACLMiddleware(auth *config.Authentication, requiredRole string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if auth != nil {
				user := r.Context().Value(userKey).(*config.User)
				err := auth.CheckAccess(user, requiredRole)
				if err != nil && errors.Is(err, config.UnauthorizedError) {
					http.Error(w, "Unauthorized", http.StatusUnauthorized)
					return
				} else if err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}
