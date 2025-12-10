// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"fmt"
	"os"

	"github.com/openchoreo/openchoreo/internal/server/middleware/auth/subject_resolver"
	"gopkg.in/yaml.v3"
)

// Config represents the top-level configuration structure for openchoreo-api
type Config struct {
	Security SecurityConfig `yaml:"security"`
}

// SecurityConfig represents the authorization configuration section
type SecurityConfig struct {
	UserTypes []subject_resolver.UserTypeConfig `yaml:"user_types"`
}

// Load loads and validates the configuration from the specified file path
func Load(filePath string) (*Config, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	if err := subject_resolver.ValidateConfig(config.Security.UserTypes); err != nil {
		return nil, fmt.Errorf("invalid user type config: %w", err)
	}

	subject_resolver.SortByPriority(config.Security.UserTypes)
	return &config, nil
}
