// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package auth

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFetchOIDCConfig(t *testing.T) {
	t.Run("happy path assembles OIDCConfig from both endpoints", func(t *testing.T) {
		mux := http.NewServeMux()
		server := httptest.NewServer(mux)
		defer server.Close()

		mux.HandleFunc("/.well-known/oauth-protected-resource", func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode(protectedResourceResponse{
				AuthorizationServers:      []string{server.URL},
				OpenChoreoClients:         []clientInfo{{Name: "cli", ClientID: "cli-client-id", Scopes: []string{"openid", "profile"}}},
				OpenChoreoSecurityEnabled: true,
			})
		})
		mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode(oidcProviderDiscovery{
				AuthorizationEndpoint: "https://auth.example.com/authorize",
				TokenEndpoint:         "https://auth.example.com/token",
				JwksURI:               "https://auth.example.com/jwks",
			})
		})

		cfg, err := FetchOIDCConfig(server.URL)
		require.NoError(t, err)
		assert.Equal(t, "https://auth.example.com/authorize", cfg.AuthorizationEndpoint)
		assert.Equal(t, "https://auth.example.com/token", cfg.TokenEndpoint)
		assert.Equal(t, "cli-client-id", cfg.ClientID)
		assert.Equal(t, []string{"openid", "profile"}, cfg.Scopes)
		assert.True(t, cfg.SecurityEnabled)
		assert.Equal(t, server.URL, cfg.Issuer)
		assert.Equal(t, "https://auth.example.com/jwks", cfg.JwksURI)
	})

	t.Run("no authorization servers", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode(protectedResourceResponse{
				AuthorizationServers: []string{},
				OpenChoreoClients: []clientInfo{
					{Name: "cli", ClientID: "c", Scopes: []string{"openid"}},
				},
			})
		}))
		defer server.Close()

		_, err := FetchOIDCConfig(server.URL)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no authorization_servers found")
	})

	t.Run("no CLI client in openchoreo_clients", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode(protectedResourceResponse{
				AuthorizationServers: []string{"https://issuer.example.com"},
				OpenChoreoClients: []clientInfo{
					{Name: "web", ClientID: "web-id", Scopes: []string{"openid"}},
				},
			})
		}))
		defer server.Close()

		_, err := FetchOIDCConfig(server.URL)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "CLI client configuration (name='cli') not found")
	})

	t.Run("404 from protected resource includes URL hint", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte("not found"))
		}))
		defer server.Close()

		_, err := FetchOIDCConfig(server.URL)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "status 404")
		assert.Contains(t, err.Error(), "control plane URL")
	})

	t.Run("non-404 error from protected resource", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte("internal error"))
		}))
		defer server.Close()

		_, err := FetchOIDCConfig(server.URL)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "status 500")
	})

	t.Run("missing authorization_endpoint in discovery", func(t *testing.T) {
		mux := http.NewServeMux()
		server := httptest.NewServer(mux)
		defer server.Close()

		mux.HandleFunc("/.well-known/oauth-protected-resource", func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode(protectedResourceResponse{
				AuthorizationServers: []string{server.URL},
				OpenChoreoClients:    []clientInfo{{Name: "cli", ClientID: "c", Scopes: []string{"openid"}}},
			})
		})
		mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode(oidcProviderDiscovery{
				TokenEndpoint: "https://auth.example.com/token",
			})
		})

		_, err := FetchOIDCConfig(server.URL)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "authorization_endpoint not found")
	})

	t.Run("missing token_endpoint in discovery", func(t *testing.T) {
		mux := http.NewServeMux()
		server := httptest.NewServer(mux)
		defer server.Close()

		mux.HandleFunc("/.well-known/oauth-protected-resource", func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode(protectedResourceResponse{
				AuthorizationServers: []string{server.URL},
				OpenChoreoClients:    []clientInfo{{Name: "cli", ClientID: "c", Scopes: []string{"openid"}}},
			})
		})
		mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode(oidcProviderDiscovery{
				AuthorizationEndpoint: "https://auth.example.com/authorize",
			})
		})

		_, err := FetchOIDCConfig(server.URL)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "token_endpoint not found")
	})
}
