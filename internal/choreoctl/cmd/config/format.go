// Copyright OpenChoreo Authors 2025
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"fmt"
	"os"
	"text/tabwriter"
)

// formatValueOrPlaceholder returns a string representation of a value, with a placeholder for empty values
func formatValueOrPlaceholder(value string) string {
	if value == "" {
		return "-"
	}
	return value
}

// printTable prints a table with headers and rows using tabwriter for consistent alignment
func printTable(headers []string, rows [][]string) error {
	if len(rows) == 0 {
		return nil
	}

	// Create a new tabwriter that writes to stdout
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', tabwriter.TabIndent)
	defer w.Flush()

	// Print headers
	for i, header := range headers {
		if i > 0 {
			fmt.Fprint(w, "\t")
		}
		fmt.Fprint(w, header)
	}
	fmt.Fprintln(w)

	// Print rows
	for _, row := range rows {
		for i, cell := range row {
			if i > 0 {
				fmt.Fprint(w, "\t")
			}
			fmt.Fprint(w, cell)
		}
		fmt.Fprintln(w)
	}

	return nil
}
