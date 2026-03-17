// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package args

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

// ExactOneArgWithUsage returns a cobra positional args validator that provides a descriptive
// error message when the required argument is missing. It extracts the argument name
// from the command's Use field (e.g., "get [NAMESPACE_NAME]" → "NAMESPACE_NAME").
func ExactOneArgWithUsage() cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error {
		if len(args) < 1 {
			argName := extractArgName(cmd.Use)
			return fmt.Errorf("required argument %s not provided\n\nUsage:\n  %s", argName, cmd.UseLine())
		}
		if len(args) > 1 {
			return fmt.Errorf("accepts 1 arg(s), received %d", len(args))
		}
		return nil
	}
}

// extractArgName extracts the argument placeholder from a cobra Use string.
// For example: "get [NAMESPACE_NAME]" → "NAMESPACE_NAME", "run WORKFLOW_NAME" → "WORKFLOW_NAME".
func extractArgName(use string) string {
	parts := strings.Fields(use)
	if len(parts) > 1 {
		arg := parts[1]
		arg = strings.Trim(arg, "[]")
		if arg != "" {
			return arg
		}
	}
	return "NAME"
}
