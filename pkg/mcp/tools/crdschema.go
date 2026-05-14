// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package tools

import (
	"fmt"
	"sync"

	"sigs.k8s.io/yaml"

	crdefs "github.com/openchoreo/openchoreo/config/crd"
)

// crdSpecSchemas caches the extracted spec schemas so YAML parsing happens only once.
var crdSpecSchemas struct {
	once                       sync.Once
	componentType              map[string]any
	clusterComponentType       map[string]any
	trait                      map[string]any
	clusterTrait               map[string]any
	workflow                   map[string]any
	clusterWorkflow            map[string]any
	authzRole                  map[string]any
	clusterAuthzRole           map[string]any
	authzRoleBinding           map[string]any
	clusterAuthzRoleBinding    map[string]any
	componentTypeErr           error
	clusterComponentTypeErr    error
	traitErr                   error
	clusterTraitErr            error
	workflowErr                error
	clusterWorkflowErr         error
	authzRoleErr               error
	clusterAuthzRoleErr        error
	authzRoleBindingErr        error
	clusterAuthzRoleBindingErr error
}

// ComponentTypeCreationSchema returns the JSON schema for the ComponentType spec,
// extracted from the embedded CRD definition.
func ComponentTypeCreationSchema() (map[string]any, error) {
	crdSpecSchemas.once.Do(parseCRDSchemas)
	return crdSpecSchemas.componentType, crdSpecSchemas.componentTypeErr
}

// ClusterComponentTypeCreationSchema returns the JSON schema for the ClusterComponentType spec,
// extracted from the embedded CRD definition.
func ClusterComponentTypeCreationSchema() (map[string]any, error) {
	crdSpecSchemas.once.Do(parseCRDSchemas)
	return crdSpecSchemas.clusterComponentType, crdSpecSchemas.clusterComponentTypeErr
}

// TraitCreationSchema returns the JSON schema for the Trait spec,
// extracted from the embedded CRD definition.
func TraitCreationSchema() (map[string]any, error) {
	crdSpecSchemas.once.Do(parseCRDSchemas)
	return crdSpecSchemas.trait, crdSpecSchemas.traitErr
}

// ClusterTraitCreationSchema returns the JSON schema for the ClusterTrait spec.
func ClusterTraitCreationSchema() (map[string]any, error) {
	crdSpecSchemas.once.Do(parseCRDSchemas)
	return crdSpecSchemas.clusterTrait, crdSpecSchemas.clusterTraitErr
}

// WorkflowCreationSchema returns the JSON schema for the Workflow spec.
func WorkflowCreationSchema() (map[string]any, error) {
	crdSpecSchemas.once.Do(parseCRDSchemas)
	return crdSpecSchemas.workflow, crdSpecSchemas.workflowErr
}

// ClusterWorkflowCreationSchema returns the JSON schema for the ClusterWorkflow spec.
func ClusterWorkflowCreationSchema() (map[string]any, error) {
	crdSpecSchemas.once.Do(parseCRDSchemas)
	return crdSpecSchemas.clusterWorkflow, crdSpecSchemas.clusterWorkflowErr
}

// AuthzRoleCreationSchema returns the JSON schema for the AuthzRole spec.
func AuthzRoleCreationSchema() (map[string]any, error) {
	crdSpecSchemas.once.Do(parseCRDSchemas)
	return crdSpecSchemas.authzRole, crdSpecSchemas.authzRoleErr
}

// ClusterAuthzRoleCreationSchema returns the JSON schema for the ClusterAuthzRole spec.
func ClusterAuthzRoleCreationSchema() (map[string]any, error) {
	crdSpecSchemas.once.Do(parseCRDSchemas)
	return crdSpecSchemas.clusterAuthzRole, crdSpecSchemas.clusterAuthzRoleErr
}

// AuthzRoleBindingCreationSchema returns the JSON schema for the AuthzRoleBinding spec.
func AuthzRoleBindingCreationSchema() (map[string]any, error) {
	crdSpecSchemas.once.Do(parseCRDSchemas)
	return crdSpecSchemas.authzRoleBinding, crdSpecSchemas.authzRoleBindingErr
}

// ClusterAuthzRoleBindingCreationSchema returns the JSON schema for the ClusterAuthzRoleBinding spec.
func ClusterAuthzRoleBindingCreationSchema() (map[string]any, error) {
	crdSpecSchemas.once.Do(parseCRDSchemas)
	return crdSpecSchemas.clusterAuthzRoleBinding, crdSpecSchemas.clusterAuthzRoleBindingErr
}

func parseCRDSchemas() {
	crdSpecSchemas.componentType, crdSpecSchemas.componentTypeErr = extractSpecSchema(
		"bases/openchoreo.dev_componenttypes.yaml",
	)
	crdSpecSchemas.clusterComponentType, crdSpecSchemas.clusterComponentTypeErr = extractSpecSchema(
		"bases/openchoreo.dev_clustercomponenttypes.yaml",
	)
	crdSpecSchemas.trait, crdSpecSchemas.traitErr = extractSpecSchema(
		"bases/openchoreo.dev_traits.yaml",
	)
	crdSpecSchemas.clusterTrait, crdSpecSchemas.clusterTraitErr = extractSpecSchema(
		"bases/openchoreo.dev_clustertraits.yaml",
	)
	crdSpecSchemas.workflow, crdSpecSchemas.workflowErr = extractSpecSchema(
		"bases/openchoreo.dev_workflows.yaml",
	)
	crdSpecSchemas.clusterWorkflow, crdSpecSchemas.clusterWorkflowErr = extractSpecSchema(
		"bases/openchoreo.dev_clusterworkflows.yaml",
	)
	crdSpecSchemas.authzRole, crdSpecSchemas.authzRoleErr = extractSpecSchema(
		"bases/openchoreo.dev_authzroles.yaml",
	)
	crdSpecSchemas.clusterAuthzRole, crdSpecSchemas.clusterAuthzRoleErr = extractSpecSchema(
		"bases/openchoreo.dev_clusterauthzroles.yaml",
	)
	crdSpecSchemas.authzRoleBinding, crdSpecSchemas.authzRoleBindingErr = extractSpecSchema(
		"bases/openchoreo.dev_authzrolebindings.yaml",
	)
	crdSpecSchemas.clusterAuthzRoleBinding, crdSpecSchemas.clusterAuthzRoleBindingErr = extractSpecSchema(
		"bases/openchoreo.dev_clusterauthzrolebindings.yaml",
	)
}

// extractSpecSchema reads the embedded CRD YAML and extracts
// .spec.versions[0].schema.openAPIV3Schema.properties.spec
func extractSpecSchema(path string) (map[string]any, error) {
	data, err := crdefs.FS.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading embedded CRD %s: %w", path, err)
	}

	var crd map[string]any
	if err := yaml.Unmarshal(data, &crd); err != nil {
		return nil, fmt.Errorf("parsing CRD YAML %s: %w", path, err)
	}

	// Navigate: spec -> versions[0] -> schema -> openAPIV3Schema -> properties -> spec
	specSchema, err := navigateMap(crd,
		"spec", "versions", 0, "schema", "openAPIV3Schema", "properties", "spec",
	)
	if err != nil {
		return nil, fmt.Errorf("extracting spec schema from %s: %w", path, err)
	}

	schema, ok := specSchema.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("spec schema in %s is not an object", path)
	}

	return schema, nil
}

// navigateMap walks a nested map/slice structure following the given path segments.
// String segments index into maps; int segments index into slices.
func navigateMap(data any, path ...any) (any, error) {
	current := data
	for _, seg := range path {
		switch key := seg.(type) {
		case string:
			m, ok := current.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("expected map at key %q, got %T", key, current)
			}
			val, exists := m[key]
			if !exists {
				return nil, fmt.Errorf("key %q not found", key)
			}
			current = val
		case int:
			s, ok := current.([]any)
			if !ok {
				return nil, fmt.Errorf("expected slice at index %d, got %T", key, current)
			}
			if key < 0 || key >= len(s) {
				return nil, fmt.Errorf("index %d out of range (len %d)", key, len(s))
			}
			current = s[key]
		default:
			return nil, fmt.Errorf("unsupported path segment type %T", seg)
		}
	}
	return current, nil
}
