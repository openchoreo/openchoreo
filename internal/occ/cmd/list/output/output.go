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
	if list == nil || len(list.Items) == 0 {
		fmt.Println("No namespaces found")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "NAME\tAGE")

	for _, ns := range list.Items {
		fmt.Fprintf(w, "%s\t%s\n",
			ns.Name,
			formatAge(ns.CreatedAt))
	}

	return w.Flush()
}

// PrintProjects prints projects in table format
func PrintProjects(list *gen.ProjectList) error {
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
			age = formatAge(*proj.Metadata.CreationTimestamp)
		}
		fmt.Fprintf(w, "%s\t%s\n", name, age)
	}

	return w.Flush()
}

// PrintComponents prints components in table format
func PrintComponents(list *gen.ComponentList) error {
	if list == nil || len(list.Items) == 0 {
		fmt.Println("No components found")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "NAME\tPROJECT\tTYPE\tAGE")

	for _, comp := range list.Items {
		projectName := ""
		componentType := ""
		if comp.Spec != nil {
			projectName = comp.Spec.Owner.ProjectName
			componentType = comp.Spec.ComponentType.Name
		}
		age := ""
		if comp.Metadata.CreationTimestamp != nil {
			age = formatAge(*comp.Metadata.CreationTimestamp)
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n",
			comp.Metadata.Name,
			projectName,
			componentType,
			age)
	}

	return w.Flush()
}

// PrintEnvironments prints environments in table format
func PrintEnvironments(list *gen.EnvironmentList) error {
	if list == nil || len(list.Items) == 0 {
		fmt.Println("No environments found")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "NAME\tDATA PLANE\tPRODUCTION\tAGE")

	for _, env := range list.Items {
		dataPlane := ""
		if env.DataPlaneRef != nil {
			dataPlane = fmt.Sprintf("%s/%s", env.DataPlaneRef.Kind, env.DataPlaneRef.Name)
		}
		production := "false"
		if env.IsProduction {
			production = "true"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n",
			env.Name,
			dataPlane,
			production,
			formatAge(env.CreatedAt))
	}

	return w.Flush()
}

// PrintDataPlanes prints data planes in table format
func PrintDataPlanes(list *gen.DataPlaneList) error {
	if list == nil || len(list.Items) == 0 {
		fmt.Println("No data planes found")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "NAME\tAGE")

	for _, dp := range list.Items {
		fmt.Fprintf(w, "%s\t%s\n",
			dp.Name,
			formatAge(dp.CreatedAt))
	}

	return w.Flush()
}

// PrintBuildPlanes prints build planes in table format
func PrintBuildPlanes(list *gen.BuildPlaneList) error {
	if list == nil || len(list.Items) == 0 {
		fmt.Println("No build planes found")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "NAME\tAGE")

	for _, bp := range list.Items {
		fmt.Fprintf(w, "%s\t%s\n",
			bp.Name,
			formatAge(bp.CreatedAt))
	}

	return w.Flush()
}

// PrintObservabilityPlanes prints observability planes in table format
func PrintObservabilityPlanes(list *gen.ObservabilityPlaneList) error {
	if list == nil || len(list.Items) == 0 {
		fmt.Println("No observability planes found")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "NAME\tAGE")

	for _, op := range list.Items {
		fmt.Fprintf(w, "%s\t%s\n",
			op.Name,
			formatAge(op.CreatedAt))
	}

	return w.Flush()
}

// PrintComponentTypes prints component types in table format
func PrintComponentTypes(list *gen.ComponentTypeList) error {
	if list == nil || len(list.Items) == 0 {
		fmt.Println("No component types found")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "NAME\tWORKLOAD TYPE\tAGE")

	for _, ct := range list.Items {
		workloadType := ""
		if ct.Spec != nil {
			workloadType = string(ct.Spec.WorkloadType)
		}
		age := ""
		if ct.Metadata.CreationTimestamp != nil {
			age = formatAge(*ct.Metadata.CreationTimestamp)
		}
		fmt.Fprintf(w, "%s\t%s\t%s\n",
			ct.Metadata.Name,
			workloadType,
			age)
	}

	return w.Flush()
}

// PrintTraits prints traits in table format
func PrintTraits(list *gen.TraitList) error {
	if list == nil || len(list.Items) == 0 {
		fmt.Println("No traits found")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "NAME\tAGE")

	for _, trait := range list.Items {
		age := ""
		if trait.Metadata.CreationTimestamp != nil {
			age = formatAge(*trait.Metadata.CreationTimestamp)
		}
		fmt.Fprintf(w, "%s\t%s\n",
			trait.Metadata.Name,
			age)
	}

	return w.Flush()
}

// PrintWorkflows prints workflows in table format
func PrintWorkflows(list *gen.WorkflowList) error {
	if list == nil || len(list.Items) == 0 {
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
	if list == nil || len(list.Items) == 0 {
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
	if list == nil || len(list.Items) == 0 {
		fmt.Println("No secret references found")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "NAME\tAGE")

	for _, sr := range list.Items {
		fmt.Fprintf(w, "%s\t%s\n",
			sr.Name,
			formatAge(sr.CreatedAt))
	}

	return w.Flush()
}

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
			age = formatAge(*release.Metadata.CreationTimestamp)
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
			age = formatAge(*binding.Metadata.CreationTimestamp)
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

// PrintWorkflowRuns prints workflow runs in table format
func PrintWorkflowRuns(list *gen.WorkflowRunList) error {
	if list == nil || len(list.Items) == 0 {
		fmt.Println("No workflow runs found")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "NAME\tWORKFLOW\tSTATUS\tAGE")

	for _, run := range list.Items {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n",
			run.Name,
			run.WorkflowName,
			run.Status,
			formatAge(run.CreatedAt))
	}

	return w.Flush()
}

// PrintComponentWorkflowRuns prints component workflow runs in table format
func PrintComponentWorkflowRuns(list *gen.ComponentWorkflowRunList) error {
	if list == nil || len(list.Items) == 0 {
		fmt.Println("No component workflow runs found")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "NAME\tCOMPONENT\tSTATUS\tAGE")

	for _, run := range list.Items {
		status := ""
		if run.Status != nil {
			status = *run.Status
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n",
			run.Name,
			run.ComponentName,
			status,
			formatAge(run.CreatedAt))
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
