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
