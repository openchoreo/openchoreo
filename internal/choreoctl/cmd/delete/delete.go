/*
 * Copyright (c) 2025, WSO2 Inc. (http://www.wso2.org) All Rights Reserved.
 *
 * WSO2 Inc. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License.
 * You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied. See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

// Package delete provides functionality to delete Choreo resources
package delete

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/serializer/yaml"

	"github.com/choreo-idp/choreo/internal/choreoctl/cmd/config"
	"github.com/choreo-idp/choreo/pkg/cli/types/api"
)

// DeleteImpl implements the delete command for Choreo resources
type DeleteImpl struct{}

// Resource dependency order from leaf to root based on the Choreo resource relationships
var resourceDeleteOrder = []string{
	"endpoint",           // Most leaf-level resource
	"deploymentrevision", // Should be deleted before its parent deployment
	"deployment",         // Should be deleted before deploymenttrack
	"deployableartifact", // Should be deleted after deployment but before build
	"build",              // Should be deleted after deployableartifact but before component
	"deploymenttrack",    // Should be deleted after deployment but before component
	"configurationgroup", // Referenced by deployableartifact
	"component",          // Should be deleted after its child resources
	"environment",        // Should be deleted after deployment
	"deploymentpipeline", // Should be deleted after environments
	"dataplane",          // Should be deleted after environments
	"project",            // Should be deleted after all components
	"organization",       // Root level resource, deleted last
}

// NewDeleteImpl creates a new instance of DeleteImpl
func NewDeleteImpl() *DeleteImpl {
	return &DeleteImpl{}
}

// Delete removes resources specified in the given file
func (i *DeleteImpl) Delete(params api.DeleteParams) error {
	if params.FilePath == "" {
		return fmt.Errorf("file path is required")
	}

	// TODO: Properly fix this, This is a quick fix to support remote URLs for samples
	isRemoteURL := strings.HasPrefix(params.FilePath, "http://") ||
		strings.HasPrefix(params.FilePath, "https://")

	var contentBytes []byte

	if !isRemoteURL {
		if _, err := os.Stat(params.FilePath); os.IsNotExist(err) {
			return fmt.Errorf("file %s does not exist", params.FilePath)
		}

		fileBytes, err := os.ReadFile(params.FilePath)
		if err != nil {
			if os.IsPermission(err) {
				return fmt.Errorf("permission denied: %s", params.FilePath)
			}
			return fmt.Errorf("error reading file: %s", params.FilePath)
		}
		contentBytes = fileBytes
	} else {
		// Read the file bytes from the remote URL
		resp, err := http.Get(params.FilePath)
		if err != nil {
			return fmt.Errorf("failed to GET %s: %w", params.FilePath, err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("failed to GET %s: status code %d", params.FilePath, resp.StatusCode)
		}

		remoteBytes, err := io.ReadAll(resp.Body)
		if err != nil {
			return fmt.Errorf("failed to read response from %s: %w", params.FilePath, err)
		}
		contentBytes = remoteBytes
	}

	kubeconfig, context, err := config.GetStoredKubeConfigValues()
	if err != nil {
		return fmt.Errorf("failed to get kubeconfig values: %w", err)
	}

	if isMultiDocYAML(contentBytes) {
		return deleteResourcesInOrder(contentBytes, kubeconfig, context, params.Wait)
	}

	deleteArgs := []string{"delete", "-f", params.FilePath}
	if params.Wait {
		deleteArgs = append(deleteArgs, "--wait")
	}

	err = executeKubectl(kubeconfig, context, deleteArgs, fmt.Sprintf("Deleting resources from %s", params.FilePath))
	if err != nil {
		return err
	}

	fmt.Printf("Resources deleted successfully from %s\n", params.FilePath)
	return nil
}

// executeKubectl executes kubectl command with the given arguments
func executeKubectl(kubeconfig, context string, args []string, description string) error {
	kubectlArgs := []string{
		"--kubeconfig", kubeconfig,
		"--context", context,
	}
	kubectlArgs = append(kubectlArgs, args...)

	cmd := exec.Command("kubectl", kubectlArgs...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	fmt.Printf("%s...\n", description)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to execute kubectl command: %w", err)
	}
	return nil
}

// isMultiDocYAML checks if the file contains multiple YAML documents
func isMultiDocYAML(content []byte) bool {
	return bytes.Count(content, []byte("---")) > 0
}

// deleteResourcesInOrder processes multiple resources and deletes them in dependency order
func deleteResourcesInOrder(fileBytes []byte, kubeconfig, context string, wait bool) error {
	resources, err := parseResources(fileBytes)
	if err != nil {
		return fmt.Errorf("error parsing resources: %w", err)
	}

	if len(resources) == 0 {
		return fmt.Errorf("no valid resources found in the file")
	}

	resourcesByKind := groupResourcesByKind(resources)

	tempDir, err := os.MkdirTemp("", "choreo-delete")
	if err != nil {
		return fmt.Errorf("failed to create temporary directory: %w", err)
	}
	defer os.RemoveAll(tempDir)

	processedKinds := make(map[string]bool)

	// Process resources according to defined order first
	for _, kind := range resourceDeleteOrder {
		lowercaseKind := strings.ToLower(kind)
		resourcesToDelete, exists := resourcesByKind[lowercaseKind]
		if !exists {
			continue
		}

		fmt.Printf("Deleting %s resources...\n", kind)
		if err := deleteResourcesOfKind(resourcesToDelete, tempDir, kubeconfig, context, wait); err != nil {
			return err
		}

		processedKinds[lowercaseKind] = true
	}

	// Process any remaining unordered kinds
	for kind, resourcesToDelete := range resourcesByKind {
		if processedKinds[kind] {
			continue
		}

		fmt.Printf("Deleting %s resources (unordered)...\n", kind)
		if err := deleteResourcesOfKind(resourcesToDelete, tempDir, kubeconfig, context, wait); err != nil {
			return err
		}
	}

	return nil
}

// deleteResourcesOfKind deletes all resources of a specific kind
func deleteResourcesOfKind(resources []*unstructured.Unstructured, tempDir, kubeconfig, context string, wait bool) error {
	for _, resource := range resources {
		tempFile := fmt.Sprintf("%s/%s-%s.yaml", tempDir, strings.ToLower(resource.GetKind()), resource.GetName())
		resourceYAML, err := unstructuredToYAML(resource)
		if err != nil {
			return fmt.Errorf("failed to convert resource to YAML for %s/%s: %w", resource.GetKind(), resource.GetName(), err)
		}

		if err := os.WriteFile(tempFile, []byte(resourceYAML), 0600); err != nil {
			return fmt.Errorf("failed to write temporary file for %s/%s: %w", resource.GetKind(), resource.GetName(), err)
		}

		deleteArgs := []string{"delete", "-f", tempFile}
		if wait {
			deleteArgs = append(deleteArgs, "--wait")
		}

		description := fmt.Sprintf("Deleting %s/%s", resource.GetKind(), resource.GetName())
		if err := executeKubectl(kubeconfig, context, deleteArgs, description); err != nil {
			return fmt.Errorf("failed to delete %s/%s: %w", resource.GetKind(), resource.GetName(), err)
		}

		fmt.Printf("Deleted %s/%s\n", resource.GetKind(), resource.GetName())
	}
	return nil
}

// parseResources parses the YAML file into a list of unstructured resources
func parseResources(fileBytes []byte) ([]*unstructured.Unstructured, error) {
	var resources []*unstructured.Unstructured
	decoder := yaml.NewDecodingSerializer(unstructured.UnstructuredJSONScheme)
	reader := bufio.NewReader(bytes.NewReader(fileBytes))

	var buffer bytes.Buffer
	for {
		line, err := reader.ReadString('\n')
		if err != nil && err != io.EOF {
			return nil, err
		}

		if strings.HasPrefix(line, "---") || err == io.EOF {
			if buffer.Len() > 0 {
				content := strings.TrimSpace(buffer.String())
				if content != "" {
					obj := &unstructured.Unstructured{}
					if _, _, err := decoder.Decode([]byte(content), nil, obj); err == nil {
						if obj.GetKind() != "" {
							resources = append(resources, obj)
						}
					}
				}
				buffer.Reset()
			}

			if err == io.EOF {
				break
			}
			continue
		}

		buffer.WriteString(line)
	}

	return resources, nil
}

// groupResourcesByKind groups resources by their lowercase kind
func groupResourcesByKind(resources []*unstructured.Unstructured) map[string][]*unstructured.Unstructured {
	result := make(map[string][]*unstructured.Unstructured)
	for _, res := range resources {
		kind := strings.ToLower(res.GetKind())
		result[kind] = append(result[kind], res)
	}
	return result
}

// unstructuredToYAML converts an unstructured resource back to YAML
func unstructuredToYAML(obj *unstructured.Unstructured) (string, error) {
	jsonBytes, err := obj.MarshalJSON()
	if err != nil {
		return "", fmt.Errorf("failed to marshal resource to JSON: %w", err)
	}

	cmd := exec.Command("kubectl", "get", "-f", "-", "-o", "yaml")
	cmd.Stdin = bytes.NewReader(jsonBytes)
	yamlBytes, err := cmd.Output()
	if err != nil {
		var stderr []byte
		// Use errors.As instead of type assertion (addressing errorlint)
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			stderr = exitErr.Stderr
		}
		return "", fmt.Errorf("failed to convert resource to YAML: %w\n%s", err, stderr)
	}

	return string(yamlBytes), nil
}
