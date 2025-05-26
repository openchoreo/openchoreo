// Copyright 2025 OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package version

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/openchoreo/openchoreo/internal/version"
	"github.com/openchoreo/openchoreo/pkg/cli/common/builder"
	"github.com/openchoreo/openchoreo/pkg/cli/common/constants"
)

// NewVersionCmd creates the login command.
func NewVersionCmd() *cobra.Command {
	return (&builder.CommandBuilder{
		Command: constants.Version,
		RunE: func(fg *builder.FlagGetter) error {
			v := version.Get()
			fmt.Printf("%s %s\n", v.Name, v.Version)
			fmt.Printf("Git revision: %s\n", v.GitRevision)
			fmt.Printf("Build time:   %s\n", v.BuildTime)
			fmt.Printf("Go version:   %s %s/%s\n",
				v.GoVersion, v.GoOS, v.GoArch)
			return nil
		},
	}).Build()
}
