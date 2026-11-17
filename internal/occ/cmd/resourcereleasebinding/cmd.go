// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package resourcereleasebinding

import (
	"github.com/spf13/cobra"

	"github.com/openchoreo/openchoreo/internal/occ/auth"
	"github.com/openchoreo/openchoreo/internal/occ/cmd/bindingrevision"
	"github.com/openchoreo/openchoreo/internal/occ/cmdutil"
	"github.com/openchoreo/openchoreo/internal/occ/flags"
	"github.com/openchoreo/openchoreo/internal/occ/resources/client"
)

func NewResourceReleaseBindingCmd(f client.NewClientFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "resourcereleasebinding",
		Aliases: []string{"resourcereleasebindings", "rrb"},
		Short:   "Manage resource release bindings",
		Long:    "Commands for managing resource release bindings.",
	}
	cmd.AddCommand(
		newListCmd(f),
		newGetCmd(f),
		newDeleteCmd(f),
		newRollbackCmd(f),
	)
	return cmd
}

func newListCmd(f client.NewClientFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List resource release bindings",
		Long:  `List all resource release bindings in a namespace, optionally filtered by resource.`,
		Example: `  # List all resource release bindings in a namespace
  occ resourcereleasebinding list --namespace acme-corp

  # List bindings for a specific resource
  occ resourcereleasebinding list --namespace acme-corp --resource analytics-db`,
		PreRunE: auth.RequireLogin(),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, err := f()
			if err != nil {
				return err
			}
			return New(cl).List(ListParams{
				Namespace: flags.GetNamespace(cmd),
				Resource:  flags.GetResource(cmd),
			})
		},
	}
	flags.AddNamespace(cmd)
	flags.AddResource(cmd)
	return cmd
}

func newGetCmd(f client.NewClientFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get [RESOURCE_RELEASE_BINDING_NAME]",
		Short: "Get a resource release binding",
		Long:  `Get a resource release binding and display its details in YAML format.`,
		Example: `  # Get a resource release binding
  occ resourcereleasebinding get analytics-db-dev --namespace acme-corp`,
		Args:    cmdutil.ExactOneArgWithUsage(),
		PreRunE: auth.RequireLogin(),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, err := f()
			if err != nil {
				return err
			}
			return New(cl).Get(GetParams{
				Namespace:                  flags.GetNamespace(cmd),
				ResourceReleaseBindingName: args[0],
			})
		},
	}
	flags.AddNamespace(cmd)
	return cmd
}

func newDeleteCmd(f client.NewClientFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete [RESOURCE_RELEASE_BINDING_NAME]",
		Short: "Delete a resource release binding",
		Long:  `Delete a resource release binding by name.`,
		Example: `  # Delete a resource release binding
  occ resourcereleasebinding delete analytics-db-dev --namespace acme-corp`,
		Args:    cmdutil.ExactOneArgWithUsage(),
		PreRunE: auth.RequireLogin(),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, err := f()
			if err != nil {
				return err
			}
			return New(cl).Delete(DeleteParams{
				Namespace:                  flags.GetNamespace(cmd),
				ResourceReleaseBindingName: args[0],
			})
		},
	}
	flags.AddNamespace(cmd)
	return cmd
}

func newRollbackCmd(f client.NewClientFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "rollback [RESOURCE_RELEASE_BINDING_NAME]",
		Short: "Roll a resource release binding back to one of its revisions",
		Long: `Restore the binding spec recorded by one of the binding's revisions:
the one named with --revision or, without it, the revision that was active
immediately before the current one (like kubectl rollout undo). That follows
activation history, not revision numbers: after a rollback, the previous
revision can be numbered above the current one. If it was deleted, or the
history cannot say which it was, nothing is changed; name a revision instead.
List the revisions with "occ resourcereleasebindingrevision list --binding <name>".

The rollback is previewed first: the spec fields it changes and any pre-flight
warnings are printed, and you are asked to confirm. No revision is created: the
controller makes the target revision active again, under its own name and number.`,
		Example: `  # Roll back to the previously active revision, confirming interactively
  occ resourcereleasebinding rollback orders-db-development --namespace acme-corp

  # Preview a rollback to revision 3 without changing anything
  occ resourcereleasebinding rollback orders-db-development --namespace acme-corp --revision 3 --dry-run

  # Roll back to revision 3, confirming interactively
  occ resourcereleasebinding rollback orders-db-development --namespace acme-corp --revision 3

  # Roll back without a prompt (for scripts)
  occ resourcereleasebinding rollback orders-db-development --namespace acme-corp --revision 3 --yes`,
		Args:    cmdutil.ExactOneArgWithUsage(),
		PreRunE: auth.RequireLogin(),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, err := f()
			if err != nil {
				return err
			}
			revision, err := bindingrevision.GetRollbackRevision(cmd)
			if err != nil {
				return err
			}
			return New(cl).Rollback(RollbackParams{
				Namespace:                  flags.GetNamespace(cmd),
				ResourceReleaseBindingName: args[0],
				Revision:                   revision,
				DryRun:                     bindingrevision.GetDryRun(cmd),
				Yes:                        bindingrevision.GetYes(cmd),
				Message:                    flags.GetMessage(cmd),
			})
		},
	}
	flags.AddNamespace(cmd)
	bindingrevision.AddRollbackRevision(cmd)
	bindingrevision.AddDryRun(cmd, "Show the changes and warnings the rollback would produce without applying it")
	bindingrevision.AddYes(cmd, "Roll back without asking for confirmation")
	flags.AddMessage(cmd, "Optional reason for the rollback, shown on the binding's events and in the audit log")
	return cmd
}
