// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package validation

import (
	"strings"
	"testing"

	"github.com/openchoreo/openchoreo/pkg/cli/types/api"
)

// mockNamespaceParams is a test helper satisfying the namespaceParams interface.
type mockNamespaceParams struct {
	Namespace string
}

func (m mockNamespaceParams) GetNamespace() string { return m.Namespace }

// mockDeleteProjectParams satisfies deleteProjectParams.
type mockDeleteProjectParams struct {
	Namespace   string
	ProjectName string
}

func (m mockDeleteProjectParams) GetNamespace() string   { return m.Namespace }
func (m mockDeleteProjectParams) GetProjectName() string { return m.ProjectName }

// mockDeleteComponentParams satisfies deleteComponentParams.
type mockDeleteComponentParams struct {
	Namespace     string
	ComponentName string
}

func (m mockDeleteComponentParams) GetNamespace() string     { return m.Namespace }
func (m mockDeleteComponentParams) GetComponentName() string { return m.ComponentName }

// mockDeployComponentParams satisfies deployComponentParams.
type mockDeployComponentParams struct {
	Namespace     string
	Project       string
	ComponentName string
}

func (m mockDeployComponentParams) GetNamespace() string     { return m.Namespace }
func (m mockDeployComponentParams) GetProject() string       { return m.Project }
func (m mockDeployComponentParams) GetComponentName() string { return m.ComponentName }

func TestValidateParams(t *testing.T) {
	tests := []struct {
		name     string
		cmdType  CommandType
		resource ResourceType
		params   any
		wantErr  bool
		errMsg   string
	}{
		// Project create: valid
		{
			name:     "project create valid",
			cmdType:  CmdCreate,
			resource: ResourceProject,
			params:   api.CreateProjectParams{Namespace: "ns", Name: "proj"},
			wantErr:  false,
		},
		// Project create: missing name
		{
			name:     "project create missing name",
			cmdType:  CmdCreate,
			resource: ResourceProject,
			params:   api.CreateProjectParams{Namespace: "ns", Name: ""},
			wantErr:  true,
			errMsg:   "name",
		},
		// Project get: valid namespace
		{
			name:     "project get valid",
			cmdType:  CmdGet,
			resource: ResourceProject,
			params:   mockNamespaceParams{Namespace: "ns"},
			wantErr:  false,
		},
		// Project get: empty namespace
		{
			name:     "project get missing namespace",
			cmdType:  CmdGet,
			resource: ResourceProject,
			params:   mockNamespaceParams{Namespace: ""},
			wantErr:  true,
			errMsg:   "namespace",
		},
		// Project delete: valid
		{
			name:     "project delete valid",
			cmdType:  CmdDelete,
			resource: ResourceProject,
			params:   mockDeleteProjectParams{Namespace: "ns", ProjectName: "proj"},
			wantErr:  false,
		},
		// Project delete: missing fields
		{
			name:     "project delete missing fields",
			cmdType:  CmdDelete,
			resource: ResourceProject,
			params:   mockDeleteProjectParams{Namespace: "", ProjectName: ""},
			wantErr:  true,
			errMsg:   "namespace",
		},
		// Component list: valid
		{
			name:     "component list valid",
			cmdType:  CmdList,
			resource: ResourceComponent,
			params:   mockNamespaceParams{Namespace: "ns"},
			wantErr:  false,
		},
		// Component list: missing namespace
		{
			name:     "component list missing namespace",
			cmdType:  CmdList,
			resource: ResourceComponent,
			params:   mockNamespaceParams{Namespace: ""},
			wantErr:  true,
			errMsg:   "namespace",
		},
		// Component delete: valid
		{
			name:     "component delete valid",
			cmdType:  CmdDelete,
			resource: ResourceComponent,
			params:   mockDeleteComponentParams{Namespace: "ns", ComponentName: "comp"},
			wantErr:  false,
		},
		// Component delete: missing fields
		{
			name:     "component delete missing fields",
			cmdType:  CmdDelete,
			resource: ResourceComponent,
			params:   mockDeleteComponentParams{Namespace: "", ComponentName: ""},
			wantErr:  true,
			errMsg:   "namespace",
		},
		// Component deploy: valid
		{
			name:     "component deploy valid",
			cmdType:  CmdDeploy,
			resource: ResourceComponent,
			params:   mockDeployComponentParams{Namespace: "ns", Project: "proj", ComponentName: "comp"},
			wantErr:  false,
		},
		// Component deploy: missing component name
		{
			name:     "component deploy missing component",
			cmdType:  CmdDeploy,
			resource: ResourceComponent,
			params:   mockDeployComponentParams{Namespace: "ns", Project: "proj", ComponentName: ""},
			wantErr:  true,
			errMsg:   "component name is required",
		},
		// Log params: build type valid
		{
			name:     "log params build valid",
			cmdType:  CmdLogs,
			resource: ResourceLogs,
			params:   api.LogParams{Type: "build", Namespace: "ns", Build: "build-1"},
			wantErr:  false,
		},
		// Log params: deployment type valid
		{
			name:     "log params deployment valid",
			cmdType:  CmdLogs,
			resource: ResourceLogs,
			params: api.LogParams{
				Type: "deployment", Namespace: "ns", Project: "proj",
				Component: "comp", Environment: "dev", Deployment: "dep",
			},
			wantErr: false,
		},
		// Log params: missing type
		{
			name:     "log params missing type",
			cmdType:  CmdLogs,
			resource: ResourceLogs,
			params:   api.LogParams{Type: ""},
			wantErr:  true,
			errMsg:   "type",
		},
		// Log params: invalid type
		{
			name:     "log params invalid type",
			cmdType:  CmdLogs,
			resource: ResourceLogs,
			params:   api.LogParams{Type: "unknown"},
			wantErr:  true,
			errMsg:   "not supported",
		},
		// Workload create: valid
		{
			name:     "workload create valid",
			cmdType:  CmdCreate,
			resource: ResourceWorkload,
			params: api.CreateWorkloadParams{
				NamespaceName: "ns", ProjectName: "proj",
				ComponentName: "comp", ImageURL: "image:latest",
			},
			wantErr: false,
		},
		// Workload create: missing image
		{
			name:     "workload create missing image",
			cmdType:  CmdCreate,
			resource: ResourceWorkload,
			params: api.CreateWorkloadParams{
				NamespaceName: "ns", ProjectName: "proj",
				ComponentName: "comp", ImageURL: "",
			},
			wantErr: true,
			errMsg:  "image",
		},
		// Unknown resource type
		{
			name:     "unknown resource type",
			cmdType:  CmdGet,
			resource: "foobar",
			params:   nil,
			wantErr:  true,
			errMsg:   "unknown resource type",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateParams(tt.cmdType, tt.resource, tt.params)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateParams() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err != nil && tt.errMsg != "" && !strings.Contains(err.Error(), tt.errMsg) {
				t.Errorf("error %q does not contain %q", err.Error(), tt.errMsg)
			}
		})
	}
}
