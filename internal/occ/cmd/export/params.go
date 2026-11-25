// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package export

import (
	"fmt"
	"strings"

	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
)

// Params specifies the destination of the export directory tree.
type Params struct {
	OutputDir string
	// AllReleases includes the complete release history.
	AllReleases bool
}

func (p Params) validate() error {
	if strings.TrimSpace(p.OutputDir) == "" {
		return fmt.Errorf("output directory is required (use -o or --output)")
	}
	return nil
}

// pageParams supplies the page limit and optional cursor.
func pageParams(limit int, cursor string) (*gen.LimitParam, *gen.CursorParam) {
	if cursor == "" {
		return &limit, nil
	}
	return &limit, &cursor
}
