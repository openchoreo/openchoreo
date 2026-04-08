// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

// Package testhelpers provides shared test utilities for the occ sub-packages.
// It is intended to be imported only from *_test.go files.
package testhelpers

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
	sigsyaml "sigs.k8s.io/yaml"

	"github.com/openchoreo/openchoreo/internal/occ/cmd/config"
)

// SetupTestHome creates a temp HOME directory so LoadStoredConfig reads from
// an isolated location. It overrides HOME/USERPROFILE for the duration of the test.
func SetupTestHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home) // Windows compat
	return home
}

// WriteOCConfig writes a StoredConfig YAML to ~/.openchoreo/config.
func WriteOCConfig(t *testing.T, home string, cfg *config.StoredConfig) {
	t.Helper()
	dir := filepath.Join(home, ".openchoreo")
	require.NoError(t, os.MkdirAll(dir, 0755))
	data, err := yaml.Marshal(cfg)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "config"), data, 0600))
}

// ExtractYAML strips non-YAML prefix lines (e.g. "Loading index...") from
// captured stdout and returns only the YAML document(s).
func ExtractYAML(out string) string {
	lines := strings.Split(out, "\n")
	for i, l := range lines {
		trimmed := strings.TrimSpace(l)
		if strings.HasPrefix(trimmed, "apiVersion:") || trimmed == "---" {
			return strings.Join(lines[i:], "\n")
		}
	}
	return out
}

// AssertYAMLEquals parses both expected and actual YAML strings and compares
// the resulting structures for equality, independent of key ordering or formatting.
func AssertYAMLEquals(t *testing.T, expectedYAML, actualYAML string) {
	t.Helper()
	var expected, actual map[string]interface{}
	require.NoError(t, sigsyaml.Unmarshal([]byte(expectedYAML), &expected), "failed to parse expected YAML")
	require.NoError(t, sigsyaml.Unmarshal([]byte(actualYAML), &actual), "failed to parse actual YAML")
	assert.Equal(t, expected, actual)
}

// WriteYAML writes a YAML file to the given relative path under repoDir,
// creating intermediate directories as needed.
func WriteYAML(t *testing.T, repoDir, relPath, content string) {
	t.Helper()
	absPath := filepath.Join(repoDir, relPath)
	require.NoError(t, os.MkdirAll(filepath.Dir(absPath), 0755))
	require.NoError(t, os.WriteFile(absPath, []byte(content), 0600))
}
