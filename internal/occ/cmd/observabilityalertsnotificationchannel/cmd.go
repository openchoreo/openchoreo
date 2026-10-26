// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package observabilityalertsnotificationchannel

import (
	"github.com/spf13/cobra"

	"github.com/openchoreo/openchoreo/internal/occ/auth"
	"github.com/openchoreo/openchoreo/internal/occ/cmdutil"
	"github.com/openchoreo/openchoreo/internal/occ/flags"
	"github.com/openchoreo/openchoreo/internal/occ/resources/client"
)

func NewObservabilityAlertsNotificationChannelCmd(f client.NewClientFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "observabilityalertsnotificationchannel",
		Aliases: []string{"oanc", "obsnotificationchannel", "observabilityalertsnotificationchannels"},
		Short:   "Manage observability alerts notification channels",
		Long: `Manage observability alerts notification channels for OpenChoreo.

A channel references its secret values, such as SMTP credentials and webhook header
values, by the name of a SecretReference and one of its keys. Create them in the
secret store of the channel's observability plane with
'occ secret create --target-plane ObservabilityPlane/<name>'. Referencing a
Kubernetes Secret by name still works but is deprecated; such channels show
DeprecatedKubernetesSecret in the SECRETS column of 'list'.`,
	}
	cmd.AddCommand(
		newListCmd(f),
		newGetCmd(f),
		newDeleteCmd(f),
	)
	return cmd
}

func newListCmd(f client.NewClientFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List observability alerts notification channels",
		Long:  `List all observability alerts notification channels in a namespace.`,
		Example: `  # List all observability alerts notification channels
  occ observabilityalertsnotificationchannel list --namespace acme-corp`,
		PreRunE: auth.RequireLogin(),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, err := f()
			if err != nil {
				return err
			}
			return New(cl).List(ListParams{
				Namespace: flags.GetNamespace(cmd),
			})
		},
	}
	flags.AddNamespace(cmd)
	return cmd
}

func newGetCmd(f client.NewClientFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get [CHANNEL_NAME]",
		Short: "Get an observability alerts notification channel",
		Long:  `Get an observability alerts notification channel and display its details in YAML format.`,
		Example: `  # Get an observability alerts notification channel
  occ observabilityalertsnotificationchannel get my-channel --namespace acme-corp`,
		Args:    cmdutil.ExactOneArgWithUsage(),
		PreRunE: auth.RequireLogin(),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, err := f()
			if err != nil {
				return err
			}
			return New(cl).Get(GetParams{
				Namespace:   flags.GetNamespace(cmd),
				ChannelName: args[0],
			})
		},
	}
	flags.AddNamespace(cmd)
	return cmd
}

func newDeleteCmd(f client.NewClientFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete [CHANNEL_NAME]",
		Short: "Delete an observability alerts notification channel",
		Long:  `Delete an observability alerts notification channel by name.`,
		Example: `  # Delete an observability alerts notification channel
  occ observabilityalertsnotificationchannel delete my-channel --namespace acme-corp`,
		Args:    cmdutil.ExactOneArgWithUsage(),
		PreRunE: auth.RequireLogin(),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, err := f()
			if err != nil {
				return err
			}
			return New(cl).Delete(DeleteParams{
				Namespace:   flags.GetNamespace(cmd),
				ChannelName: args[0],
			})
		},
	}
	flags.AddNamespace(cmd)
	return cmd
}
