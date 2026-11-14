// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

// Package templateconfig persists the user's selected template version so the
// occ CLI can honor a version picked with `occ linter templates select` across runs.
package templateconfig

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// EnvConfigPath overrides where the config file is stored. It is useful for
// tests and for running occ from a portable location.
const EnvConfigPath = "OPENCHOREO_TOOLKIT_CONFIG"

// config is the on-disk shape of the config file.
type config struct {
	TemplateVersion string `json:"templateVersion"`
}

// Path returns the config file location: the OPENCHOREO_TOOLKIT_CONFIG
// environment variable when set, otherwise ~/.openchoreo-toolkit/config.json.
func Path() string {
	if p := os.Getenv(EnvConfigPath); p != "" {
		return p
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".openchoreo-toolkit", "config.json")
}

// Load returns the persisted template version selection, or "" when no
// selection has been recorded yet.
func Load() (string, error) {
	path := Path()
	if path == "" {
		return "", nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	var cfg config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return "", fmt.Errorf("parsing %s: %w", path, err)
	}
	return cfg.TemplateVersion, nil
}

// Save persists the template version selection.
func Save(version string) error {
	path := Path()
	if path == "" {
		return nil
	}
	data, err := json.MarshalIndent(config{TemplateVersion: version}, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

// Clear removes any persisted template version selection. A nil/absent file is
// not an error.
func Clear() error {
	path := Path()
	if path == "" {
		return nil
	}
	err := os.Remove(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
