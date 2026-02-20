// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package models

import (
	"errors"
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation"
)

// BuildPlaneRef represents a reference to a BuildPlane or ClusterBuildPlane
type BuildPlaneRef struct {
	Kind string `json:"kind"`
	Name string `json:"name"`
}

// CreateProjectRequest represents the request to create a new project
type CreateProjectRequest struct {
	Name               string         `json:"name"`
	DisplayName        string         `json:"displayName,omitempty"`
	Description        string         `json:"description,omitempty"`
	DeploymentPipeline string         `json:"deploymentPipeline,omitempty"`
	BuildPlaneRef      *BuildPlaneRef `json:"buildPlaneRef,omitempty"`
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
	URL       string                              `json:"url"`
	SecretRef string                              `json:"secretRef,omitempty"`
	Revision  ComponentWorkflowRepositoryRevision `json:"revision"`
	AppPath   string                              `json:"appPath"`
}

// ComponentWorkflowRepositoryRevision represents repository revision information
type ComponentWorkflowRepositoryRevision struct {
	Branch string `json:"branch"`
	Commit string `json:"commit,omitempty"`
}

// ComponentTrait represents a trait instance attached to a component in API requests
type ComponentTrait struct {
	Kind         string                `json:"kind,omitempty"` // Trait or ClusterTrait
	Name         string                `json:"name"`
	InstanceName string                `json:"instanceName"`
	Parameters   *runtime.RawExtension `json:"parameters,omitempty"`
}

// ComponentTypeRef represents a reference to a ComponentType or ClusterComponentType in API requests
type ComponentTypeRef struct {
	Kind string `json:"kind,omitempty"` // ComponentType or ClusterComponentType
	Name string `json:"name"`           // Format: {workloadType}/{componentTypeName}
}

// CreateComponentRequest represents the request to create a new component
type CreateComponentRequest struct {
	Name              string                `json:"name"`
	DisplayName       string                `json:"displayName,omitempty"`
	Description       string                `json:"description,omitempty"`
	ComponentType     *ComponentTypeRef     `json:"componentType,omitempty"`
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
	// Parameters are component type parameters (port, replicas, exposed, etc.)
	// +optional
	Parameters *runtime.RawExtension `json:"parameters,omitempty"`
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

// DataPlaneRef represents a reference to a DataPlane or ClusterDataPlane
type DataPlaneRef struct {
	Kind string `json:"kind"`
	Name string `json:"name"`
}

// CreateEnvironmentRequest represents the request to create a new environment
type CreateEnvironmentRequest struct {
	Name         string        `json:"name"`
	DisplayName  string        `json:"displayName,omitempty"`
	Description  string        `json:"description,omitempty"`
	DataPlaneRef *DataPlaneRef `json:"dataPlaneRef,omitempty"`
	IsProduction bool          `json:"isProduction"`
	DNSPrefix    string        `json:"dnsPrefix,omitempty"`
}

// CreateDataPlaneRequest represents the request to create a new dataplane
type CreateDataPlaneRequest struct {
	Name                    string                 `json:"name"`
	DisplayName             string                 `json:"displayName,omitempty"`
	Description             string                 `json:"description,omitempty"`
	ClusterAgentClientCA    string                 `json:"clusterAgentClientCA"`
	PublicVirtualHost       string                 `json:"publicVirtualHost"`
	OrganizationVirtualHost string                 `json:"organizationVirtualHost"`
	PublicHTTPPort          *int32                 `json:"publicHTTPPort,omitempty"`
	PublicHTTPSPort         *int32                 `json:"publicHTTPSPort,omitempty"`
	OrganizationHTTPPort    *int32                 `json:"organizationHTTPPort,omitempty"`
	OrganizationHTTPSPort   *int32                 `json:"organizationHTTPSPort,omitempty"`
	ObservabilityPlaneRef   *ObservabilityPlaneRef `json:"observabilityPlaneRef,omitempty"`
}

// Validate validates the CreateProjectRequest
func (req *CreateProjectRequest) Validate() error {
	// Validate BuildPlaneRef.Kind if provided
	if req.BuildPlaneRef != nil {
		kind := req.BuildPlaneRef.Kind
		if kind != "BuildPlane" && kind != "ClusterBuildPlane" {
			return fmt.Errorf("buildPlaneRef.kind must be 'BuildPlane' or 'ClusterBuildPlane', got '%s'", kind)
		}
		if strings.TrimSpace(req.BuildPlaneRef.Name) == "" {
			return errors.New("buildPlaneRef.name is required when buildPlaneRef is provided")
		}
	}
	return nil
}

// Validate validates the CreateComponentRequest
func (req *CreateComponentRequest) Validate() error {
	if req.ComponentType == nil {
		return errors.New("componentType is required")
	}
	name := strings.TrimSpace(req.ComponentType.Name)
	if name == "" {
		return errors.New("componentType.name is required")
	}
	kind := strings.TrimSpace(req.ComponentType.Kind)
	if kind != "" && kind != "ComponentType" && kind != "ClusterComponentType" {
		return fmt.Errorf("componentType.kind must be 'ComponentType' or 'ClusterComponentType', got '%s'", kind)
	}
	return nil
}

// Validate validates the CreateEnvironmentRequest
func (req *CreateEnvironmentRequest) Validate() error {
	// TODO: Implement custom validation using Go stdlib
	return nil
}

// Validate validates the CreateDataPlaneRequest
func (req *CreateDataPlaneRequest) Validate() error {
	// Validate ObservabilityPlaneRef if provided
	if req.ObservabilityPlaneRef != nil {
		kind := req.ObservabilityPlaneRef.Kind
		if kind == "" {
			return errors.New("observabilityPlaneRef.kind is required when observabilityPlaneRef is provided")
		}
		if kind != "ObservabilityPlane" && kind != "ClusterObservabilityPlane" {
			return fmt.Errorf("observabilityPlaneRef.kind must be 'ObservabilityPlane' or 'ClusterObservabilityPlane', got '%s'", kind)
		}
		name := strings.TrimSpace(req.ObservabilityPlaneRef.Name)
		if name == "" {
			return errors.New("observabilityPlaneRef.name is required when observabilityPlaneRef is provided")
		}
		if errs := validation.IsDNS1123Label(name); len(errs) > 0 {
			return fmt.Errorf("observabilityPlaneRef.name must be a valid DNS-1123 label: %s", strings.Join(errs, ", "))
		}
	}
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
	if req.BuildPlaneRef != nil {
		req.BuildPlaneRef.Kind = strings.TrimSpace(req.BuildPlaneRef.Kind)
		req.BuildPlaneRef.Name = strings.TrimSpace(req.BuildPlaneRef.Name)
	}
}

// Sanitize sanitizes the CreateComponentRequest by trimming whitespace
func (req *CreateComponentRequest) Sanitize() {
	req.Name = strings.TrimSpace(req.Name)
	req.DisplayName = strings.TrimSpace(req.DisplayName)
	req.Description = strings.TrimSpace(req.Description)
	if req.ComponentType != nil {
		req.ComponentType.Kind = strings.TrimSpace(req.ComponentType.Kind)
		req.ComponentType.Name = strings.TrimSpace(req.ComponentType.Name)
	}

	for i := range req.Traits {
		req.Traits[i].Kind = strings.TrimSpace(req.Traits[i].Kind)
		req.Traits[i].Name = strings.TrimSpace(req.Traits[i].Name)
		req.Traits[i].InstanceName = strings.TrimSpace(req.Traits[i].InstanceName)
	}
}

// Sanitize sanitizes the CreateEnvironmentRequest by trimming whitespace
func (req *CreateEnvironmentRequest) Sanitize() {
	req.Name = strings.TrimSpace(req.Name)
	req.DisplayName = strings.TrimSpace(req.DisplayName)
	req.Description = strings.TrimSpace(req.Description)
	if req.DataPlaneRef != nil {
		req.DataPlaneRef.Kind = strings.TrimSpace(req.DataPlaneRef.Kind)
		req.DataPlaneRef.Name = strings.TrimSpace(req.DataPlaneRef.Name)
	}
	req.DNSPrefix = strings.TrimSpace(req.DNSPrefix)
}

// Sanitize sanitizes the CreateDataPlaneRequest by trimming whitespace
func (req *CreateDataPlaneRequest) Sanitize() {
	req.Name = strings.TrimSpace(req.Name)
	req.DisplayName = strings.TrimSpace(req.DisplayName)
	req.Description = strings.TrimSpace(req.Description)
	req.ClusterAgentClientCA = strings.TrimSpace(req.ClusterAgentClientCA)
	req.PublicVirtualHost = strings.TrimSpace(req.PublicVirtualHost)
	req.OrganizationVirtualHost = strings.TrimSpace(req.OrganizationVirtualHost)
	if req.ObservabilityPlaneRef != nil {
		req.ObservabilityPlaneRef.Kind = strings.TrimSpace(req.ObservabilityPlaneRef.Kind)
		req.ObservabilityPlaneRef.Name = strings.TrimSpace(req.ObservabilityPlaneRef.Name)
	}
}

// CreateClusterDataPlaneRequest represents the request to create a new cluster-scoped dataplane
type CreateClusterDataPlaneRequest struct {
	Name                    string                 `json:"name"`
	DisplayName             string                 `json:"displayName,omitempty"`
	Description             string                 `json:"description,omitempty"`
	PlaneID                 string                 `json:"planeID"`
	ClusterAgentClientCA    string                 `json:"clusterAgentClientCA"`
	PublicVirtualHost       string                 `json:"publicVirtualHost"`
	OrganizationVirtualHost string                 `json:"organizationVirtualHost"`
	PublicHTTPPort          *int32                 `json:"publicHTTPPort,omitempty"`
	PublicHTTPSPort         *int32                 `json:"publicHTTPSPort,omitempty"`
	OrganizationHTTPPort    *int32                 `json:"organizationHTTPPort,omitempty"`
	OrganizationHTTPSPort   *int32                 `json:"organizationHTTPSPort,omitempty"`
	ObservabilityPlaneRef   *ObservabilityPlaneRef `json:"observabilityPlaneRef,omitempty"`
}

// Validate validates the CreateClusterDataPlaneRequest
func (req *CreateClusterDataPlaneRequest) Validate() error {
	if strings.TrimSpace(req.Name) == "" {
		return errors.New("name is required")
	}
	if errs := validation.IsDNS1123Label(strings.TrimSpace(req.Name)); len(errs) > 0 {
		return fmt.Errorf("name must be a valid DNS-1123 label: %s", strings.Join(errs, ", "))
	}
	if strings.TrimSpace(req.PlaneID) == "" {
		return errors.New("planeID is required")
	}
	if errs := validation.IsDNS1123Label(strings.TrimSpace(req.PlaneID)); len(errs) > 0 {
		return fmt.Errorf("planeID must be a valid DNS-1123 label: %s", strings.Join(errs, ", "))
	}
	if strings.TrimSpace(req.ClusterAgentClientCA) == "" {
		return errors.New("clusterAgentClientCA is required")
	}
	if strings.TrimSpace(req.PublicVirtualHost) == "" {
		return errors.New("publicVirtualHost is required")
	}
	if strings.TrimSpace(req.OrganizationVirtualHost) == "" {
		return errors.New("organizationVirtualHost is required")
	}
	// Validate ObservabilityPlaneRef if provided
	if req.ObservabilityPlaneRef != nil {
		kind := req.ObservabilityPlaneRef.Kind
		if kind == "" {
			return errors.New("observabilityPlaneRef.kind is required when observabilityPlaneRef is provided")
		}
		if kind != "ClusterObservabilityPlane" {
			return fmt.Errorf("observabilityPlaneRef.kind must be 'ClusterObservabilityPlane' for cluster-scoped resources, got '%s'", kind)
		}
		name := strings.TrimSpace(req.ObservabilityPlaneRef.Name)
		if name == "" {
			return errors.New("observabilityPlaneRef.name is required when observabilityPlaneRef is provided")
		}
		if errs := validation.IsDNS1123Label(name); len(errs) > 0 {
			return fmt.Errorf("observabilityPlaneRef.name must be a valid DNS-1123 label: %s", strings.Join(errs, ", "))
		}
	}
	return nil
}

// Sanitize sanitizes the CreateClusterDataPlaneRequest by trimming whitespace
func (req *CreateClusterDataPlaneRequest) Sanitize() {
	req.Name = strings.TrimSpace(req.Name)
	req.DisplayName = strings.TrimSpace(req.DisplayName)
	req.Description = strings.TrimSpace(req.Description)
	req.PlaneID = strings.TrimSpace(req.PlaneID)
	req.ClusterAgentClientCA = strings.TrimSpace(req.ClusterAgentClientCA)
	req.PublicVirtualHost = strings.TrimSpace(req.PublicVirtualHost)
	req.OrganizationVirtualHost = strings.TrimSpace(req.OrganizationVirtualHost)
	if req.ObservabilityPlaneRef != nil {
		req.ObservabilityPlaneRef.Kind = strings.TrimSpace(req.ObservabilityPlaneRef.Kind)
		req.ObservabilityPlaneRef.Name = strings.TrimSpace(req.ObservabilityPlaneRef.Name)
	}
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
	// Container defines the container-specific overrides for env and file configurations
	// +optional
	Container *ContainerOverride `json:"container,omitempty"`
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

// UpdateComponentWorkflowRequest represents the request to update or initialize a component's workflow configuration
type UpdateComponentWorkflowRequest struct {
	WorkflowName     string                         `json:"workflowName,omitempty"`
	SystemParameters *ComponentWorkflowSystemParams `json:"systemParameters,omitempty"`
	Parameters       *runtime.RawExtension          `json:"parameters,omitempty"`
}

// ComponentTraitRequest represents a single trait instance in API requests
type ComponentTraitRequest struct {
	// Kind is the kind of trait (Trait or ClusterTrait)
	Kind string `json:"kind,omitempty"`
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
		req.Traits[i].Kind = strings.TrimSpace(req.Traits[i].Kind)
		req.Traits[i].Name = strings.TrimSpace(req.Traits[i].Name)
		req.Traits[i].InstanceName = strings.TrimSpace(req.Traits[i].InstanceName)
	}
}

// CreateGitSecretRequest represents the request to create a git secret
type CreateGitSecretRequest struct {
	SecretName string `json:"secretName"`
	// SecretType specifies the authentication type: "basic-auth" for token-based auth, "ssh-auth" for SSH key-based auth
	SecretType string `json:"secretType"`
	// Username is the username for basic authentication (optional for basic-auth type)
	Username string `json:"username,omitempty"`
	// Token is the authentication token (required for basic-auth type)
	Token string `json:"token,omitempty"`
	// SSHKey is the SSH private key (required for ssh-auth type)
	SSHKey string `json:"sshKey,omitempty"`
	// SSHKEYID is the SSH Key ID for AWS CodeCommit (optional for ssh-auth type)
	SSHKEYID string `json:"sshKeyId,omitempty"`
}

// Validate validates the CreateGitSecretRequest
func (req *CreateGitSecretRequest) Validate() error {
	if strings.TrimSpace(req.SecretName) == "" {
		return errors.New("secretName is required")
	}
	if len(req.SecretName) > 253 {
		return errors.New("secretName must be at most 253 characters")
	}

	// Validate secretType
	secretType := strings.TrimSpace(req.SecretType)
	//nolint:gosec // False positive: these are type checks, not hardcoded credentials
	if secretType != "basic-auth" && secretType != "ssh-auth" {
		return errors.New("secretType must be 'basic-auth' or 'ssh-auth'")
	}

	// Validate credentials match type
	if secretType == "basic-auth" {
		if strings.TrimSpace(req.Token) == "" {
			return errors.New("token is required for basic-auth type")
		}
		if strings.TrimSpace(req.SSHKey) != "" {
			return errors.New("sshKey must not be provided for basic-auth type")
		}
		if strings.TrimSpace(req.SSHKEYID) != "" {
			return errors.New("sshKeyId must not be provided for basic-auth type")
		}
		// Username is optional for basic-auth
	} else { // ssh-auth
		if strings.TrimSpace(req.SSHKey) == "" {
			return errors.New("sshKey is required for ssh-auth type")
		}
		if strings.TrimSpace(req.Token) != "" {
			return errors.New("token must not be provided for ssh-auth type")
		}
		if strings.TrimSpace(req.Username) != "" {
			return errors.New("username must not be provided for ssh-auth type")
		}
		// SSHKeyId is optional for ssh-auth (required for AWS CodeCommit)
	}

	return nil
}

// Sanitize sanitizes the CreateGitSecretRequest by trimming whitespace
func (req *CreateGitSecretRequest) Sanitize() {
	req.SecretName = strings.TrimSpace(req.SecretName)
	req.SecretType = strings.TrimSpace(req.SecretType)
	req.Username = strings.TrimSpace(req.Username)
	req.Token = strings.TrimSpace(req.Token)
	req.SSHKey = strings.TrimSpace(req.SSHKey)
	req.SSHKEYID = strings.TrimSpace(req.SSHKEYID)
}

// CreateWorkflowRunRequest represents the request to create a new workflow run
type CreateWorkflowRunRequest struct {
	WorkflowName string                 `json:"workflowName"`
	Parameters   map[string]interface{} `json:"parameters,omitempty"`
}

// Validate validates the CreateWorkflowRunRequest
func (req *CreateWorkflowRunRequest) Validate() error {
	if strings.TrimSpace(req.WorkflowName) == "" {
		return errors.New("workflowName is required")
	}
	return nil
}

// Sanitize sanitizes the CreateWorkflowRunRequest by trimming whitespace
func (req *CreateWorkflowRunRequest) Sanitize() {
	req.WorkflowName = strings.TrimSpace(req.WorkflowName)
}
