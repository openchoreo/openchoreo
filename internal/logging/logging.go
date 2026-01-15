// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

// Package logging provides a standardized logger construction for all OpenChoreo components.
package logging

import (
	"context"
	"log/slog"
	"os"
	"strings"
)

// Config defines logging settings.
type Config struct {
	// Level is the minimum log level (debug, info, warn, error).
	Level string
	// Format is the log output format (json, text).
	Format string
	// AddSource includes source file and line number in log entries.
	AddSource bool
}

// New creates a configured slog.Logger from the config.
func New(cfg Config) *slog.Logger {
	level := parseLevel(cfg.Level)

	opts := &slog.HandlerOptions{
		Level:     level,
		AddSource: cfg.AddSource,
	}

	var handler slog.Handler
	if strings.EqualFold(cfg.Format, "text") {
		handler = slog.NewTextHandler(os.Stdout, opts)
	} else {
		handler = slog.NewJSONHandler(os.Stdout, opts)
	}

	return slog.New(handler)
}

// contextKey is the type for context keys to avoid collisions.
type contextKey struct{}

// loggerKey is the context key for storing the logger.
var loggerKey = contextKey{}

// NewContext returns a new context with the logger attached.
func NewContext(ctx context.Context, logger *slog.Logger) context.Context {
	return context.WithValue(ctx, loggerKey, logger)
}

// FromContext retrieves the logger from context.
// Returns slog.Default() if no logger is found.
func FromContext(ctx context.Context) *slog.Logger {
	if logger, ok := ctx.Value(loggerKey).(*slog.Logger); ok {
		return logger
	}
	return slog.Default()
}

// parseLevel converts the level string to slog.Level.
func parseLevel(level string) slog.Level {
	switch strings.ToLower(level) {
	case "debug":
		return slog.LevelDebug
	case "info":
		return slog.LevelInfo
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
