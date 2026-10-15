// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package k8sresources_test

import (
	"bytes"
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	coreconfig "github.com/openchoreo/openchoreo/internal/config"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/models"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/testutil"
	"github.com/openchoreo/openchoreo/internal/server/middleware/audit"
)

const leakPodLogLine = "API_TOKEN=should-not-leak"

func TestGetResourceLogs_RecordsRequestAndCount(t *testing.T) {
	mockSvc, svc := newAuthzSvcWithRB(t, testutil.AllowPDP())
	since := int64(3600)
	mockSvc.On("GetResourceLogs", mock.Anything, "ns-1", "rb-1", "pod-1", "api", &since).
		Return(&models.ResourcePodLogsResponse{LogEntries: []models.PodLogEntry{
			{Log: leakPodLogLine}, {Log: "second"},
		}}, nil)

	ctx, data := audit.NewAuditContext(testutil.AuthzContext(), &audit.Resource{}, audit.RequestInfo{})
	_, err := svc.GetResourceLogs(ctx, "ns-1", "rb-1", "pod-1", "api", &since)
	require.NoError(t, err)

	require.Equal(t, map[string]any{"podName": "pod-1", "container": "api", "sinceSeconds": int64(3600)},
		data.Parameters)
	require.NotNil(t, data.Returned)
	require.Equal(t, audit.Returned{Count: 2}, *data.Returned)

	record := publishedLogRecord(t, ctx, data)
	require.Contains(t, record, `"request":{"parameters":{"container":"api","podName":"pod-1","sinceSeconds":3600}}`)
	require.Contains(t, record, `"returned":{"count":2}`)
	require.NotContains(t, record, leakPodLogLine)
}

func TestGetResourceLogs_DeniedRecordsRequestOnly(t *testing.T) {
	_, svc := newAuthzSvcWithRB(t, testutil.DenyPDP())

	ctx, data := audit.NewAuditContext(testutil.AuthzContext(), &audit.Resource{}, audit.RequestInfo{})
	_, err := svc.GetResourceLogs(ctx, "ns-1", "rb-1", "pod-1", "", nil)
	require.Error(t, err)

	require.Equal(t, map[string]any{"podName": "pod-1"}, data.Parameters)
	require.Nil(t, data.Returned)
}

func publishedLogRecord(t *testing.T, ctx context.Context, data *audit.AuditData) string {
	t.Helper()
	policies, errs := audit.NewPolicySet(coreconfig.NewPath("audit"), audit.Settings{Publish: true}, nil)
	require.Empty(t, errs)
	var buf bytes.Buffer
	emitter, err := audit.NewEmitter("openchoreo-api", policies, audit.NewLogger(&buf))
	require.NoError(t, err)
	op := &audit.Operation{
		ID: "GetReleaseBindingK8sResourceLogs", Action: "read_resource_logs",
		ResourceType: "releasebinding", Category: audit.CategoryAccess,
	}
	audit.EmitFromContext(ctx, emitter, "", op, audit.SurfaceREST, audit.ResultSuccess, data, http.Header{}, "")
	return buf.String()
}
