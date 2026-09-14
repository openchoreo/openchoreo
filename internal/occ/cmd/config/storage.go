// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
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

// getConfigFilePath returns the path to the openchoreo config file
func getConfigFilePath() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}
	return filepath.Join(homeDir, ".openchoreo", "config"), nil
}

// TokenLockPath returns the advisory lock file guarding token refreshes. It sits beside
// the config so a refresh in one occ process excludes one in another.
func TokenLockPath() (string, error) {
	configPath, err := getConfigFilePath()
	if err != nil {
		return "", err
	}
	return filepath.Join(filepath.Dir(configPath), ".token.lock"), nil
}

// LoadStoredConfig loads the configuration from disk
func LoadStoredConfig() (*StoredConfig, error) {
	configPath, err := getConfigFilePath()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(configPath)
	if os.IsNotExist(err) {
		return &StoredConfig{
			Contexts: []Context{},
		}, nil
	} else if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var cfg StoredConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	return &cfg, nil
}

// SaveStoredConfig persists the configuration to disk
func SaveStoredConfig(cfg *StoredConfig) error {
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

	// Write via a temp file in the same directory and rename, so a crash or a second
	// writer cannot leave the credentials truncated.
	tmp, err := os.CreateTemp(filepath.Dir(configPath), ".config-*")
	if err != nil {
		return fmt.Errorf("failed to create temp config file: %w", err)
	}
	tmpPath := tmp.Name()
	defer func() { _ = os.Remove(tmpPath) }()

	if err := tmp.Chmod(0600); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("failed to set config file permissions: %w", err)
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("failed to write config file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}
	if err := os.Rename(tmpPath, configPath); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}
