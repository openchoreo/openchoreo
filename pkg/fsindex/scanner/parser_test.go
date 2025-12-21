// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package scanner

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/openchoreo/openchoreo/pkg/fsindex/index"
)

func TestParseYAML(t *testing.T) {
	tests := []struct {
		name      string
		yaml      string
		wantCount int
		wantKinds []string
		wantNames []string
		wantErr   bool
	}{
		{
			name: "single document",
			yaml: `
apiVersion: openchoreo.dev/v1alpha1
kind: Component
metadata:
  name: test-component
  namespace: default
`,
			wantCount: 1,
			wantKinds: []string{"Component"},
			wantNames: []string{"test-component"},
		},
		{
			name: "multi-document YAML",
			yaml: `
apiVersion: openchoreo.dev/v1alpha1
kind: Component
metadata:
  name: component-1
---
apiVersion: openchoreo.dev/v1alpha1
kind: Workload
metadata:
  name: workload-1
---
apiVersion: openchoreo.dev/v1alpha1
kind: Trait
metadata:
  name: trait-1
`,
			wantCount: 3,
			wantKinds: []string{"Component", "Workload", "Trait"},
			wantNames: []string{"component-1", "workload-1", "trait-1"},
		},
		{
			name:      "empty document",
			yaml:      "",
			wantCount: 0,
		},
		{
			name: "document without apiVersion",
			yaml: `
kind: Component
metadata:
  name: invalid
`,
			wantCount: 0,
		},
		{
			name: "document without kind",
			yaml: `
apiVersion: openchoreo.dev/v1alpha1
metadata:
  name: invalid
`,
			wantCount: 0,
		},
		{
			name: "mixed valid and invalid documents",
			yaml: `
apiVersion: openchoreo.dev/v1alpha1
kind: Component
metadata:
  name: valid-component
---
kind: Invalid
metadata:
  name: no-api-version
---
apiVersion: openchoreo.dev/v1alpha1
kind: Trait
metadata:
  name: valid-trait
`,
			wantCount: 2,
			wantKinds: []string{"Component", "Trait"},
			wantNames: []string{"valid-component", "valid-trait"},
		},
		{
			name: "document with empty object",
			yaml: `
---
---
apiVersion: openchoreo.dev/v1alpha1
kind: Component
metadata:
  name: after-empty
`,
			wantCount: 1,
			wantKinds: []string{"Component"},
			wantNames: []string{"after-empty"},
		},
		{
			name:      "JSON format",
			yaml:      `{"apiVersion":"openchoreo.dev/v1alpha1","kind":"Component","metadata":{"name":"json-component"}}`,
			wantCount: 1,
			wantKinds: []string{"Component"},
			wantNames: []string{"json-component"},
		},
		{
			name: "cluster-scoped resource (no namespace)",
			yaml: `
apiVersion: openchoreo.dev/v1alpha1
kind: ComponentType
metadata:
  name: http-service
`,
			wantCount: 1,
			wantKinds: []string{"ComponentType"},
			wantNames: []string{"http-service"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entries, err := ParseYAML([]byte(tt.yaml), "/test/source.yaml")

			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if len(entries) != tt.wantCount {
				t.Errorf("got %d entries, want %d", len(entries), tt.wantCount)
			}

			for i, entry := range entries {
				if i >= len(tt.wantKinds) {
					break
				}

				gotKind := entry.Resource.GetKind()
				if gotKind != tt.wantKinds[i] {
					t.Errorf("entry[%d] kind = %q, want %q", i, gotKind, tt.wantKinds[i])
				}

				gotName := entry.Name()
				if gotName != tt.wantNames[i] {
					t.Errorf("entry[%d] name = %q, want %q", i, gotName, tt.wantNames[i])
				}

				// Verify source path is set
				if entry.FilePath != "/test/source.yaml" {
					t.Errorf("entry[%d] FilePath = %q, want %q", i, entry.FilePath, "/test/source.yaml")
				}
			}
		})
	}
}

func TestParseYAMLFile(t *testing.T) {
	// Create a temporary test file
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.yaml")

	content := `
apiVersion: openchoreo.dev/v1alpha1
kind: Component
metadata:
  name: file-test-component
  namespace: test-ns
spec:
  componentType: http-service
`
	if err := os.WriteFile(testFile, []byte(content), 0600); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	entries, err := ParseYAMLFile(testFile)
	if err != nil {
		t.Fatalf("ParseYAMLFile() error: %v", err)
	}

	if len(entries) != 1 {
		t.Fatalf("got %d entries, want 1", len(entries))
	}

	entry := entries[0]
	if entry.Name() != "file-test-component" {
		t.Errorf("name = %q, want %q", entry.Name(), "file-test-component")
	}

	if entry.Namespace() != "test-ns" {
		t.Errorf("namespace = %q, want %q", entry.Namespace(), "test-ns")
	}

	if entry.FilePath != testFile {
		t.Errorf("FilePath = %q, want %q", entry.FilePath, testFile)
	}
}

func TestParseYAMLFileNotFound(t *testing.T) {
	_, err := ParseYAMLFile("/nonexistent/file.yaml")
	if err == nil {
		t.Error("expected error for non-existent file")
	}
}

func TestParseYAMLMultiDocumentFile(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "multi.yaml")

	content := `
apiVersion: openchoreo.dev/v1alpha1
kind: Component
metadata:
  name: comp1
---
apiVersion: openchoreo.dev/v1alpha1
kind: Component
metadata:
  name: comp2
---
apiVersion: openchoreo.dev/v1alpha1
kind: Workload
metadata:
  name: workload1
`
	if err := os.WriteFile(testFile, []byte(content), 0600); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	entries, err := ParseYAMLFile(testFile)
	if err != nil {
		t.Fatalf("ParseYAMLFile() error: %v", err)
	}

	if len(entries) != 3 {
		t.Errorf("got %d entries, want 3", len(entries))
	}
}

func TestValidateResource(t *testing.T) {
	tests := []struct {
		name    string
		yaml    string
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid resource",
			yaml: `
apiVersion: openchoreo.dev/v1alpha1
kind: Component
metadata:
  name: valid-resource
`,
			wantErr: false,
		},
		{
			name: "missing name",
			yaml: `
apiVersion: openchoreo.dev/v1alpha1
kind: Component
metadata:
  namespace: default
`,
			wantErr: true,
			errMsg:  "no name",
		},
		{
			name: "missing apiVersion",
			yaml: `
kind: Component
metadata:
  name: test
`,
			wantErr: true,
		},
		{
			name: "missing kind",
			yaml: `
apiVersion: openchoreo.dev/v1alpha1
metadata:
  name: test
`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entries, _ := ParseYAML([]byte(tt.yaml), "/test.yaml")
			if len(entries) == 0 {
				// Parser already filtered it out - consider this validation passed
				if !tt.wantErr {
					t.Error("expected valid resource to be parsed")
				}
				return
			}

			err := ValidateResource(entries[0])
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			}
		})
	}
}

func TestValidateResourceNil(t *testing.T) {
	// Test nil entry
	if err := ValidateResource(nil); err == nil {
		t.Error("expected error for nil entry")
	}

	// Test entry with nil resource
	entry := &index.ResourceEntry{
		Resource: nil,
		FilePath: "/test.yaml",
	}
	if err := ValidateResource(entry); err == nil {
		t.Error("expected error for entry with nil resource")
	}
}
