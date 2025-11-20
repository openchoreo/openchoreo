// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package models

import (
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
			name:         "Valid state - Suspend",
			releaseState: ReleaseStateSuspend,
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
			errMsg:       "releaseState must be one of: Active, Suspend, Undeploy",
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

func TestPatchReleaseBindingRequest(t *testing.T) {
	tests := []struct {
		name        string
		req         *PatchReleaseBindingRequest
		description string
	}{
		{
			name: "With component type overrides",
			req: &PatchReleaseBindingRequest{
				ComponentTypeEnvOverrides: map[string]interface{}{
					"replicas": 3,
					"cpu":      "500m",
				},
			},
			description: "Should accept component type overrides",
		},
		{
			name: "With trait overrides",
			req: &PatchReleaseBindingRequest{
				TraitOverrides: map[string]map[string]interface{}{
					"ingress": {
						"host": "example.com",
					},
				},
			},
			description: "Should accept trait overrides",
		},
		{
			name: "With workload overrides",
			req: &PatchReleaseBindingRequest{
				WorkloadOverrides: &WorkloadOverrides{
					Containers: map[string]ContainerOverride{
						"main": {
							Env: []EnvVar{
								{Key: "ENV", Value: "production"},
							},
							Files: []FileVar{
								{Key: "config", MountPath: "/etc/config", Value: "data"},
							},
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
			if tt.req.ComponentTypeEnvOverrides != nil {
				if len(tt.req.ComponentTypeEnvOverrides) == 0 {
					t.Error("ComponentTypeEnvOverrides should not be empty when set")
				}
			}

			// For trait overrides
			if tt.req.TraitOverrides != nil {
				if len(tt.req.TraitOverrides) == 0 {
					t.Error("TraitOverrides should not be empty when set")
				}
			}

			// For workload overrides
			if tt.req.WorkloadOverrides != nil {
				if len(tt.req.WorkloadOverrides.Containers) == 0 {
					t.Error("WorkloadOverrides should have at least one container")
				}
			}
		})
	}
}
