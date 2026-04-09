// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package auth

import (
	"bytes"
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openchoreo/openchoreo/internal/occ/testutil"
)

const (
	mockAPIProtectedResource = "mock-api/.well-known/oauth-protected-resource"
	mockIssuerOIDCDiscovery  = "mock-issuer/.well-known/openid-configuration"
)

func TestFetchOIDCConfig(t *testing.T) {
	const issuer = "http://mock-issuer"
	const apiURL = "http://mock-api"

	t.Run("happy path assembles OIDCConfig from both endpoints", func(t *testing.T) {
		testutil.SetTransport(t, testutil.RoundTripFunc(func(r *http.Request) (*http.Response, error) {
			switch r.URL.Host + r.URL.Path {
			case mockAPIProtectedResource:
				return testutil.JSONResp(http.StatusOK, protectedResourceResponse{
					AuthorizationServers:      []string{issuer},
					OpenChoreoClients:         []clientInfo{{Name: "cli", ClientID: "cli-client-id", Scopes: []string{"openid", "profile"}}},
					OpenChoreoSecurityEnabled: true,
				}), nil
			case mockIssuerOIDCDiscovery:
				return testutil.JSONResp(http.StatusOK, oidcProviderDiscovery{
					AuthorizationEndpoint: "https://auth.example.com/authorize",
					TokenEndpoint:         "https://auth.example.com/token",
					JwksURI:               "https://auth.example.com/jwks",
				}), nil
			default:
				return &http.Response{StatusCode: http.StatusNotFound, Body: http.NoBody, Header: http.Header{}}, nil
			}
		}))

		cfg, err := FetchOIDCConfig(apiURL)
		require.NoError(t, err)
		assert.Equal(t, "https://auth.example.com/authorize", cfg.AuthorizationEndpoint)
		assert.Equal(t, "https://auth.example.com/token", cfg.TokenEndpoint)
		assert.Equal(t, "cli-client-id", cfg.ClientID)
		assert.Equal(t, []string{"openid", "profile"}, cfg.Scopes)
		assert.True(t, cfg.SecurityEnabled)
		assert.Equal(t, issuer, cfg.Issuer)
		assert.Equal(t, "https://auth.example.com/jwks", cfg.JwksURI)
	})

	t.Run("no authorization servers", func(t *testing.T) {
		testutil.SetTransport(t, testutil.RoundTripFunc(func(r *http.Request) (*http.Response, error) {
			return testutil.JSONResp(http.StatusOK, protectedResourceResponse{
				AuthorizationServers: []string{},
				OpenChoreoClients:    []clientInfo{{Name: "cli", ClientID: "c", Scopes: []string{"openid"}}},
			}), nil
		}))

		_, err := FetchOIDCConfig(apiURL)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no authorization_servers found")
	})

	t.Run("no CLI client in openchoreo_clients", func(t *testing.T) {
		testutil.SetTransport(t, testutil.RoundTripFunc(func(r *http.Request) (*http.Response, error) {
			return testutil.JSONResp(http.StatusOK, protectedResourceResponse{
				AuthorizationServers: []string{issuer},
				OpenChoreoClients:    []clientInfo{{Name: "web", ClientID: "web-id", Scopes: []string{"openid"}}},
			}), nil
		}))

		_, err := FetchOIDCConfig(apiURL)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "CLI client configuration (name='cli') not found")
	})

	t.Run("404 from protected resource includes URL hint", func(t *testing.T) {
		testutil.SetTransport(t, testutil.RoundTripFunc(func(r *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusNotFound,
				Body:       io.NopCloser(bytes.NewReader([]byte("not found"))),
				Header:     http.Header{},
			}, nil
		}))

		_, err := FetchOIDCConfig(apiURL)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "status 404")
		assert.Contains(t, err.Error(), "control plane URL")
	})

	t.Run("non-404 error from protected resource", func(t *testing.T) {
		testutil.SetTransport(t, testutil.RoundTripFunc(func(r *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusInternalServerError,
				Body:       io.NopCloser(bytes.NewReader([]byte("internal error"))),
				Header:     http.Header{},
			}, nil
		}))

		_, err := FetchOIDCConfig(apiURL)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "status 500")
	})

	t.Run("missing authorization_endpoint in discovery", func(t *testing.T) {
		testutil.SetTransport(t, testutil.RoundTripFunc(func(r *http.Request) (*http.Response, error) {
			switch r.URL.Host + r.URL.Path {
			case mockAPIProtectedResource:
				return testutil.JSONResp(http.StatusOK, protectedResourceResponse{
					AuthorizationServers: []string{issuer},
					OpenChoreoClients:    []clientInfo{{Name: "cli", ClientID: "c", Scopes: []string{"openid"}}},
				}), nil
			case mockIssuerOIDCDiscovery:
				return testutil.JSONResp(http.StatusOK, oidcProviderDiscovery{
					TokenEndpoint: "https://auth.example.com/token",
				}), nil
			default:
				return &http.Response{StatusCode: http.StatusNotFound, Body: http.NoBody, Header: http.Header{}}, nil
			}
		}))

		_, err := FetchOIDCConfig(apiURL)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "authorization_endpoint not found")
	})

	t.Run("missing token_endpoint in discovery", func(t *testing.T) {
		testutil.SetTransport(t, testutil.RoundTripFunc(func(r *http.Request) (*http.Response, error) {
			switch r.URL.Host + r.URL.Path {
			case mockAPIProtectedResource:
				return testutil.JSONResp(http.StatusOK, protectedResourceResponse{
					AuthorizationServers: []string{issuer},
					OpenChoreoClients:    []clientInfo{{Name: "cli", ClientID: "c", Scopes: []string{"openid"}}},
				}), nil
			case mockIssuerOIDCDiscovery:
				return testutil.JSONResp(http.StatusOK, oidcProviderDiscovery{
					AuthorizationEndpoint: "https://auth.example.com/authorize",
				}), nil
			default:
				return &http.Response{StatusCode: http.StatusNotFound, Body: http.NoBody, Header: http.Header{}}, nil
			}
		}))

		_, err := FetchOIDCConfig(apiURL)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "token_endpoint not found")
	})
}
