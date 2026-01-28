// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package output

import (
	"fmt"
	"os"
	"text/tabwriter"
	"time"

	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
)

// PrintNamespaces prints namespaces in table format
func PrintNamespaces(list *gen.NamespaceList) error {
	if len(list.Items) == 0 {
		fmt.Println("No namespaces found")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "NAME\tSTATUS\tAGE")

	for _, ns := range list.Items {
		status := ""
		if ns.Status != nil {
			status = *ns.Status
		}
		fmt.Fprintf(w, "%s\t%s\t%s\n",
			ns.Name,
			status,
			formatAge(ns.CreatedAt))
	}

	return w.Flush()
}

// PrintProjects prints projects in table format
func PrintProjects(list *gen.ProjectList) error {
	if len(list.Items) == 0 {
		fmt.Println("No projects found")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "NAME\tDISPLAY NAME\tSTATUS\tAGE")

	for _, proj := range list.Items {
		displayName := ""
		if proj.DisplayName != nil {
			displayName = *proj.DisplayName
		}
		status := ""
		if proj.Status != nil {
			status = *proj.Status
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n",
			proj.Name,
			displayName,
			status,
			formatAge(proj.CreatedAt))
	}

	return w.Flush()
}

// PrintComponents prints components in table format
func PrintComponents(list *gen.ComponentList) error {
	if len(list.Items) == 0 {
		fmt.Println("No components found")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "NAME\tPROJECT\tTYPE\tSTATUS\tAGE")

	for _, comp := range list.Items {
		status := ""
		if comp.Status != nil {
			status = *comp.Status
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
			comp.Name,
			comp.ProjectName,
			comp.Type,
			status,
			formatAge(comp.CreatedAt))
	}

	return w.Flush()
}

// PrintEnvironments prints environments in table format
func PrintEnvironments(list *gen.EnvironmentList) error {
	if len(list.Items) == 0 {
		fmt.Println("No environments found")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "NAME\tDATA PLANE\tPRODUCTION\tSTATUS\tAGE")

	for _, env := range list.Items {
		dataPlane := ""
		if env.DataPlaneRef != nil {
			dataPlane = *env.DataPlaneRef
		}
		production := "false"
		if env.IsProduction {
			production = "true"
		}
		status := ""
		if env.Status != nil {
			status = *env.Status
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
			env.Name,
			dataPlane,
			production,
			status,
			formatAge(env.CreatedAt))
	}

	return w.Flush()
}

// PrintDataPlanes prints data planes in table format
func PrintDataPlanes(list *gen.DataPlaneList) error {
	if len(list.Items) == 0 {
		fmt.Println("No data planes found")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "NAME\tSTATUS\tAGE")

	for _, dp := range list.Items {
		status := ""
		if dp.Status != nil {
			status = *dp.Status
		}
		fmt.Fprintf(w, "%s\t%s\t%s\n",
			dp.Name,
			status,
			formatAge(dp.CreatedAt))
	}

	return w.Flush()
}

// PrintBuildPlanes prints build planes in table format
func PrintBuildPlanes(list *gen.BuildPlaneList) error {
	if len(list.Items) == 0 {
		fmt.Println("No build planes found")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "NAME\tSTATUS\tAGE")

	for _, bp := range list.Items {
		status := ""
		if bp.Status != nil {
			status = *bp.Status
		}
		fmt.Fprintf(w, "%s\t%s\t%s\n",
			bp.Name,
			status,
			formatAge(bp.CreatedAt))
	}

	return w.Flush()
}

// PrintObservabilityPlanes prints observability planes in table format
func PrintObservabilityPlanes(list *gen.ObservabilityPlaneList) error {
	if len(list.Items) == 0 {
		fmt.Println("No observability planes found")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "NAME\tSTATUS\tAGE")

	for _, op := range list.Items {
		status := ""
		if op.Status != nil {
			status = *op.Status
		}
		fmt.Fprintf(w, "%s\t%s\t%s\n",
			op.Name,
			status,
			formatAge(op.CreatedAt))
	}

	return w.Flush()
}

// PrintComponentTypes prints component types in table format
func PrintComponentTypes(list *gen.ComponentTypeList) error {
	if len(list.Items) == 0 {
		fmt.Println("No component types found")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "NAME\tWORKLOAD TYPE\tAGE")

	for _, ct := range list.Items {
		fmt.Fprintf(w, "%s\t%s\t%s\n",
			ct.Name,
			ct.WorkloadType,
			formatAge(ct.CreatedAt))
	}

	return w.Flush()
}

// PrintTraits prints traits in table format
func PrintTraits(list *gen.TraitList) error {
	if len(list.Items) == 0 {
		fmt.Println("No traits found")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "NAME\tAGE")

	for _, trait := range list.Items {
		fmt.Fprintf(w, "%s\t%s\n",
			trait.Name,
			formatAge(trait.CreatedAt))
	}

	return w.Flush()
}

// PrintWorkflows prints workflows in table format
func PrintWorkflows(list *gen.WorkflowList) error {
	if len(list.Items) == 0 {
		fmt.Println("No workflows found")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "NAME\tAGE")

	for _, wf := range list.Items {
		fmt.Fprintf(w, "%s\t%s\n",
			wf.Name,
			formatAge(wf.CreatedAt),
		)
	}

	return w.Flush()
}

// PrintComponentWorkflows prints component workflows in table format
func PrintComponentWorkflows(list *gen.ComponentWorkflowTemplateList) error {
	if len(list.Items) == 0 {
		fmt.Println("No component workflows found")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "NAME\tAGE")

	for _, cwf := range list.Items {
		fmt.Fprintf(w, "%s\t%s\n",
			cwf.Name,
			formatAge(cwf.CreatedAt))
	}

	return w.Flush()
}

// PrintSecretReferences prints secret references in table format
func PrintSecretReferences(list *gen.SecretReferenceList) error {
	if len(list.Items) == 0 {
		fmt.Println("No secret references found")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "NAME\tSTATUS\tAGE")

	for _, sr := range list.Items {
		status := ""
		if sr.Status != nil {
			status = *sr.Status
		}
		fmt.Fprintf(w, "%s\t%s\t%s\n",
			sr.Name,
			status,
			formatAge(sr.CreatedAt))
	}

	return w.Flush()
}

// Helper functions

func formatAge(t time.Time) string {
	duration := time.Since(t)
	if duration.Hours() < 1 {
		return fmt.Sprintf("%dm", int(duration.Minutes()))
	} else if duration.Hours() < 24 {
		return fmt.Sprintf("%dh", int(duration.Hours()))
	} else {
		return fmt.Sprintf("%dd", int(duration.Hours()/24))
	}
}

func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
