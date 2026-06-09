// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package eventforwarder

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestHealthServer builds a HealthServer wired to an httptest server and
// returns both the HealthServer (so the test can flip readiness) and a base
// URL for sending requests.
func newTestHealthServer(t *testing.T) (*HealthServer, string, func()) {
	t.Helper()
	hs := NewHealthServer(slog.Default())
	ts := httptest.NewServer(hs.Handler())
	return hs, ts.URL, ts.Close
}

func TestHealthServer_HealthAlwaysOK(t *testing.T) {
	_, url, cleanup := newTestHealthServer(t)
	defer cleanup()

	res, err := http.Get(url + "/health")
	require.NoError(t, err)
	defer res.Body.Close()

	assert.Equal(t, http.StatusOK, res.StatusCode)
	assert.Equal(t, "application/json", res.Header.Get("Content-Type"))

	body, _ := io.ReadAll(res.Body)
	var payload map[string]string
	require.NoError(t, json.Unmarshal(body, &payload))
	assert.Equal(t, "ok", payload["status"])
}

func TestHealthServer_ReadyReflectsState(t *testing.T) {
	hs, url, cleanup := newTestHealthServer(t)
	defer cleanup()

	// Before SetReady: 503 with "not ready" body
	res, err := http.Get(url + "/ready")
	require.NoError(t, err)
	body, _ := io.ReadAll(res.Body)
	res.Body.Close()
	assert.Equal(t, http.StatusServiceUnavailable, res.StatusCode)
	var payload map[string]string
	require.NoError(t, json.Unmarshal(body, &payload))
	assert.Equal(t, "not ready", payload["status"])

	// After SetReady: 200 with "ready" body
	hs.SetReady()
	res, err = http.Get(url + "/ready")
	require.NoError(t, err)
	body, _ = io.ReadAll(res.Body)
	res.Body.Close()
	assert.Equal(t, http.StatusOK, res.StatusCode)
	require.NoError(t, json.Unmarshal(body, &payload))
	assert.Equal(t, "ready", payload["status"])
}

func TestHealthServer_NonGetMethodsRejected(t *testing.T) {
	_, url, cleanup := newTestHealthServer(t)
	defer cleanup()

	// http.NewServeMux + "GET /health" pattern returns 405 for non-GET.
	res, err := http.Post(url+"/health", "application/json", nil)
	require.NoError(t, err)
	res.Body.Close()
	assert.Equal(t, http.StatusMethodNotAllowed, res.StatusCode)
}
