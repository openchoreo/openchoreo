// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/openchoreo/openchoreo/internal/observer/types"
	"github.com/openchoreo/openchoreo/pkg/observability"
)

// QueryRawTraces runs a read-only request, written for the tracing backend's
// own API, through the tracing adapter and returns the backend's response. The
// request is not scoped; the caller's authorization is checked by the authz
// wrapper at cluster scope.
func (s *TracesService) QueryRawTraces(
	ctx context.Context,
	req *types.RawTracesQueryRequest,
) (*types.RawTracesQueryResponse, error) {
	if req == nil || len(req.Body) == 0 {
		return nil, fmt.Errorf("%w: a request body is required", ErrRawTracesInvalidRequest)
	}

	result, err := s.tracingAdapter.QueryRawTraces(ctx, observability.RawTracesQueryParams{
		ContentType: req.ContentType,
		Body:        req.Body,
	})
	if err != nil {
		var noRoute *rawTracesNoRouteError
		if errors.As(err, &noRoute) {
			s.logger.Warn("The tracing adapter has no raw query route; answering that raw queries are not supported. "+
				"If the tracing module should support them, check its version and the tracing adapter URL",
				"status", noRoute.status)
		}
		switch {
		case errors.Is(err, ErrRawTracesNotSupported),
			errors.Is(err, ErrRawTracesTimeout),
			errors.Is(err, ErrRawTracesResultTooLarge),
			errors.Is(err, ErrRawTracesRequestTooLarge),
			errors.Is(err, ErrRawTracesUnsupportedMediaType),
			errors.Is(err, ErrRawTracesRejected),
			errors.Is(err, context.Canceled):
			return nil, err
		}
		// Logged by the handler, which answers it.
		return nil, fmt.Errorf("%w: %w", ErrTracesRetrieval, err)
	}

	return &types.RawTracesQueryResponse{Body: result.Body}, nil
}
