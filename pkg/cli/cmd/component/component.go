// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package component

import (
	"strings"

	"github.com/spf13/cobra"

	"github.com/openchoreo/openchoreo/internal/occ/cmd/component"
	"github.com/openchoreo/openchoreo/internal/occ/cmd/login"
	cliargs "github.com/openchoreo/openchoreo/pkg/cli/common/args"
	"github.com/openchoreo/openchoreo/pkg/cli/common/auth"
	"github.com/openchoreo/openchoreo/pkg/cli/common/builder"
	"github.com/openchoreo/openchoreo/pkg/cli/common/constants"
	"github.com/openchoreo/openchoreo/pkg/cli/flags"
)

func NewComponentCmd() *cobra.Command {
	componentCmd := &cobra.Command{
		Use:     constants.Component.Use,
		Aliases: constants.Component.Aliases,
		Short:   constants.Component.Short,
		Long:    constants.Component.Long,
	}

	componentCmd.AddCommand(
		newListComponentCmd(),
		newGetComponentCmd(),
		newDeleteComponentCmd(),
		newScaffoldComponentCmd(),
		newDeployComponentCmd(),
		newLogsComponentCmd(),
		newComponentWorkflowCmd(),
		newComponentWorkflowRunCmd(),
	)

	return componentCmd
}

func newListComponentCmd() *cobra.Command {
	return (&builder.CommandBuilder{
		Command: constants.ListComponent,
		Flags:   []flags.Flag{flags.Namespace, flags.Project},
		PreRunE: auth.RequireLogin(login.NewAuthImpl()),
		RunE: func(fg *builder.FlagGetter) error {
			compImpl := component.New()
			return compImpl.List(component.ListParams{
				Namespace: fg.GetString(flags.Namespace),
				Project:   fg.GetString(flags.Project),
			})
		},
	}).Build()
}

func newGetComponentCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     constants.GetComponent.Use,
		Short:   constants.GetComponent.Short,
		Long:    constants.GetComponent.Long,
		Example: constants.GetComponent.Example,
		Args:    cliargs.ExactOneArgWithUsage(),
		PreRunE: auth.RequireLogin(login.NewAuthImpl()),
		RunE: func(cmd *cobra.Command, args []string) error {
			namespace, _ := cmd.Flags().GetString(flags.Namespace.Name)

			compImpl := component.New()
			return compImpl.Get(component.GetParams{
				Namespace:     namespace,
				ComponentName: args[0],
			})
		},
	}

	flags.AddFlags(cmd, flags.Namespace)

	return cmd
}

func newDeleteComponentCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     constants.DeleteComponent.Use,
		Short:   constants.DeleteComponent.Short,
		Long:    constants.DeleteComponent.Long,
		Example: constants.DeleteComponent.Example,
		Args:    cliargs.ExactOneArgWithUsage(),
		PreRunE: auth.RequireLogin(login.NewAuthImpl()),
		RunE: func(cmd *cobra.Command, args []string) error {
			namespace, _ := cmd.Flags().GetString(flags.Namespace.Name)

			compImpl := component.New()
			return compImpl.Delete(component.DeleteParams{
				Namespace:     namespace,
				ComponentName: args[0],
			})
		},
	}

	flags.AddFlags(cmd, flags.Namespace)

	return cmd
}

func newScaffoldComponentCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     constants.ScaffoldComponent.Use,
		Short:   constants.ScaffoldComponent.Short,
		Long:    constants.ScaffoldComponent.Long,
		Example: constants.ScaffoldComponent.Example,
		Args:    cliargs.ExactOneArgWithUsage(),
		PreRunE: auth.RequireLogin(login.NewAuthImpl()),
		RunE: func(cmd *cobra.Command, args []string) error {
			namespace, _ := cmd.Flags().GetString(flags.Namespace.Name)
			project, _ := cmd.Flags().GetString(flags.Project.Name)
			componentType, _ := cmd.Flags().GetString(flags.ScaffoldType.Name)
			clusterComponentType, _ := cmd.Flags().GetString(flags.ClusterComponentType.Name)
			workflow, _ := cmd.Flags().GetString(flags.Workflow.Name)
			clusterWorkflow, _ := cmd.Flags().GetString(flags.ClusterWorkflow.Name)
			outputFile, _ := cmd.Flags().GetString(flags.OutputFile.Name)
			skipComments, _ := cmd.Flags().GetBool(flags.SkipComments.Name)
			skipOptional, _ := cmd.Flags().GetBool(flags.SkipOptional.Name)

			compImpl := component.New()
			return compImpl.Scaffold(component.ScaffoldParams{
				ComponentName:        args[0],
				ComponentType:        componentType,
				ClusterComponentType: clusterComponentType,
				Traits:               parseCSV(cmd, flags.Traits.Name),
				ClusterTraits:        parseCSV(cmd, flags.ClusterTraits.Name),
				WorkflowName:         workflow,
				ClusterWorkflowName:  clusterWorkflow,
				Namespace:            namespace,
				ProjectName:          project,
				OutputPath:           outputFile,
				SkipComments:         skipComments,
				SkipOptional:         skipOptional,
			})
		},
	}

	flags.AddFlags(cmd,
		flags.ScaffoldType,
		flags.ClusterComponentType,
		flags.Traits,
		flags.ClusterTraits,
		flags.Workflow,
		flags.ClusterWorkflow,
		flags.Project,
		flags.Namespace,
		flags.OutputFile,
		flags.SkipComments,
		flags.SkipOptional,
	)

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

//nolint:dupl // deploy and logs commands follow the same pattern but are distinct
func newDeployComponentCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     constants.DeployComponent.Use,
		Short:   constants.DeployComponent.Short,
		Long:    constants.DeployComponent.Long,
		Example: constants.DeployComponent.Example,
		Args:    cliargs.ExactOneArgWithUsage(), // Requires COMPONENT_NAME
		RunE: func(cmd *cobra.Command, args []string) error {
			// Get component name from positional arg
			componentName := args[0]

			// Get flag values
			namespace, _ := cmd.Flags().GetString(flags.Namespace.Name)
			project, _ := cmd.Flags().GetString(flags.Project.Name)
			release, _ := cmd.Flags().GetString(flags.Release.Name)
			to, _ := cmd.Flags().GetString(flags.To.Name)
			set, _ := cmd.Flags().GetStringArray(flags.Set.Name)
			outputFormat, _ := cmd.Flags().GetString(flags.Output.Name)

			// Create params
			params := component.DeployParams{
				ComponentName: componentName,
				Namespace:     namespace,
				Project:       project,
				Release:       release,
				To:            to,
				Set:           set,
				OutputFormat:  outputFormat,
			}

			// Execute deploy
			compImpl := component.New()
			return compImpl.Deploy(params)
		},
	}

	// Add flags
	flags.AddFlags(cmd,
		flags.Namespace,
		flags.Project,
		flags.Release,
		flags.To,
		flags.Set,
		flags.Output,
	)

	return cmd
}

//nolint:dupl // logs and deploy commands follow the same pattern but are distinct
func newLogsComponentCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "logs COMPONENT_NAME",
		Short: "Get logs for a component",
		Long: `Retrieve logs for a component from a specific environment.
If --env is not specified, uses the lowest environment from the deployment pipeline.`,
		Example: `  # Get logs for a component (uses lowest environment if --env not specified)
  occ component logs my-component

  # Get logs from a specific environment
  occ component logs my-component --env dev

  # Get logs with custom since duration
  occ component logs my-component --env dev --since 30m

  # Show the last 100 lines of logs
  occ component logs my-component --tail 100

  # Follow logs in real-time
  occ component logs my-component --env dev -f`,
		Args: cliargs.ExactOneArgWithUsage(), // Requires COMPONENT_NAME
		RunE: func(cmd *cobra.Command, args []string) error {
			// Get component name from positional arg
			componentName := args[0]

			// Get flag values
			namespace, _ := cmd.Flags().GetString(flags.Namespace.Name)
			project, _ := cmd.Flags().GetString(flags.Project.Name)
			environment, _ := cmd.Flags().GetString(flags.Environment.Name)
			follow, _ := cmd.Flags().GetBool(flags.Follow.Name)
			since, _ := cmd.Flags().GetString(flags.Since.Name)
			tail, _ := cmd.Flags().GetInt(flags.Tail.Name)

			// Create params
			params := component.LogsParams{
				Namespace:   namespace,
				Project:     project,
				Component:   componentName,
				Environment: environment,
				Follow:      follow,
				Since:       since,
				Tail:        tail,
			}

			// Execute logs
			compImpl := component.New()
			return compImpl.Logs(params)
		},
	}

	// Add flags
	flags.AddFlags(cmd,
		flags.Namespace,
		flags.Project,
		flags.Environment,
		flags.Follow,
		flags.Since,
		flags.Tail,
	)

	return cmd
}

func newComponentWorkflowCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "workflow",
		Aliases: []string{"wf"},
		Short:   "Manage component workflows",
		Long:    `Manage component workflows for OpenChoreo.`,
	}

	cmd.AddCommand(
		newStartComponentWorkflowCmd(),
		newLogsComponentWorkflowCmd(),
	)

	return cmd
}

func newStartComponentWorkflowCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     constants.StartComponentWorkflow.Use,
		Short:   constants.StartComponentWorkflow.Short,
		Long:    constants.StartComponentWorkflow.Long,
		Example: constants.StartComponentWorkflow.Example,
		Args:    cliargs.ExactOneArgWithUsage(),
		PreRunE: auth.RequireLogin(login.NewAuthImpl()),
		RunE: func(cmd *cobra.Command, args []string) error {
			namespace, _ := cmd.Flags().GetString(flags.Namespace.Name)
			project, _ := cmd.Flags().GetString(flags.Project.Name)
			set, _ := cmd.Flags().GetStringArray(flags.Set.Name)
			return component.New().StartWorkflow(component.StartWorkflowParams{
				Namespace:     namespace,
				ComponentName: args[0],
				Project:       project,
				Set:           set,
			})
		},
	}

	flags.AddFlags(cmd, flags.Namespace, flags.Project, flags.Set)

	return cmd
}

func newLogsComponentWorkflowCmd() *cobra.Command {
	cmd := newLogsComponentWorkflowRunCmd()
	cmd.Use = constants.LogsComponentWorkflow.Use
	cmd.Short = constants.LogsComponentWorkflow.Short
	cmd.Long = constants.LogsComponentWorkflow.Long
	cmd.Example = constants.LogsComponentWorkflow.Example
	return cmd
}

func newComponentWorkflowRunCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "workflowrun",
		Aliases: []string{"wfrun", "wr"},
		Short:   "Manage component workflow runs",
		Long:    `Manage workflow runs for a component.`,
	}

	cmd.AddCommand(
		newListComponentWorkflowRunCmd(),
		newLogsComponentWorkflowRunCmd(),
	)

	return cmd
}

func newListComponentWorkflowRunCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     constants.ListComponentWorkflowRun.Use,
		Short:   constants.ListComponentWorkflowRun.Short,
		Long:    constants.ListComponentWorkflowRun.Long,
		Example: constants.ListComponentWorkflowRun.Example,
		Args:    cliargs.ExactOneArgWithUsage(),
		PreRunE: auth.RequireLogin(login.NewAuthImpl()),
		RunE: func(cmd *cobra.Command, args []string) error {
			namespace, _ := cmd.Flags().GetString(flags.Namespace.Name)
			return component.New().ListWorkflowRuns(component.ListWorkflowRunsParams{
				Namespace:     namespace,
				ComponentName: args[0],
			})
		},
	}

	flags.AddFlags(cmd, flags.Namespace)

	return cmd
}

func newLogsComponentWorkflowRunCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     constants.LogsComponentWorkflowRun.Use,
		Short:   constants.LogsComponentWorkflowRun.Short,
		Long:    constants.LogsComponentWorkflowRun.Long,
		Example: constants.LogsComponentWorkflowRun.Example,
		Args:    cliargs.ExactOneArgWithUsage(),
		PreRunE: auth.RequireLogin(login.NewAuthImpl()),
		RunE: func(cmd *cobra.Command, args []string) error {
			namespace, _ := cmd.Flags().GetString(flags.Namespace.Name)
			follow, _ := cmd.Flags().GetBool(flags.Follow.Name)
			since, _ := cmd.Flags().GetString(flags.Since.Name)
			run, _ := cmd.Flags().GetString(flags.WorkflowRun.Name)
			return component.New().WorkflowRunLogs(component.WorkflowRunLogsParams{
				Namespace:     namespace,
				ComponentName: args[0],
				RunName:       run,
				Follow:        follow,
				Since:         since,
			})
		},
	}

	flags.AddFlags(cmd, flags.Namespace, flags.Follow, flags.Since, flags.WorkflowRun)

	return cmd
}
