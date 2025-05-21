/*
 * Copyright Open Choreo Authors
 * SPDX-License-Identifier: Apache-2.0
 * The full text of the Apache license is available in the LICENSE file at
 * the root of the repo.
 */

package main

import (
	"os"

	"github.com/spf13/cobra"

	"github.com/openchoreo/openchoreo/internal/choreoctl"
	configContext "github.com/openchoreo/openchoreo/internal/choreoctl/cmd/config"
	"github.com/openchoreo/openchoreo/pkg/cli/common/config"
	"github.com/openchoreo/openchoreo/pkg/cli/core/root"
)

func main() {
	cfg := config.DefaultConfig()
	commandImpl := choreoctl.NewCommandImplementation()

	rootCmd := root.BuildRootCmd(cfg, commandImpl)
	rootCmd.SilenceUsage = true

	// Initialize choreoctl execution environment
	rootCmd.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		// Initialize default context if none exists
		if err := configContext.EnsureContext(); err != nil {
			return err
		}

		// Apply context defaults to command flags
		return configContext.ApplyContextDefaults(cmd)
	}

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
