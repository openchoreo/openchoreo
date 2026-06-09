// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package secretreference

import (
	"github.com/spf13/cobra"

	"github.com/openchoreo/openchoreo/internal/occ/auth"
	"github.com/openchoreo/openchoreo/internal/occ/cmdutil"
	"github.com/openchoreo/openchoreo/internal/occ/flags"
	"github.com/openchoreo/openchoreo/internal/occ/resources/client"
)

func NewSecretReferenceCmd(f client.NewClientFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "secretreference",
		Aliases: []string{"sr", "secretreferences", "secretref"},
		Short:   "Manage secret references",
		Long:    `Manage secret references for OpenChoreo.`,
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
		Short: "List secret references",
		Long:  `List all secret references in a namespace.`,
		Example: `  # List all secret references in a namespace
  occ secretreference list --namespace acme-corp`,
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
		Use:   "get [SECRET_REFERENCE_NAME]",
		Short: "Get a secret reference",
		Long:  `Get a secret reference and display its details in YAML format.`,
		Example: `  # Get a secret reference
  occ secretreference get my-secret --namespace acme-corp`,
		Args:    cmdutil.ExactOneArgWithUsage(),
		PreRunE: auth.RequireLogin(),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, err := f()
			if err != nil {
				return err
			}
			return New(cl).Get(GetParams{
				Namespace:           flags.GetNamespace(cmd),
				SecretReferenceName: args[0],
			})
		},
	}
	flags.AddNamespace(cmd)
	return cmd
}

func newDeleteCmd(f client.NewClientFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete [SECRET_REFERENCE_NAME]",
		Short: "Delete a secret reference",
		Long:  `Delete a secret reference by name.`,
		Example: `  # Delete a secret reference
  occ secretreference delete my-secret --namespace acme-corp`,
		Args:    cmdutil.ExactOneArgWithUsage(),
		PreRunE: auth.RequireLogin(),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, err := f()
			if err != nil {
				return err
			}
			return New(cl).Delete(DeleteParams{
				Namespace:           flags.GetNamespace(cmd),
				SecretReferenceName: args[0],
			})
		},
	}
	flags.AddNamespace(cmd)
	return cmd
}
