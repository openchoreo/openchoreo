// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package export

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/openchoreo/openchoreo/internal/occ/resources/client"
)

// Run prepares the complete export before transactionally synchronizing managed
// files. Layouts determine ownership; no inventory is written into the output.
func Run(ctx context.Context, c client.Interface, params Params) ([]SerializedFile, error) {
	if err := params.validate(); err != nil {
		return nil, err
	}
	directory, err := canonicalOutputDir(params.OutputDir)
	if err != nil {
		return nil, fmt.Errorf("resolve output directory: %w", err)
	}
	resources, err := Fetch(ctx, c)
	if err != nil {
		return nil, err
	}
	filtered, err := Filter(ctx, resources)
	if err != nil {
		return nil, err
	}
	canonical, err := Canonicalize(ctx, filtered)
	if err != nil {
		return nil, err
	}
	files, err := Serialize(ctx, canonical)
	if err != nil {
		return nil, err
	}
	options := writeOptions{}
	if err := writeFiles(ctx, directory, files, options); err != nil {
		return nil, err
	}
	return files, nil
}

// Resolve existing ancestors so output paths are consistent even when the
// destination does not exist yet or is reached through a symlink.
func canonicalOutputDir(directory string) (string, error) {
	absolute, err := filepath.Abs(directory)
	if err != nil {
		return "", err
	}
	ancestor := absolute
	for {
		resolved, err := filepath.EvalSymlinks(ancestor)
		if err == nil {
			relative, err := filepath.Rel(ancestor, absolute)
			if err != nil {
				return "", err
			}
			return filepath.Join(resolved, relative), nil
		}
		if !os.IsNotExist(err) || filepath.Dir(ancestor) == ancestor {
			return "", err
		}
		ancestor = filepath.Dir(ancestor)
	}
}
