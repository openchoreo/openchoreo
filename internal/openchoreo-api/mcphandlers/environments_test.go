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
	environmentmocks "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/environment/mocks"
)

// ---------------------------------------------------------------------------
// CreateEnvironment
// ---------------------------------------------------------------------------

func TestCreateEnvironment(t *testing.T) {
	ctx := context.Background()

	makeEnv := func() *openchoreov1alpha1.Environment {
		return &openchoreov1alpha1.Environment{ObjectMeta: metav1.ObjectMeta{Name: testEnvironmentName, Namespace: testNS}}
	}

	t.Run("happy path with all fields", func(t *testing.T) {
		envSvc := environmentmocks.NewMockService(t)
		isProduction := true
		dpKind := gen.EnvironmentSpecDataPlaneRefKind("DataPlane")
		dpName := "my-dp"
		displayName := "Dev"
		description := "dev environment"
		annotations := map[string]string{
			"openchoreo.dev/display-name": displayName,
			"openchoreo.dev/description":  description,
		}
		envSvc.EXPECT().
			CreateEnvironment(mock.Anything, testNS, mock.MatchedBy(func(e *openchoreov1alpha1.Environment) bool {
				return e.Name == testEnvironmentName &&
					e.Spec.IsProduction == true &&
					e.Spec.DataPlaneRef != nil &&
					e.Spec.DataPlaneRef.Name == dpName &&
					e.Annotations["openchoreo.dev/display-name"] == displayName &&
					e.Annotations["openchoreo.dev/description"] == description
			})).
			Return(makeEnv(), nil)

		req := &gen.CreateEnvironmentJSONRequestBody{
			Metadata: gen.ObjectMeta{
				Name:        testEnvironmentName,
				Annotations: &annotations,
			},
			Spec: &gen.EnvironmentSpec{
				IsProduction: &isProduction,
				DataPlaneRef: &struct {
					Kind gen.EnvironmentSpecDataPlaneRefKind `json:"kind"`
					Name string                              `json:"name"`
				}{Kind: dpKind, Name: dpName},
			},
		}
		h := newTestHandler(withEnvironmentService(envSvc))
		_, err := h.CreateEnvironment(ctx, testNS, req)
		require.NoError(t, err)
	})

	t.Run("empty annotation values are cleaned up", func(t *testing.T) {
		envSvc := environmentmocks.NewMockService(t)
		annotations := map[string]string{
			"openchoreo.dev/display-name": "",
			"openchoreo.dev/description":  "",
		}
		envSvc.EXPECT().
			CreateEnvironment(mock.Anything, testNS, mock.MatchedBy(func(e *openchoreov1alpha1.Environment) bool {
				_, hasDisplay := e.Annotations["openchoreo.dev/display-name"]
				_, hasDesc := e.Annotations["openchoreo.dev/description"]
				return !hasDisplay && !hasDesc
			})).
			Return(makeEnv(), nil)

		req := &gen.CreateEnvironmentJSONRequestBody{
			Metadata: gen.ObjectMeta{Name: testEnvironmentName, Annotations: &annotations},
		}
		h := newTestHandler(withEnvironmentService(envSvc))
		_, err := h.CreateEnvironment(ctx, testNS, req)
		require.NoError(t, err)
	})

	t.Run("nil annotations: no panic", func(t *testing.T) {
		envSvc := environmentmocks.NewMockService(t)
		envSvc.EXPECT().
			CreateEnvironment(mock.Anything, testNS, mock.MatchedBy(func(e *openchoreov1alpha1.Environment) bool {
				return e.Name == testEnvironmentName
			})).
			Return(makeEnv(), nil)

		req := &gen.CreateEnvironmentJSONRequestBody{
			Metadata: gen.ObjectMeta{Name: testEnvironmentName},
		}
		h := newTestHandler(withEnvironmentService(envSvc))
		_, err := h.CreateEnvironment(ctx, testNS, req)
		require.NoError(t, err)
	})

	t.Run("nil spec: IsProduction defaults to false, no DataPlaneRef", func(t *testing.T) {
		envSvc := environmentmocks.NewMockService(t)
		envSvc.EXPECT().
			CreateEnvironment(mock.Anything, testNS, mock.MatchedBy(func(e *openchoreov1alpha1.Environment) bool {
				return e.Spec.IsProduction == false && e.Spec.DataPlaneRef == nil
			})).
			Return(makeEnv(), nil)

		req := &gen.CreateEnvironmentJSONRequestBody{
			Metadata: gen.ObjectMeta{Name: testEnvironmentName},
			Spec:     nil,
		}
		h := newTestHandler(withEnvironmentService(envSvc))
		_, err := h.CreateEnvironment(ctx, testNS, req)
		require.NoError(t, err)
	})

	t.Run("service error propagated", func(t *testing.T) {
		envSvc := environmentmocks.NewMockService(t)
		envSvc.EXPECT().CreateEnvironment(mock.Anything, testNS, mock.Anything).Return(nil, errors.New("create failed"))

		req := &gen.CreateEnvironmentJSONRequestBody{Metadata: gen.ObjectMeta{Name: testEnvironmentName}}
		h := newTestHandler(withEnvironmentService(envSvc))
		_, err := h.CreateEnvironment(ctx, testNS, req)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "create failed")
	})
}

// ---------------------------------------------------------------------------
// UpdateEnvironment
// ---------------------------------------------------------------------------

func TestUpdateEnvironment(t *testing.T) {
	ctx := context.Background()

	existingDP := &openchoreov1alpha1.DataPlaneRef{
		Kind: "DataPlane",
		Name: "my-dp",
	}
	freshExisting := func() *openchoreov1alpha1.Environment {
		return &openchoreov1alpha1.Environment{
			ObjectMeta: metav1.ObjectMeta{
				Name:        testEnvironmentName,
				Namespace:   testNS,
				Annotations: map[string]string{"existing-key": testExistingVal},
			},
			Spec: openchoreov1alpha1.EnvironmentSpec{
				DataPlaneRef: existingDP,
				IsProduction: false,
			},
		}
	}

	t.Run("immutable DataPlaneRef preserved on update", func(t *testing.T) {
		envSvc := environmentmocks.NewMockService(t)
		envSvc.EXPECT().GetEnvironment(mock.Anything, testNS, testEnvironmentName).Return(freshExisting(), nil)
		isProduction := true
		envSvc.EXPECT().
			UpdateEnvironment(mock.Anything, testNS, mock.MatchedBy(func(e *openchoreov1alpha1.Environment) bool {
				return e.Spec.DataPlaneRef != nil &&
					e.Spec.DataPlaneRef.Name == "my-dp" &&
					e.Spec.IsProduction == true
			})).
			Return(freshExisting(), nil)

		req := &gen.UpdateEnvironmentJSONRequestBody{
			Metadata: gen.ObjectMeta{Name: testEnvironmentName},
			Spec:     &gen.EnvironmentSpec{IsProduction: &isProduction},
		}
		h := newTestHandler(withEnvironmentService(envSvc))
		_, err := h.UpdateEnvironment(ctx, testNS, req)
		require.NoError(t, err)
	})

	t.Run("annotations merged not replaced", func(t *testing.T) {
		envSvc := environmentmocks.NewMockService(t)
		envSvc.EXPECT().GetEnvironment(mock.Anything, testNS, testEnvironmentName).Return(freshExisting(), nil)
		newAnnotations := map[string]string{"new-key": testNewVal}
		envSvc.EXPECT().
			UpdateEnvironment(mock.Anything, testNS, mock.MatchedBy(func(e *openchoreov1alpha1.Environment) bool {
				return e.Annotations["existing-key"] == testExistingVal &&
					e.Annotations["new-key"] == testNewVal
			})).
			Return(freshExisting(), nil)

		req := &gen.UpdateEnvironmentJSONRequestBody{
			Metadata: gen.ObjectMeta{Name: testEnvironmentName, Annotations: &newAnnotations},
		}
		h := newTestHandler(withEnvironmentService(envSvc))
		_, err := h.UpdateEnvironment(ctx, testNS, req)
		require.NoError(t, err)
	})

	t.Run("GetEnvironment error propagated", func(t *testing.T) {
		envSvc := environmentmocks.NewMockService(t)
		envSvc.EXPECT().GetEnvironment(mock.Anything, testNS, testEnvironmentName).Return(nil, errors.New("not found"))

		req := &gen.UpdateEnvironmentJSONRequestBody{Metadata: gen.ObjectMeta{Name: testEnvironmentName}}
		h := newTestHandler(withEnvironmentService(envSvc))
		_, err := h.UpdateEnvironment(ctx, testNS, req)
		require.Error(t, err)
	})
}

// ---------------------------------------------------------------------------
// DeleteEnvironment
// ---------------------------------------------------------------------------

func TestDeleteEnvironment(t *testing.T) {
	ctx := context.Background()

	t.Run("response has correct fields", func(t *testing.T) {
		envSvc := environmentmocks.NewMockService(t)
		envSvc.EXPECT().DeleteEnvironment(mock.Anything, testNS, testEnvironmentName).Return(nil)

		h := newTestHandler(withEnvironmentService(envSvc))
		result, err := h.DeleteEnvironment(ctx, testNS, testEnvironmentName)
		require.NoError(t, err)
		m, ok := result.(map[string]any)
		require.True(t, ok)
		assert.Equal(t, testEnvironmentName, m["name"])
		assert.Equal(t, testNS, m["namespace"])
		assert.Equal(t, "deleted", m["action"])
	})

	t.Run("service error propagated", func(t *testing.T) {
		envSvc := environmentmocks.NewMockService(t)
		envSvc.EXPECT().DeleteEnvironment(mock.Anything, testNS, testEnvironmentName).Return(errors.New("delete failed"))

		h := newTestHandler(withEnvironmentService(envSvc))
		_, err := h.DeleteEnvironment(ctx, testNS, testEnvironmentName)
		require.Error(t, err)
	})
}
