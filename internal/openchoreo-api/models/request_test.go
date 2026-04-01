// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package models

import (
	"strings"
	"testing"
)

func TestDeployReleaseRequest_Sanitize(t *testing.T) {
	tests := []struct {
		name        string
		releaseName string
		want        string
	}{
		{
			name:        "No whitespace",
			releaseName: "myapp-20251118-1",
			want:        "myapp-20251118-1",
		},
		{
			name:        "Leading whitespace",
			releaseName: "  myapp-20251118-1",
			want:        "myapp-20251118-1",
		},
		{
			name:        "Trailing whitespace",
			releaseName: "myapp-20251118-1  ",
			want:        "myapp-20251118-1",
		},
		{
			name:        "Leading and trailing whitespace",
			releaseName: "  myapp-20251118-1  ",
			want:        "myapp-20251118-1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &DeployReleaseRequest{
				ReleaseName: tt.releaseName,
			}
			req.Sanitize()

			if req.ReleaseName != tt.want {
				t.Errorf("After Sanitize() ReleaseName = %v, want %v", req.ReleaseName, tt.want)
			}
		})
	}
}

func TestDeployReleaseRequest_Validate(t *testing.T) {
	tests := []struct {
		name        string
		releaseName string
		wantErr     bool
		errMsg      string
	}{
		{
			name:        "Valid release name",
			releaseName: "myapp-20251118-1",
			wantErr:     false,
		},
		{
			name:        "Empty release name",
			releaseName: "",
			wantErr:     true,
			errMsg:      "releaseName is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &DeployReleaseRequest{
				ReleaseName: tt.releaseName,
			}
			err := req.Validate()

			if tt.wantErr {
				if err == nil {
					t.Errorf("Validate() expected error but got none")
					return
				}
				if err.Error() != tt.errMsg {
					t.Errorf("Validate() error = %v, want %v", err.Error(), tt.errMsg)
				}
			} else {
				if err != nil {
					t.Errorf("Validate() unexpected error = %v", err)
				}
			}
		})
	}
}

func TestCreateComponentReleaseRequest_Sanitize(t *testing.T) {
	tests := []struct {
		name        string
		releaseName string
		want        string
	}{
		{
			name:        "No whitespace",
			releaseName: "release-v1",
			want:        "release-v1",
		},
		{
			name:        "With whitespace",
			releaseName: "  release-v1  ",
			want:        "release-v1",
		},
		{
			name:        "Empty string",
			releaseName: "",
			want:        "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &CreateComponentReleaseRequest{
				ReleaseName: tt.releaseName,
			}
			req.Sanitize()

			if req.ReleaseName != tt.want {
				t.Errorf("After Sanitize() ReleaseName = %v, want %v", req.ReleaseName, tt.want)
			}
		})
	}
}

func TestUpdateBindingRequest_Validate(t *testing.T) {
	tests := []struct {
		name         string
		releaseState BindingReleaseState
		wantErr      bool
		errMsg       string
	}{
		{
			name:         "Valid state - Active",
			releaseState: ReleaseStateActive,
			wantErr:      false,
		},
		{
			name:         "Valid state - Undeploy",
			releaseState: ReleaseStateUndeploy,
			wantErr:      false,
		},
		{
			name:         "Empty state",
			releaseState: "",
			wantErr:      true,
			errMsg:       "releaseState is required",
		},
		{
			name:         "Invalid state",
			releaseState: "Invalid",
			wantErr:      true,
			errMsg:       "releaseState must be one of: Active, Undeploy",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &UpdateBindingRequest{
				ReleaseState: tt.releaseState,
			}
			err := req.Validate()

			if tt.wantErr {
				if err == nil {
					t.Errorf("Validate() expected error but got none")
					return
				}
				if err.Error() != tt.errMsg {
					t.Errorf("Validate() error = %v, want %v", err.Error(), tt.errMsg)
				}
			} else {
				if err != nil {
					t.Errorf("Validate() unexpected error = %v", err)
				}
			}
		})
	}
}

func TestPromoteComponentRequest_Sanitize(t *testing.T) {
	tests := []struct {
		name      string
		sourceEnv string
		targetEnv string
		wantSrc   string
		wantTgt   string
	}{
		{
			name:      "No whitespace",
			sourceEnv: "dev",
			targetEnv: "staging",
			wantSrc:   "dev",
			wantTgt:   "staging",
		},
		{
			name:      "With whitespace",
			sourceEnv: "  dev  ",
			targetEnv: "  staging  ",
			wantSrc:   "dev",
			wantTgt:   "staging",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &PromoteComponentRequest{
				SourceEnvironment: tt.sourceEnv,
				TargetEnvironment: tt.targetEnv,
			}
			req.Sanitize()

			if req.SourceEnvironment != tt.wantSrc {
				t.Errorf("After Sanitize() SourceEnvironment = %v, want %v", req.SourceEnvironment, tt.wantSrc)
			}
			if req.TargetEnvironment != tt.wantTgt {
				t.Errorf("After Sanitize() TargetEnvironment = %v, want %v", req.TargetEnvironment, tt.wantTgt)
			}
		})
	}
}

func TestUpdateComponentTraitsRequest_Sanitize(t *testing.T) {
	tests := []struct {
		name   string
		traits []ComponentTraitRequest
		want   []ComponentTraitRequest
	}{
		{
			name: "No whitespace",
			traits: []ComponentTraitRequest{
				{Name: "logging", InstanceName: "app-logging"},
				{Name: "scaling", InstanceName: "auto-scale"},
			},
			want: []ComponentTraitRequest{
				{Name: "logging", InstanceName: "app-logging"},
				{Name: "scaling", InstanceName: "auto-scale"},
			},
		},
		{
			name: "With whitespace",
			traits: []ComponentTraitRequest{
				{Name: "  logging  ", InstanceName: "  app-logging  "},
				{Name: "scaling", InstanceName: "auto-scale"},
			},
			want: []ComponentTraitRequest{
				{Name: "logging", InstanceName: "app-logging"},
				{Name: "scaling", InstanceName: "auto-scale"},
			},
		},
		{
			name:   "Empty traits",
			traits: []ComponentTraitRequest{},
			want:   []ComponentTraitRequest{},
		},
		{
			name: "Kind field with whitespace is trimmed",
			traits: []ComponentTraitRequest{
				{Kind: "  Trait  ", Name: "logging", InstanceName: "app-logging"},
				{Kind: "  ClusterTrait  ", Name: "global-logger", InstanceName: "my-logger"},
			},
			want: []ComponentTraitRequest{
				{Kind: "Trait", Name: "logging", InstanceName: "app-logging"},
				{Kind: "ClusterTrait", Name: "global-logger", InstanceName: "my-logger"},
			},
		},
		{
			name: "Kind field without whitespace is preserved",
			traits: []ComponentTraitRequest{
				{Kind: "ClusterTrait", Name: "logging", InstanceName: "app-logging"},
			},
			want: []ComponentTraitRequest{
				{Kind: "ClusterTrait", Name: "logging", InstanceName: "app-logging"},
			},
		},
		{
			name: "Empty Kind field stays empty",
			traits: []ComponentTraitRequest{
				{Kind: "", Name: "logging", InstanceName: "app-logging"},
			},
			want: []ComponentTraitRequest{
				{Kind: "", Name: "logging", InstanceName: "app-logging"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &UpdateComponentTraitsRequest{
				Traits: tt.traits,
			}
			req.Sanitize()

			if len(req.Traits) != len(tt.want) {
				t.Errorf("After Sanitize() len(Traits) = %v, want %v", len(req.Traits), len(tt.want))
				return
			}

			for i, trait := range req.Traits {
				if trait.Kind != tt.want[i].Kind {
					t.Errorf("After Sanitize() Traits[%d].Kind = %v, want %v", i, trait.Kind, tt.want[i].Kind)
				}
				if trait.Name != tt.want[i].Name {
					t.Errorf("After Sanitize() Traits[%d].Name = %v, want %v", i, trait.Name, tt.want[i].Name)
				}
				if trait.InstanceName != tt.want[i].InstanceName {
					t.Errorf("After Sanitize() Traits[%d].InstanceName = %v, want %v", i, trait.InstanceName, tt.want[i].InstanceName)
				}
			}
		})
	}
}

func TestUpdateComponentTraitsRequest_Validate(t *testing.T) {
	tests := []struct {
		name    string
		traits  []ComponentTraitRequest
		wantErr bool
		errMsg  string
	}{
		{
			name: "Valid traits",
			traits: []ComponentTraitRequest{
				{Name: "logging", InstanceName: "app-logging"},
				{Name: "scaling", InstanceName: "auto-scale"},
			},
			wantErr: false,
		},
		{
			name:    "Empty traits - valid",
			traits:  []ComponentTraitRequest{},
			wantErr: false,
		},
		{
			name: "Missing trait name",
			traits: []ComponentTraitRequest{
				{Name: "", InstanceName: "app-logging"},
			},
			wantErr: true,
			errMsg:  "trait name is required at index 0",
		},
		{
			name: "Missing instance name",
			traits: []ComponentTraitRequest{
				{Name: "logging", InstanceName: ""},
			},
			wantErr: true,
			errMsg:  "trait instanceName is required at index 0",
		},
		{
			name: "Duplicate instance names",
			traits: []ComponentTraitRequest{
				{Name: "logging", InstanceName: "my-trait"},
				{Name: "scaling", InstanceName: "my-trait"},
			},
			wantErr: true,
			errMsg:  "duplicate trait instanceName: my-trait",
		},
		{
			name: "Same trait name with different instance names - valid",
			traits: []ComponentTraitRequest{
				{Name: "logging", InstanceName: "logging-1"},
				{Name: "logging", InstanceName: "logging-2"},
			},
			wantErr: false,
		},
		{
			name: "Whitespace-only name",
			traits: []ComponentTraitRequest{
				{Name: "   ", InstanceName: "app-logging"},
			},
			wantErr: true,
			errMsg:  "trait name is required at index 0",
		},
		{
			name: "Whitespace-only instance name",
			traits: []ComponentTraitRequest{
				{Name: "logging", InstanceName: "   "},
			},
			wantErr: true,
			errMsg:  "trait instanceName is required at index 0",
		},
		{
			name: "Error at second index",
			traits: []ComponentTraitRequest{
				{Name: "logging", InstanceName: "app-logging"},
				{Name: "", InstanceName: "other-logging"},
			},
			wantErr: true,
			errMsg:  "trait name is required at index 1",
		},
		{
			name: "Valid traits with parameters",
			traits: []ComponentTraitRequest{
				{
					Name:         "logging",
					InstanceName: "app-logging",
					Parameters:   map[string]interface{}{"level": "info"},
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &UpdateComponentTraitsRequest{
				Traits: tt.traits,
			}
			err := req.Validate()

			if tt.wantErr {
				if err == nil {
					t.Errorf("Validate() expected error but got none")
					return
				}
				if err.Error() != tt.errMsg {
					t.Errorf("Validate() error = %v, want %v", err.Error(), tt.errMsg)
				}
			} else {
				if err != nil {
					t.Errorf("Validate() unexpected error = %v", err)
				}
			}
		})
	}
}

func TestCreateComponentRequest_Validate(t *testing.T) {
	tests := []struct {
		name    string
		req     *CreateComponentRequest
		wantErr bool
		errMsg  string
	}{
		{
			name: "Valid request with ComponentType",
			req: &CreateComponentRequest{
				Name: "my-comp",
				ComponentType: &ComponentTypeRef{
					Kind: "ComponentType",
					Name: "deployment/web-app",
				},
			},
			wantErr: false,
		},
		{
			name: "Valid request with ClusterComponentType",
			req: &CreateComponentRequest{
				Name: "my-comp",
				ComponentType: &ComponentTypeRef{
					Kind: "ClusterComponentType",
					Name: "deployment/shared-web",
				},
			},
			wantErr: false,
		},
		{
			name: "Valid request with empty kind (defaults to ComponentType)",
			req: &CreateComponentRequest{
				Name: "my-comp",
				ComponentType: &ComponentTypeRef{
					Name: "deployment/web-app",
				},
			},
			wantErr: false,
		},
		{
			name: "Invalid - missing ComponentType",
			req: &CreateComponentRequest{
				Name: "my-comp",
			},
			wantErr: true,
			errMsg:  "componentType is required",
		},
		{
			name: "Invalid - ComponentType with empty name",
			req: &CreateComponentRequest{
				Name: "my-comp",
				ComponentType: &ComponentTypeRef{
					Kind: "ComponentType",
					Name: "",
				},
			},
			wantErr: true,
			errMsg:  "componentType.name is required",
		},
		{
			name: "Invalid - ComponentType with whitespace-only name",
			req: &CreateComponentRequest{
				Name: "my-comp",
				ComponentType: &ComponentTypeRef{
					Kind: "ComponentType",
					Name: "   ",
				},
			},
			wantErr: true,
			errMsg:  "componentType.name is required",
		},
		{
			name: "Invalid - ComponentType with bad kind",
			req: &CreateComponentRequest{
				Name: "my-comp",
				ComponentType: &ComponentTypeRef{
					Kind: "InvalidKind",
					Name: "deployment/web-app",
				},
			},
			wantErr: true,
			errMsg:  "componentType.kind must be 'ComponentType' or 'ClusterComponentType', got 'InvalidKind'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.req.Validate()

			if tt.wantErr {
				if err == nil {
					t.Errorf("Validate() expected error but got none")
					return
				}
				if err.Error() != tt.errMsg {
					t.Errorf("Validate() error = %v, want %v", err.Error(), tt.errMsg)
				}
			} else {
				if err != nil {
					t.Errorf("Validate() unexpected error = %v", err)
				}
			}
		})
	}
}

func TestCreateComponentRequest_Sanitize(t *testing.T) {
	tests := []struct {
		name string
		req  *CreateComponentRequest
		want *CreateComponentRequest
	}{
		{
			name: "Nil ComponentType - no panic, other fields trimmed",
			req: &CreateComponentRequest{
				Name:          "  my-comp  ",
				DisplayName:   "  My Component  ",
				Description:   "  desc  ",
				ComponentType: nil,
			},
			want: &CreateComponentRequest{
				Name:          "my-comp",
				DisplayName:   "My Component",
				Description:   "desc",
				ComponentType: nil,
			},
		},
		{
			name: "ComponentType with whitespace in Kind and Name",
			req: &CreateComponentRequest{
				Name: "comp",
				ComponentType: &ComponentTypeRef{
					Kind: "  ComponentType  ",
					Name: "  deployment/web-app  ",
				},
			},
			want: &CreateComponentRequest{
				Name: "comp",
				ComponentType: &ComponentTypeRef{
					Kind: "ComponentType",
					Name: "deployment/web-app",
				},
			},
		},
		{
			name: "Full request with all fields having whitespace",
			req: &CreateComponentRequest{
				Name:        "  my-comp  ",
				DisplayName: "  My Comp  ",
				Description: "  A description  ",
				ComponentType: &ComponentTypeRef{
					Kind: "  ClusterComponentType  ",
					Name: "  deployment/my-ct  ",
				},
				Traits: []ComponentTrait{
					{Name: "  logging  ", InstanceName: "  app-logging  "},
				},
			},
			want: &CreateComponentRequest{
				Name:        "my-comp",
				DisplayName: "My Comp",
				Description: "A description",
				ComponentType: &ComponentTypeRef{
					Kind: "ClusterComponentType",
					Name: "deployment/my-ct",
				},
				Traits: []ComponentTrait{
					{Name: "logging", InstanceName: "app-logging"},
				},
			},
		},
		{
			name: "Trait Kind field with whitespace is trimmed",
			req: &CreateComponentRequest{
				Name: "comp",
				ComponentType: &ComponentTypeRef{
					Kind: "ComponentType",
					Name: "deployment/web-app",
				},
				Traits: []ComponentTrait{
					{Kind: "  Trait  ", Name: "logging", InstanceName: "app-logging"},
					{Kind: "  ClusterTrait  ", Name: "global-logger", InstanceName: "my-logger"},
				},
			},
			want: &CreateComponentRequest{
				Name: "comp",
				ComponentType: &ComponentTypeRef{
					Kind: "ComponentType",
					Name: "deployment/web-app",
				},
				Traits: []ComponentTrait{
					{Kind: "Trait", Name: "logging", InstanceName: "app-logging"},
					{Kind: "ClusterTrait", Name: "global-logger", InstanceName: "my-logger"},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.req.Sanitize()

			if tt.req.Name != tt.want.Name {
				t.Errorf("Name = %q, want %q", tt.req.Name, tt.want.Name)
			}
			if tt.req.DisplayName != tt.want.DisplayName {
				t.Errorf("DisplayName = %q, want %q", tt.req.DisplayName, tt.want.DisplayName)
			}
			if tt.req.Description != tt.want.Description {
				t.Errorf("Description = %q, want %q", tt.req.Description, tt.want.Description)
			}

			if tt.want.ComponentType == nil {
				if tt.req.ComponentType != nil {
					t.Errorf("ComponentType = %v, want nil", tt.req.ComponentType)
				}
			} else {
				if tt.req.ComponentType == nil {
					t.Fatal("ComponentType is nil, want non-nil")
				}
				if tt.req.ComponentType.Kind != tt.want.ComponentType.Kind {
					t.Errorf("ComponentType.Kind = %q, want %q", tt.req.ComponentType.Kind, tt.want.ComponentType.Kind)
				}
				if tt.req.ComponentType.Name != tt.want.ComponentType.Name {
					t.Errorf("ComponentType.Name = %q, want %q", tt.req.ComponentType.Name, tt.want.ComponentType.Name)
				}
			}

			for i, trait := range tt.req.Traits {
				if trait.Kind != tt.want.Traits[i].Kind {
					t.Errorf("Traits[%d].Kind = %q, want %q", i, trait.Kind, tt.want.Traits[i].Kind)
				}
				if trait.Name != tt.want.Traits[i].Name {
					t.Errorf("Traits[%d].Name = %q, want %q", i, trait.Name, tt.want.Traits[i].Name)
				}
				if trait.InstanceName != tt.want.Traits[i].InstanceName {
					t.Errorf("Traits[%d].InstanceName = %q, want %q", i, trait.InstanceName, tt.want.Traits[i].InstanceName)
				}
			}
		})
	}
}

func TestPatchReleaseBindingRequest(t *testing.T) {
	tests := []struct {
		name        string
		req         *PatchReleaseBindingRequest
		description string
	}{
		{
			name: "With component type overrides",
			req: &PatchReleaseBindingRequest{
				ComponentTypeEnvironmentConfigs: map[string]interface{}{
					"replicas": 3,
					"cpu":      "500m",
				},
			},
			description: "Should accept component type overrides",
		},
		{
			name: "With trait environment configs",
			req: &PatchReleaseBindingRequest{
				TraitEnvironmentConfigs: map[string]map[string]interface{}{
					"ingress": {
						"host": "example.com",
					},
				},
			},
			description: "Should accept trait environment configs",
		},
		{
			name: "With workload overrides",
			req: &PatchReleaseBindingRequest{
				WorkloadOverrides: &WorkloadOverrides{
					Container: &ContainerOverride{
						Env: []EnvVar{
							{Key: "ENV", Value: "production"},
						},
						Files: []FileVar{
							{Key: "config", MountPath: "/etc/config", Value: "data"},
						},
					},
				},
			},
			description: "Should accept workload overrides",
		},
		{
			name:        "Empty request",
			req:         &PatchReleaseBindingRequest{},
			description: "Should accept empty overrides",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Verify that the request struct can be created and holds the expected data
			if tt.req == nil {
				t.Error("Request should not be nil")
			}

			// For component type overrides
			if tt.req.ComponentTypeEnvironmentConfigs != nil {
				if len(tt.req.ComponentTypeEnvironmentConfigs) == 0 {
					t.Error("ComponentTypeEnvironmentConfigs should not be empty when set")
				}
			}

			// For trait environment configs
			if tt.req.TraitEnvironmentConfigs != nil {
				if len(tt.req.TraitEnvironmentConfigs) == 0 {
					t.Error("TraitEnvironmentConfigs should not be empty when set")
				}
			}

			// For workload overrides
			if tt.req.WorkloadOverrides != nil {
				if tt.req.WorkloadOverrides.Container == nil {
					t.Error("WorkloadOverrides should have a container override")
				}
			}
		})
	}
}

// ---- CreateProjectRequest ----

func TestCreateProjectRequest_Sanitize(t *testing.T) {
	tests := []struct {
		name string
		req  *CreateProjectRequest
		want *CreateProjectRequest
	}{
		{
			name: "No whitespace",
			req: &CreateProjectRequest{
				Name:               "my-project",
				DisplayName:        "My Project",
				Description:        "A project",
				DeploymentPipeline: "default",
			},
			want: &CreateProjectRequest{
				Name:               "my-project",
				DisplayName:        "My Project",
				Description:        "A project",
				DeploymentPipeline: "default",
			},
		},
		{
			name: "All fields have surrounding whitespace",
			req: &CreateProjectRequest{
				Name:               "  my-project  ",
				DisplayName:        "  My Project  ",
				Description:        "  A project  ",
				DeploymentPipeline: "  default  ",
			},
			want: &CreateProjectRequest{
				Name:               "my-project",
				DisplayName:        "My Project",
				Description:        "A project",
				DeploymentPipeline: "default",
			},
		},
		{
			name: "All fields empty",
			req:  &CreateProjectRequest{},
			want: &CreateProjectRequest{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.req.Sanitize()
			if tt.req.Name != tt.want.Name {
				t.Errorf("Name = %q, want %q", tt.req.Name, tt.want.Name)
			}
			if tt.req.DisplayName != tt.want.DisplayName {
				t.Errorf("DisplayName = %q, want %q", tt.req.DisplayName, tt.want.DisplayName)
			}
			if tt.req.Description != tt.want.Description {
				t.Errorf("Description = %q, want %q", tt.req.Description, tt.want.Description)
			}
			if tt.req.DeploymentPipeline != tt.want.DeploymentPipeline {
				t.Errorf("DeploymentPipeline = %q, want %q", tt.req.DeploymentPipeline, tt.want.DeploymentPipeline)
			}
		})
	}
}

// TestCreateProjectRequest_Validate documents that Validate is a no-op stub.
// This test will catch any future logic added without a corresponding test.
func TestCreateProjectRequest_Validate(t *testing.T) {
	reqs := []*CreateProjectRequest{
		{},
		{Name: "my-project"},
		{Name: "my-project", DisplayName: "My Project", Description: "desc"},
	}
	for _, req := range reqs {
		if err := req.Validate(); err != nil {
			t.Errorf("Validate() on %+v returned unexpected error: %v", req, err)
		}
	}
}

// ---- CreateEnvironmentRequest ----

func TestCreateEnvironmentRequest_Sanitize(t *testing.T) {
	tests := []struct {
		name string
		req  *CreateEnvironmentRequest
		want *CreateEnvironmentRequest
	}{
		{
			name: "No whitespace",
			req: &CreateEnvironmentRequest{
				Name:        "dev",
				DisplayName: "Development",
				Description: "dev env",
				DNSPrefix:   "dev",
			},
			want: &CreateEnvironmentRequest{
				Name:        "dev",
				DisplayName: "Development",
				Description: "dev env",
				DNSPrefix:   "dev",
			},
		},
		{
			name: "All scalar fields trimmed",
			req: &CreateEnvironmentRequest{
				Name:        "  dev  ",
				DisplayName: "  Development  ",
				Description: "  dev env  ",
				DNSPrefix:   "  dev  ",
			},
			want: &CreateEnvironmentRequest{
				Name:        "dev",
				DisplayName: "Development",
				Description: "dev env",
				DNSPrefix:   "dev",
			},
		},
		{
			name: "DataPlaneRef fields trimmed when set",
			req: &CreateEnvironmentRequest{
				Name: "dev",
				DataPlaneRef: &DataPlaneRef{
					Kind: "  DataPlane  ",
					Name: "  primary  ",
				},
			},
			want: &CreateEnvironmentRequest{
				Name: "dev",
				DataPlaneRef: &DataPlaneRef{
					Kind: "DataPlane",
					Name: "primary",
				},
			},
		},
		{
			name: "Nil DataPlaneRef is safe",
			req: &CreateEnvironmentRequest{
				Name:         "  dev  ",
				DataPlaneRef: nil,
			},
			want: &CreateEnvironmentRequest{
				Name:         "dev",
				DataPlaneRef: nil,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.req.Sanitize()
			if tt.req.Name != tt.want.Name {
				t.Errorf("Name = %q, want %q", tt.req.Name, tt.want.Name)
			}
			if tt.req.DisplayName != tt.want.DisplayName {
				t.Errorf("DisplayName = %q, want %q", tt.req.DisplayName, tt.want.DisplayName)
			}
			if tt.req.Description != tt.want.Description {
				t.Errorf("Description = %q, want %q", tt.req.Description, tt.want.Description)
			}
			if tt.req.DNSPrefix != tt.want.DNSPrefix {
				t.Errorf("DNSPrefix = %q, want %q", tt.req.DNSPrefix, tt.want.DNSPrefix)
			}
			if tt.want.DataPlaneRef == nil {
				if tt.req.DataPlaneRef != nil {
					t.Errorf("DataPlaneRef = %v, want nil", tt.req.DataPlaneRef)
				}
			} else {
				if tt.req.DataPlaneRef == nil {
					t.Fatal("DataPlaneRef is nil, want non-nil")
				}
				if tt.req.DataPlaneRef.Kind != tt.want.DataPlaneRef.Kind {
					t.Errorf("DataPlaneRef.Kind = %q, want %q", tt.req.DataPlaneRef.Kind, tt.want.DataPlaneRef.Kind)
				}
				if tt.req.DataPlaneRef.Name != tt.want.DataPlaneRef.Name {
					t.Errorf("DataPlaneRef.Name = %q, want %q", tt.req.DataPlaneRef.Name, tt.want.DataPlaneRef.Name)
				}
			}
		})
	}
}

// TestCreateEnvironmentRequest_Validate documents that Validate is a no-op stub.
func TestCreateEnvironmentRequest_Validate(t *testing.T) {
	reqs := []*CreateEnvironmentRequest{
		{},
		{Name: "dev"},
		{Name: "prod", IsProduction: true, DataPlaneRef: &DataPlaneRef{Kind: "DataPlane", Name: "primary"}},
	}
	for _, req := range reqs {
		if err := req.Validate(); err != nil {
			t.Errorf("Validate() on %+v returned unexpected error: %v", req, err)
		}
	}
}

// ---- CreateDataPlaneRequest ----

func TestCreateDataPlaneRequest_Sanitize(t *testing.T) {
	tests := []struct {
		name string
		req  *CreateDataPlaneRequest
		want *CreateDataPlaneRequest
	}{
		{
			name: "No whitespace",
			req: &CreateDataPlaneRequest{
				Name:                 "primary",
				DisplayName:          "Primary",
				Description:          "main plane",
				ClusterAgentClientCA: "cert-data",
			},
			want: &CreateDataPlaneRequest{
				Name:                 "primary",
				DisplayName:          "Primary",
				Description:          "main plane",
				ClusterAgentClientCA: "cert-data",
			},
		},
		{
			name: "All scalar fields trimmed",
			req: &CreateDataPlaneRequest{
				Name:                 "  primary  ",
				DisplayName:          "  Primary  ",
				Description:          "  main plane  ",
				ClusterAgentClientCA: "  cert-data  ",
			},
			want: &CreateDataPlaneRequest{
				Name:                 "primary",
				DisplayName:          "Primary",
				Description:          "main plane",
				ClusterAgentClientCA: "cert-data",
			},
		},
		{
			name: "ObservabilityPlaneRef fields trimmed when set",
			req: &CreateDataPlaneRequest{
				Name: "primary",
				ObservabilityPlaneRef: &ObservabilityPlaneRef{
					Kind: "  ObservabilityPlane  ",
					Name: "  obs-plane  ",
				},
			},
			want: &CreateDataPlaneRequest{
				Name: "primary",
				ObservabilityPlaneRef: &ObservabilityPlaneRef{
					Kind: "ObservabilityPlane",
					Name: "obs-plane",
				},
			},
		},
		{
			name: "Nil ObservabilityPlaneRef is safe",
			req: &CreateDataPlaneRequest{
				Name:                  "  primary  ",
				ObservabilityPlaneRef: nil,
			},
			want: &CreateDataPlaneRequest{
				Name:                  "primary",
				ObservabilityPlaneRef: nil,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.req.Sanitize()
			if tt.req.Name != tt.want.Name {
				t.Errorf("Name = %q, want %q", tt.req.Name, tt.want.Name)
			}
			if tt.req.DisplayName != tt.want.DisplayName {
				t.Errorf("DisplayName = %q, want %q", tt.req.DisplayName, tt.want.DisplayName)
			}
			if tt.req.Description != tt.want.Description {
				t.Errorf("Description = %q, want %q", tt.req.Description, tt.want.Description)
			}
			if tt.req.ClusterAgentClientCA != tt.want.ClusterAgentClientCA {
				t.Errorf("ClusterAgentClientCA = %q, want %q", tt.req.ClusterAgentClientCA, tt.want.ClusterAgentClientCA)
			}
			if tt.want.ObservabilityPlaneRef == nil {
				if tt.req.ObservabilityPlaneRef != nil {
					t.Errorf("ObservabilityPlaneRef = %v, want nil", tt.req.ObservabilityPlaneRef)
				}
			} else {
				if tt.req.ObservabilityPlaneRef == nil {
					t.Fatal("ObservabilityPlaneRef is nil, want non-nil")
				}
				if tt.req.ObservabilityPlaneRef.Kind != tt.want.ObservabilityPlaneRef.Kind {
					t.Errorf("ObservabilityPlaneRef.Kind = %q, want %q",
						tt.req.ObservabilityPlaneRef.Kind, tt.want.ObservabilityPlaneRef.Kind)
				}
				if tt.req.ObservabilityPlaneRef.Name != tt.want.ObservabilityPlaneRef.Name {
					t.Errorf("ObservabilityPlaneRef.Name = %q, want %q",
						tt.req.ObservabilityPlaneRef.Name, tt.want.ObservabilityPlaneRef.Name)
				}
			}
		})
	}
}

func TestCreateDataPlaneRequest_Validate(t *testing.T) {
	tests := []struct {
		name      string
		req       *CreateDataPlaneRequest
		wantErr   bool
		errMsg    string // exact match; empty means skip exact check
		errPrefix string // prefix match for library-generated messages
	}{
		{
			name:    "No ObservabilityPlaneRef - valid",
			req:     &CreateDataPlaneRequest{Name: "primary", ClusterAgentClientCA: "cert"},
			wantErr: false,
		},
		{
			name: "ObservabilityPlaneRef with empty kind",
			req: &CreateDataPlaneRequest{
				Name:                  "primary",
				ObservabilityPlaneRef: &ObservabilityPlaneRef{Kind: "", Name: "obs-plane"},
			},
			wantErr: true,
			errMsg:  "observabilityPlaneRef.kind is required when observabilityPlaneRef is provided",
		},
		{
			name: "ObservabilityPlaneRef with invalid kind",
			req: &CreateDataPlaneRequest{
				Name:                  "primary",
				ObservabilityPlaneRef: &ObservabilityPlaneRef{Kind: "InvalidKind", Name: "obs-plane"},
			},
			wantErr: true,
			errMsg: "observabilityPlaneRef.kind must be 'ObservabilityPlane' or 'ClusterObservabilityPlane'" +
				", got 'InvalidKind'",
		},
		{
			name: "ObservabilityPlaneRef with valid kind ObservabilityPlane and empty name",
			req: &CreateDataPlaneRequest{
				Name:                  "primary",
				ObservabilityPlaneRef: &ObservabilityPlaneRef{Kind: "ObservabilityPlane", Name: ""},
			},
			wantErr: true,
			errMsg:  "observabilityPlaneRef.name is required when observabilityPlaneRef is provided",
		},
		{
			name: "ObservabilityPlaneRef with whitespace-only name",
			req: &CreateDataPlaneRequest{
				Name:                  "primary",
				ObservabilityPlaneRef: &ObservabilityPlaneRef{Kind: "ObservabilityPlane", Name: "   "},
			},
			wantErr: true,
			errMsg:  "observabilityPlaneRef.name is required when observabilityPlaneRef is provided",
		},
		{
			name: "ObservabilityPlaneRef name fails DNS-1123 validation",
			req: &CreateDataPlaneRequest{
				Name:                  "primary",
				ObservabilityPlaneRef: &ObservabilityPlaneRef{Kind: "ObservabilityPlane", Name: "UPPERCASE"},
			},
			wantErr:   true,
			errPrefix: "observabilityPlaneRef.name must be a valid DNS-1123 label",
		},
		{
			name: "ObservabilityPlaneRef with kind ObservabilityPlane and valid name",
			req: &CreateDataPlaneRequest{
				Name:                  "primary",
				ObservabilityPlaneRef: &ObservabilityPlaneRef{Kind: "ObservabilityPlane", Name: "obs-plane"},
			},
			wantErr: false,
		},
		{
			name: "ObservabilityPlaneRef with kind ClusterObservabilityPlane and valid name",
			req: &CreateDataPlaneRequest{
				Name:                  "primary",
				ObservabilityPlaneRef: &ObservabilityPlaneRef{Kind: "ClusterObservabilityPlane", Name: "cluster-obs"},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.req.Validate()
			if tt.wantErr {
				if err == nil {
					t.Errorf("Validate() expected error but got none")
					return
				}
				if tt.errMsg != "" && err.Error() != tt.errMsg {
					t.Errorf("Validate() error = %v, want %v", err.Error(), tt.errMsg)
				}
				if tt.errPrefix != "" && !strings.HasPrefix(err.Error(), tt.errPrefix) {
					t.Errorf("Validate() error = %v, want prefix %v", err.Error(), tt.errPrefix)
				}
			} else {
				if err != nil {
					t.Errorf("Validate() unexpected error = %v", err)
				}
			}
		})
	}
}

// ---- CreateClusterDataPlaneRequest ----

func TestCreateClusterDataPlaneRequest_Sanitize(t *testing.T) {
	tests := []struct {
		name string
		req  *CreateClusterDataPlaneRequest
		want *CreateClusterDataPlaneRequest
	}{
		{
			name: "No whitespace",
			req: &CreateClusterDataPlaneRequest{
				Name:                 "my-cdp",
				DisplayName:          "My CDP",
				Description:          "cluster plane",
				PlaneID:              "plane-01",
				ClusterAgentClientCA: "cert-data",
			},
			want: &CreateClusterDataPlaneRequest{
				Name:                 "my-cdp",
				DisplayName:          "My CDP",
				Description:          "cluster plane",
				PlaneID:              "plane-01",
				ClusterAgentClientCA: "cert-data",
			},
		},
		{
			name: "All scalar fields trimmed",
			req: &CreateClusterDataPlaneRequest{
				Name:                 "  my-cdp  ",
				DisplayName:          "  My CDP  ",
				Description:          "  cluster plane  ",
				PlaneID:              "  plane-01  ",
				ClusterAgentClientCA: "  cert-data  ",
			},
			want: &CreateClusterDataPlaneRequest{
				Name:                 "my-cdp",
				DisplayName:          "My CDP",
				Description:          "cluster plane",
				PlaneID:              "plane-01",
				ClusterAgentClientCA: "cert-data",
			},
		},
		{
			name: "ObservabilityPlaneRef trimmed when set",
			req: &CreateClusterDataPlaneRequest{
				Name:    "my-cdp",
				PlaneID: "plane-01",
				ObservabilityPlaneRef: &ObservabilityPlaneRef{
					Kind: "  ClusterObservabilityPlane  ",
					Name: "  obs-cluster  ",
				},
			},
			want: &CreateClusterDataPlaneRequest{
				Name:    "my-cdp",
				PlaneID: "plane-01",
				ObservabilityPlaneRef: &ObservabilityPlaneRef{
					Kind: "ClusterObservabilityPlane",
					Name: "obs-cluster",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.req.Sanitize()
			if tt.req.Name != tt.want.Name {
				t.Errorf("Name = %q, want %q", tt.req.Name, tt.want.Name)
			}
			if tt.req.DisplayName != tt.want.DisplayName {
				t.Errorf("DisplayName = %q, want %q", tt.req.DisplayName, tt.want.DisplayName)
			}
			if tt.req.Description != tt.want.Description {
				t.Errorf("Description = %q, want %q", tt.req.Description, tt.want.Description)
			}
			if tt.req.PlaneID != tt.want.PlaneID {
				t.Errorf("PlaneID = %q, want %q", tt.req.PlaneID, tt.want.PlaneID)
			}
			if tt.req.ClusterAgentClientCA != tt.want.ClusterAgentClientCA {
				t.Errorf("ClusterAgentClientCA = %q, want %q", tt.req.ClusterAgentClientCA, tt.want.ClusterAgentClientCA)
			}
			if tt.want.ObservabilityPlaneRef != nil {
				if tt.req.ObservabilityPlaneRef == nil {
					t.Fatal("ObservabilityPlaneRef is nil, want non-nil")
				}
				if tt.req.ObservabilityPlaneRef.Kind != tt.want.ObservabilityPlaneRef.Kind {
					t.Errorf("ObservabilityPlaneRef.Kind = %q, want %q",
						tt.req.ObservabilityPlaneRef.Kind, tt.want.ObservabilityPlaneRef.Kind)
				}
				if tt.req.ObservabilityPlaneRef.Name != tt.want.ObservabilityPlaneRef.Name {
					t.Errorf("ObservabilityPlaneRef.Name = %q, want %q",
						tt.req.ObservabilityPlaneRef.Name, tt.want.ObservabilityPlaneRef.Name)
				}
			}
		})
	}
}

func TestCreateClusterDataPlaneRequest_Validate(t *testing.T) {
	validReq := func() *CreateClusterDataPlaneRequest {
		return &CreateClusterDataPlaneRequest{
			Name:                 "my-cdp",
			PlaneID:              "plane-01",
			ClusterAgentClientCA: "cert-data",
		}
	}

	tests := []struct {
		name      string
		req       *CreateClusterDataPlaneRequest
		wantErr   bool
		errMsg    string // exact match when library output is deterministic
		errPrefix string // prefix match for DNS-1123 library messages
	}{
		{
			name:    "Valid request - no observability ref",
			req:     validReq(),
			wantErr: false,
		},
		{
			name:    "Empty name",
			req:     &CreateClusterDataPlaneRequest{PlaneID: "p1", ClusterAgentClientCA: "cert"},
			wantErr: true,
			errMsg:  "name is required",
		},
		{
			name:    "Whitespace-only name",
			req:     &CreateClusterDataPlaneRequest{Name: "   ", PlaneID: "p1", ClusterAgentClientCA: "cert"},
			wantErr: true,
			errMsg:  "name is required",
		},
		{
			name:      "Name fails DNS-1123 validation",
			req:       &CreateClusterDataPlaneRequest{Name: "UPPERCASE", PlaneID: "p1", ClusterAgentClientCA: "cert"},
			wantErr:   true,
			errPrefix: "name must be a valid DNS-1123 label",
		},
		{
			name:    "Empty planeID",
			req:     &CreateClusterDataPlaneRequest{Name: "my-cdp", ClusterAgentClientCA: "cert"},
			wantErr: true,
			errMsg:  "planeID is required",
		},
		{
			name:    "Whitespace-only planeID",
			req:     &CreateClusterDataPlaneRequest{Name: "my-cdp", PlaneID: "   ", ClusterAgentClientCA: "cert"},
			wantErr: true,
			errMsg:  "planeID is required",
		},
		{
			name:      "PlaneID fails DNS-1123 validation",
			req:       &CreateClusterDataPlaneRequest{Name: "my-cdp", PlaneID: "INVALID_ID", ClusterAgentClientCA: "cert"},
			wantErr:   true,
			errPrefix: "planeID must be a valid DNS-1123 label",
		},
		{
			name:    "Empty clusterAgentClientCA",
			req:     &CreateClusterDataPlaneRequest{Name: "my-cdp", PlaneID: "p1"},
			wantErr: true,
			errMsg:  "clusterAgentClientCA is required",
		},
		{
			name:    "Whitespace-only clusterAgentClientCA",
			req:     &CreateClusterDataPlaneRequest{Name: "my-cdp", PlaneID: "p1", ClusterAgentClientCA: "  "},
			wantErr: true,
			errMsg:  "clusterAgentClientCA is required",
		},
		{
			name: "ObservabilityPlaneRef with empty kind",
			req: func() *CreateClusterDataPlaneRequest {
				r := validReq()
				r.ObservabilityPlaneRef = &ObservabilityPlaneRef{Kind: "", Name: "obs"}
				return r
			}(),
			wantErr: true,
			errMsg:  "observabilityPlaneRef.kind is required when observabilityPlaneRef is provided",
		},
		{
			// cluster-scoped resources must use ClusterObservabilityPlane, not ObservabilityPlane
			name: "ObservabilityPlaneRef with namespace-scoped kind is rejected",
			req: func() *CreateClusterDataPlaneRequest {
				r := validReq()
				r.ObservabilityPlaneRef = &ObservabilityPlaneRef{Kind: "ObservabilityPlane", Name: "obs"}
				return r
			}(),
			wantErr: true,
			errMsg: "observabilityPlaneRef.kind must be 'ClusterObservabilityPlane' for cluster-scoped resources" +
				", got 'ObservabilityPlane'",
		},
		{
			name: "ObservabilityPlaneRef with empty name",
			req: func() *CreateClusterDataPlaneRequest {
				r := validReq()
				r.ObservabilityPlaneRef = &ObservabilityPlaneRef{Kind: "ClusterObservabilityPlane", Name: ""}
				return r
			}(),
			wantErr: true,
			errMsg:  "observabilityPlaneRef.name is required when observabilityPlaneRef is provided",
		},
		{
			name: "ObservabilityPlaneRef name fails DNS-1123 validation",
			req: func() *CreateClusterDataPlaneRequest {
				r := validReq()
				r.ObservabilityPlaneRef = &ObservabilityPlaneRef{Kind: "ClusterObservabilityPlane", Name: "INVALID"}
				return r
			}(),
			wantErr:   true,
			errPrefix: "observabilityPlaneRef.name must be a valid DNS-1123 label",
		},
		{
			name: "Valid request with ClusterObservabilityPlane ref",
			req: func() *CreateClusterDataPlaneRequest {
				r := validReq()
				r.ObservabilityPlaneRef = &ObservabilityPlaneRef{Kind: "ClusterObservabilityPlane", Name: "obs-cluster"}
				return r
			}(),
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.req.Validate()
			if tt.wantErr {
				if err == nil {
					t.Errorf("Validate() expected error but got none")
					return
				}
				if tt.errMsg != "" && err.Error() != tt.errMsg {
					t.Errorf("Validate() error = %v, want %v", err.Error(), tt.errMsg)
				}
				if tt.errPrefix != "" && !strings.HasPrefix(err.Error(), tt.errPrefix) {
					t.Errorf("Validate() error = %v, want prefix %v", err.Error(), tt.errPrefix)
				}
			} else {
				if err != nil {
					t.Errorf("Validate() unexpected error = %v", err)
				}
			}
		})
	}
}

// ---- CreateGitSecretRequest ----

func TestCreateGitSecretRequest_Sanitize(t *testing.T) {
	tests := []struct {
		name string
		req  *CreateGitSecretRequest
		want *CreateGitSecretRequest
	}{
		{
			name: "No whitespace",
			req: &CreateGitSecretRequest{
				SecretName: "my-secret",
				SecretType: "basic-auth",
				Username:   "user",
				Token:      "ghp_token",
			},
			want: &CreateGitSecretRequest{
				SecretName: "my-secret",
				SecretType: "basic-auth",
				Username:   "user",
				Token:      "ghp_token",
			},
		},
		{
			name: "All fields trimmed",
			req: &CreateGitSecretRequest{
				SecretName: "  my-secret  ",
				SecretType: "  ssh-auth  ",
				Username:   "  user  ",
				Token:      "  token  ",
				SSHKey:     "  -----BEGIN...  ",
				SSHKEYID:   "  APKA...  ",
			},
			want: &CreateGitSecretRequest{
				SecretName: "my-secret",
				SecretType: "ssh-auth",
				Username:   "user",
				Token:      "token",
				SSHKey:     "-----BEGIN...",
				SSHKEYID:   "APKA...",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.req.Sanitize()
			if tt.req.SecretName != tt.want.SecretName {
				t.Errorf("SecretName = %q, want %q", tt.req.SecretName, tt.want.SecretName)
			}
			if tt.req.SecretType != tt.want.SecretType {
				t.Errorf("SecretType = %q, want %q", tt.req.SecretType, tt.want.SecretType)
			}
			if tt.req.Username != tt.want.Username {
				t.Errorf("Username = %q, want %q", tt.req.Username, tt.want.Username)
			}
			if tt.req.Token != tt.want.Token {
				t.Errorf("Token = %q, want %q", tt.req.Token, tt.want.Token)
			}
			if tt.req.SSHKey != tt.want.SSHKey {
				t.Errorf("SSHKey = %q, want %q", tt.req.SSHKey, tt.want.SSHKey)
			}
			if tt.req.SSHKEYID != tt.want.SSHKEYID {
				t.Errorf("SSHKEYID = %q, want %q", tt.req.SSHKEYID, tt.want.SSHKEYID)
			}
		})
	}
}

func TestCreateGitSecretRequest_Validate(t *testing.T) {
	// secretName of exactly 253 characters (at the boundary — must pass).
	longValidName := strings.Repeat("a", 253)
	// secretName of 254 characters (one over — must fail).
	tooLongName := strings.Repeat("a", 254)

	tests := []struct {
		name    string
		req     *CreateGitSecretRequest
		wantErr bool
		errMsg  string
	}{
		// secretName validation
		{
			name:    "Empty secretName",
			req:     &CreateGitSecretRequest{SecretType: "basic-auth"},
			wantErr: true,
			errMsg:  "secretName is required",
		},
		{
			name:    "Whitespace-only secretName",
			req:     &CreateGitSecretRequest{SecretName: "   ", SecretType: "basic-auth"},
			wantErr: true,
			errMsg:  "secretName is required",
		},
		{
			name:    "secretName exactly 253 characters passes length check",
			req:     &CreateGitSecretRequest{SecretName: longValidName, SecretType: "basic-auth", Token: "tok"},
			wantErr: false,
		},
		{
			name:    "secretName 254 characters exceeds limit",
			req:     &CreateGitSecretRequest{SecretName: tooLongName, SecretType: "basic-auth"},
			wantErr: true,
			errMsg:  "secretName must be at most 253 characters",
		},
		// secretType validation
		{
			name:    "Empty secretType",
			req:     &CreateGitSecretRequest{SecretName: "my-secret"},
			wantErr: true,
			errMsg:  "secretType must be 'basic-auth' or 'ssh-auth'",
		},
		{
			name:    "Unknown secretType",
			req:     &CreateGitSecretRequest{SecretName: "my-secret", SecretType: "oauth"},
			wantErr: true,
			errMsg:  "secretType must be 'basic-auth' or 'ssh-auth'",
		},
		// basic-auth cases
		{
			name:    "basic-auth without token",
			req:     &CreateGitSecretRequest{SecretName: "s", SecretType: "basic-auth"},
			wantErr: true,
			errMsg:  "token is required for basic-auth type",
		},
		{
			name:    "basic-auth with whitespace-only token",
			req:     &CreateGitSecretRequest{SecretName: "s", SecretType: "basic-auth", Token: "   "},
			wantErr: true,
			errMsg:  "token is required for basic-auth type",
		},
		{
			name:    "basic-auth with token and sshKey set (mutually exclusive)",
			req:     &CreateGitSecretRequest{SecretName: "s", SecretType: "basic-auth", Token: "tok", SSHKey: "key"},
			wantErr: true,
			errMsg:  "sshKey must not be provided for basic-auth type",
		},
		{
			name: "basic-auth with token and sshKeyId set (mutually exclusive)",
			req: &CreateGitSecretRequest{
				SecretName: "s", SecretType: "basic-auth", Token: "tok", SSHKEYID: "APKA123",
			},
			wantErr: true,
			errMsg:  "sshKeyId must not be provided for basic-auth type",
		},
		{
			name:    "basic-auth with token only (no username) is valid",
			req:     &CreateGitSecretRequest{SecretName: "s", SecretType: "basic-auth", Token: "ghp_tok"},
			wantErr: false,
		},
		{
			name: "basic-auth with username and token is valid",
			req: &CreateGitSecretRequest{
				SecretName: "s", SecretType: "basic-auth", Username: "alice", Token: "ghp_tok",
			},
			wantErr: false,
		},
		// ssh-auth cases
		{
			name:    "ssh-auth without sshKey",
			req:     &CreateGitSecretRequest{SecretName: "s", SecretType: "ssh-auth"},
			wantErr: true,
			errMsg:  "sshKey is required for ssh-auth type",
		},
		{
			name:    "ssh-auth with whitespace-only sshKey",
			req:     &CreateGitSecretRequest{SecretName: "s", SecretType: "ssh-auth", SSHKey: "   "},
			wantErr: true,
			errMsg:  "sshKey is required for ssh-auth type",
		},
		{
			name: "ssh-auth with sshKey and token set (mutually exclusive)",
			req: &CreateGitSecretRequest{
				SecretName: "s", SecretType: "ssh-auth", SSHKey: "key", Token: "tok",
			},
			wantErr: true,
			errMsg:  "token must not be provided for ssh-auth type",
		},
		{
			name: "ssh-auth with sshKey and username set (mutually exclusive)",
			req: &CreateGitSecretRequest{
				SecretName: "s", SecretType: "ssh-auth", SSHKey: "key", Username: "user",
			},
			wantErr: true,
			errMsg:  "username must not be provided for ssh-auth type",
		},
		{
			name:    "ssh-auth with sshKey only is valid",
			req:     &CreateGitSecretRequest{SecretName: "s", SecretType: "ssh-auth", SSHKey: "-----BEGIN..."},
			wantErr: false,
		},
		{
			name: "ssh-auth with sshKey and sshKeyId is valid (AWS CodeCommit)",
			req: &CreateGitSecretRequest{
				SecretName: "s", SecretType: "ssh-auth", SSHKey: "-----BEGIN...", SSHKEYID: "APKA123",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.req.Validate()
			if tt.wantErr {
				if err == nil {
					t.Errorf("Validate() expected error but got none")
					return
				}
				if err.Error() != tt.errMsg {
					t.Errorf("Validate() error = %v, want %v", err.Error(), tt.errMsg)
				}
			} else {
				if err != nil {
					t.Errorf("Validate() unexpected error = %v", err)
				}
			}
		})
	}
}

// ---- CreateWorkflowRunRequest ----

func TestCreateWorkflowRunRequest_Sanitize(t *testing.T) {
	tests := []struct {
		name         string
		workflowName string
		want         string
	}{
		{name: "No whitespace", workflowName: "my-workflow", want: "my-workflow"},
		{name: "Leading whitespace", workflowName: "  my-workflow", want: "my-workflow"},
		{name: "Trailing whitespace", workflowName: "my-workflow  ", want: "my-workflow"},
		{name: "Both sides", workflowName: "  my-workflow  ", want: "my-workflow"},
		{name: "Empty string", workflowName: "", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &CreateWorkflowRunRequest{WorkflowName: tt.workflowName}
			req.Sanitize()
			if req.WorkflowName != tt.want {
				t.Errorf("WorkflowName = %q, want %q", req.WorkflowName, tt.want)
			}
		})
	}
}

func TestCreateWorkflowRunRequest_Validate(t *testing.T) {
	tests := []struct {
		name         string
		workflowName string
		wantErr      bool
		errMsg       string
	}{
		{
			name:         "Valid workflowName",
			workflowName: "build-and-push",
			wantErr:      false,
		},
		{
			name:         "Empty workflowName",
			workflowName: "",
			wantErr:      true,
			errMsg:       "workflowName is required",
		},
		{
			name:         "Whitespace-only workflowName",
			workflowName: "   ",
			wantErr:      true,
			errMsg:       "workflowName is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &CreateWorkflowRunRequest{WorkflowName: tt.workflowName}
			err := req.Validate()
			if tt.wantErr {
				if err == nil {
					t.Errorf("Validate() expected error but got none")
					return
				}
				if err.Error() != tt.errMsg {
					t.Errorf("Validate() error = %v, want %v", err.Error(), tt.errMsg)
				}
			} else {
				if err != nil {
					t.Errorf("Validate() unexpected error = %v", err)
				}
			}
		})
	}
}

// ---- PromoteComponentRequest ----

// TestPromoteComponentRequest_Validate documents that Validate is a no-op stub.
func TestPromoteComponentRequest_Validate(t *testing.T) {
	reqs := []*PromoteComponentRequest{
		{},
		{SourceEnvironment: "dev", TargetEnvironment: "staging"},
		{SourceEnvironment: "staging", TargetEnvironment: "prod"},
	}
	for _, req := range reqs {
		if err := req.Validate(); err != nil {
			t.Errorf("Validate() on %+v returned unexpected error: %v", req, err)
		}
	}
}
