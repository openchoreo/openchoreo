// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package apply

import (
	"github.com/spf13/cobra"

	internalApply "github.com/openchoreo/openchoreo/internal/occ/cmd/apply"
	"github.com/openchoreo/openchoreo/internal/occ/cmd/login"
	"github.com/openchoreo/openchoreo/pkg/cli/common/auth"
	"github.com/openchoreo/openchoreo/pkg/cli/common/builder"
	"github.com/openchoreo/openchoreo/pkg/cli/common/constants"
	"github.com/openchoreo/openchoreo/pkg/cli/flags"
)

func NewApplyCmd() *cobra.Command {
	return (&builder.CommandBuilder{
		Command: constants.Apply,
		Flags:   []flags.Flag{flags.ApplyFileFlag},
		PreRunE: auth.RequireLogin(login.NewAuthImpl()),
		RunE: func(fg *builder.FlagGetter) error {
			return internalApply.Apply(internalApply.Params{
				FilePath: fg.GetString(flags.ApplyFileFlag),
			})
		},
	}).Build()
}
