/*
 * Copyright (c) 2025, WSO2 Inc. (http://www.wso2.org) All Rights Reserved.
 *
 * WSO2 Inc. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License.
 * You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied. See the License for the
 * specific language governing permissions and limitations
 * under the License.
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
