// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// PKCEAuth handles PKCE authorization code flow
type PKCEAuth struct {
	AuthorizationEndpoint string
	TokenEndpoint         string
	ClientID              string
	RedirectURI           string
	CodeVerifier          string
	CodeChallenge         string
	State                 string
}

// PKCETokenResponse represents the OAuth2 token response for PKCE flow
type PKCETokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	Scope        string `json:"scope,omitempty"`
}

// NewPKCEAuth creates a new PKCE auth handler using the OIDC config
func NewPKCEAuth(oidcConfig *OIDCConfig, redirectURI string) (*PKCEAuth, error) {
	pkce := &PKCEAuth{
		AuthorizationEndpoint: oidcConfig.AuthorizationEndpoint,
		TokenEndpoint:         oidcConfig.TokenEndpoint,
		ClientID:              oidcConfig.CLIClientID,
		RedirectURI:           redirectURI,
	}

	if err := pkce.GeneratePKCE(); err != nil {
		return nil, err
	}

	return pkce, nil
}

// GeneratePKCE generates a new PKCE code verifier, challenge, and state
func (p *PKCEAuth) GeneratePKCE() error {
	// Generate 32 bytes of random data for verifier
	verifierBytes := make([]byte, 32)
	if _, err := rand.Read(verifierBytes); err != nil {
		return fmt.Errorf("failed to generate random bytes for verifier: %w", err)
	}

	// URL-safe base64 encode (no padding)
	p.CodeVerifier = base64.RawURLEncoding.EncodeToString(verifierBytes)

	// SHA256 hash the verifier for S256 challenge
	hash := sha256.Sum256([]byte(p.CodeVerifier))
	p.CodeChallenge = base64.RawURLEncoding.EncodeToString(hash[:])

	// Generate 16 bytes (128 bits) of cryptographically secure random data for state
	stateBytes := make([]byte, 16)
	if _, err := rand.Read(stateBytes); err != nil {
		return fmt.Errorf("failed to generate random bytes for state: %w", err)
	}
	p.State = base64.RawURLEncoding.EncodeToString(stateBytes)

	return nil
}

// GetAuthorizationURL builds the authorization URL with PKCE parameters
func (p *PKCEAuth) GetAuthorizationURL() (string, error) {
	params := url.Values{
		"client_id":             {p.ClientID},
		"redirect_uri":          {p.RedirectURI},
		"response_type":         {"code"},
		"code_challenge":        {p.CodeChallenge},
		"code_challenge_method": {"S256"},
		"scope":                 {"openid profile email offline_access"},
		"state":                 {p.State},
	}

	return p.AuthorizationEndpoint + "?" + params.Encode(), nil
}

// ExchangeAuthCode exchanges the authorization code for tokens
func (p *PKCEAuth) ExchangeAuthCode(authCode string) (*PKCETokenResponse, error) {
	data := url.Values{
		"grant_type":    {"authorization_code"},
		"client_id":     {p.ClientID},
		"code":          {authCode},
		"redirect_uri":  {p.RedirectURI},
		"code_verifier": {p.CodeVerifier},
	}

	req, err := http.NewRequest("POST", p.TokenEndpoint, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("failed to create token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send token request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read token response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("token request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var tokenResp PKCETokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return nil, fmt.Errorf("failed to parse token response: %w", err)
	}

	return &tokenResp, nil
}

// RefreshAccessToken refreshes the access token using a refresh token
func RefreshAccessToken(tokenEndpoint, clientID, refreshToken string) (*PKCETokenResponse, error) {
	data := url.Values{
		"grant_type":    {"refresh_token"},
		"client_id":     {clientID},
		"refresh_token": {refreshToken},
	}

	req, err := http.NewRequest("POST", tokenEndpoint, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("failed to create refresh request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send refresh request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read refresh response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("refresh request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var tokenResp PKCETokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return nil, fmt.Errorf("failed to parse refresh response: %w", err)
	}

	return &tokenResp, nil
}
