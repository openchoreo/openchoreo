// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package observabilityalertsnotificationchannel

import (
	"github.com/spf13/cobra"

	"github.com/openchoreo/openchoreo/internal/occ/cmd/login"
	oanc "github.com/openchoreo/openchoreo/internal/occ/cmd/observabilityalertsnotificationchannel"
	cliargs "github.com/openchoreo/openchoreo/pkg/cli/common/args"
	"github.com/openchoreo/openchoreo/pkg/cli/common/auth"
	"github.com/openchoreo/openchoreo/pkg/cli/common/builder"
	apiclient "github.com/openchoreo/openchoreo/pkg/cli/common/client"
	"github.com/openchoreo/openchoreo/pkg/cli/common/constants"
	"github.com/openchoreo/openchoreo/pkg/cli/flags"
)

func NewObservabilityAlertsNotificationChannelCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     constants.ObservabilityAlertsNotificationChannel.Use,
		Aliases: constants.ObservabilityAlertsNotificationChannel.Aliases,
		Short:   constants.ObservabilityAlertsNotificationChannel.Short,
		Long:    constants.ObservabilityAlertsNotificationChannel.Long,
	}

	cmd.AddCommand(
		newListCmd(),
		newGetCmd(),
		newDeleteCmd(),
	)

	return cmd
}

func newListCmd() *cobra.Command {
	return (&builder.CommandBuilder{
		Command: constants.ListObservabilityAlertsNotificationChannel,
		Flags:   []flags.Flag{flags.Namespace},
		PreRunE: auth.RequireLogin(login.NewAuthImpl()),
		RunE: func(fg *builder.FlagGetter) error {
			cl, err := apiclient.New()
			if err != nil {
				return err
			}
			return oanc.New(cl).List(oanc.ListParams{
				Namespace: fg.GetString(flags.Namespace),
			})
		},
	}).Build()
}

func newGetCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     constants.GetObservabilityAlertsNotificationChannel.Use,
		Short:   constants.GetObservabilityAlertsNotificationChannel.Short,
		Long:    constants.GetObservabilityAlertsNotificationChannel.Long,
		Example: constants.GetObservabilityAlertsNotificationChannel.Example,
		Args:    cliargs.ExactOneArgWithUsage(),
		PreRunE: auth.RequireLogin(login.NewAuthImpl()),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, err := apiclient.New()
			if err != nil {
				return err
			}
			namespace, _ := cmd.Flags().GetString(flags.Namespace.Name)
			return oanc.New(cl).Get(oanc.GetParams{
				Namespace:   namespace,
				ChannelName: args[0],
			})
		},
	}
	flags.AddFlags(cmd, flags.Namespace)
	return cmd
}

func newDeleteCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     constants.DeleteObservabilityAlertsNotificationChannel.Use,
		Short:   constants.DeleteObservabilityAlertsNotificationChannel.Short,
		Long:    constants.DeleteObservabilityAlertsNotificationChannel.Long,
		Example: constants.DeleteObservabilityAlertsNotificationChannel.Example,
		Args:    cliargs.ExactOneArgWithUsage(),
		PreRunE: auth.RequireLogin(login.NewAuthImpl()),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, err := apiclient.New()
			if err != nil {
				return err
			}
			namespace, _ := cmd.Flags().GetString(flags.Namespace.Name)
			return oanc.New(cl).Delete(oanc.DeleteParams{
				Namespace:   namespace,
				ChannelName: args[0],
			})
		},
	}
	flags.AddFlags(cmd, flags.Namespace)
	return cmd
}
