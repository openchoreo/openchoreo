// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"testing"

	coreconfig "github.com/openchoreo/openchoreo/internal/config"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/config"
	"github.com/openchoreo/openchoreo/internal/resourcetree/protocol"
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

// TestInImageConfig_WriteTimeoutOutlastsClientBudget pins the baked config's
// server.timeouts.write against the resource tree ladder. The file layer
// overrides the derived Go default, so a value pinned here silently shortens
// every deployment that runs the image without a mounted ConfigMap.
func TestInImageConfig_WriteTimeoutOutlastsClientBudget(t *testing.T) {
	_, cfg := loadForValidation(t, inImageConfigPath)

	write := cfg.Server.Timeouts.Write
	if write <= protocol.ClientRequestTimeout {
		t.Errorf("%s pins a server write deadline of %v, which must exceed the resource tree client budget %v",
			inImageConfigPath, write, protocol.ClientRequestTimeout)
	}
	if want := config.TimeoutsDefaults().Write; write < want {
		t.Errorf("%s pins a server write deadline of %v, below the derived default %v",
			inImageConfigPath, write, want)
	}
}
