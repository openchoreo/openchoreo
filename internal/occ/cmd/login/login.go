// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package login

import (
	"fmt"
	"os"

	"github.com/openchoreo/openchoreo/internal/occ/auth"
	"github.com/openchoreo/openchoreo/internal/occ/browser"
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
	// Default to PKCE flow for interactive login
	return i.loginWithPKCE(params)
}

func (i *AuthImpl) loginWithClientCredentials(params api.LoginParams) error {
	// Get client ID/secret from params or environment variables
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

	currentContext, err := config.GetCurrentContext()
	if err != nil {
		return fmt.Errorf("failed to get current context: %w", err)
	}
	// Use existing credential name if none specified
	credentialName := params.CredentialName
	if credentialName == "" {
		credentialName = currentContext.Credentials
	}

	if credentialName == "" {
		fmt.Println("No credential name specified")
		return fmt.Errorf("credential name must be specified when no existing credential is associated with the current context")
	}

	controlPlane, err := config.GetCurrentControlPlane()
	if err != nil {
		return fmt.Errorf("failed to get control plane: %w", err)
	}

	cfg, err := config.LoadStoredConfig()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	oidcConfig, err := auth.FetchOIDCConfig(controlPlane.URL)
	if err != nil {
		return fmt.Errorf("failed to fetch OIDC config from API: %w", err)
	}

	// Check if security is disabled on the server
	if !oidcConfig.SecurityEnabled {
		fmt.Println("Security is disabled on the server. Authentication is not required.")
		return nil
	}

	authClient := &auth.ClientCredentialsAuth{
		TokenEndpoint: oidcConfig.TokenEndpoint,
		ClientID:      clientID,
		ClientSecret:  clientSecret,
	}

	tokenResp, err := authClient.GetToken()
	if err != nil {
		return fmt.Errorf("failed to get access token: %w", err)
	}

	// Save/update credential in config file
	credentialExists := false
	for idx := range cfg.Credentials {
		if cfg.Credentials[idx].Name == credentialName {
			cfg.Credentials[idx].ClientID = clientID
			cfg.Credentials[idx].ClientSecret = clientSecret
			cfg.Credentials[idx].Token = tokenResp.AccessToken
			cfg.Credentials[idx].AuthMethod = "client_credentials"
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
			AuthMethod:   "client_credentials",
		})
	}
	currentContext.Credentials = credentialName

	// Save updated config
	if err := config.SaveStoredConfig(cfg); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	fmt.Printf("✓ Authentication successful!\n")
	fmt.Printf("Credential '%s' saved and associated with context '%s'\n", credentialName, cfg.CurrentContext)

	return nil
}

func (i *AuthImpl) loginWithPKCE(params api.LoginParams) error {
	currentContext, err := config.GetCurrentContext()
	if err != nil {
		return fmt.Errorf("failed to get current context: %w", err)
	}

	// Use existing credential name if none specified
	credentialName := params.CredentialName
	if credentialName == "" {
		credentialName = currentContext.Credentials
	}
	if credentialName == "" {
		fmt.Printf("No credential name specified")
		return fmt.Errorf("credential name must be specified when no existing credential is associated with the current context")
	}

	controlPlane, err := config.GetCurrentControlPlane()
	if err != nil {
		return fmt.Errorf("failed to get control plane: %w", err)
	}

	cfg, err := config.LoadStoredConfig()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	fmt.Println("Fetching OIDC configuration...")
	oidcConfig, err := auth.FetchOIDCConfig(controlPlane.URL)
	if err != nil {
		return fmt.Errorf("failed to fetch OIDC config: %w", err)
	}

	// Check if security is disabled on the server
	if !oidcConfig.SecurityEnabled {
		fmt.Println("Security is disabled on the server. Authentication is not required.")
		return nil
	}

	fmt.Printf("✓ OIDC configuration retrieved\n")

	redirectURI := fmt.Sprintf("http://127.0.0.1:%d%s", auth.CallbackPort, auth.CallbackPath)

	pkceAuth, err := auth.NewPKCEAuth(oidcConfig, redirectURI)
	if err != nil {
		return fmt.Errorf("failed to initialize PKCE auth: %w", err)
	}

	authURL, err := pkceAuth.GetAuthorizationURL()
	if err != nil {
		return fmt.Errorf("failed to generate authorization URL: %w", err)
	}

	fmt.Println("\nOpening browser for authentication...")
	fmt.Printf("If the browser doesn't open, visit:\n%s\n\n", authURL)

	if err := browser.Open(authURL); err != nil {
		fmt.Printf("Warning: failed to open browser: %v\n", err)
		fmt.Println("Please open the URL above manually.")
	}

	fmt.Println("Waiting for authentication (timeout: 5 minutes)...")
	authCode, err := auth.ListenForAuthCode(pkceAuth.State, auth.AuthTimeout)
	if err != nil {
		return fmt.Errorf("authentication failed: %w", err)
	}

	fmt.Println("Exchanging authorization code for tokens...")
	tokenResp, err := pkceAuth.ExchangeAuthCode(authCode)
	if err != nil {
		return fmt.Errorf("failed to exchange auth code: %w", err)
	}

	credentialExists := false
	for idx := range cfg.Credentials {
		if cfg.Credentials[idx].Name == credentialName {
			cfg.Credentials[idx].Token = tokenResp.AccessToken
			cfg.Credentials[idx].RefreshToken = tokenResp.RefreshToken
			cfg.Credentials[idx].AuthMethod = "authorization_code"
			cfg.Credentials[idx].ClientID = oidcConfig.ClientID
			cfg.Credentials[idx].ClientSecret = ""
			credentialExists = true
			break
		}
	}

	if !credentialExists {
		cfg.Credentials = append(cfg.Credentials, configContext.Credential{
			Name:         credentialName,
			ClientID:     oidcConfig.ClientID,
			Token:        tokenResp.AccessToken,
			RefreshToken: tokenResp.RefreshToken,
			AuthMethod:   "authorization_code",
		})
	}

	currentContext.Credentials = credentialName

	if err := config.SaveStoredConfig(cfg); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	fmt.Printf("\n✓ Authentication successful!\n")
	fmt.Printf("Credential '%s' saved and associated with context '%s'\n",
		credentialName, cfg.CurrentContext)

	return nil
}

func (i *AuthImpl) IsLoggedIn() bool {
	// Check if security is disabled on the server
	controlPlane, err := config.GetCurrentControlPlane()

	if err != nil {
		return false
	}

	if controlPlane != nil {
		oidcConfig, err := auth.FetchOIDCConfig(controlPlane.URL)
		if err != nil {
			return false
		}
		if !oidcConfig.SecurityEnabled {
			return true // Always "logged in" when security is disabled
		}
	}

	credential, err := config.GetCurrentCredential()
	if err != nil {
		return false
	}

	if credential.Token == "" {
		return false
	}

	if auth.IsTokenExpired(credential.Token) {
		if _, err := auth.RefreshToken(); err != nil {
			return false
		}
	}
	return true
}

func (i *AuthImpl) GetLoginPrompt() string {
	return `Authentication required. Please login using one of the following methods:

   Interactive login (browser-based):
   occ login

   Client credentials (service accounts):
   occ login --client-credentials --client-id <client-id> --client-secret <client-secret>

   Or set environment variables for client credentials:
   export OCC_CLIENT_ID=<client-id>
   export OCC_CLIENT_SECRET=<client-secret>
   occ login --client-credentials

For more information, run: occ login --help`
}
