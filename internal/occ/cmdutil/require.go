// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package cmdutil

import (
	"fmt"
	"sort"
	"strings"
)

// RequireFields returns an error listing any fields whose value is empty,
// formatted as a user-facing message with a hint to run -h.
// fields maps flag name → current value.
func RequireFields(cmd, resource string, fields map[string]string) error {
	var missing []string
	for name, val := range fields {
		if val == "" {
			missing = append(missing, name)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	sort.Strings(missing)
	n := len(missing)
	plural := ""
	if n > 1 {
		plural = "s"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Missing required parameter%s: --%s\n\n", plural, strings.Join(missing, ", --"))
	b.WriteString("To see usage details:\n")
	fmt.Fprintf(&b, "  occ %s %s -h", resource, cmd)
	return fmt.Errorf("%s", b.String())
}
