// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package projecttype

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"text/tabwriter"

	"sigs.k8s.io/yaml"

	"github.com/openchoreo/openchoreo/internal/occ/cmd/pagination"
	"github.com/openchoreo/openchoreo/internal/occ/cmd/utils"
	"github.com/openchoreo/openchoreo/internal/occ/cmdutil"
	"github.com/openchoreo/openchoreo/internal/occ/resources/client"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
)

// ProjectType implements project type operations
type ProjectType struct {
	client client.Interface
}

// New creates a new project type implementation
func New(c client.Interface) *ProjectType {
	return &ProjectType{client: c}
}

// List lists all project types in a namespace
func (pt *ProjectType) List(params ListParams) error {
	if err := cmdutil.RequireFields("list", "projecttype", map[string]string{"namespace": params.Namespace}); err != nil {
		return err
	}

	ctx := context.Background()

	items, err := pagination.FetchAll(func(limit int, cursor string) ([]gen.ProjectType, string, error) {
		p := &gen.ListProjectTypesParams{}
		p.Limit = &limit
		if cursor != "" {
			p.Cursor = &cursor
		}
		result, err := pt.client.ListProjectTypes(ctx, params.Namespace, p)
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

// Get retrieves a single project type and outputs it as YAML
func (pt *ProjectType) Get(params GetParams) error {
	if err := cmdutil.RequireFields("get", "projecttype", map[string]string{"namespace": params.Namespace}); err != nil {
		return err
	}

	ctx := context.Background()

	result, err := pt.client.GetProjectType(ctx, params.Namespace, params.ProjectTypeName)
	if err != nil {
		return err
	}

	data, err := yaml.Marshal(result)
	if err != nil {
		return fmt.Errorf("failed to marshal project type to YAML: %w", err)
	}

	fmt.Print(string(data))
	return nil
}

// Delete deletes a single project type
func (pt *ProjectType) Delete(params DeleteParams) error {
	if err := cmdutil.RequireFields("delete", "projecttype", map[string]string{"namespace": params.Namespace, "name": params.ProjectTypeName}); err != nil {
		return err
	}

	ctx := context.Background()

	if err := pt.client.DeleteProjectType(ctx, params.Namespace, params.ProjectTypeName); err != nil {
		return err
	}

	fmt.Printf("ProjectType '%s' deleted\n", params.ProjectTypeName)
	return nil
}

func printList(items []gen.ProjectType) error {
	if len(items) == 0 {
		fmt.Println("No project types found")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "NAME\tRESOURCES\tAGE")

	for _, pt := range items {
		resources := "0"
		if pt.Spec != nil {
			resources = strconv.Itoa(len(pt.Spec.Resources))
		}
		age := ""
		if pt.Metadata.CreationTimestamp != nil {
			age = utils.FormatAge(*pt.Metadata.CreationTimestamp)
		}
		fmt.Fprintf(w, "%s\t%s\t%s\n",
			pt.Metadata.Name,
			resources,
			age)
	}

	return w.Flush()
}
