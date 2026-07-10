// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package projectrelease

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

// ProjectRelease implements project release operations
type ProjectRelease struct {
	client client.Interface
}

// New creates a new project release implementation
func New(c client.Interface) *ProjectRelease {
	return &ProjectRelease{client: c}
}

// List lists project releases in a namespace, optionally filtered by project
func (pr *ProjectRelease) List(params ListParams) error {
	if err := cmdutil.RequireFields("list", "projectrelease", map[string]string{"namespace": params.Namespace}); err != nil {
		return err
	}

	ctx := context.Background()

	items, err := pagination.FetchAll(func(limit int, cursor string) ([]gen.ProjectRelease, string, error) {
		p := &gen.ListProjectReleasesParams{}
		if params.Project != "" {
			p.Project = &params.Project
		}
		p.Limit = &limit
		if cursor != "" {
			p.Cursor = &cursor
		}
		result, err := pr.client.ListProjectReleases(ctx, params.Namespace, p)
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

// Get retrieves a single project release and outputs it as YAML
func (pr *ProjectRelease) Get(params GetParams) error {
	if err := cmdutil.RequireFields("get", "projectrelease", map[string]string{"namespace": params.Namespace}); err != nil {
		return err
	}

	ctx := context.Background()

	result, err := pr.client.GetProjectRelease(ctx, params.Namespace, params.ProjectReleaseName)
	if err != nil {
		return err
	}

	data, err := yaml.Marshal(result)
	if err != nil {
		return fmt.Errorf("failed to marshal project release to YAML: %w", err)
	}

	fmt.Print(string(data))
	return nil
}

// Delete deletes a single project release
func (pr *ProjectRelease) Delete(params DeleteParams) error {
	if err := cmdutil.RequireFields("delete", "projectrelease", map[string]string{"namespace": params.Namespace, "name": params.ProjectReleaseName}); err != nil {
		return err
	}

	ctx := context.Background()

	if err := pr.client.DeleteProjectRelease(ctx, params.Namespace, params.ProjectReleaseName); err != nil {
		return err
	}

	fmt.Printf("ProjectRelease '%s' deleted\n", params.ProjectReleaseName)
	return nil
}

func printList(items []gen.ProjectRelease) error {
	if len(items) == 0 {
		fmt.Println("No project releases found")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "NAME\tPROJECT\tAGE")

	for _, release := range items {
		projectName := ""
		if release.Spec != nil {
			projectName = release.Spec.Owner.ProjectName
		}
		age := ""
		if release.Metadata.CreationTimestamp != nil {
			age = utils.FormatAge(*release.Metadata.CreationTimestamp)
		}
		fmt.Fprintf(w, "%s\t%s\t%s\n",
			release.Metadata.Name,
			projectName,
			age)
	}

	return w.Flush()
}
