// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package logout

import (
	"fmt"

	"github.com/openchoreo/openchoreo/internal/occ/cmd/config"
)

type LogoutImpl struct{}

func NewLogoutImpl() *LogoutImpl {
	return &LogoutImpl{}
}

func (i *LogoutImpl) Logout() error {
	// Get current context and credential
	currentContext, err := config.GetCurrentContext()
	if err != nil {
		return fmt.Errorf("failed to get current context: %w", err)
	}

	credential, err := config.GetCurrentCredential()
	if err != nil {
		return fmt.Errorf("failed to get current credential: %w", err)
	}

	// Load config
	cfg, err := config.LoadStoredConfig()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Find and clear token and refresh token from credential in config
	for idx := range cfg.Credentials {
		if cfg.Credentials[idx].Name == credential.Name {
			cfg.Credentials[idx].Token = ""
			cfg.Credentials[idx].RefreshToken = ""
			break
		}
	}

	// Save updated config
	if err := config.SaveStoredConfig(cfg); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	fmt.Printf("✓ Logged out successfully\n")
	fmt.Printf("Cleared credentials for '%s' in context '%s'\n",
		credential.Name, currentContext.Name)

	return nil
}
