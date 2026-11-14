// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package export

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"net"
	"os"
	"path/filepath"
	"strings"
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

func TestWriteFileInRoot_UnchangedOrdinaryFilePreservesPermissions(t *testing.T) {
	output := t.TempDir()
	data := []byte("ordinary manifest\n")
	name := filepath.Join(output, "manifest.yaml")
	require.NoError(t, os.WriteFile(name, data, 0o600))
	outcome, err := writeFile(output, "manifest.yaml", data)
	require.NoError(t, err)
	assert.Equal(t, outcomeUnchanged, outcome)
	info, err := os.Stat(name)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
}

func TestWriteFileWithMode_DoesNotChmodEscapingSymlinkTargets(t *testing.T) {
	output := t.TempDir()
	target := filepath.Join(t.TempDir(), "manifest.yaml")
	data := []byte("unchanged contents\n")
	require.NoError(t, os.WriteFile(target, data, 0o600))
	require.NoError(t, os.Chmod(target, 0o644))
	require.NoError(t, os.Symlink(target, filepath.Join(output, "manifest.yaml")))
	root, err := os.OpenRoot(output)
	require.NoError(t, err)
	defer root.Close()
	_, err = writeFileWithMode(root, "manifest.yaml", data, 0o600)
	require.Error(t, err)
	info, err := os.Stat(target)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o644), info.Mode().Perm())
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
		if entry.IsDir() && strings.HasPrefix(entry.Name(), ".occ-export-staging-") {
			return filepath.SkipDir
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
			require.NoError(t, prepareExportFiles(t.Context(), snapshot, nil, nil, report))
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
	for _, update := range []bool{false, true} {
		name := "removed files"
		if update {
			name = "updated and created files"
		}
		t.Run(name, func(t *testing.T) {
			output := filepath.Join(t.TempDir(), "output")
			_, err := runEngine(t.Context(), mockComponentClient(t, "", ""), Options{OutputDir: output})
			require.NoError(t, err)
			before := exportTreeContents(t, output)
			original, err := os.Stat(filepath.Join(output, filepath.FromSlash(managedComponentPath)))
			require.NoError(t, err)
			snapshot, err := prepareExportSnapshot(t.Context(), output)
			require.NoError(t, err)
			var resources []resource
			var rendered [][]byte
			if update {
				resources = []resource{
					{kind: "Component", name: "api", path: managedComponentPath, retainsPlaintext: true},
					{kind: "Namespace", name: "added", path: "namespaces/added/namespace.yaml"},
				}
				rendered = [][]byte{[]byte("replacement component\n"), []byte("new namespace\n")}
			}
			require.NoError(t, prepareExportFiles(t.Context(), snapshot, resources, rendered, &Report{ExportedByKind: map[string]int{}}))
			publicationErr := errors.New("injected publication failure")
			rename := func(root *os.Root, oldName, newName string) error {
				if newName == managedFilesName {
					return publicationErr
				}
				return root.Rename(oldName, newName)
			}
			err = snapshot.publishWithRename(t.Context(), rename)
			snapshot.cleanup()
			require.ErrorIs(t, err, publicationErr)
			assert.Equal(t, before, exportTreeContents(t, output))
			restored, err := os.Stat(filepath.Join(output, filepath.FromSlash(managedComponentPath)))
			require.NoError(t, err)
			assert.True(t, os.SameFile(original, restored), "rollback must restore the original inode and metadata")
			// Compare the full tree, including temporary artifacts after cleanup.
			require.NoError(t, filepath.WalkDir(output, func(name string, entry fs.DirEntry, err error) error {
				if err != nil {
					return err
				}
				assert.False(t, strings.HasPrefix(entry.Name(), ".occ-export-") && entry.Name() != managedFilesName, name)
				return nil
			}))
		})
	}
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

func TestWriteExportFiles_AllowsWorkingDirectoryInsideOutput(t *testing.T) {
	for _, inside := range []bool{false, true} {
		name := "output is cwd"
		if inside {
			name = "output contains cwd"
		}
		t.Run(name, func(t *testing.T) {
			output := t.TempDir()
			cwd := output
			if inside {
				cwd = filepath.Join(output, "child")
				require.NoError(t, os.Mkdir(cwd, 0o750))
			}
			t.Chdir(cwd)
			before, err := os.Stat(output)
			require.NoError(t, err)
			target, err := filepath.Rel(cwd, output)
			require.NoError(t, err)
			_, err = runEngine(t.Context(), mockComponentClient(t, "", ""), Options{OutputDir: target})
			require.NoError(t, err)
			after, err := os.Stat(output)
			require.NoError(t, err)
			assert.True(t, os.SameFile(before, after), "the output directory must retain its identity")
			got, err := os.Getwd()
			require.NoError(t, err)
			got, err = filepath.EvalSymlinks(got)
			require.NoError(t, err)
			want, err := filepath.EvalSymlinks(cwd)
			require.NoError(t, err)
			assert.Equal(t, want, got)
		})
	}
}

func TestRun_LeavesGitSocketAndUnrelatedMetadataUntouched(t *testing.T) {
	output, err := os.MkdirTemp("", "occ-git-")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, os.RemoveAll(output)) })
	git := filepath.Join(output, ".git")
	require.NoError(t, os.Mkdir(git, 0o750))
	listener, err := net.Listen("unix", filepath.Join(git, "fsmonitor.sock"))
	require.NoError(t, err)
	defer listener.Close()
	notes := filepath.Join(output, "notes.txt")
	require.NoError(t, os.WriteFile(notes, []byte("notes\n"), 0o600))
	require.NoError(t, os.Link(notes, filepath.Join(output, "notes-copy.txt")))
	before, err := os.Stat(notes)
	require.NoError(t, err)
	gitBefore, err := os.Stat(git)
	require.NoError(t, err)
	_, err = runEngine(t.Context(), mockComponentClient(t, "", ""), Options{OutputDir: output})
	require.NoError(t, err)
	after, err := os.Stat(notes)
	require.NoError(t, err)
	linked, err := os.Stat(filepath.Join(output, "notes-copy.txt"))
	require.NoError(t, err)
	assert.True(t, os.SameFile(before, after))
	assert.True(t, os.SameFile(after, linked), "unrelated hard links must survive")
	assert.Equal(t, before.ModTime(), after.ModTime())
	gitAfter, err := os.Stat(git)
	require.NoError(t, err)
	assert.True(t, os.SameFile(gitBefore, gitAfter))
	assert.Equal(t, gitBefore.ModTime(), gitAfter.ModTime())
	connection, err := net.Dial("unix", listener.Addr().String())
	require.NoError(t, err)
	require.NoError(t, connection.Close())
}

func TestRun_DoesNotNeedWritableRepositoryParent(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root bypasses directory write permissions")
	}
	parent := t.TempDir()
	output := filepath.Join(parent, "output")
	require.NoError(t, os.Mkdir(output, 0o750))
	require.NoError(t, os.Chmod(parent, 0o500))
	t.Cleanup(func() { require.NoError(t, os.Chmod(parent, 0o750)) })
	_, err := runEngine(t.Context(), mockComponentClient(t, "", ""), Options{OutputDir: output})
	require.NoError(t, err)
}

func TestExportSnapshot_PreservesConcurrentUnrelatedChanges(t *testing.T) {
	output := t.TempDir()
	_, err := runEngine(t.Context(), mockComponentClient(t, "", ""), Options{OutputDir: output})
	require.NoError(t, err)
	snapshot, err := prepareExportSnapshot(t.Context(), output)
	require.NoError(t, err)
	defer snapshot.cleanup()
	require.NoError(t, prepareExportFiles(t.Context(), snapshot, nil, nil, &Report{ExportedByKind: map[string]int{}}))
	const manual = "namespaces/acme/manual.yaml"
	_, err = writeFile(output, manual, []byte("concurrent manifest\n"))
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(output, "README.md"), []byte("concurrent notes\n"), 0o600))
	require.NoError(t, snapshot.publish(t.Context()))
	data, err := os.ReadFile(filepath.Join(output, filepath.FromSlash(manual)))
	require.NoError(t, err)
	assert.Equal(t, "concurrent manifest\n", string(data))
	data, err = os.ReadFile(filepath.Join(output, "README.md"))
	require.NoError(t, err)
	assert.Equal(t, "concurrent notes\n", string(data))
}

func TestExportSnapshot_RejectsConcurrentManagedChanges(t *testing.T) {
	output := t.TempDir()
	_, err := runEngine(t.Context(), mockComponentClient(t, "", ""), Options{OutputDir: output})
	require.NoError(t, err)
	snapshot, err := prepareExportSnapshot(t.Context(), output)
	require.NoError(t, err)
	require.NoError(t, prepareExportFiles(t.Context(), snapshot, nil, nil, &Report{ExportedByKind: map[string]int{}}))
	component := filepath.Join(output, filepath.FromSlash(managedComponentPath))
	require.NoError(t, os.WriteFile(component, []byte("concurrent edit\n"), 0o600))
	before := exportTreeContents(t, output)
	require.ErrorContains(t, snapshot.publish(t.Context()), "changed during export")
	snapshot.cleanup()
	assert.Equal(t, before, exportTreeContents(t, output))
}

func TestExportSnapshot_ConflictDuringPublicationRestoresEarlierWrites(t *testing.T) {
	output := t.TempDir()
	_, err := runEngine(t.Context(), mockComponentClient(t, "", ""), Options{OutputDir: output})
	require.NoError(t, err)
	snapshot, err := prepareExportSnapshot(t.Context(), output)
	require.NoError(t, err)
	const namespacePath = "namespaces/acme/namespace.yaml"
	resources := []resource{
		{kind: "Component", name: "api", path: managedComponentPath},
		{kind: "Namespace", name: "acme", path: namespacePath},
	}
	rendered := [][]byte{[]byte("updated component\n"), []byte("updated namespace\n")}
	require.NoError(t, prepareExportFiles(t.Context(), snapshot, resources, rendered, &Report{ExportedByKind: map[string]int{}}))
	before := exportTreeContents(t, output)
	rename := func(root *os.Root, oldName, newName string) error {
		if err := root.Rename(oldName, newName); err != nil {
			return err
		}
		if newName == "component.yaml" {
			return os.WriteFile(filepath.Join(output, filepath.FromSlash(namespacePath)), []byte("concurrent namespace\n"), 0o600)
		}
		return nil
	}
	err = snapshot.publishWithRename(t.Context(), rename)
	snapshot.cleanup()
	require.ErrorContains(t, err, "changed during export")
	after := exportTreeContents(t, output)
	assert.NotEqual(t, before[namespacePath], after[namespacePath])
	delete(before, namespacePath)
	delete(after, namespacePath)
	assert.Equal(t, before, after, "earlier writes must roll back without discarding the concurrent edit")
}

func TestWriteExportFiles_ReadOnlyOutputPreservesManifests(t *testing.T) {
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
	assert.Contains(t, err.Error(), "prepare export staging directory")
	assert.Zero(t, report.Exported)
	assert.Zero(t, report.Updated)
	assert.Equal(t, before, exportTreeContents(t, output))
}
