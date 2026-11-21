// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package export

import (
	"context"
	"encoding/json"
	"fmt"
)

const (
	lastAppliedAnnotation = "kubectl.kubernetes.io/last-applied-configuration"
	restartedAtAnnotation = "openchoreo.dev/restartedAt"
)

// Filter prepares independent JSON documents for export. It skips objects marked
// for deletion, removes top-level status, keeps only name/namespace/labels/
// annotations in metadata, and removes the two transient annotations above.
// Specs, nested documents, and all other labels and annotations are preserved.
// Raw messages retain numbers without decoding them into floating-point values.
func Filter(ctx context.Context, resources []Resource) ([]Resource, error) {
	filtered := make([]Resource, 0, len(resources))
	for _, resource := range resources {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		object, include, err := filterResource(resource.Object)
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

func filterResource(object any) (map[string]json.RawMessage, bool, error) {
	data, err := json.Marshal(object)
	if err != nil {
		return nil, false, fmt.Errorf("copy API object: %w", err)
	}
	var document map[string]json.RawMessage
	if err := json.Unmarshal(data, &document); err != nil {
		return nil, false, fmt.Errorf("read API object: %w", err)
	}
	if document == nil {
		return nil, false, fmt.Errorf("API object is required")
	}
	var metadata map[string]json.RawMessage
	if err := json.Unmarshal(document["metadata"], &metadata); err != nil {
		return nil, false, fmt.Errorf("read metadata: %w", err)
	}
	if metadata == nil {
		return nil, false, fmt.Errorf("metadata is required")
	}
	// Inspect deletion state before dropping server metadata. Omission or JSON
	// null means the API has not marked this object for deletion.
	if timestamp, exists := metadata["deletionTimestamp"]; exists && string(timestamp) != "null" {
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
	document["metadata"], err = json.Marshal(metadata)
	if err != nil {
		return nil, false, fmt.Errorf("encode filtered metadata: %w", err)
	}
	delete(document, "status")
	return document, true, nil
}

func filterAnnotations(metadata map[string]json.RawMessage) error {
	data, exists := metadata["annotations"]
	if !exists {
		return nil
	}
	var annotations map[string]json.RawMessage
	if err := json.Unmarshal(data, &annotations); err != nil {
		return fmt.Errorf("read metadata.annotations: %w", err)
	}
	if annotations == nil {
		return nil
	}
	delete(annotations, lastAppliedAnnotation)
	delete(annotations, restartedAtAnnotation)
	filtered, err := json.Marshal(annotations)
	if err != nil {
		return fmt.Errorf("encode filtered annotations: %w", err)
	}
	metadata["annotations"] = filtered
	return nil
}
