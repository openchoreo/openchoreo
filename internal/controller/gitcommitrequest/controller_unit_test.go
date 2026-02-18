// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package gitcommitrequest

import (
	"os"
	"path/filepath"
	"testing"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

// ---- applyEdits ----

func TestApplyEdits_EmptyEdits(t *testing.T) {
	tmp := t.TempDir()
	err := applyEdits(tmp, nil)
	if err != nil {
		t.Errorf("applyEdits(nil) = %v, want nil", err)
	}
}

func TestApplyEdits_WriteNewFile(t *testing.T) {
	tmp := t.TempDir()
	edits := []openchoreov1alpha1.FileEdit{
		{Path: "hello.txt", Content: "hello world"},
	}
	if err := applyEdits(tmp, edits); err != nil {
		t.Fatalf("applyEdits error: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(tmp, "hello.txt"))
	if err != nil {
		t.Fatalf("ReadFile error: %v", err)
	}
	if string(got) != "hello world" {
		t.Errorf("file content = %q, want %q", string(got), "hello world")
	}
}

func TestApplyEdits_CreateSubdirectory(t *testing.T) {
	tmp := t.TempDir()
	edits := []openchoreov1alpha1.FileEdit{
		{Path: "subdir/nested/file.txt", Content: "nested content"},
	}
	if err := applyEdits(tmp, edits); err != nil {
		t.Fatalf("applyEdits error: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(tmp, "subdir", "nested", "file.txt"))
	if err != nil {
		t.Fatalf("ReadFile error: %v", err)
	}
	if string(got) != "nested content" {
		t.Errorf("file content = %q, want %q", string(got), "nested content")
	}
}

func TestApplyEdits_OverwriteExisting(t *testing.T) {
	tmp := t.TempDir()
	filePath := filepath.Join(tmp, "test.txt")
	if err := os.WriteFile(filePath, []byte("original"), 0o600); err != nil {
		t.Fatalf("WriteFile setup error: %v", err)
	}
	edits := []openchoreov1alpha1.FileEdit{
		{Path: "test.txt", Content: "updated"},
	}
	if err := applyEdits(tmp, edits); err != nil {
		t.Fatalf("applyEdits error: %v", err)
	}
	got, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("ReadFile error: %v", err)
	}
	if string(got) != "updated" {
		t.Errorf("file content = %q, want %q", string(got), "updated")
	}
}

func TestApplyEdits_MultipleFiles(t *testing.T) {
	tmp := t.TempDir()
	edits := []openchoreov1alpha1.FileEdit{
		{Path: "a.txt", Content: "file-a"},
		{Path: "b.txt", Content: "file-b"},
	}
	if err := applyEdits(tmp, edits); err != nil {
		t.Fatalf("applyEdits error: %v", err)
	}
	for _, e := range edits {
		got, err := os.ReadFile(filepath.Join(tmp, e.Path))
		if err != nil {
			t.Fatalf("ReadFile(%s) error: %v", e.Path, err)
		}
		if string(got) != e.Content {
			t.Errorf("file %s content = %q, want %q", e.Path, string(got), e.Content)
		}
	}
}

func TestApplyEdits_WithJSONPatch(t *testing.T) {
	tmp := t.TempDir()
	// Create a JSON file to patch
	original := `{"name":"world","version":1}`
	filePath := filepath.Join(tmp, "config.json")
	if err := os.WriteFile(filePath, []byte(original), 0o600); err != nil {
		t.Fatalf("WriteFile setup error: %v", err)
	}

	patch := `[{"op":"replace","path":"/name","value":"openchoreo"}]`
	edits := []openchoreov1alpha1.FileEdit{
		{Path: "config.json", Patch: patch},
	}
	if err := applyEdits(tmp, edits); err != nil {
		t.Fatalf("applyEdits with patch error: %v", err)
	}
	got, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("ReadFile error: %v", err)
	}
	if string(got) != `{"name":"openchoreo","version":1}` {
		t.Errorf("patched content = %q", string(got))
	}
}

func TestApplyEdits_InvalidJSONPatch(t *testing.T) {
	tmp := t.TempDir()
	// Create a JSON file
	filePath := filepath.Join(tmp, "config.json")
	if err := os.WriteFile(filePath, []byte(`{"name":"world"}`), 0o600); err != nil {
		t.Fatalf("WriteFile setup error: %v", err)
	}

	// Invalid operation that removes a non-existent field
	patch := `[{"op":"remove","path":"/nonexistent"}]`
	edits := []openchoreov1alpha1.FileEdit{
		{Path: "config.json", Patch: patch},
	}
	// This should return an error since the path doesn't exist
	err := applyEdits(tmp, edits)
	if err == nil {
		t.Error("applyEdits with invalid patch: expected error for non-existent path, got nil")
	}
}
