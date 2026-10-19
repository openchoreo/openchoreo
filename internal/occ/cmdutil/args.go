// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package cmdutil

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

// ExactArgsWithUsage is ExactOneArgWithUsage for commands taking n arguments,
// such as "diff [FROM] [TO]". The error names the missing arguments.
func ExactArgsWithUsage(n int) cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error {
		if len(args) > n {
			return fmt.Errorf("accepts %d arg(s), received %d", n, len(args))
		}
		if len(args) < n {
			names := extractArgNames(cmd.Use, n)
			return fmt.Errorf("required argument %s not provided\n\nUsage:\n  %s", strings.Join(names[len(args):], " and "), cmd.UseLine())
		}
		return nil
	}
}

// extractArgNames returns the first n argument placeholders of a cobra Use
// string, falling back to ARG1, ARG2, ... for any that are not named.
func extractArgNames(use string, n int) []string {
	parts := strings.Fields(use)
	names := make([]string, n)
	for i := range names {
		names[i] = fmt.Sprintf("ARG%d", i+1)
		if i+1 < len(parts) {
			if arg := strings.Trim(parts[i+1], "[]"); arg != "" {
				names[i] = arg
			}
		}
	}
	return names
}
