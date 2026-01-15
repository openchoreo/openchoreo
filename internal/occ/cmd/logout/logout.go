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

	// Clear token and refresh token from credential
	credential.Token = ""
	credential.RefreshToken = ""

	// Load and save config
	cfg, err := config.LoadStoredConfig()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	if err := config.SaveStoredConfig(cfg); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	fmt.Printf("✓ Logged out successfully\n")
	fmt.Printf("Cleared credentials for '%s' in context '%s'\n",
		credential.Name, currentContext.Name)

	return nil
}
