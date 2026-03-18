// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package apply

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
				"kind":       "Project",
				"apiVersion": "core.openchoreo.dev/v1alpha1",
				"metadata": map[string]any{
					"name":      "my-project",
					"namespace": "my-ns",
				},
			},
			wantInfo: resourceInfo{kind: "Project", apiVersion: "core.openchoreo.dev/v1alpha1", name: "my-project", namespace: "my-ns"},
		},
		{
			name:     "missing kind",
			resource: map[string]any{"metadata": map[string]any{"name": "x"}},
			wantErr:  true,
			errMsg:   "missing 'kind'",
		},
		{
			name:     "missing metadata.name",
			resource: map[string]any{"kind": "Project", "metadata": map[string]any{}},
			wantErr:  true,
			errMsg:   "missing 'metadata.name'",
		},
		{
			name: "no namespace is ok",
			resource: map[string]any{
				"kind":     "Namespace",
				"metadata": map[string]any{"name": "my-ns"},
			},
			wantInfo: resourceInfo{kind: "Namespace", name: "my-ns"},
		},
		{
			name: "no apiVersion is ok",
			resource: map[string]any{
				"kind":     "Project",
				"metadata": map[string]any{"name": "p"},
			},
			wantInfo: resourceInfo{kind: "Project", name: "p"},
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
			resource: map[string]any{"kind": "Project", "apiVersion": "v1", "metadata": map[string]any{"name": "x"}},
			wantKeys: []string{"kind", "apiVersion"},
		},
		{
			name:     "empty map",
			resource: map[string]any{},
			wantKeys: []string{},
		},
		{
			name:     "fields already absent",
			resource: map[string]any{"metadata": map[string]any{"name": "x"}},
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
}
