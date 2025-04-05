/*
 * Copyright (c) 2025, WSO2 Inc. (http://www.wso2.org) All Rights Reserved.
 *
 * WSO2 Inc. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License.
 * You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied. See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

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
