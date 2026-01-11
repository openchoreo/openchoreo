// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"
	"encoding/json"

	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
	"github.com/openchoreo/openchoreo/internal/version"
)

// GetHealth returns OK if the server is healthy
func (h *Handler) GetHealth(
	ctx context.Context,
	request gen.GetHealthRequestObject,
) (gen.GetHealthResponseObject, error) {
	return gen.GetHealth200TextResponse("OK"), nil
}

// GetVersion returns version information about the API server
func (h *Handler) GetVersion(
	ctx context.Context,
	request gen.GetVersionRequestObject,
) (gen.GetVersionResponseObject, error) {
	info := version.Get()
	return gen.GetVersion200JSONResponse{
		Name:        info.Name,
		Version:     info.Version,
		GitRevision: info.GitRevision,
		BuildTime:   info.BuildTime,
		GoOS:        info.GoOS,
		GoArch:      info.GoArch,
		GoVersion:   info.GoVersion,
	}, nil
}

// GetOpenAPISpec returns the OpenAPI specification in JSON format
func (h *Handler) GetOpenAPISpec(
	ctx context.Context,
	request gen.GetOpenAPISpecRequestObject,
) (gen.GetOpenAPISpecResponseObject, error) {
	swagger, err := gen.GetSwagger()
	if err != nil {
		h.logger.Error("Failed to get OpenAPI spec", "error", err)
		return nil, err
	}

	// Convert to map for JSON response
	data, err := swagger.MarshalJSON()
	if err != nil {
		h.logger.Error("Failed to marshal OpenAPI spec", "error", err)
		return nil, err
	}

	var spec map[string]interface{}
	if err := json.Unmarshal(data, &spec); err != nil {
		h.logger.Error("Failed to unmarshal OpenAPI spec", "error", err)
		return nil, err
	}

	return gen.GetOpenAPISpec200JSONResponse(spec), nil
}

// GetReady returns Ready if the server is ready to accept requests
func (h *Handler) GetReady(
	ctx context.Context,
	request gen.GetReadyRequestObject,
) (gen.GetReadyResponseObject, error) {
	return nil, errNotImplemented
}
