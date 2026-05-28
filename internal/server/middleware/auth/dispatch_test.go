// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package auth_test

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/openchoreo/openchoreo/internal/server/middleware/auth"
)

func jwtWithIssuer(iss string) string {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none","typ":"JWT"}`))
	payload, _ := json.Marshal(map[string]string{"iss": iss})
	body := base64.RawURLEncoding.EncodeToString(payload)
	return header + "." + body + ".sig"
}

func tagger(tag string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-Verifier", tag)
			next.ServeHTTP(w, r)
		})
	}
}

func TestIssuerDispatch(t *testing.T) {
	t.Parallel()
	mw := auth.IssuerDispatch(
		"https://token.actions.githubusercontent.com",
		tagger("github"),
		tagger("jwt"),
	)
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := mw(next)

	cases := []struct {
		name   string
		header string
		want   string
	}{
		{"github issuer", "Bearer " + jwtWithIssuer("https://token.actions.githubusercontent.com"), "github"},
		{"other issuer", "Bearer " + jwtWithIssuer("https://thunder.example.com"), "jwt"},
		{"no header", "", "jwt"},
		{"non-bearer scheme", "Basic dXNlcjpwYXNz", "jwt"},
		{"malformed token", "Bearer not.a.jwt", "jwt"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			req := httptest.NewRequest(http.MethodGet, "/x", nil)
			if tc.header != "" {
				req.Header.Set("Authorization", tc.header)
			}
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, req)
			if got := w.Header().Get("X-Verifier"); got != tc.want {
				t.Fatalf("dispatched to %q, want %q", got, tc.want)
			}
		})
	}
}
