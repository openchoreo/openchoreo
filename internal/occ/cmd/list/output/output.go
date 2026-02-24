// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package output

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/openchoreo/openchoreo/internal/occ/cmd/utils"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
)

// PrintComponentReleases prints component releases in table format
func PrintComponentReleases(list *gen.ComponentReleaseList) error {
	if list == nil || len(list.Items) == 0 {
		fmt.Println("No component releases found")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "NAME\tCOMPONENT\tSTATUS\tAGE")

	for _, release := range list.Items {
		componentName := ""
		if release.Spec != nil {
			componentName = release.Spec.Owner.ComponentName
		}
		age := ""
		if release.Metadata.CreationTimestamp != nil {
			age = utils.FormatAge(*release.Metadata.CreationTimestamp)
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n",
			release.Metadata.Name,
			componentName,
			"", // status field removed in K8s-native schema
			age)
	}

	return w.Flush()
}

// PrintReleaseBindings prints release bindings in table format
func PrintReleaseBindings(list *gen.ReleaseBindingList) error {
	if list == nil || len(list.Items) == 0 {
		fmt.Println("No release bindings found")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "NAME\tENVIRONMENT\tRELEASE\tSTATUS\tAGE")

	for _, binding := range list.Items {
		status := ""
		if binding.Status != nil && binding.Status.Conditions != nil {
			for _, c := range *binding.Status.Conditions {
				if c.Type == "Ready" {
					status = c.Reason
					break
				}
			}
		}
		releaseName := ""
		if binding.Spec != nil && binding.Spec.ReleaseName != nil {
			releaseName = *binding.Spec.ReleaseName
		}
		environment := ""
		if binding.Spec != nil {
			environment = binding.Spec.Environment
		}
		age := ""
		if binding.Metadata.CreationTimestamp != nil {
			age = utils.FormatAge(*binding.Metadata.CreationTimestamp)
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
			binding.Metadata.Name,
			environment,
			releaseName,
			status,
			age)
	}

	return w.Flush()
}
