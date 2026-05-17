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
	"k8s.io/apimachinery/pkg/runtime"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
	resourcemocks "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/resource/mocks"
	resourcereleasemocks "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/resourcerelease/mocks"
	resourcereleasebindingmocks "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/resourcereleasebinding/mocks"
	"github.com/openchoreo/openchoreo/pkg/mcp/tools"
)

const (
	testResourceName               = "analytics-shared-db"
	testResourceReleaseName        = "analytics-shared-db-abc123"
	testResourceReleaseBindingName = "analytics-shared-db-dev"
	testResourceEnvironment        = "development"
)

func sampleResource() *openchoreov1alpha1.Resource {
	return &openchoreov1alpha1.Resource{
		ObjectMeta: metav1.ObjectMeta{Name: testResourceName, Namespace: testNS},
		Spec: openchoreov1alpha1.ResourceSpec{
			Owner: openchoreov1alpha1.ResourceOwner{ProjectName: testProject},
			Type: openchoreov1alpha1.ResourceTypeRef{
				Kind: openchoreov1alpha1.ResourceTypeRefKindResourceType,
				Name: "postgres",
			},
		},
	}
}

func sampleResourceRelease() *openchoreov1alpha1.ResourceRelease {
	return &openchoreov1alpha1.ResourceRelease{
		ObjectMeta: metav1.ObjectMeta{Name: testResourceReleaseName, Namespace: testNS},
		Spec: openchoreov1alpha1.ResourceReleaseSpec{
			Owner: openchoreov1alpha1.ResourceReleaseOwner{
				ProjectName:  testProject,
				ResourceName: testResourceName,
			},
			ResourceType: openchoreov1alpha1.ResourceReleaseResourceType{
				Kind: openchoreov1alpha1.ResourceTypeRefKindResourceType,
				Name: "postgres",
			},
		},
	}
}

func sampleResourceReleaseBinding() *openchoreov1alpha1.ResourceReleaseBinding {
	return &openchoreov1alpha1.ResourceReleaseBinding{
		ObjectMeta: metav1.ObjectMeta{Name: testResourceReleaseBindingName, Namespace: testNS},
		Spec: openchoreov1alpha1.ResourceReleaseBindingSpec{
			Owner: openchoreov1alpha1.ResourceReleaseBindingOwner{
				ProjectName:  testProject,
				ResourceName: testResourceName,
			},
			Environment:     testResourceEnvironment,
			ResourceRelease: testResourceReleaseName,
		},
	}
}

// ---------------------------------------------------------------------------
// Resource
// ---------------------------------------------------------------------------

func TestListResources(t *testing.T) {
	ctx := context.Background()

	t.Run("returns wrapped list", func(t *testing.T) {
		rSvc := resourcemocks.NewMockService(t)
		rSvc.EXPECT().
			ListResources(mock.Anything, testNS, testProject, mock.Anything).
			Return(&services.ListResult[openchoreov1alpha1.Resource]{
				Items:      []openchoreov1alpha1.Resource{*sampleResource()},
				NextCursor: "next-token",
			}, nil)

		h := newTestHandler(withResourceService(rSvc))
		result, err := h.ListResources(ctx, testNS, testProject, tools.ListOpts{Limit: 5})
		require.NoError(t, err)
		m, ok := result.(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "next-token", m["next_cursor"])
		items, ok := m["resources"].([]map[string]any)
		require.True(t, ok)
		require.Len(t, items, 1)
		assert.Equal(t, testResourceName, items[0]["name"])
		assert.Equal(t, testProject, items[0]["projectName"])
	})

	t.Run("service error propagates", func(t *testing.T) {
		expected := errors.New("list failed")
		rSvc := resourcemocks.NewMockService(t)
		rSvc.EXPECT().
			ListResources(mock.Anything, testNS, testProject, mock.Anything).
			Return(nil, expected)

		h := newTestHandler(withResourceService(rSvc))
		_, err := h.ListResources(ctx, testNS, testProject, tools.ListOpts{})
		require.ErrorIs(t, err, expected)
	})
}

func TestGetResource(t *testing.T) {
	ctx := context.Background()

	t.Run("returns detail map", func(t *testing.T) {
		rSvc := resourcemocks.NewMockService(t)
		rSvc.EXPECT().
			GetResource(mock.Anything, testNS, testResourceName).
			Return(sampleResource(), nil)

		h := newTestHandler(withResourceService(rSvc))
		result, err := h.GetResource(ctx, testNS, testResourceName)
		require.NoError(t, err)
		m, ok := result.(map[string]any)
		require.True(t, ok)
		assert.Equal(t, testResourceName, m["name"])
		typeMap, ok := m["type"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "ResourceType", typeMap["kind"])
		assert.Equal(t, "postgres", typeMap["name"])
	})

	t.Run("service error propagates", func(t *testing.T) {
		expected := errors.New("not found")
		rSvc := resourcemocks.NewMockService(t)
		rSvc.EXPECT().
			GetResource(mock.Anything, testNS, testResourceName).
			Return(nil, expected)

		h := newTestHandler(withResourceService(rSvc))
		_, err := h.GetResource(ctx, testNS, testResourceName)
		require.ErrorIs(t, err, expected)
	})
}

func TestCreateResource(t *testing.T) {
	ctx := context.Background()

	t.Run("builds CRD from req with project owner, type, and parameters", func(t *testing.T) {
		kind := gen.ResourceTypeRefKindResourceType
		params := map[string]interface{}{"version": "8.0"}
		annotations := map[string]string{"openchoreo.dev/display-name": "Analytics DB"}
		req := &gen.CreateResourceJSONRequestBody{
			Metadata: gen.ObjectMeta{Name: testResourceName, Annotations: &annotations},
			Spec: &gen.ResourceInstanceSpec{
				Owner: struct {
					ProjectName string `json:"projectName"`
				}{ProjectName: ""},
				Type:       gen.ResourceTypeRef{Kind: &kind, Name: "postgres"},
				Parameters: &params,
			},
		}

		rSvc := resourcemocks.NewMockService(t)
		rSvc.EXPECT().
			CreateResource(mock.Anything, testNS, mock.MatchedBy(func(r *openchoreov1alpha1.Resource) bool {
				if r.Name != testResourceName || r.Namespace != testNS {
					return false
				}
				if r.Spec.Owner.ProjectName != testProject {
					return false
				}
				if r.Spec.Type.Name != "postgres" ||
					r.Spec.Type.Kind != openchoreov1alpha1.ResourceTypeRefKindResourceType {
					return false
				}
				if r.Annotations["openchoreo.dev/display-name"] != "Analytics DB" {
					return false
				}
				if r.Spec.Parameters == nil {
					return false
				}
				var got map[string]any
				if err := json.Unmarshal(r.Spec.Parameters.Raw, &got); err != nil {
					return false
				}
				return got["version"] == "8.0"
			})).
			Return(sampleResource(), nil)

		h := newTestHandler(withResourceService(rSvc))
		result, err := h.CreateResource(ctx, testNS, testProject, req)
		require.NoError(t, err)
		m, ok := result.(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "created", m["action"])
	})

	t.Run("rejects when body project owner mismatches path-scoped projectName", func(t *testing.T) {
		// Defense-in-depth: the path-scoped projectName is the authz boundary; the body
		// must not be allowed to broaden it.
		h := newTestHandler()
		req := &gen.CreateResourceJSONRequestBody{
			Metadata: gen.ObjectMeta{Name: testResourceName},
			Spec: &gen.ResourceInstanceSpec{
				Owner: struct {
					ProjectName string `json:"projectName"`
				}{ProjectName: "other-project"},
				Type: gen.ResourceTypeRef{Name: "postgres"},
			},
		}
		_, err := h.CreateResource(ctx, testNS, testProject, req)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "must match projectName")
	})

	t.Run("accepts matching body project owner", func(t *testing.T) {
		rSvc := resourcemocks.NewMockService(t)
		rSvc.EXPECT().
			CreateResource(mock.Anything, testNS, mock.MatchedBy(func(r *openchoreov1alpha1.Resource) bool {
				return r.Spec.Owner.ProjectName == testProject
			})).
			Return(sampleResource(), nil)

		req := &gen.CreateResourceJSONRequestBody{
			Metadata: gen.ObjectMeta{Name: testResourceName},
			Spec: &gen.ResourceInstanceSpec{
				Owner: struct {
					ProjectName string `json:"projectName"`
				}{ProjectName: testProject},
				Type: gen.ResourceTypeRef{Name: "postgres"},
			},
		}
		h := newTestHandler(withResourceService(rSvc))
		_, err := h.CreateResource(ctx, testNS, testProject, req)
		require.NoError(t, err)
	})

	t.Run("rejects nil body and nil spec", func(t *testing.T) {
		h := newTestHandler()
		_, err := h.CreateResource(ctx, testNS, testProject, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "request body is required")

		_, err = h.CreateResource(ctx, testNS, testProject, &gen.CreateResourceJSONRequestBody{
			Metadata: gen.ObjectMeta{Name: testResourceName},
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "spec is required")
	})

	t.Run("service error propagates", func(t *testing.T) {
		expected := errors.New("create failed")
		rSvc := resourcemocks.NewMockService(t)
		rSvc.EXPECT().
			CreateResource(mock.Anything, testNS, mock.Anything).
			Return(nil, expected)

		h := newTestHandler(withResourceService(rSvc))
		_, err := h.CreateResource(ctx, testNS, testProject, &gen.CreateResourceJSONRequestBody{
			Metadata: gen.ObjectMeta{Name: testResourceName},
			Spec: &gen.ResourceInstanceSpec{
				Type: gen.ResourceTypeRef{Name: "postgres"},
			},
		})
		require.ErrorIs(t, err, expected)
	})
}

func TestUpdateResource(t *testing.T) {
	ctx := context.Background()

	t.Run("merges annotations and updates parameters", func(t *testing.T) {
		existing := sampleResource()
		existing.Annotations = map[string]string{"keep-me": "value"}
		params := map[string]interface{}{"version": "8.4"}
		newAnnotations := map[string]string{"openchoreo.dev/description": "Updated"}
		req := &gen.UpdateResourceJSONRequestBody{
			Metadata: gen.ObjectMeta{Name: testResourceName, Annotations: &newAnnotations},
			Spec:     &gen.ResourceInstanceSpec{Parameters: &params},
		}

		rSvc := resourcemocks.NewMockService(t)
		rSvc.EXPECT().
			GetResource(mock.Anything, testNS, testResourceName).
			Return(existing, nil)

		var sent *openchoreov1alpha1.Resource
		rSvc.EXPECT().
			UpdateResource(mock.Anything, testNS, mock.Anything).
			Run(func(_ context.Context, _ string, r *openchoreov1alpha1.Resource) {
				sent = r
			}).
			Return(existing, nil)

		h := newTestHandler(withResourceService(rSvc))
		result, err := h.UpdateResource(ctx, testNS, req)
		require.NoError(t, err)

		require.NotNil(t, sent)
		assert.Equal(t, "value", sent.Annotations["keep-me"])
		assert.Equal(t, "Updated", sent.Annotations["openchoreo.dev/description"])
		require.NotNil(t, sent.Spec.Parameters)
		var got map[string]any
		require.NoError(t, json.Unmarshal(sent.Spec.Parameters.Raw, &got))
		assert.Equal(t, "8.4", got["version"])

		m, ok := result.(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "updated", m["action"])
	})

	t.Run("rejects nil body", func(t *testing.T) {
		h := newTestHandler()
		_, err := h.UpdateResource(ctx, testNS, nil)
		require.Error(t, err)
	})

	t.Run("propagates get error", func(t *testing.T) {
		expected := errors.New("missing")
		rSvc := resourcemocks.NewMockService(t)
		rSvc.EXPECT().
			GetResource(mock.Anything, testNS, testResourceName).
			Return(nil, expected)

		h := newTestHandler(withResourceService(rSvc))
		_, err := h.UpdateResource(ctx, testNS, &gen.UpdateResourceJSONRequestBody{
			Metadata: gen.ObjectMeta{Name: testResourceName},
		})
		require.ErrorIs(t, err, expected)
	})
}

func TestDeleteResource(t *testing.T) {
	ctx := context.Background()

	t.Run("returns action deleted", func(t *testing.T) {
		rSvc := resourcemocks.NewMockService(t)
		rSvc.EXPECT().
			DeleteResource(mock.Anything, testNS, testResourceName).
			Return(nil)

		h := newTestHandler(withResourceService(rSvc))
		result, err := h.DeleteResource(ctx, testNS, testResourceName)
		require.NoError(t, err)
		m, ok := result.(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "deleted", m["action"])
		assert.Equal(t, testResourceName, m["name"])
		assert.Equal(t, testNS, m["namespace"])
	})

	t.Run("service error propagates", func(t *testing.T) {
		expected := errors.New("delete failed")
		rSvc := resourcemocks.NewMockService(t)
		rSvc.EXPECT().
			DeleteResource(mock.Anything, testNS, testResourceName).
			Return(expected)

		h := newTestHandler(withResourceService(rSvc))
		_, err := h.DeleteResource(ctx, testNS, testResourceName)
		require.ErrorIs(t, err, expected)
	})
}

// ---------------------------------------------------------------------------
// ResourceRelease
// ---------------------------------------------------------------------------

func TestListResourceReleases(t *testing.T) {
	ctx := context.Background()

	t.Run("returns wrapped list", func(t *testing.T) {
		rrSvc := resourcereleasemocks.NewMockService(t)
		rrSvc.EXPECT().
			ListResourceReleases(mock.Anything, testNS, testResourceName, mock.Anything).
			Return(&services.ListResult[openchoreov1alpha1.ResourceRelease]{
				Items: []openchoreov1alpha1.ResourceRelease{*sampleResourceRelease()},
			}, nil)

		h := newTestHandler(withResourceReleaseService(rrSvc))
		result, err := h.ListResourceReleases(ctx, testNS, testResourceName, tools.ListOpts{})
		require.NoError(t, err)
		m, ok := result.(map[string]any)
		require.True(t, ok)
		items, ok := m["resource_releases"].([]map[string]any)
		require.True(t, ok)
		require.Len(t, items, 1)
		assert.Equal(t, testResourceReleaseName, items[0]["name"])
		assert.Equal(t, testResourceName, items[0]["resourceName"])
	})

	t.Run("service error propagates", func(t *testing.T) {
		expected := errors.New("list failed")
		rrSvc := resourcereleasemocks.NewMockService(t)
		rrSvc.EXPECT().
			ListResourceReleases(mock.Anything, testNS, testResourceName, mock.Anything).
			Return(nil, expected)

		h := newTestHandler(withResourceReleaseService(rrSvc))
		_, err := h.ListResourceReleases(ctx, testNS, testResourceName, tools.ListOpts{})
		require.ErrorIs(t, err, expected)
	})
}

func TestGetResourceRelease(t *testing.T) {
	ctx := context.Background()

	t.Run("returns detail map", func(t *testing.T) {
		rrSvc := resourcereleasemocks.NewMockService(t)
		rrSvc.EXPECT().
			GetResourceRelease(mock.Anything, testNS, testResourceReleaseName).
			Return(sampleResourceRelease(), nil)

		h := newTestHandler(withResourceReleaseService(rrSvc))
		result, err := h.GetResourceRelease(ctx, testNS, testResourceReleaseName)
		require.NoError(t, err)
		m, ok := result.(map[string]any)
		require.True(t, ok)
		assert.Equal(t, testResourceReleaseName, m["name"])
		assert.Equal(t, testResourceName, m["resourceName"])
	})

	t.Run("service error propagates", func(t *testing.T) {
		expected := errors.New("not found")
		rrSvc := resourcereleasemocks.NewMockService(t)
		rrSvc.EXPECT().
			GetResourceRelease(mock.Anything, testNS, testResourceReleaseName).
			Return(nil, expected)

		h := newTestHandler(withResourceReleaseService(rrSvc))
		_, err := h.GetResourceRelease(ctx, testNS, testResourceReleaseName)
		require.ErrorIs(t, err, expected)
	})
}

func TestCreateResourceRelease(t *testing.T) {
	ctx := context.Background()

	t.Run("converts gen body to CRD with parameters", func(t *testing.T) {
		params := map[string]interface{}{"version": "8.0"}
		body := &gen.CreateResourceReleaseJSONRequestBody{
			Metadata: gen.ObjectMeta{Name: testResourceReleaseName},
			Spec: &gen.ResourceReleaseSpec{
				Owner: struct {
					ProjectName  string `json:"projectName"`
					ResourceName string `json:"resourceName"`
				}{ProjectName: testProject, ResourceName: testResourceName},
				ResourceType: struct {
					Kind gen.ResourceReleaseSpecResourceTypeKind `json:"kind"`
					Name string                                  `json:"name"`
					Spec gen.ResourceTypeSpec                    `json:"spec"`
				}{
					Kind: gen.ResourceReleaseSpecResourceTypeKindResourceType,
					Name: "postgres",
					Spec: gen.ResourceTypeSpec{
						Resources: []gen.ResourceTypeManifest{{Id: "claim"}},
					},
				},
				Parameters: &params,
			},
		}

		rrSvc := resourcereleasemocks.NewMockService(t)
		rrSvc.EXPECT().
			CreateResourceRelease(mock.Anything, testNS, mock.MatchedBy(func(rr *openchoreov1alpha1.ResourceRelease) bool {
				return rr.Name == testResourceReleaseName &&
					rr.Namespace == testNS &&
					rr.Spec.Owner.ProjectName == testProject &&
					rr.Spec.Owner.ResourceName == testResourceName &&
					rr.Spec.ResourceType.Name == "postgres" &&
					len(rr.Spec.ResourceType.Spec.Resources) == 1 &&
					rr.Spec.Parameters != nil
			})).
			Return(sampleResourceRelease(), nil)

		h := newTestHandler(withResourceReleaseService(rrSvc))
		result, err := h.CreateResourceRelease(ctx, testNS, body)
		require.NoError(t, err)
		m, ok := result.(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "created", m["action"])
	})

	t.Run("rejects nil body", func(t *testing.T) {
		h := newTestHandler()
		_, err := h.CreateResourceRelease(ctx, testNS, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "request body is required")
	})

	t.Run("service error propagates", func(t *testing.T) {
		expected := errors.New("create failed")
		rrSvc := resourcereleasemocks.NewMockService(t)
		rrSvc.EXPECT().
			CreateResourceRelease(mock.Anything, testNS, mock.Anything).
			Return(nil, expected)

		h := newTestHandler(withResourceReleaseService(rrSvc))
		_, err := h.CreateResourceRelease(ctx, testNS, &gen.CreateResourceReleaseJSONRequestBody{
			Metadata: gen.ObjectMeta{Name: testResourceReleaseName},
		})
		require.ErrorIs(t, err, expected)
	})
}

func TestDeleteResourceRelease(t *testing.T) {
	ctx := context.Background()

	t.Run("returns action deleted", func(t *testing.T) {
		rrSvc := resourcereleasemocks.NewMockService(t)
		rrSvc.EXPECT().
			DeleteResourceRelease(mock.Anything, testNS, testResourceReleaseName).
			Return(nil)

		h := newTestHandler(withResourceReleaseService(rrSvc))
		result, err := h.DeleteResourceRelease(ctx, testNS, testResourceReleaseName)
		require.NoError(t, err)
		m, ok := result.(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "deleted", m["action"])
		assert.Equal(t, testResourceReleaseName, m["name"])
	})

	t.Run("service error propagates", func(t *testing.T) {
		expected := errors.New("delete failed")
		rrSvc := resourcereleasemocks.NewMockService(t)
		rrSvc.EXPECT().
			DeleteResourceRelease(mock.Anything, testNS, testResourceReleaseName).
			Return(expected)

		h := newTestHandler(withResourceReleaseService(rrSvc))
		_, err := h.DeleteResourceRelease(ctx, testNS, testResourceReleaseName)
		require.ErrorIs(t, err, expected)
	})
}

// ---------------------------------------------------------------------------
// ResourceReleaseBinding
// ---------------------------------------------------------------------------

func TestListResourceReleaseBindings(t *testing.T) {
	ctx := context.Background()

	t.Run("returns wrapped list", func(t *testing.T) {
		rbSvc := resourcereleasebindingmocks.NewMockService(t)
		rbSvc.EXPECT().
			ListResourceReleaseBindings(mock.Anything, testNS, testResourceName, mock.Anything).
			Return(&services.ListResult[openchoreov1alpha1.ResourceReleaseBinding]{
				Items: []openchoreov1alpha1.ResourceReleaseBinding{*sampleResourceReleaseBinding()},
			}, nil)

		h := newTestHandler(withResourceReleaseBindingService(rbSvc))
		result, err := h.ListResourceReleaseBindings(ctx, testNS, testResourceName, tools.ListOpts{})
		require.NoError(t, err)
		m, ok := result.(map[string]any)
		require.True(t, ok)
		items, ok := m["resource_release_bindings"].([]map[string]any)
		require.True(t, ok)
		require.Len(t, items, 1)
		assert.Equal(t, testResourceReleaseBindingName, items[0]["name"])
		assert.Equal(t, testResourceEnvironment, items[0]["environment"])
	})

	t.Run("service error propagates", func(t *testing.T) {
		expected := errors.New("list failed")
		rbSvc := resourcereleasebindingmocks.NewMockService(t)
		rbSvc.EXPECT().
			ListResourceReleaseBindings(mock.Anything, testNS, testResourceName, mock.Anything).
			Return(nil, expected)

		h := newTestHandler(withResourceReleaseBindingService(rbSvc))
		_, err := h.ListResourceReleaseBindings(ctx, testNS, testResourceName, tools.ListOpts{})
		require.ErrorIs(t, err, expected)
	})
}

func TestGetResourceReleaseBinding(t *testing.T) {
	ctx := context.Background()

	t.Run("returns detail map", func(t *testing.T) {
		rbSvc := resourcereleasebindingmocks.NewMockService(t)
		rbSvc.EXPECT().
			GetResourceReleaseBinding(mock.Anything, testNS, testResourceReleaseBindingName).
			Return(sampleResourceReleaseBinding(), nil)

		h := newTestHandler(withResourceReleaseBindingService(rbSvc))
		result, err := h.GetResourceReleaseBinding(ctx, testNS, testResourceReleaseBindingName)
		require.NoError(t, err)
		m, ok := result.(map[string]any)
		require.True(t, ok)
		assert.Equal(t, testResourceReleaseBindingName, m["name"])
		assert.Equal(t, testResourceReleaseName, m["resourceRelease"])
	})

	t.Run("service error propagates", func(t *testing.T) {
		expected := errors.New("not found")
		rbSvc := resourcereleasebindingmocks.NewMockService(t)
		rbSvc.EXPECT().
			GetResourceReleaseBinding(mock.Anything, testNS, testResourceReleaseBindingName).
			Return(nil, expected)

		h := newTestHandler(withResourceReleaseBindingService(rbSvc))
		_, err := h.GetResourceReleaseBinding(ctx, testNS, testResourceReleaseBindingName)
		require.ErrorIs(t, err, expected)
	})
}

func TestCreateResourceReleaseBinding(t *testing.T) {
	ctx := context.Background()

	t.Run("converts gen body to CRD", func(t *testing.T) {
		release := testResourceReleaseName
		retain := gen.ResourceReleaseBindingSpecRetainPolicyRetain
		envConfigs := map[string]interface{}{"storageGB": float64(100)}
		body := &gen.CreateResourceReleaseBindingJSONRequestBody{
			Metadata: gen.ObjectMeta{Name: testResourceReleaseBindingName},
			Spec: &gen.ResourceReleaseBindingSpec{
				Environment: testResourceEnvironment,
				Owner: struct {
					ProjectName  string `json:"projectName"`
					ResourceName string `json:"resourceName"`
				}{ProjectName: testProject, ResourceName: testResourceName},
				ResourceRelease:                &release,
				RetainPolicy:                   &retain,
				ResourceTypeEnvironmentConfigs: &envConfigs,
			},
		}

		rbSvc := resourcereleasebindingmocks.NewMockService(t)
		rbSvc.EXPECT().
			CreateResourceReleaseBinding(mock.Anything, testNS, mock.MatchedBy(
				func(rb *openchoreov1alpha1.ResourceReleaseBinding) bool {
					return rb.Name == testResourceReleaseBindingName &&
						rb.Namespace == testNS &&
						rb.Spec.Owner.ProjectName == testProject &&
						rb.Spec.Owner.ResourceName == testResourceName &&
						rb.Spec.Environment == testResourceEnvironment &&
						rb.Spec.ResourceRelease == testResourceReleaseName &&
						string(rb.Spec.RetainPolicy) == "Retain" &&
						rb.Spec.ResourceTypeEnvironmentConfigs != nil
				},
			)).
			Return(sampleResourceReleaseBinding(), nil)

		h := newTestHandler(withResourceReleaseBindingService(rbSvc))
		result, err := h.CreateResourceReleaseBinding(ctx, testNS, body)
		require.NoError(t, err)
		m, ok := result.(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "created", m["action"])
	})

	t.Run("rejects nil body", func(t *testing.T) {
		h := newTestHandler()
		_, err := h.CreateResourceReleaseBinding(ctx, testNS, nil)
		require.Error(t, err)
	})

	t.Run("service error propagates", func(t *testing.T) {
		expected := errors.New("create failed")
		rbSvc := resourcereleasebindingmocks.NewMockService(t)
		rbSvc.EXPECT().
			CreateResourceReleaseBinding(mock.Anything, testNS, mock.Anything).
			Return(nil, expected)

		h := newTestHandler(withResourceReleaseBindingService(rbSvc))
		_, err := h.CreateResourceReleaseBinding(ctx, testNS, &gen.CreateResourceReleaseBindingJSONRequestBody{
			Metadata: gen.ObjectMeta{Name: testResourceReleaseBindingName},
		})
		require.ErrorIs(t, err, expected)
	})
}

func TestUpdateResourceReleaseBinding(t *testing.T) {
	ctx := context.Background()

	t.Run("converts gen body to CRD", func(t *testing.T) {
		newRelease := "analytics-shared-db-def456"
		body := &gen.UpdateResourceReleaseBindingJSONRequestBody{
			Metadata: gen.ObjectMeta{Name: testResourceReleaseBindingName},
			Spec: &gen.ResourceReleaseBindingSpec{
				Environment: testResourceEnvironment,
				Owner: struct {
					ProjectName  string `json:"projectName"`
					ResourceName string `json:"resourceName"`
				}{ProjectName: testProject, ResourceName: testResourceName},
				ResourceRelease: &newRelease,
			},
		}

		rbSvc := resourcereleasebindingmocks.NewMockService(t)
		rbSvc.EXPECT().
			UpdateResourceReleaseBinding(mock.Anything, testNS, mock.MatchedBy(
				func(rb *openchoreov1alpha1.ResourceReleaseBinding) bool {
					return rb.Name == testResourceReleaseBindingName &&
						rb.Namespace == testNS &&
						rb.Spec.ResourceRelease == newRelease
				},
			)).
			Return(sampleResourceReleaseBinding(), nil)

		h := newTestHandler(withResourceReleaseBindingService(rbSvc))
		result, err := h.UpdateResourceReleaseBinding(ctx, testNS, body)
		require.NoError(t, err)
		m, ok := result.(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "updated", m["action"])
	})

	t.Run("rejects nil body", func(t *testing.T) {
		h := newTestHandler()
		_, err := h.UpdateResourceReleaseBinding(ctx, testNS, nil)
		require.Error(t, err)
	})

	t.Run("service error propagates", func(t *testing.T) {
		expected := errors.New("update failed")
		rbSvc := resourcereleasebindingmocks.NewMockService(t)
		rbSvc.EXPECT().
			UpdateResourceReleaseBinding(mock.Anything, testNS, mock.Anything).
			Return(nil, expected)

		h := newTestHandler(withResourceReleaseBindingService(rbSvc))
		_, err := h.UpdateResourceReleaseBinding(ctx, testNS, &gen.UpdateResourceReleaseBindingJSONRequestBody{
			Metadata: gen.ObjectMeta{Name: testResourceReleaseBindingName},
		})
		require.ErrorIs(t, err, expected)
	})
}

func TestDeleteResourceReleaseBinding(t *testing.T) {
	ctx := context.Background()

	t.Run("returns action deleted", func(t *testing.T) {
		rbSvc := resourcereleasebindingmocks.NewMockService(t)
		rbSvc.EXPECT().
			DeleteResourceReleaseBinding(mock.Anything, testNS, testResourceReleaseBindingName).
			Return(nil)

		h := newTestHandler(withResourceReleaseBindingService(rbSvc))
		result, err := h.DeleteResourceReleaseBinding(ctx, testNS, testResourceReleaseBindingName)
		require.NoError(t, err)
		m, ok := result.(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "deleted", m["action"])
		assert.Equal(t, testResourceReleaseBindingName, m["name"])
	})

	t.Run("service error propagates", func(t *testing.T) {
		expected := errors.New("delete failed")
		rbSvc := resourcereleasebindingmocks.NewMockService(t)
		rbSvc.EXPECT().
			DeleteResourceReleaseBinding(mock.Anything, testNS, testResourceReleaseBindingName).
			Return(expected)

		h := newTestHandler(withResourceReleaseBindingService(rbSvc))
		_, err := h.DeleteResourceReleaseBinding(ctx, testNS, testResourceReleaseBindingName)
		require.ErrorIs(t, err, expected)
	})
}

// ---------------------------------------------------------------------------
// Transform helpers smoke
// ---------------------------------------------------------------------------

func TestResourceTransforms(t *testing.T) {
	t.Run("resource detail includes parameters and status", func(t *testing.T) {
		r := sampleResource()
		paramsBytes, err := json.Marshal(map[string]any{"version": "8.0"})
		require.NoError(t, err)
		r.Spec.Parameters = &runtime.RawExtension{Raw: paramsBytes}
		r.Status.LatestRelease = &openchoreov1alpha1.LatestResourceRelease{
			Name: testResourceReleaseName,
			Hash: "abc123",
		}

		m := resourceDetail(r)
		assert.Equal(t, testResourceName, m["name"])
		params, ok := m["parameters"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "8.0", params["version"])
		latest, ok := m["latestRelease"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, testResourceReleaseName, latest["name"])
		assert.Equal(t, "abc123", latest["hash"])
	})

	t.Run("resource release binding detail surfaces resolved outputs", func(t *testing.T) {
		rb := sampleResourceReleaseBinding()
		rb.Status.Outputs = []openchoreov1alpha1.ResolvedResourceOutput{
			{Name: "host", Value: "10.0.0.5"},
			{Name: "password", SecretKeyRef: &openchoreov1alpha1.SecretKeyRef{Name: "conn", Key: "password"}},
		}

		m := resourceReleaseBindingDetail(rb)
		outputs, ok := m["outputs"].([]map[string]any)
		require.True(t, ok)
		require.Len(t, outputs, 2)
		assert.Equal(t, "host", outputs[0]["name"])
		assert.Equal(t, "10.0.0.5", outputs[0]["value"])
		secretRef, ok := outputs[1]["secretKeyRef"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "conn", secretRef["name"])
		assert.Equal(t, "password", secretRef["key"])
	})

	t.Run("resource type detail embeds spec", func(t *testing.T) {
		rt := sampleResourceType()
		m := resourceTypeDetail(rt)
		assert.Equal(t, testResourceTypeName, m["name"])
		assert.NotNil(t, m["spec"])
	})
}
