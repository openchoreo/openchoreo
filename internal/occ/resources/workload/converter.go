// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package synth

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/yaml"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/pkg/cli/types/api"
)

// WorkloadDescriptor represents the structure of a workload.yaml file
// This is the developer-maintained descriptor alongside source code
type WorkloadDescriptor struct {
	APIVersion     string                          `yaml:"apiVersion"`
	Metadata       WorkloadDescriptorMetadata      `yaml:"metadata"`
	Endpoints      []WorkloadDescriptorEndpoint    `yaml:"endpoints,omitempty"`
	Connections    []WorkloadDescriptorConnection  `yaml:"connections,omitempty"`
	Configurations WorkloadDescriptorConfiguration `yaml:"configurations,omitempty"`
}

type WorkloadDescriptorMetadata struct {
	Name string `yaml:"name"`
}

type WorkloadDescriptorEndpoint struct {
	Name        string   `yaml:"name"`
	DisplayName string   `yaml:"displayName,omitempty"`
	Port        int32    `yaml:"port"`
	TargetPort  int32    `yaml:"targetPort,omitempty"`
	Type        string   `yaml:"type"`
	BasePath    string   `yaml:"basePath,omitempty"`
	SchemaFile  string   `yaml:"schemaFile,omitempty"`
	Context     string   `yaml:"context,omitempty"`
	Visibility  []string `yaml:"visibility,omitempty"`
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

// WorkloadDescriptorConfiguration represents the configurations section in workload.yaml
type WorkloadDescriptorConfiguration struct {
	Env   []WorkloadDescriptorEnvVar  `yaml:"env,omitempty"`
	Files []WorkloadDescriptorFileVar `yaml:"files,omitempty"`
}

// WorkloadDescriptorEnvVar represents an environment variable in the descriptor
type WorkloadDescriptorEnvVar struct {
	Name      string                          `yaml:"name"`
	Value     string                          `yaml:"value,omitempty"`
	ValueFrom *WorkloadDescriptorEnvVarSource `yaml:"valueFrom,omitempty"`
}

// WorkloadDescriptorEnvVarSource represents the source for an environment variable value
type WorkloadDescriptorEnvVarSource struct {
	SecretKeyRef *WorkloadDescriptorSecretKeyRef `yaml:"secretKeyRef,omitempty"`
	Path         string                          `yaml:"path,omitempty"`
}

// WorkloadDescriptorSecretKeyRef represents a reference to a secret key
type WorkloadDescriptorSecretKeyRef struct {
	Name string `yaml:"name"`
	Key  string `yaml:"key"`
}

// WorkloadDescriptorFileVar represents a file configuration in the descriptor
type WorkloadDescriptorFileVar struct {
	Name      string                          `yaml:"name"`
	MountPath string                          `yaml:"mountPath"`
	Value     string                          `yaml:"value,omitempty"`
	ValueFrom *WorkloadDescriptorEnvVarSource `yaml:"valueFrom,omitempty"`
}

// ConversionParams holds the parameters needed for workload conversion
type ConversionParams struct {
	NamespaceName string
	ProjectName   string
	ComponentName string
	ImageURL      string
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
	data, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("failed to read data: %w", err)
	}
	if err := yaml.Unmarshal(data, &descriptor); err != nil {
		return nil, fmt.Errorf("failed to decode YAML: %w", err)
	}

	return &descriptor, nil
}

func validateConversionParams(params api.CreateWorkloadParams) error {
	if params.NamespaceName == "" {
		return fmt.Errorf("namespace name is required")
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
			Name:      workloadName,
			Namespace: params.NamespaceName,
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
	workloadName := params.ComponentName + "-workload"
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
	addConnectionsFromDescriptor(workload, descriptor)

	// Add configurations from descriptor if present
	if err := addConfigurationsFromDescriptor(workload, descriptor, descriptorPath); err != nil {
		return nil, fmt.Errorf("failed to add configurations: %w", err)
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
		var visibility []openchoreov1alpha1.EndpointVisibility
		for _, v := range descriptorEndpoint.Visibility {
			visibility = append(visibility, openchoreov1alpha1.EndpointVisibility(v))
		}

		endpoint := openchoreov1alpha1.WorkloadEndpoint{
			DisplayName: descriptorEndpoint.DisplayName,
			Port:        descriptorEndpoint.Port,
			TargetPort:  descriptorEndpoint.TargetPort,
			Type:        openchoreov1alpha1.EndpointType(descriptorEndpoint.Type),
			BasePath:    descriptorEndpoint.BasePath,
			Visibility:  visibility,
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
func addConnectionsFromDescriptor(workload *openchoreov1alpha1.Workload, descriptor *WorkloadDescriptor) {
	if len(descriptor.Connections) == 0 {
		return
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
}

// addConfigurationsFromDescriptor adds configurations (env vars and files) from the descriptor to the workload
func addConfigurationsFromDescriptor(workload *openchoreov1alpha1.Workload, descriptor *WorkloadDescriptor, descriptorPath string) error {
	// Get the main container
	mainContainer, exists := workload.Spec.Containers["main"]
	if !exists {
		return fmt.Errorf("main container not found in workload")
	}

	// Add environment variables
	if len(descriptor.Configurations.Env) > 0 {
		mainContainer.Env = make([]openchoreov1alpha1.EnvVar, len(descriptor.Configurations.Env))
		for i, envVar := range descriptor.Configurations.Env {
			crEnvVar := openchoreov1alpha1.EnvVar{
				Key: envVar.Name,
			}

			if envVar.ValueFrom != nil && envVar.ValueFrom.SecretKeyRef != nil {
				crEnvVar.ValueFrom = convertEnvVarSource(envVar.ValueFrom)
			} else if envVar.Value != "" {
				crEnvVar.Value = envVar.Value
			}

			mainContainer.Env[i] = crEnvVar
		}
	}

	// Add file configurations
	if len(descriptor.Configurations.Files) > 0 {
		mainContainer.Files = make([]openchoreov1alpha1.FileVar, 0, len(descriptor.Configurations.Files))
		baseDir := filepath.Dir(descriptorPath)

		for _, fileVar := range descriptor.Configurations.Files {
			crFileVar := openchoreov1alpha1.FileVar{
				Key:       fileVar.Name,
				MountPath: fileVar.MountPath,
			}

			// Only set value OR valueFrom, never both
			// Priority: SecretKeyRef > Path > inline Value
			if fileVar.ValueFrom != nil && fileVar.ValueFrom.SecretKeyRef != nil {
				// Reference to secret
				crFileVar.ValueFrom = convertEnvVarSource(fileVar.ValueFrom)
			} else if fileVar.ValueFrom != nil && fileVar.ValueFrom.Path != "" {
				// Read file content from path
				filePath := filepath.Join(baseDir, fileVar.ValueFrom.Path)
				content, err := os.ReadFile(filePath)
				if err != nil {
					return fmt.Errorf("failed to read file %s: %w", filePath, err)
				}
				crFileVar.Value = string(content)
			} else if fileVar.Value != "" {
				// Inline value
				crFileVar.Value = fileVar.Value
			}

			mainContainer.Files = append(mainContainer.Files, crFileVar)
		}
	}

	// Update the container in the workload
	workload.Spec.Containers["main"] = mainContainer

	return nil
}

// convertEnvVarSource converts a descriptor env var source to a CR env var source
func convertEnvVarSource(source *WorkloadDescriptorEnvVarSource) *openchoreov1alpha1.EnvVarValueFrom {
	if source == nil {
		return nil
	}

	result := &openchoreov1alpha1.EnvVarValueFrom{}

	if source.SecretKeyRef != nil {
		result.SecretRef = &openchoreov1alpha1.SecretKeyRef{
			Name: source.SecretKeyRef.Name,
			Key:  source.SecretKeyRef.Key,
		}
	}

	return result
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
	// Note: sigs.k8s.io/yaml uses JSON tags, but we keep both for compatibility
	type orderedWorkload struct {
		APIVersion string `json:"apiVersion" yaml:"apiVersion"`
		Kind       string `json:"kind" yaml:"kind"`
		Metadata   struct {
			Name      string `json:"name" yaml:"name"`
			Namespace string `json:"namespace,omitempty" yaml:"namespace,omitempty"`
		} `json:"metadata" yaml:"metadata"`
		Spec struct {
			Owner       openchoreov1alpha1.WorkloadOwner                 `json:"owner" yaml:"owner"`
			Containers  map[string]openchoreov1alpha1.Container          `json:"containers,omitempty" yaml:"containers,omitempty"`
			Endpoints   map[string]openchoreov1alpha1.WorkloadEndpoint   `json:"endpoints,omitempty" yaml:"endpoints,omitempty"`
			Connections map[string]openchoreov1alpha1.WorkloadConnection `json:"connections,omitempty" yaml:"connections,omitempty"`
		} `json:"spec" yaml:"spec"`
	}

	// Create the ordered structure
	ordered := orderedWorkload{
		APIVersion: workload.APIVersion,
		Kind:       workload.Kind,
	}
	ordered.Metadata.Name = workload.Name
	ordered.Metadata.Namespace = workload.Namespace
	ordered.Spec.Owner = workload.Spec.Owner
	ordered.Spec.Containers = workload.Spec.Containers
	ordered.Spec.Endpoints = workload.Spec.Endpoints
	ordered.Spec.Connections = workload.Spec.Connections

	// Marshal with sigs.k8s.io/yaml for JSON tag support
	return yaml.Marshal(ordered)
}
