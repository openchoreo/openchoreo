// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/openchoreo/openchoreo/pkg/cli/common/builder"
	"github.com/openchoreo/openchoreo/pkg/cli/common/constants"
	"github.com/openchoreo/openchoreo/pkg/cli/flags"
	"github.com/openchoreo/openchoreo/pkg/cli/types/api"
)

func NewConfigCmd(impl api.CommandImplementationInterface) *cobra.Command {
	cmd := &cobra.Command{
		Use:     constants.ConfigRoot.Use,
		Short:   constants.ConfigRoot.Short,
		Long:    constants.ConfigRoot.Long,
		Example: constants.ConfigRoot.Example,
	}

	cmd.AddCommand(
		newContextCmd(impl),
		newControlPlaneCmd(impl),
	)
	return cmd
}

func newContextCmd(impl api.CommandImplementationInterface) *cobra.Command {
	cmd := &cobra.Command{
		Use:   constants.ConfigContext.Use,
		Short: constants.ConfigContext.Short,
		Long:  constants.ConfigContext.Long,
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 1 {
				return impl.DescribeContext(api.DescribeContextParams{
					Name: args[0],
				})
			}
			return cmd.Help()
		},
	}

	cmd.AddCommand(
		newContextAddCmd(impl),
		newContextListCmd(impl),
		newContextDeleteCmd(impl),
		newContextUpdateCmd(impl),
		newContextUseCmd(impl),
	)
	return cmd
}

func newContextAddCmd(impl api.CommandImplementationInterface) *cobra.Command {
	cmd := (&builder.CommandBuilder{
		Command: constants.ConfigContextAdd,
		Flags: []flags.Flag{
			flags.ControlPlane,
			flags.Credentials,
			flags.Namespace,
			flags.Project,
			flags.Component,
		},
		RunE: func(fg *builder.FlagGetter) error {
			args := fg.GetArgs()
			if len(args) == 0 {
				return fmt.Errorf("context name is required")
			}
			return impl.AddContext(api.AddContextParams{
				Name:         args[0],
				ControlPlane: fg.GetString(flags.ControlPlane),
				Credentials:  fg.GetString(flags.Credentials),
				Namespace:    fg.GetString(flags.Namespace),
				Project:      fg.GetString(flags.Project),
				Component:    fg.GetString(flags.Component),
			})
		},
	}).Build()

	cmd.Args = cobra.ExactArgs(1)
	_ = cmd.MarkFlagRequired(flags.ControlPlane.Name)
	_ = cmd.MarkFlagRequired(flags.Credentials.Name)

	return cmd
}

func newContextListCmd(impl api.CommandImplementationInterface) *cobra.Command {
	cmd := (&builder.CommandBuilder{
		Command: constants.ConfigContextList,
		RunE: func(fg *builder.FlagGetter) error {
			return impl.ListContexts()
		},
	}).Build()

	cmd.Args = cobra.NoArgs

	return cmd
}

func newContextDeleteCmd(impl api.CommandImplementationInterface) *cobra.Command {
	cmd := (&builder.CommandBuilder{
		Command: constants.ConfigContextDelete,
		RunE: func(fg *builder.FlagGetter) error {
			args := fg.GetArgs()
			if len(args) == 0 {
				return fmt.Errorf("context name is required")
			}
			return impl.DeleteContext(api.DeleteContextParams{
				Name: args[0],
			})
		},
	}).Build()

	cmd.Args = cobra.ExactArgs(1)

	return cmd
}

func newContextUpdateCmd(impl api.CommandImplementationInterface) *cobra.Command {
	cmd := (&builder.CommandBuilder{
		Command: constants.ConfigContextUpdate,
		Flags: []flags.Flag{
			flags.Namespace,
			flags.Project,
		},
		RunE: func(fg *builder.FlagGetter) error {
			args := fg.GetArgs()
			if len(args) == 0 {
				return fmt.Errorf("context name is required")
			}
			return impl.UpdateContext(api.UpdateContextParams{
				Name:      args[0],
				Namespace: fg.GetString(flags.Namespace),
				Project:   fg.GetString(flags.Project),
			})
		},
	}).Build()

	cmd.Args = cobra.ExactArgs(1)

	return cmd
}

func newContextUseCmd(impl api.CommandImplementationInterface) *cobra.Command {
	cmd := (&builder.CommandBuilder{
		Command: constants.ConfigContextUse,
		RunE: func(fg *builder.FlagGetter) error {
			args := fg.GetArgs()
			if len(args) == 0 {
				return fmt.Errorf("context name is required")
			}
			return impl.UseContext(api.UseContextParams{
				Name: args[0],
			})
		},
	}).Build()

	cmd.Args = cobra.ExactArgs(1)

	return cmd
}

func newControlPlaneCmd(impl api.CommandImplementationInterface) *cobra.Command {
	cmd := &cobra.Command{
		Use:   constants.ConfigControlPlane.Use,
		Short: constants.ConfigControlPlane.Short,
		Long:  constants.ConfigControlPlane.Long,
	}

	cmd.AddCommand(
		newControlPlaneAddCmd(impl),
		newControlPlaneListCmd(impl),
		newControlPlaneUpdateCmd(impl),
		newControlPlaneDeleteCmd(impl),
	)
	return cmd
}

func newControlPlaneAddCmd(impl api.CommandImplementationInterface) *cobra.Command {
	cmd := (&builder.CommandBuilder{
		Command: constants.ConfigControlPlaneAdd,
		Flags: []flags.Flag{
			flags.URL,
		},
		RunE: func(fg *builder.FlagGetter) error {
			args := fg.GetArgs()
			if len(args) == 0 {
				return fmt.Errorf("control plane name is required")
			}
			return impl.AddControlPlane(api.AddControlPlaneParams{
				Name: args[0],
				URL:  fg.GetString(flags.URL),
			})
		},
	}).Build()

	cmd.Args = cobra.ExactArgs(1)
	_ = cmd.MarkFlagRequired(flags.URL.Name)

	return cmd
}

func newControlPlaneListCmd(impl api.CommandImplementationInterface) *cobra.Command {
	cmd := (&builder.CommandBuilder{
		Command: constants.ConfigControlPlaneList,
		RunE: func(fg *builder.FlagGetter) error {
			return impl.ListControlPlanes()
		},
	}).Build()

	cmd.Args = cobra.NoArgs

	return cmd
}

func newControlPlaneUpdateCmd(impl api.CommandImplementationInterface) *cobra.Command {
	cmd := (&builder.CommandBuilder{
		Command: constants.ConfigControlPlaneUpdate,
		Flags: []flags.Flag{
			flags.URL,
		},
		RunE: func(fg *builder.FlagGetter) error {
			args := fg.GetArgs()
			if len(args) == 0 {
				return fmt.Errorf("control plane name is required")
			}
			return impl.UpdateControlPlane(api.UpdateControlPlaneParams{
				Name: args[0],
				URL:  fg.GetString(flags.URL),
			})
		},
	}).Build()

	cmd.Args = cobra.ExactArgs(1)

	return cmd
}

func newControlPlaneDeleteCmd(impl api.CommandImplementationInterface) *cobra.Command {
	cmd := (&builder.CommandBuilder{
		Command: constants.ConfigControlPlaneDelete,
		RunE: func(fg *builder.FlagGetter) error {
			args := fg.GetArgs()
			if len(args) == 0 {
				return fmt.Errorf("control plane name is required")
			}
			return impl.DeleteControlPlane(api.DeleteControlPlaneParams{
				Name: args[0],
			})
		},
	}).Build()

	cmd.Args = cobra.ExactArgs(1)

	return cmd
}
