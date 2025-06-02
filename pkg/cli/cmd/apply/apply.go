// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package apply

import (
	"github.com/spf13/cobra"

	"github.com/openchoreo/openchoreo/pkg/cli/common/builder"
	"github.com/openchoreo/openchoreo/pkg/cli/common/constants"
	"github.com/openchoreo/openchoreo/pkg/cli/flags"
	"github.com/openchoreo/openchoreo/pkg/cli/types/api"
)

func NewApplyCmd(impl api.CommandImplementationInterface) *cobra.Command {
	return (&builder.CommandBuilder{
		Command: constants.Apply,
		Flags:   []flags.Flag{flags.ApplyFileFlag},
		RunE: func(fg *builder.FlagGetter) error {
			return impl.Apply(api.ApplyParams{
				FilePath: fg.GetString(flags.ApplyFileFlag),
			})
		},
	}).Build()
}
