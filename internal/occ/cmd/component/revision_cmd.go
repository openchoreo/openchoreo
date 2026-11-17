// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package component

import (
	"github.com/spf13/cobra"

	"github.com/openchoreo/openchoreo/internal/occ/auth"
	"github.com/openchoreo/openchoreo/internal/occ/cmd/bindingrevision"
	"github.com/openchoreo/openchoreo/internal/occ/cmd/releasebinding"
	"github.com/openchoreo/openchoreo/internal/occ/cmd/releasebindingrevision"
	"github.com/openchoreo/openchoreo/internal/occ/cmdutil"
	"github.com/openchoreo/openchoreo/internal/occ/flags"
	"github.com/openchoreo/openchoreo/internal/occ/resources/client"
)

// The commands in this file are short forms of the releasebinding rollback and
// releasebindingrevision commands. They take the binding as an argument
// instead of --binding and run the same operations.

func newRevisionsCmd(f client.NewClientFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "revisions BINDING_NAME",
		Short: "List the revisions of a release binding",
		Long: `List the revisions of one release binding, newest first.

Short form of "occ releasebindingrevision list --binding BINDING_NAME".`,
		Example: `  # List the revisions of a release binding
  occ component revisions product-catalog-development --namespace acme-corp`,
		Args:    cmdutil.ExactOneArgWithUsage(),
		PreRunE: auth.RequireLogin(),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, err := f()
			if err != nil {
				return err
			}
			return releasebindingrevision.New(cl).List(releasebindingrevision.ListParams{
				Namespace: flags.GetNamespace(cmd),
				Binding:   args[0],
			})
		},
	}
	flags.AddNamespace(cmd)
	return cmd
}

func newRevisionCmd(f client.NewClientFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "revision",
		Short: "Show or delete one revision of a release binding",
		Long: `Show or delete one revision of a release binding. To list a binding's
revisions, use "occ component revisions BINDING_NAME".`,
	}
	cmd.AddCommand(
		newRevisionGetCmd(f),
		newRevisionDeleteCmd(f),
	)
	return cmd
}

func newRevisionGetCmd(f client.NewClientFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get BINDING_NAME REVISION",
		Short: "Get a revision of a release binding",
		Long: `Get one revision of a release binding, as a table row or in full as YAML.

REVISION is the revision number (3), r3, or the revision's name.

Short form of "occ releasebindingrevision get REVISION --binding BINDING_NAME".`,
		Example: `  # Show revision 3 of a release binding
  occ component revision get product-catalog-development 3 --namespace acme-corp

  # Show the full revision, including the binding spec it recorded
  occ component revision get product-catalog-development 3 --namespace acme-corp -o yaml`,
		Args:    cobra.ExactArgs(2),
		PreRunE: auth.RequireLogin(),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, err := f()
			if err != nil {
				return err
			}
			revision, err := bindingrevision.ParseRevisionName(args[1])
			if err != nil {
				return err
			}
			return releasebindingrevision.New(cl).Get(releasebindingrevision.GetParams{
				Namespace: flags.GetNamespace(cmd),
				Binding:   args[0],
				Revision:  revision,
				Output:    bindingrevision.GetOutput(cmd),
			})
		},
	}
	flags.AddNamespace(cmd)
	bindingrevision.AddOutput(cmd)
	return cmd
}

func newRevisionDeleteCmd(f client.NewClientFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete BINDING_NAME REVISION",
		Short: "Delete a revision of a release binding",
		Long: `Delete one revision from a release binding's history. It can no longer be
rolled back to. The revision is shown and you are asked to confirm.

REVISION is the revision number (2), r2, or the revision's name.

Short form of "occ releasebindingrevision delete REVISION --binding BINDING_NAME".`,
		Example: `  # Check that revision 2 can be deleted, without deleting it
  occ component revision delete product-catalog-development 2 --namespace acme-corp --dry-run

  # Delete revision 2, confirming interactively
  occ component revision delete product-catalog-development 2 --namespace acme-corp

  # Delete without a prompt (for scripts)
  occ component revision delete product-catalog-development 2 --namespace acme-corp --yes`,
		Args:    cobra.ExactArgs(2),
		PreRunE: auth.RequireLogin(),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, err := f()
			if err != nil {
				return err
			}
			revision, err := bindingrevision.ParseRevisionName(args[1])
			if err != nil {
				return err
			}
			return releasebindingrevision.New(cl).Delete(releasebindingrevision.DeleteParams{
				Namespace: flags.GetNamespace(cmd),
				Binding:   args[0],
				Revision:  revision,
				DryRun:    bindingrevision.GetDryRun(cmd),
				Yes:       bindingrevision.GetYes(cmd),
			})
		},
	}
	flags.AddNamespace(cmd)
	bindingrevision.AddDryRun(cmd, "Check that the revision can be deleted without deleting it")
	bindingrevision.AddYes(cmd, "Delete without asking for confirmation")
	return cmd
}

func newRollbackCmd(f client.NewClientFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "rollback BINDING_NAME",
		Short: "Roll a release binding back to one of its revisions",
		Long: `Roll a release binding back to the revision named with --revision or,
without it, to the revision that was active immediately before the current one.
The rollback is previewed first and you are asked to confirm.

Short form of "occ releasebinding rollback BINDING_NAME", which describes
the rollback in full.`,
		Example: `  # Roll back to the previously active revision, confirming interactively
  occ component rollback product-catalog-development --namespace acme-corp

  # Preview a rollback to revision 3 without changing anything
  occ component rollback product-catalog-development --namespace acme-corp --revision 3 --dry-run

  # Roll back to revision 3 without a prompt, recording why
  occ component rollback product-catalog-development --namespace acme-corp --revision 3 --yes --message "r5 broke login"`,
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
			return releasebinding.New(cl).Rollback(releasebinding.RollbackParams{
				Namespace:          flags.GetNamespace(cmd),
				ReleaseBindingName: args[0],
				Revision:           revision,
				DryRun:             bindingrevision.GetDryRun(cmd),
				Yes:                bindingrevision.GetYes(cmd),
				Message:            flags.GetMessage(cmd),
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
