// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package mcphandlers

import (
	"context"
	"errors"
	"fmt"
	"slices"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
	rolloutsvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/endpointrollout"
	"github.com/openchoreo/openchoreo/internal/server/middleware/audit"
)

// ---------------------------------------------------------------------------
// Endpoint rollouts
// ---------------------------------------------------------------------------

func (h *MCPHandler) GetEndpointRollout(ctx context.Context, namespaceName, endpointName string) (any, error) {
	return h.services.EndpointRolloutService.Get(ctx, namespaceName, endpointName)
}

// StartEndpointRollout writes the plan. A candidate named without a component
// is one the Endpoint already has, so its component is taken from there.
func (h *MCPHandler) StartEndpointRollout(
	ctx context.Context, namespaceName, endpointName string, req *gen.EndpointRolloutStartRequest,
) (any, error) {
	if req == nil {
		return nil, errors.New("request body is required")
	}
	if req.Backends != nil {
		var missing []int
		for i, b := range *req.Backends {
			if b.Component == nil {
				missing = append(missing, i)
			}
		}
		// The service keeps the stored component, release line included, for a
		// backend given by name; here only check that the Endpoint has it.
		if len(missing) > 0 {
			ep, err := h.services.EndpointService.GetEndpoint(ctx, namespaceName, endpointName)
			if err != nil {
				return nil, err
			}
			for _, i := range missing {
				b := (*req.Backends)[i]
				found := slices.ContainsFunc(ep.Spec.BackendRefs, func(ref openchoreov1alpha1.EndpointBackendRef) bool {
					return ref.Name == b.Name && ref.Component != nil
				})
				if !found {
					return nil, &services.ValidationError{Msg: fmt.Sprintf(
						"endpoint %q has no backend %q; give the candidate's component and endpoint to add it",
						endpointName, b.Name)}
				}
			}
		}
	}
	in, err := convertSpec[gen.EndpointRolloutStartRequest, rolloutsvc.StartRequest](*req)
	if err != nil {
		return nil, err
	}
	status, err := h.services.EndpointRolloutService.Start(ctx, namespaceName, endpointName, in)
	if err != nil {
		return nil, err
	}
	audit.SetResource(ctx, &audit.Resource{Namespace: namespaceName, Name: endpointName})
	return status, nil
}

func (h *MCPHandler) StepEndpointRollout(
	ctx context.Context, namespaceName, endpointName string, req *gen.EndpointRolloutStepRequest,
) (any, error) {
	if req == nil {
		return nil, errors.New("request body is required")
	}
	in, err := convertSpec[gen.EndpointRolloutStepRequest, rolloutsvc.StepRequest](*req)
	if err != nil {
		return nil, err
	}
	status, err := h.services.EndpointRolloutService.Step(ctx, namespaceName, endpointName, in)
	if err != nil {
		return nil, err
	}
	audit.SetResource(ctx, &audit.Resource{Namespace: namespaceName, Name: endpointName})
	return status, nil
}

func (h *MCPHandler) AbortEndpointRollout(
	ctx context.Context, namespaceName, endpointName string, req *gen.EndpointRolloutAbortRequest,
) (any, error) {
	if req == nil {
		return nil, errors.New("request body is required")
	}
	in, err := convertSpec[gen.EndpointRolloutAbortRequest, rolloutsvc.AbortRequest](*req)
	if err != nil {
		return nil, err
	}
	status, err := h.services.EndpointRolloutService.Abort(ctx, namespaceName, endpointName, in)
	if err != nil {
		return nil, err
	}
	audit.SetResource(ctx, &audit.Resource{Namespace: namespaceName, Name: endpointName})
	return status, nil
}

func (h *MCPHandler) FinishEndpointRollout(
	ctx context.Context, namespaceName, endpointName string, req *gen.EndpointRolloutFinishRequest,
) (any, error) {
	if req == nil {
		return nil, errors.New("request body is required")
	}
	in, err := convertSpec[gen.EndpointRolloutFinishRequest, rolloutsvc.FinishRequest](*req)
	if err != nil {
		return nil, err
	}
	status, err := h.services.EndpointRolloutService.Finish(ctx, namespaceName, endpointName, in)
	if err != nil {
		return nil, err
	}
	audit.SetResource(ctx, &audit.Resource{Namespace: namespaceName, Name: endpointName})
	return status, nil
}
