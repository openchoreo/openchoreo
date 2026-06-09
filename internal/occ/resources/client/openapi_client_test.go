// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/openchoreo/openchoreo/internal/occ/resources/client/mocks"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
)

// newMockClient returns a Client backed by the given mock.
func newMockClient(m *mocks.MockClientWithResponsesInterface) *Client {
	return &Client{client: m}
}

// httpResp returns a minimal *http.Response with the given status code.
func httpResp(code int) *http.Response {
	return &http.Response{StatusCode: code}
}

func TestApiError(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		body       []byte
		wantMsg    string
	}{
		{
			name:       "structured error with message",
			statusCode: 404,
			body:       []byte(`{"code":"NOT_FOUND","error":"component \"foo\" not found"}`),
			wantMsg:    `component "foo" not found`,
		},
		{
			name:       "structured error with details",
			statusCode: 400,
			body:       []byte(`{"code":"VALIDATION_ERROR","error":"validation failed","details":[{"field":"name","message":"must not be empty"}]}`),
			wantMsg:    `validation failed; name: must not be empty`,
		},
		{
			name:       "structured error with multiple details",
			statusCode: 400,
			body:       []byte(`{"code":"VALIDATION_ERROR","error":"validation failed","details":[{"field":"name","message":"must not be empty"},{"field":"project","message":"is required"}]}`),
			wantMsg:    `validation failed; name: must not be empty; project: is required`,
		},
		{
			name:       "non-JSON body",
			statusCode: 502,
			body:       []byte(`Bad Gateway`),
			wantMsg:    `unexpected response (HTTP 502): Bad Gateway`,
		},
		{
			name:       "empty body",
			statusCode: 500,
			body:       nil,
			wantMsg:    `unexpected response status: 500`,
		},
		{
			name:       "JSON without error field",
			statusCode: 503,
			body:       []byte(`{"status":"unavailable"}`),
			wantMsg:    `unexpected response (HTTP 503): {"status":"unavailable"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := apiError(tt.statusCode, tt.body)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if got := err.Error(); got != tt.wantMsg {
				t.Errorf("apiError() = %q, want %q", got, tt.wantMsg)
			}
		})
	}
}

func TestSchemaResponseToRaw(t *testing.T) {
	tests := []struct {
		name    string
		schema  *gen.SchemaResponse
		wantErr bool
	}{
		{
			name: "valid schema with properties",
			schema: &gen.SchemaResponse{
				"type": "object",
				"properties": map[string]interface{}{
					"port": map[string]interface{}{"type": "integer"},
				},
			},
		},
		{
			name:   "empty schema",
			schema: &gen.SchemaResponse{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			raw, err := schemaResponseToRaw(tt.schema)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, raw)

			// Verify the raw message can be unmarshalled back
			var result map[string]interface{}
			assert.NoError(t, json.Unmarshal(*raw, &result))
		})
	}
}

func TestSchemaResponseToRaw_RoundTrip(t *testing.T) {
	original := &gen.SchemaResponse{
		"type": "object",
		"properties": map[string]interface{}{
			"replicas": map[string]interface{}{"type": "integer"},
		},
	}

	raw, err := schemaResponseToRaw(original)
	require.NoError(t, err)
	require.NotNil(t, raw)

	// Verify the JSON contains expected fields
	var parsed map[string]interface{}
	require.NoError(t, json.Unmarshal(*raw, &parsed))
	assert.Equal(t, "object", parsed["type"])
	props, ok := parsed["properties"].(map[string]interface{})
	require.True(t, ok)
	assert.Contains(t, props, "replicas")
}

// --- ListNamespaces ---

func TestListNamespaces_Success(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().ListNamespacesWithResponse(mock.Anything, mock.Anything).Return(&gen.ListNamespacesResp{
		HTTPResponse: httpResp(http.StatusOK),
		JSON200: &gen.NamespaceList{
			Items:      []gen.Namespace{{Metadata: gen.ObjectMeta{Name: "org-a"}}},
			Pagination: gen.Pagination{},
		},
	}, nil)

	c := newMockClient(m)
	result, err := c.ListNamespaces(context.Background(), nil)
	require.NoError(t, err)
	require.Len(t, result.Items, 1)
	assert.Equal(t, "org-a", result.Items[0].Metadata.Name)
}

func TestListNamespaces_TransportError(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().ListNamespacesWithResponse(mock.Anything, mock.Anything).Return(nil, fmt.Errorf("connection refused"))

	c := newMockClient(m)
	_, err := c.ListNamespaces(context.Background(), nil)
	require.ErrorContains(t, err, "failed to list namespaces")
}

func TestListNamespaces_APIError(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().ListNamespacesWithResponse(mock.Anything, mock.Anything).Return(&gen.ListNamespacesResp{
		HTTPResponse: httpResp(http.StatusInternalServerError),
		Body:         []byte(`{"error":"internal error"}`),
	}, nil)

	c := newMockClient(m)
	_, err := c.ListNamespaces(context.Background(), nil)
	require.ErrorContains(t, err, "internal error")
}

// --- GetNamespace ---

func TestGetNamespace_Success(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().GetNamespaceWithResponse(mock.Anything, "org-a", mock.Anything).Return(&gen.GetNamespaceResp{
		HTTPResponse: httpResp(http.StatusOK),
		JSON200:      &gen.Namespace{Metadata: gen.ObjectMeta{Name: "org-a"}},
	}, nil)

	c := newMockClient(m)
	result, err := c.GetNamespace(context.Background(), "org-a")
	require.NoError(t, err)
	assert.Equal(t, "org-a", result.Metadata.Name)
}

func TestGetNamespace_TransportError(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().GetNamespaceWithResponse(mock.Anything, "org-a", mock.Anything).Return(nil, fmt.Errorf("timeout"))

	c := newMockClient(m)
	_, err := c.GetNamespace(context.Background(), "org-a")
	require.ErrorContains(t, err, "failed to get namespace")
}

func TestGetNamespace_APIError(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().GetNamespaceWithResponse(mock.Anything, "org-a", mock.Anything).Return(&gen.GetNamespaceResp{
		HTTPResponse: httpResp(http.StatusNotFound),
		Body:         []byte(`{"error":"namespace not found"}`),
	}, nil)

	c := newMockClient(m)
	_, err := c.GetNamespace(context.Background(), "org-a")
	require.ErrorContains(t, err, "namespace not found")
}

// --- DeleteNamespace ---

func TestDeleteNamespace_Success(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().DeleteNamespaceWithResponse(mock.Anything, "org-a", mock.Anything).Return(&gen.DeleteNamespaceResp{
		HTTPResponse: httpResp(http.StatusNoContent),
	}, nil)

	c := newMockClient(m)
	require.NoError(t, c.DeleteNamespace(context.Background(), "org-a"))
}

func TestDeleteNamespace_TransportError(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().DeleteNamespaceWithResponse(mock.Anything, "org-a", mock.Anything).Return(nil, fmt.Errorf("timeout"))

	c := newMockClient(m)
	require.ErrorContains(t, c.DeleteNamespace(context.Background(), "org-a"), "failed to delete namespace")
}

func TestDeleteNamespace_APIError(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().DeleteNamespaceWithResponse(mock.Anything, "org-a", mock.Anything).Return(&gen.DeleteNamespaceResp{
		HTTPResponse: httpResp(http.StatusForbidden),
		Body:         []byte(`{"error":"forbidden"}`),
	}, nil)

	c := newMockClient(m)
	require.ErrorContains(t, c.DeleteNamespace(context.Background(), "org-a"), "forbidden")
}

// --- ListProjects ---

func TestListProjects_Success(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().ListProjectsWithResponse(mock.Anything, "org-a", mock.Anything).Return(&gen.ListProjectsResp{
		HTTPResponse: httpResp(http.StatusOK),
		JSON200: &gen.ProjectList{
			Items:      []gen.Project{{Metadata: gen.ObjectMeta{Name: "proj-1"}}},
			Pagination: gen.Pagination{},
		},
	}, nil)

	c := newMockClient(m)
	result, err := c.ListProjects(context.Background(), "org-a", nil)
	require.NoError(t, err)
	require.Len(t, result.Items, 1)
	assert.Equal(t, "proj-1", result.Items[0].Metadata.Name)
}

func TestListProjects_TransportError(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().ListProjectsWithResponse(mock.Anything, "org-a", mock.Anything).Return(nil, fmt.Errorf("connection refused"))

	c := newMockClient(m)
	_, err := c.ListProjects(context.Background(), "org-a", nil)
	require.ErrorContains(t, err, "failed to list projects")
}

func TestListProjects_APIError(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().ListProjectsWithResponse(mock.Anything, "org-a", mock.Anything).Return(&gen.ListProjectsResp{
		HTTPResponse: httpResp(http.StatusUnauthorized),
		Body:         []byte(`{"error":"unauthorized"}`),
	}, nil)

	c := newMockClient(m)
	_, err := c.ListProjects(context.Background(), "org-a", nil)
	require.ErrorContains(t, err, "unauthorized")
}

// --- GetProject ---

func TestGetProject_Success(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().GetProjectWithResponse(mock.Anything, "org-a", "proj-1", mock.Anything).Return(&gen.GetProjectResp{
		HTTPResponse: httpResp(http.StatusOK),
		JSON200:      &gen.Project{Metadata: gen.ObjectMeta{Name: "proj-1"}},
	}, nil)

	c := newMockClient(m)
	result, err := c.GetProject(context.Background(), "org-a", "proj-1")
	require.NoError(t, err)
	assert.Equal(t, "proj-1", result.Metadata.Name)
}

func TestGetProject_TransportError(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().GetProjectWithResponse(mock.Anything, "org-a", "proj-1", mock.Anything).Return(nil, fmt.Errorf("timeout"))

	c := newMockClient(m)
	_, err := c.GetProject(context.Background(), "org-a", "proj-1")
	require.ErrorContains(t, err, "failed to get project")
}

func TestGetProject_APIError(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().GetProjectWithResponse(mock.Anything, "org-a", "proj-1", mock.Anything).Return(&gen.GetProjectResp{
		HTTPResponse: httpResp(http.StatusNotFound),
		Body:         []byte(`{"error":"project not found"}`),
	}, nil)

	c := newMockClient(m)
	_, err := c.GetProject(context.Background(), "org-a", "proj-1")
	require.ErrorContains(t, err, "project not found")
}

// --- DeleteProject ---

func TestDeleteProject_Success(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().DeleteProjectWithResponse(mock.Anything, "org-a", "proj-1", mock.Anything).Return(&gen.DeleteProjectResp{
		HTTPResponse: httpResp(http.StatusNoContent),
	}, nil)

	c := newMockClient(m)
	require.NoError(t, c.DeleteProject(context.Background(), "org-a", "proj-1"))
}

func TestDeleteProject_TransportError(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().DeleteProjectWithResponse(mock.Anything, "org-a", "proj-1", mock.Anything).Return(nil, fmt.Errorf("timeout"))

	c := newMockClient(m)
	require.ErrorContains(t, c.DeleteProject(context.Background(), "org-a", "proj-1"), "failed to delete project")
}

func TestDeleteProject_APIError(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().DeleteProjectWithResponse(mock.Anything, "org-a", "proj-1", mock.Anything).Return(&gen.DeleteProjectResp{
		HTTPResponse: httpResp(http.StatusForbidden),
		Body:         []byte(`{"error":"forbidden"}`),
	}, nil)

	c := newMockClient(m)
	require.ErrorContains(t, c.DeleteProject(context.Background(), "org-a", "proj-1"), "forbidden")
}

// --- ListComponents ---

func TestListComponents_Success(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().ListComponentsWithResponse(mock.Anything, "org-a", mock.Anything).Return(&gen.ListComponentsResp{
		HTTPResponse: httpResp(http.StatusOK),
		JSON200: &gen.ComponentList{
			Items:      []gen.Component{{Metadata: gen.ObjectMeta{Name: "comp-1"}}},
			Pagination: gen.Pagination{},
		},
	}, nil)

	c := newMockClient(m)
	result, err := c.ListComponents(context.Background(), "org-a", "", nil)
	require.NoError(t, err)
	require.Len(t, result.Items, 1)
	assert.Equal(t, "comp-1", result.Items[0].Metadata.Name)
}

func TestListComponents_TransportError(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().ListComponentsWithResponse(mock.Anything, "org-a", mock.Anything).Return(nil, fmt.Errorf("connection refused"))

	c := newMockClient(m)
	_, err := c.ListComponents(context.Background(), "org-a", "", nil)
	require.ErrorContains(t, err, "failed to list components")
}

func TestListComponents_APIError(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().ListComponentsWithResponse(mock.Anything, "org-a", mock.Anything).Return(&gen.ListComponentsResp{
		HTTPResponse: httpResp(http.StatusUnauthorized),
		Body:         []byte(`{"error":"unauthorized"}`),
	}, nil)

	c := newMockClient(m)
	_, err := c.ListComponents(context.Background(), "org-a", "", nil)
	require.ErrorContains(t, err, "unauthorized")
}

// --- GetComponent ---

func TestGetComponent_Success(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().GetComponentWithResponse(mock.Anything, "org-a", "comp-1", mock.Anything).Return(&gen.GetComponentResp{
		HTTPResponse: httpResp(http.StatusOK),
		JSON200:      &gen.Component{Metadata: gen.ObjectMeta{Name: "comp-1"}},
	}, nil)

	c := newMockClient(m)
	result, err := c.GetComponent(context.Background(), "org-a", "comp-1")
	require.NoError(t, err)
	assert.Equal(t, "comp-1", result.Metadata.Name)
}

func TestGetComponent_TransportError(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().GetComponentWithResponse(mock.Anything, "org-a", "comp-1", mock.Anything).Return(nil, fmt.Errorf("timeout"))

	c := newMockClient(m)
	_, err := c.GetComponent(context.Background(), "org-a", "comp-1")
	require.ErrorContains(t, err, "failed to get component")
}

func TestGetComponent_APIError(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().GetComponentWithResponse(mock.Anything, "org-a", "comp-1", mock.Anything).Return(&gen.GetComponentResp{
		HTTPResponse: httpResp(http.StatusNotFound),
		Body:         []byte(`{"error":"component not found"}`),
	}, nil)

	c := newMockClient(m)
	_, err := c.GetComponent(context.Background(), "org-a", "comp-1")
	require.ErrorContains(t, err, "component not found")
}

// --- DeleteComponent ---

func TestDeleteComponent_Success(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().DeleteComponentWithResponse(mock.Anything, "org-a", "comp-1", mock.Anything).Return(&gen.DeleteComponentResp{
		HTTPResponse: httpResp(http.StatusNoContent),
	}, nil)

	c := newMockClient(m)
	require.NoError(t, c.DeleteComponent(context.Background(), "org-a", "comp-1"))
}

func TestDeleteComponent_TransportError(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().DeleteComponentWithResponse(mock.Anything, "org-a", "comp-1", mock.Anything).Return(nil, fmt.Errorf("timeout"))

	c := newMockClient(m)
	require.ErrorContains(t, c.DeleteComponent(context.Background(), "org-a", "comp-1"), "failed to delete component")
}

func TestDeleteComponent_APIError(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().DeleteComponentWithResponse(mock.Anything, "org-a", "comp-1", mock.Anything).Return(&gen.DeleteComponentResp{
		HTTPResponse: httpResp(http.StatusForbidden),
		Body:         []byte(`{"error":"forbidden"}`),
	}, nil)

	c := newMockClient(m)
	require.ErrorContains(t, c.DeleteComponent(context.Background(), "org-a", "comp-1"), "forbidden")
}

// --- ListEnvironments ---

func TestListEnvironments_Success(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().ListEnvironmentsWithResponse(mock.Anything, "org-a", mock.Anything).Return(&gen.ListEnvironmentsResp{
		HTTPResponse: httpResp(http.StatusOK),
		JSON200: &gen.EnvironmentList{
			Items:      []gen.Environment{{Metadata: gen.ObjectMeta{Name: "dev"}}},
			Pagination: gen.Pagination{},
		},
	}, nil)

	c := newMockClient(m)
	result, err := c.ListEnvironments(context.Background(), "org-a", nil)
	require.NoError(t, err)
	require.Len(t, result.Items, 1)
	assert.Equal(t, "dev", result.Items[0].Metadata.Name)
}

func TestListEnvironments_TransportError(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().ListEnvironmentsWithResponse(mock.Anything, "org-a", mock.Anything).Return(nil, fmt.Errorf("connection refused"))

	c := newMockClient(m)
	_, err := c.ListEnvironments(context.Background(), "org-a", nil)
	require.ErrorContains(t, err, "failed to list environments")
}

func TestListEnvironments_APIError(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().ListEnvironmentsWithResponse(mock.Anything, "org-a", mock.Anything).Return(&gen.ListEnvironmentsResp{
		HTTPResponse: httpResp(http.StatusUnauthorized),
		Body:         []byte(`{"error":"unauthorized"}`),
	}, nil)

	c := newMockClient(m)
	_, err := c.ListEnvironments(context.Background(), "org-a", nil)
	require.ErrorContains(t, err, "unauthorized")
}

// --- GetEnvironment ---

func TestGetEnvironment_Success(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().GetEnvironmentWithResponse(mock.Anything, "org-a", "dev", mock.Anything).Return(&gen.GetEnvironmentResp{
		HTTPResponse: httpResp(http.StatusOK),
		JSON200:      &gen.Environment{Metadata: gen.ObjectMeta{Name: "dev"}},
	}, nil)

	c := newMockClient(m)
	result, err := c.GetEnvironment(context.Background(), "org-a", "dev")
	require.NoError(t, err)
	assert.Equal(t, "dev", result.Metadata.Name)
}

func TestGetEnvironment_TransportError(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().GetEnvironmentWithResponse(mock.Anything, "org-a", "dev", mock.Anything).Return(nil, fmt.Errorf("timeout"))

	c := newMockClient(m)
	_, err := c.GetEnvironment(context.Background(), "org-a", "dev")
	require.ErrorContains(t, err, "failed to get environment")
}

func TestGetEnvironment_APIError(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().GetEnvironmentWithResponse(mock.Anything, "org-a", "dev", mock.Anything).Return(&gen.GetEnvironmentResp{
		HTTPResponse: httpResp(http.StatusNotFound),
		Body:         []byte(`{"error":"environment not found"}`),
	}, nil)

	c := newMockClient(m)
	_, err := c.GetEnvironment(context.Background(), "org-a", "dev")
	require.ErrorContains(t, err, "environment not found")
}

// --- DeleteEnvironment ---

func TestDeleteEnvironment_Success(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().DeleteEnvironmentWithResponse(mock.Anything, "org-a", "dev", mock.Anything).Return(&gen.DeleteEnvironmentResp{
		HTTPResponse: httpResp(http.StatusNoContent),
	}, nil)

	c := newMockClient(m)
	require.NoError(t, c.DeleteEnvironment(context.Background(), "org-a", "dev"))
}

func TestDeleteEnvironment_TransportError(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().DeleteEnvironmentWithResponse(mock.Anything, "org-a", "dev", mock.Anything).Return(nil, fmt.Errorf("timeout"))

	c := newMockClient(m)
	require.ErrorContains(t, c.DeleteEnvironment(context.Background(), "org-a", "dev"), "failed to delete environment")
}

func TestDeleteEnvironment_APIError(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().DeleteEnvironmentWithResponse(mock.Anything, "org-a", "dev", mock.Anything).Return(&gen.DeleteEnvironmentResp{
		HTTPResponse: httpResp(http.StatusForbidden),
		Body:         []byte(`{"error":"forbidden"}`),
	}, nil)

	c := newMockClient(m)
	require.ErrorContains(t, c.DeleteEnvironment(context.Background(), "org-a", "dev"), "forbidden")
}

// --- ListDataPlanes ---

func TestListDataPlanes_Success(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().ListDataPlanesWithResponse(mock.Anything, "org-a", mock.Anything).Return(&gen.ListDataPlanesResp{
		HTTPResponse: httpResp(http.StatusOK),
		JSON200: &gen.DataPlaneList{
			Items:      []gen.DataPlane{{Metadata: gen.ObjectMeta{Name: "dp-1"}}},
			Pagination: gen.Pagination{},
		},
	}, nil)

	c := newMockClient(m)
	result, err := c.ListDataPlanes(context.Background(), "org-a", nil)
	require.NoError(t, err)
	require.Len(t, result.Items, 1)
	assert.Equal(t, "dp-1", result.Items[0].Metadata.Name)
}

func TestListDataPlanes_TransportError(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().ListDataPlanesWithResponse(mock.Anything, "org-a", mock.Anything).Return(nil, fmt.Errorf("connection refused"))

	c := newMockClient(m)
	_, err := c.ListDataPlanes(context.Background(), "org-a", nil)
	require.ErrorContains(t, err, "failed to list data planes")
}

func TestListDataPlanes_APIError(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().ListDataPlanesWithResponse(mock.Anything, "org-a", mock.Anything).Return(&gen.ListDataPlanesResp{
		HTTPResponse: httpResp(http.StatusUnauthorized),
		Body:         []byte(`{"error":"unauthorized"}`),
	}, nil)

	c := newMockClient(m)
	_, err := c.ListDataPlanes(context.Background(), "org-a", nil)
	require.ErrorContains(t, err, "unauthorized")
}

// --- GetDataPlane ---

func TestGetDataPlane_Success(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().GetDataPlaneWithResponse(mock.Anything, "org-a", "dp-1", mock.Anything).Return(&gen.GetDataPlaneResp{
		HTTPResponse: httpResp(http.StatusOK),
		JSON200:      &gen.DataPlane{Metadata: gen.ObjectMeta{Name: "dp-1"}},
	}, nil)

	c := newMockClient(m)
	result, err := c.GetDataPlane(context.Background(), "org-a", "dp-1")
	require.NoError(t, err)
	assert.Equal(t, "dp-1", result.Metadata.Name)
}

func TestGetDataPlane_TransportError(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().GetDataPlaneWithResponse(mock.Anything, "org-a", "dp-1", mock.Anything).Return(nil, fmt.Errorf("timeout"))

	c := newMockClient(m)
	_, err := c.GetDataPlane(context.Background(), "org-a", "dp-1")
	require.ErrorContains(t, err, "failed to get data plane")
}

func TestGetDataPlane_APIError(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().GetDataPlaneWithResponse(mock.Anything, "org-a", "dp-1", mock.Anything).Return(&gen.GetDataPlaneResp{
		HTTPResponse: httpResp(http.StatusNotFound),
		Body:         []byte(`{"error":"data plane not found"}`),
	}, nil)

	c := newMockClient(m)
	_, err := c.GetDataPlane(context.Background(), "org-a", "dp-1")
	require.ErrorContains(t, err, "data plane not found")
}

// --- DeleteDataPlane ---

func TestDeleteDataPlane_Success(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().DeleteDataPlaneWithResponse(mock.Anything, "org-a", "dp-1", mock.Anything).Return(&gen.DeleteDataPlaneResp{
		HTTPResponse: httpResp(http.StatusNoContent),
	}, nil)

	c := newMockClient(m)
	require.NoError(t, c.DeleteDataPlane(context.Background(), "org-a", "dp-1"))
}

func TestDeleteDataPlane_TransportError(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().DeleteDataPlaneWithResponse(mock.Anything, "org-a", "dp-1", mock.Anything).Return(nil, fmt.Errorf("timeout"))

	c := newMockClient(m)
	require.ErrorContains(t, c.DeleteDataPlane(context.Background(), "org-a", "dp-1"), "failed to delete data plane")
}

func TestDeleteDataPlane_APIError(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().DeleteDataPlaneWithResponse(mock.Anything, "org-a", "dp-1", mock.Anything).Return(&gen.DeleteDataPlaneResp{
		HTTPResponse: httpResp(http.StatusForbidden),
		Body:         []byte(`{"error":"forbidden"}`),
	}, nil)

	c := newMockClient(m)
	require.ErrorContains(t, c.DeleteDataPlane(context.Background(), "org-a", "dp-1"), "forbidden")
}

// --- ListWorkflowPlanes ---

func TestListWorkflowPlanes_Success(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().ListWorkflowPlanesWithResponse(mock.Anything, "org-a", mock.Anything).Return(&gen.ListWorkflowPlanesResp{
		HTTPResponse: httpResp(http.StatusOK),
		JSON200: &gen.WorkflowPlaneList{
			Items:      []gen.WorkflowPlane{{Metadata: gen.ObjectMeta{Name: "wp-1"}}},
			Pagination: gen.Pagination{},
		},
	}, nil)

	c := newMockClient(m)
	result, err := c.ListWorkflowPlanes(context.Background(), "org-a", nil)
	require.NoError(t, err)
	require.Len(t, result.Items, 1)
	assert.Equal(t, "wp-1", result.Items[0].Metadata.Name)
}

func TestListWorkflowPlanes_TransportError(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().ListWorkflowPlanesWithResponse(mock.Anything, "org-a", mock.Anything).Return(nil, fmt.Errorf("connection refused"))

	c := newMockClient(m)
	_, err := c.ListWorkflowPlanes(context.Background(), "org-a", nil)
	require.ErrorContains(t, err, "failed to list workflow planes")
}

func TestListWorkflowPlanes_APIError(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().ListWorkflowPlanesWithResponse(mock.Anything, "org-a", mock.Anything).Return(&gen.ListWorkflowPlanesResp{
		HTTPResponse: httpResp(http.StatusUnauthorized),
		Body:         []byte(`{"error":"unauthorized"}`),
	}, nil)

	c := newMockClient(m)
	_, err := c.ListWorkflowPlanes(context.Background(), "org-a", nil)
	require.ErrorContains(t, err, "unauthorized")
}

// --- GetWorkflowPlane ---

func TestGetWorkflowPlane_Success(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().GetWorkflowPlaneWithResponse(mock.Anything, "org-a", "wp-1", mock.Anything).Return(&gen.GetWorkflowPlaneResp{
		HTTPResponse: httpResp(http.StatusOK),
		JSON200:      &gen.WorkflowPlane{Metadata: gen.ObjectMeta{Name: "wp-1"}},
	}, nil)

	c := newMockClient(m)
	result, err := c.GetWorkflowPlane(context.Background(), "org-a", "wp-1")
	require.NoError(t, err)
	assert.Equal(t, "wp-1", result.Metadata.Name)
}

func TestGetWorkflowPlane_TransportError(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().GetWorkflowPlaneWithResponse(mock.Anything, "org-a", "wp-1", mock.Anything).Return(nil, fmt.Errorf("timeout"))

	c := newMockClient(m)
	_, err := c.GetWorkflowPlane(context.Background(), "org-a", "wp-1")
	require.ErrorContains(t, err, "failed to get workflow plane")
}

func TestGetWorkflowPlane_APIError(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().GetWorkflowPlaneWithResponse(mock.Anything, "org-a", "wp-1", mock.Anything).Return(&gen.GetWorkflowPlaneResp{
		HTTPResponse: httpResp(http.StatusNotFound),
		Body:         []byte(`{"error":"workflow plane not found"}`),
	}, nil)

	c := newMockClient(m)
	_, err := c.GetWorkflowPlane(context.Background(), "org-a", "wp-1")
	require.ErrorContains(t, err, "workflow plane not found")
}

// --- DeleteWorkflowPlane ---

func TestDeleteWorkflowPlane_Success(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().DeleteWorkflowPlaneWithResponse(mock.Anything, "org-a", "wp-1", mock.Anything).Return(&gen.DeleteWorkflowPlaneResp{
		HTTPResponse: httpResp(http.StatusNoContent),
	}, nil)

	c := newMockClient(m)
	require.NoError(t, c.DeleteWorkflowPlane(context.Background(), "org-a", "wp-1"))
}

func TestDeleteWorkflowPlane_TransportError(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().DeleteWorkflowPlaneWithResponse(mock.Anything, "org-a", "wp-1", mock.Anything).Return(nil, fmt.Errorf("timeout"))

	c := newMockClient(m)
	require.ErrorContains(t, c.DeleteWorkflowPlane(context.Background(), "org-a", "wp-1"), "failed to delete workflow plane")
}

func TestDeleteWorkflowPlane_APIError(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().DeleteWorkflowPlaneWithResponse(mock.Anything, "org-a", "wp-1", mock.Anything).Return(&gen.DeleteWorkflowPlaneResp{
		HTTPResponse: httpResp(http.StatusForbidden),
		Body:         []byte(`{"error":"forbidden"}`),
	}, nil)

	c := newMockClient(m)
	require.ErrorContains(t, c.DeleteWorkflowPlane(context.Background(), "org-a", "wp-1"), "forbidden")
}

// --- ListObservabilityPlanes ---

func TestListObservabilityPlanes_Success(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().ListObservabilityPlanesWithResponse(mock.Anything, "org-a", mock.Anything).Return(&gen.ListObservabilityPlanesResp{
		HTTPResponse: httpResp(http.StatusOK),
		JSON200: &gen.ObservabilityPlaneList{
			Items:      []gen.ObservabilityPlane{{Metadata: gen.ObjectMeta{Name: "obs-1"}}},
			Pagination: gen.Pagination{},
		},
	}, nil)

	c := newMockClient(m)
	result, err := c.ListObservabilityPlanes(context.Background(), "org-a", nil)
	require.NoError(t, err)
	require.Len(t, result.Items, 1)
	assert.Equal(t, "obs-1", result.Items[0].Metadata.Name)
}

func TestListObservabilityPlanes_TransportError(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().ListObservabilityPlanesWithResponse(mock.Anything, "org-a", mock.Anything).Return(nil, fmt.Errorf("connection refused"))

	c := newMockClient(m)
	_, err := c.ListObservabilityPlanes(context.Background(), "org-a", nil)
	require.ErrorContains(t, err, "failed to list observability planes")
}

func TestListObservabilityPlanes_APIError(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().ListObservabilityPlanesWithResponse(mock.Anything, "org-a", mock.Anything).Return(&gen.ListObservabilityPlanesResp{
		HTTPResponse: httpResp(http.StatusUnauthorized),
		Body:         []byte(`{"error":"unauthorized"}`),
	}, nil)

	c := newMockClient(m)
	_, err := c.ListObservabilityPlanes(context.Background(), "org-a", nil)
	require.ErrorContains(t, err, "unauthorized")
}

// --- GetObservabilityPlane ---

func TestGetObservabilityPlane_Success(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().GetObservabilityPlaneWithResponse(mock.Anything, "org-a", "obs-1", mock.Anything).Return(&gen.GetObservabilityPlaneResp{
		HTTPResponse: httpResp(http.StatusOK),
		JSON200:      &gen.ObservabilityPlane{Metadata: gen.ObjectMeta{Name: "obs-1"}},
	}, nil)

	c := newMockClient(m)
	result, err := c.GetObservabilityPlane(context.Background(), "org-a", "obs-1")
	require.NoError(t, err)
	assert.Equal(t, "obs-1", result.Metadata.Name)
}

func TestGetObservabilityPlane_TransportError(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().GetObservabilityPlaneWithResponse(mock.Anything, "org-a", "obs-1", mock.Anything).Return(nil, fmt.Errorf("timeout"))

	c := newMockClient(m)
	_, err := c.GetObservabilityPlane(context.Background(), "org-a", "obs-1")
	require.ErrorContains(t, err, "failed to get observability plane")
}

func TestGetObservabilityPlane_APIError(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().GetObservabilityPlaneWithResponse(mock.Anything, "org-a", "obs-1", mock.Anything).Return(&gen.GetObservabilityPlaneResp{
		HTTPResponse: httpResp(http.StatusNotFound),
		Body:         []byte(`{"error":"observability plane not found"}`),
	}, nil)

	c := newMockClient(m)
	_, err := c.GetObservabilityPlane(context.Background(), "org-a", "obs-1")
	require.ErrorContains(t, err, "observability plane not found")
}

// --- DeleteObservabilityPlane ---

func TestDeleteObservabilityPlane_Success(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().DeleteObservabilityPlaneWithResponse(mock.Anything, "org-a", "obs-1", mock.Anything).Return(&gen.DeleteObservabilityPlaneResp{
		HTTPResponse: httpResp(http.StatusNoContent),
	}, nil)

	c := newMockClient(m)
	require.NoError(t, c.DeleteObservabilityPlane(context.Background(), "org-a", "obs-1"))
}

func TestDeleteObservabilityPlane_TransportError(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().DeleteObservabilityPlaneWithResponse(mock.Anything, "org-a", "obs-1", mock.Anything).Return(nil, fmt.Errorf("timeout"))

	c := newMockClient(m)
	require.ErrorContains(t, c.DeleteObservabilityPlane(context.Background(), "org-a", "obs-1"), "failed to delete observability plane")
}

func TestDeleteObservabilityPlane_APIError(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().DeleteObservabilityPlaneWithResponse(mock.Anything, "org-a", "obs-1", mock.Anything).Return(&gen.DeleteObservabilityPlaneResp{
		HTTPResponse: httpResp(http.StatusForbidden),
		Body:         []byte(`{"error":"forbidden"}`),
	}, nil)

	c := newMockClient(m)
	require.ErrorContains(t, c.DeleteObservabilityPlane(context.Background(), "org-a", "obs-1"), "forbidden")
}

// --- ListComponentTypes ---

func TestListComponentTypes_Success(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().ListComponentTypesWithResponse(mock.Anything, "org-a", mock.Anything).Return(&gen.ListComponentTypesResp{
		HTTPResponse: httpResp(http.StatusOK),
		JSON200: &gen.ComponentTypeList{
			Items:      []gen.ComponentType{{Metadata: gen.ObjectMeta{Name: "ct-1"}}},
			Pagination: gen.Pagination{},
		},
	}, nil)

	c := newMockClient(m)
	result, err := c.ListComponentTypes(context.Background(), "org-a", nil)
	require.NoError(t, err)
	require.Len(t, result.Items, 1)
	assert.Equal(t, "ct-1", result.Items[0].Metadata.Name)
}

func TestListComponentTypes_TransportError(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().ListComponentTypesWithResponse(mock.Anything, "org-a", mock.Anything).Return(nil, fmt.Errorf("connection refused"))

	c := newMockClient(m)
	_, err := c.ListComponentTypes(context.Background(), "org-a", nil)
	require.ErrorContains(t, err, "failed to list component types")
}

func TestListComponentTypes_APIError(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().ListComponentTypesWithResponse(mock.Anything, "org-a", mock.Anything).Return(&gen.ListComponentTypesResp{
		HTTPResponse: httpResp(http.StatusUnauthorized),
		Body:         []byte(`{"error":"unauthorized"}`),
	}, nil)

	c := newMockClient(m)
	_, err := c.ListComponentTypes(context.Background(), "org-a", nil)
	require.ErrorContains(t, err, "unauthorized")
}

// --- GetComponentType ---

func TestGetComponentType_Success(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().GetComponentTypeWithResponse(mock.Anything, "org-a", "ct-1", mock.Anything).Return(&gen.GetComponentTypeResp{
		HTTPResponse: httpResp(http.StatusOK),
		JSON200:      &gen.ComponentType{Metadata: gen.ObjectMeta{Name: "ct-1"}},
	}, nil)

	c := newMockClient(m)
	result, err := c.GetComponentType(context.Background(), "org-a", "ct-1")
	require.NoError(t, err)
	assert.Equal(t, "ct-1", result.Metadata.Name)
}

func TestGetComponentType_TransportError(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().GetComponentTypeWithResponse(mock.Anything, "org-a", "ct-1", mock.Anything).Return(nil, fmt.Errorf("timeout"))

	c := newMockClient(m)
	_, err := c.GetComponentType(context.Background(), "org-a", "ct-1")
	require.ErrorContains(t, err, "failed to get component type")
}

func TestGetComponentType_APIError(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().GetComponentTypeWithResponse(mock.Anything, "org-a", "ct-1", mock.Anything).Return(&gen.GetComponentTypeResp{
		HTTPResponse: httpResp(http.StatusNotFound),
		Body:         []byte(`{"error":"component type not found"}`),
	}, nil)

	c := newMockClient(m)
	_, err := c.GetComponentType(context.Background(), "org-a", "ct-1")
	require.ErrorContains(t, err, "component type not found")
}

// --- DeleteComponentType ---

func TestDeleteComponentType_Success(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().DeleteComponentTypeWithResponse(mock.Anything, "org-a", "ct-1", mock.Anything).Return(&gen.DeleteComponentTypeResp{
		HTTPResponse: httpResp(http.StatusNoContent),
	}, nil)

	c := newMockClient(m)
	require.NoError(t, c.DeleteComponentType(context.Background(), "org-a", "ct-1"))
}

func TestDeleteComponentType_TransportError(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().DeleteComponentTypeWithResponse(mock.Anything, "org-a", "ct-1", mock.Anything).Return(nil, fmt.Errorf("timeout"))

	c := newMockClient(m)
	require.ErrorContains(t, c.DeleteComponentType(context.Background(), "org-a", "ct-1"), "failed to delete component type")
}

func TestDeleteComponentType_APIError(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().DeleteComponentTypeWithResponse(mock.Anything, "org-a", "ct-1", mock.Anything).Return(&gen.DeleteComponentTypeResp{
		HTTPResponse: httpResp(http.StatusForbidden),
		Body:         []byte(`{"error":"forbidden"}`),
	}, nil)

	c := newMockClient(m)
	require.ErrorContains(t, c.DeleteComponentType(context.Background(), "org-a", "ct-1"), "forbidden")
}

// --- ListClusterComponentTypes ---

func TestListClusterComponentTypes_Success(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().ListClusterComponentTypesWithResponse(mock.Anything, mock.Anything).Return(&gen.ListClusterComponentTypesResp{
		HTTPResponse: httpResp(http.StatusOK),
		JSON200: &gen.ClusterComponentTypeList{
			Items:      []gen.ClusterComponentType{{Metadata: gen.ObjectMeta{Name: "cct-1"}}},
			Pagination: gen.Pagination{},
		},
	}, nil)

	c := newMockClient(m)
	result, err := c.ListClusterComponentTypes(context.Background(), nil)
	require.NoError(t, err)
	require.Len(t, result.Items, 1)
	assert.Equal(t, "cct-1", result.Items[0].Metadata.Name)
}

func TestListClusterComponentTypes_TransportError(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().ListClusterComponentTypesWithResponse(mock.Anything, mock.Anything).Return(nil, fmt.Errorf("connection refused"))

	c := newMockClient(m)
	_, err := c.ListClusterComponentTypes(context.Background(), nil)
	require.ErrorContains(t, err, "failed to list cluster component types")
}

func TestListClusterComponentTypes_APIError(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().ListClusterComponentTypesWithResponse(mock.Anything, mock.Anything).Return(&gen.ListClusterComponentTypesResp{
		HTTPResponse: httpResp(http.StatusUnauthorized),
		Body:         []byte(`{"error":"unauthorized"}`),
	}, nil)

	c := newMockClient(m)
	_, err := c.ListClusterComponentTypes(context.Background(), nil)
	require.ErrorContains(t, err, "unauthorized")
}

// --- GetClusterComponentType ---

func TestGetClusterComponentType_Success(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().GetClusterComponentTypeWithResponse(mock.Anything, "cct-1", mock.Anything).Return(&gen.GetClusterComponentTypeResp{
		HTTPResponse: httpResp(http.StatusOK),
		JSON200:      &gen.ClusterComponentType{Metadata: gen.ObjectMeta{Name: "cct-1"}},
	}, nil)

	c := newMockClient(m)
	result, err := c.GetClusterComponentType(context.Background(), "cct-1")
	require.NoError(t, err)
	assert.Equal(t, "cct-1", result.Metadata.Name)
}

func TestGetClusterComponentType_TransportError(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().GetClusterComponentTypeWithResponse(mock.Anything, "cct-1", mock.Anything).Return(nil, fmt.Errorf("timeout"))

	c := newMockClient(m)
	_, err := c.GetClusterComponentType(context.Background(), "cct-1")
	require.ErrorContains(t, err, "failed to get cluster component type")
}

func TestGetClusterComponentType_APIError(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().GetClusterComponentTypeWithResponse(mock.Anything, "cct-1", mock.Anything).Return(&gen.GetClusterComponentTypeResp{
		HTTPResponse: httpResp(http.StatusNotFound),
		Body:         []byte(`{"error":"cluster component type not found"}`),
	}, nil)

	c := newMockClient(m)
	_, err := c.GetClusterComponentType(context.Background(), "cct-1")
	require.ErrorContains(t, err, "cluster component type not found")
}

// --- DeleteClusterComponentType ---

func TestDeleteClusterComponentType_Success(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().DeleteClusterComponentTypeWithResponse(mock.Anything, "cct-1", mock.Anything).Return(&gen.DeleteClusterComponentTypeResp{
		HTTPResponse: httpResp(http.StatusNoContent),
	}, nil)

	c := newMockClient(m)
	require.NoError(t, c.DeleteClusterComponentType(context.Background(), "cct-1"))
}

func TestDeleteClusterComponentType_TransportError(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().DeleteClusterComponentTypeWithResponse(mock.Anything, "cct-1", mock.Anything).Return(nil, fmt.Errorf("timeout"))

	c := newMockClient(m)
	require.ErrorContains(t, c.DeleteClusterComponentType(context.Background(), "cct-1"), "failed to delete cluster component type")
}

func TestDeleteClusterComponentType_APIError(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().DeleteClusterComponentTypeWithResponse(mock.Anything, "cct-1", mock.Anything).Return(&gen.DeleteClusterComponentTypeResp{
		HTTPResponse: httpResp(http.StatusForbidden),
		Body:         []byte(`{"error":"forbidden"}`),
	}, nil)

	c := newMockClient(m)
	require.ErrorContains(t, c.DeleteClusterComponentType(context.Background(), "cct-1"), "forbidden")
}

// --- ListClusterTraits ---

func TestListClusterTraits_Success(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().ListClusterTraitsWithResponse(mock.Anything, mock.Anything).Return(&gen.ListClusterTraitsResp{
		HTTPResponse: httpResp(http.StatusOK),
		JSON200: &gen.ClusterTraitList{
			Items:      []gen.ClusterTrait{{Metadata: gen.ObjectMeta{Name: "ct-1"}}},
			Pagination: gen.Pagination{},
		},
	}, nil)

	c := newMockClient(m)
	result, err := c.ListClusterTraits(context.Background(), nil)
	require.NoError(t, err)
	require.Len(t, result.Items, 1)
	assert.Equal(t, "ct-1", result.Items[0].Metadata.Name)
}

func TestListClusterTraits_TransportError(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().ListClusterTraitsWithResponse(mock.Anything, mock.Anything).Return(nil, fmt.Errorf("connection refused"))

	c := newMockClient(m)
	_, err := c.ListClusterTraits(context.Background(), nil)
	require.ErrorContains(t, err, "failed to list cluster traits")
}

func TestListClusterTraits_APIError(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().ListClusterTraitsWithResponse(mock.Anything, mock.Anything).Return(&gen.ListClusterTraitsResp{
		HTTPResponse: httpResp(http.StatusUnauthorized),
		Body:         []byte(`{"error":"unauthorized"}`),
	}, nil)

	c := newMockClient(m)
	_, err := c.ListClusterTraits(context.Background(), nil)
	require.ErrorContains(t, err, "unauthorized")
}

// --- GetClusterTrait ---

func TestGetClusterTrait_Success(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().GetClusterTraitWithResponse(mock.Anything, "ct-1", mock.Anything).Return(&gen.GetClusterTraitResp{
		HTTPResponse: httpResp(http.StatusOK),
		JSON200:      &gen.ClusterTrait{Metadata: gen.ObjectMeta{Name: "ct-1"}},
	}, nil)

	c := newMockClient(m)
	result, err := c.GetClusterTrait(context.Background(), "ct-1")
	require.NoError(t, err)
	assert.Equal(t, "ct-1", result.Metadata.Name)
}

func TestGetClusterTrait_TransportError(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().GetClusterTraitWithResponse(mock.Anything, "ct-1", mock.Anything).Return(nil, fmt.Errorf("timeout"))

	c := newMockClient(m)
	_, err := c.GetClusterTrait(context.Background(), "ct-1")
	require.ErrorContains(t, err, "failed to get cluster trait")
}

func TestGetClusterTrait_APIError(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().GetClusterTraitWithResponse(mock.Anything, "ct-1", mock.Anything).Return(&gen.GetClusterTraitResp{
		HTTPResponse: httpResp(http.StatusNotFound),
		Body:         []byte(`{"error":"cluster trait not found"}`),
	}, nil)

	c := newMockClient(m)
	_, err := c.GetClusterTrait(context.Background(), "ct-1")
	require.ErrorContains(t, err, "cluster trait not found")
}

// --- DeleteClusterTrait ---

func TestDeleteClusterTrait_Success(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().DeleteClusterTraitWithResponse(mock.Anything, "ct-1", mock.Anything).Return(&gen.DeleteClusterTraitResp{
		HTTPResponse: httpResp(http.StatusNoContent),
	}, nil)

	c := newMockClient(m)
	require.NoError(t, c.DeleteClusterTrait(context.Background(), "ct-1"))
}

func TestDeleteClusterTrait_TransportError(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().DeleteClusterTraitWithResponse(mock.Anything, "ct-1", mock.Anything).Return(nil, fmt.Errorf("timeout"))

	c := newMockClient(m)
	require.ErrorContains(t, c.DeleteClusterTrait(context.Background(), "ct-1"), "failed to delete cluster trait")
}

func TestDeleteClusterTrait_APIError(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().DeleteClusterTraitWithResponse(mock.Anything, "ct-1", mock.Anything).Return(&gen.DeleteClusterTraitResp{
		HTTPResponse: httpResp(http.StatusForbidden),
		Body:         []byte(`{"error":"forbidden"}`),
	}, nil)

	c := newMockClient(m)
	require.ErrorContains(t, c.DeleteClusterTrait(context.Background(), "ct-1"), "forbidden")
}

// --- ListTraits ---

func TestListTraits_Success(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().ListTraitsWithResponse(mock.Anything, "org-a", mock.Anything).Return(&gen.ListTraitsResp{
		HTTPResponse: httpResp(http.StatusOK),
		JSON200: &gen.TraitList{
			Items:      []gen.Trait{{Metadata: gen.ObjectMeta{Name: "trait-1"}}},
			Pagination: gen.Pagination{},
		},
	}, nil)

	c := newMockClient(m)
	result, err := c.ListTraits(context.Background(), "org-a", nil)
	require.NoError(t, err)
	require.Len(t, result.Items, 1)
	assert.Equal(t, "trait-1", result.Items[0].Metadata.Name)
}

func TestListTraits_TransportError(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().ListTraitsWithResponse(mock.Anything, "org-a", mock.Anything).Return(nil, fmt.Errorf("connection refused"))

	c := newMockClient(m)
	_, err := c.ListTraits(context.Background(), "org-a", nil)
	require.ErrorContains(t, err, "failed to list traits")
}

func TestListTraits_APIError(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().ListTraitsWithResponse(mock.Anything, "org-a", mock.Anything).Return(&gen.ListTraitsResp{
		HTTPResponse: httpResp(http.StatusUnauthorized),
		Body:         []byte(`{"error":"unauthorized"}`),
	}, nil)

	c := newMockClient(m)
	_, err := c.ListTraits(context.Background(), "org-a", nil)
	require.ErrorContains(t, err, "unauthorized")
}

// --- GetTrait ---

func TestGetTrait_Success(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().GetTraitWithResponse(mock.Anything, "org-a", "trait-1", mock.Anything).Return(&gen.GetTraitResp{
		HTTPResponse: httpResp(http.StatusOK),
		JSON200:      &gen.Trait{Metadata: gen.ObjectMeta{Name: "trait-1"}},
	}, nil)

	c := newMockClient(m)
	result, err := c.GetTrait(context.Background(), "org-a", "trait-1")
	require.NoError(t, err)
	assert.Equal(t, "trait-1", result.Metadata.Name)
}

func TestGetTrait_TransportError(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().GetTraitWithResponse(mock.Anything, "org-a", "trait-1", mock.Anything).Return(nil, fmt.Errorf("timeout"))

	c := newMockClient(m)
	_, err := c.GetTrait(context.Background(), "org-a", "trait-1")
	require.ErrorContains(t, err, "failed to get trait")
}

func TestGetTrait_APIError(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().GetTraitWithResponse(mock.Anything, "org-a", "trait-1", mock.Anything).Return(&gen.GetTraitResp{
		HTTPResponse: httpResp(http.StatusNotFound),
		Body:         []byte(`{"error":"trait not found"}`),
	}, nil)

	c := newMockClient(m)
	_, err := c.GetTrait(context.Background(), "org-a", "trait-1")
	require.ErrorContains(t, err, "trait not found")
}

// --- DeleteTrait ---

func TestDeleteTrait_Success(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().DeleteTraitWithResponse(mock.Anything, "org-a", "trait-1", mock.Anything).Return(&gen.DeleteTraitResp{
		HTTPResponse: httpResp(http.StatusNoContent),
	}, nil)

	c := newMockClient(m)
	require.NoError(t, c.DeleteTrait(context.Background(), "org-a", "trait-1"))
}

func TestDeleteTrait_TransportError(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().DeleteTraitWithResponse(mock.Anything, "org-a", "trait-1", mock.Anything).Return(nil, fmt.Errorf("timeout"))

	c := newMockClient(m)
	require.ErrorContains(t, c.DeleteTrait(context.Background(), "org-a", "trait-1"), "failed to delete trait")
}

func TestDeleteTrait_APIError(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().DeleteTraitWithResponse(mock.Anything, "org-a", "trait-1", mock.Anything).Return(&gen.DeleteTraitResp{
		HTTPResponse: httpResp(http.StatusForbidden),
		Body:         []byte(`{"error":"forbidden"}`),
	}, nil)

	c := newMockClient(m)
	require.ErrorContains(t, c.DeleteTrait(context.Background(), "org-a", "trait-1"), "forbidden")
}

// --- ListWorkflows ---

func TestListWorkflows_Success(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().ListWorkflowsWithResponse(mock.Anything, "org-a", mock.Anything).Return(&gen.ListWorkflowsResp{
		HTTPResponse: httpResp(http.StatusOK),
		JSON200: &gen.WorkflowList{
			Items:      []gen.Workflow{{Metadata: gen.ObjectMeta{Name: "wf-1"}}},
			Pagination: gen.Pagination{},
		},
	}, nil)

	c := newMockClient(m)
	result, err := c.ListWorkflows(context.Background(), "org-a", nil)
	require.NoError(t, err)
	require.Len(t, result.Items, 1)
	assert.Equal(t, "wf-1", result.Items[0].Metadata.Name)
}

func TestListWorkflows_TransportError(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().ListWorkflowsWithResponse(mock.Anything, "org-a", mock.Anything).Return(nil, fmt.Errorf("connection refused"))

	c := newMockClient(m)
	_, err := c.ListWorkflows(context.Background(), "org-a", nil)
	require.ErrorContains(t, err, "failed to list workflows")
}

func TestListWorkflows_APIError(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().ListWorkflowsWithResponse(mock.Anything, "org-a", mock.Anything).Return(&gen.ListWorkflowsResp{
		HTTPResponse: httpResp(http.StatusUnauthorized),
		Body:         []byte(`{"error":"unauthorized"}`),
	}, nil)

	c := newMockClient(m)
	_, err := c.ListWorkflows(context.Background(), "org-a", nil)
	require.ErrorContains(t, err, "unauthorized")
}

// --- GetWorkflow ---

func TestGetWorkflow_Success(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().GetWorkflowWithResponse(mock.Anything, "org-a", "wf-1", mock.Anything).Return(&gen.GetWorkflowResp{
		HTTPResponse: httpResp(http.StatusOK),
		JSON200:      &gen.Workflow{Metadata: gen.ObjectMeta{Name: "wf-1"}},
	}, nil)

	c := newMockClient(m)
	result, err := c.GetWorkflow(context.Background(), "org-a", "wf-1")
	require.NoError(t, err)
	assert.Equal(t, "wf-1", result.Metadata.Name)
}

func TestGetWorkflow_TransportError(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().GetWorkflowWithResponse(mock.Anything, "org-a", "wf-1", mock.Anything).Return(nil, fmt.Errorf("timeout"))

	c := newMockClient(m)
	_, err := c.GetWorkflow(context.Background(), "org-a", "wf-1")
	require.ErrorContains(t, err, "failed to get workflow")
}

func TestGetWorkflow_APIError(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().GetWorkflowWithResponse(mock.Anything, "org-a", "wf-1", mock.Anything).Return(&gen.GetWorkflowResp{
		HTTPResponse: httpResp(http.StatusNotFound),
		Body:         []byte(`{"error":"workflow not found"}`),
	}, nil)

	c := newMockClient(m)
	_, err := c.GetWorkflow(context.Background(), "org-a", "wf-1")
	require.ErrorContains(t, err, "workflow not found")
}

// --- DeleteWorkflow ---

func TestDeleteWorkflow_Success(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().DeleteWorkflowWithResponse(mock.Anything, "org-a", "wf-1", mock.Anything).Return(&gen.DeleteWorkflowResp{
		HTTPResponse: httpResp(http.StatusNoContent),
	}, nil)

	c := newMockClient(m)
	require.NoError(t, c.DeleteWorkflow(context.Background(), "org-a", "wf-1"))
}

func TestDeleteWorkflow_TransportError(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().DeleteWorkflowWithResponse(mock.Anything, "org-a", "wf-1", mock.Anything).Return(nil, fmt.Errorf("timeout"))

	c := newMockClient(m)
	require.ErrorContains(t, c.DeleteWorkflow(context.Background(), "org-a", "wf-1"), "failed to delete workflow")
}

func TestDeleteWorkflow_APIError(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().DeleteWorkflowWithResponse(mock.Anything, "org-a", "wf-1", mock.Anything).Return(&gen.DeleteWorkflowResp{
		HTTPResponse: httpResp(http.StatusForbidden),
		Body:         []byte(`{"error":"forbidden"}`),
	}, nil)

	c := newMockClient(m)
	require.ErrorContains(t, c.DeleteWorkflow(context.Background(), "org-a", "wf-1"), "forbidden")
}

// --- ListSecretReferences ---

func TestListSecretReferences_Success(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().ListSecretReferencesWithResponse(mock.Anything, "org-a", mock.Anything).Return(&gen.ListSecretReferencesResp{
		HTTPResponse: httpResp(http.StatusOK),
		JSON200: &gen.SecretReferenceList{
			Items:      []gen.SecretReference{{Metadata: gen.ObjectMeta{Name: "sr-1"}}},
			Pagination: gen.Pagination{},
		},
	}, nil)

	c := newMockClient(m)
	result, err := c.ListSecretReferences(context.Background(), "org-a", nil)
	require.NoError(t, err)
	require.Len(t, result.Items, 1)
	assert.Equal(t, "sr-1", result.Items[0].Metadata.Name)
}

func TestListSecretReferences_TransportError(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().ListSecretReferencesWithResponse(mock.Anything, "org-a", mock.Anything).Return(nil, fmt.Errorf("connection refused"))

	c := newMockClient(m)
	_, err := c.ListSecretReferences(context.Background(), "org-a", nil)
	require.ErrorContains(t, err, "failed to list secret references")
}

func TestListSecretReferences_APIError(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().ListSecretReferencesWithResponse(mock.Anything, "org-a", mock.Anything).Return(&gen.ListSecretReferencesResp{
		HTTPResponse: httpResp(http.StatusUnauthorized),
		Body:         []byte(`{"error":"unauthorized"}`),
	}, nil)

	c := newMockClient(m)
	_, err := c.ListSecretReferences(context.Background(), "org-a", nil)
	require.ErrorContains(t, err, "unauthorized")
}

// --- GetSecretReference ---

func TestGetSecretReference_Success(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().GetSecretReferenceWithResponse(mock.Anything, "org-a", "sr-1", mock.Anything).Return(&gen.GetSecretReferenceResp{
		HTTPResponse: httpResp(http.StatusOK),
		JSON200:      &gen.SecretReference{Metadata: gen.ObjectMeta{Name: "sr-1"}},
	}, nil)

	c := newMockClient(m)
	result, err := c.GetSecretReference(context.Background(), "org-a", "sr-1")
	require.NoError(t, err)
	assert.Equal(t, "sr-1", result.Metadata.Name)
}

func TestGetSecretReference_TransportError(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().GetSecretReferenceWithResponse(mock.Anything, "org-a", "sr-1", mock.Anything).Return(nil, fmt.Errorf("timeout"))

	c := newMockClient(m)
	_, err := c.GetSecretReference(context.Background(), "org-a", "sr-1")
	require.ErrorContains(t, err, "failed to get secret reference")
}

func TestGetSecretReference_APIError(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().GetSecretReferenceWithResponse(mock.Anything, "org-a", "sr-1", mock.Anything).Return(&gen.GetSecretReferenceResp{
		HTTPResponse: httpResp(http.StatusNotFound),
		Body:         []byte(`{"error":"secret reference not found"}`),
	}, nil)

	c := newMockClient(m)
	_, err := c.GetSecretReference(context.Background(), "org-a", "sr-1")
	require.ErrorContains(t, err, "secret reference not found")
}

// --- DeleteSecretReference ---

func TestDeleteSecretReference_Success(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().DeleteSecretReferenceWithResponse(mock.Anything, "org-a", "sr-1", mock.Anything).Return(&gen.DeleteSecretReferenceResp{
		HTTPResponse: httpResp(http.StatusNoContent),
	}, nil)

	c := newMockClient(m)
	require.NoError(t, c.DeleteSecretReference(context.Background(), "org-a", "sr-1"))
}

func TestDeleteSecretReference_TransportError(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().DeleteSecretReferenceWithResponse(mock.Anything, "org-a", "sr-1", mock.Anything).Return(nil, fmt.Errorf("timeout"))

	c := newMockClient(m)
	require.ErrorContains(t, c.DeleteSecretReference(context.Background(), "org-a", "sr-1"), "failed to delete secret reference")
}

func TestDeleteSecretReference_APIError(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().DeleteSecretReferenceWithResponse(mock.Anything, "org-a", "sr-1", mock.Anything).Return(&gen.DeleteSecretReferenceResp{
		HTTPResponse: httpResp(http.StatusForbidden),
		Body:         []byte(`{"error":"forbidden"}`),
	}, nil)

	c := newMockClient(m)
	require.ErrorContains(t, c.DeleteSecretReference(context.Background(), "org-a", "sr-1"), "forbidden")
}

// --- ListWorkflowRuns ---

func TestListWorkflowRuns_Success(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().ListWorkflowRunsWithResponse(mock.Anything, "org-a", mock.Anything).Return(&gen.ListWorkflowRunsResp{
		HTTPResponse: httpResp(http.StatusOK),
		JSON200: &gen.WorkflowRunList{
			Items:      []gen.WorkflowRun{{Metadata: gen.ObjectMeta{Name: "run-1"}}},
			Pagination: gen.Pagination{},
		},
	}, nil)

	c := newMockClient(m)
	result, err := c.ListWorkflowRuns(context.Background(), "org-a", nil)
	require.NoError(t, err)
	require.Len(t, result.Items, 1)
	assert.Equal(t, "run-1", result.Items[0].Metadata.Name)
}

func TestListWorkflowRuns_TransportError(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().ListWorkflowRunsWithResponse(mock.Anything, "org-a", mock.Anything).Return(nil, fmt.Errorf("connection refused"))

	c := newMockClient(m)
	_, err := c.ListWorkflowRuns(context.Background(), "org-a", nil)
	require.ErrorContains(t, err, "failed to list workflow runs")
}

func TestListWorkflowRuns_APIError(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().ListWorkflowRunsWithResponse(mock.Anything, "org-a", mock.Anything).Return(&gen.ListWorkflowRunsResp{
		HTTPResponse: httpResp(http.StatusUnauthorized),
		Body:         []byte(`{"error":"unauthorized"}`),
	}, nil)

	c := newMockClient(m)
	_, err := c.ListWorkflowRuns(context.Background(), "org-a", nil)
	require.ErrorContains(t, err, "unauthorized")
}

// --- GetWorkflowRun ---

func TestGetWorkflowRun_Success(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().GetWorkflowRunWithResponse(mock.Anything, "org-a", "run-1", mock.Anything).Return(&gen.GetWorkflowRunResp{
		HTTPResponse: httpResp(http.StatusOK),
		JSON200:      &gen.WorkflowRun{Metadata: gen.ObjectMeta{Name: "run-1"}},
	}, nil)

	c := newMockClient(m)
	result, err := c.GetWorkflowRun(context.Background(), "org-a", "run-1")
	require.NoError(t, err)
	assert.Equal(t, "run-1", result.Metadata.Name)
}

func TestGetWorkflowRun_TransportError(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().GetWorkflowRunWithResponse(mock.Anything, "org-a", "run-1", mock.Anything).Return(nil, fmt.Errorf("timeout"))

	c := newMockClient(m)
	_, err := c.GetWorkflowRun(context.Background(), "org-a", "run-1")
	require.ErrorContains(t, err, "failed to get workflow run")
}

func TestGetWorkflowRun_APIError(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().GetWorkflowRunWithResponse(mock.Anything, "org-a", "run-1", mock.Anything).Return(&gen.GetWorkflowRunResp{
		HTTPResponse: httpResp(http.StatusNotFound),
		Body:         []byte(`{"error":"workflow run not found"}`),
	}, nil)

	c := newMockClient(m)
	_, err := c.GetWorkflowRun(context.Background(), "org-a", "run-1")
	require.ErrorContains(t, err, "workflow run not found")
}

// --- CreateWorkflowRun ---

func TestCreateWorkflowRun_Success(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().CreateWorkflowRunWithResponse(mock.Anything, "org-a", mock.Anything, mock.Anything).Return(&gen.CreateWorkflowRunResp{
		HTTPResponse: httpResp(http.StatusCreated),
		JSON201:      &gen.WorkflowRun{Metadata: gen.ObjectMeta{Name: "run-1"}},
	}, nil)

	c := newMockClient(m)
	result, err := c.CreateWorkflowRun(context.Background(), "org-a", gen.WorkflowRun{})
	require.NoError(t, err)
	assert.Equal(t, "run-1", result.Metadata.Name)
}

func TestCreateWorkflowRun_TransportError(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().CreateWorkflowRunWithResponse(mock.Anything, "org-a", mock.Anything, mock.Anything).Return(nil, fmt.Errorf("timeout"))

	c := newMockClient(m)
	_, err := c.CreateWorkflowRun(context.Background(), "org-a", gen.WorkflowRun{})
	require.ErrorContains(t, err, "failed to create workflow run")
}

func TestCreateWorkflowRun_APIError(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().CreateWorkflowRunWithResponse(mock.Anything, "org-a", mock.Anything, mock.Anything).Return(&gen.CreateWorkflowRunResp{
		HTTPResponse: httpResp(http.StatusBadRequest),
		Body:         []byte(`{"error":"validation failed"}`),
	}, nil)

	c := newMockClient(m)
	_, err := c.CreateWorkflowRun(context.Background(), "org-a", gen.WorkflowRun{})
	require.ErrorContains(t, err, "validation failed")
}

// --- ListReleaseBindings ---

func TestListReleaseBindings_Success(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().ListReleaseBindingsWithResponse(mock.Anything, "org-a", mock.Anything).Return(&gen.ListReleaseBindingsResp{
		HTTPResponse: httpResp(http.StatusOK),
		JSON200: &gen.ReleaseBindingList{
			Items:      []gen.ReleaseBinding{{Metadata: gen.ObjectMeta{Name: "rb-1"}}},
			Pagination: gen.Pagination{},
		},
	}, nil)

	c := newMockClient(m)
	result, err := c.ListReleaseBindings(context.Background(), "org-a", nil)
	require.NoError(t, err)
	require.Len(t, result.Items, 1)
	assert.Equal(t, "rb-1", result.Items[0].Metadata.Name)
}

func TestListReleaseBindings_TransportError(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().ListReleaseBindingsWithResponse(mock.Anything, "org-a", mock.Anything).Return(nil, fmt.Errorf("connection refused"))

	c := newMockClient(m)
	_, err := c.ListReleaseBindings(context.Background(), "org-a", nil)
	require.ErrorContains(t, err, "failed to list release bindings")
}

func TestListReleaseBindings_APIError(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().ListReleaseBindingsWithResponse(mock.Anything, "org-a", mock.Anything).Return(&gen.ListReleaseBindingsResp{
		HTTPResponse: httpResp(http.StatusUnauthorized),
		Body:         []byte(`{"error":"unauthorized"}`),
	}, nil)

	c := newMockClient(m)
	_, err := c.ListReleaseBindings(context.Background(), "org-a", nil)
	require.ErrorContains(t, err, "unauthorized")
}

// --- GetReleaseBinding ---

func TestGetReleaseBinding_Success(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().GetReleaseBindingWithResponse(mock.Anything, "org-a", "rb-1", mock.Anything).Return(&gen.GetReleaseBindingResp{
		HTTPResponse: httpResp(http.StatusOK),
		JSON200:      &gen.ReleaseBinding{Metadata: gen.ObjectMeta{Name: "rb-1"}},
	}, nil)

	c := newMockClient(m)
	result, err := c.GetReleaseBinding(context.Background(), "org-a", "rb-1")
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "rb-1", result.Metadata.Name)
}

func TestGetReleaseBinding_NotFound_ReturnsNil(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().GetReleaseBindingWithResponse(mock.Anything, "org-a", "rb-missing", mock.Anything).Return(&gen.GetReleaseBindingResp{
		HTTPResponse: httpResp(http.StatusNotFound),
	}, nil)

	c := newMockClient(m)
	result, err := c.GetReleaseBinding(context.Background(), "org-a", "rb-missing")
	require.NoError(t, err)
	assert.Nil(t, result)
}

func TestGetReleaseBinding_TransportError(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().GetReleaseBindingWithResponse(mock.Anything, "org-a", "rb-1", mock.Anything).Return(nil, fmt.Errorf("timeout"))

	c := newMockClient(m)
	_, err := c.GetReleaseBinding(context.Background(), "org-a", "rb-1")
	require.ErrorContains(t, err, "failed to get release binding")
}

// --- DeleteReleaseBinding ---

func TestDeleteReleaseBinding_Success(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().DeleteReleaseBindingWithResponse(mock.Anything, "org-a", "rb-1", mock.Anything).Return(&gen.DeleteReleaseBindingResp{
		HTTPResponse: httpResp(http.StatusNoContent),
	}, nil)

	c := newMockClient(m)
	require.NoError(t, c.DeleteReleaseBinding(context.Background(), "org-a", "rb-1"))
}

func TestDeleteReleaseBinding_TransportError(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().DeleteReleaseBindingWithResponse(mock.Anything, "org-a", "rb-1", mock.Anything).Return(nil, fmt.Errorf("timeout"))

	c := newMockClient(m)
	require.ErrorContains(t, c.DeleteReleaseBinding(context.Background(), "org-a", "rb-1"), "failed to delete release binding")
}

func TestDeleteReleaseBinding_APIError(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().DeleteReleaseBindingWithResponse(mock.Anything, "org-a", "rb-1", mock.Anything).Return(&gen.DeleteReleaseBindingResp{
		HTTPResponse: httpResp(http.StatusForbidden),
		Body:         []byte(`{"error":"forbidden"}`),
	}, nil)

	c := newMockClient(m)
	require.ErrorContains(t, c.DeleteReleaseBinding(context.Background(), "org-a", "rb-1"), "forbidden")
}

// --- ListComponentReleases ---

func TestListComponentReleases_Success(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().ListComponentReleasesWithResponse(mock.Anything, "org-a", mock.Anything).Return(&gen.ListComponentReleasesResp{
		HTTPResponse: httpResp(http.StatusOK),
		JSON200: &gen.ComponentReleaseList{
			Items:      []gen.ComponentRelease{{Metadata: gen.ObjectMeta{Name: "cr-1"}}},
			Pagination: gen.Pagination{},
		},
	}, nil)

	c := newMockClient(m)
	result, err := c.ListComponentReleases(context.Background(), "org-a", nil)
	require.NoError(t, err)
	require.Len(t, result.Items, 1)
	assert.Equal(t, "cr-1", result.Items[0].Metadata.Name)
}

func TestListComponentReleases_TransportError(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().ListComponentReleasesWithResponse(mock.Anything, "org-a", mock.Anything).Return(nil, fmt.Errorf("connection refused"))

	c := newMockClient(m)
	_, err := c.ListComponentReleases(context.Background(), "org-a", nil)
	require.ErrorContains(t, err, "failed to list component releases")
}

func TestListComponentReleases_APIError(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().ListComponentReleasesWithResponse(mock.Anything, "org-a", mock.Anything).Return(&gen.ListComponentReleasesResp{
		HTTPResponse: httpResp(http.StatusUnauthorized),
		Body:         []byte(`{"error":"unauthorized"}`),
	}, nil)

	c := newMockClient(m)
	_, err := c.ListComponentReleases(context.Background(), "org-a", nil)
	require.ErrorContains(t, err, "unauthorized")
}

// --- GetComponentRelease ---

func TestGetComponentRelease_Success(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().GetComponentReleaseWithResponse(mock.Anything, "org-a", "cr-1", mock.Anything).Return(&gen.GetComponentReleaseResp{
		HTTPResponse: httpResp(http.StatusOK),
		JSON200:      &gen.ComponentRelease{Metadata: gen.ObjectMeta{Name: "cr-1"}},
	}, nil)

	c := newMockClient(m)
	result, err := c.GetComponentRelease(context.Background(), "org-a", "cr-1")
	require.NoError(t, err)
	assert.Equal(t, "cr-1", result.Metadata.Name)
}

func TestGetComponentRelease_TransportError(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().GetComponentReleaseWithResponse(mock.Anything, "org-a", "cr-1", mock.Anything).Return(nil, fmt.Errorf("timeout"))

	c := newMockClient(m)
	_, err := c.GetComponentRelease(context.Background(), "org-a", "cr-1")
	require.ErrorContains(t, err, "failed to get component release")
}

func TestGetComponentRelease_APIError(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().GetComponentReleaseWithResponse(mock.Anything, "org-a", "cr-1", mock.Anything).Return(&gen.GetComponentReleaseResp{
		HTTPResponse: httpResp(http.StatusNotFound),
		Body:         []byte(`{"error":"component release not found"}`),
	}, nil)

	c := newMockClient(m)
	_, err := c.GetComponentRelease(context.Background(), "org-a", "cr-1")
	require.ErrorContains(t, err, "component release not found")
}

// --- DeleteComponentRelease ---

func TestDeleteComponentRelease_Success(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().DeleteComponentReleaseWithResponse(mock.Anything, "org-a", "cr-1", mock.Anything).Return(&gen.DeleteComponentReleaseResp{
		HTTPResponse: httpResp(http.StatusNoContent),
	}, nil)

	c := newMockClient(m)
	require.NoError(t, c.DeleteComponentRelease(context.Background(), "org-a", "cr-1"))
}

func TestDeleteComponentRelease_TransportError(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().DeleteComponentReleaseWithResponse(mock.Anything, "org-a", "cr-1", mock.Anything).Return(nil, fmt.Errorf("timeout"))

	c := newMockClient(m)
	require.ErrorContains(t, c.DeleteComponentRelease(context.Background(), "org-a", "cr-1"), "failed to delete component release")
}

func TestDeleteComponentRelease_APIError(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().DeleteComponentReleaseWithResponse(mock.Anything, "org-a", "cr-1", mock.Anything).Return(&gen.DeleteComponentReleaseResp{
		HTTPResponse: httpResp(http.StatusForbidden),
		Body:         []byte(`{"error":"forbidden"}`),
	}, nil)

	c := newMockClient(m)
	require.ErrorContains(t, c.DeleteComponentRelease(context.Background(), "org-a", "cr-1"), "forbidden")
}

// --- ListWorkloads ---

func TestListWorkloads_Success(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().ListWorkloadsWithResponse(mock.Anything, "org-a", mock.Anything).Return(&gen.ListWorkloadsResp{
		HTTPResponse: httpResp(http.StatusOK),
		JSON200: &gen.WorkloadList{
			Items:      []gen.Workload{{Metadata: gen.ObjectMeta{Name: "wl-1"}}},
			Pagination: gen.Pagination{},
		},
	}, nil)

	c := newMockClient(m)
	result, err := c.ListWorkloads(context.Background(), "org-a", nil)
	require.NoError(t, err)
	require.Len(t, result.Items, 1)
	assert.Equal(t, "wl-1", result.Items[0].Metadata.Name)
}

func TestListWorkloads_TransportError(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().ListWorkloadsWithResponse(mock.Anything, "org-a", mock.Anything).Return(nil, fmt.Errorf("connection refused"))

	c := newMockClient(m)
	_, err := c.ListWorkloads(context.Background(), "org-a", nil)
	require.ErrorContains(t, err, "failed to list workloads")
}

func TestListWorkloads_APIError(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().ListWorkloadsWithResponse(mock.Anything, "org-a", mock.Anything).Return(&gen.ListWorkloadsResp{
		HTTPResponse: httpResp(http.StatusUnauthorized),
		Body:         []byte(`{"error":"unauthorized"}`),
	}, nil)

	c := newMockClient(m)
	_, err := c.ListWorkloads(context.Background(), "org-a", nil)
	require.ErrorContains(t, err, "unauthorized")
}

// --- GetWorkload ---

func TestGetWorkload_Success(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().GetWorkloadWithResponse(mock.Anything, "org-a", "wl-1", mock.Anything).Return(&gen.GetWorkloadResp{
		HTTPResponse: httpResp(http.StatusOK),
		JSON200:      &gen.Workload{Metadata: gen.ObjectMeta{Name: "wl-1"}},
	}, nil)

	c := newMockClient(m)
	result, err := c.GetWorkload(context.Background(), "org-a", "wl-1")
	require.NoError(t, err)
	assert.Equal(t, "wl-1", result.Metadata.Name)
}

func TestGetWorkload_TransportError(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().GetWorkloadWithResponse(mock.Anything, "org-a", "wl-1", mock.Anything).Return(nil, fmt.Errorf("timeout"))

	c := newMockClient(m)
	_, err := c.GetWorkload(context.Background(), "org-a", "wl-1")
	require.ErrorContains(t, err, "failed to get workload")
}

func TestGetWorkload_APIError(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().GetWorkloadWithResponse(mock.Anything, "org-a", "wl-1", mock.Anything).Return(&gen.GetWorkloadResp{
		HTTPResponse: httpResp(http.StatusNotFound),
		Body:         []byte(`{"error":"workload not found"}`),
	}, nil)

	c := newMockClient(m)
	_, err := c.GetWorkload(context.Background(), "org-a", "wl-1")
	require.ErrorContains(t, err, "workload not found")
}

// --- DeleteWorkload ---

func TestDeleteWorkload_Success(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().DeleteWorkloadWithResponse(mock.Anything, "org-a", "wl-1", mock.Anything).Return(&gen.DeleteWorkloadResp{
		HTTPResponse: httpResp(http.StatusNoContent),
	}, nil)

	c := newMockClient(m)
	require.NoError(t, c.DeleteWorkload(context.Background(), "org-a", "wl-1"))
}

func TestDeleteWorkload_TransportError(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().DeleteWorkloadWithResponse(mock.Anything, "org-a", "wl-1", mock.Anything).Return(nil, fmt.Errorf("timeout"))

	c := newMockClient(m)
	require.ErrorContains(t, c.DeleteWorkload(context.Background(), "org-a", "wl-1"), "failed to delete workload")
}

func TestDeleteWorkload_APIError(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().DeleteWorkloadWithResponse(mock.Anything, "org-a", "wl-1", mock.Anything).Return(&gen.DeleteWorkloadResp{
		HTTPResponse: httpResp(http.StatusForbidden),
		Body:         []byte(`{"error":"forbidden"}`),
	}, nil)

	c := newMockClient(m)
	require.ErrorContains(t, c.DeleteWorkload(context.Background(), "org-a", "wl-1"), "forbidden")
}

// --- ListDeploymentPipelines ---

func TestListDeploymentPipelines_Success(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().ListDeploymentPipelinesWithResponse(mock.Anything, "org-a", mock.Anything).Return(&gen.ListDeploymentPipelinesResp{
		HTTPResponse: httpResp(http.StatusOK),
		JSON200: &gen.DeploymentPipelineList{
			Items:      []gen.DeploymentPipeline{{Metadata: gen.ObjectMeta{Name: "pipe-1"}}},
			Pagination: gen.Pagination{},
		},
	}, nil)

	c := newMockClient(m)
	result, err := c.ListDeploymentPipelines(context.Background(), "org-a", nil)
	require.NoError(t, err)
	require.Len(t, result.Items, 1)
	assert.Equal(t, "pipe-1", result.Items[0].Metadata.Name)
}

func TestListDeploymentPipelines_TransportError(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().ListDeploymentPipelinesWithResponse(mock.Anything, "org-a", mock.Anything).Return(nil, fmt.Errorf("connection refused"))

	c := newMockClient(m)
	_, err := c.ListDeploymentPipelines(context.Background(), "org-a", nil)
	require.ErrorContains(t, err, "failed to list deployment pipelines")
}

func TestListDeploymentPipelines_APIError(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().ListDeploymentPipelinesWithResponse(mock.Anything, "org-a", mock.Anything).Return(&gen.ListDeploymentPipelinesResp{
		HTTPResponse: httpResp(http.StatusUnauthorized),
		Body:         []byte(`{"error":"unauthorized"}`),
	}, nil)

	c := newMockClient(m)
	_, err := c.ListDeploymentPipelines(context.Background(), "org-a", nil)
	require.ErrorContains(t, err, "unauthorized")
}

// --- GetDeploymentPipeline ---

func TestGetDeploymentPipeline_Success(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().GetDeploymentPipelineWithResponse(mock.Anything, "org-a", "pipe-1", mock.Anything).Return(&gen.GetDeploymentPipelineResp{
		HTTPResponse: httpResp(http.StatusOK),
		JSON200:      &gen.DeploymentPipeline{Metadata: gen.ObjectMeta{Name: "pipe-1"}},
	}, nil)

	c := newMockClient(m)
	result, err := c.GetDeploymentPipeline(context.Background(), "org-a", "pipe-1")
	require.NoError(t, err)
	assert.Equal(t, "pipe-1", result.Metadata.Name)
}

func TestGetDeploymentPipeline_TransportError(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().GetDeploymentPipelineWithResponse(mock.Anything, "org-a", "pipe-1", mock.Anything).Return(nil, fmt.Errorf("timeout"))

	c := newMockClient(m)
	_, err := c.GetDeploymentPipeline(context.Background(), "org-a", "pipe-1")
	require.ErrorContains(t, err, "failed to get deployment pipeline")
}

func TestGetDeploymentPipeline_APIError(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().GetDeploymentPipelineWithResponse(mock.Anything, "org-a", "pipe-1", mock.Anything).Return(&gen.GetDeploymentPipelineResp{
		HTTPResponse: httpResp(http.StatusNotFound),
		Body:         []byte(`{"error":"deployment pipeline not found"}`),
	}, nil)

	c := newMockClient(m)
	_, err := c.GetDeploymentPipeline(context.Background(), "org-a", "pipe-1")
	require.ErrorContains(t, err, "deployment pipeline not found")
}

// --- DeleteDeploymentPipeline ---

func TestDeleteDeploymentPipeline_Success(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().DeleteDeploymentPipelineWithResponse(mock.Anything, "org-a", "pipe-1", mock.Anything).Return(&gen.DeleteDeploymentPipelineResp{
		HTTPResponse: httpResp(http.StatusNoContent),
	}, nil)

	c := newMockClient(m)
	require.NoError(t, c.DeleteDeploymentPipeline(context.Background(), "org-a", "pipe-1"))
}

func TestDeleteDeploymentPipeline_TransportError(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().DeleteDeploymentPipelineWithResponse(mock.Anything, "org-a", "pipe-1", mock.Anything).Return(nil, fmt.Errorf("timeout"))

	c := newMockClient(m)
	require.ErrorContains(t, c.DeleteDeploymentPipeline(context.Background(), "org-a", "pipe-1"), "failed to delete deployment pipeline")
}

func TestDeleteDeploymentPipeline_APIError(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().DeleteDeploymentPipelineWithResponse(mock.Anything, "org-a", "pipe-1", mock.Anything).Return(&gen.DeleteDeploymentPipelineResp{
		HTTPResponse: httpResp(http.StatusForbidden),
		Body:         []byte(`{"error":"forbidden"}`),
	}, nil)

	c := newMockClient(m)
	require.ErrorContains(t, c.DeleteDeploymentPipeline(context.Background(), "org-a", "pipe-1"), "forbidden")
}

// --- ListClusterDataPlanes ---

func TestListClusterDataPlanes_Success(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().ListClusterDataPlanesWithResponse(mock.Anything, mock.Anything).Return(&gen.ListClusterDataPlanesResp{
		HTTPResponse: httpResp(http.StatusOK),
		JSON200: &gen.ClusterDataPlaneList{
			Items:      []gen.ClusterDataPlane{{Metadata: gen.ObjectMeta{Name: "cdp-1"}}},
			Pagination: gen.Pagination{},
		},
	}, nil)

	c := newMockClient(m)
	result, err := c.ListClusterDataPlanes(context.Background(), nil)
	require.NoError(t, err)
	require.Len(t, result.Items, 1)
	assert.Equal(t, "cdp-1", result.Items[0].Metadata.Name)
}

func TestListClusterDataPlanes_TransportError(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().ListClusterDataPlanesWithResponse(mock.Anything, mock.Anything).Return(nil, fmt.Errorf("connection refused"))

	c := newMockClient(m)
	_, err := c.ListClusterDataPlanes(context.Background(), nil)
	require.ErrorContains(t, err, "failed to list cluster data planes")
}

func TestListClusterDataPlanes_APIError(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().ListClusterDataPlanesWithResponse(mock.Anything, mock.Anything).Return(&gen.ListClusterDataPlanesResp{
		HTTPResponse: httpResp(http.StatusUnauthorized),
		Body:         []byte(`{"error":"unauthorized"}`),
	}, nil)

	c := newMockClient(m)
	_, err := c.ListClusterDataPlanes(context.Background(), nil)
	require.ErrorContains(t, err, "unauthorized")
}

// --- GetClusterDataPlane ---

func TestGetClusterDataPlane_Success(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().GetClusterDataPlaneWithResponse(mock.Anything, "cdp-1", mock.Anything).Return(&gen.GetClusterDataPlaneResp{
		HTTPResponse: httpResp(http.StatusOK),
		JSON200:      &gen.ClusterDataPlane{Metadata: gen.ObjectMeta{Name: "cdp-1"}},
	}, nil)

	c := newMockClient(m)
	result, err := c.GetClusterDataPlane(context.Background(), "cdp-1")
	require.NoError(t, err)
	assert.Equal(t, "cdp-1", result.Metadata.Name)
}

func TestGetClusterDataPlane_TransportError(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().GetClusterDataPlaneWithResponse(mock.Anything, "cdp-1", mock.Anything).Return(nil, fmt.Errorf("timeout"))

	c := newMockClient(m)
	_, err := c.GetClusterDataPlane(context.Background(), "cdp-1")
	require.ErrorContains(t, err, "failed to get cluster data plane")
}

func TestGetClusterDataPlane_APIError(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().GetClusterDataPlaneWithResponse(mock.Anything, "cdp-1", mock.Anything).Return(&gen.GetClusterDataPlaneResp{
		HTTPResponse: httpResp(http.StatusNotFound),
		Body:         []byte(`{"error":"cluster data plane not found"}`),
	}, nil)

	c := newMockClient(m)
	_, err := c.GetClusterDataPlane(context.Background(), "cdp-1")
	require.ErrorContains(t, err, "cluster data plane not found")
}

// --- DeleteClusterDataPlane ---

func TestDeleteClusterDataPlane_Success(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().DeleteClusterDataPlaneWithResponse(mock.Anything, "cdp-1", mock.Anything).Return(&gen.DeleteClusterDataPlaneResp{
		HTTPResponse: httpResp(http.StatusNoContent),
	}, nil)

	c := newMockClient(m)
	require.NoError(t, c.DeleteClusterDataPlane(context.Background(), "cdp-1"))
}

func TestDeleteClusterDataPlane_TransportError(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().DeleteClusterDataPlaneWithResponse(mock.Anything, "cdp-1", mock.Anything).Return(nil, fmt.Errorf("timeout"))

	c := newMockClient(m)
	require.ErrorContains(t, c.DeleteClusterDataPlane(context.Background(), "cdp-1"), "failed to delete cluster data plane")
}

func TestDeleteClusterDataPlane_APIError(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().DeleteClusterDataPlaneWithResponse(mock.Anything, "cdp-1", mock.Anything).Return(&gen.DeleteClusterDataPlaneResp{
		HTTPResponse: httpResp(http.StatusForbidden),
		Body:         []byte(`{"error":"forbidden"}`),
	}, nil)

	c := newMockClient(m)
	require.ErrorContains(t, c.DeleteClusterDataPlane(context.Background(), "cdp-1"), "forbidden")
}

// --- ListClusterWorkflows ---

func TestListClusterWorkflows_Success(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().ListClusterWorkflowsWithResponse(mock.Anything, mock.Anything).Return(&gen.ListClusterWorkflowsResp{
		HTTPResponse: httpResp(http.StatusOK),
		JSON200: &gen.ClusterWorkflowList{
			Items:      []gen.ClusterWorkflow{{Metadata: gen.ObjectMeta{Name: "cwf-1"}}},
			Pagination: gen.Pagination{},
		},
	}, nil)

	c := newMockClient(m)
	result, err := c.ListClusterWorkflows(context.Background(), nil)
	require.NoError(t, err)
	require.Len(t, result.Items, 1)
	assert.Equal(t, "cwf-1", result.Items[0].Metadata.Name)
}

func TestListClusterWorkflows_TransportError(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().ListClusterWorkflowsWithResponse(mock.Anything, mock.Anything).Return(nil, fmt.Errorf("connection refused"))

	c := newMockClient(m)
	_, err := c.ListClusterWorkflows(context.Background(), nil)
	require.ErrorContains(t, err, "failed to list cluster workflows")
}

func TestListClusterWorkflows_APIError(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().ListClusterWorkflowsWithResponse(mock.Anything, mock.Anything).Return(&gen.ListClusterWorkflowsResp{
		HTTPResponse: httpResp(http.StatusUnauthorized),
		Body:         []byte(`{"error":"unauthorized"}`),
	}, nil)

	c := newMockClient(m)
	_, err := c.ListClusterWorkflows(context.Background(), nil)
	require.ErrorContains(t, err, "unauthorized")
}

// --- GetClusterWorkflow ---

func TestGetClusterWorkflow_Success(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().GetClusterWorkflowWithResponse(mock.Anything, "cwf-1", mock.Anything).Return(&gen.GetClusterWorkflowResp{
		HTTPResponse: httpResp(http.StatusOK),
		JSON200:      &gen.ClusterWorkflow{Metadata: gen.ObjectMeta{Name: "cwf-1"}},
	}, nil)

	c := newMockClient(m)
	result, err := c.GetClusterWorkflow(context.Background(), "cwf-1")
	require.NoError(t, err)
	assert.Equal(t, "cwf-1", result.Metadata.Name)
}

func TestGetClusterWorkflow_TransportError(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().GetClusterWorkflowWithResponse(mock.Anything, "cwf-1", mock.Anything).Return(nil, fmt.Errorf("timeout"))

	c := newMockClient(m)
	_, err := c.GetClusterWorkflow(context.Background(), "cwf-1")
	require.ErrorContains(t, err, "failed to get cluster workflow")
}

func TestGetClusterWorkflow_APIError(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().GetClusterWorkflowWithResponse(mock.Anything, "cwf-1", mock.Anything).Return(&gen.GetClusterWorkflowResp{
		HTTPResponse: httpResp(http.StatusNotFound),
		Body:         []byte(`{"error":"cluster workflow not found"}`),
	}, nil)

	c := newMockClient(m)
	_, err := c.GetClusterWorkflow(context.Background(), "cwf-1")
	require.ErrorContains(t, err, "cluster workflow not found")
}

// --- DeleteClusterWorkflow ---

func TestDeleteClusterWorkflow_Success(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().DeleteClusterWorkflowWithResponse(mock.Anything, "cwf-1", mock.Anything).Return(&gen.DeleteClusterWorkflowResp{
		HTTPResponse: httpResp(http.StatusNoContent),
	}, nil)

	c := newMockClient(m)
	require.NoError(t, c.DeleteClusterWorkflow(context.Background(), "cwf-1"))
}

func TestDeleteClusterWorkflow_TransportError(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().DeleteClusterWorkflowWithResponse(mock.Anything, "cwf-1", mock.Anything).Return(nil, fmt.Errorf("timeout"))

	c := newMockClient(m)
	require.ErrorContains(t, c.DeleteClusterWorkflow(context.Background(), "cwf-1"), "failed to delete cluster workflow")
}

func TestDeleteClusterWorkflow_APIError(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().DeleteClusterWorkflowWithResponse(mock.Anything, "cwf-1", mock.Anything).Return(&gen.DeleteClusterWorkflowResp{
		HTTPResponse: httpResp(http.StatusForbidden),
		Body:         []byte(`{"error":"forbidden"}`),
	}, nil)

	c := newMockClient(m)
	require.ErrorContains(t, c.DeleteClusterWorkflow(context.Background(), "cwf-1"), "forbidden")
}

// --- ListClusterWorkflowPlanes ---

func TestListClusterWorkflowPlanes_Success(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().ListClusterWorkflowPlanesWithResponse(mock.Anything, mock.Anything).Return(&gen.ListClusterWorkflowPlanesResp{
		HTTPResponse: httpResp(http.StatusOK),
		JSON200: &gen.ClusterWorkflowPlaneList{
			Items:      []gen.ClusterWorkflowPlane{{Metadata: gen.ObjectMeta{Name: "cwp-1"}}},
			Pagination: gen.Pagination{},
		},
	}, nil)

	c := newMockClient(m)
	result, err := c.ListClusterWorkflowPlanes(context.Background(), nil)
	require.NoError(t, err)
	require.Len(t, result.Items, 1)
	assert.Equal(t, "cwp-1", result.Items[0].Metadata.Name)
}

func TestListClusterWorkflowPlanes_TransportError(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().ListClusterWorkflowPlanesWithResponse(mock.Anything, mock.Anything).Return(nil, fmt.Errorf("connection refused"))

	c := newMockClient(m)
	_, err := c.ListClusterWorkflowPlanes(context.Background(), nil)
	require.ErrorContains(t, err, "failed to list cluster workflow planes")
}

func TestListClusterWorkflowPlanes_APIError(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().ListClusterWorkflowPlanesWithResponse(mock.Anything, mock.Anything).Return(&gen.ListClusterWorkflowPlanesResp{
		HTTPResponse: httpResp(http.StatusUnauthorized),
		Body:         []byte(`{"error":"unauthorized"}`),
	}, nil)

	c := newMockClient(m)
	_, err := c.ListClusterWorkflowPlanes(context.Background(), nil)
	require.ErrorContains(t, err, "unauthorized")
}

// --- GetClusterWorkflowPlane ---

func TestGetClusterWorkflowPlane_Success(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().GetClusterWorkflowPlaneWithResponse(mock.Anything, "cwp-1", mock.Anything).Return(&gen.GetClusterWorkflowPlaneResp{
		HTTPResponse: httpResp(http.StatusOK),
		JSON200:      &gen.ClusterWorkflowPlane{Metadata: gen.ObjectMeta{Name: "cwp-1"}},
	}, nil)

	c := newMockClient(m)
	result, err := c.GetClusterWorkflowPlane(context.Background(), "cwp-1")
	require.NoError(t, err)
	assert.Equal(t, "cwp-1", result.Metadata.Name)
}

func TestGetClusterWorkflowPlane_TransportError(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().GetClusterWorkflowPlaneWithResponse(mock.Anything, "cwp-1", mock.Anything).Return(nil, fmt.Errorf("timeout"))

	c := newMockClient(m)
	_, err := c.GetClusterWorkflowPlane(context.Background(), "cwp-1")
	require.ErrorContains(t, err, "failed to get cluster workflow plane")
}

func TestGetClusterWorkflowPlane_APIError(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().GetClusterWorkflowPlaneWithResponse(mock.Anything, "cwp-1", mock.Anything).Return(&gen.GetClusterWorkflowPlaneResp{
		HTTPResponse: httpResp(http.StatusNotFound),
		Body:         []byte(`{"error":"cluster workflow plane not found"}`),
	}, nil)

	c := newMockClient(m)
	_, err := c.GetClusterWorkflowPlane(context.Background(), "cwp-1")
	require.ErrorContains(t, err, "cluster workflow plane not found")
}

// --- DeleteClusterWorkflowPlane ---

func TestDeleteClusterWorkflowPlane_Success(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().DeleteClusterWorkflowPlaneWithResponse(mock.Anything, "cwp-1", mock.Anything).Return(&gen.DeleteClusterWorkflowPlaneResp{
		HTTPResponse: httpResp(http.StatusNoContent),
	}, nil)

	c := newMockClient(m)
	require.NoError(t, c.DeleteClusterWorkflowPlane(context.Background(), "cwp-1"))
}

func TestDeleteClusterWorkflowPlane_TransportError(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().DeleteClusterWorkflowPlaneWithResponse(mock.Anything, "cwp-1", mock.Anything).Return(nil, fmt.Errorf("timeout"))

	c := newMockClient(m)
	require.ErrorContains(t, c.DeleteClusterWorkflowPlane(context.Background(), "cwp-1"), "failed to delete cluster workflow plane")
}

func TestDeleteClusterWorkflowPlane_APIError(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().DeleteClusterWorkflowPlaneWithResponse(mock.Anything, "cwp-1", mock.Anything).Return(&gen.DeleteClusterWorkflowPlaneResp{
		HTTPResponse: httpResp(http.StatusForbidden),
		Body:         []byte(`{"error":"forbidden"}`),
	}, nil)

	c := newMockClient(m)
	require.ErrorContains(t, c.DeleteClusterWorkflowPlane(context.Background(), "cwp-1"), "forbidden")
}

// --- ListClusterObservabilityPlanes ---

func TestListClusterObservabilityPlanes_Success(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().ListClusterObservabilityPlanesWithResponse(mock.Anything, mock.Anything).Return(&gen.ListClusterObservabilityPlanesResp{
		HTTPResponse: httpResp(http.StatusOK),
		JSON200: &gen.ClusterObservabilityPlaneList{
			Items:      []gen.ClusterObservabilityPlane{{Metadata: gen.ObjectMeta{Name: "cop-1"}}},
			Pagination: gen.Pagination{},
		},
	}, nil)

	c := newMockClient(m)
	result, err := c.ListClusterObservabilityPlanes(context.Background(), nil)
	require.NoError(t, err)
	require.Len(t, result.Items, 1)
	assert.Equal(t, "cop-1", result.Items[0].Metadata.Name)
}

func TestListClusterObservabilityPlanes_TransportError(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().ListClusterObservabilityPlanesWithResponse(mock.Anything, mock.Anything).Return(nil, fmt.Errorf("connection refused"))

	c := newMockClient(m)
	_, err := c.ListClusterObservabilityPlanes(context.Background(), nil)
	require.ErrorContains(t, err, "failed to list cluster observability planes")
}

func TestListClusterObservabilityPlanes_APIError(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().ListClusterObservabilityPlanesWithResponse(mock.Anything, mock.Anything).Return(&gen.ListClusterObservabilityPlanesResp{
		HTTPResponse: httpResp(http.StatusUnauthorized),
		Body:         []byte(`{"error":"unauthorized"}`),
	}, nil)

	c := newMockClient(m)
	_, err := c.ListClusterObservabilityPlanes(context.Background(), nil)
	require.ErrorContains(t, err, "unauthorized")
}

// --- GetClusterObservabilityPlane ---

func TestGetClusterObservabilityPlane_Success(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().GetClusterObservabilityPlaneWithResponse(mock.Anything, "cop-1", mock.Anything).Return(&gen.GetClusterObservabilityPlaneResp{
		HTTPResponse: httpResp(http.StatusOK),
		JSON200:      &gen.ClusterObservabilityPlane{Metadata: gen.ObjectMeta{Name: "cop-1"}},
	}, nil)

	c := newMockClient(m)
	result, err := c.GetClusterObservabilityPlane(context.Background(), "cop-1")
	require.NoError(t, err)
	assert.Equal(t, "cop-1", result.Metadata.Name)
}

func TestGetClusterObservabilityPlane_TransportError(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().GetClusterObservabilityPlaneWithResponse(mock.Anything, "cop-1", mock.Anything).Return(nil, fmt.Errorf("timeout"))

	c := newMockClient(m)
	_, err := c.GetClusterObservabilityPlane(context.Background(), "cop-1")
	require.ErrorContains(t, err, "failed to get cluster observability plane")
}

func TestGetClusterObservabilityPlane_APIError(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().GetClusterObservabilityPlaneWithResponse(mock.Anything, "cop-1", mock.Anything).Return(&gen.GetClusterObservabilityPlaneResp{
		HTTPResponse: httpResp(http.StatusNotFound),
		Body:         []byte(`{"error":"cluster observability plane not found"}`),
	}, nil)

	c := newMockClient(m)
	_, err := c.GetClusterObservabilityPlane(context.Background(), "cop-1")
	require.ErrorContains(t, err, "cluster observability plane not found")
}

// --- DeleteClusterObservabilityPlane ---

func TestDeleteClusterObservabilityPlane_Success(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().DeleteClusterObservabilityPlaneWithResponse(mock.Anything, "cop-1", mock.Anything).Return(&gen.DeleteClusterObservabilityPlaneResp{
		HTTPResponse: httpResp(http.StatusNoContent),
	}, nil)

	c := newMockClient(m)
	require.NoError(t, c.DeleteClusterObservabilityPlane(context.Background(), "cop-1"))
}

func TestDeleteClusterObservabilityPlane_TransportError(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().DeleteClusterObservabilityPlaneWithResponse(mock.Anything, "cop-1", mock.Anything).Return(nil, fmt.Errorf("timeout"))

	c := newMockClient(m)
	require.ErrorContains(t, c.DeleteClusterObservabilityPlane(context.Background(), "cop-1"), "failed to delete cluster observability plane")
}

func TestDeleteClusterObservabilityPlane_APIError(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().DeleteClusterObservabilityPlaneWithResponse(mock.Anything, "cop-1", mock.Anything).Return(&gen.DeleteClusterObservabilityPlaneResp{
		HTTPResponse: httpResp(http.StatusForbidden),
		Body:         []byte(`{"error":"forbidden"}`),
	}, nil)

	c := newMockClient(m)
	require.ErrorContains(t, c.DeleteClusterObservabilityPlane(context.Background(), "cop-1"), "forbidden")
}

// --- GetWorkflowRunLogs ---

func TestGetWorkflowRunLogs_Success(t *testing.T) {
	entries := []gen.WorkflowRunLogEntry{{Log: "build started"}}
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().GetWorkflowRunLogsWithResponse(mock.Anything, "org-a", "run-1", mock.Anything).Return(&gen.GetWorkflowRunLogsResp{
		HTTPResponse: httpResp(http.StatusOK),
		JSON200:      &entries,
	}, nil)

	c := newMockClient(m)
	result, err := c.GetWorkflowRunLogs(context.Background(), "org-a", "run-1", nil)
	require.NoError(t, err)
	require.Len(t, result, 1)
	assert.Equal(t, "build started", result[0].Log)
}

func TestGetWorkflowRunLogs_TransportError(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().GetWorkflowRunLogsWithResponse(mock.Anything, "org-a", "run-1", mock.Anything).Return(nil, fmt.Errorf("timeout"))

	c := newMockClient(m)
	_, err := c.GetWorkflowRunLogs(context.Background(), "org-a", "run-1", nil)
	require.ErrorContains(t, err, "failed to get workflow run logs")
}

func TestGetWorkflowRunLogs_APIError(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().GetWorkflowRunLogsWithResponse(mock.Anything, "org-a", "run-1", mock.Anything).Return(&gen.GetWorkflowRunLogsResp{
		HTTPResponse: httpResp(http.StatusNotFound),
		Body:         []byte(`{"error":"workflow run not found"}`),
	}, nil)

	c := newMockClient(m)
	_, err := c.GetWorkflowRunLogs(context.Background(), "org-a", "run-1", nil)
	require.ErrorContains(t, err, "workflow run not found")
}

// --- GetWorkflowRunStatus ---

func TestGetWorkflowRunStatus_Success(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().GetWorkflowRunStatusWithResponse(mock.Anything, "org-a", "run-1", mock.Anything).Return(&gen.GetWorkflowRunStatusResp{
		HTTPResponse: httpResp(http.StatusOK),
		JSON200:      &gen.WorkflowRunStatusResponse{},
	}, nil)

	c := newMockClient(m)
	result, err := c.GetWorkflowRunStatus(context.Background(), "org-a", "run-1")
	require.NoError(t, err)
	require.NotNil(t, result)
}

func TestGetWorkflowRunStatus_TransportError(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().GetWorkflowRunStatusWithResponse(mock.Anything, "org-a", "run-1", mock.Anything).Return(nil, fmt.Errorf("timeout"))

	c := newMockClient(m)
	_, err := c.GetWorkflowRunStatus(context.Background(), "org-a", "run-1")
	require.ErrorContains(t, err, "failed to get workflow run status")
}

func TestGetWorkflowRunStatus_APIError(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().GetWorkflowRunStatusWithResponse(mock.Anything, "org-a", "run-1", mock.Anything).Return(&gen.GetWorkflowRunStatusResp{
		HTTPResponse: httpResp(http.StatusNotFound),
		Body:         []byte(`{"error":"workflow run not found"}`),
	}, nil)

	c := newMockClient(m)
	_, err := c.GetWorkflowRunStatus(context.Background(), "org-a", "run-1")
	require.ErrorContains(t, err, "workflow run not found")
}

// --- ListObservabilityAlertsNotificationChannels ---

func TestListObservabilityAlertsNotificationChannels_Success(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().ListObservabilityAlertsNotificationChannelsWithResponse(mock.Anything, "org-a", mock.Anything).Return(&gen.ListObservabilityAlertsNotificationChannelsResp{
		HTTPResponse: httpResp(http.StatusOK),
		JSON200: &gen.ObservabilityAlertsNotificationChannelList{
			Items:      []gen.ObservabilityAlertsNotificationChannel{{Metadata: gen.ObjectMeta{Name: "ch-1"}}},
			Pagination: gen.Pagination{},
		},
	}, nil)

	c := newMockClient(m)
	result, err := c.ListObservabilityAlertsNotificationChannels(context.Background(), "org-a", nil)
	require.NoError(t, err)
	require.Len(t, result.Items, 1)
	assert.Equal(t, "ch-1", result.Items[0].Metadata.Name)
}

func TestListObservabilityAlertsNotificationChannels_TransportError(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().ListObservabilityAlertsNotificationChannelsWithResponse(mock.Anything, "org-a", mock.Anything).Return(nil, fmt.Errorf("connection refused"))

	c := newMockClient(m)
	_, err := c.ListObservabilityAlertsNotificationChannels(context.Background(), "org-a", nil)
	require.ErrorContains(t, err, "failed to list observability alerts notification channels")
}

func TestListObservabilityAlertsNotificationChannels_APIError(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().ListObservabilityAlertsNotificationChannelsWithResponse(mock.Anything, "org-a", mock.Anything).Return(&gen.ListObservabilityAlertsNotificationChannelsResp{
		HTTPResponse: httpResp(http.StatusUnauthorized),
		Body:         []byte(`{"error":"unauthorized"}`),
	}, nil)

	c := newMockClient(m)
	_, err := c.ListObservabilityAlertsNotificationChannels(context.Background(), "org-a", nil)
	require.ErrorContains(t, err, "unauthorized")
}

// --- GetObservabilityAlertsNotificationChannel ---

func TestGetObservabilityAlertsNotificationChannel_Success(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().GetObservabilityAlertsNotificationChannelWithResponse(mock.Anything, "org-a", "ch-1", mock.Anything).Return(&gen.GetObservabilityAlertsNotificationChannelResp{
		HTTPResponse: httpResp(http.StatusOK),
		JSON200:      &gen.ObservabilityAlertsNotificationChannel{Metadata: gen.ObjectMeta{Name: "ch-1"}},
	}, nil)

	c := newMockClient(m)
	result, err := c.GetObservabilityAlertsNotificationChannel(context.Background(), "org-a", "ch-1")
	require.NoError(t, err)
	assert.Equal(t, "ch-1", result.Metadata.Name)
}

func TestGetObservabilityAlertsNotificationChannel_TransportError(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().GetObservabilityAlertsNotificationChannelWithResponse(mock.Anything, "org-a", "ch-1", mock.Anything).Return(nil, fmt.Errorf("timeout"))

	c := newMockClient(m)
	_, err := c.GetObservabilityAlertsNotificationChannel(context.Background(), "org-a", "ch-1")
	require.ErrorContains(t, err, "failed to get observability alerts notification channel")
}

func TestGetObservabilityAlertsNotificationChannel_APIError(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().GetObservabilityAlertsNotificationChannelWithResponse(mock.Anything, "org-a", "ch-1", mock.Anything).Return(&gen.GetObservabilityAlertsNotificationChannelResp{
		HTTPResponse: httpResp(http.StatusNotFound),
		Body:         []byte(`{"error":"channel not found"}`),
	}, nil)

	c := newMockClient(m)
	_, err := c.GetObservabilityAlertsNotificationChannel(context.Background(), "org-a", "ch-1")
	require.ErrorContains(t, err, "channel not found")
}

// --- DeleteObservabilityAlertsNotificationChannel ---

func TestDeleteObservabilityAlertsNotificationChannel_Success(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().DeleteObservabilityAlertsNotificationChannelWithResponse(mock.Anything, "org-a", "ch-1", mock.Anything).Return(&gen.DeleteObservabilityAlertsNotificationChannelResp{
		HTTPResponse: httpResp(http.StatusNoContent),
	}, nil)

	c := newMockClient(m)
	require.NoError(t, c.DeleteObservabilityAlertsNotificationChannel(context.Background(), "org-a", "ch-1"))
}

func TestDeleteObservabilityAlertsNotificationChannel_TransportError(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().DeleteObservabilityAlertsNotificationChannelWithResponse(mock.Anything, "org-a", "ch-1", mock.Anything).Return(nil, fmt.Errorf("timeout"))

	c := newMockClient(m)
	require.ErrorContains(t, c.DeleteObservabilityAlertsNotificationChannel(context.Background(), "org-a", "ch-1"), "failed to delete observability alerts notification channel")
}

func TestDeleteObservabilityAlertsNotificationChannel_APIError(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().DeleteObservabilityAlertsNotificationChannelWithResponse(mock.Anything, "org-a", "ch-1", mock.Anything).Return(&gen.DeleteObservabilityAlertsNotificationChannelResp{
		HTTPResponse: httpResp(http.StatusForbidden),
		Body:         []byte(`{"error":"forbidden"}`),
	}, nil)

	c := newMockClient(m)
	require.ErrorContains(t, c.DeleteObservabilityAlertsNotificationChannel(context.Background(), "org-a", "ch-1"), "forbidden")
}

// --- ListClusterRoles ---

func TestListClusterRoles_Success(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().ListClusterRolesWithResponse(mock.Anything, mock.Anything).Return(&gen.ListClusterRolesResp{
		HTTPResponse: httpResp(http.StatusOK),
		JSON200: &gen.ClusterAuthzRoleList{
			Items:      []gen.ClusterAuthzRole{{Metadata: gen.ObjectMeta{Name: "role-1"}}},
			Pagination: gen.Pagination{},
		},
	}, nil)

	c := newMockClient(m)
	result, err := c.ListClusterRoles(context.Background(), nil)
	require.NoError(t, err)
	require.Len(t, result.Items, 1)
	assert.Equal(t, "role-1", result.Items[0].Metadata.Name)
}

func TestListClusterRoles_TransportError(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().ListClusterRolesWithResponse(mock.Anything, mock.Anything).Return(nil, fmt.Errorf("connection refused"))

	c := newMockClient(m)
	_, err := c.ListClusterRoles(context.Background(), nil)
	require.ErrorContains(t, err, "failed to list cluster roles")
}

func TestListClusterRoles_APIError(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().ListClusterRolesWithResponse(mock.Anything, mock.Anything).Return(&gen.ListClusterRolesResp{
		HTTPResponse: httpResp(http.StatusUnauthorized),
		Body:         []byte(`{"error":"unauthorized"}`),
	}, nil)

	c := newMockClient(m)
	_, err := c.ListClusterRoles(context.Background(), nil)
	require.ErrorContains(t, err, "unauthorized")
}

// --- GetClusterRole ---

func TestGetClusterRole_Success(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().GetClusterRoleWithResponse(mock.Anything, "role-1", mock.Anything).Return(&gen.GetClusterRoleResp{
		HTTPResponse: httpResp(http.StatusOK),
		JSON200:      &gen.ClusterAuthzRole{Metadata: gen.ObjectMeta{Name: "role-1"}},
	}, nil)

	c := newMockClient(m)
	result, err := c.GetClusterRole(context.Background(), "role-1")
	require.NoError(t, err)
	assert.Equal(t, "role-1", result.Metadata.Name)
}

func TestGetClusterRole_TransportError(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().GetClusterRoleWithResponse(mock.Anything, "role-1", mock.Anything).Return(nil, fmt.Errorf("timeout"))

	c := newMockClient(m)
	_, err := c.GetClusterRole(context.Background(), "role-1")
	require.ErrorContains(t, err, "failed to get cluster role")
}

// --- DeleteClusterRole ---

func TestDeleteClusterRole_Success(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().DeleteClusterRoleWithResponse(mock.Anything, "role-1", mock.Anything).Return(&gen.DeleteClusterRoleResp{
		HTTPResponse: httpResp(http.StatusNoContent),
	}, nil)

	c := newMockClient(m)
	require.NoError(t, c.DeleteClusterRole(context.Background(), "role-1"))
}

func TestDeleteClusterRole_TransportError(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().DeleteClusterRoleWithResponse(mock.Anything, "role-1", mock.Anything).Return(nil, fmt.Errorf("timeout"))

	c := newMockClient(m)
	require.ErrorContains(t, c.DeleteClusterRole(context.Background(), "role-1"), "failed to delete cluster role")
}

func TestDeleteClusterRole_APIError(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().DeleteClusterRoleWithResponse(mock.Anything, "role-1", mock.Anything).Return(&gen.DeleteClusterRoleResp{
		HTTPResponse: httpResp(http.StatusForbidden),
		Body:         []byte(`{"error":"forbidden"}`),
	}, nil)

	c := newMockClient(m)
	require.ErrorContains(t, c.DeleteClusterRole(context.Background(), "role-1"), "forbidden")
}

// --- ListClusterRoleBindings ---

func TestListClusterRoleBindings_Success(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().ListClusterRoleBindingsWithResponse(mock.Anything, mock.Anything).Return(&gen.ListClusterRoleBindingsResp{
		HTTPResponse: httpResp(http.StatusOK),
		JSON200: &gen.ClusterAuthzRoleBindingList{
			Items:      []gen.ClusterAuthzRoleBinding{{Metadata: gen.ObjectMeta{Name: "rb-1"}}},
			Pagination: gen.Pagination{},
		},
	}, nil)

	c := newMockClient(m)
	result, err := c.ListClusterRoleBindings(context.Background(), nil)
	require.NoError(t, err)
	require.Len(t, result.Items, 1)
	assert.Equal(t, "rb-1", result.Items[0].Metadata.Name)
}

func TestListClusterRoleBindings_TransportError(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().ListClusterRoleBindingsWithResponse(mock.Anything, mock.Anything).Return(nil, fmt.Errorf("connection refused"))

	c := newMockClient(m)
	_, err := c.ListClusterRoleBindings(context.Background(), nil)
	require.ErrorContains(t, err, "failed to list cluster role bindings")
}

// --- GetClusterRoleBinding ---

func TestGetClusterRoleBinding_Success(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().GetClusterRoleBindingWithResponse(mock.Anything, "rb-1", mock.Anything).Return(&gen.GetClusterRoleBindingResp{
		HTTPResponse: httpResp(http.StatusOK),
		JSON200:      &gen.ClusterAuthzRoleBinding{Metadata: gen.ObjectMeta{Name: "rb-1"}},
	}, nil)

	c := newMockClient(m)
	result, err := c.GetClusterRoleBinding(context.Background(), "rb-1")
	require.NoError(t, err)
	assert.Equal(t, "rb-1", result.Metadata.Name)
}

func TestGetClusterRoleBinding_TransportError(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().GetClusterRoleBindingWithResponse(mock.Anything, "rb-1", mock.Anything).Return(nil, fmt.Errorf("timeout"))

	c := newMockClient(m)
	_, err := c.GetClusterRoleBinding(context.Background(), "rb-1")
	require.ErrorContains(t, err, "failed to get cluster role binding")
}

// --- DeleteClusterRoleBinding ---

func TestDeleteClusterRoleBinding_Success(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().DeleteClusterRoleBindingWithResponse(mock.Anything, "rb-1", mock.Anything).Return(&gen.DeleteClusterRoleBindingResp{
		HTTPResponse: httpResp(http.StatusNoContent),
	}, nil)

	c := newMockClient(m)
	require.NoError(t, c.DeleteClusterRoleBinding(context.Background(), "rb-1"))
}

func TestDeleteClusterRoleBinding_TransportError(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().DeleteClusterRoleBindingWithResponse(mock.Anything, "rb-1", mock.Anything).Return(nil, fmt.Errorf("timeout"))

	c := newMockClient(m)
	require.ErrorContains(t, c.DeleteClusterRoleBinding(context.Background(), "rb-1"), "failed to delete cluster role binding")
}

// --- ListNamespaceRoles ---

func TestListNamespaceRoles_Success(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().ListNamespaceRolesWithResponse(mock.Anything, "org-a", mock.Anything).Return(&gen.ListNamespaceRolesResp{
		HTTPResponse: httpResp(http.StatusOK),
		JSON200: &gen.AuthzRoleList{
			Items:      []gen.AuthzRole{{Metadata: gen.ObjectMeta{Name: "role-1"}}},
			Pagination: gen.Pagination{},
		},
	}, nil)

	c := newMockClient(m)
	result, err := c.ListNamespaceRoles(context.Background(), "org-a", nil)
	require.NoError(t, err)
	require.Len(t, result.Items, 1)
	assert.Equal(t, "role-1", result.Items[0].Metadata.Name)
}

func TestListNamespaceRoles_TransportError(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().ListNamespaceRolesWithResponse(mock.Anything, "org-a", mock.Anything).Return(nil, fmt.Errorf("connection refused"))

	c := newMockClient(m)
	_, err := c.ListNamespaceRoles(context.Background(), "org-a", nil)
	require.ErrorContains(t, err, "failed to list roles")
}

// --- GetNamespaceRole ---

func TestGetNamespaceRole_Success(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().GetNamespaceRoleWithResponse(mock.Anything, "org-a", "role-1", mock.Anything).Return(&gen.GetNamespaceRoleResp{
		HTTPResponse: httpResp(http.StatusOK),
		JSON200:      &gen.AuthzRole{Metadata: gen.ObjectMeta{Name: "role-1"}},
	}, nil)

	c := newMockClient(m)
	result, err := c.GetNamespaceRole(context.Background(), "org-a", "role-1")
	require.NoError(t, err)
	assert.Equal(t, "role-1", result.Metadata.Name)
}

func TestGetNamespaceRole_TransportError(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().GetNamespaceRoleWithResponse(mock.Anything, "org-a", "role-1", mock.Anything).Return(nil, fmt.Errorf("timeout"))

	c := newMockClient(m)
	_, err := c.GetNamespaceRole(context.Background(), "org-a", "role-1")
	require.ErrorContains(t, err, "failed to get role")
}

// --- DeleteNamespaceRole ---

func TestDeleteNamespaceRole_Success(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().DeleteNamespaceRoleWithResponse(mock.Anything, "org-a", "role-1", mock.Anything).Return(&gen.DeleteNamespaceRoleResp{
		HTTPResponse: httpResp(http.StatusNoContent),
	}, nil)

	c := newMockClient(m)
	require.NoError(t, c.DeleteNamespaceRole(context.Background(), "org-a", "role-1"))
}

func TestDeleteNamespaceRole_TransportError(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().DeleteNamespaceRoleWithResponse(mock.Anything, "org-a", "role-1", mock.Anything).Return(nil, fmt.Errorf("timeout"))

	c := newMockClient(m)
	require.ErrorContains(t, c.DeleteNamespaceRole(context.Background(), "org-a", "role-1"), "failed to delete role")
}

// --- ListNamespaceRoleBindings ---

func TestListNamespaceRoleBindings_Success(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().ListNamespaceRoleBindingsWithResponse(mock.Anything, "org-a", mock.Anything).Return(&gen.ListNamespaceRoleBindingsResp{
		HTTPResponse: httpResp(http.StatusOK),
		JSON200: &gen.AuthzRoleBindingList{
			Items:      []gen.AuthzRoleBinding{{Metadata: gen.ObjectMeta{Name: "rb-1"}}},
			Pagination: gen.Pagination{},
		},
	}, nil)

	c := newMockClient(m)
	result, err := c.ListNamespaceRoleBindings(context.Background(), "org-a", nil)
	require.NoError(t, err)
	require.Len(t, result.Items, 1)
	assert.Equal(t, "rb-1", result.Items[0].Metadata.Name)
}

func TestListNamespaceRoleBindings_TransportError(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().ListNamespaceRoleBindingsWithResponse(mock.Anything, "org-a", mock.Anything).Return(nil, fmt.Errorf("connection refused"))

	c := newMockClient(m)
	_, err := c.ListNamespaceRoleBindings(context.Background(), "org-a", nil)
	require.ErrorContains(t, err, "failed to list role bindings")
}

// --- GetNamespaceRoleBinding ---

func TestGetNamespaceRoleBinding_Success(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().GetNamespaceRoleBindingWithResponse(mock.Anything, "org-a", "rb-1", mock.Anything).Return(&gen.GetNamespaceRoleBindingResp{
		HTTPResponse: httpResp(http.StatusOK),
		JSON200:      &gen.AuthzRoleBinding{Metadata: gen.ObjectMeta{Name: "rb-1"}},
	}, nil)

	c := newMockClient(m)
	result, err := c.GetNamespaceRoleBinding(context.Background(), "org-a", "rb-1")
	require.NoError(t, err)
	assert.Equal(t, "rb-1", result.Metadata.Name)
}

func TestGetNamespaceRoleBinding_TransportError(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().GetNamespaceRoleBindingWithResponse(mock.Anything, "org-a", "rb-1", mock.Anything).Return(nil, fmt.Errorf("timeout"))

	c := newMockClient(m)
	_, err := c.GetNamespaceRoleBinding(context.Background(), "org-a", "rb-1")
	require.ErrorContains(t, err, "failed to get role binding")
}

// --- DeleteNamespaceRoleBinding ---

func TestDeleteNamespaceRoleBinding_Success(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().DeleteNamespaceRoleBindingWithResponse(mock.Anything, "org-a", "rb-1", mock.Anything).Return(&gen.DeleteNamespaceRoleBindingResp{
		HTTPResponse: httpResp(http.StatusNoContent),
	}, nil)

	c := newMockClient(m)
	require.NoError(t, c.DeleteNamespaceRoleBinding(context.Background(), "org-a", "rb-1"))
}

func TestDeleteNamespaceRoleBinding_TransportError(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().DeleteNamespaceRoleBindingWithResponse(mock.Anything, "org-a", "rb-1", mock.Anything).Return(nil, fmt.Errorf("timeout"))

	c := newMockClient(m)
	require.ErrorContains(t, c.DeleteNamespaceRoleBinding(context.Background(), "org-a", "rb-1"), "failed to delete role binding")
}

// --- APIError paths for role/binding methods ---

func TestListClusterRoleBindings_APIError(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().ListClusterRoleBindingsWithResponse(mock.Anything, mock.Anything).Return(&gen.ListClusterRoleBindingsResp{
		HTTPResponse: httpResp(http.StatusUnauthorized),
		Body:         []byte(`{"error":"unauthorized"}`),
	}, nil)

	c := newMockClient(m)
	_, err := c.ListClusterRoleBindings(context.Background(), nil)
	require.ErrorContains(t, err, "unauthorized")
}

func TestGetClusterRoleBinding_APIError(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().GetClusterRoleBindingWithResponse(mock.Anything, "rb-1", mock.Anything).Return(&gen.GetClusterRoleBindingResp{
		HTTPResponse: httpResp(http.StatusNotFound),
		Body:         []byte(`{"error":"not found"}`),
	}, nil)

	c := newMockClient(m)
	_, err := c.GetClusterRoleBinding(context.Background(), "rb-1")
	require.ErrorContains(t, err, "not found")
}

func TestDeleteClusterRoleBinding_APIError(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().DeleteClusterRoleBindingWithResponse(mock.Anything, "rb-1", mock.Anything).Return(&gen.DeleteClusterRoleBindingResp{
		HTTPResponse: httpResp(http.StatusForbidden),
		Body:         []byte(`{"error":"forbidden"}`),
	}, nil)

	c := newMockClient(m)
	require.ErrorContains(t, c.DeleteClusterRoleBinding(context.Background(), "rb-1"), "forbidden")
}

func TestListNamespaceRoles_APIError(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().ListNamespaceRolesWithResponse(mock.Anything, "org-a", mock.Anything).Return(&gen.ListNamespaceRolesResp{
		HTTPResponse: httpResp(http.StatusUnauthorized),
		Body:         []byte(`{"error":"unauthorized"}`),
	}, nil)

	c := newMockClient(m)
	_, err := c.ListNamespaceRoles(context.Background(), "org-a", nil)
	require.ErrorContains(t, err, "unauthorized")
}

func TestGetNamespaceRole_APIError(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().GetNamespaceRoleWithResponse(mock.Anything, "org-a", "role-1", mock.Anything).Return(&gen.GetNamespaceRoleResp{
		HTTPResponse: httpResp(http.StatusNotFound),
		Body:         []byte(`{"error":"not found"}`),
	}, nil)

	c := newMockClient(m)
	_, err := c.GetNamespaceRole(context.Background(), "org-a", "role-1")
	require.ErrorContains(t, err, "not found")
}

func TestDeleteNamespaceRole_APIError(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().DeleteNamespaceRoleWithResponse(mock.Anything, "org-a", "role-1", mock.Anything).Return(&gen.DeleteNamespaceRoleResp{
		HTTPResponse: httpResp(http.StatusForbidden),
		Body:         []byte(`{"error":"forbidden"}`),
	}, nil)

	c := newMockClient(m)
	require.ErrorContains(t, c.DeleteNamespaceRole(context.Background(), "org-a", "role-1"), "forbidden")
}

func TestListNamespaceRoleBindings_APIError(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().ListNamespaceRoleBindingsWithResponse(mock.Anything, "org-a", mock.Anything).Return(&gen.ListNamespaceRoleBindingsResp{
		HTTPResponse: httpResp(http.StatusUnauthorized),
		Body:         []byte(`{"error":"unauthorized"}`),
	}, nil)

	c := newMockClient(m)
	_, err := c.ListNamespaceRoleBindings(context.Background(), "org-a", nil)
	require.ErrorContains(t, err, "unauthorized")
}

func TestGetNamespaceRoleBinding_APIError(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().GetNamespaceRoleBindingWithResponse(mock.Anything, "org-a", "rb-1", mock.Anything).Return(&gen.GetNamespaceRoleBindingResp{
		HTTPResponse: httpResp(http.StatusNotFound),
		Body:         []byte(`{"error":"not found"}`),
	}, nil)

	c := newMockClient(m)
	_, err := c.GetNamespaceRoleBinding(context.Background(), "org-a", "rb-1")
	require.ErrorContains(t, err, "not found")
}

func TestDeleteNamespaceRoleBinding_APIError(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().DeleteNamespaceRoleBindingWithResponse(mock.Anything, "org-a", "rb-1", mock.Anything).Return(&gen.DeleteNamespaceRoleBindingResp{
		HTTPResponse: httpResp(http.StatusForbidden),
		Body:         []byte(`{"error":"forbidden"}`),
	}, nil)

	c := newMockClient(m)
	require.ErrorContains(t, c.DeleteNamespaceRoleBinding(context.Background(), "org-a", "rb-1"), "forbidden")
}

// --- CreateComponentType ---

func TestCreateComponentType_Success(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().CreateComponentTypeWithResponse(mock.Anything, "org-a", mock.Anything, mock.Anything).Return(&gen.CreateComponentTypeResp{
		HTTPResponse: httpResp(http.StatusCreated),
		JSON201:      &gen.ComponentType{Metadata: gen.ObjectMeta{Name: "ct-1"}},
	}, nil)

	c := newMockClient(m)
	result, err := c.CreateComponentType(context.Background(), "org-a", gen.ComponentType{})
	require.NoError(t, err)
	assert.Equal(t, "ct-1", result.Metadata.Name)
}

func TestCreateComponentType_TransportError(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().CreateComponentTypeWithResponse(mock.Anything, "org-a", mock.Anything, mock.Anything).Return(nil, fmt.Errorf("timeout"))

	c := newMockClient(m)
	_, err := c.CreateComponentType(context.Background(), "org-a", gen.ComponentType{})
	require.ErrorContains(t, err, "failed to create component type")
}

func TestCreateComponentType_APIError(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().CreateComponentTypeWithResponse(mock.Anything, "org-a", mock.Anything, mock.Anything).Return(&gen.CreateComponentTypeResp{
		HTTPResponse: httpResp(http.StatusConflict),
		Body:         []byte(`{"error":"already exists"}`),
	}, nil)

	c := newMockClient(m)
	_, err := c.CreateComponentType(context.Background(), "org-a", gen.ComponentType{})
	require.ErrorContains(t, err, "already exists")
}

// --- UpdateComponentType ---

func TestUpdateComponentType_Success(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().UpdateComponentTypeWithResponse(mock.Anything, "org-a", "ct-1", mock.Anything, mock.Anything).Return(&gen.UpdateComponentTypeResp{
		HTTPResponse: httpResp(http.StatusOK),
		JSON200:      &gen.ComponentType{Metadata: gen.ObjectMeta{Name: "ct-1"}},
	}, nil)

	c := newMockClient(m)
	result, err := c.UpdateComponentType(context.Background(), "org-a", "ct-1", gen.ComponentType{})
	require.NoError(t, err)
	assert.Equal(t, "ct-1", result.Metadata.Name)
}

func TestUpdateComponentType_TransportError(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().UpdateComponentTypeWithResponse(mock.Anything, "org-a", "ct-1", mock.Anything, mock.Anything).Return(nil, fmt.Errorf("timeout"))

	c := newMockClient(m)
	_, err := c.UpdateComponentType(context.Background(), "org-a", "ct-1", gen.ComponentType{})
	require.ErrorContains(t, err, "failed to update component type")
}

func TestUpdateComponentType_APIError(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().UpdateComponentTypeWithResponse(mock.Anything, "org-a", "ct-1", mock.Anything, mock.Anything).Return(&gen.UpdateComponentTypeResp{
		HTTPResponse: httpResp(http.StatusNotFound),
		Body:         []byte(`{"error":"not found"}`),
	}, nil)

	c := newMockClient(m)
	_, err := c.UpdateComponentType(context.Background(), "org-a", "ct-1", gen.ComponentType{})
	require.ErrorContains(t, err, "not found")
}

// --- CreateTrait ---

func TestCreateTrait_Success(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().CreateTraitWithResponse(mock.Anything, "org-a", mock.Anything, mock.Anything).Return(&gen.CreateTraitResp{
		HTTPResponse: httpResp(http.StatusCreated),
		JSON201:      &gen.Trait{Metadata: gen.ObjectMeta{Name: "trait-1"}},
	}, nil)

	c := newMockClient(m)
	result, err := c.CreateTrait(context.Background(), "org-a", gen.Trait{})
	require.NoError(t, err)
	assert.Equal(t, "trait-1", result.Metadata.Name)
}

func TestCreateTrait_TransportError(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().CreateTraitWithResponse(mock.Anything, "org-a", mock.Anything, mock.Anything).Return(nil, fmt.Errorf("timeout"))

	c := newMockClient(m)
	_, err := c.CreateTrait(context.Background(), "org-a", gen.Trait{})
	require.ErrorContains(t, err, "failed to create trait")
}

func TestCreateTrait_APIError(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().CreateTraitWithResponse(mock.Anything, "org-a", mock.Anything, mock.Anything).Return(&gen.CreateTraitResp{
		HTTPResponse: httpResp(http.StatusConflict),
		Body:         []byte(`{"error":"already exists"}`),
	}, nil)

	c := newMockClient(m)
	_, err := c.CreateTrait(context.Background(), "org-a", gen.Trait{})
	require.ErrorContains(t, err, "already exists")
}

// --- UpdateTrait ---

func TestUpdateTrait_Success(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().UpdateTraitWithResponse(mock.Anything, "org-a", "trait-1", mock.Anything, mock.Anything).Return(&gen.UpdateTraitResp{
		HTTPResponse: httpResp(http.StatusOK),
		JSON200:      &gen.Trait{Metadata: gen.ObjectMeta{Name: "trait-1"}},
	}, nil)

	c := newMockClient(m)
	result, err := c.UpdateTrait(context.Background(), "org-a", "trait-1", gen.Trait{})
	require.NoError(t, err)
	assert.Equal(t, "trait-1", result.Metadata.Name)
}

func TestUpdateTrait_TransportError(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().UpdateTraitWithResponse(mock.Anything, "org-a", "trait-1", mock.Anything, mock.Anything).Return(nil, fmt.Errorf("timeout"))

	c := newMockClient(m)
	_, err := c.UpdateTrait(context.Background(), "org-a", "trait-1", gen.Trait{})
	require.ErrorContains(t, err, "failed to update trait")
}

func TestUpdateTrait_APIError(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().UpdateTraitWithResponse(mock.Anything, "org-a", "trait-1", mock.Anything, mock.Anything).Return(&gen.UpdateTraitResp{
		HTTPResponse: httpResp(http.StatusNotFound),
		Body:         []byte(`{"error":"not found"}`),
	}, nil)

	c := newMockClient(m)
	_, err := c.UpdateTrait(context.Background(), "org-a", "trait-1", gen.Trait{})
	require.ErrorContains(t, err, "not found")
}

// --- GenerateRelease ---

func TestGenerateRelease_Success(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().GenerateReleaseWithResponse(mock.Anything, "org-a", "comp-1", mock.Anything, mock.Anything).Return(&gen.GenerateReleaseResp{
		HTTPResponse: httpResp(http.StatusCreated),
		JSON201:      &gen.ComponentRelease{Metadata: gen.ObjectMeta{Name: "cr-1"}},
	}, nil)

	c := newMockClient(m)
	result, err := c.GenerateRelease(context.Background(), "org-a", "comp-1", gen.GenerateReleaseRequest{})
	require.NoError(t, err)
	assert.Equal(t, "cr-1", result.Metadata.Name)
}

func TestGenerateRelease_TransportError(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().GenerateReleaseWithResponse(mock.Anything, "org-a", "comp-1", mock.Anything, mock.Anything).Return(nil, fmt.Errorf("timeout"))

	c := newMockClient(m)
	_, err := c.GenerateRelease(context.Background(), "org-a", "comp-1", gen.GenerateReleaseRequest{})
	require.ErrorContains(t, err, "failed to generate release")
}

func TestGenerateRelease_APIError(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().GenerateReleaseWithResponse(mock.Anything, "org-a", "comp-1", mock.Anything, mock.Anything).Return(&gen.GenerateReleaseResp{
		HTTPResponse: httpResp(http.StatusBadRequest),
		Body:         []byte(`{"error":"validation failed"}`),
	}, nil)

	c := newMockClient(m)
	_, err := c.GenerateRelease(context.Background(), "org-a", "comp-1", gen.GenerateReleaseRequest{})
	require.ErrorContains(t, err, "validation failed")
}

// --- CreateReleaseBinding ---

func TestCreateReleaseBinding_Success(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().CreateReleaseBindingWithResponse(mock.Anything, "org-a", mock.Anything, mock.Anything).Return(&gen.CreateReleaseBindingResp{
		HTTPResponse: httpResp(http.StatusCreated),
		JSON201:      &gen.ReleaseBinding{Metadata: gen.ObjectMeta{Name: "rb-1"}},
	}, nil)

	c := newMockClient(m)
	result, err := c.CreateReleaseBinding(context.Background(), "org-a", gen.ReleaseBinding{})
	require.NoError(t, err)
	assert.Equal(t, "rb-1", result.Metadata.Name)
}

func TestCreateReleaseBinding_TransportError(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().CreateReleaseBindingWithResponse(mock.Anything, "org-a", mock.Anything, mock.Anything).Return(nil, fmt.Errorf("timeout"))

	c := newMockClient(m)
	_, err := c.CreateReleaseBinding(context.Background(), "org-a", gen.ReleaseBinding{})
	require.ErrorContains(t, err, "failed to create release binding")
}

func TestCreateReleaseBinding_APIError(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().CreateReleaseBindingWithResponse(mock.Anything, "org-a", mock.Anything, mock.Anything).Return(&gen.CreateReleaseBindingResp{
		HTTPResponse: httpResp(http.StatusConflict),
		Body:         []byte(`{"error":"already exists"}`),
	}, nil)

	c := newMockClient(m)
	_, err := c.CreateReleaseBinding(context.Background(), "org-a", gen.ReleaseBinding{})
	require.ErrorContains(t, err, "already exists")
}

// --- UpdateReleaseBinding ---

func TestUpdateReleaseBinding_Success(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().UpdateReleaseBindingWithResponse(mock.Anything, "org-a", "rb-1", mock.Anything, mock.Anything).Return(&gen.UpdateReleaseBindingResp{
		HTTPResponse: httpResp(http.StatusOK),
		JSON200:      &gen.ReleaseBinding{Metadata: gen.ObjectMeta{Name: "rb-1"}},
	}, nil)

	c := newMockClient(m)
	result, err := c.UpdateReleaseBinding(context.Background(), "org-a", "rb-1", gen.ReleaseBinding{})
	require.NoError(t, err)
	assert.Equal(t, "rb-1", result.Metadata.Name)
}

func TestUpdateReleaseBinding_TransportError(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().UpdateReleaseBindingWithResponse(mock.Anything, "org-a", "rb-1", mock.Anything, mock.Anything).Return(nil, fmt.Errorf("timeout"))

	c := newMockClient(m)
	_, err := c.UpdateReleaseBinding(context.Background(), "org-a", "rb-1", gen.ReleaseBinding{})
	require.ErrorContains(t, err, "failed to update release binding")
}

func TestUpdateReleaseBinding_APIError(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().UpdateReleaseBindingWithResponse(mock.Anything, "org-a", "rb-1", mock.Anything, mock.Anything).Return(&gen.UpdateReleaseBindingResp{
		HTTPResponse: httpResp(http.StatusNotFound),
		Body:         []byte(`{"error":"not found"}`),
	}, nil)

	c := newMockClient(m)
	_, err := c.UpdateReleaseBinding(context.Background(), "org-a", "rb-1", gen.ReleaseBinding{})
	require.ErrorContains(t, err, "not found")
}

// --- CreateComponentRelease ---

func TestCreateComponentRelease_Success(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().CreateComponentReleaseWithResponse(mock.Anything, "org-a", mock.Anything, mock.Anything).Return(&gen.CreateComponentReleaseResp{
		HTTPResponse: httpResp(http.StatusCreated),
		JSON201:      &gen.ComponentRelease{Metadata: gen.ObjectMeta{Name: "cr-1"}},
	}, nil)

	c := newMockClient(m)
	result, err := c.CreateComponentRelease(context.Background(), "org-a", gen.ComponentRelease{})
	require.NoError(t, err)
	assert.Equal(t, "cr-1", result.Metadata.Name)
}

func TestCreateComponentRelease_TransportError(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().CreateComponentReleaseWithResponse(mock.Anything, "org-a", mock.Anything, mock.Anything).Return(nil, fmt.Errorf("timeout"))

	c := newMockClient(m)
	_, err := c.CreateComponentRelease(context.Background(), "org-a", gen.ComponentRelease{})
	require.ErrorContains(t, err, "failed to create component release")
}

func TestCreateComponentRelease_APIError(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().CreateComponentReleaseWithResponse(mock.Anything, "org-a", mock.Anything, mock.Anything).Return(&gen.CreateComponentReleaseResp{
		HTTPResponse: httpResp(http.StatusConflict),
		Body:         []byte(`{"error":"already exists"}`),
	}, nil)

	c := newMockClient(m)
	_, err := c.CreateComponentRelease(context.Background(), "org-a", gen.ComponentRelease{})
	require.ErrorContains(t, err, "already exists")
}

// --- GetProjectDeploymentPipeline ---

func TestGetProjectDeploymentPipeline_Success(t *testing.T) {
	pipelineName := "pipe-1"
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().GetProjectWithResponse(mock.Anything, "org-a", "proj-1", mock.Anything).Return(&gen.GetProjectResp{
		HTTPResponse: httpResp(http.StatusOK),
		JSON200: &gen.Project{
			Metadata: gen.ObjectMeta{Name: "proj-1"},
			Spec: &gen.ProjectSpec{DeploymentPipelineRef: &struct {
				Kind *gen.ProjectSpecDeploymentPipelineRefKind `json:"kind,omitempty"`
				Name string                                    `json:"name"`
			}{Name: pipelineName}},
		},
	}, nil)
	m.EXPECT().GetDeploymentPipelineWithResponse(mock.Anything, "org-a", pipelineName, mock.Anything).Return(&gen.GetDeploymentPipelineResp{
		HTTPResponse: httpResp(http.StatusOK),
		JSON200:      &gen.DeploymentPipeline{Metadata: gen.ObjectMeta{Name: pipelineName}},
	}, nil)

	c := newMockClient(m)
	result, err := c.GetProjectDeploymentPipeline(context.Background(), "org-a", "proj-1")
	require.NoError(t, err)
	assert.Equal(t, pipelineName, result.Metadata.Name)
}

func TestGetProjectDeploymentPipeline_GetProjectError(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().GetProjectWithResponse(mock.Anything, "org-a", "proj-1", mock.Anything).Return(nil, fmt.Errorf("timeout"))

	c := newMockClient(m)
	_, err := c.GetProjectDeploymentPipeline(context.Background(), "org-a", "proj-1")
	require.ErrorContains(t, err, "failed to get project")
}

func TestGetProjectDeploymentPipeline_NoPipelineConfigured(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().GetProjectWithResponse(mock.Anything, "org-a", "proj-1", mock.Anything).Return(&gen.GetProjectResp{
		HTTPResponse: httpResp(http.StatusOK),
		JSON200: &gen.Project{
			Metadata: gen.ObjectMeta{Name: "proj-1"},
			Spec:     &gen.ProjectSpec{},
		},
	}, nil)

	c := newMockClient(m)
	_, err := c.GetProjectDeploymentPipeline(context.Background(), "org-a", "proj-1")
	require.ErrorContains(t, err, "does not have a deployment pipeline configured")
}

func TestGetProjectDeploymentPipeline_GetPipelineError(t *testing.T) {
	pipelineName := "pipe-1"
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().GetProjectWithResponse(mock.Anything, "org-a", "proj-1", mock.Anything).Return(&gen.GetProjectResp{
		HTTPResponse: httpResp(http.StatusOK),
		JSON200: &gen.Project{
			Metadata: gen.ObjectMeta{Name: "proj-1"},
			Spec: &gen.ProjectSpec{DeploymentPipelineRef: &struct {
				Kind *gen.ProjectSpecDeploymentPipelineRefKind `json:"kind,omitempty"`
				Name string                                    `json:"name"`
			}{Name: pipelineName}},
		},
	}, nil)
	m.EXPECT().GetDeploymentPipelineWithResponse(mock.Anything, "org-a", pipelineName, mock.Anything).Return(nil, fmt.Errorf("timeout"))

	c := newMockClient(m)
	_, err := c.GetProjectDeploymentPipeline(context.Background(), "org-a", "proj-1")
	require.ErrorContains(t, err, "failed to get deployment pipeline")
}

// --- Schema methods ---

func TestGetComponentTypeSchema_Success(t *testing.T) {
	schema := gen.SchemaResponse{"type": "object"}
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().GetComponentTypeSchemaWithResponse(mock.Anything, "org-a", "ct-1", mock.Anything).Return(&gen.GetComponentTypeSchemaResp{
		HTTPResponse: httpResp(http.StatusOK),
		JSON200:      &schema,
	}, nil)

	c := newMockClient(m)
	result, err := c.GetComponentTypeSchema(context.Background(), "org-a", "ct-1")
	require.NoError(t, err)
	require.NotNil(t, result)
}

func TestGetComponentTypeSchema_NotFound(t *testing.T) {
	notFound := gen.NotFound{}
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().GetComponentTypeSchemaWithResponse(mock.Anything, "org-a", "ct-missing", mock.Anything).Return(&gen.GetComponentTypeSchemaResp{
		HTTPResponse: httpResp(http.StatusNotFound),
		JSON404:      &notFound,
	}, nil)

	c := newMockClient(m)
	_, err := c.GetComponentTypeSchema(context.Background(), "org-a", "ct-missing")
	require.ErrorContains(t, err, `component type "ct-missing" not found`)
}

func TestGetComponentTypeSchema_TransportError(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().GetComponentTypeSchemaWithResponse(mock.Anything, "org-a", "ct-1", mock.Anything).Return(nil, fmt.Errorf("timeout"))

	c := newMockClient(m)
	_, err := c.GetComponentTypeSchema(context.Background(), "org-a", "ct-1")
	require.ErrorContains(t, err, "failed to get component type schema")
}

func TestGetTraitSchema_Success(t *testing.T) {
	schema := gen.SchemaResponse{"type": "object"}
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().GetTraitSchemaWithResponse(mock.Anything, "org-a", "trait-1", mock.Anything).Return(&gen.GetTraitSchemaResp{
		HTTPResponse: httpResp(http.StatusOK),
		JSON200:      &schema,
	}, nil)

	c := newMockClient(m)
	result, err := c.GetTraitSchema(context.Background(), "org-a", "trait-1")
	require.NoError(t, err)
	require.NotNil(t, result)
}

func TestGetTraitSchema_NotFound(t *testing.T) {
	notFound := gen.NotFound{}
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().GetTraitSchemaWithResponse(mock.Anything, "org-a", "trait-missing", mock.Anything).Return(&gen.GetTraitSchemaResp{
		HTTPResponse: httpResp(http.StatusNotFound),
		JSON404:      &notFound,
	}, nil)

	c := newMockClient(m)
	_, err := c.GetTraitSchema(context.Background(), "org-a", "trait-missing")
	require.ErrorContains(t, err, `trait "trait-missing" not found`)
}

func TestGetTraitSchema_TransportError(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().GetTraitSchemaWithResponse(mock.Anything, "org-a", "trait-1", mock.Anything).Return(nil, fmt.Errorf("timeout"))

	c := newMockClient(m)
	_, err := c.GetTraitSchema(context.Background(), "org-a", "trait-1")
	require.ErrorContains(t, err, "failed to get trait schema")
}

func TestGetWorkflowSchema_Success(t *testing.T) {
	schema := gen.SchemaResponse{"type": "object"}
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().GetWorkflowSchemaWithResponse(mock.Anything, "org-a", "wf-1", mock.Anything).Return(&gen.GetWorkflowSchemaResp{
		HTTPResponse: httpResp(http.StatusOK),
		JSON200:      &schema,
	}, nil)

	c := newMockClient(m)
	result, err := c.GetWorkflowSchema(context.Background(), "org-a", "wf-1")
	require.NoError(t, err)
	require.NotNil(t, result)
}

func TestGetWorkflowSchema_NotFound(t *testing.T) {
	notFound := gen.NotFound{}
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().GetWorkflowSchemaWithResponse(mock.Anything, "org-a", "wf-missing", mock.Anything).Return(&gen.GetWorkflowSchemaResp{
		HTTPResponse: httpResp(http.StatusNotFound),
		JSON404:      &notFound,
	}, nil)

	c := newMockClient(m)
	_, err := c.GetWorkflowSchema(context.Background(), "org-a", "wf-missing")
	require.ErrorContains(t, err, `workflow "wf-missing" not found`)
}

func TestGetWorkflowSchema_TransportError(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().GetWorkflowSchemaWithResponse(mock.Anything, "org-a", "wf-1", mock.Anything).Return(nil, fmt.Errorf("timeout"))

	c := newMockClient(m)
	_, err := c.GetWorkflowSchema(context.Background(), "org-a", "wf-1")
	require.ErrorContains(t, err, "failed to get workflow schema")
}

func TestGetClusterComponentTypeSchema_Success(t *testing.T) {
	schema := gen.SchemaResponse{"type": "object"}
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().GetClusterComponentTypeSchemaWithResponse(mock.Anything, "cct-1", mock.Anything).Return(&gen.GetClusterComponentTypeSchemaResp{
		HTTPResponse: httpResp(http.StatusOK),
		JSON200:      &schema,
	}, nil)

	c := newMockClient(m)
	result, err := c.GetClusterComponentTypeSchema(context.Background(), "cct-1")
	require.NoError(t, err)
	require.NotNil(t, result)
}

func TestGetClusterComponentTypeSchema_NotFound(t *testing.T) {
	notFound := gen.NotFound{}
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().GetClusterComponentTypeSchemaWithResponse(mock.Anything, "cct-missing", mock.Anything).Return(&gen.GetClusterComponentTypeSchemaResp{
		HTTPResponse: httpResp(http.StatusNotFound),
		JSON404:      &notFound,
	}, nil)

	c := newMockClient(m)
	_, err := c.GetClusterComponentTypeSchema(context.Background(), "cct-missing")
	require.ErrorContains(t, err, `cluster component type "cct-missing" not found`)
}

func TestGetClusterComponentTypeSchema_TransportError(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().GetClusterComponentTypeSchemaWithResponse(mock.Anything, "cct-1", mock.Anything).Return(nil, fmt.Errorf("timeout"))

	c := newMockClient(m)
	_, err := c.GetClusterComponentTypeSchema(context.Background(), "cct-1")
	require.ErrorContains(t, err, "failed to get cluster component type schema")
}

func TestGetClusterTraitSchema_Success(t *testing.T) {
	schema := gen.SchemaResponse{"type": "object"}
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().GetClusterTraitSchemaWithResponse(mock.Anything, "ct-1", mock.Anything).Return(&gen.GetClusterTraitSchemaResp{
		HTTPResponse: httpResp(http.StatusOK),
		JSON200:      &schema,
	}, nil)

	c := newMockClient(m)
	result, err := c.GetClusterTraitSchema(context.Background(), "ct-1")
	require.NoError(t, err)
	require.NotNil(t, result)
}

func TestGetClusterTraitSchema_NotFound(t *testing.T) {
	notFound := gen.NotFound{}
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().GetClusterTraitSchemaWithResponse(mock.Anything, "ct-missing", mock.Anything).Return(&gen.GetClusterTraitSchemaResp{
		HTTPResponse: httpResp(http.StatusNotFound),
		JSON404:      &notFound,
	}, nil)

	c := newMockClient(m)
	_, err := c.GetClusterTraitSchema(context.Background(), "ct-missing")
	require.ErrorContains(t, err, `cluster trait "ct-missing" not found`)
}

func TestGetClusterTraitSchema_TransportError(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().GetClusterTraitSchemaWithResponse(mock.Anything, "ct-1", mock.Anything).Return(nil, fmt.Errorf("timeout"))

	c := newMockClient(m)
	_, err := c.GetClusterTraitSchema(context.Background(), "ct-1")
	require.ErrorContains(t, err, "failed to get cluster trait schema")
}

func TestGetClusterWorkflowSchema_Success(t *testing.T) {
	schema := gen.SchemaResponse{"type": "object"}
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().GetClusterWorkflowSchemaWithResponse(mock.Anything, "cwf-1", mock.Anything).Return(&gen.GetClusterWorkflowSchemaResp{
		HTTPResponse: httpResp(http.StatusOK),
		JSON200:      &schema,
	}, nil)

	c := newMockClient(m)
	result, err := c.GetClusterWorkflowSchema(context.Background(), "cwf-1")
	require.NoError(t, err)
	require.NotNil(t, result)
}

func TestGetClusterWorkflowSchema_NotFound(t *testing.T) {
	notFound := gen.NotFound{}
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().GetClusterWorkflowSchemaWithResponse(mock.Anything, "cwf-missing", mock.Anything).Return(&gen.GetClusterWorkflowSchemaResp{
		HTTPResponse: httpResp(http.StatusNotFound),
		JSON404:      &notFound,
	}, nil)

	c := newMockClient(m)
	_, err := c.GetClusterWorkflowSchema(context.Background(), "cwf-missing")
	require.ErrorContains(t, err, `cluster workflow "cwf-missing" not found`)
}

func TestGetClusterWorkflowSchema_TransportError(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().GetClusterWorkflowSchemaWithResponse(mock.Anything, "cwf-1", mock.Anything).Return(nil, fmt.Errorf("timeout"))

	c := newMockClient(m)
	_, err := c.GetClusterWorkflowSchema(context.Background(), "cwf-1")
	require.ErrorContains(t, err, "failed to get cluster workflow schema")
}

// --- ListComponents with project filter ---

func TestListComponents_WithProjectFilter(t *testing.T) {
	m := mocks.NewMockClientWithResponsesInterface(t)
	m.EXPECT().ListComponentsWithResponse(mock.Anything, "org-a", mock.MatchedBy(func(p *gen.ListComponentsParams) bool {
		return p.Project != nil && *p.Project == "proj-1"
	})).Return(&gen.ListComponentsResp{
		HTTPResponse: httpResp(http.StatusOK),
		JSON200: &gen.ComponentList{
			Items:      []gen.Component{{Metadata: gen.ObjectMeta{Name: "comp-1"}}},
			Pagination: gen.Pagination{},
		},
	}, nil)

	c := newMockClient(m)
	result, err := c.ListComponents(context.Background(), "org-a", "proj-1", nil)
	require.NoError(t, err)
	require.Len(t, result.Items, 1)
}
