// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package mcphandlers

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/utils/ptr"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
	endpointmocks "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/endpoint/mocks"
	rolloutsvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/endpointrollout"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/handlerservices"
)

// fakeRolloutService records the start request it was given.
type fakeRolloutService struct {
	rolloutsvc.Service
	started *rolloutsvc.StartRequest
	stepped *rolloutsvc.StepRequest
}

func (f *fakeRolloutService) Start(_ context.Context, _, endpointName string, req rolloutsvc.StartRequest) (*rolloutsvc.Status, error) {
	f.started = &req
	return &rolloutsvc.Status{Endpoint: endpointName}, nil
}

func (f *fakeRolloutService) Step(_ context.Context, _, endpointName string, req rolloutsvc.StepRequest) (*rolloutsvc.Status, error) {
	f.stepped = &req
	return &rolloutsvc.Status{Endpoint: endpointName}, nil
}

func withEndpointRolloutService(s rolloutsvc.Service) func(*handlerservices.Services) {
	return func(svc *handlerservices.Services) { svc.EndpointRolloutService = s }
}

func rolloutStart(candidate gen.EndpointBackendRef) *gen.EndpointRolloutStartRequest {
	ro := gen.EndpointRollout{Strategy: gen.EndpointRolloutStrategyBlueGreen, Stable: "primary"}
	ro.Candidates = append(ro.Candidates, struct {
		Name string `json:"name"`
	}{Name: candidate.Name})
	backends := []gen.EndpointBackendRef{candidate}
	return &gen.EndpointRolloutStartRequest{Rollout: ro, Backends: &backends}
}

func TestStartEndpointRollout(t *testing.T) {
	ctx := context.Background()
	existing := sampleEndpoint()
	existing.Spec.BackendRefs = append(existing.Spec.BackendRefs, openchoreov1alpha1.EndpointBackendRef{
		Name:      "green",
		Kind:      openchoreov1alpha1.EndpointBackendKindComponent,
		Component: &openchoreov1alpha1.EndpointComponentBackend{Name: "orders-v2", Endpoint: "http"},
	})

	t.Run("leaves a named candidate's component to the service, release line included", func(t *testing.T) {
		eps := endpointmocks.NewMockService(t)
		eps.EXPECT().GetEndpoint(ctx, testNS, testEndpointName).Return(existing, nil)
		rollouts := &fakeRolloutService{}
		h := newTestHandler(withEndpointService(eps), withEndpointRolloutService(rollouts))

		_, err := h.StartEndpointRollout(ctx, testNS, testEndpointName,
			rolloutStart(gen.EndpointBackendRef{Name: "green", Weight: ptr.To[int32](0)}))
		require.NoError(t, err)

		require.NotNil(t, rollouts.started)
		require.Len(t, rollouts.started.Backends, 1)
		got := rollouts.started.Backends[0]
		assert.Nil(t, got.Component, "the service copies the stored component, release line and all")
		assert.Equal(t, ptr.To[int32](0), got.Weight)
		assert.Equal(t, "primary", rollouts.started.Rollout.Stable)
	})

	t.Run("refuses a candidate the endpoint doesn't have and gives no component for", func(t *testing.T) {
		eps := endpointmocks.NewMockService(t)
		eps.EXPECT().GetEndpoint(ctx, testNS, testEndpointName).Return(existing, nil)
		rollouts := &fakeRolloutService{}
		h := newTestHandler(withEndpointService(eps), withEndpointRolloutService(rollouts))

		_, err := h.StartEndpointRollout(ctx, testNS, testEndpointName, rolloutStart(gen.EndpointBackendRef{Name: "blue"}))
		var validationErr *services.ValidationError
		require.ErrorAs(t, err, &validationErr)
		assert.Nil(t, rollouts.started)
	})

	t.Run("adds a candidate given its component without reading the endpoint", func(t *testing.T) {
		rollouts := &fakeRolloutService{}
		h := newTestHandler(withEndpointRolloutService(rollouts))

		_, err := h.StartEndpointRollout(ctx, testNS, testEndpointName, rolloutStart(gen.EndpointBackendRef{
			Name:      "blue",
			Component: &gen.EndpointComponentBackend{Name: "orders-v3", Endpoint: "http"},
		}))
		require.NoError(t, err)
		assert.Equal(t, "orders-v3", rollouts.started.Backends[0].Component.Name)
	})
}

func TestStepEndpointRollout(t *testing.T) {
	rollouts := &fakeRolloutService{}
	h := newTestHandler(withEndpointRolloutService(rollouts))

	_, err := h.StepEndpointRollout(context.Background(), testNS, testEndpointName, &gen.EndpointRolloutStepRequest{
		Environment: testEndpointEnvironment, ExpectedStep: ptr.To(1), DryRun: ptr.To(true),
	})
	require.NoError(t, err)
	assert.Equal(t, rolloutsvc.StepRequest{Environment: testEndpointEnvironment, ExpectedStep: ptr.To(1), DryRun: true},
		*rollouts.stepped)
}
