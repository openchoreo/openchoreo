// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package project

import (
	"context"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/openchoreo/openchoreo/internal/occ/cmd/utils"
	"github.com/openchoreo/openchoreo/internal/occ/resources/client"
	"github.com/openchoreo/openchoreo/internal/occ/validation"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
)

// Project implements project operations
type Project struct{}

// New creates a new project implementation
func New() *Project {
	return &Project{}
}

// List lists all projects in a namespace
func (l *Project) List(params ListParams) error {
	if err := validation.ValidateParams(validation.CmdList, validation.ResourceProject, params); err != nil {
		return err
	}

	ctx := context.Background()

	c, err := client.NewClient()
	if err != nil {
		return fmt.Errorf("failed to create API client: %w", err)
	}

	result, err := c.ListProjects(ctx, params.Namespace, &gen.ListProjectsParams{})
	if err != nil {
		return fmt.Errorf("failed to list projects: %w", err)
	}

	return printList(result)
}

func printList(list *gen.ProjectList) error {
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
