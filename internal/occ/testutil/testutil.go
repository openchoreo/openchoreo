// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

// Package testutil provides shared test helper utilities for occ sub-packages.
// It is intended to be imported only from *_test.go files.
package testutil

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
	sigsyaml "sigs.k8s.io/yaml"
)

// NonExpiredJWT is a minimal unsigned JWT with exp=9999999999 (year 2286).
// header: {"alg":"none","typ":"JWT"}, payload: {"exp":9999999999}
const NonExpiredJWT = "eyJhbGciOiJub25lIiwidHlwIjoiSldUIn0.eyJleHAiOjk5OTk5OTk5OTl9." //nolint:gosec // test token

// RoundTripFunc lets a plain function satisfy http.RoundTripper.
type RoundTripFunc func(*http.Request) (*http.Response, error)

// RoundTrip implements http.RoundTripper.
func (f RoundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// SetTransport replaces http.DefaultTransport for the duration of the test.
func SetTransport(t *testing.T, rt http.RoundTripper) {
	t.Helper()
	original := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = original })
	http.DefaultTransport = rt
}

// JSONResp builds an *http.Response with the given status and JSON body.
func JSONResp(status int, body any) *http.Response {
	b, _ := json.Marshal(body)
	return &http.Response{
		StatusCode: status,
		Body:       io.NopCloser(bytes.NewReader(b)),
		Header:     http.Header{"Content-Type": []string{"application/json"}},
	}
}

// CaptureStdout runs fn while redirecting os.Stdout and returns what was written.
func CaptureStdout(t *testing.T, fn func()) string {
	t.Helper()

	r, w, err := os.Pipe()
	require.NoError(t, err)

	origStdout := os.Stdout
	os.Stdout = w
	defer func() {
		os.Stdout = origStdout
		w.Close()
		r.Close()
	}()

	fn()

	os.Stdout = origStdout
	w.Close()

	var buf bytes.Buffer
	_, err = io.Copy(&buf, r)
	require.NoError(t, err)

	return buf.String()
}

// SetupTestHome creates a temp HOME directory so LoadStoredConfig reads from
// an isolated location. It overrides HOME/USERPROFILE for the duration of the test.
func SetupTestHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home) // Windows compat
	return home
}

// WriteOCConfig writes a config YAML to ~/.openchoreo/config.
// The cfg parameter is marshaled to YAML and written to disk.
func WriteOCConfig(t *testing.T, home string, cfg any) {
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

// SetOSArgs replaces os.Args for the duration of the test and restores
// the original value via t.Cleanup.
func SetOSArgs(t *testing.T, args []string) {
	t.Helper()
	original := os.Args
	t.Cleanup(func() { os.Args = original })
	os.Args = args
}
