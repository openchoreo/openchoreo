// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package observerclient

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	obsgen "github.com/openchoreo/openchoreo/internal/observer/api/gen"
	"github.com/openchoreo/openchoreo/internal/occ/cmd/config"
	"github.com/openchoreo/openchoreo/internal/occ/testutil"
)

const (
	controlPlaneURL = "http://mock-control-plane"
	observerURL     = "http://observer.test"
)

func expiredJWT(t *testing.T) string {
	t.Helper()
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"exp": time.Now().Add(-10 * time.Minute).Unix(),
	})
	signed, err := token.SignedString([]byte("test-secret"))
	require.NoError(t, err)
	return signed
}

type recorder struct {
	observerAuth []string
	tokenCalls   int
}

func transport(t *testing.T, issuedToken string, rec *recorder) http.RoundTripper {
	t.Helper()
	return testutil.RoundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host == "observer.test" {
			rec.observerAuth = append(rec.observerAuth, r.Header.Get("Authorization"))
			return testutil.JSONResp(http.StatusOK, obsgen.AuditLogsResponse{}), nil
		}
		switch r.URL.Path {
		case "/.well-known/oauth-protected-resource":
			return testutil.JSONResp(http.StatusOK, map[string]any{
				"authorization_servers":       []string{controlPlaneURL},
				"openchoreo_clients":          []map[string]any{{"name": "cli", "client_id": "cli-id", "scopes": []string{"openid"}}},
				"openchoreo_security_enabled": true,
			}), nil
		case "/.well-known/openid-configuration":
			return testutil.JSONResp(http.StatusOK, map[string]any{
				"authorization_endpoint": controlPlaneURL + "/authorize",
				"token_endpoint":         controlPlaneURL + "/token",
			}), nil
		case "/token":
			rec.tokenCalls++
			return testutil.JSONResp(http.StatusOK, map[string]any{
				"access_token":  issuedToken,
				"refresh_token": "new-refresh-token",
				"token_type":    "Bearer",
				"expires_in":    3600,
			}), nil
		}
		return testutil.JSONResp(http.StatusNotFound, nil), nil
	})
}

func saveConfig(t *testing.T, token string) {
	t.Helper()
	testutil.SetupTestHome(t)
	require.NoError(t, config.SaveStoredConfig(&config.StoredConfig{
		CurrentContext: "ctx",
		ControlPlanes:  []config.ControlPlane{{Name: "cp", URL: controlPlaneURL}},
		Credentials: []config.Credential{{
			Name:         "cred",
			Token:        token,
			RefreshToken: "old-refresh-token",
			ClientID:     "cli-id",
			AuthMethod:   "authorization_code",
		}},
		Contexts: []config.Context{{Name: "ctx", ControlPlane: "cp", Credentials: "cred"}},
	}))
}

func query(t *testing.T, api *obsgen.ClientWithResponses) {
	t.Helper()
	now := time.Now()
	resp, err := api.QueryAuditLogsWithResponse(context.Background(),
		obsgen.AuditLogsQueryRequest{StartTime: now.Add(-time.Hour), EndTime: now})
	require.NoError(t, err)
	require.NotNil(t, resp.JSON200)
}

func TestNew_SendsCurrentToken(t *testing.T) {
	saveConfig(t, testutil.NonExpiredJWT)
	var rec recorder
	testutil.SetTransport(t, transport(t, "unused", &rec))

	api, err := New(observerURL, testutil.NonExpiredJWT)
	require.NoError(t, err)
	query(t, api)

	assert.Equal(t, []string{"Bearer " + testutil.NonExpiredJWT}, rec.observerAuth)
	assert.Zero(t, rec.tokenCalls)
}

func TestNew_RefreshesExpiredTokenOnce(t *testing.T) {
	expired := expiredJWT(t)
	saveConfig(t, expired)
	var rec recorder
	testutil.SetTransport(t, transport(t, testutil.NonExpiredJWT, &rec))

	api, err := New(observerURL, expired)
	require.NoError(t, err)
	query(t, api)
	query(t, api)

	assert.Equal(t, []string{
		"Bearer " + testutil.NonExpiredJWT,
		"Bearer " + testutil.NonExpiredJWT,
	}, rec.observerAuth)
	assert.Equal(t, 1, rec.tokenCalls)
}
