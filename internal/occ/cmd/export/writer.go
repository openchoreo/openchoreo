// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package export

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

type writeOutcome int

const (
	outcomeCreated writeOutcome = iota
	outcomeUpdated
	outcomeUnchanged
)

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

func writeFile(root, relativePath string, data []byte) (writeOutcome, error) {
	localPath := filepath.FromSlash(relativePath)
	if !filepath.IsLocal(localPath) {
		return 0, fmt.Errorf("refusing to write outside the output directory: %q", relativePath)
	}
	fullPath := filepath.Join(root, localPath)

	outcome := outcomeCreated
	oldData, err := os.ReadFile(fullPath)
	switch {
	case err == nil:
		if bytes.Equal(oldData, data) {
			return outcomeUnchanged, nil
		}
		outcome = outcomeUpdated
	case !errors.Is(err, os.ErrNotExist):
		return 0, fmt.Errorf("read existing file %q: %w", fullPath, err)
	}

	if err := os.MkdirAll(filepath.Dir(fullPath), 0o750); err != nil {
		return 0, fmt.Errorf("create output directory: %w", err)
	}
	temporary, err := os.CreateTemp(filepath.Dir(fullPath), ".occ-export-*")
	if err != nil {
		return 0, fmt.Errorf("create temporary output file: %w", err)
	}
	temporaryPath := temporary.Name()
	if _, err := temporary.Write(data); err != nil {
		_ = temporary.Close()
		_ = os.Remove(temporaryPath)
		return 0, fmt.Errorf("write temporary output file: %w", err)
	}
	if err := temporary.Close(); err != nil {
		_ = os.Remove(temporaryPath)
		return 0, fmt.Errorf("close temporary output file: %w", err)
	}
	if err := os.Chmod(temporaryPath, 0o644); err != nil {
		_ = os.Remove(temporaryPath)
		return 0, fmt.Errorf("set output permissions: %w", err)
	}
	if err := os.Rename(temporaryPath, fullPath); err != nil {
		_ = os.Remove(temporaryPath)
		return 0, fmt.Errorf("replace output file %q: %w", fullPath, err)
	}
	return outcome, nil
}
