// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package environment

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

// Environment implements environment operations
type Environment struct{}

// New creates a new environment implementation
func New() *Environment {
	return &Environment{}
}

// List lists all environments in a namespace
func (e *Environment) List(params ListParams) error {
	if err := validation.ValidateParams(validation.CmdList, validation.ResourceEnvironment, params); err != nil {
		return err
	}

	ctx := context.Background()

	c, err := client.NewClient()
	if err != nil {
		return fmt.Errorf("failed to create API client: %w", err)
	}

	result, err := c.ListEnvironments(ctx, params.Namespace, &gen.ListEnvironmentsParams{})
	if err != nil {
		return fmt.Errorf("failed to list environments: %w", err)
	}

	return printList(result)
}

func printList(list *gen.EnvironmentList) error {
	if list == nil || len(list.Items) == 0 {
		fmt.Println("No environments found")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "NAME\tDATA PLANE\tPRODUCTION\tAGE")

	for _, env := range list.Items {
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
