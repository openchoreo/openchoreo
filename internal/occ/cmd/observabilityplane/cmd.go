// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package observabilityplane

import (
	"github.com/spf13/cobra"

	"github.com/openchoreo/openchoreo/internal/occ/auth"
	"github.com/openchoreo/openchoreo/internal/occ/cmdutil"
	"github.com/openchoreo/openchoreo/internal/occ/flags"
	"github.com/openchoreo/openchoreo/internal/occ/resources/client"
)

func NewObservabilityPlaneCmd(f client.NewClientFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "observabilityplane",
		Aliases: []string{"op", "observabilityplanes"},
		Short:   "Manage observability planes",
		Long:    `Manage observability planes for OpenChoreo.`,
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
		Short: "List observability planes",
		Long:  `List all observability planes in a namespace.`,
		Example: `  # List all observability planes in a namespace
  occ observabilityplane list --namespace acme-corp`,
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
		Use:   "get [OBSERVABILITYPLANE_NAME]",
		Short: "Get an observability plane",
		Long:  `Get an observability plane and display its details in YAML format.`,
		Example: `  # Get an observability plane
  occ observabilityplane get primary-observabilityplane --namespace acme-corp`,
		Args:    cmdutil.ExactOneArgWithUsage(),
		PreRunE: auth.RequireLogin(),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, err := f()
			if err != nil {
				return err
			}
			return New(cl).Get(GetParams{
				Namespace:              flags.GetNamespace(cmd),
				ObservabilityPlaneName: args[0],
			})
		},
	}
	flags.AddNamespace(cmd)
	return cmd
}

func newDeleteCmd(f client.NewClientFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete [OBSERVABILITYPLANE_NAME]",
		Short: "Delete an observability plane",
		Long:  `Delete an observability plane by name.`,
		Example: `  # Delete an observability plane
  occ observabilityplane delete primary-observabilityplane --namespace acme-corp`,
		Args:    cmdutil.ExactOneArgWithUsage(),
		PreRunE: auth.RequireLogin(),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, err := f()
			if err != nil {
				return err
			}
			return New(cl).Delete(DeleteParams{
				Namespace:              flags.GetNamespace(cmd),
				ObservabilityPlaneName: args[0],
			})
		},
	}
	flags.AddNamespace(cmd)
	return cmd
}
