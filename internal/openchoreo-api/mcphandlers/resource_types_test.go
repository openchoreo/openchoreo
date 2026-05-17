// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package mcphandlers

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
	clusterresourcetypemocks "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/clusterresourcetype/mocks"
	resourcetypemocks "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/resourcetype/mocks"
	"github.com/openchoreo/openchoreo/pkg/mcp/tools"
)

const (
	testResourceTypeName        = "postgres"
	testClusterResourceTypeName = "managed-redis"
)

func sampleResourceType() *openchoreov1alpha1.ResourceType {
	return &openchoreov1alpha1.ResourceType{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testResourceTypeName,
			Namespace: testNS,
		},
		Spec: openchoreov1alpha1.ResourceTypeSpec{
			RetainPolicy: openchoreov1alpha1.ResourceRetainPolicy("Delete"),
			Resources: []openchoreov1alpha1.ResourceTypeManifest{{
				ID: "claim",
			}},
		},
	}
}

func sampleClusterResourceType() *openchoreov1alpha1.ClusterResourceType {
	return &openchoreov1alpha1.ClusterResourceType{
		ObjectMeta: metav1.ObjectMeta{Name: testClusterResourceTypeName},
		Spec: openchoreov1alpha1.ClusterResourceTypeSpec{
			RetainPolicy: openchoreov1alpha1.ResourceRetainPolicy("Retain"),
			Resources: []openchoreov1alpha1.ResourceTypeManifest{{
				ID: "claim",
			}},
		},
	}
}

// ---------------------------------------------------------------------------
// ResourceType
// ---------------------------------------------------------------------------

func TestListResourceTypes(t *testing.T) {
	ctx := context.Background()

	t.Run("returns wrapped list with pagination cursor", func(t *testing.T) {
		rtSvc := resourcetypemocks.NewMockService(t)
		rtSvc.EXPECT().
			ListResourceTypes(mock.Anything, testNS, mock.Anything).
			Return(&services.ListResult[openchoreov1alpha1.ResourceType]{
				Items:      []openchoreov1alpha1.ResourceType{*sampleResourceType()},
				NextCursor: "next-token",
			}, nil)

		h := newTestHandler(withResourceTypeService(rtSvc))
		result, err := h.ListResourceTypes(ctx, testNS, tools.ListOpts{Limit: 10})
		require.NoError(t, err)

		m, ok := result.(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "next-token", m["next_cursor"])
		items, ok := m["resource_types"].([]map[string]any)
		require.True(t, ok)
		require.Len(t, items, 1)
		assert.Equal(t, testResourceTypeName, items[0]["name"])
	})

	t.Run("service error propagates", func(t *testing.T) {
		expected := errors.New("list failed")
		rtSvc := resourcetypemocks.NewMockService(t)
		rtSvc.EXPECT().
			ListResourceTypes(mock.Anything, testNS, mock.Anything).
			Return(nil, expected)

		h := newTestHandler(withResourceTypeService(rtSvc))
		_, err := h.ListResourceTypes(ctx, testNS, tools.ListOpts{})
		require.ErrorIs(t, err, expected)
	})
}

func TestGetResourceType(t *testing.T) {
	ctx := context.Background()

	t.Run("returns detail map", func(t *testing.T) {
		rtSvc := resourcetypemocks.NewMockService(t)
		rtSvc.EXPECT().
			GetResourceType(mock.Anything, testNS, testResourceTypeName).
			Return(sampleResourceType(), nil)

		h := newTestHandler(withResourceTypeService(rtSvc))
		result, err := h.GetResourceType(ctx, testNS, testResourceTypeName)
		require.NoError(t, err)

		m, ok := result.(map[string]any)
		require.True(t, ok)
		assert.Equal(t, testResourceTypeName, m["name"])
		assert.NotNil(t, m["spec"])
	})

	t.Run("service error propagates", func(t *testing.T) {
		expected := errors.New("not found")
		rtSvc := resourcetypemocks.NewMockService(t)
		rtSvc.EXPECT().
			GetResourceType(mock.Anything, testNS, testResourceTypeName).
			Return(nil, expected)

		h := newTestHandler(withResourceTypeService(rtSvc))
		_, err := h.GetResourceType(ctx, testNS, testResourceTypeName)
		require.ErrorIs(t, err, expected)
	})
}

func TestGetResourceTypeSchema(t *testing.T) {
	ctx := context.Background()

	t.Run("returns service schema map", func(t *testing.T) {
		schema := map[string]any{"type": "object", "properties": map[string]any{}}
		rtSvc := resourcetypemocks.NewMockService(t)
		rtSvc.EXPECT().
			GetResourceTypeSchema(mock.Anything, testNS, testResourceTypeName).
			Return(schema, nil)

		h := newTestHandler(withResourceTypeService(rtSvc))
		result, err := h.GetResourceTypeSchema(ctx, testNS, testResourceTypeName)
		require.NoError(t, err)
		assert.Equal(t, schema, result)
	})

	t.Run("service error propagates", func(t *testing.T) {
		expected := errors.New("schema lookup failed")
		rtSvc := resourcetypemocks.NewMockService(t)
		rtSvc.EXPECT().
			GetResourceTypeSchema(mock.Anything, testNS, testResourceTypeName).
			Return(nil, expected)

		h := newTestHandler(withResourceTypeService(rtSvc))
		_, err := h.GetResourceTypeSchema(ctx, testNS, testResourceTypeName)
		require.ErrorIs(t, err, expected)
	})
}

func TestCreateResourceType(t *testing.T) {
	ctx := context.Background()

	t.Run("converts gen spec to CRD spec and cleans annotations", func(t *testing.T) {
		annotations := map[string]string{
			"openchoreo.dev/display-name": "",
			"openchoreo.dev/description":  "Postgres template",
			"custom":                      "keep",
		}
		retain := gen.ResourceTypeSpecRetainPolicyRetain
		req := &gen.CreateResourceTypeJSONRequestBody{
			Metadata: gen.ObjectMeta{Name: testResourceTypeName, Annotations: &annotations},
			Spec: &gen.ResourceTypeSpec{
				RetainPolicy: &retain,
				Resources: []gen.ResourceTypeManifest{{
					Id: "claim",
				}},
			},
		}

		rtSvc := resourcetypemocks.NewMockService(t)
		rtSvc.EXPECT().
			CreateResourceType(mock.Anything, testNS, mock.MatchedBy(func(rt *openchoreov1alpha1.ResourceType) bool {
				_, hasDisplay := rt.Annotations["openchoreo.dev/display-name"]
				return rt.Name == testResourceTypeName &&
					rt.Namespace == testNS &&
					!hasDisplay &&
					rt.Annotations["openchoreo.dev/description"] == "Postgres template" &&
					rt.Annotations["custom"] == "keep" &&
					rt.Spec.RetainPolicy == openchoreov1alpha1.ResourceRetainPolicy("Retain") &&
					len(rt.Spec.Resources) == 1 &&
					rt.Spec.Resources[0].ID == "claim"
			})).
			Return(sampleResourceType(), nil)

		h := newTestHandler(withResourceTypeService(rtSvc))
		result, err := h.CreateResourceType(ctx, testNS, req)
		require.NoError(t, err)
		m, ok := result.(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "created", m["action"])
	})

	t.Run("service error propagates", func(t *testing.T) {
		expected := errors.New("create failed")
		rtSvc := resourcetypemocks.NewMockService(t)
		rtSvc.EXPECT().
			CreateResourceType(mock.Anything, testNS, mock.Anything).
			Return(nil, expected)

		h := newTestHandler(withResourceTypeService(rtSvc))
		_, err := h.CreateResourceType(ctx, testNS, &gen.CreateResourceTypeJSONRequestBody{
			Metadata: gen.ObjectMeta{Name: testResourceTypeName},
		})
		require.ErrorIs(t, err, expected)
	})

	t.Run("rejects nil body", func(t *testing.T) {
		h := newTestHandler()
		_, err := h.CreateResourceType(ctx, testNS, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "request body is required")
	})
}

func TestUpdateResourceType(t *testing.T) {
	ctx := context.Background()

	t.Run("fetches existing then applies spec and annotation merge", func(t *testing.T) {
		existing := sampleResourceType()
		existing.Annotations = map[string]string{"custom": "keep"}
		newAnnotations := map[string]string{"openchoreo.dev/display-name": "Postgres"}
		req := &gen.UpdateResourceTypeJSONRequestBody{
			Metadata: gen.ObjectMeta{Name: testResourceTypeName, Annotations: &newAnnotations},
			Spec: &gen.ResourceTypeSpec{
				Resources: []gen.ResourceTypeManifest{{Id: "claim-v2"}},
			},
		}

		rtSvc := resourcetypemocks.NewMockService(t)
		rtSvc.EXPECT().
			GetResourceType(mock.Anything, testNS, testResourceTypeName).
			Return(existing, nil)

		var sent *openchoreov1alpha1.ResourceType
		rtSvc.EXPECT().
			UpdateResourceType(mock.Anything, testNS, mock.Anything).
			Run(func(_ context.Context, _ string, rt *openchoreov1alpha1.ResourceType) {
				sent = rt
			}).
			Return(existing, nil)

		h := newTestHandler(withResourceTypeService(rtSvc))
		result, err := h.UpdateResourceType(ctx, testNS, req)
		require.NoError(t, err)

		require.NotNil(t, sent)
		assert.Equal(t, "keep", sent.Annotations["custom"])
		assert.Equal(t, "Postgres", sent.Annotations["openchoreo.dev/display-name"])
		require.Len(t, sent.Spec.Resources, 1)
		assert.Equal(t, "claim-v2", sent.Spec.Resources[0].ID)

		m, ok := result.(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "updated", m["action"])
	})

	t.Run("propagates get error before attempting update", func(t *testing.T) {
		expected := errors.New("missing")
		rtSvc := resourcetypemocks.NewMockService(t)
		rtSvc.EXPECT().
			GetResourceType(mock.Anything, testNS, testResourceTypeName).
			Return(nil, expected)

		h := newTestHandler(withResourceTypeService(rtSvc))
		_, err := h.UpdateResourceType(ctx, testNS, &gen.UpdateResourceTypeJSONRequestBody{
			Metadata: gen.ObjectMeta{Name: testResourceTypeName},
		})
		require.ErrorIs(t, err, expected)
	})

	t.Run("propagates update error", func(t *testing.T) {
		expected := errors.New("conflict")
		rtSvc := resourcetypemocks.NewMockService(t)
		rtSvc.EXPECT().
			GetResourceType(mock.Anything, testNS, testResourceTypeName).
			Return(sampleResourceType(), nil)
		rtSvc.EXPECT().
			UpdateResourceType(mock.Anything, testNS, mock.Anything).
			Return(nil, expected)

		h := newTestHandler(withResourceTypeService(rtSvc))
		_, err := h.UpdateResourceType(ctx, testNS, &gen.UpdateResourceTypeJSONRequestBody{
			Metadata: gen.ObjectMeta{Name: testResourceTypeName},
		})
		require.ErrorIs(t, err, expected)
	})

	t.Run("rejects nil body", func(t *testing.T) {
		h := newTestHandler()
		_, err := h.UpdateResourceType(ctx, testNS, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "request body is required")
	})

	t.Run("strips empty display-name and description on merge", func(t *testing.T) {
		existing := sampleResourceType()
		existing.Annotations = map[string]string{
			"openchoreo.dev/display-name": "Existing Name",
			"keep-me":                     "value",
		}
		incoming := map[string]string{
			"openchoreo.dev/display-name": "",
			"openchoreo.dev/description":  "",
			"new-key":                     "new-value",
		}

		rtSvc := resourcetypemocks.NewMockService(t)
		rtSvc.EXPECT().
			GetResourceType(mock.Anything, testNS, testResourceTypeName).
			Return(existing, nil)

		var sent *openchoreov1alpha1.ResourceType
		rtSvc.EXPECT().
			UpdateResourceType(mock.Anything, testNS, mock.Anything).
			Run(func(_ context.Context, _ string, rt *openchoreov1alpha1.ResourceType) {
				sent = rt
			}).
			Return(existing, nil)

		h := newTestHandler(withResourceTypeService(rtSvc))
		_, err := h.UpdateResourceType(ctx, testNS, &gen.UpdateResourceTypeJSONRequestBody{
			Metadata: gen.ObjectMeta{Name: testResourceTypeName, Annotations: &incoming},
		})
		require.NoError(t, err)
		require.NotNil(t, sent)
		assert.Equal(t, "Existing Name", sent.Annotations["openchoreo.dev/display-name"],
			"empty display-name in incoming must not overwrite existing value")
		_, hasDesc := sent.Annotations["openchoreo.dev/description"]
		assert.False(t, hasDesc, "empty description annotation must be stripped, not added")
		assert.Equal(t, "value", sent.Annotations["keep-me"])
		assert.Equal(t, "new-value", sent.Annotations["new-key"])
	})
}

func TestDeleteResourceType(t *testing.T) {
	ctx := context.Background()

	t.Run("returns action deleted", func(t *testing.T) {
		rtSvc := resourcetypemocks.NewMockService(t)
		rtSvc.EXPECT().
			DeleteResourceType(mock.Anything, testNS, testResourceTypeName).
			Return(nil)

		h := newTestHandler(withResourceTypeService(rtSvc))
		result, err := h.DeleteResourceType(ctx, testNS, testResourceTypeName)
		require.NoError(t, err)
		m, ok := result.(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "deleted", m["action"])
		assert.Equal(t, testResourceTypeName, m["name"])
		assert.Equal(t, testNS, m["namespace"])
	})

	t.Run("service error propagates", func(t *testing.T) {
		expected := errors.New("delete failed")
		rtSvc := resourcetypemocks.NewMockService(t)
		rtSvc.EXPECT().
			DeleteResourceType(mock.Anything, testNS, testResourceTypeName).
			Return(expected)

		h := newTestHandler(withResourceTypeService(rtSvc))
		_, err := h.DeleteResourceType(ctx, testNS, testResourceTypeName)
		require.ErrorIs(t, err, expected)
	})
}

// ---------------------------------------------------------------------------
// ClusterResourceType
// ---------------------------------------------------------------------------

func TestListClusterResourceTypes(t *testing.T) {
	ctx := context.Background()

	t.Run("returns wrapped list", func(t *testing.T) {
		crtSvc := clusterresourcetypemocks.NewMockService(t)
		crtSvc.EXPECT().
			ListClusterResourceTypes(mock.Anything, mock.Anything).
			Return(&services.ListResult[openchoreov1alpha1.ClusterResourceType]{
				Items: []openchoreov1alpha1.ClusterResourceType{*sampleClusterResourceType()},
			}, nil)

		h := newTestHandler(withClusterResourceTypeService(crtSvc))
		result, err := h.ListClusterResourceTypes(ctx, tools.ListOpts{})
		require.NoError(t, err)
		m, ok := result.(map[string]any)
		require.True(t, ok)
		items, ok := m["cluster_resource_types"].([]map[string]any)
		require.True(t, ok)
		require.Len(t, items, 1)
		assert.Equal(t, testClusterResourceTypeName, items[0]["name"])
	})

	t.Run("service error propagates", func(t *testing.T) {
		expected := errors.New("list failed")
		crtSvc := clusterresourcetypemocks.NewMockService(t)
		crtSvc.EXPECT().
			ListClusterResourceTypes(mock.Anything, mock.Anything).
			Return(nil, expected)

		h := newTestHandler(withClusterResourceTypeService(crtSvc))
		_, err := h.ListClusterResourceTypes(ctx, tools.ListOpts{})
		require.ErrorIs(t, err, expected)
	})
}

func TestGetClusterResourceType(t *testing.T) {
	ctx := context.Background()

	t.Run("returns detail map", func(t *testing.T) {
		crtSvc := clusterresourcetypemocks.NewMockService(t)
		crtSvc.EXPECT().
			GetClusterResourceType(mock.Anything, testClusterResourceTypeName).
			Return(sampleClusterResourceType(), nil)

		h := newTestHandler(withClusterResourceTypeService(crtSvc))
		result, err := h.GetClusterResourceType(ctx, testClusterResourceTypeName)
		require.NoError(t, err)
		m, ok := result.(map[string]any)
		require.True(t, ok)
		assert.Equal(t, testClusterResourceTypeName, m["name"])
	})

	t.Run("service error propagates", func(t *testing.T) {
		expected := errors.New("missing")
		crtSvc := clusterresourcetypemocks.NewMockService(t)
		crtSvc.EXPECT().
			GetClusterResourceType(mock.Anything, testClusterResourceTypeName).
			Return(nil, expected)

		h := newTestHandler(withClusterResourceTypeService(crtSvc))
		_, err := h.GetClusterResourceType(ctx, testClusterResourceTypeName)
		require.ErrorIs(t, err, expected)
	})
}

func TestGetClusterResourceTypeSchema(t *testing.T) {
	ctx := context.Background()

	t.Run("returns service schema map", func(t *testing.T) {
		schema := map[string]any{"type": "object"}
		crtSvc := clusterresourcetypemocks.NewMockService(t)
		crtSvc.EXPECT().
			GetClusterResourceTypeSchema(mock.Anything, testClusterResourceTypeName).
			Return(schema, nil)

		h := newTestHandler(withClusterResourceTypeService(crtSvc))
		result, err := h.GetClusterResourceTypeSchema(ctx, testClusterResourceTypeName)
		require.NoError(t, err)
		assert.Equal(t, schema, result)
	})

	t.Run("service error propagates", func(t *testing.T) {
		expected := errors.New("schema lookup failed")
		crtSvc := clusterresourcetypemocks.NewMockService(t)
		crtSvc.EXPECT().
			GetClusterResourceTypeSchema(mock.Anything, testClusterResourceTypeName).
			Return(nil, expected)

		h := newTestHandler(withClusterResourceTypeService(crtSvc))
		_, err := h.GetClusterResourceTypeSchema(ctx, testClusterResourceTypeName)
		require.ErrorIs(t, err, expected)
	})
}

func TestCreateClusterResourceType(t *testing.T) {
	ctx := context.Background()

	t.Run("converts gen spec to ClusterResourceTypeSpec and cleans annotations", func(t *testing.T) {
		annotations := map[string]string{
			"openchoreo.dev/description": "",
			"custom":                     "keep",
		}
		req := &gen.CreateClusterResourceTypeJSONRequestBody{
			Metadata: gen.ObjectMeta{Name: testClusterResourceTypeName, Annotations: &annotations},
			Spec: &gen.ResourceTypeSpec{
				Resources: []gen.ResourceTypeManifest{{Id: "claim"}},
			},
		}

		crtSvc := clusterresourcetypemocks.NewMockService(t)
		crtSvc.EXPECT().
			CreateClusterResourceType(mock.Anything, mock.MatchedBy(func(crt *openchoreov1alpha1.ClusterResourceType) bool {
				_, hasDesc := crt.Annotations["openchoreo.dev/description"]
				return crt.Name == testClusterResourceTypeName &&
					crt.Namespace == "" &&
					!hasDesc &&
					crt.Annotations["custom"] == "keep" &&
					len(crt.Spec.Resources) == 1 &&
					crt.Spec.Resources[0].ID == "claim"
			})).
			Return(sampleClusterResourceType(), nil)

		h := newTestHandler(withClusterResourceTypeService(crtSvc))
		result, err := h.CreateClusterResourceType(ctx, req)
		require.NoError(t, err)
		m, ok := result.(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "created", m["action"])
	})

	t.Run("service error propagates", func(t *testing.T) {
		expected := errors.New("conflict")
		crtSvc := clusterresourcetypemocks.NewMockService(t)
		crtSvc.EXPECT().
			CreateClusterResourceType(mock.Anything, mock.Anything).
			Return(nil, expected)

		h := newTestHandler(withClusterResourceTypeService(crtSvc))
		_, err := h.CreateClusterResourceType(ctx, &gen.CreateClusterResourceTypeJSONRequestBody{
			Metadata: gen.ObjectMeta{Name: testClusterResourceTypeName},
		})
		require.ErrorIs(t, err, expected)
	})

	t.Run("rejects nil body", func(t *testing.T) {
		h := newTestHandler()
		_, err := h.CreateClusterResourceType(ctx, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "request body is required")
	})
}

func TestUpdateClusterResourceType(t *testing.T) {
	ctx := context.Background()

	t.Run("fetches existing then applies spec and annotation merge", func(t *testing.T) {
		existing := sampleClusterResourceType()
		existing.Annotations = map[string]string{"custom": "keep"}
		newAnnotations := map[string]string{"openchoreo.dev/display-name": "Managed Redis"}
		req := &gen.UpdateClusterResourceTypeJSONRequestBody{
			Metadata: gen.ObjectMeta{Name: testClusterResourceTypeName, Annotations: &newAnnotations},
			Spec: &gen.ResourceTypeSpec{
				Resources: []gen.ResourceTypeManifest{{Id: "claim-v2"}},
			},
		}

		crtSvc := clusterresourcetypemocks.NewMockService(t)
		crtSvc.EXPECT().
			GetClusterResourceType(mock.Anything, testClusterResourceTypeName).
			Return(existing, nil)

		var sent *openchoreov1alpha1.ClusterResourceType
		crtSvc.EXPECT().
			UpdateClusterResourceType(mock.Anything, mock.Anything).
			Run(func(_ context.Context, crt *openchoreov1alpha1.ClusterResourceType) {
				sent = crt
			}).
			Return(existing, nil)

		h := newTestHandler(withClusterResourceTypeService(crtSvc))
		result, err := h.UpdateClusterResourceType(ctx, req)
		require.NoError(t, err)

		require.NotNil(t, sent)
		assert.Equal(t, "keep", sent.Annotations["custom"])
		assert.Equal(t, "Managed Redis", sent.Annotations["openchoreo.dev/display-name"])
		require.Len(t, sent.Spec.Resources, 1)
		assert.Equal(t, "claim-v2", sent.Spec.Resources[0].ID)

		m, ok := result.(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "updated", m["action"])
	})

	t.Run("propagates get error", func(t *testing.T) {
		expected := errors.New("missing")
		crtSvc := clusterresourcetypemocks.NewMockService(t)
		crtSvc.EXPECT().
			GetClusterResourceType(mock.Anything, testClusterResourceTypeName).
			Return(nil, expected)

		h := newTestHandler(withClusterResourceTypeService(crtSvc))
		_, err := h.UpdateClusterResourceType(ctx, &gen.UpdateClusterResourceTypeJSONRequestBody{
			Metadata: gen.ObjectMeta{Name: testClusterResourceTypeName},
		})
		require.ErrorIs(t, err, expected)
	})

	t.Run("propagates update error", func(t *testing.T) {
		expected := errors.New("conflict")
		crtSvc := clusterresourcetypemocks.NewMockService(t)
		crtSvc.EXPECT().
			GetClusterResourceType(mock.Anything, testClusterResourceTypeName).
			Return(sampleClusterResourceType(), nil)
		crtSvc.EXPECT().
			UpdateClusterResourceType(mock.Anything, mock.Anything).
			Return(nil, expected)

		h := newTestHandler(withClusterResourceTypeService(crtSvc))
		_, err := h.UpdateClusterResourceType(ctx, &gen.UpdateClusterResourceTypeJSONRequestBody{
			Metadata: gen.ObjectMeta{Name: testClusterResourceTypeName},
		})
		require.ErrorIs(t, err, expected)
	})

	t.Run("rejects nil body", func(t *testing.T) {
		h := newTestHandler()
		_, err := h.UpdateClusterResourceType(ctx, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "request body is required")
	})
}

func TestDeleteClusterResourceType(t *testing.T) {
	ctx := context.Background()

	t.Run("returns action deleted", func(t *testing.T) {
		crtSvc := clusterresourcetypemocks.NewMockService(t)
		crtSvc.EXPECT().
			DeleteClusterResourceType(mock.Anything, testClusterResourceTypeName).
			Return(nil)

		h := newTestHandler(withClusterResourceTypeService(crtSvc))
		result, err := h.DeleteClusterResourceType(ctx, testClusterResourceTypeName)
		require.NoError(t, err)
		m, ok := result.(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "deleted", m["action"])
		assert.Equal(t, testClusterResourceTypeName, m["name"])
	})

	t.Run("service error propagates", func(t *testing.T) {
		expected := errors.New("delete failed")
		crtSvc := clusterresourcetypemocks.NewMockService(t)
		crtSvc.EXPECT().
			DeleteClusterResourceType(mock.Anything, testClusterResourceTypeName).
			Return(expected)

		h := newTestHandler(withClusterResourceTypeService(crtSvc))
		_, err := h.DeleteClusterResourceType(ctx, testClusterResourceTypeName)
		require.ErrorIs(t, err, expected)
	})
}
