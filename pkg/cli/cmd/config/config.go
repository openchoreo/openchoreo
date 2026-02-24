// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/openchoreo/openchoreo/internal/occ/cmd/config"
	"github.com/openchoreo/openchoreo/pkg/cli/common/builder"
	"github.com/openchoreo/openchoreo/pkg/cli/common/constants"
	"github.com/openchoreo/openchoreo/pkg/cli/flags"
)

func NewConfigCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     constants.ConfigRoot.Use,
		Short:   constants.ConfigRoot.Short,
		Long:    constants.ConfigRoot.Long,
		Example: constants.ConfigRoot.Example,
	}

	cmd.AddCommand(
		newContextCmd(),
		newControlPlaneCmd(),
		newCredentialsCmd(),
	)
	return cmd
}

func newContextCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   constants.ConfigContext.Use,
		Short: constants.ConfigContext.Short,
		Long:  constants.ConfigContext.Long,
	}

	cmd.AddCommand(
		newContextAddCmd(),
		newContextListCmd(),
		newContextDeleteCmd(),
		newContextUpdateCmd(),
		newContextUseCmd(),
	)
	return cmd
}

func newContextAddCmd() *cobra.Command {
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
			return config.New().AddContext(config.AddContextParams{
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

func newContextListCmd() *cobra.Command {
	cmd := (&builder.CommandBuilder{
		Command: constants.ConfigContextList,
		RunE: func(fg *builder.FlagGetter) error {
			return config.New().ListContexts()
		},
	}).Build()

	cmd.Args = cobra.NoArgs

	return cmd
}

func newContextDeleteCmd() *cobra.Command {
	cmd := (&builder.CommandBuilder{
		Command: constants.ConfigContextDelete,
		RunE: func(fg *builder.FlagGetter) error {
			args := fg.GetArgs()
			if len(args) == 0 {
				return fmt.Errorf("context name is required")
			}
			return config.New().DeleteContext(config.DeleteContextParams{
				Name: args[0],
			})
		},
	}).Build()

	cmd.Args = cobra.ExactArgs(1)

	return cmd
}

func newContextUpdateCmd() *cobra.Command {
	cmd := (&builder.CommandBuilder{
		Command: constants.ConfigContextUpdate,
		Flags: []flags.Flag{
			flags.Namespace,
			flags.Project,
			flags.Component,
			flags.ControlPlane,
			flags.Credentials,
		},
		RunE: func(fg *builder.FlagGetter) error {
			args := fg.GetArgs()
			if len(args) == 0 {
				return fmt.Errorf("context name is required")
			}
			return config.New().UpdateContext(config.UpdateContextParams{
				Name:         args[0],
				Namespace:    fg.GetString(flags.Namespace),
				Project:      fg.GetString(flags.Project),
				Component:    fg.GetString(flags.Component),
				ControlPlane: fg.GetString(flags.ControlPlane),
				Credentials:  fg.GetString(flags.Credentials),
			})
		},
	}).Build()

	cmd.Args = cobra.ExactArgs(1)

	return cmd
}

func newContextUseCmd() *cobra.Command {
	cmd := (&builder.CommandBuilder{
		Command: constants.ConfigContextUse,
		RunE: func(fg *builder.FlagGetter) error {
			args := fg.GetArgs()
			if len(args) == 0 {
				return fmt.Errorf("context name is required")
			}
			return config.New().UseContext(config.UseContextParams{
				Name: args[0],
			})
		},
	}).Build()

	cmd.Args = cobra.ExactArgs(1)

	return cmd
}

func newControlPlaneCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   constants.ConfigControlPlane.Use,
		Short: constants.ConfigControlPlane.Short,
		Long:  constants.ConfigControlPlane.Long,
	}

	cmd.AddCommand(
		newControlPlaneAddCmd(),
		newControlPlaneListCmd(),
		newControlPlaneUpdateCmd(),
		newControlPlaneDeleteCmd(),
	)
	return cmd
}

func newControlPlaneAddCmd() *cobra.Command {
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
			return config.New().AddControlPlane(config.AddControlPlaneParams{
				Name: args[0],
				URL:  fg.GetString(flags.URL),
			})
		},
	}).Build()

	cmd.Args = cobra.ExactArgs(1)
	_ = cmd.MarkFlagRequired(flags.URL.Name)

	return cmd
}

func newControlPlaneListCmd() *cobra.Command {
	cmd := (&builder.CommandBuilder{
		Command: constants.ConfigControlPlaneList,
		RunE: func(fg *builder.FlagGetter) error {
			return config.New().ListControlPlanes()
		},
	}).Build()

	cmd.Args = cobra.NoArgs

	return cmd
}

func newControlPlaneUpdateCmd() *cobra.Command {
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
			return config.New().UpdateControlPlane(config.UpdateControlPlaneParams{
				Name: args[0],
				URL:  fg.GetString(flags.URL),
			})
		},
	}).Build()

	cmd.Args = cobra.ExactArgs(1)

	return cmd
}

func newControlPlaneDeleteCmd() *cobra.Command {
	cmd := (&builder.CommandBuilder{
		Command: constants.ConfigControlPlaneDelete,
		RunE: func(fg *builder.FlagGetter) error {
			args := fg.GetArgs()
			if len(args) == 0 {
				return fmt.Errorf("control plane name is required")
			}
			return config.New().DeleteControlPlane(config.DeleteControlPlaneParams{
				Name: args[0],
			})
		},
	}).Build()

	cmd.Args = cobra.ExactArgs(1)

	return cmd
}

func newCredentialsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   constants.ConfigCredentials.Use,
		Short: constants.ConfigCredentials.Short,
		Long:  constants.ConfigCredentials.Long,
	}

	cmd.AddCommand(
		newCredentialsAddCmd(),
		newCredentialsListCmd(),
		newCredentialsDeleteCmd(),
	)
	return cmd
}

func newCredentialsAddCmd() *cobra.Command {
	cmd := (&builder.CommandBuilder{
		Command: constants.ConfigCredentialsAdd,
		RunE: func(fg *builder.FlagGetter) error {
			args := fg.GetArgs()
			if len(args) == 0 {
				return fmt.Errorf("credentials name is required")
			}
			return config.New().AddCredentials(config.AddCredentialsParams{
				Name: args[0],
			})
		},
	}).Build()

	cmd.Args = cobra.ExactArgs(1)

	return cmd
}

func newCredentialsListCmd() *cobra.Command {
	cmd := (&builder.CommandBuilder{
		Command: constants.ConfigCredentialsList,
		RunE: func(fg *builder.FlagGetter) error {
			return config.New().ListCredentials()
		},
	}).Build()

	cmd.Args = cobra.NoArgs

	return cmd
}

func newCredentialsDeleteCmd() *cobra.Command {
	cmd := (&builder.CommandBuilder{
		Command: constants.ConfigCredentialsDelete,
		RunE: func(fg *builder.FlagGetter) error {
			args := fg.GetArgs()
			if len(args) == 0 {
				return fmt.Errorf("credentials name is required")
			}
			return config.New().DeleteCredentials(config.DeleteCredentialsParams{
				Name: args[0],
			})
		},
	}).Build()

	cmd.Args = cobra.ExactArgs(1)

	return cmd
}
