// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package version

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openchoreo/openchoreo/internal/occ/testutil"
)

func TestNewVersionCmd_Use(t *testing.T) {
	cmd := NewVersionCmd()
	assert.Equal(t, "version", cmd.Use)
}

func TestNewVersionCmd_PrintsClientVersion(t *testing.T) {
	// Set up isolated home so no real config is read.
	testutil.SetupTestHome(t)

	cmd := NewVersionCmd()
	out := testutil.CaptureStdout(t, func() {
		err := cmd.RunE(cmd, nil)
		require.NoError(t, err)
	})

	assert.Contains(t, out, "Client:")
	assert.Contains(t, out, "Version:")
	assert.Contains(t, out, "Git Revision:")
	assert.Contains(t, out, "Build Time:")
	assert.Contains(t, out, "Go Version:")
}

func TestNewVersionCmd_NoServer(t *testing.T) {
	// With no config, server should show <not connected>.
	testutil.SetupTestHome(t)

	cmd := NewVersionCmd()
	out := testutil.CaptureStdout(t, func() {
		err := cmd.RunE(cmd, nil)
		require.NoError(t, err)
	})

	assert.Contains(t, out, "Server: <not connected>")
}

func TestFetchServerVersion_Success(t *testing.T) {
	const cpURL = "http://mock-control-plane"

	expected := serverVersionResponse{
		Name:        "openchoreo",
		Version:     "v0.1.0",
		GitRevision: "abc123",
		BuildTime:   "2026-01-01T00:00:00Z",
		GoOS:        "linux",
		GoArch:      "amd64",
		GoVersion:   "go1.23.0",
	}

	testutil.SetTransport(t, testutil.RoundTripFunc(func(r *http.Request) (*http.Response, error) {
		assert.Equal(t, cpURL+"/version", r.URL.String())
		return testutil.JSONResp(http.StatusOK, expected), nil
	}))
	writeTestConfig(t, cpURL)

	// Verify fetchServerVersion returns the correct data.
	result, err := fetchServerVersion()
	require.NoError(t, err)
	assert.Equal(t, expected.Version, result.Version)
	assert.Equal(t, expected.GitRevision, result.GitRevision)
	assert.Equal(t, expected.GoOS, result.GoOS)

	// Verify the command prints both client and server sections.
	cmd := NewVersionCmd()
	out := testutil.CaptureStdout(t, func() {
		err := cmd.RunE(cmd, nil)
		require.NoError(t, err)
	})

	assert.Contains(t, out, "Client:")
	assert.Contains(t, out, "Server:")
	assert.Contains(t, out, expected.Version)
	assert.Contains(t, out, expected.GitRevision)
}

func TestFetchServerVersion_ServerError(t *testing.T) {
	const cpURL = "http://mock-control-plane"

	testutil.SetTransport(t, testutil.RoundTripFunc(func(_ *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusInternalServerError,
			Body:       http.NoBody,
			Header:     http.Header{},
		}, nil
	}))

	home := testutil.SetupTestHome(t)
	testutil.WriteOCConfig(t, home, map[string]any{
		"currentContext": "test",
		"contexts": []map[string]any{
			{"name": "test", "controlplane": "test-cp", "credentials": "test-cred"},
		},
		"controlplanes": []map[string]any{
			{"name": "test-cp", "url": cpURL},
		},
		"credentials": []map[string]any{
			{"name": "test-cred"},
		},
	})

	_, err := fetchServerVersion()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "500")
}

func TestFetchServerVersion_NoConfig(t *testing.T) {
	testutil.SetupTestHome(t)

	_, err := fetchServerVersion()
	require.Error(t, err)
}

// writeTestConfig writes a minimal OC config pointing at the given control plane URL.
func writeTestConfig(t *testing.T, cpURL string) {
	t.Helper()
	home := testutil.SetupTestHome(t)
	testutil.WriteOCConfig(t, home, map[string]any{
		"currentContext": "test",
		"contexts": []map[string]any{
			{"name": "test", "controlplane": "test-cp", "credentials": "test-cred"},
		},
		"controlplanes": []map[string]any{
			{"name": "test-cp", "url": cpURL},
		},
		"credentials": []map[string]any{
			{"name": "test-cred"},
		},
	})
}

func TestFetchServerVersion_EmptyURL(t *testing.T) {
	writeTestConfig(t, "")

	_, err := fetchServerVersion()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "control plane URL not configured")
}

func TestFetchServerVersion_NetworkError(t *testing.T) {
	const cpURL = "http://mock-control-plane"

	testutil.SetTransport(t, testutil.RoundTripFunc(func(_ *http.Request) (*http.Response, error) {
		return nil, fmt.Errorf("connection refused")
	}))
	writeTestConfig(t, cpURL)

	_, err := fetchServerVersion()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to fetch server version")
}

func TestFetchServerVersion_InvalidJSON(t *testing.T) {
	const cpURL = "http://mock-control-plane"

	testutil.SetTransport(t, testutil.RoundTripFunc(func(_ *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(bytes.NewReader([]byte("not json"))),
			Header:     http.Header{},
		}, nil
	}))
	writeTestConfig(t, cpURL)

	_, err := fetchServerVersion()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse server version")
}
