// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package scanner

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/yaml"

	"github.com/openchoreo/openchoreo/pkg/fsindex/index"
)

// ParseYAMLFile parses a YAML file and returns all resources found in it
// Supports multi-document YAML files (separated by ---)
func ParseYAMLFile(path string) ([]*index.ResourceEntry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	return ParseYAML(data, path)
}

// ParseYAML parses YAML data and returns all resources found
func ParseYAML(data []byte, sourcePath string) ([]*index.ResourceEntry, error) {
	if len(data) == 0 {
		return nil, nil
	}

	decoder := yaml.NewYAMLOrJSONDecoder(bytes.NewReader(data), 4096)
	var entries []*index.ResourceEntry

	for {
		obj := &unstructured.Unstructured{}
		if err := decoder.Decode(obj); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			// Skip invalid documents but continue parsing
			continue
		}

		// Skip empty documents
		if len(obj.Object) == 0 {
			continue
		}

		// Validate it's a Kubernetes resource (has apiVersion and kind)
		if obj.GetAPIVersion() == "" || obj.GetKind() == "" {
			continue
		}

		entries = append(entries, &index.ResourceEntry{
			Resource: obj,
			FilePath: sourcePath,
		})
	}

	return entries, nil
}

// ValidateResource checks if a resource is valid for indexing
func ValidateResource(entry *index.ResourceEntry) error {
	if entry == nil {
		return fmt.Errorf("entry is nil")
	}

	if entry.Resource == nil {
		return fmt.Errorf("resource is nil")
	}

	if entry.Resource.GetAPIVersion() == "" {
		return fmt.Errorf("resource has no apiVersion")
	}

	if entry.Resource.GetKind() == "" {
		return fmt.Errorf("resource has no kind")
	}

	if entry.Resource.GetName() == "" {
		return fmt.Errorf("resource has no name")
	}

	return nil
}
