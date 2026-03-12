// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"encoding/json"
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/runtime"
)

// This file contains the all the annotations that are used to store Choreo specific the metadata in the Kubernetes objects.

const (
	AnnotationKeyDisplayName = "openchoreo.dev/display-name"
	AnnotationKeyDescription = "openchoreo.dev/description"

	// SchemaExtensionComponentParameterRepositoryPrefix is the common prefix for all openAPIV3Schema
	// x- extension keys that mark component repository parameter fields (set to true on the property).
	// The suffix after the prefix is used as the role key in the map returned by ExtractComponentRepositoryPaths
	// (e.g. "url", "branch", "commit", "app-path", "secret-ref").
	SchemaExtensionComponentParameterRepositoryPrefix    = "x-openchoreo-component-parameter-repository-"
	SchemaExtensionComponentParameterRepositoryURL       = SchemaExtensionComponentParameterRepositoryPrefix + "url"
	SchemaExtensionComponentParameterRepositoryBranch    = SchemaExtensionComponentParameterRepositoryPrefix + "branch"
	SchemaExtensionComponentParameterRepositoryCommit    = SchemaExtensionComponentParameterRepositoryPrefix + "commit"
	SchemaExtensionComponentParameterRepositoryAppPath   = SchemaExtensionComponentParameterRepositoryPrefix + "app-path"
	SchemaExtensionComponentParameterRepositorySecretRef = SchemaExtensionComponentParameterRepositoryPrefix + "secret-ref"
)

// ExtractComponentRepositoryPaths scans an openAPIV3Schema RawExtension for boolean
// x-openchoreo-component-parameter-repository-* extension keys
// (e.g. "x-openchoreo-component-parameter-repository-url",
// "x-openchoreo-component-parameter-repository-branch", "-commit", "-app-path", "-secret-ref")
// and returns a map from the key suffix (e.g. "url", "branch", "commit", "app-path", "secret-ref")
// to the dotted property path of the annotated field within the parameters object
// (e.g. "repository.url", "repository.revision.branch").
func ExtractComponentRepositoryPaths(schema *runtime.RawExtension) (map[string]string, error) {
	result := make(map[string]string)
	if schema == nil || schema.Raw == nil {
		return result, nil
	}
	var schemaObj map[string]interface{}
	if err := json.Unmarshal(schema.Raw, &schemaObj); err != nil {
		return nil, fmt.Errorf("failed to unmarshal schema: %w", err)
	}
	if err := walkSchemaForComponentRepository(schemaObj, "", result); err != nil {
		return nil, err
	}
	return result, nil
}

// walkSchemaForComponentRepository recursively walks an openAPIV3Schema properties tree,
// collecting all fields that carry an x-openchoreo-component-parameter-repository-* extension.
// It returns an error if the same role appears on more than one field.
func walkSchemaForComponentRepository(schema map[string]interface{}, prefix string, result map[string]string) error {
	for key, val := range schema {
		if !strings.HasPrefix(key, SchemaExtensionComponentParameterRepositoryPrefix) {
			continue
		}
		if enabled, ok := val.(bool); ok && enabled && prefix != "" {
			role := strings.TrimPrefix(key, SchemaExtensionComponentParameterRepositoryPrefix)
			if _, exists := result[role]; exists {
				return fmt.Errorf("duplicate %s extension found at path %q (role %q already mapped to %q)", key, prefix, role, result[role])
			}
			result[role] = prefix
		}
	}
	props, ok := schema["properties"].(map[string]interface{})
	if !ok {
		return nil
	}
	for propName, propVal := range props {
		propSchema, ok := propVal.(map[string]interface{})
		if !ok {
			continue
		}
		childPath := propName
		if prefix != "" {
			childPath = prefix + "." + propName
		}
		if err := walkSchemaForComponentRepository(propSchema, childPath, result); err != nil {
			return err
		}
	}
	return nil
}
