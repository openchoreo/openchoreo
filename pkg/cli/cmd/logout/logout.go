// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package logout

import (
	"github.com/spf13/cobra"

	"github.com/openchoreo/openchoreo/pkg/cli/common/builder"
	"github.com/openchoreo/openchoreo/pkg/cli/common/constants"
	"github.com/openchoreo/openchoreo/pkg/cli/types/api"
)

// NewLogoutCmd creates the logout command.
func NewLogoutCmd(impl api.CommandImplementationInterface) *cobra.Command {
	return (&builder.CommandBuilder{
		Command: constants.Logout,
		RunE: func(fg *builder.FlagGetter) error {
			return impl.Logout()
		},
	}).Build()
}
