// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package login

import (
	"fmt"
	"os"

	"github.com/openchoreo/openchoreo/internal/occ/auth"
	"github.com/openchoreo/openchoreo/internal/occ/cmd/config"
	configContext "github.com/openchoreo/openchoreo/pkg/cli/cmd/config"
	"github.com/openchoreo/openchoreo/pkg/cli/types/api"
)

type AuthImpl struct{}

var _ api.LoginAPI = &AuthImpl{}

func NewAuthImpl() *AuthImpl {
	return &AuthImpl{}
}

func (i *AuthImpl) Login(params api.LoginParams) error {
	if params.ClientCredentials {
		return i.loginWithClientCredentials(params)
	}
	return fmt.Errorf("interactive login not yet implemented, use --client-credentials")
}

func (i *AuthImpl) loginWithClientCredentials(params api.LoginParams) error {
	// 1. Get client ID/secret from params or environment variables
	clientID := params.ClientID
	if clientID == "" {
		clientID = os.Getenv("OCC_CLIENT_ID")
	}
	clientSecret := params.ClientSecret
	if clientSecret == "" {
		clientSecret = os.Getenv("OCC_CLIENT_SECRET")
	}

	if clientID == "" || clientSecret == "" {
		return fmt.Errorf("client ID and client secret are required (use --client-id and --client-secret flags or OCC_CLIENT_ID and OCC_CLIENT_SECRET environment variables)")
	}

	credentialName := params.CredentialName
	if credentialName == "" {
		credentialName = "default"
	}

	// 2. Load config to get current context's control plane
	cfg, err := config.LoadStoredConfig()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	if cfg.CurrentContext == "" {
		return fmt.Errorf("no current context set, please set a context first using 'occ config set-context'")
	}

	// Find current context
	var currentContext *configContext.Context
	for idx := range cfg.Contexts {
		if cfg.Contexts[idx].Name == cfg.CurrentContext {
			currentContext = &cfg.Contexts[idx]
			break
		}
	}

	if currentContext == nil {
		return fmt.Errorf("current context '%s' not found in config", cfg.CurrentContext)
	}

	// Find control plane for this context
	var controlPlane *configContext.ControlPlane
	for idx := range cfg.ControlPlanes {
		if cfg.ControlPlanes[idx].Name == currentContext.ControlPlane {
			controlPlane = &cfg.ControlPlanes[idx]
			break
		}
	}

	if controlPlane == nil {
		return fmt.Errorf("control plane '%s' not found in config", currentContext.ControlPlane)
	}

	if controlPlane.TokenEndpoint == "" {
		return fmt.Errorf("token endpoint not configured for control plane '%s'", controlPlane.Name)
	}

	// 3. Exchange credentials for token
	authClient := &auth.ClientCredentialsAuth{
		TokenEndpoint: controlPlane.TokenEndpoint,
		ClientID:      clientID,
		ClientSecret:  clientSecret,
	}

	tokenResp, err := authClient.GetToken()
	if err != nil {
		return fmt.Errorf("failed to get access token: %w", err)
	}

	// 4. Save/update credential in config file
	credentialExists := false
	for idx := range cfg.Credentials {
		if cfg.Credentials[idx].Name == credentialName {
			cfg.Credentials[idx].ClientID = clientID
			cfg.Credentials[idx].ClientSecret = clientSecret
			cfg.Credentials[idx].Token = tokenResp.AccessToken
			credentialExists = true
			break
		}
	}

	if !credentialExists {
		cfg.Credentials = append(cfg.Credentials, configContext.Credential{
			Name:         credentialName,
			ClientID:     clientID,
			ClientSecret: clientSecret,
			Token:        tokenResp.AccessToken,
		})
	}

	// 5. Associate credential with current context
	currentContext.Credentials = credentialName

	// Save updated config
	if err := config.SaveStoredConfig(cfg); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	fmt.Printf("✓ Authentication successful!\n")
	fmt.Printf("Credential '%s' saved and associated with context '%s'\n", credentialName, cfg.CurrentContext)

	return nil
}

func (i *AuthImpl) IsLoggedIn() bool {
	return false
}

func (i *AuthImpl) GetLoginPrompt() string {
	return "login functionality is not supported"
}
