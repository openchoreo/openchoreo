/*
 * Copyright OpenChoreo Authors
 * SPDX-License-Identifier: Apache-2.0
 * The full text of the Apache license is available in the LICENSE file at
 * the root of the repo.
 */

package delete

import (
	"github.com/spf13/cobra"

	"github.com/openchoreo/openchoreo/pkg/cli/common/constants"
	"github.com/openchoreo/openchoreo/pkg/cli/flags"
	"github.com/openchoreo/openchoreo/pkg/cli/types/api"
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
