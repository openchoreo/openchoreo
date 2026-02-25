// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package deploymentpipeline

import (
	"context"
	"fmt"
	"os"
	"text/tabwriter"

	"sigs.k8s.io/yaml"

	"github.com/openchoreo/openchoreo/internal/occ/cmd/utils"
	"github.com/openchoreo/openchoreo/internal/occ/resources/client"
	"github.com/openchoreo/openchoreo/internal/occ/validation"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
)

// DeploymentPipeline implements deployment pipeline operations
type DeploymentPipeline struct{}

// New creates a new deployment pipeline implementation
func New() *DeploymentPipeline {
	return &DeploymentPipeline{}
}

// List lists all deployment pipelines in a namespace
func (d *DeploymentPipeline) List(params ListParams) error {
	if err := validation.ValidateParams(validation.CmdList, validation.ResourceDeploymentPipeline, params); err != nil {
		return err
	}

	ctx := context.Background()
	c, err := client.NewClient()
	if err != nil {
		return fmt.Errorf("failed to create API client: %w", err)
	}

	result, err := c.ListDeploymentPipelines(ctx, params.Namespace)
	if err != nil {
		return err
	}

	return printList(result)
}

// Get retrieves a single deployment pipeline and outputs it as YAML
func (d *DeploymentPipeline) Get(params GetParams) error {
	if err := validation.ValidateParams(validation.CmdGet, validation.ResourceDeploymentPipeline, params); err != nil {
		return err
	}

	ctx := context.Background()
	c, err := client.NewClient()
	if err != nil {
		return fmt.Errorf("failed to create API client: %w", err)
	}

	result, err := c.GetDeploymentPipeline(ctx, params.Namespace, params.DeploymentPipelineName)
	if err != nil {
		return err
	}

	data, err := yaml.Marshal(result)
	if err != nil {
		return fmt.Errorf("failed to marshal deployment pipeline to YAML: %w", err)
	}

	fmt.Print(string(data))
	return nil
}

// Delete deletes a single deployment pipeline
func (d *DeploymentPipeline) Delete(params DeleteParams) error {
	if err := validation.ValidateParams(validation.CmdDelete, validation.ResourceDeploymentPipeline, params); err != nil {
		return err
	}

	ctx := context.Background()
	c, err := client.NewClient()
	if err != nil {
		return fmt.Errorf("failed to create API client: %w", err)
	}

	if err := c.DeleteDeploymentPipeline(ctx, params.Namespace, params.DeploymentPipelineName); err != nil {
		return err
	}

	fmt.Printf("DeploymentPipeline '%s' deleted\n", params.DeploymentPipelineName)
	return nil
}

func printList(list *gen.DeploymentPipelineList) error {
	if list == nil || len(list.Items) == 0 {
		fmt.Println("No deployment pipelines found")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "NAME\tAGE")

	for _, dp := range list.Items {
		age := ""
		if dp.Metadata.CreationTimestamp != nil {
			age = utils.FormatAge(*dp.Metadata.CreationTimestamp)
		}
		fmt.Fprintf(w, "%s\t%s\n",
			dp.Metadata.Name,
			age)
	}

	return w.Flush()
}
