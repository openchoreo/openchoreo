// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package auth

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGeneratePKCE(t *testing.T) {
	t.Run("verifier and state are non-empty", func(t *testing.T) {
		p := &PKCEAuth{}
		err := p.GeneratePKCE()
		require.NoError(t, err)
		assert.NotEmpty(t, p.CodeVerifier)
		assert.NotEmpty(t, p.State)
	})

	t.Run("challenge equals base64url(SHA256(verifier))", func(t *testing.T) {
		p := &PKCEAuth{}
		err := p.GeneratePKCE()
		require.NoError(t, err)

		hash := sha256.Sum256([]byte(p.CodeVerifier))
		expectedChallenge := base64.RawURLEncoding.EncodeToString(hash[:])
		assert.Equal(t, expectedChallenge, p.CodeChallenge)
	})

	t.Run("two calls produce different values", func(t *testing.T) {
		p1 := &PKCEAuth{}
		require.NoError(t, p1.GeneratePKCE())

		p2 := &PKCEAuth{}
		require.NoError(t, p2.GeneratePKCE())

		assert.NotEqual(t, p1.CodeVerifier, p2.CodeVerifier)
		assert.NotEqual(t, p1.State, p2.State)
	})
}

func TestGetAuthorizationURL(t *testing.T) {
	p := &PKCEAuth{
		AuthorizationEndpoint: "https://auth.example.com/authorize",
		ClientID:              "my-client",
		RedirectURI:           "http://localhost:8080/callback",
		CodeChallenge:         "test-challenge",
		State:                 "test-state",
		Scopes:                []string{"openid", "profile", "email"},
	}

	t.Run("contains all required params", func(t *testing.T) {
		rawURL, err := p.GetAuthorizationURL()
		require.NoError(t, err)

		parsed, err := url.Parse(rawURL)
		require.NoError(t, err)

		params := parsed.Query()
		assert.Equal(t, "my-client", params.Get("client_id"))
		assert.Equal(t, "http://localhost:8080/callback", params.Get("redirect_uri"))
		assert.Equal(t, "code", params.Get("response_type"))
		assert.Equal(t, "test-challenge", params.Get("code_challenge"))
		assert.Equal(t, "S256", params.Get("code_challenge_method"))
		assert.Equal(t, "test-state", params.Get("state"))
	})

	t.Run("well-formed URL", func(t *testing.T) {
		rawURL, err := p.GetAuthorizationURL()
		require.NoError(t, err)

		parsed, err := url.Parse(rawURL)
		require.NoError(t, err)
		assert.Equal(t, "https", parsed.Scheme)
		assert.Equal(t, "auth.example.com", parsed.Host)
		assert.Equal(t, "/authorize", parsed.Path)
	})

	t.Run("scope is space-joined", func(t *testing.T) {
		rawURL, err := p.GetAuthorizationURL()
		require.NoError(t, err)

		parsed, err := url.Parse(rawURL)
		require.NoError(t, err)
		assert.Equal(t, "openid profile email", parsed.Query().Get("scope"))
	})
}

func TestExchangeAuthCode(t *testing.T) {
	t.Run("successful exchange", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, http.MethodPost, r.Method)
			assert.Equal(t, "application/x-www-form-urlencoded", r.Header.Get("Content-Type"))

			body, err := io.ReadAll(r.Body)
			require.NoError(t, err)
			params, err := url.ParseQuery(string(body))
			require.NoError(t, err)

			assert.Equal(t, "authorization_code", params.Get("grant_type"))
			assert.Equal(t, "test-client", params.Get("client_id"))
			assert.Equal(t, "auth-code-123", params.Get("code"))
			assert.Equal(t, "http://localhost/callback", params.Get("redirect_uri"))
			assert.Equal(t, "test-verifier", params.Get("code_verifier"))

			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(PKCETokenResponse{
				AccessToken:  "access-token-abc",
				RefreshToken: "refresh-token-xyz",
				TokenType:    "Bearer",
				ExpiresIn:    3600,
			})
		}))
		defer server.Close()

		p := &PKCEAuth{
			TokenEndpoint: server.URL,
			ClientID:      "test-client",
			RedirectURI:   "http://localhost/callback",
			CodeVerifier:  "test-verifier",
		}

		resp, err := p.ExchangeAuthCode("auth-code-123")
		require.NoError(t, err)
		assert.Equal(t, "access-token-abc", resp.AccessToken)
		assert.Equal(t, "refresh-token-xyz", resp.RefreshToken)
		assert.Equal(t, "Bearer", resp.TokenType)
		assert.Equal(t, 3600, resp.ExpiresIn)
	})

	t.Run("server returns error status", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":"invalid_grant"}`))
		}))
		defer server.Close()

		p := &PKCEAuth{TokenEndpoint: server.URL, ClientID: "c"}
		_, err := p.ExchangeAuthCode("bad-code")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "token request failed with status 400")
	})

	t.Run("server returns invalid JSON", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`not json`))
		}))
		defer server.Close()

		p := &PKCEAuth{TokenEndpoint: server.URL, ClientID: "c"}
		_, err := p.ExchangeAuthCode("code")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to parse token response")
	})
}

func TestRefreshAccessToken(t *testing.T) {
	t.Run("successful refresh", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, http.MethodPost, r.Method)
			assert.Equal(t, "application/x-www-form-urlencoded", r.Header.Get("Content-Type"))

			body, err := io.ReadAll(r.Body)
			require.NoError(t, err)
			params, err := url.ParseQuery(string(body))
			require.NoError(t, err)

			assert.Equal(t, "refresh_token", params.Get("grant_type"))
			assert.Equal(t, "test-client", params.Get("client_id"))
			assert.Equal(t, "old-refresh-token", params.Get("refresh_token"))

			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(PKCETokenResponse{
				AccessToken:  "new-access-token",
				RefreshToken: "new-refresh-token",
				TokenType:    "Bearer",
				ExpiresIn:    3600,
			})
		}))
		defer server.Close()

		resp, err := RefreshAccessToken(server.URL, "test-client", "old-refresh-token")
		require.NoError(t, err)
		assert.Equal(t, "new-access-token", resp.AccessToken)
		assert.Equal(t, "new-refresh-token", resp.RefreshToken)
		assert.Equal(t, "Bearer", resp.TokenType)
		assert.Equal(t, 3600, resp.ExpiresIn)
	})

	t.Run("server returns error status", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":"invalid_token"}`))
		}))
		defer server.Close()

		_, err := RefreshAccessToken(server.URL, "c", "bad-token")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "refresh request failed with status 401")
	})

	t.Run("server returns invalid JSON", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`not json`))
		}))
		defer server.Close()

		_, err := RefreshAccessToken(server.URL, "c", "token")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to parse refresh response")
	})
}
