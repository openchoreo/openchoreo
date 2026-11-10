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

func writeExportFiles(ctx context.Context, output string, resources []resource, rendered [][]byte, report *Report) (resultErr error) {
	root, err := os.OpenRoot(output)
	if err != nil {
		return err
	}
	defer root.Close()
	inventory, err := readManagedFiles(root)
	if err != nil {
		return fmt.Errorf("read export inventory: %w", err)
	}
	// Record successful writes even if a later write fails. Do not prune stale
	// files on failure; the existing partial-report behavior remains unchanged.
	dirty := false
	defer func() {
		if dirty {
			resultErr = errors.Join(resultErr, saveManagedFiles(root, inventory))
		}
	}()
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
		dirty = true
	}
	dirty = dirty || len(inventory.Files) > 0
	if err := reconcileManagedFiles(ctx, root, inventory, current); err != nil {
		return err
	}
	dirty = true
	return nil
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
