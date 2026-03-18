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
			if (err != nil) != tt.wantErr {
				t.Errorf("extractResourceInfo() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err != nil && tt.errMsg != "" && !strings.Contains(err.Error(), tt.errMsg) {
				t.Errorf("error %q does not contain %q", err.Error(), tt.errMsg)
			}
			if err == nil && info != tt.wantInfo {
				t.Errorf("extractResourceInfo() = %+v, want %+v", info, tt.wantInfo)
			}
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
			if err != nil {
				t.Fatalf("stripKindAndAPIVersion() error = %v", err)
			}
			result := string(jsonBytes)
			for _, key := range tt.wantKeys {
				if strings.Contains(result, `"`+key+`"`) {
					t.Errorf("result JSON should not contain key %q, got %s", key, result)
				}
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
			if (err != nil) != tt.wantErr {
				t.Errorf("parseYAMLResources() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err != nil && tt.errMsg != "" && !strings.Contains(err.Error(), tt.errMsg) {
				t.Errorf("error %q does not contain %q", err.Error(), tt.errMsg)
			}
			if err == nil && len(resources) != tt.wantCount {
				t.Errorf("parseYAMLResources() returned %d resources, want %d", len(resources), tt.wantCount)
			}
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
			if got := parseErrorBody(tt.body); got != tt.want {
				t.Errorf("parseErrorBody() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDiscoverResourceFiles(t *testing.T) {
	t.Run("single file", func(t *testing.T) {
		dir := t.TempDir()
		f := filepath.Join(dir, "resource.yaml")
		if err := os.WriteFile(f, []byte("kind: Project"), 0600); err != nil {
			t.Fatal(err)
		}

		files, err := discoverResourceFiles(f)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(files) != 1 || files[0] != f {
			t.Errorf("got %v, want [%s]", files, f)
		}
	})

	t.Run("directory with mixed files", func(t *testing.T) {
		dir := t.TempDir()
		for _, f := range []struct{ name, content string }{
			{"a.yaml", "kind: A"}, {"b.yml", "kind: B"}, {"c.txt", "not yaml"},
		} {
			if err := os.WriteFile(filepath.Join(dir, f.name), []byte(f.content), 0600); err != nil {
				t.Fatal(err)
			}
		}

		files, err := discoverResourceFiles(dir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(files) != 2 {
			t.Errorf("got %d files, want 2", len(files))
		}
	})

	t.Run("http URL passthrough", func(t *testing.T) {
		files, err := discoverResourceFiles("https://example.com/resource.yaml")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(files) != 1 || files[0] != "https://example.com/resource.yaml" {
			t.Errorf("got %v, want [https://example.com/resource.yaml]", files)
		}
	})

	t.Run("nonexistent path", func(t *testing.T) {
		_, err := discoverResourceFiles("/nonexistent/path")
		if err == nil {
			t.Fatal("expected error for nonexistent path")
		}
		if !strings.Contains(err.Error(), "does not exist") {
			t.Errorf("error %q should mention 'does not exist'", err.Error())
		}
	})

	t.Run("empty directory", func(t *testing.T) {
		dir := t.TempDir()
		files, err := discoverResourceFiles(dir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(files) != 0 {
			t.Errorf("got %d files, want 0", len(files))
		}
	})
}

func TestReadResourceContent(t *testing.T) {
	ctx := context.Background()

	t.Run("read local file", func(t *testing.T) {
		dir := t.TempDir()
		f := filepath.Join(dir, "resource.yaml")
		want := "kind: Project\nmetadata:\n  name: test\n"
		if err := os.WriteFile(f, []byte(want), 0600); err != nil {
			t.Fatal(err)
		}

		got, err := readResourceContent(ctx, f)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if string(got) != want {
			t.Errorf("readResourceContent() = %q, want %q", string(got), want)
		}
	})

	t.Run("file not found", func(t *testing.T) {
		_, err := readResourceContent(ctx, "/nonexistent/file.yaml")
		if err == nil {
			t.Fatal("expected error for missing file")
		}
	})

	t.Run("permission denied", func(t *testing.T) {
		dir := t.TempDir()
		f := filepath.Join(dir, "noperm.yaml")
		if err := os.WriteFile(f, []byte("data"), 0600); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(f, 0000); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Chmod(f, 0600) })

		_, err := readResourceContent(ctx, f)
		if err == nil {
			t.Fatal("expected error for unreadable file")
		}
		if !strings.Contains(err.Error(), "permission denied") {
			t.Errorf("error %q should mention 'permission denied'", err.Error())
		}
	})

	t.Run("read from HTTP URL", func(t *testing.T) {
		want := "kind: Component\nmetadata:\n  name: web\n"
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprint(w, want)
		}))
		t.Cleanup(srv.Close)

		got, err := readResourceContent(ctx, srv.URL+"/resource.yaml")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if string(got) != want {
			t.Errorf("readResourceContent() = %q, want %q", string(got), want)
		}
	})

	t.Run("HTTP URL returns error status", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))
		t.Cleanup(srv.Close)

		_, err := readResourceContent(ctx, srv.URL+"/missing.yaml")
		if err == nil {
			t.Fatal("expected error for HTTP 404")
		}
		if !strings.Contains(err.Error(), "HTTP 404") {
			t.Errorf("error %q should mention 'HTTP 404'", err.Error())
		}
	})
}
