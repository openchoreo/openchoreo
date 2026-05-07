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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/controller"
	"github.com/openchoreo/openchoreo/internal/labels"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
	namespacemocks "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/namespace/mocks"
	secretreferencemocks "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/secretreference/mocks"
)

func TestCreateNamespace(t *testing.T) {
	ctx := context.Background()

	makeCreated := func() *corev1.Namespace {
		return &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "my-ns"}}
	}

	t.Run("control plane namespace label always added", func(t *testing.T) {
		nsSvc := namespacemocks.NewMockService(t)
		nsSvc.EXPECT().
			CreateNamespace(mock.Anything, mock.MatchedBy(func(ns *corev1.Namespace) bool {
				return ns.Labels[labels.LabelKeyControlPlaneNamespace] == labels.LabelValueTrue
			})).
			Return(makeCreated(), nil)

		req := &gen.CreateNamespaceJSONRequestBody{
			Metadata: gen.ObjectMeta{Name: "my-ns"},
		}
		h := newTestHandler(withNamespaceService(nsSvc))
		_, err := h.CreateNamespace(ctx, req)
		require.NoError(t, err)
	})

	t.Run("custom labels merged with control plane label", func(t *testing.T) {
		nsSvc := namespacemocks.NewMockService(t)
		customLabels := map[string]string{"team": "platform"}
		nsSvc.EXPECT().
			CreateNamespace(mock.Anything, mock.MatchedBy(func(ns *corev1.Namespace) bool {
				return ns.Labels["team"] == "platform" &&
					ns.Labels[labels.LabelKeyControlPlaneNamespace] == labels.LabelValueTrue
			})).
			Return(makeCreated(), nil)

		req := &gen.CreateNamespaceJSONRequestBody{
			Metadata: gen.ObjectMeta{Name: "my-ns", Labels: &customLabels},
		}
		h := newTestHandler(withNamespaceService(nsSvc))
		_, err := h.CreateNamespace(ctx, req)
		require.NoError(t, err)
	})

	t.Run("empty annotation values cleaned", func(t *testing.T) {
		nsSvc := namespacemocks.NewMockService(t)
		annotations := map[string]string{
			"openchoreo.dev/display-name": "",
		}
		nsSvc.EXPECT().
			CreateNamespace(mock.Anything, mock.MatchedBy(func(ns *corev1.Namespace) bool {
				_, hasDisplay := ns.Annotations["openchoreo.dev/display-name"]
				return !hasDisplay
			})).
			Return(makeCreated(), nil)

		req := &gen.CreateNamespaceJSONRequestBody{
			Metadata: gen.ObjectMeta{Name: "my-ns", Annotations: &annotations},
		}
		h := newTestHandler(withNamespaceService(nsSvc))
		_, err := h.CreateNamespace(ctx, req)
		require.NoError(t, err)
	})

	t.Run("nil annotations and labels: no panic", func(t *testing.T) {
		nsSvc := namespacemocks.NewMockService(t)
		nsSvc.EXPECT().CreateNamespace(mock.Anything, mock.Anything).Return(makeCreated(), nil)

		req := &gen.CreateNamespaceJSONRequestBody{
			Metadata: gen.ObjectMeta{Name: "my-ns"},
		}
		h := newTestHandler(withNamespaceService(nsSvc))
		_, err := h.CreateNamespace(ctx, req)
		require.NoError(t, err)
	})

	t.Run("control plane label cannot be overridden by custom label", func(t *testing.T) {
		nsSvc := namespacemocks.NewMockService(t)
		// Even if user passes a false value for the control plane label, it must be overwritten to true.
		customLabels := map[string]string{labels.LabelKeyControlPlaneNamespace: "false"}
		nsSvc.EXPECT().
			CreateNamespace(mock.Anything, mock.MatchedBy(func(ns *corev1.Namespace) bool {
				return ns.Labels[labels.LabelKeyControlPlaneNamespace] == labels.LabelValueTrue
			})).
			Return(makeCreated(), nil)

		req := &gen.CreateNamespaceJSONRequestBody{
			Metadata: gen.ObjectMeta{Name: "my-ns", Labels: &customLabels},
		}
		h := newTestHandler(withNamespaceService(nsSvc))
		_, err := h.CreateNamespace(ctx, req)
		assert.NoError(t, err)
	})
}

// ---------------------------------------------------------------------------
// SecretReference handlers
// ---------------------------------------------------------------------------

const testSecretRefNS = "org-ns"
const testSecretRefName = "db-creds"

func makeSecretRefSpec() *gen.SecretReferenceSpec {
	opaque := gen.SecretTemplateTypeOpaque
	return &gen.SecretReferenceSpec{
		Template: gen.SecretTemplate{Type: &opaque},
		Data: []gen.SecretDataSource{
			{
				SecretKey: "password",
				RemoteRef: gen.RemoteReference{Key: "prod/db/password"},
			},
		},
	}
}

func TestGetSecretReference(t *testing.T) {
	ctx := context.Background()

	t.Run("returns detail with spec", func(t *testing.T) {
		opaque := corev1.SecretTypeOpaque
		existing := &openchoreov1alpha1.SecretReference{
			ObjectMeta: metav1.ObjectMeta{Name: testSecretRefName, Namespace: testSecretRefNS},
			Spec: openchoreov1alpha1.SecretReferenceSpec{
				Template: openchoreov1alpha1.SecretTemplate{Type: opaque},
				Data: []openchoreov1alpha1.SecretDataSource{
					{SecretKey: "password", RemoteRef: openchoreov1alpha1.RemoteReference{Key: "prod/db/password"}},
				},
			},
		}
		srSvc := secretreferencemocks.NewMockService(t)
		srSvc.EXPECT().GetSecretReference(mock.Anything, testSecretRefNS, testSecretRefName).Return(existing, nil)

		h := newTestHandler(withSecretReferenceService(srSvc))
		result, err := h.GetSecretReference(ctx, testSecretRefNS, testSecretRefName)
		require.NoError(t, err)

		m, ok := result.(map[string]any)
		require.True(t, ok)
		assert.Equal(t, testSecretRefName, m["name"])
		spec, ok := m["spec"].(map[string]any)
		require.True(t, ok, "expected spec map in detail")
		assert.NotNil(t, spec["template"])
		assert.NotNil(t, spec["data"])
	})

	t.Run("not found error propagated", func(t *testing.T) {
		srSvc := secretreferencemocks.NewMockService(t)
		srSvc.EXPECT().GetSecretReference(mock.Anything, testSecretRefNS, "missing").Return(nil, errors.New("not found"))

		h := newTestHandler(withSecretReferenceService(srSvc))
		_, err := h.GetSecretReference(ctx, testSecretRefNS, "missing")
		require.Error(t, err)
	})
}

func TestCreateSecretReference(t *testing.T) {
	ctx := context.Background()

	t.Run("creates with spec, display name, description as annotations", func(t *testing.T) {
		srSvc := secretreferencemocks.NewMockService(t)
		srSvc.EXPECT().
			CreateSecretReference(mock.Anything, testSecretRefNS, mock.MatchedBy(func(sr *openchoreov1alpha1.SecretReference) bool {
				return sr.Name == testSecretRefName &&
					sr.Annotations[controller.AnnotationKeyDisplayName] == "DB Creds" &&
					sr.Annotations[controller.AnnotationKeyDescription] == "production database credentials" &&
					sr.Spec.Template.Type == corev1.SecretTypeOpaque &&
					len(sr.Spec.Data) == 1 &&
					sr.Spec.Data[0].SecretKey == "password" &&
					sr.Spec.Data[0].RemoteRef.Key == "prod/db/password"
			})).
			Return(&openchoreov1alpha1.SecretReference{ObjectMeta: metav1.ObjectMeta{Name: testSecretRefName}}, nil)

		annotations := map[string]string{
			controller.AnnotationKeyDisplayName: "DB Creds",
			controller.AnnotationKeyDescription: "production database credentials",
		}
		req := &gen.CreateSecretReferenceJSONRequestBody{
			Metadata: gen.ObjectMeta{Name: testSecretRefName, Annotations: &annotations},
			Spec:     makeSecretRefSpec(),
		}

		h := newTestHandler(withSecretReferenceService(srSvc))
		result, err := h.CreateSecretReference(ctx, testSecretRefNS, req)
		require.NoError(t, err)

		m, ok := result.(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "created", m["action"])
	})

	t.Run("strips empty display_name / description annotations", func(t *testing.T) {
		srSvc := secretreferencemocks.NewMockService(t)
		srSvc.EXPECT().
			CreateSecretReference(mock.Anything, testSecretRefNS, mock.MatchedBy(func(sr *openchoreov1alpha1.SecretReference) bool {
				_, hasDN := sr.Annotations[controller.AnnotationKeyDisplayName]
				_, hasDesc := sr.Annotations[controller.AnnotationKeyDescription]
				return !hasDN && !hasDesc
			})).
			Return(&openchoreov1alpha1.SecretReference{ObjectMeta: metav1.ObjectMeta{Name: testSecretRefName}}, nil)

		annotations := map[string]string{
			controller.AnnotationKeyDisplayName: "",
			controller.AnnotationKeyDescription: "",
		}
		req := &gen.CreateSecretReferenceJSONRequestBody{
			Metadata: gen.ObjectMeta{Name: testSecretRefName, Annotations: &annotations},
			Spec:     makeSecretRefSpec(),
		}

		h := newTestHandler(withSecretReferenceService(srSvc))
		_, err := h.CreateSecretReference(ctx, testSecretRefNS, req)
		require.NoError(t, err)
	})

	t.Run("service create error propagated", func(t *testing.T) {
		srSvc := secretreferencemocks.NewMockService(t)
		srSvc.EXPECT().CreateSecretReference(mock.Anything, testSecretRefNS, mock.Anything).
			Return(nil, errors.New("already exists"))

		req := &gen.CreateSecretReferenceJSONRequestBody{
			Metadata: gen.ObjectMeta{Name: testSecretRefName},
			Spec:     makeSecretRefSpec(),
		}

		h := newTestHandler(withSecretReferenceService(srSvc))
		_, err := h.CreateSecretReference(ctx, testSecretRefNS, req)
		require.Error(t, err)
	})
}

func TestUpdateSecretReference(t *testing.T) {
	ctx := context.Background()

	freshExisting := func() *openchoreov1alpha1.SecretReference {
		return &openchoreov1alpha1.SecretReference{
			ObjectMeta: metav1.ObjectMeta{
				Name:      testSecretRefName,
				Namespace: testSecretRefNS,
				Annotations: map[string]string{
					controller.AnnotationKeyDisplayName: "Original",
					"unrelated":                         "kept",
				},
			},
			Spec: openchoreov1alpha1.SecretReferenceSpec{
				Template: openchoreov1alpha1.SecretTemplate{Type: corev1.SecretTypeOpaque},
				Data: []openchoreov1alpha1.SecretDataSource{
					{SecretKey: "old-key", RemoteRef: openchoreov1alpha1.RemoteReference{Key: "old/path"}},
				},
			},
		}
	}

	t.Run("replaces spec when provided", func(t *testing.T) {
		existing := freshExisting()
		srSvc := secretreferencemocks.NewMockService(t)
		srSvc.EXPECT().GetSecretReference(mock.Anything, testSecretRefNS, testSecretRefName).Return(existing, nil)
		srSvc.EXPECT().
			UpdateSecretReference(mock.Anything, testSecretRefNS, mock.MatchedBy(func(sr *openchoreov1alpha1.SecretReference) bool {
				return len(sr.Spec.Data) == 1 &&
					sr.Spec.Data[0].SecretKey == "password" &&
					sr.Spec.Data[0].RemoteRef.Key == "prod/db/password"
			})).
			Return(existing, nil)

		req := &gen.UpdateSecretReferenceJSONRequestBody{
			Metadata: gen.ObjectMeta{Name: testSecretRefName},
			Spec:     makeSecretRefSpec(),
		}

		h := newTestHandler(withSecretReferenceService(srSvc))
		_, err := h.UpdateSecretReference(ctx, testSecretRefNS, req)
		require.NoError(t, err)
	})

	t.Run("nil spec leaves spec unchanged", func(t *testing.T) {
		existing := freshExisting()
		srSvc := secretreferencemocks.NewMockService(t)
		srSvc.EXPECT().GetSecretReference(mock.Anything, testSecretRefNS, testSecretRefName).Return(existing, nil)
		srSvc.EXPECT().
			UpdateSecretReference(mock.Anything, testSecretRefNS, mock.MatchedBy(func(sr *openchoreov1alpha1.SecretReference) bool {
				return len(sr.Spec.Data) == 1 && sr.Spec.Data[0].SecretKey == "old-key"
			})).
			Return(existing, nil)

		req := &gen.UpdateSecretReferenceJSONRequestBody{
			Metadata: gen.ObjectMeta{Name: testSecretRefName},
		}

		h := newTestHandler(withSecretReferenceService(srSvc))
		_, err := h.UpdateSecretReference(ctx, testSecretRefNS, req)
		require.NoError(t, err)
	})

	t.Run("merges annotations and preserves unrelated ones", func(t *testing.T) {
		existing := freshExisting()
		srSvc := secretreferencemocks.NewMockService(t)
		srSvc.EXPECT().GetSecretReference(mock.Anything, testSecretRefNS, testSecretRefName).Return(existing, nil)
		srSvc.EXPECT().
			UpdateSecretReference(mock.Anything, testSecretRefNS, mock.MatchedBy(func(sr *openchoreov1alpha1.SecretReference) bool {
				return sr.Annotations[controller.AnnotationKeyDisplayName] == "New Display" &&
					sr.Annotations["unrelated"] == "kept"
			})).
			Return(existing, nil)

		annotations := map[string]string{controller.AnnotationKeyDisplayName: "New Display"}
		req := &gen.UpdateSecretReferenceJSONRequestBody{
			Metadata: gen.ObjectMeta{Name: testSecretRefName, Annotations: &annotations},
		}

		h := newTestHandler(withSecretReferenceService(srSvc))
		_, err := h.UpdateSecretReference(ctx, testSecretRefNS, req)
		require.NoError(t, err)
	})

	t.Run("get error propagated", func(t *testing.T) {
		srSvc := secretreferencemocks.NewMockService(t)
		srSvc.EXPECT().GetSecretReference(mock.Anything, testSecretRefNS, testSecretRefName).
			Return(nil, errors.New("not found"))

		req := &gen.UpdateSecretReferenceJSONRequestBody{
			Metadata: gen.ObjectMeta{Name: testSecretRefName},
		}

		h := newTestHandler(withSecretReferenceService(srSvc))
		_, err := h.UpdateSecretReference(ctx, testSecretRefNS, req)
		require.Error(t, err)
	})
}

func TestDeleteSecretReference(t *testing.T) {
	ctx := context.Background()

	t.Run("deletes and returns action: deleted", func(t *testing.T) {
		srSvc := secretreferencemocks.NewMockService(t)
		srSvc.EXPECT().DeleteSecretReference(mock.Anything, testSecretRefNS, testSecretRefName).Return(nil)

		h := newTestHandler(withSecretReferenceService(srSvc))
		result, err := h.DeleteSecretReference(ctx, testSecretRefNS, testSecretRefName)
		require.NoError(t, err)

		m, ok := result.(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "deleted", m["action"])
		assert.Equal(t, testSecretRefName, m["name"])
		assert.Equal(t, testSecretRefNS, m["namespace"])
	})

	t.Run("service delete error propagated", func(t *testing.T) {
		expected := errors.New("conflict")
		srSvc := secretreferencemocks.NewMockService(t)
		srSvc.EXPECT().DeleteSecretReference(mock.Anything, testSecretRefNS, testSecretRefName).
			Return(expected)

		h := newTestHandler(withSecretReferenceService(srSvc))
		_, err := h.DeleteSecretReference(ctx, testSecretRefNS, testSecretRefName)
		require.ErrorIs(t, err, expected)
	})
}
