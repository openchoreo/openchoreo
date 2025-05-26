// Copyright 2025 OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package root

import (
	"github.com/spf13/cobra"

	"github.com/openchoreo/openchoreo/pkg/cli/cmd/apply"
	configContext "github.com/openchoreo/openchoreo/pkg/cli/cmd/config"
	"github.com/openchoreo/openchoreo/pkg/cli/cmd/create"
	"github.com/openchoreo/openchoreo/pkg/cli/cmd/delete"
	"github.com/openchoreo/openchoreo/pkg/cli/cmd/get"
	"github.com/openchoreo/openchoreo/pkg/cli/cmd/logs"
	"github.com/openchoreo/openchoreo/pkg/cli/cmd/version"
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
		get.NewListCmd(impl),
		// login.NewLoginCmd(impl), // Removed login and logout until we finalize the user experience
		// logout.NewLogoutCmd(impl),
		logs.NewLogsCmd(impl),
		configContext.NewConfigCmd(impl),
		delete.NewDeleteCmd(impl),
		version.NewVersionCmd(),
	)

	return rootCmd
}
