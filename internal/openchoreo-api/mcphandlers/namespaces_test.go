// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package mcphandlers

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/openchoreo/openchoreo/internal/labels"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
	namespacemocks "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/namespace/mocks"
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
