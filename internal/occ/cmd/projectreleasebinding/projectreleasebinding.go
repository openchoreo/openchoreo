// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package projectreleasebinding

import (
	"context"
	"fmt"
	"os"
	"text/tabwriter"

	"sigs.k8s.io/yaml"

	"github.com/openchoreo/openchoreo/internal/occ/cmd/pagination"
	"github.com/openchoreo/openchoreo/internal/occ/cmd/utils"
	"github.com/openchoreo/openchoreo/internal/occ/cmdutil"
	"github.com/openchoreo/openchoreo/internal/occ/resources/client"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
)

// ProjectReleaseBinding implements project release binding operations
type ProjectReleaseBinding struct {
	client client.Interface
}

// New creates a new project release binding implementation
func New(c client.Interface) *ProjectReleaseBinding {
	return &ProjectReleaseBinding{client: c}
}

// List lists project release bindings in a namespace, optionally filtered by project
func (prb *ProjectReleaseBinding) List(params ListParams) error {
	if err := cmdutil.RequireFields("list", "projectreleasebinding", map[string]string{"namespace": params.Namespace}); err != nil {
		return err
	}

	ctx := context.Background()

	items, err := pagination.FetchAll(func(limit int, cursor string) ([]gen.ProjectReleaseBinding, string, error) {
		p := &gen.ListProjectReleaseBindingsParams{}
		if params.Project != "" {
			p.Project = &params.Project
		}
		p.Limit = &limit
		if cursor != "" {
			p.Cursor = &cursor
		}
		result, err := prb.client.ListProjectReleaseBindings(ctx, params.Namespace, p)
		if err != nil {
			return nil, "", err
		}
		next := ""
		if result.Pagination.NextCursor != nil {
			next = *result.Pagination.NextCursor
		}
		return result.Items, next, nil
	})
	if err != nil {
		return err
	}

	return printList(items)
}

// Get retrieves a single project release binding and outputs it as YAML
func (prb *ProjectReleaseBinding) Get(params GetParams) error {
	if err := cmdutil.RequireFields("get", "projectreleasebinding", map[string]string{"namespace": params.Namespace}); err != nil {
		return err
	}

	ctx := context.Background()

	result, err := prb.client.GetProjectReleaseBinding(ctx, params.Namespace, params.ProjectReleaseBindingName)
	if err != nil {
		return err
	}

	data, err := yaml.Marshal(result)
	if err != nil {
		return fmt.Errorf("failed to marshal project release binding to YAML: %w", err)
	}

	fmt.Print(string(data))
	return nil
}

// Delete deletes a single project release binding
func (prb *ProjectReleaseBinding) Delete(params DeleteParams) error {
	if err := cmdutil.RequireFields("delete", "projectreleasebinding", map[string]string{"namespace": params.Namespace, "name": params.ProjectReleaseBindingName}); err != nil {
		return err
	}

	ctx := context.Background()

	if err := prb.client.DeleteProjectReleaseBinding(ctx, params.Namespace, params.ProjectReleaseBindingName); err != nil {
		return err
	}

	fmt.Printf("ProjectReleaseBinding '%s' deleted\n", params.ProjectReleaseBindingName)
	return nil
}

func printList(items []gen.ProjectReleaseBinding) error {
	if len(items) == 0 {
		fmt.Println("No project release bindings found")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "NAME\tPROJECT\tENVIRONMENT\tRELEASE\tSTATUS\tAGE")

	for _, b := range items {
		projectName := ""
		env := ""
		release := ""
		if b.Spec != nil {
			projectName = b.Spec.Owner.ProjectName
			env = b.Spec.Environment
			if b.Spec.ProjectRelease != nil {
				release = *b.Spec.ProjectRelease
			}
		}
		status := ""
		if b.Status != nil && b.Status.Conditions != nil {
			for _, c := range *b.Status.Conditions {
				if c.Type == "Ready" {
					status = c.Reason
					break
				}
			}
		}
		age := ""
		if b.Metadata.CreationTimestamp != nil {
			age = utils.FormatAge(*b.Metadata.CreationTimestamp)
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n",
			b.Metadata.Name,
			projectName,
			env,
			release,
			status,
			age)
	}

	return w.Flush()
}
