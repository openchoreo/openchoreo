// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package celvalidator

import "embed"

// SetCelEnvFS is retained for source compatibility with descriptor-based clients.
// Schema-aware validation derives its environment from document schemas and no
// longer reads the descriptor filesystem.
func SetCelEnvFS(_ embed.FS) {}
