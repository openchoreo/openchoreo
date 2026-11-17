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
}

func (p Params) validate() error {
	if strings.TrimSpace(p.OutputDir) == "" {
		return fmt.Errorf("output directory is required (use -o or --output)")
	}
	return nil
}

// pageParams supplies only pagination parameters. Fetching deliberately leaves
// project, component, resource, and label selectors unset.
func pageParams(limit int, cursor string) (*gen.LimitParam, *gen.CursorParam) {
	if cursor == "" {
		return &limit, nil
	}
	return &limit, &cursor
}
