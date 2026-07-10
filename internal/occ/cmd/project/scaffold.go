// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package project

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	extv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"

	"github.com/openchoreo/openchoreo/internal/occ/cmd/utils"
	"github.com/openchoreo/openchoreo/internal/occ/cmdutil"
	projectscaffold "github.com/openchoreo/openchoreo/internal/scaffold/project"
)

// Scaffold generates a Project YAML from a (Cluster)ProjectType, and by default
// one ProjectReleaseBinding per environment in the deployment pipeline.
func (p *Project) Scaffold(params ScaffoldParams) error {
	if err := validateScaffoldParams(params); err != nil {
		return err
	}

	ctx := context.Background()

	kind, typeName, schemaRaw, err := p.fetchProjectTypeSchema(ctx, params)
	if err != nil {
		return err
	}
	schema, err := unmarshalSchema(schemaRaw)
	if err != nil {
		return fmt.Errorf("invalid %s schema: %w", kind, err)
	}

	// The generated Project references the pipeline via spec.deploymentPipelineRef,
	// so it must resolve even when bindings are not being generated.
	pipeline, err := p.client.GetDeploymentPipeline(ctx, params.Namespace, params.DeploymentPipeline)
	if err != nil {
		return fmt.Errorf("failed to resolve deployment pipeline %q: %w", params.DeploymentPipeline, err)
	}

	// A pipeline with no environments is valid: emit the Project alone and say why.
	var environments []string
	var bindingsNote string
	if !params.NoBindings {
		environments = utils.ExpandEnvironments(pipeline)
		if len(environments) == 0 {
			bindingsNote = fmt.Sprintf(
				"No ProjectReleaseBindings were generated: deployment pipeline %q defines no environments.",
				params.DeploymentPipeline)
		}
	}

	gen := projectscaffold.NewGenerator(schema, &projectscaffold.Options{
		ProjectName:               params.ProjectName,
		Namespace:                 params.Namespace,
		ProjectTypeKind:           kind,
		ProjectTypeName:           typeName,
		DeploymentPipeline:        params.DeploymentPipeline,
		Environments:              environments,
		BindingsNote:              bindingsNote,
		IncludeAllFields:          !params.SkipOptional,
		IncludeFieldDescriptions:  !params.SkipComments,
		IncludeStructuralComments: !params.SkipComments,
	})

	out, err := gen.Generate()
	if err != nil {
		return err
	}

	if params.OutputPath != "" {
		if err := os.WriteFile(params.OutputPath, []byte(out), 0600); err != nil {
			return fmt.Errorf("failed to write output file %s: %w", params.OutputPath, err)
		}
		fmt.Printf("Project YAML written to %s\n", params.OutputPath)
		return nil
	}

	fmt.Print(out)
	return nil
}

// fetchProjectTypeSchema returns the type kind, name, and its parameter schema.
func (p *Project) fetchProjectTypeSchema(ctx context.Context, params ScaffoldParams) (string, string, *json.RawMessage, error) {
	if params.ClusterProjectType != "" {
		raw, err := p.client.GetClusterProjectTypeSchema(ctx, params.ClusterProjectType)
		return "ClusterProjectType", params.ClusterProjectType, raw, err
	}
	raw, err := p.client.GetProjectTypeSchema(ctx, params.Namespace, params.ProjectType)
	return "ProjectType", params.ProjectType, raw, err
}

func validateScaffoldParams(params ScaffoldParams) error {
	if err := cmdutil.RequireFields("scaffold", "project", map[string]string{
		"namespace": params.Namespace,
		"name":      params.ProjectName,
	}); err != nil {
		return err
	}
	if params.ProjectType != "" && params.ClusterProjectType != "" {
		return fmt.Errorf("--projecttype and --clusterprojecttype are mutually exclusive")
	}
	if params.ProjectType == "" && params.ClusterProjectType == "" {
		return fmt.Errorf("one of --projecttype or --clusterprojecttype is required")
	}
	if params.DeploymentPipeline == "" {
		return fmt.Errorf("deployment pipeline is required")
	}
	return nil
}

// unmarshalSchema unmarshals a JSON RawMessage to JSONSchemaProps.
func unmarshalSchema(raw *json.RawMessage) (*extv1.JSONSchemaProps, error) {
	if raw == nil {
		return nil, nil
	}
	var schema extv1.JSONSchemaProps
	if err := json.Unmarshal(*raw, &schema); err != nil {
		return nil, fmt.Errorf("failed to unmarshal schema: %w", err)
	}
	return &schema, nil
}
