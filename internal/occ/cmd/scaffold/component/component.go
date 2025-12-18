// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package component

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	extv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"

	"github.com/openchoreo/openchoreo/internal/occ/resources/client"
	scaffold "github.com/openchoreo/openchoreo/internal/scaffold/component"
	"github.com/openchoreo/openchoreo/pkg/cli/types/api"
)

type ScaffoldComponentImpl struct{}

func NewScaffoldComponentImpl() *ScaffoldComponentImpl {
	return &ScaffoldComponentImpl{}
}

func (i *ScaffoldComponentImpl) ScaffoldComponent(params api.ScaffoldComponentParams) error {
	return scaffoldComponent(params)
}

func scaffoldComponent(params api.ScaffoldComponentParams) error {
	// Validate required parameters
	if params.ComponentName == "" {
		return fmt.Errorf("component name is required (--name)")
	}
	if params.ComponentType == "" {
		return fmt.Errorf("component type is required (--type)")
	}
	if params.Organization == "" {
		return fmt.Errorf("organization is required (--organization or set via context)")
	}
	if params.ProjectName == "" {
		return fmt.Errorf("project is required (--project or set via context)")
	}

	// Parse component type (format: workloadType/componentTypeName)
	workloadType, componentTypeName, err := parseComponentType(params.ComponentType)
	if err != nil {
		return err
	}

	// Create API client
	apiClient, err := client.NewAPIClient()
	if err != nil {
		return fmt.Errorf("failed to create API client: %w", err)
	}

	// Create context with timeout for all API requests (ComponentType, Traits, Workflow)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Fetch ComponentType schema
	componentTypeSchemaRaw, err := apiClient.GetComponentTypeSchema(ctx, params.Organization, componentTypeName)
	if err != nil {
		return err
	}
	componentTypeSchema, err := unmarshalSchema(componentTypeSchemaRaw)
	if err != nil {
		return fmt.Errorf("invalid ComponentType schema: %w", err)
	}

	// Fetch Trait schemas if specified
	traitSchemas := make(map[string]*extv1.JSONSchemaProps)
	for _, traitName := range params.Traits {
		traitSchemaRaw, err := apiClient.GetTraitSchema(ctx, params.Organization, traitName)
		if err != nil {
			return err
		}
		traitSchema, err := unmarshalSchema(traitSchemaRaw)
		if err != nil {
			return fmt.Errorf("invalid Trait schema for %q: %w", traitName, err)
		}
		traitSchemas[traitName] = traitSchema
	}

	// Fetch Workflow schema if specified
	var workflowSchema *extv1.JSONSchemaProps
	if params.WorkflowName != "" {
		workflowSchemaRaw, err := apiClient.GetComponentWorkflowSchema(ctx, params.Organization, params.WorkflowName)
		if err != nil {
			return err
		}
		workflowSchema, err = unmarshalSchema(workflowSchemaRaw)
		if err != nil {
			return fmt.Errorf("invalid ComponentWorkflow schema: %w", err)
		}
	}

	// Create generator options
	// Default behavior: include all comments and optional fields
	// --skip-comments disables both structural comments and field descriptions
	// --skip-optional disables optional fields without defaults
	opts := &scaffold.Options{
		ComponentName:             params.ComponentName,
		Namespace:                 params.Organization, // organization name = k8s namespace
		ProjectName:               params.ProjectName,
		IncludeAllFields:          !params.SkipOptional,
		IncludeFieldDescriptions:  !params.SkipComments,
		IncludeStructuralComments: !params.SkipComments,
		IncludeWorkflow:           params.WorkflowName != "",
	}

	// Create generator from schemas
	generator, err := scaffold.NewGeneratorFromSchemas(
		componentTypeName,
		workloadType,
		componentTypeSchema,
		traitSchemas,
		params.WorkflowName,
		workflowSchema,
		opts,
	)
	if err != nil {
		return fmt.Errorf("failed to create generator: %w", err)
	}

	// Generate YAML
	yamlContent, err := generator.Generate()
	if err != nil {
		return fmt.Errorf("failed to generate Component YAML: %w", err)
	}

	// Output
	if params.OutputPath != "" {
		if err := os.WriteFile(params.OutputPath, []byte(yamlContent), 0600); err != nil {
			return fmt.Errorf("failed to write output file %s: %w", params.OutputPath, err)
		}
		fmt.Printf("Component YAML written to %s\n", params.OutputPath)
	} else {
		fmt.Print(yamlContent)
	}

	return nil
}

// parseComponentType parses "workloadType/componentTypeName" format
func parseComponentType(typeStr string) (workloadType, componentTypeName string, err error) {
	if typeStr == "" {
		return "", "", fmt.Errorf("--type is required (format: workloadType/componentTypeName, e.g., deployment/web-app)")
	}

	parts := strings.SplitN(typeStr, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("invalid --type format: expected 'workloadType/componentTypeName' (e.g., deployment/web-app), got %q", typeStr)
	}

	return parts[0], parts[1], nil
}

// unmarshalSchema unmarshals a JSON RawMessage to JSONSchemaProps
func unmarshalSchema(raw *json.RawMessage) (*extv1.JSONSchemaProps, error) {
	var schema extv1.JSONSchemaProps
	if err := json.Unmarshal(*raw, &schema); err != nil {
		return nil, fmt.Errorf("failed to unmarshal schema: %w", err)
	}
	return &schema, nil
}
