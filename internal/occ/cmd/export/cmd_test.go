// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package export

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openchoreo/openchoreo/internal/occ/resources/client"
	"github.com/openchoreo/openchoreo/internal/occ/testutil"
)

func TestExportCommand_ExportsWithBothFlagsAndCurrentDirectory(t *testing.T) {
	for _, tc := range []struct{ name, flag, directory string }{
		{name: "short flag", flag: "-o", directory: "nested/export"},
		{name: "long flag", flag: "--output", directory: "nested/export"},
		{name: "current directory", flag: "-o", directory: "."},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Chdir(t.TempDir())
			f := newFetchFixture(t, false)
			transport := http.DefaultTransport
			testutil.SetTransport(t, testutil.RoundTripFunc(func(req *http.Request) (*http.Response, error) {
				switch req.URL.Path {
				case "/.well-known/oauth-protected-resource":
					return testutil.JSONResp(http.StatusOK, map[string]any{
						"authorization_servers":       []string{"https://issuer.example.test"},
						"openchoreo_security_enabled": true,
						"openchoreo_clients":          []any{map[string]any{"name": "cli", "client_id": "test-cli"}},
					}), nil
				case "/.well-known/openid-configuration":
					return testutil.JSONResp(http.StatusOK, map[string]string{
						"authorization_endpoint": "https://issuer.example.test/authorize",
						"token_endpoint":         "https://issuer.example.test/token",
					}), nil
				default:
					return transport.RoundTrip(req)
				}
			}))
			if tc.directory == "." {
				require.NoError(t, os.WriteFile("README.md", []byte("keep repository content\n"), 0o600))
			}
			ctx := context.WithValue(context.Background(), fetchContextKey{}, "command")
			f.override = func(req *http.Request) (*http.Response, error) {
				assert.Equal(t, "command", req.Context().Value(fetchContextKey{}))
				return nil, nil
			}
			cmd := NewExportCmd(func() (client.Interface, error) { return f.client, nil })
			cmd.SetContext(ctx)
			var output bytes.Buffer
			cmd.SetOut(&output)
			cmd.SetErr(&bytes.Buffer{})
			cmd.SetArgs([]string{tc.flag, tc.directory})

			require.NoError(t, cmd.Execute())
			assert.Equal(t, "Exported 58 resources to "+tc.directory+".\n", output.String())
			assert.FileExists(t, filepath.Join(tc.directory, "namespaces", "acme", "namespace.yaml"))
			assert.FileExists(t, filepath.Join(tc.directory, "namespaces", "acme", "projects", "store", "components", "checkout", "workloads", "workloads-0.yaml"))
			if tc.directory == "." {
				data, err := os.ReadFile("README.md")
				require.NoError(t, err)
				assert.Equal(t, "keep repository content\n", string(data))
			}
		})
	}
}

func TestExportCommand_ClientCreationError(t *testing.T) {
	wantErr := errors.New("no control plane configured")
	cmd := NewExportCmd(func() (client.Interface, error) { return nil, wantErr })
	require.NoError(t, cmd.Flags().Set("output", t.TempDir()))
	var output bytes.Buffer
	cmd.SetOut(&output)

	require.ErrorIs(t, cmd.RunE(cmd, nil), wantErr)
	assert.Empty(t, output.String())
}

func TestExportCommand_FetchError(t *testing.T) {
	f := newFetchFixture(t, false)
	f.override = func(_ *http.Request) (*http.Response, error) {
		return testutil.JSONResp(http.StatusForbidden, map[string]string{"error": "access denied"}), nil
	}
	cmd := NewExportCmd(func() (client.Interface, error) { return f.client, nil })
	directory := filepath.Join(t.TempDir(), "export")
	require.NoError(t, cmd.Flags().Set("output", directory))
	var output bytes.Buffer
	cmd.SetContext(context.Background())
	cmd.SetOut(&output)

	require.ErrorContains(t, cmd.RunE(cmd, nil), "access denied")
	assert.Empty(t, output.String(), "a failed fetch must not print a success count")
	assert.NoDirExists(t, directory)
}

func TestExportCommand_RequiresLogin(t *testing.T) {
	testutil.SetupTestHome(t)
	called := false
	cmd := NewExportCmd(func() (client.Interface, error) {
		called = true
		return nil, errors.New("must not create client before login")
	})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"-o", t.TempDir()})

	require.ErrorContains(t, cmd.Execute(), "Authentication required")
	assert.False(t, called)
}

func TestExportCommand_RejectsArguments(t *testing.T) {
	called := false
	cmd := NewExportCmd(func() (client.Interface, error) {
		called = true
		return nil, errors.New("unexpected client creation")
	})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"unexpected"})

	require.Error(t, cmd.Execute())
	assert.False(t, called)
}

func TestExportCommand_RequiresOutputBeforeClientCreation(t *testing.T) {
	for _, args := range [][]string{nil, {"-o", ""}, {"--output", "   "}} {
		cmd := NewExportCmd(func() (client.Interface, error) {
			t.Fatal("must not create a client without an output directory")
			return nil, nil
		})
		cmd.SetOut(&bytes.Buffer{})
		cmd.SetErr(&bytes.Buffer{})
		cmd.SetArgs(args)
		require.ErrorContains(t, cmd.Execute(), "output directory is required")
	}
}

func TestExportCommand_AllReleasesFlag(t *testing.T) {
	old := time.Date(2000, time.January, 1, 0, 0, 0, 0, time.UTC)
	for _, all := range []bool{false, true} {
		t.Run(strconv.FormatBool(all), func(t *testing.T) {
			f := newFetchFixture(t, false)
			tc := releaseTestCases[0]
			f.override = func(req *http.Request) (*http.Response, error) {
				if req.URL.Path == "/api/v1/namespaces/acme/"+tc.collection {
					return testutil.JSONResp(http.StatusOK, map[string]any{
						"items": []any{releaseTestObject(tc, "acme", "historical", &old)}, "pagination": map[string]any{},
					}), nil
				}
				return nil, nil
			}
			directory := t.TempDir()
			cmd := NewExportCmd(func() (client.Interface, error) { return f.client, nil })
			require.NoError(t, cmd.Flags().Set("output", directory))
			assert.Equal(t, "false", cmd.Flags().Lookup("all-releases").DefValue)
			require.NoError(t, cmd.Flags().Set("all-releases", strconv.FormatBool(all)))
			cmd.SetContext(context.Background())
			var output bytes.Buffer
			cmd.SetOut(&output)
			require.NoError(t, cmd.RunE(cmd, nil))
			name := filepath.Join(directory, "namespaces", "acme", "projects", "store", "components", "checkout", "releases", "historical.yaml")
			if all {
				assert.FileExists(t, name)
				assert.Contains(t, output.String(), "Exported 58 resources")
			} else {
				assert.NoFileExists(t, name)
				assert.Contains(t, output.String(), "Exported 57 resources")
			}
		})
	}
}
