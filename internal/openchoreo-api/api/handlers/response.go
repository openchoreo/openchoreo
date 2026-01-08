// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import "github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"

// Error response helpers - create the error response types

func badRequest(message string) gen.BadRequestJSONResponse {
	return gen.BadRequestJSONResponse{
		Code:  gen.BADREQUEST,
		Error: message,
	}
}

func forbidden() gen.ForbiddenJSONResponse {
	return gen.ForbiddenJSONResponse{
		Code:  gen.FORBIDDEN,
		Error: "You do not have permission to access this resource",
	}
}

func notFound(resource string) gen.NotFoundJSONResponse {
	return gen.NotFoundJSONResponse{
		Code:  gen.NOTFOUND,
		Error: resource + " not found",
	}
}

func conflict(message string) gen.ConflictJSONResponse {
	return gen.ConflictJSONResponse{
		Code:  gen.CONFLICT,
		Error: message,
	}
}

func internalError() gen.InternalErrorJSONResponse {
	return gen.InternalErrorJSONResponse{
		Code:  gen.INTERNALERROR,
		Error: "Internal server error",
	}
}
