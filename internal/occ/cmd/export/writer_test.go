// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package export

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
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

func TestWriteFile_RejectsNonLocalPaths(t *testing.T) {
	for _, relative := range []string{"", "../escape.yaml", "/absolute.yaml"} {
		t.Run(relative, func(t *testing.T) {
			output := filepath.Join(t.TempDir(), "output")
			_, err := writeFile(output, relative, []byte("manifest\n"))
			require.ErrorContains(t, err, "refusing to write outside the output directory")
			assert.NoDirExists(t, output)
			root, err := os.OpenRoot(t.TempDir())
			require.NoError(t, err)
			defer root.Close()
			_, err = writeFileInRoot(root, relative, []byte("manifest\n"))
			require.ErrorContains(t, err, "refusing to write outside the output directory")
		})
	}
}

func TestWriteFile_RejectsEscapingParentSymlinks(t *testing.T) {
	tests := []struct {
		name     string
		relative bool
		existing bool
	}{
		{name: "absolute link with existing file", existing: true},
		{name: "absolute link with missing directories"},
		{name: "relative link with existing file", relative: true, existing: true},
		{name: "relative link with missing directories", relative: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output, outside := t.TempDir(), t.TempDir()
			target := filepath.Join(outside, "nested", "manifest.yaml")
			if tt.existing {
				require.NoError(t, os.Mkdir(filepath.Dir(target), 0o750))
				require.NoError(t, os.WriteFile(target, []byte("original\n"), 0o600))
			}
			linkTarget := outside
			if tt.relative {
				var err error
				linkTarget, err = filepath.Rel(output, outside)
				require.NoError(t, err)
			}
			require.NoError(t, os.Symlink(linkTarget, filepath.Join(output, "namespaces")))
			_, err := writeFile(output, "namespaces/nested/manifest.yaml", []byte("replacement\n"))
			require.Error(t, err)
			if tt.existing {
				data, err := os.ReadFile(target)
				require.NoError(t, err)
				assert.Equal(t, "original\n", string(data))
				entries, err := os.ReadDir(filepath.Dir(target))
				require.NoError(t, err)
				require.Len(t, entries, 1, "no temporary files should be created outside the output")
			} else {
				entries, err := os.ReadDir(outside)
				require.NoError(t, err)
				assert.Empty(t, entries, "no directories should be created outside the output")
			}
		})
	}
}

func TestWriteFile_RejectsEscapingFileSymlinks(t *testing.T) {
	for _, existing := range []bool{false, true} {
		name := "dangling target"
		if existing {
			name = "existing target with identical content"
		}
		t.Run(name, func(t *testing.T) {
			output := t.TempDir()
			target := filepath.Join(t.TempDir(), "manifest.yaml")
			data := []byte("original\n")
			if existing {
				require.NoError(t, os.WriteFile(target, data, 0o600))
			}
			link := filepath.Join(output, "manifest.yaml")
			require.NoError(t, os.Symlink(target, link))
			_, err := writeFile(output, "manifest.yaml", data)
			require.Error(t, err, "outside targets must not be read even for unchanged detection")
			gotTarget, err := os.Readlink(link)
			require.NoError(t, err)
			assert.Equal(t, target, gotTarget)
			if existing {
				got, err := os.ReadFile(target)
				require.NoError(t, err)
				assert.Equal(t, data, got)
			} else {
				assert.NoFileExists(t, target)
			}
		})
	}
}

func TestWriteFile_AllowsContainedSymlinksAndPreservesOutcomes(t *testing.T) {
	output := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(output, "real"), 0o750))
	require.NoError(t, os.Symlink("real", filepath.Join(output, "linked")))
	for _, tt := range []struct {
		data string
		want writeOutcome
	}{
		{data: "first\n", want: outcomeCreated},
		{data: "first\n", want: outcomeUnchanged},
		{data: "second\n", want: outcomeUpdated},
	} {
		outcome, err := writeFile(output, "linked/manifest.yaml", []byte(tt.data))
		require.NoError(t, err)
		assert.Equal(t, tt.want, outcome)
		data, err := os.ReadFile(filepath.Join(output, "real", "manifest.yaml"))
		require.NoError(t, err)
		assert.Equal(t, tt.data, string(data))
	}
	info, err := os.Stat(filepath.Join(output, "real", "manifest.yaml"))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o644), info.Mode().Perm())
	entries, err := os.ReadDir(filepath.Join(output, "real"))
	require.NoError(t, err)
	assert.Len(t, entries, 1, "temporary files should be cleaned up")
}

func TestWriteFileInRoot_KeepsOpenedOutputWhenPathIsReplaced(t *testing.T) {
	parent := t.TempDir()
	output := filepath.Join(parent, "output")
	require.NoError(t, os.Mkdir(output, 0o750))
	root, err := os.OpenRoot(output)
	require.NoError(t, err)
	defer root.Close()
	moved := filepath.Join(parent, "moved")
	require.NoError(t, os.Rename(output, moved))
	outside := t.TempDir()
	require.NoError(t, os.Symlink(outside, output))
	_, err = writeFileInRoot(root, "manifest.yaml", []byte("contained\n"))
	require.NoError(t, err)
	require.NoError(t, saveManagedFiles(root, &managedFiles{Version: 1, Files: map[string]string{}}))
	assert.FileExists(t, filepath.Join(moved, "manifest.yaml"))
	assert.FileExists(t, filepath.Join(moved, managedFilesName))
	entries, err := os.ReadDir(outside)
	require.NoError(t, err)
	assert.Empty(t, entries)
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

func TestWriteExportFiles_FailurePreservesPreviousSnapshot(t *testing.T) {
	root := t.TempDir()
	_, err := runEngine(t.Context(), mockComponentClient(t, "", ""), Options{OutputDir: root})
	require.NoError(t, err)
	const added = "namespaces/acme/added.yaml"
	const blocked = "namespaces/acme/blocked.yaml"
	require.NoError(t, os.Mkdir(filepath.Join(root, filepath.FromSlash(blocked)), 0o750))
	before := exportTreeContents(t, root)
	report := &Report{ExportedByKind: map[string]int{}}
	err = writeExportFiles(t.Context(), root, []resource{
		{kind: "Namespace", name: "acme", path: "namespaces/acme/namespace.yaml"},
		{kind: "Namespace", name: "added", path: added},
		{kind: "Namespace", name: "blocked", path: blocked},
	}, [][]byte{[]byte("replacement namespace\n"), []byte("new manifest\n"), []byte("blocked\n")}, report)
	require.ErrorContains(t, err, "write Namespace")
	assert.Zero(t, report.Created)
	assert.Zero(t, report.Exported)
	assert.FileExists(t, filepath.Join(root, filepath.FromSlash(managedComponentPath)))
	assert.NoFileExists(t, filepath.Join(root, filepath.FromSlash(added)))
	assert.Equal(t, before, exportTreeContents(t, root), "failed staging must preserve manifests, inventory, and unrelated files")

	// Failed preparation leaves the previous ownership inventory intact.
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
	before := exportTreeContents(t, root)
	var output bytes.Buffer
	err = Run(t.Context(), emptyExportClient(t), &output, Params{OutputDir: root})
	require.ErrorIs(t, err, fs.ErrPermission)
	assert.Contains(t, err.Error(), "remove stale export file")
	assert.NotContains(t, err.Error(), "your role")
	assert.Contains(t, output.String(), "Export incomplete")
	assert.Contains(t, output.String(), "0 resource(s)")
	assert.FileExists(t, filepath.Join(projectDir, "project.yaml"))
	assert.Equal(t, before, exportTreeContents(t, root), "failed stale-file cleanup must not affect the published export")

	// After permissions are restored, a subsequent export retries the stale file.
	require.NoError(t, os.Chmod(projectDir, 0o750))
	_, err = runEngine(t.Context(), emptyExportClient(t), Options{OutputDir: root})
	require.NoError(t, err)
	assert.NoFileExists(t, filepath.Join(projectDir, "project.yaml"))
}

func exportTreeContents(t *testing.T, root string) map[string]string {
	t.Helper()
	contents := map[string]string{}
	require.NoError(t, filepath.WalkDir(root, func(name string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(root, name)
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		value := info.Mode().String()
		if info.Mode()&os.ModeSymlink != 0 {
			target, err := os.Readlink(name)
			if err != nil {
				return err
			}
			value += target
		} else if info.Mode().IsRegular() {
			data, err := os.ReadFile(name)
			if err != nil {
				return err
			}
			value += string(data)
		}
		contents[relative] = value
		return nil
	}))
	return contents
}

func TestExportSnapshot_PreparedChangesAreInvisibleAndCancellationPreservesOutput(t *testing.T) {
	for _, existing := range []bool{false, true} {
		name := "new output"
		if existing {
			name = "existing output"
		}
		t.Run(name, func(t *testing.T) {
			output := filepath.Join(t.TempDir(), "output")
			before := map[string]string{}
			if existing {
				_, err := runEngine(t.Context(), mockComponentClient(t, "tier", "old"), Options{OutputDir: output})
				require.NoError(t, err)
				before = exportTreeContents(t, output)
			}
			snapshot, err := prepareExportSnapshot(t.Context(), output)
			require.NoError(t, err)
			defer snapshot.cleanup()
			report := &Report{ExportedByKind: map[string]int{}}
			require.NoError(t, prepareExportFiles(t.Context(), snapshot.staging, nil, nil, report))
			if existing {
				assert.Equal(t, before, exportTreeContents(t, output))
			} else {
				assert.NoDirExists(t, output)
			}
			ctx, cancel := context.WithCancel(t.Context())
			cancel()
			require.ErrorIs(t, snapshot.publish(ctx), context.Canceled)
			if existing {
				assert.Equal(t, before, exportTreeContents(t, output))
			} else {
				assert.NoDirExists(t, output)
			}
		})
	}
}

func TestExportSnapshot_PublicationFailureRestoresPreviousOutput(t *testing.T) {
	output := filepath.Join(t.TempDir(), "output")
	_, err := runEngine(t.Context(), mockComponentClient(t, "", ""), Options{OutputDir: output})
	require.NoError(t, err)
	before := exportTreeContents(t, output)
	snapshot, err := prepareExportSnapshot(t.Context(), output)
	require.NoError(t, err)
	defer snapshot.cleanup()
	require.NoError(t, prepareExportFiles(t.Context(), snapshot.staging, nil, nil, &Report{ExportedByKind: map[string]int{}}))
	publicationErr := errors.New("injected publication failure")
	rename := func(oldName, newName string) error {
		if oldName == snapshot.stagingName {
			return publicationErr
		}
		return snapshot.parent.Rename(oldName, newName)
	}
	err = publishExportSnapshot(snapshot.parent, snapshot.stagingName, snapshot.outputName, true, rename)
	require.ErrorIs(t, err, publicationErr)
	assert.Equal(t, before, exportTreeContents(t, output))
	backups, err := filepath.Glob(filepath.Join(filepath.Dir(output), ".occ-export-backup-*"))
	require.NoError(t, err)
	assert.Empty(t, backups)
}

func TestRun_SnapshotPreservesUnrelatedFilesPermissionsAndSymlinks(t *testing.T) {
	output := filepath.Join(t.TempDir(), "output")
	require.NoError(t, os.Mkdir(output, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(output, "notes.txt"), []byte("private notes\n"), 0o600))
	require.NoError(t, os.Symlink("notes.txt", filepath.Join(output, "notes-link")))
	outside := filepath.Join(t.TempDir(), "outside.txt")
	require.NoError(t, os.WriteFile(outside, []byte("outside\n"), 0o600))
	require.NoError(t, os.Symlink(outside, filepath.Join(output, "outside-link")))
	before := exportTreeContents(t, output)
	report, err := runEngine(t.Context(), mockComponentClient(t, "", ""), Options{OutputDir: output})
	require.NoError(t, err)
	assert.Equal(t, 3, report.Created)
	after := exportTreeContents(t, output)
	for name, content := range before {
		assert.Equal(t, content, after[name], name)
	}
	data, err := os.ReadFile(outside)
	require.NoError(t, err)
	assert.Equal(t, "outside\n", string(data))
}

func TestWriteExportFiles_RejectsWorkingDirectoryAncestor(t *testing.T) {
	output := t.TempDir()
	child := filepath.Join(output, "child")
	require.NoError(t, os.Mkdir(child, 0o750))
	t.Chdir(child)
	before := exportTreeContents(t, output)
	err := writeExportFiles(t.Context(), output, nil, nil, &Report{})
	require.ErrorContains(t, err, "contains the working directory")
	assert.Equal(t, before, exportTreeContents(t, output))
}

func TestWriteExportFiles_InventoryFailureDiscardsPreparedManifests(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root bypasses directory write permissions")
	}
	output := filepath.Join(t.TempDir(), "output")
	_, err := runEngine(t.Context(), mockComponentClient(t, "tier", "old"), Options{OutputDir: output})
	require.NoError(t, err)
	require.NoError(t, os.Chmod(output, 0o500))
	t.Cleanup(func() { require.NoError(t, os.Chmod(output, 0o750)) })
	before := exportTreeContents(t, output)
	report, err := runEngine(t.Context(), mockComponentClient(t, "tier", "new"), Options{OutputDir: output})
	require.ErrorIs(t, err, fs.ErrPermission)
	assert.Contains(t, err.Error(), "write export inventory")
	assert.Zero(t, report.Exported)
	assert.Zero(t, report.Updated)
	assert.Equal(t, before, exportTreeContents(t, output))
}
