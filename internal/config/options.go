// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"log/slog"
)

// Option configures a Loader.
type Option func(*Loader)

// WithLogger sets a custom logger for debug output.
func WithLogger(logger *slog.Logger) Option {
	return func(l *Loader) {
		l.logger = logger
	}
}
