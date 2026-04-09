// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package logout

import (
	"github.com/spf13/cobra"
)

func NewLogoutCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "logout",
		Short: "Logout from OpenChoreo CLI",
		RunE: func(cmd *cobra.Command, args []string) error {
			return NewLogoutImpl().Logout()
		},
	}
}
