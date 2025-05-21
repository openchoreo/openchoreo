/*
 * Copyright Open Choreo Authors
 * SPDX-License-Identifier: Apache-2.0
 * The full text of the Apache license is available in the LICENSE file at
 * the root of the repo.
 */

package builder

import (
	"github.com/spf13/cobra"

	"github.com/openchoreo/openchoreo/pkg/cli/common/constants"
	"github.com/openchoreo/openchoreo/pkg/cli/flags"
)

type CommandBuilder struct {
	Command constants.Command
	Flags   []flags.Flag
	RunE    func(fg *FlagGetter) error
}

type FlagGetter struct {
	cmd  *cobra.Command
	args []string
}

func (f *FlagGetter) GetString(flag flags.Flag) string {
	val, _ := f.cmd.Flags().GetString(flag.Name)
	return val
}

func (f *FlagGetter) GetBool(flag flags.Flag) bool {
	val, _ := f.cmd.Flags().GetBool(flag.Name)
	return val
}

func (f *FlagGetter) GetInt(flag flags.Flag) int {
	val, _ := f.cmd.Flags().GetInt(flag.Name)
	return val
}

func (f *FlagGetter) GetArgs() []string {
	return f.args
}

func (b *CommandBuilder) Build() *cobra.Command {
	cmd := &cobra.Command{
		Use:     b.Command.Use,
		Aliases: b.Command.Aliases,
		Short:   b.Command.Short,
		Long:    b.Command.Long,
		Example: b.Command.Example,
		RunE: func(cmd *cobra.Command, args []string) error {
			fg := &FlagGetter{cmd: cmd, args: args}
			return b.RunE(fg)
		},
	}
	flags.AddFlags(cmd, b.Flags...)
	return cmd
}
