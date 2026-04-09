// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package environment

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

// Environment implements environment operations
type Environment struct {
	client client.Interface
}

// New creates a new environment implementation
func New(c client.Interface) *Environment {
	return &Environment{client: c}
}

// List lists all environments in a namespace
func (e *Environment) List(params ListParams) error {
	if err := cmdutil.RequireFields("list", "environment", map[string]string{"namespace": params.Namespace}); err != nil {
		return err
	}

	ctx := context.Background()

	items, err := pagination.FetchAll(func(limit int, cursor string) ([]gen.Environment, string, error) {
		p := &gen.ListEnvironmentsParams{}
		p.Limit = &limit
		if cursor != "" {
			p.Cursor = &cursor
		}
		result, err := e.client.ListEnvironments(ctx, params.Namespace, p)
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

// Get retrieves a single environment and outputs it as YAML
func (e *Environment) Get(params GetParams) error {
	if err := cmdutil.RequireFields("get", "environment", map[string]string{"namespace": params.Namespace}); err != nil {
		return err
	}

	ctx := context.Background()

	result, err := e.client.GetEnvironment(ctx, params.Namespace, params.EnvironmentName)
	if err != nil {
		return err
	}

	data, err := yaml.Marshal(result)
	if err != nil {
		return fmt.Errorf("failed to marshal environment to YAML: %w", err)
	}

	fmt.Print(string(data))
	return nil
}

// Delete deletes a single environment
func (e *Environment) Delete(params DeleteParams) error {
	if err := cmdutil.RequireFields("delete", "environment", map[string]string{"namespace": params.Namespace, "name": params.EnvironmentName}); err != nil {
		return err
	}

	ctx := context.Background()

	if err := e.client.DeleteEnvironment(ctx, params.Namespace, params.EnvironmentName); err != nil {
		return err
	}

	fmt.Printf("Environment '%s' deleted\n", params.EnvironmentName)
	return nil
}

func printList(items []gen.Environment) error {
	if len(items) == 0 {
		fmt.Println("No environments found")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "NAME\tDATA PLANE\tPRODUCTION\tAGE")

	for _, env := range items {
		dataPlane := ""
		if env.Spec != nil && env.Spec.DataPlaneRef != nil {
			dataPlane = fmt.Sprintf("%s/%s", env.Spec.DataPlaneRef.Kind, env.Spec.DataPlaneRef.Name)
		}
		production := "false"
		if env.Spec != nil && env.Spec.IsProduction != nil && *env.Spec.IsProduction {
			production = "true"
		}
		age := ""
		if env.Metadata.CreationTimestamp != nil {
			age = utils.FormatAge(*env.Metadata.CreationTimestamp)
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n",
			env.Metadata.Name,
			dataPlane,
			production,
			age)
	}

	return w.Flush()
}
