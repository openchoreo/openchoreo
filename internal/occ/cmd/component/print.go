// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package component

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/openchoreo/openchoreo/internal/occ/cmd/utils"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
)

func print(list *gen.ComponentList, showProject bool) error {
	if list == nil || len(list.Items) == 0 {
		fmt.Println("No components found")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	if showProject {
		fmt.Fprintln(w, "NAME\tPROJECT\tTYPE\tAGE")
	} else {
		fmt.Fprintln(w, "NAME\tTYPE\tAGE")
	}

	for _, comp := range list.Items {
		projectName := ""
		componentType := ""
		if comp.Spec != nil {
			projectName = comp.Spec.Owner.ProjectName
			componentType = comp.Spec.ComponentType.Name
		}
		age := ""
		if comp.Metadata.CreationTimestamp != nil {
			age = utils.FormatAge(*comp.Metadata.CreationTimestamp)
		}
		if showProject {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\n",
				comp.Metadata.Name,
				projectName,
				componentType,
				age)
		} else {
			fmt.Fprintf(w, "%s\t%s\t%s\n",
				comp.Metadata.Name,
				componentType,
				age)
		}
	}

	return w.Flush()
}
