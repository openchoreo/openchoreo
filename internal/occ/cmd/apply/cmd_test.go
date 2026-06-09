// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package apply

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openchoreo/openchoreo/internal/occ/cmd/config"
	"github.com/openchoreo/openchoreo/internal/occ/resources/client"
	"github.com/openchoreo/openchoreo/internal/occ/testutil"
)

// newClientFactory wraps client.NewClient into a NewClientFunc.
func newClientFactory() client.NewClientFunc {
	return func() (client.Interface, error) {
		return client.NewClient()
	}
}

// --- NewApplyCmd structure ---

func TestNewApplyCmd_Structure(t *testing.T) {
	f := func() (client.Interface, error) { return nil, fmt.Errorf("unused") }
	cmd := NewApplyCmd(f)

	assert.Equal(t, "apply", cmd.Use)
	assert.NotEmpty(t, cmd.Short)
	assert.NotEmpty(t, cmd.Long)
	assert.NotNil(t, cmd.RunE)
	assert.NotNil(t, cmd.PreRunE)
}

func TestNewApplyCmd_FileFlag(t *testing.T) {
	f := func() (client.Interface, error) { return nil, fmt.Errorf("unused") }
	cmd := NewApplyCmd(f)

	flag := cmd.Flags().Lookup("file")
	require.NotNil(t, flag, "expected --file flag")
	assert.Equal(t, "f", flag.Shorthand)
	assert.Equal(t, "", flag.DefValue)
}

// --- RunE: factory error ---

func TestNewApplyCmd_FactoryError(t *testing.T) {
	f := func() (client.Interface, error) { return nil, fmt.Errorf("factory failed") }
	cmd := NewApplyCmd(f)

	err := cmd.RunE(cmd, nil)
	require.Error(t, err)
	assert.EqualError(t, err, "factory failed")
}

// --- RunE: missing file flag ---

func TestNewApplyCmd_RunE_NoFile(t *testing.T) {
	home := testutil.SetupTestHome(t)
	testutil.WriteOCConfig(t, home, config.StoredConfig{
		CurrentContext: "test",
		ControlPlanes:  []config.ControlPlane{{Name: "cp", URL: "http://mock"}},
		Credentials:    []config.Credential{{Name: "cred", Token: testutil.NonExpiredJWT}},
		Contexts:       []config.Context{{Name: "test", ControlPlane: "cp", Credentials: "cred"}},
	})

	testutil.SetTransport(t, testutil.RoundTripFunc(func(_ *http.Request) (*http.Response, error) {
		t.Fatal("no HTTP call expected")
		return nil, nil
	}))

	cmd := NewApplyCmd(newClientFactory())
	err := cmd.RunE(cmd, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "file path is required")
}

// --- RunE: happy path (create namespace) ---

func TestNewApplyCmd_RunE_CreateNamespace(t *testing.T) {
	home := testutil.SetupTestHome(t)
	testutil.WriteOCConfig(t, home, config.StoredConfig{
		CurrentContext: "test",
		ControlPlanes:  []config.ControlPlane{{Name: "cp", URL: "http://mock-api"}},
		Credentials:    []config.Credential{{Name: "cred", Token: testutil.NonExpiredJWT}},
		Contexts:       []config.Context{{Name: "test", ControlPlane: "cp", Credentials: "cred"}},
	})

	testutil.SetTransport(t, testutil.RoundTripFunc(func(r *http.Request) (*http.Response, error) {
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/namespaces/cmd-ns"):
			return &http.Response{
				StatusCode: http.StatusNotFound,
				Body:       io.NopCloser(bytes.NewReader([]byte(`{"error":"not found"}`))),
				Header:     http.Header{},
			}, nil
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/namespaces"):
			return testutil.JSONResp(http.StatusCreated, map[string]any{
				"metadata": map[string]any{"name": "cmd-ns"},
			}), nil
		default:
			return &http.Response{
				StatusCode: http.StatusNotFound,
				Body:       http.NoBody,
				Header:     http.Header{},
			}, nil
		}
	}))

	dir := t.TempDir()
	yamlFile := filepath.Join(dir, "ns.yaml")
	require.NoError(t, os.WriteFile(yamlFile, []byte(`kind: Namespace
apiVersion: core.openchoreo.dev/v1alpha1
metadata:
  name: cmd-ns
`), 0600))

	cmd := NewApplyCmd(newClientFactory())
	require.NoError(t, cmd.Flags().Set("file", yamlFile))

	out := testutil.CaptureStdout(t, func() {
		err := cmd.RunE(cmd, nil)
		require.NoError(t, err)
	})
	assert.Contains(t, out, "namespace/cmd-ns created")
}

// --- RunE: happy path (update existing namespace) ---

func TestNewApplyCmd_RunE_UpdateNamespace(t *testing.T) {
	home := testutil.SetupTestHome(t)
	testutil.WriteOCConfig(t, home, config.StoredConfig{
		CurrentContext: "test",
		ControlPlanes:  []config.ControlPlane{{Name: "cp", URL: "http://mock-api"}},
		Credentials:    []config.Credential{{Name: "cred", Token: testutil.NonExpiredJWT}},
		Contexts:       []config.Context{{Name: "test", ControlPlane: "cp", Credentials: "cred"}},
	})

	testutil.SetTransport(t, testutil.RoundTripFunc(func(r *http.Request) (*http.Response, error) {
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/namespaces/upd-ns"):
			return testutil.JSONResp(http.StatusOK, map[string]any{
				"metadata": map[string]any{"name": "upd-ns"},
			}), nil
		case r.Method == http.MethodPut && strings.HasSuffix(r.URL.Path, "/namespaces/upd-ns"):
			return testutil.JSONResp(http.StatusOK, map[string]any{
				"metadata": map[string]any{"name": "upd-ns"},
			}), nil
		default:
			return &http.Response{
				StatusCode: http.StatusNotFound,
				Body:       http.NoBody,
				Header:     http.Header{},
			}, nil
		}
	}))

	dir := t.TempDir()
	yamlFile := filepath.Join(dir, "ns.yaml")
	require.NoError(t, os.WriteFile(yamlFile, []byte(`kind: Namespace
metadata:
  name: upd-ns
`), 0600))

	cmd := NewApplyCmd(newClientFactory())
	require.NoError(t, cmd.Flags().Set("file", yamlFile))

	out := testutil.CaptureStdout(t, func() {
		err := cmd.RunE(cmd, nil)
		require.NoError(t, err)
	})
	assert.Contains(t, out, "namespace/upd-ns configured")
}
