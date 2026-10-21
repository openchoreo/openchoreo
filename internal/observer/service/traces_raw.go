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
		switch {
		case errors.Is(err, ErrRawTracesNotSupported),
			errors.Is(err, ErrRawTracesTimeout),
			errors.Is(err, ErrRawTracesResultTooLarge),
			errors.Is(err, ErrRawTracesRequestTooLarge),
			errors.Is(err, ErrRawTracesUnsupportedMediaType),
			errors.Is(err, ErrRawTracesRejected):
			return nil, err
		}
		s.logger.Error("Failed to run raw trace query", "error", err)
		return nil, fmt.Errorf("%w: %w", ErrTracesRetrieval, err)
	}

	return &types.RawTracesQueryResponse{Body: result.Body}, nil
}
