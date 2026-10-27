// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package parser

import (
	"os"
	"path/filepath"
	"sort"
	"testing"
)

const openChoreoYAML = "apiVersion: openchoreo.dev/v1alpha1\nkind: Component\nmetadata:\n  name: demo\n"

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func TestCollectYAMLFilesNonRecursive(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "component.yaml"), openChoreoYAML)
	writeFile(t, filepath.Join(dir, "workload.yml"), openChoreoYAML)
	writeFile(t, filepath.Join(dir, "notes.yaml"), "apiVersion: v1\nkind: ConfigMap\n")
	writeFile(t, filepath.Join(dir, "README.md"), "not yaml")
	writeFile(t, filepath.Join(dir, "nested", "trait.yaml"), openChoreoYAML)

	got, err := CollectYAMLFiles(dir, false)
	if err != nil {
		t.Fatalf("CollectYAMLFiles: %v", err)
	}
	sort.Strings(got)
	want := []string{
		filepath.Join(dir, "component.yaml"),
		filepath.Join(dir, "workload.yml"),
	}
	if len(got) != len(want) {
		t.Fatalf("files = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("files[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestCollectYAMLFilesRecursive(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "component.yaml"), openChoreoYAML)
	writeFile(t, filepath.Join(dir, "nested", "trait.yml"), openChoreoYAML)
	writeFile(t, filepath.Join(dir, "nested", "unrelated.yaml"), "apiVersion: apps/v1\nkind: Deployment\n")

	got, err := CollectYAMLFiles(dir, true)
	if err != nil {
		t.Fatalf("CollectYAMLFiles: %v", err)
	}
	sort.Strings(got)
	want := []string{
		filepath.Join(dir, "component.yaml"),
		filepath.Join(dir, "nested", "trait.yml"),
	}
	if len(got) != len(want) {
		t.Fatalf("files = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("files[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestCollectYAMLFilesExtensionCaseInsensitive(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "COMPONENT.YAML"), openChoreoYAML)

	got, err := CollectYAMLFiles(dir, false)
	if err != nil {
		t.Fatalf("CollectYAMLFiles: %v", err)
	}
	if len(got) != 1 || got[0] != filepath.Join(dir, "COMPONENT.YAML") {
		t.Fatalf("files = %v, want [%s]", got, filepath.Join(dir, "COMPONENT.YAML"))
	}
}

func TestCollectYAMLFilesMissingFolder(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "missing")
	if _, err := CollectYAMLFiles(dir, false); err == nil {
		t.Fatal("expected an error for a missing folder")
	}
	if _, err := CollectYAMLFiles(dir, true); err == nil {
		t.Fatal("expected an error for a missing folder")
	}
}
