// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"

	authzcore "github.com/openchoreo/openchoreo/internal/authz/core"
)

// Config represents the top-level configuration structure for openchoreo-api
type Config struct {
	Authz AuthzConfig `yaml:"authz"`
}

// AuthzConfig represents the authorization configuration section
type AuthzConfig struct {
	UserTypes []authzcore.UserTypeConfig `yaml:"user_types"`
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

	if err := authzcore.ValidateConfig(config.Authz.UserTypes); err != nil {
		return nil, fmt.Errorf("invalid user type config: %w", err)
	}

	authzcore.SortByPriority(config.Authz.UserTypes)
	return &config, nil
}
