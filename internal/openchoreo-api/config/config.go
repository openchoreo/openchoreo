// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"

	"github.com/openchoreo/openchoreo/internal/server/middleware/auth/subject"
)

// Config represents the top-level configuration structure for openchoreo-api
type Config struct {
	Security SecurityConfig `yaml:"security"`
}

// SecurityConfig represents the authorization configuration section
type SecurityConfig struct {
	UserTypes []subject.UserTypeConfig `yaml:"user_types"`
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

	if err := subject.ValidateConfig(config.Security.UserTypes); err != nil {
		return nil, fmt.Errorf("invalid user type config: %w", err)
	}

	subject.SortByPriority(config.Security.UserTypes)
	return &config, nil
}
