// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package project

import (
	"github.com/spf13/cobra"

	"github.com/openchoreo/openchoreo/internal/occ/auth"
	"github.com/openchoreo/openchoreo/internal/occ/cmdutil"
	"github.com/openchoreo/openchoreo/internal/occ/flags"
	"github.com/openchoreo/openchoreo/internal/occ/resources/client"
)

func NewProjectCmd(f client.NewClientFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "project",
		Aliases: []string{"proj", "projects"},
		Short:   "Manage projects",
		Long:    `Manage projects for OpenChoreo.`,
	}
	cmd.AddCommand(
		newListCmd(f),
		newGetCmd(f),
		newDeleteCmd(f),
		newDeployCmd(f),
		newScaffoldCmd(f),
	)
	return cmd
}

func newListCmd(f client.NewClientFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List projects",
		Long:  `List all projects in a namespace.`,
		Example: `  # List all projects in a namespace
  occ project list --namespace acme-corp`,
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
		Use:   "get [PROJECT_NAME]",
		Short: "Get a project",
		Long:  `Get a project and display its details in YAML format.`,
		Example: `  # Get a project
  occ project get my-project --namespace acme-corp`,
		Args:    cmdutil.ExactOneArgWithUsage(),
		PreRunE: auth.RequireLogin(),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, err := f()
			if err != nil {
				return err
			}
			return New(cl).Get(GetParams{
				Namespace:   flags.GetNamespace(cmd),
				ProjectName: args[0],
			})
		},
	}
	flags.AddNamespace(cmd)
	return cmd
}

func newDeleteCmd(f client.NewClientFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete [PROJECT_NAME]",
		Short: "Delete a project",
		Long:  `Delete a project by name.`,
		Example: `  # Delete a project
  occ project delete my-project --namespace acme-corp`,
		Args:    cmdutil.ExactOneArgWithUsage(),
		PreRunE: auth.RequireLogin(),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, err := f()
			if err != nil {
				return err
			}
			return New(cl).Delete(DeleteParams{
				Namespace:   flags.GetNamespace(cmd),
				ProjectName: args[0],
			})
		},
	}
	flags.AddNamespace(cmd)
	return cmd
}

func newDeployCmd(f client.NewClientFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "deploy [PROJECT_NAME]",
		Short: "Deploy or promote a project",
		Long: "Deploy a project's ProjectReleaseBinding to the lowest environment in " +
			"the pipeline, or promote it to the next environment.",
		Example: `  # Deploy to the lowest environment (controller seeds the latest release)
  occ project deploy online-store --namespace acme-corp

  # Promote to a specific environment
  occ project deploy online-store --namespace acme-corp --to staging

  # Pin an explicit release
  occ project deploy online-store --namespace acme-corp --release online-store-abc123

  # Override environment config values
  occ project deploy online-store --namespace acme-corp --to staging --set replicas=3`,
		Args:    cmdutil.ExactOneArgWithUsage(),
		PreRunE: auth.RequireLogin(),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, err := f()
			if err != nil {
				return err
			}
			return New(cl).Deploy(DeployParams{
				Namespace:   flags.GetNamespace(cmd),
				ProjectName: args[0],
				To:          flags.GetTo(cmd),
				Release:     flags.GetRelease(cmd),
				Set:         flags.GetSet(cmd),
			})
		},
	}
	flags.AddNamespace(cmd)
	flags.AddTo(cmd)
	flags.AddRelease(cmd)
	flags.AddSet(cmd)
	return cmd
}

func newScaffoldCmd(f client.NewClientFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "scaffold PROJECT_NAME",
		Short: "Scaffold a Project YAML from a ProjectType",
		Long: `Generate a Project YAML file based on a (Cluster)ProjectType definition.

The command fetches the type's parameter schema, applies default values, and
generates a YAML file with required fields as placeholders and optional fields as
commented examples. By default it also emits one ProjectReleaseBinding per
environment in the deployment pipeline (like the UI's auto-deploy); pass
--no-bindings to generate only the Project.

The deployment pipeline is always resolved, because the generated Project
references it via spec.deploymentPipelineRef. If the pipeline defines no
environments, only the Project is generated and a comment explains why.

Use --projecttype for a namespace-scoped ProjectType or --clusterprojecttype for
a cluster-scoped ClusterProjectType. They are mutually exclusive.

Examples:
  # Scaffold using a cluster-scoped ClusterProjectType
  occ project scaffold online-store --clusterprojecttype default --namespace acme-corp

  # Scaffold using a namespace-scoped ProjectType
  occ project scaffold online-store --projecttype web-service --namespace acme-corp

  # Only the Project (no ProjectReleaseBindings)
  occ project scaffold online-store --projecttype web-service --namespace acme-corp --no-bindings

  # Output to file
  occ project scaffold online-store --projecttype web-service --namespace acme-corp -o project.yaml`,
		Args:    cmdutil.ExactOneArgWithUsage(),
		PreRunE: auth.RequireLogin(),
		RunE: func(cmd *cobra.Command, args []string) error {
			projectType, _ := cmd.Flags().GetString("projecttype")
			clusterProjectType, _ := cmd.Flags().GetString("clusterprojecttype")
			deploymentPipeline, _ := cmd.Flags().GetString("deployment-pipeline")
			skipComments, _ := cmd.Flags().GetBool("skip-comments")
			skipOptional, _ := cmd.Flags().GetBool("skip-optional")
			noBindings, _ := cmd.Flags().GetBool("no-bindings")
			cl, err := f()
			if err != nil {
				return err
			}
			return New(cl).Scaffold(ScaffoldParams{
				ProjectName:        args[0],
				Namespace:          flags.GetNamespace(cmd),
				ProjectType:        projectType,
				ClusterProjectType: clusterProjectType,
				DeploymentPipeline: deploymentPipeline,
				OutputPath:         flags.GetOutputFile(cmd),
				SkipComments:       skipComments,
				SkipOptional:       skipOptional,
				NoBindings:         noBindings,
			})
		},
	}
	cmd.Flags().String("projecttype", "", "Namespace-scoped ProjectType name")
	cmd.Flags().String("clusterprojecttype", "", "Cluster-scoped ClusterProjectType name")
	cmd.Flags().String("deployment-pipeline", "default", "DeploymentPipeline referenced by the project")
	cmd.Flags().Bool("skip-comments", false, "Skip section headers and field description comments for minimal output")
	cmd.Flags().Bool("skip-optional", false, "Skip optional fields without defaults (show only required fields)")
	cmd.Flags().Bool("no-bindings", false, "Do not generate per-environment ProjectReleaseBindings")
	flags.AddNamespace(cmd)
	flags.AddOutputFile(cmd)
	return cmd
}
