// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package component

import (
	"strings"

	"github.com/spf13/cobra"

	"github.com/openchoreo/openchoreo/internal/occ/auth"
	"github.com/openchoreo/openchoreo/internal/occ/cmdutil"
	"github.com/openchoreo/openchoreo/internal/occ/flags"
	"github.com/openchoreo/openchoreo/internal/occ/resources/client"
)

func NewComponentCmd(f client.NewClientFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "component",
		Aliases: []string{"comp", "components"},
		Short:   "Manage components",
		Long:    `Manage components for OpenChoreo.`,
	}
	cmd.AddCommand(
		newListCmd(f),
		newGetCmd(f),
		newDeleteCmd(f),
		newScaffoldCmd(f),
		newDeployCmd(f),
		newLogsCmd(f),
		newWorkflowCmd(f),
		newWorkflowRunCmd(f),
	)
	return cmd
}

func newListCmd(f client.NewClientFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List components",
		Long:  `List all components in a project.`,
		Example: `  # List all components in a project
  occ component list --namespace acme-corp --project online-store`,
		PreRunE: auth.RequireLogin(),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, err := f()
			if err != nil {
				return err
			}
			return New(cl).List(ListParams{
				Namespace: flags.GetNamespace(cmd),
				Project:   flags.GetProject(cmd),
			})
		},
	}
	flags.AddNamespace(cmd)
	flags.AddProject(cmd)
	return cmd
}

func newGetCmd(f client.NewClientFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get [COMPONENT_NAME]",
		Short: "Get a component",
		Long:  `Get a component and display its details in YAML format.`,
		Example: `  # Get a component
  occ component get my-component --namespace acme-corp`,
		Args:    cmdutil.ExactOneArgWithUsage(),
		PreRunE: auth.RequireLogin(),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, err := f()
			if err != nil {
				return err
			}
			return New(cl).Get(GetParams{
				Namespace:     flags.GetNamespace(cmd),
				ComponentName: args[0],
			})
		},
	}
	flags.AddNamespace(cmd)
	return cmd
}

func newDeleteCmd(f client.NewClientFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete [COMPONENT_NAME]",
		Short: "Delete a component",
		Long:  `Delete a component by name.`,
		Example: `  # Delete a component
  occ component delete my-component --namespace acme-corp`,
		Args:    cmdutil.ExactOneArgWithUsage(),
		PreRunE: auth.RequireLogin(),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, err := f()
			if err != nil {
				return err
			}
			return New(cl).Delete(DeleteParams{
				Namespace:     flags.GetNamespace(cmd),
				ComponentName: args[0],
			})
		},
	}
	flags.AddNamespace(cmd)
	return cmd
}

func newScaffoldCmd(f client.NewClientFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "scaffold COMPONENT_NAME",
		Short: "Scaffold a Component YAML from ComponentType and Traits",
		Long: `Generate a Component YAML file based on a ComponentType definition.

The command fetches the ComponentType and any specified Traits from the cluster,
applies default values, and generates a YAML file with required fields as
placeholders and optional fields as commented examples.

Use --componenttype/--traits/--workflow for namespace-scoped resources, or
--clustercomponenttype/--clustertraits/--clusterworkflow for cluster-scoped resources.
Each pair is mutually exclusive.

Examples:
  # Scaffold using a cluster-scoped ClusterComponentType
  occ component scaffold my-app --clustercomponenttype deployment/web-app

  # Scaffold using a namespace-scoped ComponentType
  occ component scaffold my-app --componenttype deployment/web-app

  # Scaffold with cluster-scoped traits
  occ component scaffold my-app --clustercomponenttype deployment/web-app --clustertraits storage,ingress

  # Output to file
  occ component scaffold my-app --clustercomponenttype deployment/web-app -o my-app.yaml`,
		Args:    cmdutil.ExactOneArgWithUsage(),
		PreRunE: auth.RequireLogin(),
		RunE: func(cmd *cobra.Command, args []string) error {
			componentType, _ := cmd.Flags().GetString("componenttype")
			clusterComponentType, _ := cmd.Flags().GetString("clustercomponenttype")
			workflow, _ := cmd.Flags().GetString("workflow")
			clusterWorkflow, _ := cmd.Flags().GetString("clusterworkflow")
			skipComments, _ := cmd.Flags().GetBool("skip-comments")
			skipOptional, _ := cmd.Flags().GetBool("skip-optional")
			cl, err := f()
			if err != nil {
				return err
			}
			return New(cl).Scaffold(ScaffoldParams{
				ComponentName:        args[0],
				ComponentType:        componentType,
				ClusterComponentType: clusterComponentType,
				Traits:               parseCSV(cmd, "traits"),
				ClusterTraits:        parseCSV(cmd, "clustertraits"),
				WorkflowName:         workflow,
				ClusterWorkflowName:  clusterWorkflow,
				Namespace:            flags.GetNamespace(cmd),
				ProjectName:          flags.GetProject(cmd),
				OutputPath:           flags.GetOutputFile(cmd),
				SkipComments:         skipComments,
				SkipOptional:         skipOptional,
			})
		},
	}
	cmd.Flags().String("componenttype", "", "Namespace-scoped component type in format workloadType/componentTypeName (e.g., deployment/web-app)")
	cmd.Flags().String("clustercomponenttype", "", "Cluster-scoped component type in format workloadType/componentTypeName (e.g., deployment/web-app)")
	cmd.Flags().String("traits", "", "Comma-separated list of namespace-scoped Trait names to include")
	cmd.Flags().String("clustertraits", "", "Comma-separated list of cluster-scoped ClusterTrait names to include")
	cmd.Flags().String("workflow", "", "Namespace-scoped Workflow name")
	cmd.Flags().String("clusterworkflow", "", "Cluster-scoped ClusterWorkflow name")
	cmd.Flags().Bool("skip-comments", false, "Skip section headers and field description comments for minimal output")
	cmd.Flags().Bool("skip-optional", false, "Skip optional fields without defaults (show only required fields)")
	flags.AddNamespace(cmd)
	flags.AddProject(cmd)
	flags.AddOutputFile(cmd)
	return cmd
}

func newDeployCmd(f client.NewClientFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "deploy [COMPONENT_NAME]",
		Short: "Deploy or promote a component",
		Long:  "Deploy a component release to the root environment or promote to the next environment in the pipeline.",
		Example: `  # Deploy latest release to root environment
  occ component deploy api-service --namespace acme-corp --project online-store

  # Promote to a specific environment
  occ component deploy api-service --namespace acme-corp --project online-store --to staging`,
		Args:    cmdutil.ExactOneArgWithUsage(),
		PreRunE: auth.RequireLogin(),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, err := f()
			if err != nil {
				return err
			}
			return New(cl).Deploy(DeployParams{
				ComponentName: args[0],
				Namespace:     flags.GetNamespace(cmd),
				Project:       flags.GetProject(cmd),
				Release:       flags.GetRelease(cmd),
				To:            flags.GetTo(cmd),
				Set:           flags.GetSet(cmd),
			})
		},
	}
	flags.AddNamespace(cmd)
	flags.AddProject(cmd)
	flags.AddRelease(cmd)
	flags.AddTo(cmd)
	flags.AddSet(cmd)
	return cmd
}

func newLogsCmd(f client.NewClientFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "logs COMPONENT_NAME",
		Short: "Get logs for a component",
		Long: `Retrieve logs for a component from a specific environment.
If --env is not specified, uses the lowest environment from the deployment pipeline.`,
		Example: `  # Get logs for a component (uses lowest environment if --env not specified)
  occ component logs my-component

  # Get logs from a specific environment
  occ component logs my-component --env dev

  # Follow logs in real-time
  occ component logs my-component --env dev -f`,
		Args:    cmdutil.ExactOneArgWithUsage(),
		PreRunE: auth.RequireLogin(),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, err := f()
			if err != nil {
				return err
			}
			tail := flags.GetTail(cmd)
			return New(cl).Logs(LogsParams{
				Namespace:   flags.GetNamespace(cmd),
				Project:     flags.GetProject(cmd),
				Component:   args[0],
				Environment: flags.GetEnvironment(cmd),
				Follow:      flags.GetFollow(cmd),
				Since:       flags.GetSince(cmd),
				Tail:        tail,
			})
		},
	}
	flags.AddNamespace(cmd)
	flags.AddProject(cmd)
	flags.AddEnvironment(cmd)
	flags.AddFollow(cmd)
	flags.AddSince(cmd)
	flags.AddTail(cmd)
	return cmd
}

func newWorkflowCmd(f client.NewClientFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "workflow",
		Aliases: []string{"wf"},
		Short:   "Manage component workflows",
		Long:    `Manage component workflows for OpenChoreo.`,
	}
	cmd.AddCommand(
		newStartWorkflowCmd(f),
		newWorkflowLogsCmd(f),
	)
	return cmd
}

func newStartWorkflowCmd(f client.NewClientFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "run [COMPONENT_NAME]",
		Short: "Run a component's workflow",
		Long:  `Run a workflow for a component using its configured workflow.`,
		Example: `  # Run workflow for a component
  occ component workflow run my-service --namespace acme-corp`,
		Args:    cmdutil.ExactOneArgWithUsage(),
		PreRunE: auth.RequireLogin(),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, err := f()
			if err != nil {
				return err
			}
			return New(cl).StartWorkflow(StartWorkflowParams{
				Namespace:     flags.GetNamespace(cmd),
				ComponentName: args[0],
				Project:       flags.GetProject(cmd),
				Set:           flags.GetSet(cmd),
			})
		},
	}
	flags.AddNamespace(cmd)
	flags.AddProject(cmd)
	flags.AddSet(cmd)
	return cmd
}

func newWorkflowLogsCmd(f client.NewClientFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "logs [COMPONENT_NAME]",
		Short: "Get logs for a component's workflow run",
		Long: `Get logs for a component's workflow run.
Finds the latest workflow run for the component by default.
Use --workflowrun to specify a particular run.`,
		Example: `  # Get logs for the latest workflow run of a component
  occ component workflow logs my-service --namespace acme-corp`,
		Args:    cmdutil.ExactOneArgWithUsage(),
		PreRunE: auth.RequireLogin(),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, err := f()
			if err != nil {
				return err
			}
			return New(cl).WorkflowRunLogs(WorkflowRunLogsParams{
				Namespace:     flags.GetNamespace(cmd),
				ComponentName: args[0],
				RunName:       flags.GetWorkflowRun(cmd),
				Follow:        flags.GetFollow(cmd),
				Since:         flags.GetSince(cmd),
			})
		},
	}
	flags.AddNamespace(cmd)
	flags.AddFollow(cmd)
	flags.AddSince(cmd)
	flags.AddWorkflowRun(cmd)
	return cmd
}

func newWorkflowRunCmd(f client.NewClientFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "workflowrun",
		Aliases: []string{"wfrun", "wr"},
		Short:   "Manage component workflow runs",
		Long:    `Manage workflow runs for a component.`,
	}
	cmd.AddCommand(
		newListWorkflowRunCmd(f),
		newWorkflowRunLogsCmd(f),
	)
	return cmd
}

func newListWorkflowRunCmd(f client.NewClientFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list [COMPONENT_NAME]",
		Short: "List workflow runs for a component",
		Long:  `List all workflow runs for a component.`,
		Example: `  # List workflow runs for a component
  occ component workflowrun list my-component --namespace acme-corp`,
		Args:    cmdutil.ExactOneArgWithUsage(),
		PreRunE: auth.RequireLogin(),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, err := f()
			if err != nil {
				return err
			}
			return New(cl).ListWorkflowRuns(ListWorkflowRunsParams{
				Namespace:     flags.GetNamespace(cmd),
				ComponentName: args[0],
			})
		},
	}
	flags.AddNamespace(cmd)
	return cmd
}

func newWorkflowRunLogsCmd(f client.NewClientFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "logs [COMPONENT_NAME]",
		Short: "Get logs for a component's workflow run",
		Long: `Get logs for a component's workflow run.
Finds the latest workflow run for the component by default.
Use --workflowrun to specify a particular run.`,
		Example: `  # Get logs for the latest workflow run of a component
  occ component workflowrun logs my-service --namespace acme-corp`,
		Args:    cmdutil.ExactOneArgWithUsage(),
		PreRunE: auth.RequireLogin(),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, err := f()
			if err != nil {
				return err
			}
			return New(cl).WorkflowRunLogs(WorkflowRunLogsParams{
				Namespace:     flags.GetNamespace(cmd),
				ComponentName: args[0],
				RunName:       flags.GetWorkflowRun(cmd),
				Follow:        flags.GetFollow(cmd),
				Since:         flags.GetSince(cmd),
			})
		},
	}
	flags.AddNamespace(cmd)
	flags.AddFollow(cmd)
	flags.AddSince(cmd)
	flags.AddWorkflowRun(cmd)
	return cmd
}

// parseCSV parses a comma-separated flag value into a trimmed, non-empty string slice.
func parseCSV(cmd *cobra.Command, flagName string) []string {
	val, _ := cmd.Flags().GetString(flagName)
	if val == "" {
		return nil
	}
	parts := strings.Split(val, ",")
	var result []string
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}
