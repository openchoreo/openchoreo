// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package export

import (
	"context"

	"github.com/openchoreo/openchoreo/internal/occ/resources/client"
)

// Run coordinates the export pipeline. This milestone implements only Fetch;
// filtering, redaction, canonicalization, and serialization are future stages.
func Run(ctx context.Context, c client.Interface) ([]Resource, error) {
	return Fetch(ctx, c)
}
