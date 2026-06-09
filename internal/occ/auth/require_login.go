// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package auth

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/openchoreo/openchoreo/internal/occ/cmd/config"
)

const loginPrompt = `Authentication required. Please login using one of the following methods:

   Interactive login (browser-based):
   occ login

   Client credentials (service accounts):
   occ login --client-credentials --client-id <client-id> --client-secret <client-secret>

   Or set environment variables for client credentials:
   export OCC_CLIENT_ID=<client-id>
   export OCC_CLIENT_SECRET=<client-secret>
   occ login --client-credentials

For more information, run: occ login --help`

// RequireLogin returns a cobra PreRunE function that checks whether the user is
// authenticated before allowing the command to run.
func RequireLogin() func(cmd *cobra.Command, args []string) error {
	return func(cmd *cobra.Command, args []string) error {
		if !IsLoggedIn() {
			return fmt.Errorf("%s", loginPrompt)
		}
		return nil
	}
}

// IsLoggedIn reports whether the current user has a valid, non-expired token.
// It returns true when security is disabled on the control plane.
func IsLoggedIn() bool {
	controlPlane, err := config.GetCurrentControlPlane()
	if err != nil {
		return false
	}

	if controlPlane != nil {
		oidcConfig, err := FetchOIDCConfig(controlPlane.URL)
		if err == nil && !oidcConfig.SecurityEnabled {
			return true
		}
		// If OIDC discovery fails, fall through to check stored token.
	}

	credential, err := config.GetCurrentCredential()
	if err != nil {
		return false
	}

	if credential.Token == "" {
		return false
	}

	if IsTokenExpired(credential.Token) {
		if _, err := RefreshToken(); err != nil {
			return false
		}
	}
	return true
}
