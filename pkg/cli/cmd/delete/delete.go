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

package delete

import (
	"github.com/spf13/cobra"

	"github.com/choreo-idp/choreo/pkg/cli/common/constants"
	"github.com/choreo-idp/choreo/pkg/cli/flags"
	"github.com/choreo-idp/choreo/pkg/cli/types/api"
)

// NewDeleteCmd creates the main delete command
func NewDeleteCmd(impl api.CommandImplementationInterface) *cobra.Command {
	deleteCmd := &cobra.Command{
		Use:     constants.Delete.Use,
		Short:   constants.Delete.Short,
		Long:    constants.Delete.Long,
		Example: constants.Delete.Example,
	}

	// Add the file and wait flags directly to deleteCmd
	deleteCmd.Flags().StringP(flags.DeleteFileFlag.Name, flags.DeleteFileFlag.Shorthand, "", flags.DeleteFileFlag.Usage)
	deleteCmd.Flags().BoolP(flags.Wait.Name, flags.Wait.Shorthand, false, flags.Wait.Usage)

	// Add a special handler for -f/--file flag
	deleteCmd.RunE = func(cmd *cobra.Command, args []string) error {
		filePath, _ := cmd.Flags().GetString(flags.DeleteFileFlag.Name)
		if filePath != "" {
			wait, _ := cmd.Flags().GetBool(flags.Wait.Name)
			return impl.Delete(api.DeleteParams{
				FilePath: filePath,
				Wait:     wait,
			})
		}
		// Default behavior
		return cmd.Help()
	}

	return deleteCmd
}
