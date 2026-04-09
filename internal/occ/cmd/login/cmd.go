// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package login

import (
	"github.com/spf13/cobra"
)

// LoginParams defines parameters for the login command.
type LoginParams struct {
	ClientCredentials bool
	ClientID          string
	ClientSecret      string
	CredentialName    string
}

func NewLoginCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "login",
		Short: "Login to OpenChoreo CLI",
		Long:  "Login to OpenChoreo CLI",
		RunE: func(cmd *cobra.Command, args []string) error {
			clientCreds, _ := cmd.Flags().GetBool("client-credentials")
			clientID, _ := cmd.Flags().GetString("client-id")
			clientSecret, _ := cmd.Flags().GetString("client-secret")
			credentialName, _ := cmd.Flags().GetString("credential")
			return NewAuthImpl().Login(LoginParams{
				ClientCredentials: clientCreds,
				ClientID:          clientID,
				ClientSecret:      clientSecret,
				CredentialName:    credentialName,
			})
		},
	}
	cmd.Flags().Bool("client-credentials", false, "Use OAuth2 client credentials flow for authentication")
	cmd.Flags().String("client-id", "", "OAuth2 client ID for service account authentication")
	cmd.Flags().String("client-secret", "", "OAuth2 client secret for service account authentication")
	cmd.Flags().String("credential", "", "Name to save the credential as in config")
	return cmd
}
