// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package apply

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/openchoreo/openchoreo/internal/occ/cmd/config"
	"github.com/openchoreo/openchoreo/internal/occ/resources/client"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
)

// resourceInfo holds extracted metadata from a YAML resource.
type resourceInfo struct {
	kind       string
	apiVersion string
	name       string
	namespace  string
}

// Apply applies resources from the specified file or directory.
func Apply(params Params) error {
	if params.FilePath == "" {
		return fmt.Errorf("file path is required")
	}

	c, err := client.NewClient()
	if err != nil {
		return fmt.Errorf("failed to create API client: %w", err)
	}
	genClient := c.GetClient()

	// Discover all resource files to process
	resourceFiles, err := discoverResourceFiles(params.FilePath)
	if err != nil {
		return fmt.Errorf("failed to discover resources: %w", err)
	}
	if len(resourceFiles) == 0 {
		return fmt.Errorf("no YAML files found in: %s", params.FilePath)
	}

	registry := getResourceRegistry()

	// Resolve default namespace from CLI context
	defaultNamespace := resolveDefaultNamespace()

	ctx := context.Background()
	totalResources := 0
	var errs []string

	for _, filePath := range resourceFiles {
		content, err := readResourceContent(ctx, filePath)
		if err != nil {
			errs = append(errs, fmt.Sprintf("failed to read %s: %v", filePath, err))
			continue
		}

		resources, err := parseYAMLResources(content)
		if err != nil {
			errs = append(errs, fmt.Sprintf("failed to parse %s: %v", filePath, err))
			continue
		}

		totalResources += len(resources)
		for _, resource := range resources {
			if err := applyResource(ctx, genClient, registry, resource, defaultNamespace); err != nil {
				errs = append(errs, err.Error())
			}
		}
	}

	applied := totalResources - len(errs)

	for _, e := range errs {
		fmt.Printf("Error: %s\n", e)
	}

	if len(errs) > 0 {
		fmt.Printf("\nApplied %d resource(s) from %d file(s) with %d error(s)\n",
			applied, len(resourceFiles), len(errs))
		return fmt.Errorf("apply completed with %d error(s)", len(errs))
	}

	fmt.Printf("\nApplied %d resource(s) from %d file(s)\n", applied, len(resourceFiles))
	return nil
}

// extractResourceInfo extracts kind, apiVersion, name, and namespace from a resource map.
func extractResourceInfo(resource map[string]interface{}) (resourceInfo, error) {
	kind, _ := resource["kind"].(string)
	if kind == "" {
		return resourceInfo{}, fmt.Errorf("resource is missing 'kind'")
	}

	apiVersion, _ := resource["apiVersion"].(string)

	metadata, _ := resource["metadata"].(map[string]interface{})
	name, _ := metadata["name"].(string)
	if name == "" {
		return resourceInfo{}, fmt.Errorf("%s: resource is missing 'metadata.name'", kind)
	}

	namespace, _ := metadata["namespace"].(string)

	return resourceInfo{
		kind:       kind,
		apiVersion: apiVersion,
		name:       name,
		namespace:  namespace,
	}, nil
}

// stripKindAndAPIVersion removes kind and apiVersion from the resource map
// and marshals the result to JSON.
func stripKindAndAPIVersion(resource map[string]interface{}) ([]byte, error) {
	delete(resource, "kind")
	delete(resource, "apiVersion")
	return json.Marshal(resource)
}

// applyResource applies a single resource using the registry.
func applyResource(
	ctx context.Context,
	c *gen.ClientWithResponses,
	registry map[string]resourceEntry,
	resource map[string]interface{},
	defaultNamespace string,
) error {
	info, err := extractResourceInfo(resource)
	if err != nil {
		return err
	}

	// Check for read-only kinds
	if readOnlyKinds[info.kind] {
		return fmt.Errorf("%s/%s: kind %q is not supported by apply (read-only resource)", strings.ToLower(info.kind), info.name, info.kind)
	}

	// Validate apiVersion if present
	if info.apiVersion != "" && !strings.Contains(info.apiVersion, apiGroup) {
		return fmt.Errorf("%s/%s: unsupported apiVersion %q (expected group %q)", strings.ToLower(info.kind), info.name, info.apiVersion, apiGroup)
	}

	entry, ok := registry[info.kind]
	if !ok {
		return fmt.Errorf("%s/%s: unsupported kind %q (supported: %s)", strings.ToLower(info.kind), info.name, info.kind, strings.Join(supportedKinds(), ", "))
	}

	// Resolve namespace for namespaced resources
	ns := info.namespace
	if entry.scope == scopeNamespaced {
		if ns == "" {
			ns = defaultNamespace
		}
		// If the namespace is not in the YAML or CLI context, return an error since we don't want to accidentally apply to the wrong namespace
		if ns == "" {
			return fmt.Errorf("%s/%s: namespace is required (set in YAML metadata.namespace or via 'occ config set-context')", strings.ToLower(info.kind), info.name)
		}
	}

	// Prepare JSON body (strip kind and apiVersion)
	jsonBody, err := stripKindAndAPIVersion(resource)
	if err != nil {
		return fmt.Errorf("%s/%s: failed to marshal resource: %w", strings.ToLower(info.kind), info.name, err)
	}

	// Check if resource exists
	statusCode, err := entry.get(ctx, c, ns, info.name)
	if err != nil {
		return fmt.Errorf("%s/%s: failed to check existence: %w", strings.ToLower(info.kind), info.name, err)
	}

	switch statusCode {
	case http.StatusOK:
		// Resource exists — update (or error for create-only)
		if entry.capability == capCreateOnly {
			return fmt.Errorf("%s/%s: resource already exists and cannot be updated (create-only resource)", strings.ToLower(info.kind), info.name)
		}
		code, body, err := entry.update(ctx, c, ns, info.name, bytes.NewReader(jsonBody))
		if err != nil {
			return fmt.Errorf("%s/%s: update failed: %w", strings.ToLower(info.kind), info.name, err)
		}
		if code != http.StatusOK {
			return fmt.Errorf("%s/%s: update failed: %s", strings.ToLower(info.kind), info.name, parseErrorBody(body))
		}
		fmt.Printf("%s/%s configured\n", strings.ToLower(info.kind), info.name)

	case http.StatusNotFound:
		// Resource doesn't exist — create
		code, body, err := entry.create(ctx, c, ns, bytes.NewReader(jsonBody))
		if err != nil {
			return fmt.Errorf("%s/%s: create failed: %w", strings.ToLower(info.kind), info.name, err)
		}
		if code != http.StatusOK && code != http.StatusCreated {
			return fmt.Errorf("%s/%s: create failed: %s", strings.ToLower(info.kind), info.name, parseErrorBody(body))
		}
		fmt.Printf("%s/%s created\n", strings.ToLower(info.kind), info.name)

	default:
		return fmt.Errorf("%s/%s: unexpected status %d when checking existence", strings.ToLower(info.kind), info.name, statusCode)
	}

	return nil
}

// parseErrorBody attempts to extract a human-readable message from an error response body.
func parseErrorBody(body []byte) string {
	if len(body) == 0 {
		return "unknown error (empty response)"
	}
	var errResp gen.ErrorResponse
	if err := json.Unmarshal(body, &errResp); err == nil && errResp.Error != "" {
		return errResp.Error
	}
	// Truncate raw body for readability
	s := string(body)
	if len(s) > 200 {
		s = s[:200] + "..."
	}
	return s
}

// resolveDefaultNamespace returns the namespace from the current CLI context, or empty string.
func resolveDefaultNamespace() string {
	ctx, err := config.GetCurrentContext()
	if err != nil {
		return ""
	}
	return ctx.Namespace
}

// discoverResourceFiles discovers all YAML files to process.
func discoverResourceFiles(path string) ([]string, error) {
	if strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {
		return []string{path}, nil
	}

	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("path %s does not exist", path)
		}
		return nil, fmt.Errorf("error accessing path %s: %w", path, err)
	}

	if !info.IsDir() {
		return []string{path}, nil
	}

	var yamlFiles []string
	err = filepath.Walk(path, func(filePath string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
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

// readResourceContent reads resource content from file or URL.
func readResourceContent(ctx context.Context, filePath string) ([]byte, error) {
	if strings.HasPrefix(filePath, "http://") || strings.HasPrefix(filePath, "https://") {
		// #nosec G107 - URL is validated to be HTTP/HTTPS and is intentional functionality
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, filePath, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to create request for %s: %w", filePath, err)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("failed to download from %s: %w", filePath, err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("failed to download from %s: HTTP %d", filePath, resp.StatusCode)
		}
		return io.ReadAll(resp.Body)
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

// parseYAMLResources parses YAML content that may contain multiple documents.
func parseYAMLResources(content []byte) ([]map[string]interface{}, error) {
	var resources []map[string]interface{}
	decoder := yaml.NewDecoder(bytes.NewReader(content))

	for {
		var resource map[string]interface{}
		if err := decoder.Decode(&resource); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, fmt.Errorf("failed to parse YAML document: %w", err)
		}
		if resource == nil || resource["kind"] == nil {
			continue
		}
		resources = append(resources, resource)
	}
	return resources, nil
}
