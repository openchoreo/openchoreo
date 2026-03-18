// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package auth

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// signTestJWT creates a JWT signed with HS256 using a test key.
func signTestJWT(t *testing.T, claims jwt.MapClaims) string {
	t.Helper()
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	s, err := token.SignedString([]byte("test-secret"))
	require.NoError(t, err, "failed to sign test JWT")
	return s
}

func TestIsTokenExpired(t *testing.T) {
	tests := []struct {
		name  string
		token string
		want  bool
	}{
		{
			name:  "empty token returns false",
			token: "",
			want:  false,
		},
		{
			name: "valid non-expired JWT",
			token: signTestJWT(t, jwt.MapClaims{
				"exp": time.Now().Add(10 * time.Minute).Unix(),
			}),
			want: false,
		},
		{
			name: "expired JWT",
			token: signTestJWT(t, jwt.MapClaims{
				"exp": time.Now().Add(-10 * time.Minute).Unix(),
			}),
			want: true,
		},
		{
			name: "expiring within 1 minute",
			token: signTestJWT(t, jwt.MapClaims{
				"exp": time.Now().Add(30 * time.Second).Unix(),
			}),
			want: true,
		},
		{
			name:  "malformed token",
			token: "not-a-jwt-at-all",
			want:  true,
		},
		{
			name: "no exp claim",
			token: signTestJWT(t, jwt.MapClaims{
				"sub": "user123",
			}),
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsTokenExpired(tt.token)
			assert.Equal(t, tt.want, got)
		})
	}
}
