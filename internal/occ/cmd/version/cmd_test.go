// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package version

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openchoreo/openchoreo/internal/occ/testutil"
)

// roundTripFunc lets a plain function satisfy http.RoundTripper.
type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// jsonResponse builds a 200 OK *http.Response with a JSON body.
func jsonResponse(t *testing.T, body any) *http.Response {
	t.Helper()
	b, err := json.Marshal(body)
	require.NoError(t, err)
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(bytes.NewReader(b)),
		Header:     http.Header{"Content-Type": []string{"application/json"}},
	}
}

// setTransport replaces http.DefaultTransport for the duration of the test.
func setTransport(t *testing.T, rt http.RoundTripper) {
	t.Helper()
	original := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = original })
	http.DefaultTransport = rt
}

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

	setTransport(t, roundTripFunc(func(r *http.Request) (*http.Response, error) {
		assert.Equal(t, cpURL+"/version", r.URL.String())
		return jsonResponse(t, expected), nil
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

	result, err := fetchServerVersion()
	require.NoError(t, err)
	assert.Equal(t, expected.Version, result.Version)
	assert.Equal(t, expected.GitRevision, result.GitRevision)
	assert.Equal(t, expected.GoOS, result.GoOS)
}

func TestFetchServerVersion_ServerError(t *testing.T) {
	const cpURL = "http://mock-control-plane"

	setTransport(t, roundTripFunc(func(_ *http.Request) (*http.Response, error) {
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
