// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package login

import (
	"github.com/spf13/cobra"

	"github.com/openchoreo/openchoreo/pkg/cli/common/builder"
	"github.com/openchoreo/openchoreo/pkg/cli/common/constants"
	"github.com/openchoreo/openchoreo/pkg/cli/flags"
	"github.com/openchoreo/openchoreo/pkg/cli/types/api"
)

// NewLoginCmd creates the login command.
func NewLoginCmd(impl api.CommandImplementationInterface) *cobra.Command {
	return (&builder.CommandBuilder{
		Command: constants.Login,
		Flags: []flags.Flag{
			flags.Kubeconfig,
			flags.KubeContext,
		},
		RunE: func(fg *builder.FlagGetter) error {
			return impl.Login(api.LoginParams{
				KubeconfigPath: fg.GetString(flags.Kubeconfig),
				Kubecontext:    fg.GetString(flags.KubeContext),
			})
		},
	}).Build()
}
