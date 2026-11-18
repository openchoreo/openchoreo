// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
	endpointsvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/endpoint"
	rolloutsvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/endpointrollout"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/handlerservices"
)

type stubRolloutService struct {
	rolloutsvc.Service
	err  error
	step rolloutsvc.StepRequest
}

func (s *stubRolloutService) Step(_ context.Context, _, name string, req rolloutsvc.StepRequest) (*rolloutsvc.Status, error) {
	s.step = req
	if s.err != nil {
		return nil, s.err
	}
	step := 1
	return &rolloutsvc.Status{Endpoint: name, Environments: []rolloutsvc.EnvironmentStatus{{Environment: req.Environment, Step: &step}}}, nil
}

func stepRequest(env string, expected *int) gen.StepEndpointRolloutRequestObject {
	return gen.StepEndpointRolloutRequestObject{
		NamespaceName: "ns",
		EndpointName:  "greeter-api",
		Body:          &gen.EndpointRolloutStepRequest{Environment: env, ExpectedStep: expected},
	}
}

func TestStepEndpointRolloutHandler(t *testing.T) {
	ctx := context.Background()
	handler := func(svc rolloutsvc.Service) *Handler {
		return &Handler{services: &handlerservices.Services{EndpointRolloutService: svc}, logger: slog.Default()}
	}

	t.Run("passes the request through and returns the status", func(t *testing.T) {
		svc := &stubRolloutService{}
		expected := 0
		resp, err := handler(svc).StepEndpointRollout(ctx, stepRequest("dev", &expected))
		require.NoError(t, err)
		typed, ok := resp.(gen.StepEndpointRollout200JSONResponse)
		require.True(t, ok, "expected 200 response, got %T", resp)
		assert.Equal(t, 1, *typed.Environments[0].Step)
		assert.Equal(t, "dev", svc.step.Environment)
		assert.Equal(t, 0, *svc.step.ExpectedStep)
	})

	errorCases := []struct {
		name string
		err  error
		want any
	}{
		{"stale step is a conflict", &rolloutsvc.ConflictError{Msg: "dev is at step 1, not step 0"}, gen.StepEndpointRollout409JSONResponse{}},
		{"no plan is a conflict", rolloutsvc.ErrNoRollout, gen.StepEndpointRollout409JSONResponse{}},
		{"unknown environment is not found", rolloutsvc.ErrEnvironmentNotBound, gen.StepEndpointRollout404JSONResponse{}},
		{"unknown endpoint is not found", endpointsvc.ErrEndpointNotFound, gen.StepEndpointRollout404JSONResponse{}},
		{"forbidden", services.ErrForbidden, gen.StepEndpointRollout403JSONResponse{}},
		{"invalid", &services.ValidationError{Msg: "step must be from 0 to 2"}, gen.StepEndpointRollout400JSONResponse{}},
	}
	for _, tc := range errorCases {
		t.Run(tc.name, func(t *testing.T) {
			resp, err := handler(&stubRolloutService{err: tc.err}).StepEndpointRollout(ctx, stepRequest("dev", nil))
			require.NoError(t, err)
			assert.IsType(t, tc.want, resp)
		})
	}

	t.Run("nil body returns 400", func(t *testing.T) {
		resp, err := handler(&stubRolloutService{}).StepEndpointRollout(ctx, gen.StepEndpointRolloutRequestObject{})
		require.NoError(t, err)
		assert.IsType(t, gen.StepEndpointRollout400JSONResponse{}, resp)
	})
}
