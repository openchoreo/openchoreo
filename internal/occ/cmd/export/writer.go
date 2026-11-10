// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package export

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

type writeOutcome int

const (
	outcomeCreated writeOutcome = iota
	outcomeUpdated
	outcomeUnchanged
)

// prepareOutputDir creates the output directory if it does not exist.
func prepareOutputDir(root string) error {
	info, err := os.Stat(root)
	switch {
	case err == nil && !info.IsDir():
		return fmt.Errorf("output path %q exists and is not a directory", root)
	case err == nil:
		return nil
	case errors.Is(err, os.ErrNotExist):
		if err := os.MkdirAll(root, 0o750); err != nil {
			return fmt.Errorf("create output directory %q: %w", root, err)
		}
		return nil
	default:
		return fmt.Errorf("access output directory %q: %w", root, err)
	}
}

// writeFile atomically writes a manifest and reports whether its contents changed.
func writeFile(root, relativePath string, data []byte) (writeOutcome, error) {
	if !filepath.IsLocal(filepath.FromSlash(relativePath)) {
		return 0, fmt.Errorf("refusing to write outside the output directory: %q", relativePath)
	}
	if err := prepareOutputDir(root); err != nil {
		return 0, err
	}
	output, err := os.OpenRoot(root)
	if err != nil {
		return 0, err
	}
	defer output.Close()
	return writeFileInRoot(output, relativePath, data)
}

func writeFileInRoot(root *os.Root, relativePath string, data []byte) (writeOutcome, error) {
	localPath := filepath.FromSlash(relativePath)
	if !filepath.IsLocal(localPath) {
		return 0, fmt.Errorf("refusing to write outside the output directory: %q", relativePath)
	}
	outcome := outcomeCreated
	oldData, err := root.ReadFile(localPath)
	switch {
	case err == nil:
		if bytes.Equal(oldData, data) {
			return outcomeUnchanged, nil
		}
		outcome = outcomeUpdated
	case !errors.Is(err, os.ErrNotExist):
		return 0, fmt.Errorf("read existing file %q: %w", relativePath, err)
	}

	if err := root.MkdirAll(filepath.Dir(localPath), 0o750); err != nil {
		return 0, fmt.Errorf("create output directory: %w", err)
	}
	// Pin the contained parent so changing a symlink cannot redirect later
	// temporary-file, rename, or cleanup operations to a different directory.
	parent, err := root.OpenRoot(filepath.Dir(localPath))
	if err != nil {
		return 0, fmt.Errorf("open output directory: %w", err)
	}
	defer parent.Close()
	temporaryName := ".occ-export-" + rand.Text()
	temporary, err := parent.OpenFile(temporaryName, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return 0, fmt.Errorf("create temporary output file: %w", err)
	}
	defer func() { _ = parent.Remove(temporaryName) }()
	if _, err := temporary.Write(data); err != nil {
		_ = temporary.Close()
		return 0, fmt.Errorf("write temporary output file: %w", err)
	}
	if err := temporary.Chmod(0o644); err != nil {
		_ = temporary.Close()
		return 0, fmt.Errorf("set output permissions: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return 0, fmt.Errorf("close temporary output file: %w", err)
	}
	if err := parent.Rename(temporaryName, filepath.Base(localPath)); err != nil {
		return 0, fmt.Errorf("replace output file %q: %w", relativePath, err)
	}
	return outcome, nil
}

// This extensionless file is metadata, not a manifest for GitOps to apply.
const managedFilesName = ".occ-export-inventory"

type managedFiles struct {
	Version int               `json:"version"`
	Files   map[string]string `json:"files"`
}

func writeExportFiles(ctx context.Context, output string, resources []resource, rendered [][]byte, report *Report) error {
	snapshot, err := prepareExportSnapshot(ctx, output)
	if err != nil {
		return err
	}
	defer snapshot.cleanup()
	pending := *report
	pending.ExportedByKind = make(map[string]int, len(report.ExportedByKind))
	for kind, count := range report.ExportedByKind {
		pending.ExportedByKind[kind] = count
	}
	if err := prepareExportFiles(ctx, snapshot.staging, resources, rendered, &pending); err != nil {
		return err
	}
	if err := snapshot.publish(ctx); err != nil {
		return err
	}
	*report = pending
	return nil
}

func prepareExportFiles(ctx context.Context, root *os.Root, resources []resource, rendered [][]byte, report *Report) error {
	inventory, err := readManagedFiles(root)
	if err != nil {
		return fmt.Errorf("read export inventory: %w", err)
	}
	current := make(map[string]bool, len(resources))
	for i, item := range resources {
		if err := ctx.Err(); err != nil {
			return err
		}
		outcome, err := writeFileInRoot(root, item.path, rendered[i])
		if err != nil {
			return fmt.Errorf("write %s %q: %w", item.kind, item.name, err)
		}
		switch outcome {
		case outcomeCreated:
			report.Created++
		case outcomeUpdated:
			report.Updated++
		case outcomeUnchanged:
			report.Unchanged++
		}
		report.Exported++
		report.ExportedByKind[item.kind]++
		current[item.path] = true
		inventory.Files[item.path] = manifestDigest(rendered[i])
	}
	if err := reconcileManagedFiles(ctx, root, inventory, current); err != nil {
		return err
	}
	return saveManagedFiles(root, inventory)
}

type exportSnapshot struct {
	parent      *os.Root
	staging     *os.Root
	stagingName string
	outputName  string
	existing    bool
}

func (s *exportSnapshot) publish(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return publishExportSnapshot(s.parent, s.stagingName, s.outputName, s.existing, s.parent.Rename)
}

func prepareExportSnapshot(ctx context.Context, output string) (*exportSnapshot, error) {
	output, err := filepath.Abs(output)
	if err != nil {
		return nil, err
	}
	_, err = os.Lstat(output)
	existing := err == nil
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	if existing {
		output, err = filepath.EvalSymlinks(output)
		if err != nil {
			return nil, err
		}
		info, err := os.Stat(output)
		if err != nil {
			return nil, err
		}
		if !info.IsDir() {
			return nil, fmt.Errorf("output path %q exists and is not a directory", output)
		}
		if err := checkExportSnapshotLocation(output); err != nil {
			return nil, err
		}
	}
	if err := os.MkdirAll(filepath.Dir(output), 0o750); err != nil {
		return nil, err
	}
	parent, err := os.OpenRoot(filepath.Dir(output))
	if err != nil {
		return nil, err
	}
	stagingName := ".occ-export-staging-" + rand.Text()
	if err := parent.Mkdir(stagingName, 0o700); err != nil {
		_ = parent.Close()
		return nil, err
	}
	staging, err := parent.OpenRoot(stagingName)
	if err != nil {
		_ = parent.RemoveAll(stagingName)
		_ = parent.Close()
		return nil, err
	}
	snapshot := &exportSnapshot{parent: parent, staging: staging, stagingName: stagingName, outputName: filepath.Base(output), existing: existing}
	if existing {
		var source *os.Root
		source, err = parent.OpenRoot(snapshot.outputName)
		if err == nil {
			err = copyExportSnapshot(ctx, source, staging)
			_ = source.Close()
		}
	} else {
		err = staging.Chmod(".", 0o750)
	}
	if err != nil {
		snapshot.cleanup()
		return nil, fmt.Errorf("prepare export snapshot: %w", err)
	}
	return snapshot, nil
}

// Moving an ancestor of the working directory would detach the caller's cwd.
func checkExportSnapshotLocation(output string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	cwd, err = filepath.EvalSymlinks(cwd)
	if err != nil {
		return err
	}
	relative, err := filepath.Rel(output, cwd)
	if err != nil {
		return err
	}
	if relative == "." || filepath.IsLocal(relative) {
		return fmt.Errorf("output directory %q contains the working directory; run export from outside it", output)
	}
	return nil
}

func copyExportSnapshot(ctx context.Context, source, destination *os.Root) error {
	type directory struct {
		name string
		mode fs.FileMode
	}
	var directories []directory
	err := fs.WalkDir(source.FS(), ".", func(name string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		switch {
		case entry.IsDir():
			directories = append(directories, directory{name: name, mode: info.Mode().Perm()})
			return destination.MkdirAll(name, 0o700)
		case info.Mode()&os.ModeSymlink != 0:
			target, err := source.Readlink(name)
			if err != nil {
				return err
			}
			return destination.Symlink(target, name)
		case info.Mode().IsRegular():
			return copyExportFile(source, destination, name, info.Mode().Perm())
		default:
			return fmt.Errorf("cannot copy non-regular output file %q", name)
		}
	})
	if err != nil {
		return err
	}
	for i := len(directories) - 1; i >= 0; i-- {
		dir := directories[i]
		if err := chmodExportDirectory(destination, dir.name, dir.mode); err != nil {
			return err
		}
	}
	return nil
}

func copyExportFile(source, destination *os.Root, name string, mode fs.FileMode) error {
	input, err := source.Open(name)
	if err != nil {
		return err
	}
	defer input.Close()
	output, err := destination.OpenFile(name, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(output, input)
	modeErr := output.Chmod(mode)
	return errors.Join(copyErr, modeErr, output.Close())
}

func chmodExportDirectory(root *os.Root, name string, mode fs.FileMode) error {
	directory, err := root.Open(name)
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Chmod(mode)
}

// All manifest and inventory writes precede this handoff. A failed publication
// restores the original directory; a failed restoration retains the backup.
// The output path is briefly absent between the backup and publication renames.
func publishExportSnapshot(parent *os.Root, staging, output string, existing bool, rename func(string, string) error) error {
	if !existing {
		if err := rename(staging, output); err != nil {
			return fmt.Errorf("publish export snapshot: %w", err)
		}
		return nil
	}
	backup := ".occ-export-backup-" + rand.Text()
	if err := rename(output, backup); err != nil {
		return fmt.Errorf("back up previous export: %w", err)
	}
	if err := rename(staging, output); err != nil {
		if restoreErr := rename(backup, output); restoreErr != nil {
			return errors.Join(fmt.Errorf("publish export snapshot: %w", err),
				fmt.Errorf("restore previous export (backup retained at %q): %w", filepath.Join(parent.Name(), backup), restoreErr))
		}
		return fmt.Errorf("publish export snapshot: %w", err)
	}
	removeExportSnapshot(parent, backup)
	return nil
}

func removeExportSnapshot(parent *os.Root, name string) {
	root, err := parent.OpenRoot(name)
	if err != nil {
		return
	}
	// Read-only copied directories still need to be removed after failure, and
	// the previous snapshot may have read-only directories after publication.
	_ = fs.WalkDir(root.FS(), ".", func(name string, entry fs.DirEntry, walkErr error) error {
		if walkErr == nil && entry.IsDir() {
			return chmodExportDirectory(root, name, 0o700)
		}
		return walkErr
	})
	_ = root.Close()
	_ = parent.RemoveAll(name)
}

func (s *exportSnapshot) cleanup() {
	_ = s.staging.Close()
	removeExportSnapshot(s.parent, s.stagingName)
	_ = s.parent.Close()
}

func readManagedFiles(root *os.Root) (*managedFiles, error) {
	info, err := root.Lstat(managedFilesName)
	if errors.Is(err, fs.ErrNotExist) {
		return &managedFiles{Version: 1, Files: map[string]string{}}, nil
	}
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%s must be a regular file", managedFilesName)
	}
	data, err := root.ReadFile(managedFilesName)
	if err != nil {
		return nil, err
	}
	var inventory managedFiles
	if err := json.Unmarshal(data, &inventory); err != nil {
		return nil, err
	}
	if inventory.Version != 1 || inventory.Files == nil {
		return nil, errors.New("unsupported export inventory format")
	}
	for name, digest := range inventory.Files {
		if !filepath.IsLocal(filepath.FromSlash(name)) || path.Clean(name) != name ||
			strings.Contains(name, "\\") || path.Ext(name) != ".yaml" ||
			(!strings.HasPrefix(name, "namespaces/") && !strings.HasPrefix(name, "platform-shared/")) {
			return nil, fmt.Errorf("invalid export inventory path %q", name)
		}
		if decoded, err := hex.DecodeString(digest); err != nil || len(decoded) != sha256.Size {
			return nil, fmt.Errorf("invalid export inventory digest for %q", name)
		}
	}
	return &inventory, nil
}

func saveManagedFiles(root *os.Root, inventory *managedFiles) error {
	data, err := json.MarshalIndent(inventory, "", "  ")
	if err != nil {
		return fmt.Errorf("encode export inventory: %w", err)
	}
	if _, err := writeFileInRoot(root, managedFilesName, append(data, '\n')); err != nil {
		return fmt.Errorf("write export inventory: %w", err)
	}
	return nil
}

func reconcileManagedFiles(ctx context.Context, root *os.Root, inventory *managedFiles, current map[string]bool) error {
	stale := make([]string, 0, len(inventory.Files))
	for name := range inventory.Files {
		if !current[name] {
			stale = append(stale, name)
		}
	}
	sort.Strings(stale)
	for _, name := range stale {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := removeManagedFile(root, name, inventory.Files[name]); err != nil {
			return fmt.Errorf("remove stale export file %q: %w", name, err)
		}
		delete(inventory.Files, name)
	}
	return nil
}

func removeManagedFile(root *os.Root, name, digest string) error {
	// Do not follow symlinks, including symlinked parent directories. Ownership
	// applies to the previously written regular file, not a replacement or target.
	parts := strings.Split(name, "/")
	for i := range parts {
		part := path.Join(parts[:i+1]...)
		info, err := root.Lstat(filepath.FromSlash(part))
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 || (part == name && !info.Mode().IsRegular()) {
			return nil
		}
	}
	data, err := root.ReadFile(filepath.FromSlash(name))
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if manifestDigest(data) != digest {
		return nil
	}
	err = root.Remove(filepath.FromSlash(name))
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return err
}

func manifestDigest(data []byte) string {
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}
