// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	configContext "github.com/openchoreo/openchoreo/pkg/cli/cmd/config"
)

// IsConfigFileExists checks if the configuration file exists
func IsConfigFileExists() bool {
	configPath, err := getConfigFilePath()
	if err != nil {
		return false
	}
	_, err = os.Stat(configPath)
	return err == nil
}

// getConfigFilePath returns the path to the choreoctl config file
func getConfigFilePath() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}
	return filepath.Join(homeDir, ".choreoctl", "config"), nil
}

// LoadStoredConfig loads the configuration from disk
func LoadStoredConfig() (*configContext.StoredConfig, error) {
	configPath, err := getConfigFilePath()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(configPath)
	if os.IsNotExist(err) {
		return &configContext.StoredConfig{
			Contexts: []configContext.Context{},
		}, nil
	} else if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var cfg configContext.StoredConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	return &cfg, nil
}

// SaveStoredConfig persists the configuration to disk
func SaveStoredConfig(cfg *configContext.StoredConfig) error {
	configPath, err := getConfigFilePath()
	if err != nil {
		return fmt.Errorf("failed to get config file path: %w", err)
	}

	// Create directory if it doesn't exist
	if err := os.MkdirAll(filepath.Dir(configPath), 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("failed to serialize config: %w", err)
	}

	if err := os.WriteFile(configPath, data, 0600); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}
