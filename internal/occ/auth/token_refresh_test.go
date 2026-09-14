// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package auth

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openchoreo/openchoreo/internal/occ/cmd/config"
	"github.com/openchoreo/openchoreo/internal/occ/testutil"
)

// mockControlPlaneURL is the control-plane URL every test in this file registers.
const mockControlPlaneURL = "http://mock-control-plane"

// oidcTransport returns a RoundTripper that serves the OIDC discovery endpoints
// and a /token endpoint returning the given access token.
func oidcTransport(t *testing.T, accessToken string) http.RoundTripper {
	return oidcTransportFor(t, mockControlPlaneURL, accessToken)
}

// oidcTransportFor is oidcTransport against an arbitrary control-plane URL.
func oidcTransportFor(t *testing.T, baseURL, accessToken string) http.RoundTripper {
	t.Helper()
	return testutil.RoundTripFunc(func(r *http.Request) (*http.Response, error) {
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
	t.Run("refreshes via authorization_code grant when refresh token present", func(t *testing.T) {
		home := testutil.SetupTestHome(t)
		testutil.SetTransport(t, oidcTransport(t, "new-access-token"))

		require.NoError(t, config.SaveStoredConfig(&config.StoredConfig{
			CurrentContext: "ctx",
			ControlPlanes:  []config.ControlPlane{{Name: "cp", URL: mockControlPlaneURL}},
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
		home := testutil.SetupTestHome(t)
		testutil.SetTransport(t, oidcTransport(t, "new-cc-token"))

		require.NoError(t, config.SaveStoredConfig(&config.StoredConfig{
			CurrentContext: "ctx",
			ControlPlanes:  []config.ControlPlane{{Name: "cp", URL: mockControlPlaneURL}},
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
		testutil.SetupTestHome(t)
		// No config — no current context

		_, err := RefreshToken()
		require.Error(t, err)
	})

	t.Run("returns error when client credentials are missing for refresh", func(t *testing.T) {
		home := testutil.SetupTestHome(t)
		testutil.SetTransport(t, oidcTransport(t, ""))

		require.NoError(t, config.SaveStoredConfig(&config.StoredConfig{
			CurrentContext: "ctx",
			ControlPlanes:  []config.ControlPlane{{Name: "cp", URL: mockControlPlaneURL}},
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

// freshJWT returns a signed JWT that is still valid for an hour.
func freshJWT(t *testing.T) string {
	t.Helper()
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"exp": time.Now().Add(time.Hour).Unix(),
	})
	s, err := token.SignedString([]byte("test-secret"))
	require.NoError(t, err)
	return s
}

// storedCredential reads the credential back off disk.
func storedCredential(t *testing.T) config.Credential {
	t.Helper()
	cfg, err := config.LoadStoredConfig()
	require.NoError(t, err)
	require.Len(t, cfg.Credentials, 1)
	return cfg.Credentials[0]
}

func writeRefreshableConfig(t *testing.T) {
	t.Helper()
	require.NoError(t, config.SaveStoredConfig(&config.StoredConfig{
		CurrentContext: "ctx",
		ControlPlanes:  []config.ControlPlane{{Name: "cp", URL: mockControlPlaneURL}},
		Credentials: []config.Credential{{
			Name:         "cred",
			Token:        expiredJWT(t),
			RefreshToken: "old-refresh-token",
			ClientID:     "cli-id",
			AuthMethod:   "authorization_code",
		}},
		Contexts: []config.Context{{Name: "ctx", ControlPlane: "cp", Credentials: "cred"}},
	}))
}

// A session that outlives its access token refreshes repeatedly, each time reading the
// credential back off disk.
func TestRefreshTokenPersistsRotatedTokens(t *testing.T) {
	testutil.SetupTestHome(t)
	testutil.SetTransport(t, oidcTransport(t, "new-access-token"))
	writeRefreshableConfig(t)

	token, err := RefreshToken()
	require.NoError(t, err)
	require.Equal(t, "new-access-token", token)

	stored := storedCredential(t)
	assert.Equal(t, "new-access-token", stored.Token,
		"the stored access token must be the refreshed one, or every later call refreshes again")
	assert.Equal(t, "new-refresh-token", stored.RefreshToken,
		"the rotated refresh token must be stored, or the next refresh replays the old one")
}

// Renewal runs one goroutine per workload, so a single expiry can wake several at once.
func TestRefreshTokenSerializesConcurrentCallers(t *testing.T) {
	testutil.SetupTestHome(t)

	issued := freshJWT(t)
	var mu sync.Mutex
	var sentRefreshTokens []string
	inner := oidcTransport(t, issued)
	testutil.SetTransport(t, testutil.RoundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/token" {
			body, err := io.ReadAll(r.Body)
			require.NoError(t, err)
			r.Body = io.NopCloser(bytes.NewReader(body))
			mu.Lock()
			sentRefreshTokens = append(sentRefreshTokens, string(body))
			mu.Unlock()
		}
		return inner.RoundTrip(r)
	}))
	writeRefreshableConfig(t)

	const callers = 4
	tokens := make([]string, callers)
	errs := make([]error, callers)
	var wg sync.WaitGroup
	for i := range callers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			tokens[i], errs[i] = RefreshToken()
		}()
	}
	wg.Wait()

	for i := range callers {
		require.NoErrorf(t, errs[i], "caller %d", i)
		assert.Equalf(t, issued, tokens[i], "caller %d should get the refreshed token", i)
	}

	mu.Lock()
	defer mu.Unlock()
	assert.Len(t, sentRefreshTokens, 1,
		"callers waiting on the lock should take the token the first one stored, not refresh again")
	assert.Contains(t, sentRefreshTokens[0], "old-refresh-token")
}

// RefreshToken resolves the control plane and the credential from one snapshot, so a
// context switch between reads can never pair one context's credential with another's
// control plane.
func TestRefreshTokenPairsCredentialWithItsControlPlane(t *testing.T) {
	const wantHost, otherHost = "cp-a.example", "cp-b.example"
	testutil.SetupTestHome(t)

	var mu sync.Mutex
	var seenHosts []string
	testutil.SetTransport(t, testutil.RoundTripFunc(func(r *http.Request) (*http.Response, error) {
		mu.Lock()
		seenHosts = append(seenHosts, r.URL.Host)
		mu.Unlock()
		return oidcTransportFor(t, "http://"+r.URL.Host, "refreshed-for-"+r.URL.Host).RoundTrip(r)
	}))

	require.NoError(t, config.SaveStoredConfig(&config.StoredConfig{
		CurrentContext: "ctx-a",
		ControlPlanes: []config.ControlPlane{
			{Name: "cp-a", URL: "http://" + wantHost},
			{Name: "cp-b", URL: "http://" + otherHost},
		},
		Credentials: []config.Credential{
			{Name: "cred-a", Token: expiredJWT(t), RefreshToken: "rt-a", ClientID: "cli-a", AuthMethod: "authorization_code"},
			{Name: "cred-b", Token: expiredJWT(t), RefreshToken: "rt-b", ClientID: "cli-b", AuthMethod: "authorization_code"},
		},
		Contexts: []config.Context{
			{Name: "ctx-a", ControlPlane: "cp-a", Credentials: "cred-a"},
			{Name: "ctx-b", ControlPlane: "cp-b", Credentials: "cred-b"},
		},
	}))

	token, err := RefreshToken()
	require.NoError(t, err)
	assert.Equal(t, "refreshed-for-"+wantHost, token)

	mu.Lock()
	defer mu.Unlock()
	require.NotEmpty(t, seenHosts)
	for _, h := range seenHosts {
		assert.Equalf(t, wantHost, h, "the current context's credential was redeemed against %s", h)
	}

	cfg, err := config.LoadStoredConfig()
	require.NoError(t, err)
	updated, err := cfg.CredentialByName("cred-a")
	require.NoError(t, err)
	assert.Equal(t, "refreshed-for-"+wantHost, updated.Token)

	untouched, err := cfg.CredentialByName("cred-b")
	require.NoError(t, err)
	assert.Equal(t, "rt-b", untouched.RefreshToken, "the other context's credential must not be touched")
}
