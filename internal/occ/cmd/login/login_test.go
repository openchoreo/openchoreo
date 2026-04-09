// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package login

import (
	"bytes"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openchoreo/openchoreo/internal/occ/cmd/config"
	"github.com/openchoreo/openchoreo/internal/occ/testutil"
)

// mockOIDCTransport replaces http.DefaultTransport for the duration of the test.
// It intercepts the three OIDC discovery/token calls and returns canned responses.
// tokenStatus controls the HTTP status returned by the /token endpoint.
// Returns the mock control-plane base URL to write into config.
func mockOIDCTransport(t *testing.T, securityEnabled bool, tokenStatus int) string {
	t.Helper()
	const baseURL = "http://mock-control-plane"

	testutil.SetTransport(t, testutil.RoundTripFunc(func(r *http.Request) (*http.Response, error) {
		switch r.URL.Path {
		case "/.well-known/oauth-protected-resource":
			return testutil.JSONResp(http.StatusOK, map[string]any{
				"authorization_servers": []string{baseURL},
				"openchoreo_clients": []map[string]any{
					{"name": "cli", "client_id": "cli-id", "scopes": []string{"openid"}},
				},
				"openchoreo_security_enabled": securityEnabled,
			}), nil

		case "/.well-known/openid-configuration":
			return testutil.JSONResp(http.StatusOK, map[string]any{
				"authorization_endpoint": baseURL + "/authorize",
				"token_endpoint":         baseURL + "/token",
			}), nil

		case "/token":
			if tokenStatus != http.StatusOK {
				return &http.Response{
					StatusCode: tokenStatus,
					Body:       io.NopCloser(bytes.NewReader([]byte(`{"error":"invalid_client"}`))),
					Header:     http.Header{},
				}, nil
			}
			return testutil.JSONResp(http.StatusOK, map[string]any{
				"access_token": "test-access-token",
				"token_type":   "Bearer",
				"expires_in":   3600,
			}), nil

		default:
			return &http.Response{StatusCode: http.StatusNotFound, Body: http.NoBody, Header: http.Header{}}, nil
		}
	}))

	return baseURL
}

func TestNewAuthImpl(t *testing.T) {
	impl := NewAuthImpl()
	assert.NotNil(t, impl)
}

func TestGetLoginPrompt(t *testing.T) {
	impl := NewAuthImpl()
	prompt := impl.GetLoginPrompt()
	assert.Contains(t, prompt, "occ login")
	assert.Contains(t, prompt, "--client-credentials")
}

func TestLoginWithClientCredentials(t *testing.T) {
	t.Run("successful login stores token and credential", func(t *testing.T) {
		home := testutil.SetupTestHome(t)
		baseURL := mockOIDCTransport(t, true, http.StatusOK)

		testutil.WriteOCConfig(t, home, &config.StoredConfig{
			CurrentContext: "ctx",
			ControlPlanes:  []config.ControlPlane{{Name: "cp", URL: baseURL}},
			Credentials:    []config.Credential{{Name: "cred"}},
			Contexts:       []config.Context{{Name: "ctx", ControlPlane: "cp", Credentials: "cred"}},
		})

		err := NewAuthImpl().Login(LoginParams{
			ClientCredentials: true,
			ClientID:          "my-client",
			ClientSecret:      "my-secret",
			CredentialName:    "cred",
		})
		require.NoError(t, err)

		cfg, err := config.LoadStoredConfig()
		require.NoError(t, err)
		require.Len(t, cfg.Credentials, 1)
		assert.Equal(t, "test-access-token", cfg.Credentials[0].Token)
		assert.Equal(t, "client_credentials", cfg.Credentials[0].AuthMethod)
	})

	t.Run("creates new credential entry when name not yet in config", func(t *testing.T) {
		home := testutil.SetupTestHome(t)
		baseURL := mockOIDCTransport(t, true, http.StatusOK)

		testutil.WriteOCConfig(t, home, &config.StoredConfig{
			CurrentContext: "ctx",
			ControlPlanes:  []config.ControlPlane{{Name: "cp", URL: baseURL}},
			Credentials:    []config.Credential{},
			Contexts:       []config.Context{{Name: "ctx", ControlPlane: "cp"}},
		})

		err := NewAuthImpl().Login(LoginParams{
			ClientCredentials: true,
			ClientID:          "my-client",
			ClientSecret:      "my-secret",
			CredentialName:    "new-cred",
		})
		require.NoError(t, err)

		cfg, err := config.LoadStoredConfig()
		require.NoError(t, err)
		require.Len(t, cfg.Credentials, 1)
		assert.Equal(t, "new-cred", cfg.Credentials[0].Name)
		assert.Equal(t, "test-access-token", cfg.Credentials[0].Token)
	})

	t.Run("reads client id and secret from env vars when flags are empty", func(t *testing.T) {
		home := testutil.SetupTestHome(t)
		baseURL := mockOIDCTransport(t, true, http.StatusOK)
		t.Setenv("OCC_CLIENT_ID", "env-client")
		t.Setenv("OCC_CLIENT_SECRET", "env-secret")

		testutil.WriteOCConfig(t, home, &config.StoredConfig{
			CurrentContext: "ctx",
			ControlPlanes:  []config.ControlPlane{{Name: "cp", URL: baseURL}},
			Credentials:    []config.Credential{{Name: "cred"}},
			Contexts:       []config.Context{{Name: "ctx", ControlPlane: "cp", Credentials: "cred"}},
		})

		err := NewAuthImpl().Login(LoginParams{
			ClientCredentials: true,
			CredentialName:    "cred",
		})
		require.NoError(t, err)
	})

	t.Run("returns error when client id and secret are both missing", func(t *testing.T) {
		home := testutil.SetupTestHome(t)
		t.Setenv("OCC_CLIENT_ID", "")
		t.Setenv("OCC_CLIENT_SECRET", "")

		testutil.WriteOCConfig(t, home, &config.StoredConfig{
			CurrentContext: "ctx",
			ControlPlanes:  []config.ControlPlane{{Name: "cp", URL: "http://mock-control-plane"}},
			Credentials:    []config.Credential{{Name: "cred"}},
			Contexts:       []config.Context{{Name: "ctx", ControlPlane: "cp", Credentials: "cred"}},
		})

		err := NewAuthImpl().Login(LoginParams{ClientCredentials: true})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "client ID and client secret are required")
	})

	t.Run("returns error when no credential name and context has none", func(t *testing.T) {
		home := testutil.SetupTestHome(t)
		baseURL := mockOIDCTransport(t, true, http.StatusOK)

		testutil.WriteOCConfig(t, home, &config.StoredConfig{
			CurrentContext: "ctx",
			ControlPlanes:  []config.ControlPlane{{Name: "cp", URL: baseURL}},
			Credentials:    []config.Credential{},
			Contexts:       []config.Context{{Name: "ctx", ControlPlane: "cp"}},
		})

		err := NewAuthImpl().Login(LoginParams{
			ClientCredentials: true,
			ClientID:          "id",
			ClientSecret:      "secret",
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "credential name must be specified")
	})

	t.Run("returns no error when security is disabled on server", func(t *testing.T) {
		home := testutil.SetupTestHome(t)
		baseURL := mockOIDCTransport(t, false, http.StatusOK)

		testutil.WriteOCConfig(t, home, &config.StoredConfig{
			CurrentContext: "ctx",
			ControlPlanes:  []config.ControlPlane{{Name: "cp", URL: baseURL}},
			Credentials:    []config.Credential{{Name: "cred"}},
			Contexts:       []config.Context{{Name: "ctx", ControlPlane: "cp", Credentials: "cred"}},
		})

		err := NewAuthImpl().Login(LoginParams{
			ClientCredentials: true,
			ClientID:          "id",
			ClientSecret:      "secret",
			CredentialName:    "cred",
		})
		require.NoError(t, err)
	})

	t.Run("returns error when no current context is set", func(t *testing.T) {
		testutil.SetupTestHome(t)
		// No config file — no current context

		err := NewAuthImpl().Login(LoginParams{
			ClientCredentials: true,
			ClientID:          "id",
			ClientSecret:      "secret",
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get current context")
	})

	t.Run("returns error when token endpoint returns non-200", func(t *testing.T) {
		home := testutil.SetupTestHome(t)
		baseURL := mockOIDCTransport(t, true, http.StatusUnauthorized)

		testutil.WriteOCConfig(t, home, &config.StoredConfig{
			CurrentContext: "ctx",
			ControlPlanes:  []config.ControlPlane{{Name: "cp", URL: baseURL}},
			Credentials:    []config.Credential{{Name: "cred"}},
			Contexts:       []config.Context{{Name: "ctx", ControlPlane: "cp", Credentials: "cred"}},
		})

		err := NewAuthImpl().Login(LoginParams{
			ClientCredentials: true,
			ClientID:          "id",
			ClientSecret:      "secret",
			CredentialName:    "cred",
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get access token")
	})
}

func TestIsLoggedIn(t *testing.T) {
	t.Run("returns false when no config exists", func(t *testing.T) {
		testutil.SetupTestHome(t)
		assert.False(t, NewAuthImpl().IsLoggedIn())
	})

	t.Run("returns true when security is disabled on server", func(t *testing.T) {
		home := testutil.SetupTestHome(t)
		baseURL := mockOIDCTransport(t, false, http.StatusOK)

		testutil.WriteOCConfig(t, home, &config.StoredConfig{
			CurrentContext: "ctx",
			ControlPlanes:  []config.ControlPlane{{Name: "cp", URL: baseURL}},
			Credentials:    []config.Credential{{Name: "cred"}},
			Contexts:       []config.Context{{Name: "ctx", ControlPlane: "cp", Credentials: "cred"}},
		})

		assert.True(t, NewAuthImpl().IsLoggedIn())
	})

	t.Run("returns false when token is empty", func(t *testing.T) {
		home := testutil.SetupTestHome(t)
		baseURL := mockOIDCTransport(t, true, http.StatusOK)

		testutil.WriteOCConfig(t, home, &config.StoredConfig{
			CurrentContext: "ctx",
			ControlPlanes:  []config.ControlPlane{{Name: "cp", URL: baseURL}},
			Credentials:    []config.Credential{{Name: "cred", Token: ""}},
			Contexts:       []config.Context{{Name: "ctx", ControlPlane: "cp", Credentials: "cred"}},
		})

		assert.False(t, NewAuthImpl().IsLoggedIn())
	})
}

func TestLoginConfigFilePersistence(t *testing.T) {
	t.Run("config file is written under the test home directory", func(t *testing.T) {
		home := testutil.SetupTestHome(t)
		baseURL := mockOIDCTransport(t, true, http.StatusOK)

		testutil.WriteOCConfig(t, home, &config.StoredConfig{
			CurrentContext: "ctx",
			ControlPlanes:  []config.ControlPlane{{Name: "cp", URL: baseURL}},
			Credentials:    []config.Credential{{Name: "cred"}},
			Contexts:       []config.Context{{Name: "ctx", ControlPlane: "cp", Credentials: "cred"}},
		})

		err := NewAuthImpl().Login(LoginParams{
			ClientCredentials: true,
			ClientID:          "id",
			ClientSecret:      "secret",
			CredentialName:    "cred",
		})
		require.NoError(t, err)

		_, err = os.Stat(filepath.Join(home, ".openchoreo", "config"))
		require.NoError(t, err)
	})
}
