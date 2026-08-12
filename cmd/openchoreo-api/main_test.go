// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"testing"

	coreconfig "github.com/openchoreo/openchoreo/internal/config"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/config"
)

// inImageConfigPath is the config file baked into the openchoreo-api image by
// the Dockerfile. It carries no resource_tree section: those rules come from the
// Go defaults.
const inImageConfigPath = "config.yaml"

// loadForValidation loads a config file the way main() does, up to the point
// where the configuration is validated.
func loadForValidation(t *testing.T, configPath string) (*coreconfig.Loader, config.Config) {
	t.Helper()

	loader, err := config.NewLoader(configPath, nil)
	if err != nil {
		t.Fatalf("failed to load %s: %v", configPath, err)
	}

	var cfg config.Config
	if err := loader.Unmarshal("", &cfg); err != nil {
		t.Fatalf("failed to unmarshal %s: %v", configPath, err)
	}
	return loader, cfg
}

// TestInImageConfig_PassesStartupValidation is the acceptance check on the
// config file baked into the image: it has to pass both startup validators
// unchanged, so a defect in it fails here rather than in a running container.
func TestInImageConfig_PassesStartupValidation(t *testing.T) {
	loader, cfg := loadForValidation(t, inImageConfigPath)

	if err := cfg.ValidateWithRaw(loader); err != nil {
		t.Fatalf("%s must pass startup validation, got:\n%v", inImageConfigPath, err)
	}
}
