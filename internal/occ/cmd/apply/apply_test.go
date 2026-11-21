// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package apply

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/iotest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openchoreo/openchoreo/internal/occ/cmd/config"
	"github.com/openchoreo/openchoreo/internal/occ/resources/client"
	"github.com/openchoreo/openchoreo/internal/occ/testutil"
)

const (
	keyKind       = "kind"
	keyAPIVersion = "apiVersion"
	keyMetadata   = "metadata"
	keyName       = "name"
	keyNamespace  = "namespace"
	kindProject   = "Project"
	testMyNs      = "my-ns"
)

// --- Params ---

func TestParams_GetFilePath(t *testing.T) {
	p := Params{FilePath: "/tmp/test.yaml"}
	assert.Equal(t, "/tmp/test.yaml", p.GetFilePath())
}

func TestParams_GetDryRun(t *testing.T) {
	p := Params{FilePath: "/tmp/dryrun.yaml", DryRun: true}
	assert.True(t, p.GetDryRun())
	pFalse := Params{FilePath: "/tmp/dryrun.yaml", DryRun: false}
	assert.False(t, pFalse.GetDryRun())
}

func TestExtractResourceInfo(t *testing.T) {
	tests := []struct {
		name     string
		resource map[string]any
		wantInfo resourceInfo
		wantErr  bool
		errMsg   string
	}{
		{
			name: "valid resource",
			resource: map[string]any{
				keyKind:       kindProject,
				keyAPIVersion: "core.openchoreo.dev/v1alpha1",
				keyMetadata: map[string]any{
					keyName:      "my-project",
					keyNamespace: testMyNs,
				},
			},
			wantInfo: resourceInfo{kind: kindProject, apiVersion: "core.openchoreo.dev/v1alpha1", name: "my-project", namespace: testMyNs},
		},
		{
			name:     "missing kind",
			resource: map[string]any{keyMetadata: map[string]any{keyName: "x"}},
			wantErr:  true,
			errMsg:   "missing 'kind'",
		},
		{
			name:     "missing metadata.name",
			resource: map[string]any{keyKind: kindProject, keyMetadata: map[string]any{}},
			wantErr:  true,
			errMsg:   "missing 'metadata.name'",
		},
		{
			name: "no namespace is ok",
			resource: map[string]any{
				keyKind:     "Namespace",
				keyMetadata: map[string]any{keyName: testMyNs},
			},
			wantInfo: resourceInfo{kind: "Namespace", name: testMyNs},
		},
		{
			name: "no apiVersion is ok",
			resource: map[string]any{
				keyKind:     kindProject,
				keyMetadata: map[string]any{keyName: "p"},
			},
			wantInfo: resourceInfo{kind: kindProject, name: "p"},
		},
		{
			name:     "empty map",
			resource: map[string]any{},
			wantErr:  true,
			errMsg:   "missing 'kind'",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info, err := extractResourceInfo(tt.resource)
			if tt.wantErr {
				require.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantInfo, info)
		})
	}
}

func TestStripKindAndAPIVersion(t *testing.T) {
	tests := []struct {
		name     string
		resource map[string]any
		wantKeys []string
	}{
		{
			name:     "removes kind and apiVersion",
			resource: map[string]any{keyKind: kindProject, keyAPIVersion: "v1", keyMetadata: map[string]any{keyName: "x"}},
			wantKeys: []string{keyKind, keyAPIVersion},
		},
		{
			name:     "empty map",
			resource: map[string]any{},
			wantKeys: []string{},
		},
		{
			name:     "fields already absent",
			resource: map[string]any{keyMetadata: map[string]any{keyName: "x"}},
			wantKeys: []string{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			jsonBytes, err := stripKindAndAPIVersion(tt.resource)
			require.NoError(t, err)
			result := string(jsonBytes)
			for _, key := range tt.wantKeys {
				assert.NotContains(t, result, `"`+key+`"`)
			}
		})
	}
}

func TestParseYAMLResources(t *testing.T) {
	tests := []struct {
		name      string
		content   string
		wantCount int
		wantErr   bool
		errMsg    string
	}{
		{
			name:      "single document",
			content:   "kind: Project\nmetadata:\n  name: p1\n",
			wantCount: 1,
		},
		{
			name:      "multi-document",
			content:   "kind: Project\nmetadata:\n  name: p1\n---\nkind: Component\nmetadata:\n  name: c1\n",
			wantCount: 2,
		},
		{
			name:      "empty doc skipped",
			content:   "kind: Project\nmetadata:\n  name: p1\n---\n---\nkind: Component\nmetadata:\n  name: c1\n",
			wantCount: 2,
		},
		{
			name:      "doc without kind skipped",
			content:   "metadata:\n  name: p1\n---\nkind: Project\nmetadata:\n  name: p2\n",
			wantCount: 1,
		},
		{
			name:    "invalid YAML",
			content: ":\n  invalid: [yaml\n",
			wantErr: true,
			errMsg:  "failed to parse YAML",
		},
		{
			name:      "empty input",
			content:   "",
			wantCount: 0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resources, err := parseYAMLResources([]byte(tt.content))
			if tt.wantErr {
				require.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
				return
			}
			require.NoError(t, err)
			assert.Len(t, resources, tt.wantCount)
		})
	}
}

func TestParseErrorBody(t *testing.T) {
	tests := []struct {
		name string
		body []byte
		want string
	}{
		{
			name: "valid error response JSON",
			body: []byte(`{"code":"INVALID_REQUEST","error":"field X is required"}`),
			want: "field X is required",
		},
		{
			name: "empty body",
			body: []byte{},
			want: "unknown error (empty response)",
		},
		{
			name: "non-JSON body",
			body: []byte("Internal Server Error"),
			want: "Internal Server Error",
		},
		{
			name: "long body gets truncated",
			body: []byte(strings.Repeat("x", 300)),
			want: strings.Repeat("x", 200) + "...",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, parseErrorBody(tt.body))
		})
	}
}

func TestDiscoverResourceFiles(t *testing.T) {
	t.Run("single file", func(t *testing.T) {
		dir := t.TempDir()
		f := filepath.Join(dir, "resource.yaml")
		require.NoError(t, os.WriteFile(f, []byte("kind: Project"), 0600))

		files, err := discoverResourceFiles(f)
		require.NoError(t, err)
		assert.Equal(t, []string{f}, files)
	})

	t.Run("directory with mixed files", func(t *testing.T) {
		dir := t.TempDir()
		for _, f := range []struct{ name, content string }{
			{"a.yaml", "kind: A"}, {"b.yml", "kind: B"}, {"c.txt", "not yaml"},
		} {
			require.NoError(t, os.WriteFile(filepath.Join(dir, f.name), []byte(f.content), 0600))
		}

		files, err := discoverResourceFiles(dir)
		require.NoError(t, err)
		assert.Len(t, files, 2)
	})

	t.Run("http URL passthrough", func(t *testing.T) {
		files, err := discoverResourceFiles("https://example.com/resource.yaml")
		require.NoError(t, err)
		assert.Equal(t, []string{"https://example.com/resource.yaml"}, files)
	})

	t.Run("stdin dash", func(t *testing.T) {
		files, err := discoverResourceFiles("-")
		require.NoError(t, err)
		assert.Equal(t, []string{"-"}, files)
	})

	t.Run("nonexistent path", func(t *testing.T) {
		dir := t.TempDir()
		_, err := discoverResourceFiles(filepath.Join(dir, "no-such-subdir"))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "does not exist")
	})

	t.Run("empty directory", func(t *testing.T) {
		dir := t.TempDir()
		files, err := discoverResourceFiles(dir)
		require.NoError(t, err)
		assert.Empty(t, files)
	})
}

func TestReadResourceContent(t *testing.T) {
	ctx := context.Background()

	t.Run("read local file", func(t *testing.T) {
		dir := t.TempDir()
		f := filepath.Join(dir, "resource.yaml")
		want := "kind: Project\nmetadata:\n  name: test\n"
		require.NoError(t, os.WriteFile(f, []byte(want), 0600))

		got, err := readResourceContent(ctx, f)
		require.NoError(t, err)
		assert.Equal(t, want, string(got))
	})

	t.Run("file not found", func(t *testing.T) {
		dir := t.TempDir()
		_, err := readResourceContent(ctx, filepath.Join(dir, "missing.yaml"))
		require.Error(t, err)
	})

	t.Run("read from HTTP URL", func(t *testing.T) {
		want := "kind: Component\nmetadata:\n  name: web\n"
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprint(w, want)
		}))
		t.Cleanup(srv.Close)

		got, err := readResourceContent(ctx, srv.URL+"/resource.yaml")
		require.NoError(t, err)
		assert.Equal(t, want, string(got))
	})

	t.Run("HTTP URL returns error status", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))
		t.Cleanup(srv.Close)

		_, err := readResourceContent(ctx, srv.URL+"/missing.yaml")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "HTTP 404")
	})

	t.Run("read from stdin", func(t *testing.T) {
		origStdin := stdinReader
		defer func() { stdinReader = origStdin }()

		want := "kind: Namespace\nmetadata:\n  name: from-stdin\n"
		stdinReader = strings.NewReader(want)

		got, err := readResourceContent(ctx, "-")
		require.NoError(t, err)
		assert.Equal(t, want, string(got))
	})

	t.Run("read from empty stdin", func(t *testing.T) {
		origStdin := stdinReader
		defer func() { stdinReader = origStdin }()

		stdinReader = strings.NewReader("   \n\t  ")

		_, err := readResourceContent(ctx, "-")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "standard input is empty")
	})

	t.Run("read from stdin read error", func(t *testing.T) {
		origStdin := stdinReader
		defer func() { stdinReader = origStdin }()

		stdinReader = iotest.ErrReader(errors.New("simulated read error"))

		_, err := readResourceContent(ctx, "-")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "error reading from standard input")
		assert.Contains(t, err.Error(), "simulated read error")
	})
}

// writeTestOCConfig configures a test home with standard test OC config.
func writeTestOCConfig(t *testing.T, home string) {
	t.Helper()
	const baseURL = "http://mock-api"
	testutil.WriteOCConfig(t, home, config.StoredConfig{
		CurrentContext: "test",
		ControlPlanes:  []config.ControlPlane{{Name: "cp", URL: baseURL}},
		Credentials:    []config.Credential{{Name: "cred", Token: testutil.NonExpiredJWT}},
		Contexts:       []config.Context{{Name: "test", ControlPlane: "cp", Credentials: "cred"}},
	})
}

// setupApplyTest configures a test home with OC config and mocks HTTP transport.
// It returns a *client.Client ready for use.
func setupApplyTest(t *testing.T, handler testutil.RoundTripFunc) *client.Client {
	t.Helper()
	home := testutil.SetupTestHome(t)
	writeTestOCConfig(t, home)
	testutil.SetTransport(t, handler)

	cl, err := client.NewClient()
	require.NoError(t, err)
	return cl
}

func TestApply_MultipleResources(t *testing.T) {
	cl := setupApplyTest(t, testutil.RoundTripFunc(func(r *http.Request) (*http.Response, error) {
		// All GETs return 404 (not found), all POSTs return 201 (created)
		if r.Method == http.MethodGet {
			return &http.Response{
				StatusCode: http.StatusNotFound,
				Body:       io.NopCloser(bytes.NewReader([]byte(`{"error":"not found"}`))),
				Header:     http.Header{},
			}, nil
		}
		if r.Method == http.MethodPost {
			return testutil.JSONResp(http.StatusCreated, map[string]any{}), nil
		}
		return &http.Response{StatusCode: http.StatusNotFound, Body: http.NoBody, Header: http.Header{}}, nil
	}))

	dir := t.TempDir()
	yamlFile := filepath.Join(dir, "multi.yaml")
	require.NoError(t, os.WriteFile(yamlFile, []byte(`kind: Namespace
metadata:
  name: ns1
---
kind: Namespace
metadata:
  name: ns2
`), 0600))

	out := testutil.CaptureStdout(t, func() {
		err := Apply(cl, Params{FilePath: yamlFile})
		require.NoError(t, err)
	})
	assert.Contains(t, out, "namespace/ns1 created")
	assert.Contains(t, out, "namespace/ns2 created")
	assert.Contains(t, out, "Applied 2 resource(s) from 1 file(s)")
}

func TestApply_EmptyFilePath(t *testing.T) {
	cl := setupApplyTest(t, testutil.RoundTripFunc(func(_ *http.Request) (*http.Response, error) {
		t.Fatal("no HTTP call expected")
		return nil, nil
	}))

	err := Apply(cl, Params{FilePath: ""})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "file path is required")
}

func TestApply_UnsupportedKind(t *testing.T) {
	cl := setupApplyTest(t, testutil.RoundTripFunc(func(_ *http.Request) (*http.Response, error) {
		t.Fatal("no HTTP call expected")
		return nil, nil
	}))

	dir := t.TempDir()
	yamlFile := filepath.Join(dir, "bad.yaml")
	require.NoError(t, os.WriteFile(yamlFile, []byte(`kind: FakeResource
metadata:
  name: fake
`), 0600))

	out := testutil.CaptureStdout(t, func() {
		err := Apply(cl, Params{FilePath: yamlFile})
		require.Error(t, err)
	})
	assert.Contains(t, out, "unsupported kind")
}

func TestApply_ReadOnlyKind(t *testing.T) {
	cl := setupApplyTest(t, testutil.RoundTripFunc(func(_ *http.Request) (*http.Response, error) {
		t.Fatal("no HTTP call expected")
		return nil, nil
	}))

	dir := t.TempDir()
	yamlFile := filepath.Join(dir, "ro.yaml")
	require.NoError(t, os.WriteFile(yamlFile, []byte(`kind: RenderedRelease
metadata:
  name: rr1
  namespace: test
`), 0600))

	out := testutil.CaptureStdout(t, func() {
		err := Apply(cl, Params{FilePath: yamlFile})
		require.Error(t, err)
	})
	assert.Contains(t, out, "read-only resource")
}

func TestApply_CreateFails(t *testing.T) {
	cl := setupApplyTest(t, testutil.RoundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.Method == http.MethodGet {
			return &http.Response{
				StatusCode: http.StatusNotFound,
				Body:       io.NopCloser(bytes.NewReader([]byte(`{}`))),
				Header:     http.Header{},
			}, nil
		}
		if r.Method == http.MethodPost {
			return testutil.JSONResp(http.StatusBadRequest, map[string]any{
				"code": "INVALID_REQUEST", "error": "name is required",
			}), nil
		}
		return &http.Response{StatusCode: http.StatusNotFound, Body: http.NoBody, Header: http.Header{}}, nil
	}))

	dir := t.TempDir()
	yamlFile := filepath.Join(dir, "fail.yaml")
	require.NoError(t, os.WriteFile(yamlFile, []byte(`kind: Namespace
metadata:
  name: bad-ns
`), 0600))

	out := testutil.CaptureStdout(t, func() {
		err := Apply(cl, Params{FilePath: yamlFile})
		require.Error(t, err)
	})
	assert.Contains(t, out, "name is required")
}

func TestApply_Directory(t *testing.T) {
	cl := setupApplyTest(t, testutil.RoundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.Method == http.MethodGet {
			return &http.Response{
				StatusCode: http.StatusNotFound,
				Body:       io.NopCloser(bytes.NewReader([]byte(`{}`))),
				Header:     http.Header{},
			}, nil
		}
		return testutil.JSONResp(http.StatusCreated, map[string]any{}), nil
	}))

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.yaml"), []byte("kind: Namespace\nmetadata:\n  name: ns-a\n"), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "b.yml"), []byte("kind: Namespace\nmetadata:\n  name: ns-b\n"), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "c.txt"), []byte("not yaml"), 0600))

	out := testutil.CaptureStdout(t, func() {
		err := Apply(cl, Params{FilePath: dir})
		require.NoError(t, err)
	})
	assert.Contains(t, out, "namespace/ns-a created")
	assert.Contains(t, out, "namespace/ns-b created")
	assert.Contains(t, out, "Applied 2 resource(s) from 2 file(s)")
}

func TestApply_NamespacedResourceMissingNamespace(t *testing.T) {
	cl := setupApplyTest(t, testutil.RoundTripFunc(func(_ *http.Request) (*http.Response, error) {
		t.Fatal("no HTTP call expected")
		return nil, nil
	}))

	dir := t.TempDir()
	yamlFile := filepath.Join(dir, "project.yaml")
	require.NoError(t, os.WriteFile(yamlFile, []byte(`kind: Project
metadata:
  name: my-project
`), 0600))

	out := testutil.CaptureStdout(t, func() {
		err := Apply(cl, Params{FilePath: yamlFile})
		require.Error(t, err)
	})
	assert.Contains(t, out, "namespace is required")
}

func TestApply_ReleaseLineRoutedToOwnerComponent(t *testing.T) {
	const linePath = "/api/v1/namespaces/default/components/hello/releaselines"
	var calls []string
	cl := setupApplyTest(t, testutil.RoundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls = append(calls, r.Method+" "+r.URL.Path)
		switch {
		case r.Method == http.MethodGet && r.URL.Path == linePath+"/main":
			return testutil.JSONResp(http.StatusOK, map[string]any{}), nil
		case r.Method == http.MethodGet:
			return testutil.JSONResp(http.StatusNotFound, map[string]any{"error": "not found"}), nil
		case r.Method == http.MethodPut:
			return testutil.JSONResp(http.StatusOK, map[string]any{}), nil
		default:
			return testutil.JSONResp(http.StatusCreated, map[string]any{}), nil
		}
	}))

	dir := t.TempDir()
	yamlFile := filepath.Join(dir, "lines.yaml")
	require.NoError(t, os.WriteFile(yamlFile, []byte(`apiVersion: openchoreo.dev/v1alpha1
kind: ReleaseLine
metadata:
  name: main
  namespace: default
spec:
  owner:
    projectName: default
    componentName: hello
---
apiVersion: openchoreo.dev/v1alpha1
kind: ReleaseLine
metadata:
  name: next
  namespace: default
spec:
  owner:
    projectName: default
    componentName: hello
`), 0600))

	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, Apply(cl, Params{FilePath: yamlFile}))
	})
	assert.Contains(t, out, "releaseline/main configured")
	assert.Contains(t, out, "releaseline/next created")
	assert.Equal(t, []string{
		"GET " + linePath + "/main",
		"PUT " + linePath + "/main",
		"GET " + linePath + "/next",
		"POST " + linePath,
	}, calls)
}

func TestApply_ReleaseLineWithoutOwnerComponent(t *testing.T) {
	cl := setupApplyTest(t, testutil.RoundTripFunc(func(_ *http.Request) (*http.Response, error) {
		t.Fatal("no request expected")
		return nil, nil
	}))

	dir := t.TempDir()
	yamlFile := filepath.Join(dir, "line.yaml")
	require.NoError(t, os.WriteFile(yamlFile, []byte(`kind: ReleaseLine
metadata:
  name: next
  namespace: default
spec: {}
`), 0600))

	out := testutil.CaptureStdout(t, func() {
		require.Error(t, Apply(cl, Params{FilePath: yamlFile}))
	})
	assert.Contains(t, out, "releaseline/next: spec.owner.componentName is required")
}

func TestApply_Stdin(t *testing.T) {
	origStdin := stdinReader
	defer func() { stdinReader = origStdin }()

	stdinContent := `kind: Namespace
metadata:
  name: stdin-ns
`
	stdinReader = strings.NewReader(stdinContent)

	cl := setupApplyTest(t, testutil.RoundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/namespaces/stdin-ns") {
			return &http.Response{
				StatusCode: http.StatusNotFound,
				Body:       io.NopCloser(bytes.NewReader([]byte(`{"error":"not found"}`))),
				Header:     http.Header{},
			}, nil
		}
		if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/namespaces") {
			return testutil.JSONResp(http.StatusCreated, map[string]any{
				keyMetadata: map[string]any{keyName: "stdin-ns"},
			}), nil
		}
		return &http.Response{StatusCode: http.StatusNotFound, Body: http.NoBody}, nil
	}))

	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, Apply(cl, Params{FilePath: "-"}))
	})
	assert.Contains(t, out, "namespace/stdin-ns created")
	assert.Contains(t, out, "Applied 1 resource(s) from 1 file(s)")
}

func TestApply_DryRun_Create(t *testing.T) {
	postCalled := false
	cl := setupApplyTest(t, testutil.RoundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/namespaces/dry-ns") {
			return &http.Response{
				StatusCode: http.StatusNotFound,
				Body:       io.NopCloser(bytes.NewReader([]byte(`{"error":"not found"}`))),
				Header:     http.Header{},
			}, nil
		}
		if r.Method == http.MethodPost {
			postCalled = true
			return testutil.JSONResp(http.StatusCreated, map[string]any{}), nil
		}
		return &http.Response{StatusCode: http.StatusNotFound, Body: http.NoBody}, nil
	}))

	dir := t.TempDir()
	yamlFile := filepath.Join(dir, "ns.yaml")
	require.NoError(t, os.WriteFile(yamlFile, []byte(`kind: Namespace
metadata:
  name: dry-ns
`), 0600))

	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, Apply(cl, Params{FilePath: yamlFile, DryRun: true}))
	})
	assert.Contains(t, out, "namespace/dry-ns created (dry run)")
	assert.Contains(t, out, "Dry run: 1 resource(s) would be applied from 1 file(s)")
	assert.False(t, postCalled, "POST should not be called in dry-run mode")
}

func TestApply_DryRun_Update(t *testing.T) {
	putCalled := false
	cl := setupApplyTest(t, testutil.RoundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/namespaces/existing-ns") {
			return testutil.JSONResp(http.StatusOK, map[string]any{
				keyMetadata: map[string]any{keyName: "existing-ns"},
			}), nil
		}
		if r.Method == http.MethodPut {
			putCalled = true
			return testutil.JSONResp(http.StatusOK, map[string]any{}), nil
		}
		return &http.Response{StatusCode: http.StatusNotFound, Body: http.NoBody}, nil
	}))

	dir := t.TempDir()
	yamlFile := filepath.Join(dir, "ns.yaml")
	require.NoError(t, os.WriteFile(yamlFile, []byte(`kind: Namespace
metadata:
  name: existing-ns
`), 0600))

	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, Apply(cl, Params{FilePath: yamlFile, DryRun: true}))
	})
	assert.Contains(t, out, "namespace/existing-ns configured (dry run)")
	assert.Contains(t, out, "Dry run: 1 resource(s) would be applied from 1 file(s)")
	assert.False(t, putCalled, "PUT should not be called in dry-run mode")
}

func TestApply_DryRun_CreateOnly_Error(t *testing.T) {
	cl := setupApplyTest(t, testutil.RoundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.Method == http.MethodGet {
			return testutil.JSONResp(http.StatusOK, map[string]any{
				keyMetadata: map[string]any{keyName: "p1"},
			}), nil
		}
		return &http.Response{StatusCode: http.StatusNotFound, Body: http.NoBody}, nil
	}))

	dir := t.TempDir()
	yamlFile := filepath.Join(dir, "project_release.yaml")
	require.NoError(t, os.WriteFile(yamlFile, []byte(`kind: ProjectRelease
metadata:
  name: p1
  namespace: test-ns
`), 0600))

	out := testutil.CaptureStdout(t, func() {
		err := Apply(cl, Params{FilePath: yamlFile, DryRun: true})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "dry run completed with 1 error(s)")
	})
	assert.Contains(t, out, "resource already exists and cannot be updated (create-only resource)")
}

func TestApply_CreateOnly_Error(t *testing.T) {
	cl := setupApplyTest(t, testutil.RoundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.Method == http.MethodGet {
			return testutil.JSONResp(http.StatusOK, map[string]any{
				keyMetadata: map[string]any{keyName: "p1"},
			}), nil
		}
		return &http.Response{StatusCode: http.StatusNotFound, Body: http.NoBody}, nil
	}))

	dir := t.TempDir()
	yamlFile := filepath.Join(dir, "project_release.yaml")
	require.NoError(t, os.WriteFile(yamlFile, []byte(`kind: ProjectRelease
metadata:
  name: p1
  namespace: test-ns
`), 0600))

	out := testutil.CaptureStdout(t, func() {
		err := Apply(cl, Params{FilePath: yamlFile})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "apply completed with 1 error(s)")
	})
	assert.Contains(t, out, "resource already exists and cannot be updated (create-only resource)")
}

func TestApply_Stdin_DryRun(t *testing.T) {
	origStdin := stdinReader
	defer func() { stdinReader = origStdin }()

	stdinContent := `kind: Namespace
metadata:
  name: stdin-dry-ns
`
	stdinReader = strings.NewReader(stdinContent)

	postCalled := false
	cl := setupApplyTest(t, testutil.RoundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/namespaces/stdin-dry-ns") {
			return &http.Response{
				StatusCode: http.StatusNotFound,
				Body:       io.NopCloser(bytes.NewReader([]byte(`{"error":"not found"}`))),
				Header:     http.Header{},
			}, nil
		}
		if r.Method == http.MethodPost {
			postCalled = true
			return testutil.JSONResp(http.StatusCreated, map[string]any{}), nil
		}
		return &http.Response{StatusCode: http.StatusNotFound, Body: http.NoBody}, nil
	}))

	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, Apply(cl, Params{FilePath: "-", DryRun: true}))
	})
	assert.Contains(t, out, "namespace/stdin-dry-ns created (dry run)")
	assert.Contains(t, out, "Dry run: 1 resource(s) would be applied from 1 file(s)")
	assert.False(t, postCalled, "POST should not be called in dry-run mode")
}
