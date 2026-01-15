// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"github.com/openchoreo/openchoreo/internal/config"
	"github.com/openchoreo/openchoreo/internal/logging"
)

// LoggingConfig defines logging settings.
type LoggingConfig struct {
	// Level is the minimum log level (debug, info, warn, error).
	Level string `koanf:"level"`
	// Format is the log output format (json, text).
	Format string `koanf:"format"`
	// AddSource includes source file and line number in log entries.
	AddSource bool `koanf:"add_source"`
}

// LoggingDefaults returns the default logging configuration.
func LoggingDefaults() LoggingConfig {
	return LoggingConfig{
		Level:     "info",
		Format:    "json",
		AddSource: false,
	}
}

// Validate validates the logging configuration.
func (c *LoggingConfig) Validate(path *config.Path) config.ValidationErrors {
	var errs config.ValidationErrors

	if err := config.MustBeOneOf(path.Child("level"), c.Level, []string{"debug", "info", "warn", "error"}); err != nil {
		errs = append(errs, err)
	}

	if err := config.MustBeOneOf(path.Child("format"), c.Format, []string{"json", "text"}); err != nil {
		errs = append(errs, err)
	}

	return errs
}

// ToLoggingConfig converts to the logging library config.
func (c *LoggingConfig) ToLoggingConfig() logging.Config {
	return logging.Config{
		Level:     c.Level,
		Format:    c.Format,
		AddSource: c.AddSource,
	}
}
