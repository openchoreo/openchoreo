// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package auth

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openchoreo/openchoreo/internal/occ/cmd/config"
	"github.com/openchoreo/openchoreo/internal/occ/testhelpers"
)

// oidcTransport returns a RoundTripper that serves the OIDC discovery endpoints
// and a /token endpoint returning the given access token.
func oidcTransport(t *testing.T, baseURL, accessToken string) http.RoundTripper {
	t.Helper()
	return roundTripFunc(func(r *http.Request) (*http.Response, error) {
		var body any
		switch r.URL.Path {
		case "/.well-known/oauth-protected-resource":
			body = map[string]any{
				"authorization_servers": []string{baseURL},
				"openchoreo_clients": []map[string]any{
					{"name": "cli", "client_id": "cli-id", "scopes": []string{"openid"}},
				},
				"openchoreo_security_enabled": true,
			}
		case "/.well-known/openid-configuration":
			body = map[string]any{
				"authorization_endpoint": baseURL + "/authorize",
				"token_endpoint":         baseURL + "/token",
			}
		case "/token":
			body = map[string]any{
				"access_token":  accessToken,
				"refresh_token": "new-refresh-token",
				"token_type":    "Bearer",
				"expires_in":    3600,
			}
		default:
			return &http.Response{StatusCode: http.StatusNotFound, Body: http.NoBody, Header: http.Header{}}, nil
		}
		b, err := json.Marshal(body)
		require.NoError(t, err)
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(bytes.NewReader(b)),
			Header:     http.Header{"Content-Type": []string{"application/json"}},
		}, nil
	})
}

// expiredJWT returns a signed JWT that expired 10 minutes ago.
func expiredJWT(t *testing.T) string {
	t.Helper()
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"exp": time.Now().Add(-10 * time.Minute).Unix(),
	})
	s, err := token.SignedString([]byte("test-secret"))
	require.NoError(t, err)
	return s
}

func TestNewPKCEAuth(t *testing.T) {
	t.Run("initializes all fields from OIDCConfig", func(t *testing.T) {
		oidcCfg := &OIDCConfig{
			AuthorizationEndpoint: "https://auth.example.com/authorize",
			TokenEndpoint:         "https://auth.example.com/token",
			ClientID:              "cli-client",
			Scopes:                []string{"openid", "profile"},
		}
		redirectURI := "http://127.0.0.1:55152/auth-callback"

		p, err := NewPKCEAuth(oidcCfg, redirectURI)
		require.NoError(t, err)

		assert.Equal(t, oidcCfg.AuthorizationEndpoint, p.AuthorizationEndpoint)
		assert.Equal(t, oidcCfg.TokenEndpoint, p.TokenEndpoint)
		assert.Equal(t, oidcCfg.ClientID, p.ClientID)
		assert.Equal(t, redirectURI, p.RedirectURI)
		assert.Equal(t, oidcCfg.Scopes, p.Scopes)
		assert.NotEmpty(t, p.CodeVerifier)
		assert.NotEmpty(t, p.CodeChallenge)
		assert.NotEmpty(t, p.State)
	})
}

func TestRefreshToken(t *testing.T) {
	const baseURL = "http://mock-control-plane"

	t.Run("refreshes via authorization_code grant when refresh token present", func(t *testing.T) {
		home := testhelpers.SetupTestHome(t)
		setTransport(t, oidcTransport(t, baseURL, "new-access-token"))

		require.NoError(t, config.SaveStoredConfig(&config.StoredConfig{
			CurrentContext: "ctx",
			ControlPlanes:  []config.ControlPlane{{Name: "cp", URL: baseURL}},
			Credentials: []config.Credential{{
				Name:         "cred",
				Token:        expiredJWT(t),
				RefreshToken: "old-refresh-token",
				ClientID:     "cli-id",
				AuthMethod:   "authorization_code",
			}},
			Contexts: []config.Context{{Name: "ctx", ControlPlane: "cp", Credentials: "cred"}},
		}))
		_ = home

		token, err := RefreshToken()
		require.NoError(t, err)
		assert.Equal(t, "new-access-token", token)
	})

	t.Run("refreshes via client_credentials when no refresh token", func(t *testing.T) {
		home := testhelpers.SetupTestHome(t)
		setTransport(t, oidcTransport(t, baseURL, "new-cc-token"))

		require.NoError(t, config.SaveStoredConfig(&config.StoredConfig{
			CurrentContext: "ctx",
			ControlPlanes:  []config.ControlPlane{{Name: "cp", URL: baseURL}},
			Credentials: []config.Credential{{
				Name:         "cred",
				Token:        expiredJWT(t),
				ClientID:     "cli-id",
				ClientSecret: "cli-secret",
				AuthMethod:   "client_credentials",
			}},
			Contexts: []config.Context{{Name: "ctx", ControlPlane: "cp", Credentials: "cred"}},
		}))
		_ = home

		token, err := RefreshToken()
		require.NoError(t, err)
		assert.Equal(t, "new-cc-token", token)
	})

	t.Run("returns error when no current context", func(t *testing.T) {
		testhelpers.SetupTestHome(t)
		// No config — no current context

		_, err := RefreshToken()
		require.Error(t, err)
	})

	t.Run("returns error when client credentials are missing for refresh", func(t *testing.T) {
		home := testhelpers.SetupTestHome(t)
		setTransport(t, oidcTransport(t, baseURL, ""))

		require.NoError(t, config.SaveStoredConfig(&config.StoredConfig{
			CurrentContext: "ctx",
			ControlPlanes:  []config.ControlPlane{{Name: "cp", URL: baseURL}},
			Credentials: []config.Credential{{
				Name:       "cred",
				Token:      expiredJWT(t),
				AuthMethod: "client_credentials",
				// ClientID and ClientSecret intentionally empty
			}},
			Contexts: []config.Context{{Name: "ctx", ControlPlane: "cp", Credentials: "cred"}},
		}))
		_ = home

		_, err := RefreshToken()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "credential does not have client credentials for refresh")
	})
}
