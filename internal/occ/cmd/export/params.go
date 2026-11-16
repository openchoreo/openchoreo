// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package export

import "github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"

// pageParams supplies only pagination parameters. Fetching deliberately leaves
// project, component, resource, and label selectors unset.
func pageParams(limit int, cursor string) (*gen.LimitParam, *gen.CursorParam) {
	if cursor == "" {
		return &limit, nil
	}
	return &limit, &cursor
}
