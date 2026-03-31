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
)

func newMinimalHandler() *Handler {
	return &Handler{logger: slog.Default()}
}

func TestGetHealth(t *testing.T) {
	h := newMinimalHandler()
	resp, err := h.GetHealth(context.Background(), gen.GetHealthRequestObject{})
	require.NoError(t, err)
	typed, ok := resp.(gen.GetHealth200TextResponse)
	require.True(t, ok, "expected 200 text response, got %T", resp)
	assert.Equal(t, "OK", string(typed))
}

func TestGetReady(t *testing.T) {
	h := newMinimalHandler()
	resp, err := h.GetReady(context.Background(), gen.GetReadyRequestObject{})
	require.NoError(t, err)
	typed, ok := resp.(gen.GetReady200TextResponse)
	require.True(t, ok, "expected 200 text response, got %T", resp)
	assert.Equal(t, "Ready", string(typed))
}

func TestGetVersion(t *testing.T) {
	h := newMinimalHandler()
	resp, err := h.GetVersion(context.Background(), gen.GetVersionRequestObject{})
	require.NoError(t, err)
	_, ok := resp.(gen.GetVersion200JSONResponse)
	require.True(t, ok, "expected 200 JSON response, got %T", resp)
}

func TestGetOpenAPISpec(t *testing.T) {
	h := newMinimalHandler()
	resp, err := h.GetOpenAPISpec(context.Background(), gen.GetOpenAPISpecRequestObject{})
	require.NoError(t, err)
	typed, ok := resp.(gen.GetOpenAPISpec200JSONResponse)
	require.True(t, ok, "expected 200 JSON response, got %T", resp)
	assert.NotEmpty(t, typed)
}
