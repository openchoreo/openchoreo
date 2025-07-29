// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package synth

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/pkg/cli/types/api"
)

// WorkloadDescriptor represents the structure of a workload.yaml file
// This is the developer-maintained descriptor alongside source code
type WorkloadDescriptor struct {
	APIVersion  string                         `yaml:"apiVersion"`
	Metadata    WorkloadDescriptorMetadata     `yaml:"metadata"`
	Endpoints   []WorkloadDescriptorEndpoint   `yaml:"endpoints,omitempty"`
	Connections []WorkloadDescriptorConnection `yaml:"connections,omitempty"`
}

type WorkloadDescriptorMetadata struct {
	Name string `yaml:"name"`
}

type WorkloadDescriptorEndpoint struct {
	Name       string `yaml:"name"`
	Port       int32  `yaml:"port"`
	Type       string `yaml:"type"`
	SchemaFile string `yaml:"schemaFile,omitempty"`
	Context    string `yaml:"context,omitempty"`
}

type WorkloadDescriptorConnection struct {
	Name   string                             `yaml:"name"`
	Type   string                             `yaml:"type"`
	Params map[string]string                  `yaml:"params,omitempty"`
	Inject WorkloadDescriptorConnectionInject `yaml:"inject"`
}

type WorkloadDescriptorConnectionInject struct {
	Env []WorkloadDescriptorConnectionEnvVar `yaml:"env"`
}

type WorkloadDescriptorConnectionEnvVar struct {
	Name  string `yaml:"name"`
	Value string `yaml:"value"`
}

// ConversionParams holds the parameters needed for workload conversion
type ConversionParams struct {
	OrganizationName string
	ProjectName      string
	ComponentName    string
	ImageURL         string
}

// ConvertWorkloadDescriptorToWorkloadCR converts a workload.yaml descriptor to a Workload CR
func ConvertWorkloadDescriptorToWorkloadCR(descriptorPath string, params api.CreateWorkloadParams) (*openchoreov1alpha1.Workload, error) {
	// Read the workload descriptor file
	descriptor, err := readWorkloadDescriptor(descriptorPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read workload descriptor: %w", err)
	}

	// Validate conversion parameters
	if err := validateConversionParams(params); err != nil {
		return nil, fmt.Errorf("invalid conversion parameters: %w", err)
	}

	// Convert descriptor to Workload CR with the base directory for resolving relative paths
	workload, err := convertDescriptorToWorkload(descriptor, params, descriptorPath)
	if err != nil {
		return nil, fmt.Errorf("failed to convert descriptor to workload CR: %w", err)
	}

	return workload, nil
}

func readSchemaFile(path string) (string, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("failed to read schema file %s: %w", path, err)
	}
	return string(content), nil
}

func readWorkloadDescriptor(path string) (*WorkloadDescriptor, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open file %s: %w", path, err)
	}
	defer file.Close()

	return readWorkloadDescriptorFromReader(file)
}

func readWorkloadDescriptorFromReader(reader io.Reader) (*WorkloadDescriptor, error) {
	var descriptor WorkloadDescriptor
	decoder := yaml.NewDecoder(reader)
	if err := decoder.Decode(&descriptor); err != nil {
		return nil, fmt.Errorf("failed to decode YAML: %w", err)
	}

	return &descriptor, nil
}

func validateConversionParams(params api.CreateWorkloadParams) error {
	if params.OrganizationName == "" {
		return fmt.Errorf("organization name is required")
	}
	if params.ProjectName == "" {
		return fmt.Errorf("project name is required")
	}
	if params.ComponentName == "" {
		return fmt.Errorf("component name is required")
	}
	if params.ImageURL == "" {
		return fmt.Errorf("image URL is required")
	}
	return nil
}

// createBaseWorkload creates the basic workload structure with common fields
func createBaseWorkload(workloadName string, params api.CreateWorkloadParams) *openchoreov1alpha1.Workload {
	workload := &openchoreov1alpha1.Workload{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "openchoreo.dev/v1alpha1",
			Kind:       "Workload",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: workloadName,
		},
		Spec: openchoreov1alpha1.WorkloadSpec{
			Owner: openchoreov1alpha1.WorkloadOwner{
				ProjectName:   params.ProjectName,
				ComponentName: params.ComponentName,
			},
			WorkloadTemplateSpec: openchoreov1alpha1.WorkloadTemplateSpec{
				Containers: map[string]openchoreov1alpha1.Container{
					"main": {
						Image: params.ImageURL,
					},
				},
			},
		},
	}

	return workload
}

func convertDescriptorToWorkload(descriptor *WorkloadDescriptor, params api.CreateWorkloadParams, descriptorPath string) (*openchoreov1alpha1.Workload, error) {
	// Determine workload name
	workloadName := descriptor.Metadata.Name
	if workloadName == "" {
		return nil, fmt.Errorf("workload name must be provided either in params or descriptor metadata")
	}

	// Create the base workload structure
	workload := createBaseWorkload(workloadName, params)

	// Add endpoints from descriptor if present
	if err := addEndpointsFromDescriptor(workload, descriptor, descriptorPath); err != nil {
		return nil, fmt.Errorf("failed to add endpoints: %w", err)
	}

	// Add connections from descriptor if present
	if err := addConnectionsFromDescriptor(workload, descriptor, descriptorPath); err != nil {
		return nil, fmt.Errorf("failed to add connections: %w", err)
	}

	return workload, nil
}

// addEndpointsFromDescriptor adds endpoints from the descriptor to the workload
func addEndpointsFromDescriptor(workload *openchoreov1alpha1.Workload, descriptor *WorkloadDescriptor, descriptorPath string) error {
	if len(descriptor.Endpoints) == 0 {
		return nil
	}

	workload.Spec.Endpoints = make(map[string]openchoreov1alpha1.WorkloadEndpoint)
	for _, descriptorEndpoint := range descriptor.Endpoints {
		endpoint := openchoreov1alpha1.WorkloadEndpoint{
			Type: openchoreov1alpha1.EndpointType(descriptorEndpoint.Type),
			Port: descriptorEndpoint.Port,
		}

		// Set schema if provided
		if descriptorEndpoint.SchemaFile != "" {
			// Resolve schema file path relative to the workload descriptor directory
			baseDir := filepath.Dir(descriptorPath)
			schemaFilePath := filepath.Join(baseDir, descriptorEndpoint.SchemaFile)

			// Read schema file content and inline it
			schemaContent, err := readSchemaFile(schemaFilePath)
			if err != nil {
				return fmt.Errorf("failed to read schema file %s: %w", schemaFilePath, err)
			}

			endpoint.Schema = &openchoreov1alpha1.Schema{
				Type:    descriptorEndpoint.Type,
				Content: schemaContent,
			}
		}

		workload.Spec.Endpoints[descriptorEndpoint.Name] = endpoint
	}
	return nil
}

// addConnectionsFromDescriptor adds connections from the descriptor to the workload
func addConnectionsFromDescriptor(workload *openchoreov1alpha1.Workload, descriptor *WorkloadDescriptor, descriptorPath string) error {
	if len(descriptor.Connections) == 0 {
		return nil
	}

	workload.Spec.Connections = make(map[string]openchoreov1alpha1.WorkloadConnection)
	for _, descriptorConnection := range descriptor.Connections {
		// Convert environment variables
		envVars := make([]openchoreov1alpha1.WorkloadConnectionEnvVar, len(descriptorConnection.Inject.Env))
		for i, envVar := range descriptorConnection.Inject.Env {
			envVars[i] = openchoreov1alpha1.WorkloadConnectionEnvVar{
				Name:  envVar.Name,
				Value: envVar.Value,
			}
		}

		connection := openchoreov1alpha1.WorkloadConnection{
			Type:   descriptorConnection.Type,
			Params: descriptorConnection.Params,
			Inject: openchoreov1alpha1.WorkloadConnectionInject{
				Env: envVars,
			},
		}

		workload.Spec.Connections[descriptorConnection.Name] = connection
	}
	return nil
}

// CreateBasicWorkload creates a basic Workload CR without reading from a descriptor file
func CreateBasicWorkload(params api.CreateWorkloadParams) (*openchoreov1alpha1.Workload, error) {
	// Validate conversion parameters
	if err := validateConversionParams(params); err != nil {
		return nil, fmt.Errorf("invalid conversion parameters: %w", err)
	}

	// Generate workload name from component name
	workloadName := params.ComponentName + "-workload"

	// Create the basic workload using shared function
	workload := createBaseWorkload(workloadName, params)

	return workload, nil
}

// ConvertWorkloadCRToYAML converts a Workload CR to clean YAML bytes with proper field ordering
func ConvertWorkloadCRToYAML(workload *openchoreov1alpha1.Workload) ([]byte, error) {
	// Create a custom structure to control field ordering
	type orderedWorkload struct {
		APIVersion string `yaml:"apiVersion"`
		Kind       string `yaml:"kind"`
		Metadata   struct {
			Name string `yaml:"name"`
		} `yaml:"metadata"`
		Spec struct {
			Owner       openchoreov1alpha1.WorkloadOwner                 `yaml:"owner"`
			Containers  map[string]openchoreov1alpha1.Container          `yaml:"containers,omitempty"`
			Endpoints   map[string]openchoreov1alpha1.WorkloadEndpoint   `yaml:"endpoints,omitempty"`
			Connections map[string]openchoreov1alpha1.WorkloadConnection `yaml:"connections,omitempty"`
		} `yaml:"spec"`
		Status openchoreov1alpha1.WorkloadStatus `yaml:"status,omitempty"`
	}

	// Create the ordered structure
	ordered := orderedWorkload{
		APIVersion: workload.APIVersion,
		Kind:       workload.Kind,
		Status:     workload.Status,
	}
	ordered.Metadata.Name = workload.Name
	ordered.Spec.Owner = workload.Spec.Owner
	ordered.Spec.Containers = workload.Spec.Containers
	ordered.Spec.Endpoints = workload.Spec.Endpoints
	ordered.Spec.Connections = workload.Spec.Connections

	// Marshal with gopkg.in/yaml.v3 for better control
	return yaml.Marshal(ordered)
}
