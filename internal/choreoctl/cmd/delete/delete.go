// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package delete

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/openchoreo/openchoreo/internal/choreoctl/resources/client"
	"github.com/openchoreo/openchoreo/internal/choreoctl/validation"
	"github.com/openchoreo/openchoreo/pkg/cli/types/api"
)

// DeleteImpl implements the delete command for Choreo resources
type DeleteImpl struct{}

// NewDeleteImpl creates a new instance of DeleteImpl
func NewDeleteImpl() *DeleteImpl {
	return &DeleteImpl{}
}

// Delete removes resources specified in the given file or directory
func (i *DeleteImpl) Delete(params api.DeleteParams) error {
	if err := validation.ValidateParams(validation.CmdDelete, validation.ResourceDelete, params); err != nil {
		return err
	}

	// Create API client with auto-detection
	apiClient, err := client.NewAPIClient()
	if err != nil {
		return fmt.Errorf("failed to create API client: %w", err)
	}

	// Check API server connectivity
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := apiClient.HealthCheck(ctx); err != nil {
		return fmt.Errorf("OpenChoreo API server not accessible: %w", err)
	}

	// Discover all resource files to process
	resourceFiles, err := discoverResourceFiles(params.FilePath)
	if err != nil {
		return fmt.Errorf("failed to discover resources: %w", err)
	}

	if len(resourceFiles) == 0 {
		return fmt.Errorf("no YAML files found in: %s", params.FilePath)
	}

	totalResources := 0

	// Process each file in reverse order for deletion (dependencies first)
	for i := len(resourceFiles) - 1; i >= 0; i-- {
		filePath := resourceFiles[i]
		fmt.Printf("Processing file: %s\n", filePath)

		// Read resource content
		content, err := readResourceContent(filePath)
		if err != nil {
			return fmt.Errorf("failed to read resource file %s: %w", filePath, err)
		}

		// Parse resources from this file
		resources, err := parseYAMLResources(content)
		if err != nil {
			return fmt.Errorf("failed to parse resources in %s: %w", filePath, err)
		}

		if len(resources) == 0 {
			fmt.Printf("  No resources found in %s\n", filePath)
			continue
		}

		// Delete each resource in this file (reverse order for dependencies)
		for j := len(resources) - 1; j >= 0; j-- {
			resource := resources[j]
			if err := deleteResource(ctx, apiClient, resource, len(resources)-j, len(resources)); err != nil {
				return fmt.Errorf("failed to delete resource from %s: %w", filePath, err)
			}
		}

		totalResources += len(resources)
		fmt.Printf("  Deleted %d resource(s) from %s\n", len(resources), filePath)
	}

	fmt.Printf("\nSuccessfully deleted %d resource(s) from %d file(s) in: %s\n", totalResources, len(resourceFiles), params.FilePath)
	return nil
}

// discoverResourceFiles discovers all YAML files to process (same as apply)
func discoverResourceFiles(path string) ([]string, error) {
	// Check if path is a URL
	if strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {
		return []string{path}, nil
	}

	// Check if path exists
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("path %s does not exist", path)
		}
		return nil, fmt.Errorf("error accessing path %s: %w", path, err)
	}

	// If it's a file, return it directly
	if !info.IsDir() {
		return []string{path}, nil
	}

	// It's a directory - recursively find all YAML files
	var yamlFiles []string
	err = filepath.Walk(path, func(filePath string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip directories
		if info.IsDir() {
			return nil
		}

		// Check for YAML file extensions
		ext := strings.ToLower(filepath.Ext(filePath))
		if ext == ".yaml" || ext == ".yml" {
			yamlFiles = append(yamlFiles, filePath)
		}

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("error walking directory %s: %w", path, err)
	}

	return yamlFiles, nil
}

// readResourceContent reads resource content from file or URL (same as apply)
func readResourceContent(filePath string) ([]byte, error) {
	isRemoteURL := strings.HasPrefix(filePath, "http://") || strings.HasPrefix(filePath, "https://")

	if isRemoteURL {
		// Download from remote URL
		resp, err := http.Get(filePath)
		if err != nil {
			return nil, fmt.Errorf("failed to download from %s: %w", filePath, err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("failed to download from %s: HTTP %d", filePath, resp.StatusCode)
		}

		return io.ReadAll(resp.Body)
	} else {
		// Read from local file
		if _, err := os.Stat(filePath); os.IsNotExist(err) {
			return nil, fmt.Errorf("file %s does not exist", filePath)
		}

		content, err := os.ReadFile(filePath)
		if err != nil {
			if os.IsPermission(err) {
				return nil, fmt.Errorf("permission denied: %s", filePath)
			}
			return nil, fmt.Errorf("error reading file %s: %w", filePath, err)
		}
		return content, nil
	}
}

// parseYAMLResources parses YAML content that may contain multiple documents (same as apply)
func parseYAMLResources(content []byte) ([]map[string]interface{}, error) {
	var resources []map[string]interface{}

	// Split by YAML document separator
	documents := strings.Split(string(content), "---")

	for _, doc := range documents {
		doc = strings.TrimSpace(doc)
		if doc == "" {
			continue // Skip empty documents
		}

		var resource map[string]interface{}
		if err := yaml.Unmarshal([]byte(doc), &resource); err != nil {
			return nil, fmt.Errorf("failed to parse YAML document: %w", err)
		}

		// Skip if it's an empty resource or missing kind
		if resource == nil || resource["kind"] == nil {
			continue
		}

		resources = append(resources, resource)
	}

	return resources, nil
}

// deleteResource deletes a single resource using the API client
func deleteResource(ctx context.Context, apiClient *client.APIClient, resource map[string]interface{}, index, total int) error {
	kind, _ := resource["kind"].(string)
	metadata, _ := resource["metadata"].(map[string]interface{})
	name, _ := metadata["name"].(string)

	fmt.Printf("Deleting %d/%d: %s/%s...", index, total, kind, name)

	resp, err := apiClient.Delete(ctx, resource)
	if err != nil {
		fmt.Printf(" FAILED\n")
		return fmt.Errorf("failed to delete %s/%s: %w", kind, name, err)
	}

	operation := resp.Data.Operation
	if resp.Data.Namespace != "" {
		fmt.Printf(" %s (%s/%s in %s)\n", strings.ToUpper(operation), kind, name, resp.Data.Namespace)
	} else {
		fmt.Printf(" %s (%s/%s)\n", strings.ToUpper(operation), kind, name)
	}

	return nil
}
