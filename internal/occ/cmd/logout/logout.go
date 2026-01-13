// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package logout

import (
	"fmt"

	"github.com/openchoreo/openchoreo/internal/occ/cmd/config"
	configContext "github.com/openchoreo/openchoreo/pkg/cli/cmd/config"
)

type LogoutImpl struct{}

func NewLogoutImpl() *LogoutImpl {
	return &LogoutImpl{}
}

func (i *LogoutImpl) Logout() error {
	// 1. Load config
	cfg, err := config.LoadStoredConfig()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	if cfg.CurrentContext == "" {
		return fmt.Errorf("no current context set")
	}

	// 2. Find current context
	var currentContext *configContext.Context
	for idx := range cfg.Contexts {
		if cfg.Contexts[idx].Name == cfg.CurrentContext {
			currentContext = &cfg.Contexts[idx]
			break
		}
	}

	if currentContext == nil {
		return fmt.Errorf("current context '%s' not found", cfg.CurrentContext)
	}

	if currentContext.Credentials == "" {
		return fmt.Errorf("no credentials associated with context '%s'", cfg.CurrentContext)
	}

	// 3. Find and clear token from credential
	credentialFound := false
	for idx := range cfg.Credentials {
		if cfg.Credentials[idx].Name == currentContext.Credentials {
			cfg.Credentials[idx].Token = ""
			credentialFound = true
			break
		}
	}

	if !credentialFound {
		return fmt.Errorf("credential '%s' not found", currentContext.Credentials)
	}

	// 4. Save updated config
	if err := config.SaveStoredConfig(cfg); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	fmt.Printf("✓ Logged out successfully\n")
	fmt.Printf("Cleared token for credential '%s' in context '%s'\n",
		currentContext.Credentials, cfg.CurrentContext)

	return nil
}
