// Copyright OpenChoreo Authors 2025
// SPDX-License-Identifier: Apache-2.0

package config

import "github.com/openchoreo/openchoreo/pkg/cli/common/messages"

type CLIConfig struct {
	Name             string
	ShortDescription string
	LongDescription  string
}

func DefaultConfig() *CLIConfig {
	return &CLIConfig{
		Name:             messages.DefaultCLIName,
		ShortDescription: messages.DefaultCLIShortDescription,
	}
}
