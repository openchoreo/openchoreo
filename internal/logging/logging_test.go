// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package logging

import (
	"context"
	"log/slog"
	"testing"
)

func TestNew_JSONFormat(t *testing.T) {
	cfg := Config{Level: "info", Format: "json"}
	logger := New(cfg)
	if logger == nil {
		t.Fatal("New() returned nil logger")
	}
}

func TestNew_TextFormat(t *testing.T) {
	cfg := Config{Level: "info", Format: "text"}
	logger := New(cfg)
	if logger == nil {
		t.Fatal("New() returned nil for text format logger")
	}
}

func TestNew_TextFormatCaseInsensitive(t *testing.T) {
	cfg := Config{Level: "INFO", Format: "TEXT"}
	logger := New(cfg)
	if logger == nil {
		t.Fatal("New() returned nil for uppercase TEXT format")
	}
}

func TestNew_WithAddSource(t *testing.T) {
	cfg := Config{Level: "debug", Format: "json", AddSource: true}
	logger := New(cfg)
	if logger == nil {
		t.Fatal("New() returned nil with AddSource=true")
	}
}

func TestNew_AllLogLevels(t *testing.T) {
	levels := []string{"debug", "info", "warn", "warning", "error", "invalid", ""}
	for _, level := range levels {
		t.Run(level, func(t *testing.T) {
			cfg := Config{Level: level, Format: "json"}
			logger := New(cfg)
			if logger == nil {
				t.Errorf("New() returned nil for level %q", level)
			}
		})
	}
}

func TestNewWithComponent_AddsComponentField(t *testing.T) {
	cfg := Config{Level: "info", Format: "json"}
	logger := NewWithComponent(cfg, "test-component")
	if logger == nil {
		t.Fatal("NewWithComponent() returned nil logger")
	}
}

func TestBootstrap_NotNil(t *testing.T) {
	logger := Bootstrap("test-component")
	if logger == nil {
		t.Fatal("Bootstrap() returned nil logger")
	}
}

func TestNewContext_AndFromContext(t *testing.T) {
	cfg := Config{Level: "info", Format: "json"}
	logger := New(cfg)

	ctx := context.Background()
	ctxWithLogger := NewContext(ctx, logger)

	retrieved := FromContext(ctxWithLogger)
	if retrieved == nil {
		t.Fatal("FromContext() returned nil logger")
	}
	// The retrieved logger should be the same one we put in
	if retrieved != logger {
		t.Error("FromContext() returned different logger than what was stored")
	}
}

func TestFromContext_WithoutLogger_ReturnsDefault(t *testing.T) {
	ctx := context.Background()
	logger := FromContext(ctx)
	if logger == nil {
		t.Fatal("FromContext() without logger should return non-nil default")
	}
	// Should return slog.Default()
	if logger != slog.Default() {
		t.Error("FromContext() without logger should return slog.Default()")
	}
}

func TestParseLevel(t *testing.T) {
	tests := []struct {
		input    string
		expected slog.Level
	}{
		{"debug", slog.LevelDebug},
		{"DEBUG", slog.LevelDebug},
		{"info", slog.LevelInfo},
		{"INFO", slog.LevelInfo},
		{"warn", slog.LevelWarn},
		{"WARN", slog.LevelWarn},
		{"warning", slog.LevelWarn},
		{"WARNING", slog.LevelWarn},
		{"error", slog.LevelError},
		{"ERROR", slog.LevelError},
		{"unknown", slog.LevelInfo},
		{"", slog.LevelInfo},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			level := parseLevel(tt.input)
			if level != tt.expected {
				t.Errorf("parseLevel(%q) = %v, want %v", tt.input, level, tt.expected)
			}
		})
	}
}
