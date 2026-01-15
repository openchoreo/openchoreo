// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package login

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"

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

// getTokenEndpointFromAPI fetches the OIDC configuration from the API server
func (i *AuthImpl) getTokenEndpointFromAPI(apiURL string) (string, error) {
	req, err := http.NewRequest(
		http.MethodGet,
		apiURL+"/api/v1/oidc-config",
		nil,
	)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	// Add the header here
	req.Header.Set("X-Use-OpenAPI", "true")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to fetch OIDC config: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("OIDC config request failed with status: %d", resp.StatusCode)
	}

	var oidcConfig struct {
		TokenEndpoint string `json:"token_endpoint"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&oidcConfig); err != nil {
		return "", fmt.Errorf("failed to decode OIDC config: %w", err)
	}

	if oidcConfig.TokenEndpoint == "" {
		return "", fmt.Errorf("token endpoint not found in OIDC config response")
	}

	return oidcConfig.TokenEndpoint, nil
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

	// Update control plane URL if specified in params
	if params.URL != "" {
		fmt.Printf("Updating control plane URL to: %s\n", params.URL)
		for idx := range cfg.ControlPlanes {
			if cfg.ControlPlanes[idx].Name == controlPlane.Name {
				cfg.ControlPlanes[idx].URL = params.URL
				controlPlane = &cfg.ControlPlanes[idx]
				break
			}
		}
	}

	// Always fetch token endpoint from API
	tokenEndpoint, err := i.getTokenEndpointFromAPI(controlPlane.URL)
	if err != nil {
		return fmt.Errorf("failed to fetch token endpoint from API: %w", err)
	}
	fmt.Printf("✓ Token endpoint discovered: %s\n", tokenEndpoint)

	// 3. Exchange credentials for token
	authClient := &auth.ClientCredentialsAuth{
		TokenEndpoint: tokenEndpoint,
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

func (i *AuthImpl) loginWithPKCE(params api.LoginParams) error {
	credentialName := params.CredentialName
	if credentialName == "" {
		credentialName = "default"
	}

	// 1. Load config to get current context's control plane
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

	// Update control plane URL if specified in params
	if params.URL != "" {
		fmt.Printf("Updating control plane URL to: %s\n", params.URL)
		for idx := range cfg.ControlPlanes {
			if cfg.ControlPlanes[idx].Name == controlPlane.Name {
				cfg.ControlPlanes[idx].URL = params.URL
				controlPlane = &cfg.ControlPlanes[idx]
				break
			}
		}
	}

	// 2. Fetch OIDC configuration (both auth and token endpoints, plus client ID)
	fmt.Println("Fetching OIDC configuration...")
	oidcConfig, err := auth.FetchOIDCConfig(controlPlane.URL)
	if err != nil {
		return fmt.Errorf("failed to fetch OIDC config: %w", err)
	}
	fmt.Printf("✓ OIDC configuration retrieved\n")

	// 3. Construct redirect URI using fixed callback port
	redirectURI := fmt.Sprintf("http://127.0.0.1:%d%s", auth.CallbackPort, auth.CallbackPath)

	// 4. Create PKCE auth handler and generate challenge
	pkceAuth, err := auth.NewPKCEAuth(oidcConfig, redirectURI)
	if err != nil {
		return fmt.Errorf("failed to initialize PKCE auth: %w", err)
	}

	// 5. Get authorization URL
	authURL, err := pkceAuth.GetAuthorizationURL()
	if err != nil {
		return fmt.Errorf("failed to generate authorization URL: %w", err)
	}

	// 6. Open browser
	fmt.Println("\nOpening browser for authentication...")
	fmt.Printf("If the browser doesn't open, visit:\n%s\n\n", authURL)

	if err := browser.Open(authURL); err != nil {
		fmt.Printf("Warning: failed to open browser: %v\n", err)
		fmt.Println("Please open the URL above manually.")
	}

	// 7. Wait for callback with auth code
	fmt.Println("Waiting for authentication (timeout: 5 minutes)...")
	authCode, err := auth.ListenForAuthCode(pkceAuth.State, auth.AuthTimeout)
	if err != nil {
		return fmt.Errorf("authentication failed: %w", err)
	}

	// 8. Exchange auth code for tokens
	fmt.Println("Exchanging authorization code for tokens...")
	tokenResp, err := pkceAuth.ExchangeAuthCode(authCode)
	if err != nil {
		return fmt.Errorf("failed to exchange auth code: %w", err)
	}

	// 9. Calculate token expiry time
	expiryTime := time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second)

	// 10. Save tokens to config
	credentialExists := false
	for idx := range cfg.Credentials {
		if cfg.Credentials[idx].Name == credentialName {
			cfg.Credentials[idx].Token = tokenResp.AccessToken
			cfg.Credentials[idx].RefreshToken = tokenResp.RefreshToken
			cfg.Credentials[idx].TokenExpiry = expiryTime.Format(time.RFC3339)
			cfg.Credentials[idx].AuthMethod = "pkce"
			cfg.Credentials[idx].ClientID = oidcConfig.CLIClientID
			cfg.Credentials[idx].ClientSecret = "" // Clear any existing secret
			credentialExists = true
			break
		}
	}

	if !credentialExists {
		cfg.Credentials = append(cfg.Credentials, configContext.Credential{
			Name:         credentialName,
			ClientID:     oidcConfig.CLIClientID,
			Token:        tokenResp.AccessToken,
			RefreshToken: tokenResp.RefreshToken,
			TokenExpiry:  expiryTime.Format(time.RFC3339),
			AuthMethod:   "pkce",
		})
	}

	// Associate credential with current context
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
	cfg, err := config.LoadStoredConfig()
	if err != nil {
		return false
	}

	if cfg.CurrentContext == "" {
		return false
	}

	// Find current context
	var currentContext *configContext.Context
	for idx := range cfg.Contexts {
		if cfg.Contexts[idx].Name == cfg.CurrentContext {
			currentContext = &cfg.Contexts[idx]
			break
		}
	}

	if currentContext == nil || currentContext.Credentials == "" {
		return false
	}

	// Find credential
	var credential *configContext.Credential
	for idx := range cfg.Credentials {
		if cfg.Credentials[idx].Name == currentContext.Credentials {
			credential = &cfg.Credentials[idx]
			break
		}
	}

	if credential == nil || credential.Token == "" {
		return false
	}

	// Check token expiry for PKCE tokens
	if credential.TokenExpiry != "" {
		expiryTime, err := time.Parse(time.RFC3339, credential.TokenExpiry)
		if err == nil && time.Now().After(expiryTime.Add(-1*time.Minute)) {
			// Token expired or expiring soon - attempt refresh
			if credential.RefreshToken != "" && credential.AuthMethod == "pkce" {
				if err := i.refreshPKCEToken(cfg, credential, currentContext); err != nil {
					return false
				}
				return true
			}
			// For client credentials, the API client handles refresh
			if credential.ClientID != "" && credential.ClientSecret != "" {
				return true // Let API client refresh on demand
			}
			return false
		}
	}

	return true
}

func (i *AuthImpl) refreshPKCEToken(cfg *configContext.StoredConfig, credential *configContext.Credential, currentContext *configContext.Context) error {
	// Find control plane for token endpoint
	var controlPlane *configContext.ControlPlane
	for idx := range cfg.ControlPlanes {
		if cfg.ControlPlanes[idx].Name == currentContext.ControlPlane {
			controlPlane = &cfg.ControlPlanes[idx]
			break
		}
	}

	if controlPlane == nil {
		return fmt.Errorf("control plane not found")
	}

	// Fetch OIDC config to get token endpoint
	oidcConfig, err := auth.FetchOIDCConfig(controlPlane.URL)
	if err != nil {
		return fmt.Errorf("failed to fetch OIDC config: %w", err)
	}

	// Refresh the token
	tokenResp, err := auth.RefreshAccessToken(
		oidcConfig.TokenEndpoint,
		credential.ClientID,
		credential.RefreshToken,
	)
	if err != nil {
		return fmt.Errorf("failed to refresh token: %w", err)
	}

	// Update credential
	credential.Token = tokenResp.AccessToken
	if tokenResp.RefreshToken != "" {
		credential.RefreshToken = tokenResp.RefreshToken
	}
	expiryTime := time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second)
	credential.TokenExpiry = expiryTime.Format(time.RFC3339)

	return config.SaveStoredConfig(cfg)
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
