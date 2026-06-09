// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package auth

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClientCredentialsGetToken(t *testing.T) {
	t.Run("successful token exchange", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, http.MethodPost, r.Method)
			assert.Equal(t, "application/x-www-form-urlencoded", r.Header.Get("Content-Type"))

			body, err := io.ReadAll(r.Body)
			require.NoError(t, err)
			params, err := url.ParseQuery(string(body))
			require.NoError(t, err)

			assert.Equal(t, "client_credentials", params.Get("grant_type"))
			assert.Equal(t, "my-client", params.Get("client_id"))
			assert.Equal(t, "my-secret", params.Get("client_secret"))

			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(TokenResponse{
				AccessToken: "access-token-123",
				TokenType:   "Bearer",
				ExpiresIn:   3600,
				Scope:       "openid",
			})
		}))
		defer server.Close()

		c := &ClientCredentialsAuth{
			TokenEndpoint: server.URL,
			ClientID:      "my-client",
			ClientSecret:  "my-secret",
		}

		resp, err := c.GetToken()
		require.NoError(t, err)
		assert.Equal(t, "access-token-123", resp.AccessToken)
		assert.Equal(t, "Bearer", resp.TokenType)
		assert.Equal(t, 3600, resp.ExpiresIn)
		assert.Equal(t, "openid", resp.Scope)
	})

	t.Run("server returns error status", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":"invalid_client"}`))
		}))
		defer server.Close()

		c := &ClientCredentialsAuth{TokenEndpoint: server.URL, ClientID: "c", ClientSecret: "s"}
		_, err := c.GetToken()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "token request failed with status 401")
	})

	t.Run("server returns invalid JSON", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`not json`))
		}))
		defer server.Close()

		c := &ClientCredentialsAuth{TokenEndpoint: server.URL, ClientID: "c", ClientSecret: "s"}
		_, err := c.GetToken()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to parse token response")
	})

	t.Run("includes scope in request when Scope is set", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			body, err := io.ReadAll(r.Body)
			require.NoError(t, err)
			params, err := url.ParseQuery(string(body))
			require.NoError(t, err)

			assert.Equal(t, "openid profile", params.Get("scope"))

			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(TokenResponse{AccessToken: "tok", TokenType: "Bearer", ExpiresIn: 3600})
		}))
		defer server.Close()

		c := &ClientCredentialsAuth{
			TokenEndpoint: server.URL,
			ClientID:      "c",
			ClientSecret:  "s",
			Scope:         "openid profile",
		}
		resp, err := c.GetToken()
		require.NoError(t, err)
		assert.Equal(t, "tok", resp.AccessToken)
	})

	t.Run("omits scope from request when Scope is empty", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			body, err := io.ReadAll(r.Body)
			require.NoError(t, err)
			params, err := url.ParseQuery(string(body))
			require.NoError(t, err)

			assert.Equal(t, "", params.Get("scope"))

			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(TokenResponse{AccessToken: "tok", TokenType: "Bearer", ExpiresIn: 3600})
		}))
		defer server.Close()

		c := &ClientCredentialsAuth{TokenEndpoint: server.URL, ClientID: "c", ClientSecret: "s"}
		_, err := c.GetToken()
		require.NoError(t, err)
	})
}
