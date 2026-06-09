// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package auth

import (
	"net/http"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openchoreo/openchoreo/internal/occ/cmd/config"
	"github.com/openchoreo/openchoreo/internal/occ/testutil"
)

func validToken(t *testing.T) string {
	t.Helper()
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"exp": time.Now().Add(10 * time.Minute).Unix(),
	})
	s, err := token.SignedString([]byte("secret"))
	require.NoError(t, err)
	return s
}

func setupConfig(t *testing.T, cfg *config.StoredConfig) {
	t.Helper()
	home := testutil.SetupTestHome(t)
	testutil.WriteOCConfig(t, home, cfg)
}

// oidcSecurityTransport returns a RoundTripper that serves both OIDC endpoints.
// The protected-resource endpoint returns securityEnabled, the discovery
// endpoint returns minimal valid auth/token URLs.
func oidcSecurityTransport(t *testing.T, securityEnabled bool) http.RoundTripper {
	t.Helper()
	return testutil.RoundTripFunc(func(r *http.Request) (*http.Response, error) {
		path := r.URL.Host + r.URL.Path
		switch path {
		case mockAPIProtectedResource:
			return testutil.JSONResp(http.StatusOK, protectedResourceResponse{
				AuthorizationServers:      []string{"http://mock-issuer"},
				OpenChoreoClients:         []clientInfo{{Name: "cli", ClientID: "cli-id", Scopes: []string{"openid"}}},
				OpenChoreoSecurityEnabled: securityEnabled,
			}), nil
		case mockIssuerOIDCDiscovery:
			return testutil.JSONResp(http.StatusOK, oidcProviderDiscovery{
				AuthorizationEndpoint: "https://auth.example.com/authorize",
				TokenEndpoint:         "https://auth.example.com/token",
			}), nil
		default:
			return &http.Response{StatusCode: http.StatusNotFound, Body: http.NoBody, Header: http.Header{}}, nil
		}
	})
}

func TestIsLoggedIn(t *testing.T) {
	t.Run("no config file returns false", func(t *testing.T) {
		testutil.SetupTestHome(t)
		assert.False(t, IsLoggedIn())
	})

	t.Run("no current context returns false", func(t *testing.T) {
		setupConfig(t, &config.StoredConfig{})
		assert.False(t, IsLoggedIn())
	})

	t.Run("control plane not found returns false", func(t *testing.T) {
		setupConfig(t, &config.StoredConfig{
			CurrentContext: "ctx",
			ControlPlanes:  []config.ControlPlane{},
			Contexts:       []config.Context{{Name: "ctx", ControlPlane: "missing-cp"}},
		})
		assert.False(t, IsLoggedIn())
	})

	t.Run("security disabled returns true without credential check", func(t *testing.T) {
		setupConfig(t, &config.StoredConfig{
			CurrentContext: "ctx",
			ControlPlanes:  []config.ControlPlane{{Name: "cp", URL: "http://mock-api"}},
			Contexts:       []config.Context{{Name: "ctx", ControlPlane: "cp"}},
		})
		testutil.SetTransport(t, oidcSecurityTransport(t, false))
		assert.True(t, IsLoggedIn())
	})

	t.Run("OIDC fetch error returns false", func(t *testing.T) {
		setupConfig(t, &config.StoredConfig{
			CurrentContext: "ctx",
			ControlPlanes:  []config.ControlPlane{{Name: "cp", URL: "http://mock-api"}},
			Contexts:       []config.Context{{Name: "ctx", ControlPlane: "cp"}},
		})
		testutil.SetTransport(t, testutil.RoundTripFunc(func(r *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusInternalServerError,
				Body:       http.NoBody,
				Header:     http.Header{},
			}, nil
		}))
		assert.False(t, IsLoggedIn())
	})

	t.Run("security enabled with no credentials reference returns false", func(t *testing.T) {
		setupConfig(t, &config.StoredConfig{
			CurrentContext: "ctx",
			ControlPlanes:  []config.ControlPlane{{Name: "cp", URL: "http://mock-api"}},
			Contexts:       []config.Context{{Name: "ctx", ControlPlane: "cp"}}, // no credentials field
		})
		testutil.SetTransport(t, oidcSecurityTransport(t, true))
		assert.False(t, IsLoggedIn())
	})

	t.Run("security enabled with empty token returns false", func(t *testing.T) {
		setupConfig(t, &config.StoredConfig{
			CurrentContext: "ctx",
			ControlPlanes:  []config.ControlPlane{{Name: "cp", URL: "http://mock-api"}},
			Credentials:    []config.Credential{{Name: "cred", Token: ""}},
			Contexts:       []config.Context{{Name: "ctx", ControlPlane: "cp", Credentials: "cred"}},
		})
		testutil.SetTransport(t, oidcSecurityTransport(t, true))
		assert.False(t, IsLoggedIn())
	})

	t.Run("security enabled with valid non-expired token returns true", func(t *testing.T) {
		setupConfig(t, &config.StoredConfig{
			CurrentContext: "ctx",
			ControlPlanes:  []config.ControlPlane{{Name: "cp", URL: "http://mock-api"}},
			Credentials:    []config.Credential{{Name: "cred", Token: validToken(t)}},
			Contexts:       []config.Context{{Name: "ctx", ControlPlane: "cp", Credentials: "cred"}},
		})
		testutil.SetTransport(t, oidcSecurityTransport(t, true))
		assert.True(t, IsLoggedIn())
	})
}

func TestRequireLogin(t *testing.T) {
	t.Run("returns error when not logged in", func(t *testing.T) {
		testutil.SetupTestHome(t)
		fn := RequireLogin()
		err := fn(nil, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "occ login")
	})

	t.Run("returns nil when logged in", func(t *testing.T) {
		setupConfig(t, &config.StoredConfig{
			CurrentContext: "ctx",
			ControlPlanes:  []config.ControlPlane{{Name: "cp", URL: "http://mock-api"}},
			Credentials:    []config.Credential{{Name: "cred", Token: validToken(t)}},
			Contexts:       []config.Context{{Name: "ctx", ControlPlane: "cp", Credentials: "cred"}},
		})
		testutil.SetTransport(t, oidcSecurityTransport(t, true))
		fn := RequireLogin()
		require.NoError(t, fn(nil, nil))
	})
}
