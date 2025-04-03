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
