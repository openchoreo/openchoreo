// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package export

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/openchoreo/openchoreo/internal/labels"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
	"sigs.k8s.io/yaml"
)

var labelsToDrop = map[string]bool{
	labels.LabelKeyProjectUID:     true,
	labels.LabelKeyComponentUID:   true,
	labels.LabelKeyEnvironmentUID: true,
	labels.LabelKeyResourceUID:    true,
	labels.LabelKeyCreatedBy:      true,
	labels.LabelKeyManagedBy:      true,
}

func componentObject(component gen.Component, namespace string) (map[string]any, error) {
	raw, err := json.Marshal(component)
	if err != nil {
		return nil, fmt.Errorf("marshal API response: %w", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var object map[string]any
	if err := decoder.Decode(&object); err != nil {
		return nil, fmt.Errorf("decode API response: %w", err)
	}

	delete(object, "status")
	object["apiVersion"] = componentAPIVersion
	object["kind"] = "Component"
	metadata, ok := object["metadata"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("API response is missing metadata")
	}
	for _, key := range []string{
		"uid", "creationTimestamp", "deletionTimestamp", "deletionGracePeriodSeconds",
		"resourceVersion", "generation", "managedFields", "ownerReferences", "finalizers", "selfLink",
	} {
		delete(metadata, key)
	}
	metadata["namespace"] = namespace
	if labelsMap, ok := metadata["labels"].(map[string]any); ok {
		for key := range labelsMap {
			if labelsToDrop[key] {
				delete(labelsMap, key)
			}
		}
		if len(labelsMap) == 0 {
			delete(metadata, "labels")
		}
	}
	return object, nil
}

func serialize(object map[string]any) ([]byte, error) {
	data, err := yaml.Marshal(object)
	if err != nil {
		return nil, fmt.Errorf("marshal YAML: %w", err)
	}
	return data, nil
}
