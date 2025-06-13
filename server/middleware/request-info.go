package middleware

import (
	"net/http"

	"github.com/openchoreo/openchoreo/server/request"
)

func WithRequestInfo(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		info, err := request.NewRequestInfo(r)
		if err != nil {
			w.Write(([]byte(err.Error())))
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		r = r.WithContext(request.WithRequestInfo(ctx, info))

		next.ServeHTTP(w, r)
	})
}
