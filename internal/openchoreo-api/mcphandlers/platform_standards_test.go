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
	"github.com/openchoreo/openchoreo/internal/controller"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
	clustercomponenttypemocks "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/clustercomponenttype/mocks"
	componenttypemocks "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/componenttype/mocks"
)

// ---------------------------------------------------------------------------
// cleanAnnotations
// ---------------------------------------------------------------------------

func TestCleanAnnotations(t *testing.T) {
	t.Run("removes empty display name", func(t *testing.T) {
		m := map[string]string{controller.AnnotationKeyDisplayName: ""}
		cleanAnnotations(m)
		_, ok := m[controller.AnnotationKeyDisplayName]
		assert.False(t, ok)
	})

	t.Run("removes empty description", func(t *testing.T) {
		m := map[string]string{controller.AnnotationKeyDescription: ""}
		cleanAnnotations(m)
		_, ok := m[controller.AnnotationKeyDescription]
		assert.False(t, ok)
	})

	t.Run("preserves non-empty values", func(t *testing.T) {
		m := map[string]string{
			controller.AnnotationKeyDisplayName: "My Name",
			controller.AnnotationKeyDescription: "My Desc",
		}
		cleanAnnotations(m)
		assert.Equal(t, "My Name", m[controller.AnnotationKeyDisplayName])
		assert.Equal(t, "My Desc", m[controller.AnnotationKeyDescription])
	})

	t.Run("missing keys are no-op", func(t *testing.T) {
		m := map[string]string{"other-key": "val"}
		cleanAnnotations(m)
		assert.Equal(t, "val", m["other-key"])
	})

	t.Run("empty map does not panic", func(t *testing.T) {
		assert.NotPanics(t, func() {
			cleanAnnotations(map[string]string{})
		})
	})
}

// ---------------------------------------------------------------------------
// convertSpec
// ---------------------------------------------------------------------------

// testSpecSrc and testSpecDst are minimal struct pairs for testing convertSpec.
type testSpecSrc struct {
	Field string `json:"field"`
	Count int    `json:"count"`
}

type testSpecDst struct {
	Field string `json:"field"`
	Count int    `json:"count"`
}

func TestConvertSpec(t *testing.T) {
	t.Run("converts matching fields via JSON round-trip", func(t *testing.T) {
		src := testSpecSrc{Field: "hello", Count: 42}
		dst, err := convertSpec[testSpecSrc, testSpecDst](src)
		require.NoError(t, err)
		assert.Equal(t, "hello", dst.Field)
		assert.Equal(t, 42, dst.Count)
	})

	t.Run("unknown source fields are silently dropped", func(t *testing.T) {
		type srcWithExtra struct {
			Field string `json:"field"`
			Extra string `json:"extra"`
		}
		src := srcWithExtra{Field: "val", Extra: "ignored"}
		dst, err := convertSpec[srcWithExtra, testSpecDst](src)
		require.NoError(t, err)
		assert.Equal(t, "val", dst.Field)
	})
}

// ---------------------------------------------------------------------------
// CreateComponentType (representative namespace-scoped CRUD)
// ---------------------------------------------------------------------------

func TestCreateComponentType(t *testing.T) {
	ctx := context.Background()

	makeCreated := func() *openchoreov1alpha1.ComponentType {
		return &openchoreov1alpha1.ComponentType{ObjectMeta: metav1.ObjectMeta{Name: "my-ct", Namespace: testNS}}
	}

	t.Run("happy path: name, namespace, annotations, spec set", func(t *testing.T) {
		ctSvc := componenttypemocks.NewMockService(t)
		annotations := map[string]string{controller.AnnotationKeyDisplayName: "My Type"}
		ctSvc.EXPECT().
			CreateComponentType(mock.Anything, testNS, mock.MatchedBy(func(ct *openchoreov1alpha1.ComponentType) bool {
				return ct.Name == "my-ct" &&
					ct.Namespace == testNS &&
					ct.Annotations[controller.AnnotationKeyDisplayName] == "My Type"
			})).
			Return(makeCreated(), nil)

		req := &gen.CreateComponentTypeJSONRequestBody{
			Metadata: gen.ObjectMeta{Name: "my-ct", Annotations: &annotations},
		}
		h := newTestHandler(withComponentTypeService(ctSvc))
		_, err := h.CreateComponentType(ctx, testNS, req)
		require.NoError(t, err)
	})

	t.Run("nil annotations: no panic", func(t *testing.T) {
		ctSvc := componenttypemocks.NewMockService(t)
		ctSvc.EXPECT().CreateComponentType(mock.Anything, testNS, mock.Anything).Return(makeCreated(), nil)

		req := &gen.CreateComponentTypeJSONRequestBody{
			Metadata: gen.ObjectMeta{Name: "my-ct"},
		}
		h := newTestHandler(withComponentTypeService(ctSvc))
		_, err := h.CreateComponentType(ctx, testNS, req)
		require.NoError(t, err)
	})

	t.Run("empty annotation values cleaned", func(t *testing.T) {
		ctSvc := componenttypemocks.NewMockService(t)
		annotations := map[string]string{
			controller.AnnotationKeyDisplayName: "",
			controller.AnnotationKeyDescription: "",
		}
		ctSvc.EXPECT().
			CreateComponentType(mock.Anything, testNS, mock.MatchedBy(func(ct *openchoreov1alpha1.ComponentType) bool {
				_, hasDisplay := ct.Annotations[controller.AnnotationKeyDisplayName]
				_, hasDesc := ct.Annotations[controller.AnnotationKeyDescription]
				return !hasDisplay && !hasDesc
			})).
			Return(makeCreated(), nil)

		req := &gen.CreateComponentTypeJSONRequestBody{
			Metadata: gen.ObjectMeta{Name: "my-ct", Annotations: &annotations},
		}
		h := newTestHandler(withComponentTypeService(ctSvc))
		_, err := h.CreateComponentType(ctx, testNS, req)
		require.NoError(t, err)
	})

	t.Run("service error propagated", func(t *testing.T) {
		ctSvc := componenttypemocks.NewMockService(t)
		ctSvc.EXPECT().CreateComponentType(mock.Anything, testNS, mock.Anything).Return(nil, errors.New("create failed"))

		req := &gen.CreateComponentTypeJSONRequestBody{Metadata: gen.ObjectMeta{Name: "my-ct"}}
		h := newTestHandler(withComponentTypeService(ctSvc))
		_, err := h.CreateComponentType(ctx, testNS, req)
		require.Error(t, err)
	})
}

// ---------------------------------------------------------------------------
// UpdateComponentType (representative update pattern)
// ---------------------------------------------------------------------------

func TestUpdateComponentType(t *testing.T) {
	ctx := context.Background()

	freshExisting := func() *openchoreov1alpha1.ComponentType {
		return &openchoreov1alpha1.ComponentType{
			ObjectMeta: metav1.ObjectMeta{
				Name:        "my-ct",
				Namespace:   testNS,
				Annotations: map[string]string{"existing-key": testExistingVal},
			},
		}
	}

	t.Run("annotations merged with existing", func(t *testing.T) {
		ctSvc := componenttypemocks.NewMockService(t)
		ctSvc.EXPECT().GetComponentType(mock.Anything, testNS, "my-ct").Return(freshExisting(), nil)
		newAnnotations := map[string]string{"new-key": testNewVal}
		ctSvc.EXPECT().
			UpdateComponentType(mock.Anything, testNS, mock.MatchedBy(func(ct *openchoreov1alpha1.ComponentType) bool {
				return ct.Annotations["existing-key"] == testExistingVal &&
					ct.Annotations["new-key"] == testNewVal
			})).
			Return(freshExisting(), nil)

		req := &gen.UpdateComponentTypeJSONRequestBody{
			Metadata: gen.ObjectMeta{Name: "my-ct", Annotations: &newAnnotations},
		}
		h := newTestHandler(withComponentTypeService(ctSvc))
		_, err := h.UpdateComponentType(ctx, testNS, req)
		require.NoError(t, err)
	})

	t.Run("GetComponentType error propagated", func(t *testing.T) {
		ctSvc := componenttypemocks.NewMockService(t)
		ctSvc.EXPECT().GetComponentType(mock.Anything, testNS, "my-ct").Return(nil, errors.New("not found"))

		req := &gen.UpdateComponentTypeJSONRequestBody{Metadata: gen.ObjectMeta{Name: "my-ct"}}
		h := newTestHandler(withComponentTypeService(ctSvc))
		_, err := h.UpdateComponentType(ctx, testNS, req)
		require.Error(t, err)
	})
}

// ---------------------------------------------------------------------------
// DeleteComponentType
// ---------------------------------------------------------------------------

func TestDeleteComponentType(t *testing.T) {
	ctx := context.Background()

	t.Run("response has correct fields", func(t *testing.T) {
		ctSvc := componenttypemocks.NewMockService(t)
		ctSvc.EXPECT().DeleteComponentType(mock.Anything, testNS, "my-ct").Return(nil)

		h := newTestHandler(withComponentTypeService(ctSvc))
		result, err := h.DeleteComponentType(ctx, testNS, "my-ct")
		require.NoError(t, err)
		m, ok := result.(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "my-ct", m["name"])
		assert.Equal(t, testNS, m["namespace"])
		assert.Equal(t, "deleted", m["action"])
	})
}

// ---------------------------------------------------------------------------
// CreateClusterComponentType (representative cluster-scoped operation)
// ---------------------------------------------------------------------------

func TestCreateClusterComponentType(t *testing.T) {
	ctx := context.Background()

	makeCreated := func() *openchoreov1alpha1.ClusterComponentType {
		return &openchoreov1alpha1.ClusterComponentType{ObjectMeta: metav1.ObjectMeta{Name: "my-cct"}}
	}

	t.Run("no namespace field on cluster-scoped resource", func(t *testing.T) {
		cctSvc := clustercomponenttypemocks.NewMockService(t)
		cctSvc.EXPECT().
			CreateClusterComponentType(mock.Anything, mock.MatchedBy(func(cct *openchoreov1alpha1.ClusterComponentType) bool {
				return cct.Name == "my-cct" && cct.Namespace == ""
			})).
			Return(makeCreated(), nil)

		req := &gen.CreateClusterComponentTypeJSONRequestBody{
			Metadata: gen.ObjectMeta{Name: "my-cct"},
		}
		h := newTestHandler(withClusterComponentTypeService(cctSvc))
		_, err := h.CreateClusterComponentType(ctx, req)
		require.NoError(t, err)
	})

	t.Run("annotations cleaned", func(t *testing.T) {
		cctSvc := clustercomponenttypemocks.NewMockService(t)
		annotations := map[string]string{controller.AnnotationKeyDisplayName: ""}
		cctSvc.EXPECT().
			CreateClusterComponentType(mock.Anything, mock.MatchedBy(func(cct *openchoreov1alpha1.ClusterComponentType) bool {
				_, hasDisplay := cct.Annotations[controller.AnnotationKeyDisplayName]
				return !hasDisplay
			})).
			Return(makeCreated(), nil)

		req := &gen.CreateClusterComponentTypeJSONRequestBody{
			Metadata: gen.ObjectMeta{Name: "my-cct", Annotations: &annotations},
		}
		h := newTestHandler(withClusterComponentTypeService(cctSvc))
		_, err := h.CreateClusterComponentType(ctx, req)
		require.NoError(t, err)
	})
}
