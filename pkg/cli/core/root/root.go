// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package root

import (
	"github.com/spf13/cobra"

	"github.com/openchoreo/openchoreo/pkg/cli/cmd/apply"
	"github.com/openchoreo/openchoreo/pkg/cli/cmd/buildplane"
	"github.com/openchoreo/openchoreo/pkg/cli/cmd/component"
	componentrelease "github.com/openchoreo/openchoreo/pkg/cli/cmd/component-release"
	"github.com/openchoreo/openchoreo/pkg/cli/cmd/componenttype"
	"github.com/openchoreo/openchoreo/pkg/cli/cmd/componentworkflow"
	configContext "github.com/openchoreo/openchoreo/pkg/cli/cmd/config"
	"github.com/openchoreo/openchoreo/pkg/cli/cmd/create"
	"github.com/openchoreo/openchoreo/pkg/cli/cmd/dataplane"
	"github.com/openchoreo/openchoreo/pkg/cli/cmd/delete"
	"github.com/openchoreo/openchoreo/pkg/cli/cmd/environment"
	"github.com/openchoreo/openchoreo/pkg/cli/cmd/login"
	"github.com/openchoreo/openchoreo/pkg/cli/cmd/logout"
	"github.com/openchoreo/openchoreo/pkg/cli/cmd/namespace"
	"github.com/openchoreo/openchoreo/pkg/cli/cmd/observabilityplane"
	"github.com/openchoreo/openchoreo/pkg/cli/cmd/project"
	releasebinding "github.com/openchoreo/openchoreo/pkg/cli/cmd/release-binding"
	"github.com/openchoreo/openchoreo/pkg/cli/cmd/secretreference"
	"github.com/openchoreo/openchoreo/pkg/cli/cmd/trait"
	"github.com/openchoreo/openchoreo/pkg/cli/cmd/version"
	"github.com/openchoreo/openchoreo/pkg/cli/cmd/workflow"
	"github.com/openchoreo/openchoreo/pkg/cli/common/config"
	"github.com/openchoreo/openchoreo/pkg/cli/types/api"
)

// BuildRootCmd assembles the root command with all subcommands
func BuildRootCmd(config *config.CLIConfig, impl api.CommandImplementationInterface) *cobra.Command {
	rootCmd := &cobra.Command{
		Use:   config.Name,
		Short: config.ShortDescription,
		Long:  config.LongDescription,
	}

	// Add all commands directly
	rootCmd.AddCommand(
		apply.NewApplyCmd(impl),
		create.NewCreateCmd(impl),
		login.NewLoginCmd(impl),
		logout.NewLogoutCmd(impl),
		configContext.NewConfigCmd(impl),
		delete.NewDeleteCmd(impl),
		version.NewVersionCmd(),
		componentrelease.NewComponentReleaseCmd(impl),
		releasebinding.NewReleaseBindingCmd(impl),
		// Resource commands with list subcommands
		namespace.NewNamespaceCmd(impl),
		project.NewProjectCmd(impl),
		component.NewComponentCmd(impl),
		environment.NewEnvironmentCmd(impl),
		dataplane.NewDataPlaneCmd(impl),
		buildplane.NewBuildPlaneCmd(impl),
		observabilityplane.NewObservabilityPlaneCmd(impl),
		componenttype.NewComponentTypeCmd(impl),
		trait.NewTraitCmd(impl),
		workflow.NewWorkflowCmd(impl),
		componentworkflow.NewComponentWorkflowCmd(impl),
		secretreference.NewSecretReferenceCmd(impl),
	)

	return rootCmd
}
