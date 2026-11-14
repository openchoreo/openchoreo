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
	return writeFileWithMode(root, relativePath, data, 0o644)
}

func writeFileWithMode(root *os.Root, relativePath string, data []byte, mode fs.FileMode) (writeOutcome, error) {
	localPath := filepath.FromSlash(relativePath)
	if !filepath.IsLocal(localPath) {
		return 0, fmt.Errorf("refusing to write outside the output directory: %q", relativePath)
	}
	outcome := outcomeCreated
	oldData, err := root.ReadFile(localPath)
	switch {
	case err == nil:
		if bytes.Equal(oldData, data) {
			if mode == 0o600 {
				if err := chmodExportPath(root, localPath, mode); err != nil {
					return 0, fmt.Errorf("set output permissions: %w", err)
				}
			}
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
	if err := temporary.Chmod(mode); err != nil {
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
	if err := prepareExportFiles(ctx, snapshot, resources, rendered, &pending); err != nil {
		return err
	}
	if err := snapshot.publish(ctx); err != nil {
		return err
	}
	*report = pending
	return nil
}

// Prepare only generated manifests, obsolete managed files, and the inventory.
// Unrelated repository content is never traversed or copied.
func prepareExportFiles(ctx context.Context, snapshot *exportSnapshot, resources []resource, rendered [][]byte, report *Report) error {
	inventory := &managedFiles{Version: 1, Files: map[string]string{}}
	if snapshot.existing {
		state, err := readExportFileState(snapshot.parent, managedFilesName)
		if err != nil {
			return fmt.Errorf("read export inventory: %w", err)
		}
		snapshot.observed[managedFilesName] = state
		inventory, err = readManagedFiles(snapshot.parent)
		if err != nil {
			return fmt.Errorf("read export inventory: %w", err)
		}
	}
	current := make(map[string]bool, len(resources))
	for i, item := range resources {
		if err := ctx.Err(); err != nil {
			return err
		}
		mode := fs.FileMode(0o644)
		if item.retainsPlaintext {
			mode = 0o600
		}
		description := fmt.Sprintf("write %s %q", item.kind, item.name)
		outcome, err := snapshot.prepareFile(item.path, rendered[i], mode, description)
		if err != nil {
			return fmt.Errorf("%s: %w", description, err)
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
		if snapshot.existing {
			matches, err := managedFileMatches(snapshot.parent, name, inventory.Files[name])
			if err != nil {
				return fmt.Errorf("remove stale export file %q: %w", name, err)
			}
			if matches {
				state, err := readExportFileState(snapshot.parent, name)
				if err != nil {
					return err
				}
				// Recheck the digest in case the file changed during inspection.
				if state.info != nil && state.info.Mode().IsRegular() && state.digest == inventory.Files[name] {
					snapshot.observed[name] = state
					snapshot.changes = append(snapshot.changes, &exportFileChange{
						name: name, before: state, remove: true,
						description: fmt.Sprintf("remove stale export file %q", name),
					})
				}
			}
		}
		delete(inventory.Files, name)
	}
	data, err := json.MarshalIndent(inventory, "", "  ")
	if err != nil {
		return fmt.Errorf("encode export inventory: %w", err)
	}
	_, err = snapshot.prepareFile(managedFilesName, append(data, '\n'), 0o644, "write export inventory")
	if err != nil {
		return fmt.Errorf("write export inventory: %w", err)
	}
	return nil
}

type exportFileState struct {
	info   fs.FileInfo
	digest string
	link   string
}

// Lstat records the directory entry that publication will replace. ReadFile
// follows only links contained by os.Root, including for unchanged detection.
func readExportFileState(root *os.Root, name string) (exportFileState, error) {
	info, err := root.Lstat(name)
	if errors.Is(err, fs.ErrNotExist) {
		return exportFileState{}, nil
	}
	if err != nil {
		return exportFileState{}, err
	}
	state := exportFileState{info: info}
	if info.Mode()&os.ModeSymlink != 0 {
		state.link, err = root.Readlink(name)
		if err != nil {
			return state, err
		}
		info, err = root.Stat(name)
		if errors.Is(err, fs.ErrNotExist) {
			return state, nil
		}
		if err != nil {
			return state, err
		}
	}
	if !info.Mode().IsRegular() {
		return state, fmt.Errorf("output file %q is not a regular file", name)
	}
	data, err := root.ReadFile(name)
	if err != nil {
		return state, err
	}
	state.digest = manifestDigest(data)
	return state, nil
}

func (s exportFileState) equal(other exportFileState) bool {
	if s.info == nil || other.info == nil {
		return s.info == nil && other.info == nil
	}
	return os.SameFile(s.info, other.info) && s.info.Mode() == other.info.Mode() &&
		s.digest == other.digest && s.link == other.link
}

type exportFileChange struct {
	name, description string
	before, after     exportFileState
	remove            bool
	parent            *os.Root
	temporary, backup string
}

type exportSnapshot struct {
	parent      *os.Root
	staging     *os.Root
	stagingName string
	outputName  string
	existing    bool
	observed    map[string]exportFileState
	changes     []*exportFileChange
	directories []string
	published   bool
	retain      bool
}

func (s *exportSnapshot) prepareFile(name string, data []byte, mode fs.FileMode, description string) (writeOutcome, error) {
	if !filepath.IsLocal(filepath.FromSlash(name)) {
		return 0, fmt.Errorf("refusing to write outside the output directory: %q", name)
	}
	before := exportFileState{}
	if s.existing {
		var err error
		before, err = readExportFileState(s.parent, name)
		if err != nil {
			return 0, err
		}
		if expected, ok := s.observed[name]; ok && !expected.equal(before) {
			return 0, fmt.Errorf("output file %q changed during export; retry the export", name)
		}
		s.observed[name] = before
	}
	outcome := outcomeCreated
	if before.info != nil {
		outcome = outcomeUpdated
		if before.digest == manifestDigest(data) {
			outcome = outcomeUnchanged
			if mode != 0o600 || before.info.Mode().Perm() == mode {
				return outcome, nil
			}
			// Preserve owner-only permissions even when contents are unchanged.
			// Replacing a contained symlink avoids chmod of its unrelated target.
		}
	}
	if _, err := writeFileWithMode(s.staging, name, data, mode); err != nil {
		return 0, err
	}
	s.changes = append(s.changes, &exportFileChange{name: name, description: description, before: before})
	return outcome, nil
}

func prepareExportSnapshot(ctx context.Context, output string) (*exportSnapshot, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	output, err := filepath.Abs(output)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(output)
	existing := err == nil
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	if existing && !info.IsDir() {
		return nil, fmt.Errorf("output path %q exists and is not a directory", output)
	}
	// Existing repositories keep their root, including when it is the cwd or
	// a mount point. Staging inside it also needs no write access to its parent.
	location := output
	if !existing {
		location = filepath.Dir(output)
		if err := os.MkdirAll(location, 0o750); err != nil {
			return nil, err
		}
	}
	parent, err := os.OpenRoot(location)
	if err != nil {
		return nil, err
	}
	stagingName := ".occ-export-staging-" + rand.Text()
	if err := parent.Mkdir(stagingName, 0o700); err != nil {
		_ = parent.Close()
		return nil, fmt.Errorf("prepare export staging directory: %w", err)
	}
	staging, err := parent.OpenRoot(stagingName)
	if err != nil {
		_ = parent.RemoveAll(stagingName)
		_ = parent.Close()
		return nil, err
	}
	return &exportSnapshot{
		parent: parent, staging: staging, stagingName: stagingName, outputName: filepath.Base(output),
		existing: existing, observed: map[string]exportFileState{},
	}, nil
}

func (s *exportSnapshot) publish(ctx context.Context) error {
	return s.publishWithRename(ctx, func(root *os.Root, oldName, newName string) error {
		return root.Rename(oldName, newName)
	})
}

// Every file is prepared before publication starts. Renames are atomic for
// individual manifests; a multi-file export is not a filesystem transaction.
// Backups allow failed publication to restore only the paths it changed.
func (s *exportSnapshot) publishWithRename(ctx context.Context, rename func(*os.Root, string, string) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if !s.existing {
		if _, err := s.parent.Lstat(s.outputName); !errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("output directory appeared during export: %q", s.outputName)
		}
		if err := chmodExportPath(s.staging, ".", 0o750); err != nil {
			return err
		}
		if err := rename(s.parent, s.stagingName, s.outputName); err != nil {
			return fmt.Errorf("publish export snapshot: %w", err)
		}
		s.published = true
		return nil
	}
	for name, expected := range s.observed {
		if err := checkExportFileState(s.parent, name, expected); err != nil {
			return err
		}
	}
	// Create destination temporaries and backups before any manifest changes.
	// Each pinned parent keeps renames contained despite symlink replacement.
	for _, change := range s.changes {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := s.preparePublication(change); err != nil {
			return fmt.Errorf("%s: %w", change.description, err)
		}
	}
	for i, change := range s.changes {
		err := ctx.Err()
		if err == nil {
			err = checkExportFileState(change.parent, filepath.Base(change.name), change.before)
		}
		if err == nil {
			if change.remove {
				err = change.parent.Remove(filepath.Base(change.name))
			} else {
				err = rename(change.parent, change.temporary, filepath.Base(change.name))
			}
		}
		if err != nil {
			return errors.Join(fmt.Errorf("%s: %w", change.description, err), s.rollback(i))
		}
	}
	s.published = true
	return nil
}

func checkExportFileState(root *os.Root, name string, expected exportFileState) error {
	actual, err := readExportFileState(root, name)
	if err != nil {
		return err
	}
	if !expected.equal(actual) {
		return fmt.Errorf("output file %q changed during export; retry the export", name)
	}
	return nil
}

func (s *exportSnapshot) preparePublication(change *exportFileChange) error {
	directory := filepath.Dir(change.name)
	if err := s.makeDirectories(directory); err != nil {
		return err
	}
	parent, err := s.parent.OpenRoot(directory)
	if err != nil {
		return err
	}
	change.parent = parent
	base := filepath.Base(change.name)
	if err := checkExportFileState(parent, base, change.before); err != nil {
		return err
	}
	if !change.remove {
		change.temporary = ".occ-export-" + rand.Text()
		info, err := s.staging.Stat(change.name)
		if err != nil {
			return err
		}
		if err := copyExportFile(s.staging, parent, change.name, change.temporary, info.Mode().Perm()); err != nil {
			return err
		}
		change.after, err = readExportFileState(parent, change.temporary)
		if err != nil {
			return err
		}
	}
	if change.before.info != nil {
		change.backup = ".occ-export-backup-" + rand.Text()
		// Hard links retain the original inode and its metadata without making
		// the published file disappear before its replacement is ready.
		if err := parent.Link(base, change.backup); err != nil {
			return err
		}
	}
	return nil
}

func (s *exportSnapshot) makeDirectories(directory string) error {
	if directory == "." {
		return nil
	}
	if err := s.makeDirectories(filepath.Dir(directory)); err != nil {
		return err
	}
	err := s.parent.Mkdir(directory, 0o750)
	if errors.Is(err, fs.ErrExist) {
		return nil
	}
	if err == nil {
		s.directories = append(s.directories, directory)
	}
	return err
}

func (s *exportSnapshot) rollback(count int) error {
	var result error
	for i := count - 1; i >= 0; i-- {
		change := s.changes[i]
		base := filepath.Base(change.name)
		err := checkExportFileState(change.parent, base, change.after)
		if err == nil {
			if change.backup == "" {
				err = change.parent.Remove(base)
			} else {
				err = change.parent.Rename(change.backup, base)
			}
		}
		if err != nil {
			s.retain = true
			result = errors.Join(result, fmt.Errorf("restore %q (backup retained at %q): %w", change.name,
				filepath.Join(change.parent.Name(), change.backup), err))
		}
	}
	return result
}

func copyExportFile(source, destination *os.Root, name, target string, mode fs.FileMode) error {
	input, err := source.Open(name)
	if err != nil {
		return err
	}
	defer input.Close()
	output, err := destination.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(output, input)
	modeErr := output.Chmod(mode)
	return errors.Join(copyErr, modeErr, output.Close())
}

func chmodExportPath(root *os.Root, name string, mode fs.FileMode) error {
	file, err := root.Open(name)
	if err != nil {
		return err
	}
	defer file.Close()
	return file.Chmod(mode)
}

func (s *exportSnapshot) cleanup() {
	for _, change := range s.changes {
		if change.parent != nil {
			if change.temporary != "" {
				_ = change.parent.Remove(change.temporary)
			}
			if change.backup != "" && !s.retain {
				_ = change.parent.Remove(change.backup)
			}
			_ = change.parent.Close()
		}
	}
	if !s.published {
		for i := len(s.directories) - 1; i >= 0; i-- {
			// Remove only empty directories created by this attempt.
			_ = s.parent.Remove(s.directories[i])
		}
	}
	_ = s.staging.Close()
	_ = s.parent.RemoveAll(s.stagingName)
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
	matches, err := managedFileMatches(root, name, digest)
	if err != nil || !matches {
		return err
	}
	err = root.Remove(filepath.FromSlash(name))
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return err
}

func managedFileMatches(root *os.Root, name, digest string) (bool, error) {
	// Do not follow symlinks, including symlinked parent directories. Ownership
	// applies to the previously written regular file, not a replacement or target.
	parts := strings.Split(name, "/")
	for i := range parts {
		part := path.Join(parts[:i+1]...)
		info, err := root.Lstat(filepath.FromSlash(part))
		if errors.Is(err, fs.ErrNotExist) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		if info.Mode()&os.ModeSymlink != 0 || (part == name && !info.Mode().IsRegular()) {
			return false, nil
		}
	}
	data, err := root.ReadFile(filepath.FromSlash(name))
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return manifestDigest(data) == digest, nil
}

func manifestDigest(data []byte) string {
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}
