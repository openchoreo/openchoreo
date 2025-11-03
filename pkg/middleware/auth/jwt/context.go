// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package jwt

import (
	"net/http"

	"github.com/golang-jwt/jwt/v5"
)

// contextKey is a custom type for context keys to avoid collisions
type contextKey string

const (
	// claimsContextKey is the key used to store claims in the request context
	claimsContextKey contextKey = "jwt_claims"
	// tokenContextKey is the key used to store the raw token in the request context
	tokenContextKey contextKey = "jwt_token"
)

// GetClaims retrieves the JWT claims from the request context
func GetClaims(r *http.Request) (jwt.MapClaims, bool) {
	claims, ok := r.Context().Value(claimsContextKey).(jwt.MapClaims)
	return claims, ok
}

// GetToken retrieves the raw JWT token string from the request context
func GetToken(r *http.Request) (string, bool) {
	token, ok := r.Context().Value(tokenContextKey).(string)
	return token, ok
}

// GetClaimValue retrieves a specific claim value from the request context
func GetClaimValue(r *http.Request, key string) (interface{}, bool) {
	claims, ok := GetClaims(r)
	if !ok {
		return nil, false
	}
	value, exists := claims[key]
	return value, exists
}

// GetSubject retrieves the subject (sub) claim from the request context
func GetSubject(r *http.Request) (string, bool) {
	value, ok := GetClaimValue(r, "sub")
	if !ok {
		return "", false
	}
	sub, ok := value.(string)
	return sub, ok
}
