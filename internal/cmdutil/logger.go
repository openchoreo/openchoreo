// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package cmdutil

import (
	"log/slog"
	"os"
)

// SetupLogger creates a structured logger with the specified level
func SetupLogger(levelStr string) *slog.Logger {
	var level slog.Level
	switch levelStr {
	case "debug":
		level = slog.LevelDebug
	case "info":
		level = slog.LevelInfo
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	default:
		level = slog.LevelInfo
	}

	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: level,
	})

	return slog.New(handler)
}
