// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package project

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/openchoreo/openchoreo/internal/occ/cmd/utils"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
)

func print(list *gen.ProjectList) error {
	if list == nil || len(list.Items) == 0 {
		fmt.Println("No projects found")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "NAME\tAGE")

	for _, proj := range list.Items {
		name := proj.Metadata.Name
		age := "n/a"
		if proj.Metadata.CreationTimestamp != nil {
			age = utils.FormatAge(*proj.Metadata.CreationTimestamp)
		}
		fmt.Fprintf(w, "%s\t%s\n", name, age)
	}

	return w.Flush()
}
