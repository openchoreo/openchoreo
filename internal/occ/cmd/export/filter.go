// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package export

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
)

const (
	lastAppliedAnnotation = "kubectl.kubernetes.io/last-applied-configuration"
	restartedAtAnnotation = "openchoreo.dev/restartedAt"
)

// resourceDocument holds a JSON object decoded once with exact numeric literals.
type resourceDocument map[string]any

// Filter prepares independent maps for export. It skips objects marked
// for deletion, removes top-level status, keeps only name/namespace/labels/
// annotations in metadata, and removes the two transient annotations above.
// Specs, nested documents, and all other labels and annotations are preserved.
// Numbers remain json.Number values from conversion through YAML serialization.
func Filter(ctx context.Context, resources []Resource) ([]Resource, error) {
	filtered := make([]Resource, 0, len(resources))
	for _, resource := range resources {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		object, include, err := filterResource(resource)
		if err != nil {
			return nil, fmt.Errorf("filter %s %q in namespace %q: %w", resource.Kind, resource.Name, resource.Namespace, err)
		}
		if include {
			resource.Object = object
			filtered = append(filtered, resource)
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return filtered, nil
}

func copyResourceDocument(resource Resource) (resourceDocument, error) {
	data, err := json.Marshal(resource.Object)
	if err != nil {
		return nil, fmt.Errorf("copy API object: %w", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var document resourceDocument
	if err := decoder.Decode(&document); err != nil {
		return nil, fmt.Errorf("read API object: %w", err)
	}
	if document == nil {
		return nil, fmt.Errorf("API object is required")
	}
	// The generated Namespace model has no Kubernetes type metadata.
	if resource.Kind == "Namespace" {
		document["apiVersion"] = "v1"
		document["kind"] = resource.Kind
	}
	return document, nil
}

func filterResource(resource Resource) (resourceDocument, bool, error) {
	document, err := copyResourceDocument(resource)
	if err != nil {
		return nil, false, err
	}
	metadata, ok := document["metadata"].(map[string]any)
	if !ok {
		if document["metadata"] == nil {
			return nil, false, fmt.Errorf("read metadata: metadata is required")
		}
		return nil, false, fmt.Errorf("read metadata: must be an object")
	}
	// Inspect deletion state before dropping server metadata. Omission or JSON
	// null means the API has not marked this object for deletion.
	if metadata["deletionTimestamp"] != nil {
		return nil, false, nil
	}
	for field := range metadata {
		switch field {
		case "name", "namespace", "labels", "annotations":
		default:
			delete(metadata, field)
		}
	}
	if err := filterAnnotations(metadata); err != nil {
		return nil, false, err
	}
	delete(document, "status")
	return document, true, nil
}

func filterAnnotations(metadata map[string]any) error {
	value := metadata["annotations"]
	if value == nil {
		return nil
	}
	annotations, ok := value.(map[string]any)
	if !ok {
		return fmt.Errorf("read metadata.annotations: must be an object")
	}
	delete(annotations, lastAppliedAnnotation)
	delete(annotations, restartedAtAnnotation)
	return nil
}
