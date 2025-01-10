package api

import (
	"context"
	"errors"
	"net/http"

	"github.com/warewulf/warewulf/internal/pkg/config"
)

var authentication = config.GetAuthentication()

type userKey string

var ukey userKey = "user"

func AuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		username, password, ok := r.BasicAuth()
		if !ok {
			w.Header().Set("WWW-Authenticate", `Basic realm="Restricted"`)
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		user, err := authentication.Auth(username, password)
		if err != nil {
			w.Header().Set("WWW-Authenticate", `Basic realm="Restricted"`)
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		ctx := context.WithValue(r.Context(), ukey, user)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func ACLMiddleware(requiredRole string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user := r.Context().Value(ukey).(*config.User)
			err := authentication.Access(user, requiredRole)
			if err != nil && errors.Is(err, config.UnauthorizedError) {
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			} else if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
