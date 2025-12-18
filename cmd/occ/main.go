// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"

	"github.com/spf13/cobra"

	"github.com/openchoreo/openchoreo/internal/occ"
	configContext "github.com/openchoreo/openchoreo/internal/occ/cmd/config"
	"github.com/openchoreo/openchoreo/pkg/cli/common/config"
	"github.com/openchoreo/openchoreo/pkg/cli/core/root"
)

func main() {
	cfg := config.DefaultConfig()
	commandImpl := occ.NewCommandImplementation()

	rootCmd := root.BuildRootCmd(cfg, commandImpl)
	rootCmd.SilenceUsage = true

	// Initialize occ execution environment
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
