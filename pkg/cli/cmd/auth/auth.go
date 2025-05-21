/*
 * Copyright OpenChoreo Authors
 * SPDX-License-Identifier: Apache-2.0
 * The full text of the Apache license is available in the LICENSE file at
 * the root of the repo.
 */

package auth

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/openchoreo/openchoreo/pkg/cli/types/api"
)

// CheckLoginStatus ensures the user is logged in before executing any command.
func CheckLoginStatus(impl api.CommandImplementationInterface) func(cmd *cobra.Command, args []string) error {
	return func(cmd *cobra.Command, args []string) error {
		if cmd.Name() != "login" && cmd.Name() != "logout" && !impl.IsLoggedIn() {
			fmt.Println(impl.GetLoginPrompt())
			os.Exit(1)
		}
		return nil
	}
}
