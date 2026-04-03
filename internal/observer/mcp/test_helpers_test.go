// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package mcp

import (
	"log/slog"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/openchoreo/openchoreo/internal/observer/service"
	servicemocks "github.com/openchoreo/openchoreo/internal/observer/service/mocks"
)

// handlerTestDeps holds dependencies for building an MCPHandler in unit tests.
type handlerTestDeps struct {
	logs    service.LogsQuerier
	metrics service.MetricsQuerier
	alerts  service.AlertIncidentService
	traces  service.TracesQuerier
}

// newTestMCPHandler builds an MCPHandler with mockery mocks by default; options override individual deps.
func newTestMCPHandler(t *testing.T, opts ...func(*handlerTestDeps)) *MCPHandler {
	t.Helper()

	d := handlerTestDeps{
		logs:    servicemocks.NewMockLogsQuerier(t),
		metrics: servicemocks.NewMockMetricsQuerier(t),
		alerts:  servicemocks.NewMockAlertIncidentService(t),
		traces:  servicemocks.NewMockTracesQuerier(t),
	}
	for _, o := range opts {
		o(&d)
	}

	logger := slog.Default()
	healthSvc, err := service.NewHealthService(logger)
	require.NoError(t, err)

	h, err := NewMCPHandler(healthSvc, d.logs, d.metrics, d.alerts, d.traces, logger)
	require.NoError(t, err)
	return h
}

func withLogsService(s service.LogsQuerier) func(*handlerTestDeps) {
	return func(d *handlerTestDeps) { d.logs = s }
}

func withMetricsService(s service.MetricsQuerier) func(*handlerTestDeps) {
	return func(d *handlerTestDeps) { d.metrics = s }
}

func withAlertIncidentService(s service.AlertIncidentService) func(*handlerTestDeps) {
	return func(d *handlerTestDeps) { d.alerts = s }
}

func withTracesService(s service.TracesQuerier) func(*handlerTestDeps) {
	return func(d *handlerTestDeps) { d.traces = s }
}
