// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package observabilityalertsnotificationchannel

import (
	"github.com/spf13/cobra"

	"github.com/openchoreo/openchoreo/internal/occ/cmd/login"
	oanc "github.com/openchoreo/openchoreo/internal/occ/cmd/observabilityalertsnotificationchannel"
	"github.com/openchoreo/openchoreo/pkg/cli/common/auth"
	"github.com/openchoreo/openchoreo/pkg/cli/common/builder"
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
			return oanc.New().List(oanc.ListParams{
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
		Args:    cobra.ExactArgs(1),
		PreRunE: auth.RequireLogin(login.NewAuthImpl()),
		RunE: func(cmd *cobra.Command, args []string) error {
			namespace, _ := cmd.Flags().GetString(flags.Namespace.Name)
			return oanc.New().Get(oanc.GetParams{
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
		Args:    cobra.ExactArgs(1),
		PreRunE: auth.RequireLogin(login.NewAuthImpl()),
		RunE: func(cmd *cobra.Command, args []string) error {
			namespace, _ := cmd.Flags().GetString(flags.Namespace.Name)
			return oanc.New().Delete(oanc.DeleteParams{
				Namespace:   namespace,
				ChannelName: args[0],
			})
		},
	}
	flags.AddFlags(cmd, flags.Namespace)
	return cmd
}
