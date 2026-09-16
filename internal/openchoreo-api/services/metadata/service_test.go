// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package metadata

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	"github.com/openchoreo/openchoreo/internal/openchoreo-api/config"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/testutil"
)

const testObserverURL = "https://observer.example.com"

func TestGetMetadata_AuditLogs(t *testing.T) {
	clusterPlane := testutil.NewClusterObservabilityPlane("audit")
	clusterPlane.Spec.ObserverURL = testObserverURL

	namespacedPlane := testutil.NewObservabilityPlane("platform", "audit")
	namespacedPlane.Spec.ObserverURL = testObserverURL

	noURLPlane := testutil.NewClusterObservabilityPlane("no-url")
	noURLPlane.Spec.ObserverURL = ""

	clusterRef := config.AuditObservabilityPlaneRef{Kind: "ClusterObservabilityPlane", Name: "audit"}

	tests := []struct {
		name         string
		auditEnabled bool
		ref          config.AuditObservabilityPlaneRef
		objs         []client.Object
		want         AuditLogs
	}{
		{
			name: "audit disabled",
			ref:  clusterRef,
			objs: []client.Object{clusterPlane},
			want: AuditLogs{},
		},
		{
			name:         "cluster observability plane",
			auditEnabled: true,
			ref:          clusterRef,
			objs:         []client.Object{clusterPlane},
			want:         AuditLogs{Enabled: true, ObserverURL: testObserverURL},
		},
		{
			name:         "namespaced observability plane",
			auditEnabled: true,
			ref:          config.AuditObservabilityPlaneRef{Kind: "ObservabilityPlane", Name: "audit", Namespace: "platform"},
			objs:         []client.Object{namespacedPlane},
			want:         AuditLogs{Enabled: true, ObserverURL: testObserverURL},
		},
		{
			name:         "referenced plane not found",
			auditEnabled: true,
			ref:          config.AuditObservabilityPlaneRef{Kind: "ClusterObservabilityPlane", Name: "missing"},
			want:         AuditLogs{Enabled: true},
		},
		{
			name:         "referenced plane has no observer URL",
			auditEnabled: true,
			ref:          config.AuditObservabilityPlaneRef{Kind: "ClusterObservabilityPlane", Name: "no-url"},
			objs:         []client.Object{noURLPlane},
			want:         AuditLogs{Enabled: true},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			auditCfg := config.AuditDefaults()
			auditCfg.Enabled = tt.auditEnabled
			auditCfg.ObservabilityPlaneRef = tt.ref
			svc := NewService(testutil.NewFakeClient(tt.objs...), auditCfg, testutil.TestLogger())

			md, err := svc.GetMetadata(context.Background())
			require.NoError(t, err)
			assert.Equal(t, tt.want, md.AuditLogs)
		})
	}
}

func TestGetMetadata_AuditLookupErrorPropagates(t *testing.T) {
	lookupErr := errors.New("apiserver unavailable")
	k8sClient := fake.NewClientBuilder().
		WithScheme(testutil.NewScheme()).
		WithInterceptorFuncs(interceptor.Funcs{
			Get: func(context.Context, client.WithWatch, client.ObjectKey, client.Object, ...client.GetOption) error {
				return lookupErr
			},
		}).
		Build()

	auditCfg := config.AuditDefaults()
	auditCfg.Enabled = true
	auditCfg.ObservabilityPlaneRef = config.AuditObservabilityPlaneRef{Kind: "ClusterObservabilityPlane", Name: "audit"}
	svc := NewService(k8sClient, auditCfg, testutil.TestLogger())

	_, err := svc.GetMetadata(context.Background())
	require.ErrorIs(t, err, lookupErr)
}
