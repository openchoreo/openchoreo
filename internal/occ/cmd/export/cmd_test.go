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
	"strings"
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
			f.profileOverride = func(_ *http.Request) (*http.Response, error) {
				assert.Equal(t, "Exporting resources...\n", output.String(), "progress must precede API calls and completion must follow commit")
				return testutil.JSONResp(http.StatusOK, fullExportProfile()), nil
			}

			require.NoError(t, cmd.Execute())
			assert.True(t, strings.HasPrefix(output.String(), "Exporting resources...\n\nExport completed\n"))
			resolved, err := filepath.Abs(tc.directory)
			require.NoError(t, err)
			resolved, err = filepath.EvalSymlinks(resolved)
			require.NoError(t, err)
			assert.Contains(t, output.String(), "  Output:      "+resolved+"\n")
			assert.Contains(t, output.String(), "  Resources:   69 selected resource(s)\n")
			assert.Contains(t, output.String(), "CREATED   UPDATED   UNCHANGED   DELETED")
			assert.Contains(t, output.String(), "Cutoff (UTC):")
			assert.NotContains(t, output.String(), "PRESERVED")
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
	assert.Equal(t, "Exporting resources...\n", output.String(), "a failed fetch must not print a completion summary")
	assert.NoDirExists(t, directory)
}

func TestExportCommand_MissingReferencedReleasesWarnsAndCompletes(t *testing.T) {
	f := newFetchFixture(t, false)
	tc := releaseTestCases[0]
	f.override = func(req *http.Request) (*http.Response, error) {
		if req.URL.Path == "/api/v1/namespaces/acme/"+tc.bindings {
			first := bindingTestObject(tc, "missing-z")
			second := bindingTestObject(tc, "missing-a")
			second["metadata"].(map[string]any)["name"] = "another-binding"
			return testutil.JSONResp(http.StatusOK, map[string]any{
				"items": []any{first, second}, "pagination": map[string]any{},
			}), nil
		}
		return nil, nil
	}
	cmd := NewExportCmd(func() (client.Interface, error) { return f.client, nil })
	directory := t.TempDir()
	require.NoError(t, cmd.Flags().Set("output", directory))
	cmd.SetContext(context.Background())
	var output, warnings bytes.Buffer
	cmd.SetOut(&output)
	cmd.SetErr(&warnings)

	require.NoError(t, cmd.RunE(cmd, nil))
	assert.Equal(t, "Warning: binding-referenced release is missing or marked for deletion: ComponentRelease \"missing-a\" in namespace \"acme\"\n"+
		"Warning: binding-referenced release is missing or marked for deletion: ComponentRelease \"missing-z\" in namespace \"acme\"\n", warnings.String())
	assert.Contains(t, output.String(), "Export completed")
	assert.NotContains(t, output.String(), "Warning:")
	assert.FileExists(t, filepath.Join(directory, "namespaces", "acme", "projects", "store", "components", "checkout", "release-bindings", "another-binding.yaml"))
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
	for _, args := range [][]string{{}, {"-o", ""}, {"--output", "   "}} {
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
				assert.Contains(t, output.String(), "Resources:   69 selected resource(s)")
				assert.Contains(t, output.String(), "Policy:        All releases")
				assert.NotContains(t, output.String(), "Cutoff (UTC):")
			} else {
				assert.NoFileExists(t, name)
				assert.Contains(t, output.String(), "Resources:   68 selected resource(s)")
				assert.Contains(t, output.String(), "Policy:        Last 3 calendar months, plus binding-referenced releases")
			}
		})
	}
}

func TestWriteExportSummary_FormatsCommittedCountsAndExactCutoff(t *testing.T) {
	cutoff := time.Date(2026, time.August, 19, 14, 39, 4, 801915000, time.FixedZone("test", 19800))
	result := &ExportResult{
		OutputDir: "/repo/gitops", Files: make([]SerializedFile, 123),
		Counts: FileCounts{Created: 4, Updated: 91, Unchanged: 28, Deleted: 2}, ReleaseCutoff: &cutoff,
	}
	var output bytes.Buffer
	require.NoError(t, writeExportSummary(&output, result))
	assert.Equal(t, `
Export completed
  Output:      /repo/gitops
  Resources:   123 selected resource(s)

Files:
  CREATED   UPDATED   UNCHANGED   DELETED
  4         91        28          2

Release history:
  Policy:        Last 3 calendar months, plus binding-referenced releases
  Cutoff (UTC):  2026-08-19T09:09:04.801915Z (inclusive)
`, output.String())
}

func TestExportCommand_HelpDescribesExportAndReleasePolicy(t *testing.T) {
	cmd := NewExportCmd(func() (client.Interface, error) {
		t.Fatal("help must not create an API client")
		return nil, nil
	})
	var output bytes.Buffer
	cmd.SetOut(&output)
	cmd.SetErr(&output)
	cmd.SetArgs([]string{"-h"})
	require.NoError(t, cmd.Execute())
	for _, text := range []string{
		"GitOps", "one YAML manifest per resource", "unrestricted",
		"including local edits", "resources deleted from the cluster", "Unrelated files",
		"changed managed manifests",
		"three calendar months", "binding-referenced", "--all-releases", "-o, --output",
		"summary of file changes and release history",
	} {
		assert.Contains(t, output.String(), text)
	}
}

type exportErrorWriter struct {
	err error
}

func (w exportErrorWriter) Write([]byte) (int, error) {
	return 0, w.err
}

func TestExportCommand_ProgressWriteFailureStopsBeforeFetching(t *testing.T) {
	f := newFetchFixture(t, false)
	directory := filepath.Join(t.TempDir(), "export")
	cmd := NewExportCmd(func() (client.Interface, error) { return f.client, nil })
	require.NoError(t, cmd.Flags().Set("output", directory))
	wantErr := errors.New("output unavailable")
	cmd.SetOut(exportErrorWriter{err: wantErr})
	require.ErrorIs(t, cmd.RunE(cmd, nil), wantErr)
	assert.Empty(t, f.calls)
	assert.Zero(t, f.profileCalls)
	assert.NoDirExists(t, directory)
}
