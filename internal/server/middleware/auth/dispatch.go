// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package auth

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
)

// IssuerDispatch returns an HTTP middleware that picks one of two downstream
// authentication middlewares based on the unverified `iss` claim of the
// presented Bearer token. Requests whose token's issuer matches matchIssuer
// are routed to onMatch; everything else (including requests with no token,
// which the fallback should also handle for public endpoints) is routed to
// fallback.
//
// The issuer is read from the second JWT segment without signature
// verification. This is safe because the selected downstream middleware
// performs the actual cryptographic verification - the issuer string is only
// used for routing.
//
// onMatch and fallback are both expected to be standard authentication
// middlewares that populate the SubjectContext on success and emit a 401
// response on failure.
func IssuerDispatch(
	matchIssuer string,
	onMatch func(http.Handler) http.Handler,
	fallback func(http.Handler) http.Handler,
) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		matched := onMatch(next)
		fellBack := fallback(next)
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if matchIssuer != "" && tokenIssuer(r) == matchIssuer {
				matched.ServeHTTP(w, r)
				return
			}
			fellBack.ServeHTTP(w, r)
		})
	}
}

// tokenIssuer extracts the `iss` claim from the Bearer token without
// verifying its signature. Returns "" if the request has no Bearer token or
// the token is malformed.
func tokenIssuer(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if h == "" {
		return ""
	}
	parts := strings.SplitN(h, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") || parts[1] == "" {
		return ""
	}
	segments := strings.Split(parts[1], ".")
	if len(segments) < 2 {
		return ""
	}
	payload, err := base64.RawURLEncoding.DecodeString(segments[1])
	if err != nil {
		return ""
	}
	var c struct {
		Iss string `json:"iss"`
	}
	if err := json.Unmarshal(payload, &c); err != nil {
		return ""
	}
	return c.Iss
}
