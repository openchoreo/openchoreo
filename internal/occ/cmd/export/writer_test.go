// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package export

import (
	"bytes"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/openchoreo/openchoreo/internal/occ/resources/client/mocks"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
)

const managedComponentPath = "namespaces/acme/projects/shop/components/api/component.yaml"

func emptyExportClient(t *testing.T) *mocks.MockInterface {
	t.Helper()
	c := mocks.NewMockInterface(t)
	expectNoClusterResources(c)
	c.EXPECT().ListNamespaces(mock.Anything, mock.Anything).Return(&gen.NamespaceList{}, nil)
	return c
}

func TestRun_RemovesOnlyUnmodifiedPreviouslyManagedFiles(t *testing.T) {
	root := t.TempDir()
	first, err := runEngine(t.Context(), mockComponentClient(t, "", ""), Options{OutputDir: root})
	require.NoError(t, err)
	assert.Equal(t, 3, first.Created)
	// A manifest-shaped path alone does not establish exporter ownership.
	const untracked = "namespaces/acme/projects/shop/components/manual/component.yaml"
	_, err = writeFile(root, untracked, []byte("untracked manifest\n"))
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(root, "README.md"), []byte("keep me\n"), 0o600))

	report, err := runEngine(t.Context(), emptyExportClient(t), Options{OutputDir: root})
	require.NoError(t, err)
	assert.Zero(t, report.Exported)
	assert.Zero(t, report.Created)
	assert.Zero(t, report.Updated)
	assert.Zero(t, report.Unchanged)
	assert.NoFileExists(t, filepath.Join(root, filepath.FromSlash(managedComponentPath)))
	assert.NoFileExists(t, filepath.Join(root, "namespaces", "acme", "namespace.yaml"))
	assert.NoFileExists(t, filepath.Join(root, "namespaces", "acme", "projects", testProjectName, "project.yaml"))
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(untracked)))
	require.NoError(t, err)
	assert.Equal(t, "untracked manifest\n", string(data))
	data, err = os.ReadFile(filepath.Join(root, "README.md"))
	require.NoError(t, err)
	assert.Equal(t, "keep me\n", string(data))
}

func TestRun_PreservesModifiedStaleFiles(t *testing.T) {
	root := t.TempDir()
	_, err := runEngine(t.Context(), mockComponentClient(t, "", ""), Options{OutputDir: root})
	require.NoError(t, err)
	component := filepath.Join(root, filepath.FromSlash(managedComponentPath))
	require.NoError(t, os.WriteFile(component, []byte("user edited manifest\n"), 0o600))
	for range 2 {
		_, err = runEngine(t.Context(), emptyExportClient(t), Options{OutputDir: root})
		require.NoError(t, err)
		data, err := os.ReadFile(component)
		require.NoError(t, err)
		assert.Equal(t, "user edited manifest\n", string(data))
	}
}

func TestRun_PreservesExportsWithoutInventory(t *testing.T) {
	root := t.TempDir()
	_, err := writeFile(root, managedComponentPath, []byte("legacy export\n"))
	require.NoError(t, err)
	_, err = runEngine(t.Context(), emptyExportClient(t), Options{OutputDir: root})
	require.NoError(t, err)
	assert.FileExists(t, filepath.Join(root, filepath.FromSlash(managedComponentPath)))
}

func TestWriteExportFiles_FailureDoesNotPruneAndTracksPartialWrites(t *testing.T) {
	root := t.TempDir()
	_, err := runEngine(t.Context(), mockComponentClient(t, "", ""), Options{OutputDir: root})
	require.NoError(t, err)
	const added = "namespaces/acme/added.yaml"
	const blocked = "namespaces/acme/blocked.yaml"
	require.NoError(t, os.Mkdir(filepath.Join(root, filepath.FromSlash(blocked)), 0o750))
	report := &Report{ExportedByKind: map[string]int{}}
	err = writeExportFiles(t.Context(), root, []resource{
		{kind: "Namespace", name: "added", path: added},
		{kind: "Namespace", name: "blocked", path: blocked},
	}, [][]byte{[]byte("new manifest\n"), []byte("blocked\n")}, report)
	require.ErrorContains(t, err, "write Namespace")
	assert.Equal(t, 1, report.Created)
	assert.Equal(t, 1, report.Exported)
	assert.FileExists(t, filepath.Join(root, filepath.FromSlash(managedComponentPath)))
	assert.FileExists(t, filepath.Join(root, filepath.FromSlash(added)))

	// A later successful export can also clean up the partial run's managed file.
	_, err = runEngine(t.Context(), emptyExportClient(t), Options{OutputDir: root})
	require.NoError(t, err)
	assert.NoFileExists(t, filepath.Join(root, filepath.FromSlash(added)))
	assert.NoFileExists(t, filepath.Join(root, filepath.FromSlash(managedComponentPath)))
	assert.DirExists(t, filepath.Join(root, filepath.FromSlash(blocked)))
}

func TestReconcileManagedFiles_PreservesSymlinksAndTheirTargets(t *testing.T) {
	for _, parentLink := range []bool{false, true} {
		name := "manifest symlink"
		if parentLink {
			name = "parent symlink"
		}
		t.Run(name, func(t *testing.T) {
			rootPath := t.TempDir()
			outside := t.TempDir()
			data := []byte("previous export\n")
			_, err := writeFile(outside, "manifest.yaml", data)
			require.NoError(t, err)
			const relative = "namespaces/manifest.yaml"
			if parentLink {
				require.NoError(t, os.Symlink(outside, filepath.Join(rootPath, "namespaces")))
			} else {
				require.NoError(t, os.Mkdir(filepath.Join(rootPath, "namespaces"), 0o750))
				require.NoError(t, os.Symlink(filepath.Join(outside, "manifest.yaml"), filepath.Join(rootPath, filepath.FromSlash(relative))))
			}
			root, err := os.OpenRoot(rootPath)
			require.NoError(t, err)
			defer root.Close()
			inventory := &managedFiles{Version: 1, Files: map[string]string{relative: manifestDigest(data)}}
			require.NoError(t, reconcileManagedFiles(t.Context(), root, inventory, nil))
			assert.FileExists(t, filepath.Join(rootPath, filepath.FromSlash(relative)))
			got, err := os.ReadFile(filepath.Join(outside, "manifest.yaml"))
			require.NoError(t, err)
			assert.Equal(t, data, got)
		})
	}
}

func TestRun_RejectsInvalidInventoryBeforeWriting(t *testing.T) {
	tests := []struct {
		name      string
		inventory managedFiles
	}{
		{name: "unsupported version", inventory: managedFiles{Version: 2, Files: map[string]string{}}},
		{name: "path traversal", inventory: managedFiles{Version: 1, Files: map[string]string{"../outside.yaml": manifestDigest(nil)}}},
		{name: "non-manifest", inventory: managedFiles{Version: 1, Files: map[string]string{"namespaces/README.md": manifestDigest(nil)}}},
		{name: "invalid digest", inventory: managedFiles{Version: 1, Files: map[string]string{managedComponentPath: "invalid"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			data, err := json.Marshal(tt.inventory)
			require.NoError(t, err)
			require.NoError(t, os.WriteFile(filepath.Join(root, managedFilesName), data, 0o600))
			report, err := runEngine(t.Context(), mockComponentClient(t, "", ""), Options{OutputDir: root})
			require.ErrorContains(t, err, "read export inventory")
			assert.Zero(t, report.Exported)
			assert.NoDirExists(t, filepath.Join(root, "namespaces"))
		})
	}
}

func TestRun_CleanupFailurePreservesFilesystemErrorAndIncompleteReport(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root bypasses directory write permissions")
	}
	root := t.TempDir()
	_, err := runEngine(t.Context(), mockComponentClient(t, "", ""), Options{OutputDir: root})
	require.NoError(t, err)
	projectDir := filepath.Join(root, "namespaces", "acme", "projects", testProjectName)
	require.NoError(t, os.Chmod(projectDir, 0o500))
	t.Cleanup(func() { require.NoError(t, os.Chmod(projectDir, 0o750)) })
	var output bytes.Buffer
	err = Run(t.Context(), emptyExportClient(t), &output, Params{OutputDir: root})
	require.ErrorIs(t, err, fs.ErrPermission)
	assert.Contains(t, err.Error(), "remove stale export file")
	assert.NotContains(t, err.Error(), "your role")
	assert.Contains(t, output.String(), "Export incomplete")
	assert.Contains(t, output.String(), "0 resource(s)")
	assert.FileExists(t, filepath.Join(projectDir, "project.yaml"))

	// After permissions are restored, a subsequent export retries the stale file.
	require.NoError(t, os.Chmod(projectDir, 0o750))
	_, err = runEngine(t.Context(), emptyExportClient(t), Options{OutputDir: root})
	require.NoError(t, err)
	assert.NoFileExists(t, filepath.Join(projectDir, "project.yaml"))
}
