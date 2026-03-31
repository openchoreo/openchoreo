// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package auth

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/openchoreo/openchoreo/pkg/cli/types/api"
)

// RequireLogin returns a PreRunE handler that checks if the user is logged in.
// Commands that need authentication should add this as their PreRunE.
func RequireLogin(impl api.LoginAPI) func(cmd *cobra.Command, args []string) error {
	return func(cmd *cobra.Command, args []string) error {
		if !impl.IsLoggedIn() {
			return fmt.Errorf("%s", impl.GetLoginPrompt())
		}
		return nil
	}
}
