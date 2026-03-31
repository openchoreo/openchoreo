// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
)

func TestBadRequest(t *testing.T) {
	resp := badRequest("invalid input")
	assert.Equal(t, gen.BADREQUEST, resp.Code)
	assert.Equal(t, "invalid input", resp.Error)
}

func TestForbidden(t *testing.T) {
	resp := forbidden()
	assert.Equal(t, gen.FORBIDDEN, resp.Code)
	assert.Contains(t, resp.Error, "permission")
}

func TestNotFound(t *testing.T) {
	resp := notFound("Project")
	assert.Equal(t, gen.NOTFOUND, resp.Code)
	assert.Equal(t, "Project not found", resp.Error)
}

func TestConflict(t *testing.T) {
	resp := conflict("already exists")
	assert.Equal(t, gen.CONFLICT, resp.Code)
	assert.Equal(t, "already exists", resp.Error)
}

func TestInternalError(t *testing.T) {
	resp := internalError()
	assert.Equal(t, gen.INTERNALERROR, resp.Code)
	assert.Equal(t, "Internal server error", resp.Error)
}
