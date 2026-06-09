// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"

	"github.com/spf13/cobra"

	"github.com/openchoreo/openchoreo/internal/occ/cmd/config"
	"github.com/openchoreo/openchoreo/internal/occ/root"
)

func main() {
	rootCmd := root.BuildRootCmd()
	rootCmd.SilenceUsage = true

	// Initialize occ execution environment
	rootCmd.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		// Initialize default context if none exists
		if err := config.EnsureContext(); err != nil {
			return err
		}

		// Apply context defaults to command flags
		return config.ApplyContextDefaults(cmd)
	}

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
