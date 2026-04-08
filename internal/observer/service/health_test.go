// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"context"
	"log/slog"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
}

func TestNewHealthService(t *testing.T) {
	t.Run("with logger", func(t *testing.T) {
		svc, err := NewHealthService(testLogger())
		require.NoError(t, err)
		assert.NotNil(t, svc)
	})

	t.Run("nil logger uses default", func(t *testing.T) {
		svc, err := NewHealthService(nil)
		require.NoError(t, err)
		assert.NotNil(t, svc)
	})
}

func TestHealthService_Check(t *testing.T) {
	svc, err := NewHealthService(testLogger())
	require.NoError(t, err)

	err = svc.Check(context.Background())
	assert.NoError(t, err)
}
