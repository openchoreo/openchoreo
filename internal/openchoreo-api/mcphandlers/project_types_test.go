// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package mcphandlers

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
	clusterprojecttypemocks "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/clusterprojecttype/mocks"
	projecttypemocks "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/projecttype/mocks"
	"github.com/openchoreo/openchoreo/pkg/mcp/tools"
)

const (
	testProjectTypeName        = "standard-project"
	testClusterProjectTypeName = "platform-project"
	testKeepAnnValue           = "keep"
)

func sampleProjectType() *openchoreov1alpha1.ProjectType {
	return &openchoreov1alpha1.ProjectType{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testProjectTypeName,
			Namespace: testNS,
		},
		Spec: openchoreov1alpha1.ProjectTypeSpec{
			Resources: []openchoreov1alpha1.ResourceTemplate{{ID: "cell-namespace"}},
		},
	}
}

func sampleClusterProjectType() *openchoreov1alpha1.ClusterProjectType {
	return &openchoreov1alpha1.ClusterProjectType{
		ObjectMeta: metav1.ObjectMeta{Name: testClusterProjectTypeName},
		Spec: openchoreov1alpha1.ClusterProjectTypeSpec{
			Resources: []openchoreov1alpha1.ResourceTemplate{{ID: "cell-namespace"}},
		},
	}
}

// genProjectTypeSpec builds a gen.ProjectTypeSpec carrying a single resource with
// the given id, decoded from JSON to avoid depending on the generated anonymous
// struct shape.
func genProjectTypeSpec(t *testing.T, resourceID string) *gen.ProjectTypeSpec {
	t.Helper()
	var spec gen.ProjectTypeSpec
	require.NoError(t, json.Unmarshal(
		[]byte(`{"resources":[{"id":"`+resourceID+`","template":{}}]}`), &spec))
	return &spec
}

// ---------------------------------------------------------------------------
// ProjectType
// ---------------------------------------------------------------------------

func TestListProjectTypes(t *testing.T) {
	ctx := context.Background()

	t.Run("returns wrapped list with pagination cursor", func(t *testing.T) {
		ptSvc := projecttypemocks.NewMockService(t)
		ptSvc.EXPECT().
			ListProjectTypes(mock.Anything, testNS, mock.Anything).
			Return(&services.ListResult[openchoreov1alpha1.ProjectType]{
				Items:      []openchoreov1alpha1.ProjectType{*sampleProjectType()},
				NextCursor: "next-token",
			}, nil)

		h := newTestHandler(withProjectTypeService(ptSvc))
		result, err := h.ListProjectTypes(ctx, testNS, tools.ListOpts{Limit: 10})
		require.NoError(t, err)

		m, ok := result.(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "next-token", m["next_cursor"])
		items, ok := m["project_types"].([]map[string]any)
		require.True(t, ok)
		require.Len(t, items, 1)
		assert.Equal(t, testProjectTypeName, items[0]["name"])
	})

	t.Run("service error propagates", func(t *testing.T) {
		expected := errors.New("list failed")
		ptSvc := projecttypemocks.NewMockService(t)
		ptSvc.EXPECT().
			ListProjectTypes(mock.Anything, testNS, mock.Anything).
			Return(nil, expected)

		h := newTestHandler(withProjectTypeService(ptSvc))
		_, err := h.ListProjectTypes(ctx, testNS, tools.ListOpts{})
		require.ErrorIs(t, err, expected)
	})
}

func TestGetProjectType(t *testing.T) {
	ctx := context.Background()

	t.Run("returns detail map", func(t *testing.T) {
		ptSvc := projecttypemocks.NewMockService(t)
		ptSvc.EXPECT().
			GetProjectType(mock.Anything, testNS, testProjectTypeName).
			Return(sampleProjectType(), nil)

		h := newTestHandler(withProjectTypeService(ptSvc))
		result, err := h.GetProjectType(ctx, testNS, testProjectTypeName)
		require.NoError(t, err)

		m, ok := result.(map[string]any)
		require.True(t, ok)
		assert.Equal(t, testProjectTypeName, m["name"])
		assert.NotNil(t, m["spec"])
	})

	t.Run("service error propagates", func(t *testing.T) {
		expected := errors.New("not found")
		ptSvc := projecttypemocks.NewMockService(t)
		ptSvc.EXPECT().
			GetProjectType(mock.Anything, testNS, testProjectTypeName).
			Return(nil, expected)

		h := newTestHandler(withProjectTypeService(ptSvc))
		_, err := h.GetProjectType(ctx, testNS, testProjectTypeName)
		require.ErrorIs(t, err, expected)
	})
}

func TestGetProjectTypeSchema(t *testing.T) {
	ctx := context.Background()

	t.Run("returns service schema map", func(t *testing.T) {
		schema := map[string]any{"type": "object", "properties": map[string]any{}}
		ptSvc := projecttypemocks.NewMockService(t)
		ptSvc.EXPECT().
			GetProjectTypeSchema(mock.Anything, testNS, testProjectTypeName).
			Return(schema, nil)

		h := newTestHandler(withProjectTypeService(ptSvc))
		result, err := h.GetProjectTypeSchema(ctx, testNS, testProjectTypeName)
		require.NoError(t, err)
		assert.Equal(t, schema, result)
	})

	t.Run("service error propagates", func(t *testing.T) {
		expected := errors.New("schema lookup failed")
		ptSvc := projecttypemocks.NewMockService(t)
		ptSvc.EXPECT().
			GetProjectTypeSchema(mock.Anything, testNS, testProjectTypeName).
			Return(nil, expected)

		h := newTestHandler(withProjectTypeService(ptSvc))
		_, err := h.GetProjectTypeSchema(ctx, testNS, testProjectTypeName)
		require.ErrorIs(t, err, expected)
	})
}

func TestCreateProjectType(t *testing.T) {
	ctx := context.Background()

	t.Run("converts gen spec to CRD spec and cleans annotations", func(t *testing.T) {
		annotations := map[string]string{
			"openchoreo.dev/display-name": "",
			"openchoreo.dev/description":  "Standard project",
			"custom":                      testKeepAnnValue,
		}
		req := &gen.CreateProjectTypeJSONRequestBody{
			Metadata: gen.ObjectMeta{Name: testProjectTypeName, Annotations: &annotations},
			Spec:     genProjectTypeSpec(t, "cell-namespace"),
		}

		ptSvc := projecttypemocks.NewMockService(t)
		ptSvc.EXPECT().
			CreateProjectType(mock.Anything, testNS, mock.MatchedBy(func(pt *openchoreov1alpha1.ProjectType) bool {
				_, hasDisplay := pt.Annotations["openchoreo.dev/display-name"]
				return pt.Name == testProjectTypeName &&
					pt.Namespace == testNS &&
					!hasDisplay &&
					pt.Annotations["openchoreo.dev/description"] == "Standard project" &&
					pt.Annotations["custom"] == testKeepAnnValue &&
					len(pt.Spec.Resources) == 1 &&
					pt.Spec.Resources[0].ID == "cell-namespace"
			})).
			Return(sampleProjectType(), nil)

		h := newTestHandler(withProjectTypeService(ptSvc))
		result, err := h.CreateProjectType(ctx, testNS, req)
		require.NoError(t, err)
		m, ok := result.(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "created", m["action"])
	})

	t.Run("service error propagates", func(t *testing.T) {
		expected := errors.New("create failed")
		ptSvc := projecttypemocks.NewMockService(t)
		ptSvc.EXPECT().
			CreateProjectType(mock.Anything, testNS, mock.Anything).
			Return(nil, expected)

		h := newTestHandler(withProjectTypeService(ptSvc))
		_, err := h.CreateProjectType(ctx, testNS, &gen.CreateProjectTypeJSONRequestBody{
			Metadata: gen.ObjectMeta{Name: testProjectTypeName},
		})
		require.ErrorIs(t, err, expected)
	})

	t.Run("rejects nil body", func(t *testing.T) {
		h := newTestHandler()
		_, err := h.CreateProjectType(ctx, testNS, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "request body is required")
	})
}

func TestUpdateProjectType(t *testing.T) {
	ctx := context.Background()

	t.Run("fetches existing then applies spec and annotation merge", func(t *testing.T) {
		existing := sampleProjectType()
		existing.Annotations = map[string]string{"custom": testKeepAnnValue}
		newAnnotations := map[string]string{"openchoreo.dev/display-name": "Standard"}
		req := &gen.UpdateProjectTypeJSONRequestBody{
			Metadata: gen.ObjectMeta{Name: testProjectTypeName, Annotations: &newAnnotations},
			Spec:     genProjectTypeSpec(t, "cell-namespace-v2"),
		}

		ptSvc := projecttypemocks.NewMockService(t)
		ptSvc.EXPECT().
			GetProjectType(mock.Anything, testNS, testProjectTypeName).
			Return(existing, nil)

		var sent *openchoreov1alpha1.ProjectType
		ptSvc.EXPECT().
			UpdateProjectType(mock.Anything, testNS, mock.Anything).
			Run(func(_ context.Context, _ string, pt *openchoreov1alpha1.ProjectType) {
				sent = pt
			}).
			Return(existing, nil)

		h := newTestHandler(withProjectTypeService(ptSvc))
		result, err := h.UpdateProjectType(ctx, testNS, req)
		require.NoError(t, err)

		require.NotNil(t, sent)
		assert.Equal(t, testKeepAnnValue, sent.Annotations["custom"])
		assert.Equal(t, "Standard", sent.Annotations["openchoreo.dev/display-name"])
		require.Len(t, sent.Spec.Resources, 1)
		assert.Equal(t, "cell-namespace-v2", sent.Spec.Resources[0].ID)

		m, ok := result.(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "updated", m["action"])
	})

	t.Run("propagates get error before attempting update", func(t *testing.T) {
		expected := errors.New("missing")
		ptSvc := projecttypemocks.NewMockService(t)
		ptSvc.EXPECT().
			GetProjectType(mock.Anything, testNS, testProjectTypeName).
			Return(nil, expected)

		h := newTestHandler(withProjectTypeService(ptSvc))
		_, err := h.UpdateProjectType(ctx, testNS, &gen.UpdateProjectTypeJSONRequestBody{
			Metadata: gen.ObjectMeta{Name: testProjectTypeName},
		})
		require.ErrorIs(t, err, expected)
	})

	t.Run("rejects nil body", func(t *testing.T) {
		h := newTestHandler()
		_, err := h.UpdateProjectType(ctx, testNS, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "request body is required")
	})
}

func TestDeleteProjectType(t *testing.T) {
	ctx := context.Background()

	t.Run("returns action deleted", func(t *testing.T) {
		ptSvc := projecttypemocks.NewMockService(t)
		ptSvc.EXPECT().
			DeleteProjectType(mock.Anything, testNS, testProjectTypeName).
			Return(nil)

		h := newTestHandler(withProjectTypeService(ptSvc))
		result, err := h.DeleteProjectType(ctx, testNS, testProjectTypeName)
		require.NoError(t, err)
		m, ok := result.(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "deleted", m["action"])
		assert.Equal(t, testProjectTypeName, m["name"])
		assert.Equal(t, testNS, m["namespace"])
	})

	t.Run("service error propagates", func(t *testing.T) {
		expected := errors.New("delete failed")
		ptSvc := projecttypemocks.NewMockService(t)
		ptSvc.EXPECT().
			DeleteProjectType(mock.Anything, testNS, testProjectTypeName).
			Return(expected)

		h := newTestHandler(withProjectTypeService(ptSvc))
		_, err := h.DeleteProjectType(ctx, testNS, testProjectTypeName)
		require.ErrorIs(t, err, expected)
	})
}

// ---------------------------------------------------------------------------
// ClusterProjectType
// ---------------------------------------------------------------------------

func TestListClusterProjectTypes(t *testing.T) {
	ctx := context.Background()

	t.Run("returns wrapped list", func(t *testing.T) {
		cptSvc := clusterprojecttypemocks.NewMockService(t)
		cptSvc.EXPECT().
			ListClusterProjectTypes(mock.Anything, mock.Anything).
			Return(&services.ListResult[openchoreov1alpha1.ClusterProjectType]{
				Items: []openchoreov1alpha1.ClusterProjectType{*sampleClusterProjectType()},
			}, nil)

		h := newTestHandler(withClusterProjectTypeService(cptSvc))
		result, err := h.ListClusterProjectTypes(ctx, tools.ListOpts{})
		require.NoError(t, err)
		m, ok := result.(map[string]any)
		require.True(t, ok)
		items, ok := m["cluster_project_types"].([]map[string]any)
		require.True(t, ok)
		require.Len(t, items, 1)
		assert.Equal(t, testClusterProjectTypeName, items[0]["name"])
	})

	t.Run("service error propagates", func(t *testing.T) {
		expected := errors.New("list failed")
		cptSvc := clusterprojecttypemocks.NewMockService(t)
		cptSvc.EXPECT().
			ListClusterProjectTypes(mock.Anything, mock.Anything).
			Return(nil, expected)

		h := newTestHandler(withClusterProjectTypeService(cptSvc))
		_, err := h.ListClusterProjectTypes(ctx, tools.ListOpts{})
		require.ErrorIs(t, err, expected)
	})
}

func TestGetClusterProjectType(t *testing.T) {
	ctx := context.Background()

	t.Run("returns detail map", func(t *testing.T) {
		cptSvc := clusterprojecttypemocks.NewMockService(t)
		cptSvc.EXPECT().
			GetClusterProjectType(mock.Anything, testClusterProjectTypeName).
			Return(sampleClusterProjectType(), nil)

		h := newTestHandler(withClusterProjectTypeService(cptSvc))
		result, err := h.GetClusterProjectType(ctx, testClusterProjectTypeName)
		require.NoError(t, err)
		m, ok := result.(map[string]any)
		require.True(t, ok)
		assert.Equal(t, testClusterProjectTypeName, m["name"])
	})

	t.Run("service error propagates", func(t *testing.T) {
		expected := errors.New("missing")
		cptSvc := clusterprojecttypemocks.NewMockService(t)
		cptSvc.EXPECT().
			GetClusterProjectType(mock.Anything, testClusterProjectTypeName).
			Return(nil, expected)

		h := newTestHandler(withClusterProjectTypeService(cptSvc))
		_, err := h.GetClusterProjectType(ctx, testClusterProjectTypeName)
		require.ErrorIs(t, err, expected)
	})
}

func TestGetClusterProjectTypeSchema(t *testing.T) {
	ctx := context.Background()

	t.Run("returns service schema map", func(t *testing.T) {
		schema := map[string]any{"type": "object"}
		cptSvc := clusterprojecttypemocks.NewMockService(t)
		cptSvc.EXPECT().
			GetClusterProjectTypeSchema(mock.Anything, testClusterProjectTypeName).
			Return(schema, nil)

		h := newTestHandler(withClusterProjectTypeService(cptSvc))
		result, err := h.GetClusterProjectTypeSchema(ctx, testClusterProjectTypeName)
		require.NoError(t, err)
		assert.Equal(t, schema, result)
	})

	t.Run("service error propagates", func(t *testing.T) {
		expected := errors.New("schema lookup failed")
		cptSvc := clusterprojecttypemocks.NewMockService(t)
		cptSvc.EXPECT().
			GetClusterProjectTypeSchema(mock.Anything, testClusterProjectTypeName).
			Return(nil, expected)

		h := newTestHandler(withClusterProjectTypeService(cptSvc))
		_, err := h.GetClusterProjectTypeSchema(ctx, testClusterProjectTypeName)
		require.ErrorIs(t, err, expected)
	})
}

func TestCreateClusterProjectType(t *testing.T) {
	ctx := context.Background()

	t.Run("converts gen spec to ClusterProjectTypeSpec and cleans annotations", func(t *testing.T) {
		annotations := map[string]string{
			"openchoreo.dev/description": "",
			"custom":                     testKeepAnnValue,
		}
		req := &gen.CreateClusterProjectTypeJSONRequestBody{
			Metadata: gen.ObjectMeta{Name: testClusterProjectTypeName, Annotations: &annotations},
			Spec:     genProjectTypeSpec(t, "cell-namespace"),
		}

		cptSvc := clusterprojecttypemocks.NewMockService(t)
		cptSvc.EXPECT().
			CreateClusterProjectType(mock.Anything, mock.MatchedBy(func(cpt *openchoreov1alpha1.ClusterProjectType) bool {
				_, hasDesc := cpt.Annotations["openchoreo.dev/description"]
				return cpt.Name == testClusterProjectTypeName &&
					cpt.Namespace == "" &&
					!hasDesc &&
					cpt.Annotations["custom"] == testKeepAnnValue &&
					len(cpt.Spec.Resources) == 1 &&
					cpt.Spec.Resources[0].ID == "cell-namespace"
			})).
			Return(sampleClusterProjectType(), nil)

		h := newTestHandler(withClusterProjectTypeService(cptSvc))
		result, err := h.CreateClusterProjectType(ctx, req)
		require.NoError(t, err)
		m, ok := result.(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "created", m["action"])
	})

	t.Run("rejects nil body", func(t *testing.T) {
		h := newTestHandler()
		_, err := h.CreateClusterProjectType(ctx, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "request body is required")
	})
}

func TestUpdateClusterProjectType(t *testing.T) {
	ctx := context.Background()

	t.Run("fetches existing then applies spec and annotation merge", func(t *testing.T) {
		existing := sampleClusterProjectType()
		existing.Annotations = map[string]string{"custom": testKeepAnnValue}
		newAnnotations := map[string]string{"openchoreo.dev/display-name": "Platform Project"}
		req := &gen.UpdateClusterProjectTypeJSONRequestBody{
			Metadata: gen.ObjectMeta{Name: testClusterProjectTypeName, Annotations: &newAnnotations},
			Spec:     genProjectTypeSpec(t, "cell-namespace-v2"),
		}

		cptSvc := clusterprojecttypemocks.NewMockService(t)
		cptSvc.EXPECT().
			GetClusterProjectType(mock.Anything, testClusterProjectTypeName).
			Return(existing, nil)

		var sent *openchoreov1alpha1.ClusterProjectType
		cptSvc.EXPECT().
			UpdateClusterProjectType(mock.Anything, mock.Anything).
			Run(func(_ context.Context, cpt *openchoreov1alpha1.ClusterProjectType) {
				sent = cpt
			}).
			Return(existing, nil)

		h := newTestHandler(withClusterProjectTypeService(cptSvc))
		result, err := h.UpdateClusterProjectType(ctx, req)
		require.NoError(t, err)

		require.NotNil(t, sent)
		assert.Equal(t, testKeepAnnValue, sent.Annotations["custom"])
		assert.Equal(t, "Platform Project", sent.Annotations["openchoreo.dev/display-name"])
		require.Len(t, sent.Spec.Resources, 1)
		assert.Equal(t, "cell-namespace-v2", sent.Spec.Resources[0].ID)

		m, ok := result.(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "updated", m["action"])
	})

	t.Run("rejects nil body", func(t *testing.T) {
		h := newTestHandler()
		_, err := h.UpdateClusterProjectType(ctx, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "request body is required")
	})
}

func TestDeleteClusterProjectType(t *testing.T) {
	ctx := context.Background()

	t.Run("returns action deleted", func(t *testing.T) {
		cptSvc := clusterprojecttypemocks.NewMockService(t)
		cptSvc.EXPECT().
			DeleteClusterProjectType(mock.Anything, testClusterProjectTypeName).
			Return(nil)

		h := newTestHandler(withClusterProjectTypeService(cptSvc))
		result, err := h.DeleteClusterProjectType(ctx, testClusterProjectTypeName)
		require.NoError(t, err)
		m, ok := result.(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "deleted", m["action"])
		assert.Equal(t, testClusterProjectTypeName, m["name"])
	})

	t.Run("service error propagates", func(t *testing.T) {
		expected := errors.New("delete failed")
		cptSvc := clusterprojecttypemocks.NewMockService(t)
		cptSvc.EXPECT().
			DeleteClusterProjectType(mock.Anything, testClusterProjectTypeName).
			Return(expected)

		h := newTestHandler(withClusterProjectTypeService(cptSvc))
		_, err := h.DeleteClusterProjectType(ctx, testClusterProjectTypeName)
		require.ErrorIs(t, err, expected)
	})
}
