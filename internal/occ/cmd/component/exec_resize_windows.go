// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

//go:build windows

package component

import (
	"context"
)

// watchResize is a no-op on Windows — SIGWINCH is not available.
func watchResize(_ context.Context, _ *wsWriter, _ int) {}
