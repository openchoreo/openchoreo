// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package resourcereleasebindingrevision

import (
	"github.com/spf13/cobra"

	"github.com/openchoreo/openchoreo/internal/occ/auth"
	"github.com/openchoreo/openchoreo/internal/occ/cmd/bindingrevision"
	"github.com/openchoreo/openchoreo/internal/occ/cmd/config"
	"github.com/openchoreo/openchoreo/internal/occ/flags"
	"github.com/openchoreo/openchoreo/internal/occ/resources/client"
)

func NewResourceReleaseBindingRevisionCmd(f client.NewClientFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "resourcereleasebindingrevision",
		Aliases: []string{"resourcereleasebindingrevisions", "rrbr", "rrbrs"},
		Short:   "Manage resource release binding revisions",
		Long:    "Commands for listing, showing and deleting resource release binding revisions. To roll back, use occ resourcereleasebinding rollback.",
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
		Args:  cobra.NoArgs,
		Short: "List resource release binding revisions",
		Long: `List revisions, newest first. With --binding, the revisions of that one
binding. Without it, the revisions of every resource release binding in the namespace, with
the project, binding and environment of each; narrow them with --project,
--resource and --environment.`,
		Example: `  # List the revisions of every resource release binding in the namespace
  occ resourcereleasebindingrevision list --namespace acme-corp

  # Every binding in one project
  occ resourcereleasebindingrevision list --namespace acme-corp --project online-store

  # Every environment of one resource
  occ resourcereleasebindingrevision list --namespace acme-corp --resource orders-db

  # Every resource in one environment
  occ resourcereleasebindingrevision list --namespace acme-corp --environment production

  # One binding
  occ resourcereleasebindingrevision list --namespace acme-corp --binding orders-db-development`,
		// --project, --component and --resource are filters here: filling them
		// from the current context would silently narrow the list, so context
		// defaults are skipped and only the namespace is taken from the context.
		Annotations: map[string]string{config.SkipContextDefaultsAnnotation: ""},
		PreRunE:     auth.RequireLogin(),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, err := f()
			if err != nil {
				return err
			}
			return New(cl).List(ListParams{
				Namespace:   bindingrevision.NamespaceOrContext(cmd),
				Binding:     bindingrevision.GetBinding(cmd),
				Project:     flags.GetProject(cmd),
				Resource:    flags.GetResource(cmd),
				Environment: bindingrevision.GetEnvironment(cmd),
			})
		},
	}
	flags.AddNamespace(cmd)
	bindingrevision.AddBinding(cmd, "resource release binding")
	flags.AddProject(cmd)
	flags.AddResource(cmd)
	bindingrevision.AddEnvironment(cmd)
	return cmd
}

func newGetCmd(f client.NewClientFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get [REVISION_NAME]",
		Short: "Get a resource release binding revision",
		Long: `Get one revision of a resource release binding, as a table row or in full as YAML.

Name the revision with REVISION_NAME (its name, for example
orders-db-development-r3 or the display name shown by list, or r3) or with --revision.`,
		Args: cobra.MaximumNArgs(1),
		Example: `  # Show revision 3 of a resource release binding
  occ resourcereleasebindingrevision get --namespace acme-corp --binding orders-db-development --revision 3

  # The same revision, by name
  occ resourcereleasebindingrevision get orders-db-development-r3 --namespace acme-corp --binding orders-db-development

  # Show the full revision, including the binding spec it recorded
  occ resourcereleasebindingrevision get --namespace acme-corp --binding orders-db-development --revision 3 -o yaml`,
		PreRunE: auth.RequireLogin(),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, err := f()
			if err != nil {
				return err
			}
			revisionNumber, err := bindingrevision.RevisionFromArgs(cmd, args)
			if err != nil {
				return err
			}
			return New(cl).Get(GetParams{
				Namespace: flags.GetNamespace(cmd),
				Binding:   bindingrevision.GetBinding(cmd),
				Revision:  revisionNumber,
				Name:      nameArg(args),
				Output:    bindingrevision.GetOutput(cmd),
			})
		},
	}
	flags.AddNamespace(cmd)
	bindingrevision.AddBinding(cmd, "resource release binding")
	bindingrevision.AddRevision(cmd)
	bindingrevision.AddOutput(cmd)
	return cmd
}

func newDeleteCmd(f client.NewClientFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete [REVISION_NAME]",
		Short: "Delete a resource release binding revision",
		Long: `Delete one revision from a resource release binding's history. It can no longer be
rolled back to. The other revisions keep their numbers, and the deleted number is
never given to a new revision.

Name the revision with REVISION_NAME (its name, its display name, or r3) or with
--revision. The revision is shown and you are asked to confirm. The server refuses
to delete the active revision and the newest known-good revision.`,
		Args: cobra.MaximumNArgs(1),
		Example: `  # Check that revision 2 can be deleted, without deleting it
  occ resourcereleasebindingrevision delete --namespace acme-corp --binding orders-db-development --revision 2 --dry-run

  # Delete revision 2, confirming interactively
  occ resourcereleasebindingrevision delete r2 --namespace acme-corp --binding orders-db-development

  # Delete without a prompt (for scripts)
  occ resourcereleasebindingrevision delete --namespace acme-corp --binding orders-db-development --revision 2 --yes`,
		PreRunE: auth.RequireLogin(),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, err := f()
			if err != nil {
				return err
			}
			revisionNumber, err := bindingrevision.RevisionFromArgs(cmd, args)
			if err != nil {
				return err
			}
			return New(cl).Delete(DeleteParams{
				Namespace: flags.GetNamespace(cmd),
				Binding:   bindingrevision.GetBinding(cmd),
				Revision:  revisionNumber,
				Name:      nameArg(args),
				DryRun:    bindingrevision.GetDryRun(cmd),
				Yes:       bindingrevision.GetYes(cmd),
			})
		},
	}
	flags.AddNamespace(cmd)
	bindingrevision.AddBinding(cmd, "resource release binding")
	bindingrevision.AddRevision(cmd)
	bindingrevision.AddDryRun(cmd, "Check that the revision can be deleted without deleting it")
	bindingrevision.AddYes(cmd, "Delete without asking for confirmation")
	return cmd
}

// nameArg returns the revision name the optional REVISION_NAME argument gives,
// or "" (see bindingrevision.NameArg).
func nameArg(args []string) string {
	if len(args) == 0 {
		return ""
	}
	return bindingrevision.NameArg(args[0])
}
