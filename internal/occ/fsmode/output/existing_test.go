// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package output

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeYAML(t *testing.T, dir, filename, content string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(dir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, filename), []byte(content), 0600))
}

func TestFindLatestRelease(t *testing.T) {
	t.Run("directory does not exist", func(t *testing.T) {
		release, path, err := FindLatestRelease("/nonexistent/dir", "my-comp")
		require.NoError(t, err)
		assert.Nil(t, release)
		assert.Empty(t, path)
	})

	t.Run("empty directory", func(t *testing.T) {
		dir := t.TempDir()
		release, path, err := FindLatestRelease(dir, "my-comp")
		require.NoError(t, err)
		assert.Nil(t, release)
		assert.Empty(t, path)
	})

	t.Run("single release file", func(t *testing.T) {
		dir := t.TempDir()
		writeYAML(t, dir, "my-comp-20250315-1.yaml", `apiVersion: openchoreo.dev/v1alpha1
kind: ComponentRelease
metadata:
  name: my-comp-20250315-1
`)

		release, path, err := FindLatestRelease(dir, "my-comp")
		require.NoError(t, err)
		require.NotNil(t, release)
		assert.Equal(t, "my-comp-20250315-1", release.GetName())
		assert.Equal(t, filepath.Join(dir, "my-comp-20250315-1.yaml"), path)
	})

	t.Run("multiple releases picks latest by name sort", func(t *testing.T) {
		dir := t.TempDir()
		writeYAML(t, dir, "my-comp-20250315-1.yaml", `apiVersion: openchoreo.dev/v1alpha1
kind: ComponentRelease
metadata:
  name: my-comp-20250315-1
`)
		writeYAML(t, dir, "my-comp-20250315-3.yaml", `apiVersion: openchoreo.dev/v1alpha1
kind: ComponentRelease
metadata:
  name: my-comp-20250315-3
`)
		writeYAML(t, dir, "my-comp-20250315-2.yaml", `apiVersion: openchoreo.dev/v1alpha1
kind: ComponentRelease
metadata:
  name: my-comp-20250315-2
`)

		release, path, err := FindLatestRelease(dir, "my-comp")
		require.NoError(t, err)
		require.NotNil(t, release)
		assert.Equal(t, "my-comp-20250315-3", release.GetName())
		assert.Equal(t, filepath.Join(dir, "my-comp-20250315-3.yaml"), path)
	})

	t.Run("non-matching files skipped", func(t *testing.T) {
		dir := t.TempDir()
		writeYAML(t, dir, "other-comp-20250315-1.yaml", `apiVersion: v1
kind: ConfigMap
metadata:
  name: other
`)
		writeYAML(t, dir, "readme.txt", "not a release")

		release, path, err := FindLatestRelease(dir, "my-comp")
		require.NoError(t, err)
		assert.Nil(t, release)
		assert.Empty(t, path)
	})
}

func TestGetNextVersionNumber(t *testing.T) {
	t.Run("directory does not exist", func(t *testing.T) {
		version, err := GetNextVersionNumber("/nonexistent/dir", "my-comp", "20250315")
		require.NoError(t, err)
		assert.Equal(t, "1", version)
	})

	t.Run("empty directory", func(t *testing.T) {
		dir := t.TempDir()
		version, err := GetNextVersionNumber(dir, "my-comp", "20250315")
		require.NoError(t, err)
		assert.Equal(t, "1", version)
	})

	t.Run("existing releases increments from max", func(t *testing.T) {
		dir := t.TempDir()
		writeYAML(t, dir, "my-comp-20250315-1.yaml", "")
		writeYAML(t, dir, "my-comp-20250315-3.yaml", "")
		writeYAML(t, dir, "my-comp-20250315-2.yaml", "")

		version, err := GetNextVersionNumber(dir, "my-comp", "20250315")
		require.NoError(t, err)
		assert.Equal(t, "4", version)
	})

	t.Run("different component ignored", func(t *testing.T) {
		dir := t.TempDir()
		writeYAML(t, dir, "other-comp-20250315-10.yaml", "")

		version, err := GetNextVersionNumber(dir, "my-comp", "20250315")
		require.NoError(t, err)
		assert.Equal(t, "1", version)
	})

	t.Run("different date ignored", func(t *testing.T) {
		dir := t.TempDir()
		writeYAML(t, dir, "my-comp-20250314-5.yaml", "")

		version, err := GetNextVersionNumber(dir, "my-comp", "20250315")
		require.NoError(t, err)
		assert.Equal(t, "1", version)
	})
}
