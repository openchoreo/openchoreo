// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package export

import (
	"context"

	"github.com/openchoreo/openchoreo/internal/occ/resources/client"
)

// Run fetches, filters, serializes, and writes the export inventory.
// Redaction and canonicalization remain future stages.
func Run(ctx context.Context, c client.Interface, params Params) ([]SerializedFile, error) {
	if err := params.validate(); err != nil {
		return nil, err
	}
	resources, err := Fetch(ctx, c)
	if err != nil {
		return nil, err
	}
	filtered, err := Filter(ctx, resources)
	if err != nil {
		return nil, err
	}
	files, err := Serialize(ctx, filtered)
	if err != nil {
		return nil, err
	}
	if err := writeFiles(ctx, params.OutputDir, files); err != nil {
		return nil, err
	}
	return files, nil
}
