// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package models

import (
	"errors"
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/runtime"

	"github.com/openchoreo/openchoreo/pkg/constants"
)

// ListOptions represents parameters for list operations
type ListOptions struct {
	Limit    int    // Items per page (optional, default: 100, max: 500)
	Continue string // Opaque K8s continue token (optional)
}

// DefaultPageLimit is the default number of items per page for list operations
const DefaultPageLimit = constants.DefaultPageLimit

// MaxPageLimit is the maximum number of items allowed per page
const MaxPageLimit = constants.MaxPageLimit

// MinPageLimit is the minimum number of items per page
const MinPageLimit = constants.MinPageLimit

// CreateProjectRequest represents the request to create a new project
type CreateProjectRequest struct {
	Name               string `json:"name"`
	DisplayName        string `json:"displayName,omitempty"`
	Description        string `json:"description,omitempty"`
	DeploymentPipeline string `json:"deploymentPipeline,omitempty"`
}

// BuildConfig represents the build configuration for a component

type TemplateParameter struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type Workflow struct {
	Name   string                `json:"name"`
	Schema *runtime.RawExtension `json:"schema,omitempty"`
}

// ComponentWorkflow represents the component workflow configuration in API requests/responses
type ComponentWorkflow struct {
	Name             string                         `json:"name"`
	SystemParameters *ComponentWorkflowSystemParams `json:"systemParameters"`
	Parameters       *runtime.RawExtension          `json:"parameters,omitempty"`
}

// ComponentWorkflowSystemParams represents the system parameters for component component-component-workflows
type ComponentWorkflowSystemParams struct {
	Repository ComponentWorkflowRepository `json:"repository"`
}

// ComponentWorkflowRepository represents repository information
type ComponentWorkflowRepository struct {
	URL      string                              `json:"url"`
	Revision ComponentWorkflowRepositoryRevision `json:"revision"`
	AppPath  string                              `json:"appPath"`
}

// ComponentWorkflowRepositoryRevision represents repository revision information
type ComponentWorkflowRepositoryRevision struct {
	Branch string `json:"branch"`
	Commit string `json:"commit,omitempty"`
}

// ComponentTrait represents a trait instance attached to a component in API requests
type ComponentTrait struct {
	Name         string                `json:"name"`
	InstanceName string                `json:"instanceName"`
	Parameters   *runtime.RawExtension `json:"parameters,omitempty"`
}

// CreateComponentRequest represents the request to create a new component
type CreateComponentRequest struct {
	Name              string                `json:"name"`
	DisplayName       string                `json:"displayName,omitempty"`
	Description       string                `json:"description,omitempty"`
	Type              string                `json:"type,omitempty"`          // LEGACY: Use componentType instead
	ComponentType     string                `json:"componentType,omitempty"` // Format: {workloadType}/{componentTypeName}
	AutoDeploy        *bool                 `json:"autoDeploy,omitempty"`
	Parameters        *runtime.RawExtension `json:"parameters,omitempty"`
	Traits            []ComponentTrait      `json:"traits,omitempty"`
	ComponentWorkflow *ComponentWorkflow    `json:"workflow,omitempty"`
}

// PromoteComponentRequest Promote from one environment to another
type PromoteComponentRequest struct {
	SourceEnvironment string `json:"sourceEnv"`
	TargetEnvironment string `json:"targetEnv"`
	// TODO Support overrides for the target environment
}

// PatchComponentRequest represents the request to patch a Component
type PatchComponentRequest struct {
	// AutoDeploy controls whether the component should automatically deploy to the default environment
	// +optional
	AutoDeploy *bool `json:"autoDeploy,omitempty"`
	// TODO Add support for other fields to be patched
}

type CreateComponentReleaseRequest struct {
	ReleaseName string `json:"releaseName,omitempty"`
}

// Sanitize sanitizes the CreateComponentReleaseRequest by trimming whitespace
func (req *CreateComponentReleaseRequest) Sanitize() {
	req.ReleaseName = strings.TrimSpace(req.ReleaseName)
}

// DeployReleaseRequest represents the request to deploy a release to the lowest environment
type DeployReleaseRequest struct {
	ReleaseName string `json:"releaseName"`
}

// Sanitize sanitizes the DeployReleaseRequest by trimming whitespace
func (req *DeployReleaseRequest) Sanitize() {
	req.ReleaseName = strings.TrimSpace(req.ReleaseName)
}

// Validate validates the DeployReleaseRequest
func (req *DeployReleaseRequest) Validate() error {
	if req.ReleaseName == "" {
		return errors.New("releaseName is required")
	}
	return nil
}

// CreateEnvironmentRequest represents the request to create a new environment
type CreateEnvironmentRequest struct {
	Name         string `json:"name"`
	DisplayName  string `json:"displayName,omitempty"`
	Description  string `json:"description,omitempty"`
	DataPlaneRef string `json:"dataPlaneRef,omitempty"`
	IsProduction bool   `json:"isProduction"`
	DNSPrefix    string `json:"dnsPrefix,omitempty"`
}

// CreateDataPlaneRequest represents the request to create a new dataplane
type CreateDataPlaneRequest struct {
	Name                    string `json:"name"`
	DisplayName             string `json:"displayName,omitempty"`
	Description             string `json:"description,omitempty"`
	KubernetesClusterName   string `json:"kubernetesClusterName"`
	APIServerURL            string `json:"apiServerURL"`
	CACert                  string `json:"caCert"`
	ClientCert              string `json:"clientCert"`
	ClientKey               string `json:"clientKey"`
	PublicVirtualHost       string `json:"publicVirtualHost"`
	OrganizationVirtualHost string `json:"organizationVirtualHost"`
	ObservabilityPlaneRef   string `json:"observabilityPlaneRef,omitempty"`
}

// Validate validates the CreateProjectRequest
func (req *CreateProjectRequest) Validate() error {
	// TODO: Implement custom validation using Go stdlib
	return nil
}

// Validate validates the CreateComponentRequest
func (req *CreateComponentRequest) Validate() error {
	// TODO: Implement custom validation using Go stdlib
	return nil
}

// Validate validates the CreateEnvironmentRequest
func (req *CreateEnvironmentRequest) Validate() error {
	// TODO: Implement custom validation using Go stdlib
	return nil
}

// Validate validates the CreateDataPlaneRequest
func (req *CreateDataPlaneRequest) Validate() error {
	// TODO: Implement custom validation using Go stdlib
	return nil
}

// Validate validates the PromoteComponentRequest
func (req *PromoteComponentRequest) Validate() error {
	// TODO: Implement custom validation using Go stdlib
	return nil
}

// Sanitize sanitizes the CreateProjectRequest by trimming whitespace
func (req *CreateProjectRequest) Sanitize() {
	req.Name = strings.TrimSpace(req.Name)
	req.DisplayName = strings.TrimSpace(req.DisplayName)
	req.Description = strings.TrimSpace(req.Description)
	req.DeploymentPipeline = strings.TrimSpace(req.DeploymentPipeline)
}

// Sanitize sanitizes the CreateComponentRequest by trimming whitespace
func (req *CreateComponentRequest) Sanitize() {
	req.Name = strings.TrimSpace(req.Name)
	req.DisplayName = strings.TrimSpace(req.DisplayName)
	req.Description = strings.TrimSpace(req.Description)
	req.Type = strings.TrimSpace(req.Type)
	req.ComponentType = strings.TrimSpace(req.ComponentType)

	for i := range req.Traits {
		req.Traits[i].Name = strings.TrimSpace(req.Traits[i].Name)
		req.Traits[i].InstanceName = strings.TrimSpace(req.Traits[i].InstanceName)
	}
}

// Sanitize sanitizes the CreateEnvironmentRequest by trimming whitespace
func (req *CreateEnvironmentRequest) Sanitize() {
	req.Name = strings.TrimSpace(req.Name)
	req.DisplayName = strings.TrimSpace(req.DisplayName)
	req.Description = strings.TrimSpace(req.Description)
	req.DataPlaneRef = strings.TrimSpace(req.DataPlaneRef)
	req.DNSPrefix = strings.TrimSpace(req.DNSPrefix)
}

// Sanitize sanitizes the CreateDataPlaneRequest by trimming whitespace
func (req *CreateDataPlaneRequest) Sanitize() {
	req.Name = strings.TrimSpace(req.Name)
	req.DisplayName = strings.TrimSpace(req.DisplayName)
	req.Description = strings.TrimSpace(req.Description)
	req.KubernetesClusterName = strings.TrimSpace(req.KubernetesClusterName)
	req.APIServerURL = strings.TrimSpace(req.APIServerURL)
	req.CACert = strings.TrimSpace(req.CACert)
	req.ClientCert = strings.TrimSpace(req.ClientCert)
	req.ClientKey = strings.TrimSpace(req.ClientKey)
	req.PublicVirtualHost = strings.TrimSpace(req.PublicVirtualHost)
	req.OrganizationVirtualHost = strings.TrimSpace(req.OrganizationVirtualHost)
	req.ObservabilityPlaneRef = strings.TrimSpace(req.ObservabilityPlaneRef)
}

// Sanitize sanitizes the PromoteComponentRequest by trimming whitespace
func (req *PromoteComponentRequest) Sanitize() {
	req.SourceEnvironment = strings.TrimSpace(req.SourceEnvironment)
	req.TargetEnvironment = strings.TrimSpace(req.TargetEnvironment)
}

type BindingReleaseState string

const (
	ReleaseStateActive   BindingReleaseState = "Active"
	ReleaseStateSuspend  BindingReleaseState = "Suspend"
	ReleaseStateUndeploy BindingReleaseState = "Undeploy"
)

// UpdateBindingRequest represents the request to update a component binding
// Only includes fields that can be updated via PATCH
type UpdateBindingRequest struct {
	// ReleaseState controls the state of the Release created by this binding.
	// Valid values: Active, Suspend, Undeploy
	ReleaseState BindingReleaseState `json:"releaseState"`
}

// Validate validates the UpdateBindingRequest
func (req *UpdateBindingRequest) Validate() error {
	// Validate releaseState values
	switch req.ReleaseState {
	case "Active", "Suspend", "Undeploy":
		// Valid values
	case "":
		// Empty is not allowed for PATCH
		return errors.New("releaseState is required")
	default:
		return errors.New("releaseState must be one of: Active, Suspend, Undeploy")
	}
	return nil
}

// PatchReleaseBindingRequest represents the request to patch a ReleaseBinding
type PatchReleaseBindingRequest struct {
	// ReleaseName is the name of the release to bind (required when creating a new binding)
	// +optional
	ReleaseName string `json:"releaseName,omitempty"`

	// Environment is the target environment (required when creating a new binding)
	// +optional
	Environment string `json:"environment,omitempty"`

	// ComponentTypeEnvOverrides for ComponentType envOverrides parameters
	// These values override the defaults defined in the Component for this specific environment
	// +optional
	ComponentTypeEnvOverrides map[string]interface{} `json:"componentTypeEnvOverrides,omitempty"`

	// TraitOverrides provides environment-specific overrides for trait configurations
	// Keyed by instanceName (which must be unique across all traits in the component)
	// Structure: map[instanceName]overrideValues
	// +optional
	TraitOverrides map[string]map[string]interface{} `json:"traitOverrides,omitempty"`

	// WorkloadOverrides provides environment-specific overrides for the entire workload spec
	// These values override the workload specification for this specific environment
	// +optional
	WorkloadOverrides *WorkloadOverrides `json:"workloadOverrides,omitempty"`
}

// WorkloadOverrides represents environment-specific workload overrides
type WorkloadOverrides struct {
	// Containers define the container-specific overrides
	// The key is the container name, and the value contains env and file overrides for that container
	// +optional
	Containers map[string]ContainerOverride `json:"containers,omitempty"`
}

// ContainerOverride represents overrides for a specific container
type ContainerOverride struct {
	// Environment variable overrides
	// +optional
	Env []EnvVar `json:"env,omitempty"`

	// File configuration overrides
	// +optional
	Files []FileVar `json:"files,omitempty"`
}

// EnvVar represents an environment variable
type EnvVar struct {
	Key   string `json:"key"`
	Value string `json:"value,omitempty"`
	// Extract the environment variable value from another resource.
	// Mutually exclusive with value.
	// +optional
	ValueFrom *EnvVarValueFrom `json:"valueFrom,omitempty"`
}

// FileVar represents a file configuration
type FileVar struct {
	Key       string `json:"key"`
	MountPath string `json:"mountPath"`
	Value     string `json:"value,omitempty"`
	// Extract the file value from another resource.
	// Mutually exclusive with value.
	// +optional
	ValueFrom *EnvVarValueFrom `json:"valueFrom,omitempty"`
}

// EnvVarValueFrom holds references to external sources for environment variables and files
type EnvVarValueFrom struct {
	// Reference to a secret resource.
	// +optional
	SecretRef *SecretKeyRef `json:"secretRef,omitempty"`
}

// SecretKeyRef references a specific key in a secret
type SecretKeyRef struct {
	Name string `json:"name"`
	Key  string `json:"key"`
}

// UpdateComponentWorkflowSchemaRequest represents the request to update a component's workflow schema
type UpdateComponentWorkflowSchemaRequest struct {
	SystemParameters *ComponentWorkflowSystemParams `json:"systemParameters,omitempty"`
	Parameters       *runtime.RawExtension          `json:"parameters,omitempty"`
}

// ComponentTraitRequest represents a single trait instance in API requests
type ComponentTraitRequest struct {
	// Name is the name of the Trait resource to use
	Name string `json:"name"`
	// InstanceName uniquely identifies this trait instance within the component
	InstanceName string `json:"instanceName"`
	// Parameters contains the trait parameter values
	Parameters map[string]interface{} `json:"parameters,omitempty"`
}

// UpdateComponentTraitsRequest represents the request to update all traits on a component
type UpdateComponentTraitsRequest struct {
	Traits []ComponentTraitRequest `json:"traits"`
}

// Validate validates the UpdateComponentTraitsRequest
func (req *UpdateComponentTraitsRequest) Validate() error {
	instanceNames := make(map[string]bool)
	for i, trait := range req.Traits {
		if strings.TrimSpace(trait.Name) == "" {
			return errors.New("trait name is required at index " + fmt.Sprintf("%d", i))
		}
		if strings.TrimSpace(trait.InstanceName) == "" {
			return errors.New("trait instanceName is required at index " + fmt.Sprintf("%d", i))
		}
		if instanceNames[trait.InstanceName] {
			return errors.New("duplicate trait instanceName: " + trait.InstanceName)
		}
		instanceNames[trait.InstanceName] = true
	}
	return nil
}

// Sanitize sanitizes the UpdateComponentTraitsRequest by trimming whitespace
func (req *UpdateComponentTraitsRequest) Sanitize() {
	for i := range req.Traits {
		req.Traits[i].Name = strings.TrimSpace(req.Traits[i].Name)
		req.Traits[i].InstanceName = strings.TrimSpace(req.Traits[i].InstanceName)
	}
}
