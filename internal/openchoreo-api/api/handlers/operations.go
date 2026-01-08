// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"

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
