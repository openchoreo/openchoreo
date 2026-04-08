// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	k8sMocks "github.com/openchoreo/openchoreo/internal/clients/kubernetes/mocks"
)

// test helpers

func newScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	require.NoError(t, openchoreov1alpha1.AddToScheme(s))
	return s
}

func newFakeClient(t *testing.T, scheme *runtime.Scheme, objects ...client.Object) client.Client {
	t.Helper()
	return fake.NewClientBuilder().WithScheme(scheme).WithObjects(objects...).Build()
}

// ─────────────────────────────────────────────────────────────
// GetDataPlaneFromRef
// ─────────────────────────────────────────────────────────────

func TestGetDataPlaneFromRef(t *testing.T) {
	scheme := newScheme(t)
	ctx := context.Background()

	dp := &openchoreov1alpha1.DataPlane{
		ObjectMeta: metav1.ObjectMeta{Name: "my-dp", Namespace: "ns-a"},
	}
	defaultDP := &openchoreov1alpha1.DataPlane{
		ObjectMeta: metav1.ObjectMeta{Name: "default", Namespace: "ns-a"},
	}
	defaultCDP := &openchoreov1alpha1.ClusterDataPlane{
		ObjectMeta: metav1.ObjectMeta{Name: "default"},
	}
	namedCDP := &openchoreov1alpha1.ClusterDataPlane{
		ObjectMeta: metav1.ObjectMeta{Name: "shared-cdp"},
	}

	tests := []struct {
		name      string
		namespace string
		ref       *openchoreov1alpha1.DataPlaneRef
		objects   []client.Object
		wantName  string
		wantNS    bool // true = expect namespace-scoped result
		wantErr   string
	}{
		// ── explicit ref ──
		{
			name:      "explicit ref to namespace-scoped DataPlane",
			namespace: "ns-a",
			ref:       &openchoreov1alpha1.DataPlaneRef{Kind: openchoreov1alpha1.DataPlaneRefKindDataPlane, Name: "my-dp"},
			objects:   []client.Object{dp},
			wantName:  "my-dp",
			wantNS:    true,
		},
		{
			name:      "explicit ref to ClusterDataPlane",
			namespace: "ns-a",
			ref:       &openchoreov1alpha1.DataPlaneRef{Kind: openchoreov1alpha1.DataPlaneRefKindClusterDataPlane, Name: "shared-cdp"},
			objects:   []client.Object{namedCDP},
			wantName:  "shared-cdp",
			wantNS:    false,
		},
		{
			name:      "explicit ref not found",
			namespace: "ns-a",
			ref:       &openchoreov1alpha1.DataPlaneRef{Kind: openchoreov1alpha1.DataPlaneRefKindDataPlane, Name: "missing"},
			wantErr:   "not found",
		},
		{
			name:      "explicit ClusterDataPlane ref not found",
			namespace: "ns-a",
			ref:       &openchoreov1alpha1.DataPlaneRef{Kind: openchoreov1alpha1.DataPlaneRefKindClusterDataPlane, Name: "missing"},
			wantErr:   "not found",
		},
		{
			name:      "unsupported ref kind",
			namespace: "ns-a",
			ref:       &openchoreov1alpha1.DataPlaneRef{Kind: "InvalidKind", Name: "x"},
			wantErr:   "unsupported DataPlaneRef kind",
		},
		// ── nil ref, namespace-scoped referrer ──
		{
			name:      "nil ref resolves namespace default",
			namespace: "ns-a",
			ref:       nil,
			objects:   []client.Object{defaultDP, defaultCDP},
			wantName:  "default",
			wantNS:    true, // namespace-scoped takes priority
		},
		{
			name:      "nil ref falls back to cluster default when namespace default missing",
			namespace: "ns-a",
			ref:       nil,
			objects:   []client.Object{defaultCDP}, // no namespace-scoped default
			wantName:  "default",
			wantNS:    false,
		},
		{
			name:      "nil ref errors when no defaults exist",
			namespace: "ns-a",
			ref:       nil,
			wantErr:   "no DataPlaneRef specified",
		},
		// ── nil ref, cluster-scoped referrer ──
		{
			name:      "nil ref with empty namespace resolves cluster default",
			namespace: "",
			ref:       nil,
			objects:   []client.Object{defaultCDP},
			wantName:  "default",
			wantNS:    false,
		},
		{
			name:      "nil ref with empty namespace skips namespace lookup",
			namespace: "",
			ref:       nil,
			objects:   []client.Object{defaultDP}, // only namespace-scoped exists, should not find it
			wantErr:   "no DataPlaneRef specified",
		},
		{
			name:      "nil ref with empty namespace errors when cluster default missing",
			namespace: "",
			ref:       nil,
			wantErr:   "default ClusterDataPlane 'default' not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fc := newFakeClient(t, scheme, tt.objects...)
			result, err := GetDataPlaneFromRef(ctx, fc, tt.namespace, tt.ref)

			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				assert.Nil(t, result)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, result)
			assert.Equal(t, tt.wantName, result.GetName())
			if tt.wantNS {
				assert.NotNil(t, result.DataPlane, "expected namespace-scoped DataPlane")
				assert.Nil(t, result.ClusterDataPlane)
			} else {
				assert.Nil(t, result.DataPlane)
				assert.NotNil(t, result.ClusterDataPlane, "expected cluster-scoped ClusterDataPlane")
			}
		})
	}
}

// ─────────────────────────────────────────────────────────────
// GetObservabilityPlaneFromRef
// ─────────────────────────────────────────────────────────────

func TestGetObservabilityPlaneFromRef(t *testing.T) {
	scheme := newScheme(t)
	ctx := context.Background()

	op := &openchoreov1alpha1.ObservabilityPlane{
		ObjectMeta: metav1.ObjectMeta{Name: "my-op", Namespace: "ns-a"},
	}
	defaultOP := &openchoreov1alpha1.ObservabilityPlane{
		ObjectMeta: metav1.ObjectMeta{Name: "default", Namespace: "ns-a"},
	}
	defaultCOP := &openchoreov1alpha1.ClusterObservabilityPlane{
		ObjectMeta: metav1.ObjectMeta{Name: "default"},
	}
	namedCOP := &openchoreov1alpha1.ClusterObservabilityPlane{
		ObjectMeta: metav1.ObjectMeta{Name: "shared-cop"},
	}

	tests := []struct {
		name      string
		namespace string
		ref       *openchoreov1alpha1.ObservabilityPlaneRef
		objects   []client.Object
		wantName  string
		wantNS    bool
		wantErr   string
	}{
		// ── explicit ref ──
		{
			name:      "explicit ref to namespace-scoped ObservabilityPlane",
			namespace: "ns-a",
			ref:       &openchoreov1alpha1.ObservabilityPlaneRef{Kind: openchoreov1alpha1.ObservabilityPlaneRefKindObservabilityPlane, Name: "my-op"},
			objects:   []client.Object{op},
			wantName:  "my-op",
			wantNS:    true,
		},
		{
			name:      "explicit ref to ClusterObservabilityPlane",
			namespace: "ns-a",
			ref:       &openchoreov1alpha1.ObservabilityPlaneRef{Kind: openchoreov1alpha1.ObservabilityPlaneRefKindClusterObservabilityPlane, Name: "shared-cop"},
			objects:   []client.Object{namedCOP},
			wantName:  "shared-cop",
			wantNS:    false,
		},
		{
			name:      "explicit ref not found",
			namespace: "ns-a",
			ref:       &openchoreov1alpha1.ObservabilityPlaneRef{Kind: openchoreov1alpha1.ObservabilityPlaneRefKindObservabilityPlane, Name: "missing"},
			wantErr:   "not found",
		},
		{
			name:      "explicit ClusterObservabilityPlane ref not found",
			namespace: "ns-a",
			ref:       &openchoreov1alpha1.ObservabilityPlaneRef{Kind: openchoreov1alpha1.ObservabilityPlaneRefKindClusterObservabilityPlane, Name: "missing"},
			wantErr:   "not found",
		},
		{
			name:      "unsupported ref kind",
			namespace: "ns-a",
			ref:       &openchoreov1alpha1.ObservabilityPlaneRef{Kind: "InvalidKind", Name: "x"},
			wantErr:   "unsupported ObservabilityPlaneRef kind",
		},
		// ── nil ref, namespace-scoped referrer ──
		{
			name:      "nil ref resolves namespace default",
			namespace: "ns-a",
			ref:       nil,
			objects:   []client.Object{defaultOP, defaultCOP},
			wantName:  "default",
			wantNS:    true,
		},
		{
			name:      "nil ref falls back to cluster default when namespace default missing",
			namespace: "ns-a",
			ref:       nil,
			objects:   []client.Object{defaultCOP},
			wantName:  "default",
			wantNS:    false,
		},
		{
			name:      "nil ref errors when no defaults exist",
			namespace: "ns-a",
			ref:       nil,
			wantErr:   "no ObservabilityPlaneRef specified",
		},
		// ── nil ref, cluster-scoped referrer ──
		{
			name:      "nil ref with empty namespace resolves cluster default",
			namespace: "",
			ref:       nil,
			objects:   []client.Object{defaultCOP},
			wantName:  "default",
			wantNS:    false,
		},
		{
			name:      "nil ref with empty namespace skips namespace lookup",
			namespace: "",
			ref:       nil,
			objects:   []client.Object{defaultOP},
			wantErr:   "no ObservabilityPlaneRef specified",
		},
		{
			name:      "nil ref with empty namespace errors when cluster default missing",
			namespace: "",
			ref:       nil,
			wantErr:   "default ClusterObservabilityPlane 'default' not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fc := newFakeClient(t, scheme, tt.objects...)
			result, err := GetObservabilityPlaneFromRef(ctx, fc, tt.namespace, tt.ref)

			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				assert.Nil(t, result)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, result)
			assert.Equal(t, tt.wantName, result.GetName())
			if tt.wantNS {
				assert.NotNil(t, result.ObservabilityPlane, "expected namespace-scoped ObservabilityPlane")
				assert.Nil(t, result.ClusterObservabilityPlane)
			} else {
				assert.Nil(t, result.ObservabilityPlane)
				assert.NotNil(t, result.ClusterObservabilityPlane, "expected cluster-scoped ClusterObservabilityPlane")
			}
		})
	}
}

// ─────────────────────────────────────────────────────────────
// GetWorkflowPlaneFromRef
// ─────────────────────────────────────────────────────────────

func TestGetWorkflowPlaneFromRef(t *testing.T) {
	scheme := newScheme(t)
	ctx := context.Background()

	wp := &openchoreov1alpha1.WorkflowPlane{
		ObjectMeta: metav1.ObjectMeta{Name: "my-wp", Namespace: "ns-a"},
	}
	defaultWP := &openchoreov1alpha1.WorkflowPlane{
		ObjectMeta: metav1.ObjectMeta{Name: "default", Namespace: "ns-a"},
	}
	defaultCWP := &openchoreov1alpha1.ClusterWorkflowPlane{
		ObjectMeta: metav1.ObjectMeta{Name: "default"},
	}
	namedCWP := &openchoreov1alpha1.ClusterWorkflowPlane{
		ObjectMeta: metav1.ObjectMeta{Name: "shared-cwp"},
	}

	tests := []struct {
		name      string
		namespace string
		ref       *openchoreov1alpha1.WorkflowPlaneRef
		objects   []client.Object
		wantName  string
		wantNS    bool
		wantErr   string
	}{
		// ── explicit ref ──
		{
			name:      "explicit ref to namespace-scoped WorkflowPlane",
			namespace: "ns-a",
			ref:       &openchoreov1alpha1.WorkflowPlaneRef{Kind: openchoreov1alpha1.WorkflowPlaneRefKindWorkflowPlane, Name: "my-wp"},
			objects:   []client.Object{wp},
			wantName:  "my-wp",
			wantNS:    true,
		},
		{
			name:      "explicit ref to ClusterWorkflowPlane",
			namespace: "ns-a",
			ref:       &openchoreov1alpha1.WorkflowPlaneRef{Kind: openchoreov1alpha1.WorkflowPlaneRefKindClusterWorkflowPlane, Name: "shared-cwp"},
			objects:   []client.Object{namedCWP},
			wantName:  "shared-cwp",
			wantNS:    false,
		},
		{
			name:      "explicit ref not found",
			namespace: "ns-a",
			ref:       &openchoreov1alpha1.WorkflowPlaneRef{Kind: openchoreov1alpha1.WorkflowPlaneRefKindWorkflowPlane, Name: "missing"},
			wantErr:   "not found",
		},
		{
			name:      "explicit ClusterWorkflowPlane ref not found",
			namespace: "ns-a",
			ref:       &openchoreov1alpha1.WorkflowPlaneRef{Kind: openchoreov1alpha1.WorkflowPlaneRefKindClusterWorkflowPlane, Name: "missing"},
			wantErr:   "not found",
		},
		{
			name:      "unsupported ref kind",
			namespace: "ns-a",
			ref:       &openchoreov1alpha1.WorkflowPlaneRef{Kind: "InvalidKind", Name: "x"},
			wantErr:   "unsupported WorkflowPlaneRef kind",
		},
		// ── nil ref, namespace-scoped referrer ──
		{
			name:      "nil ref resolves namespace default",
			namespace: "ns-a",
			ref:       nil,
			objects:   []client.Object{defaultWP, defaultCWP},
			wantName:  "default",
			wantNS:    true,
		},
		{
			name:      "nil ref falls back to cluster default when namespace default missing",
			namespace: "ns-a",
			ref:       nil,
			objects:   []client.Object{defaultCWP},
			wantName:  "default",
			wantNS:    false,
		},
		{
			name:      "nil ref errors when no defaults exist",
			namespace: "ns-a",
			ref:       nil,
			wantErr:   "no WorkflowPlaneRef specified",
		},
		// ── nil ref, cluster-scoped referrer ──
		{
			name:      "nil ref with empty namespace resolves cluster default",
			namespace: "",
			ref:       nil,
			objects:   []client.Object{defaultCWP},
			wantName:  "default",
			wantNS:    false,
		},
		{
			name:      "nil ref with empty namespace skips namespace lookup",
			namespace: "",
			ref:       nil,
			objects:   []client.Object{defaultWP},
			wantErr:   "no WorkflowPlaneRef specified",
		},
		{
			name:      "nil ref with empty namespace errors when cluster default missing",
			namespace: "",
			ref:       nil,
			wantErr:   "default ClusterWorkflowPlane 'default' not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fc := newFakeClient(t, scheme, tt.objects...)
			result, err := GetWorkflowPlaneFromRef(ctx, fc, tt.namespace, tt.ref)

			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				assert.Nil(t, result)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, result)
			assert.Equal(t, tt.wantName, result.GetName())
			if tt.wantNS {
				assert.NotNil(t, result.WorkflowPlane, "expected namespace-scoped WorkflowPlane")
				assert.Nil(t, result.ClusterWorkflowPlane)
			} else {
				assert.Nil(t, result.WorkflowPlane)
				assert.NotNil(t, result.ClusterWorkflowPlane, "expected cluster-scoped ClusterWorkflowPlane")
			}
		})
	}
}

// ─────────────────────────────────────────────────────────────
// ObservabilityPlaneResult methods
// ─────────────────────────────────────────────────────────────

func TestObservabilityPlaneResult_Methods(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, openchoreov1alpha1.AddToScheme(scheme))

	// Test with ObservabilityPlane
	opResult := &ObservabilityPlaneResult{
		ObservabilityPlane: &openchoreov1alpha1.ObservabilityPlane{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-op",
				Namespace: "test-namespace",
			},
			Spec: openchoreov1alpha1.ObservabilityPlaneSpec{
				ObserverURL: "http://observer.example.com",
				PlaneID:     "plane-123",
			},
		},
	}

	assert.Equal(t, "test-op", opResult.GetName())
	assert.Equal(t, "test-namespace", opResult.GetNamespace())
	assert.Equal(t, "http://observer.example.com", opResult.GetObserverURL())
	assert.Equal(t, "plane-123", opResult.GetPlaneID())

	// Test with ClusterObservabilityPlane
	copResult := &ObservabilityPlaneResult{
		ClusterObservabilityPlane: &openchoreov1alpha1.ClusterObservabilityPlane{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-cop",
			},
			Spec: openchoreov1alpha1.ClusterObservabilityPlaneSpec{
				ObserverURL: "http://cluster-observer.example.com",
				PlaneID:     "cluster-plane-456",
			},
		},
	}

	assert.Equal(t, "test-cop", copResult.GetName())
	assert.Equal(t, "", copResult.GetNamespace())
	assert.Equal(t, "http://cluster-observer.example.com", copResult.GetObserverURL())
	assert.Equal(t, "cluster-plane-456", copResult.GetPlaneID())

	// Test with empty result
	emptyResult := &ObservabilityPlaneResult{}
	assert.Equal(t, "", emptyResult.GetName())
	assert.Equal(t, "", emptyResult.GetNamespace())
	assert.Equal(t, "", emptyResult.GetObserverURL())
	assert.Equal(t, "", emptyResult.GetPlaneID())
}

func TestDataPlaneResult_Methods(t *testing.T) {
	// Namespace-scoped DataPlane
	dpResult := &DataPlaneResult{
		DataPlane: &openchoreov1alpha1.DataPlane{
			ObjectMeta: metav1.ObjectMeta{Name: "my-dp", Namespace: "ns-a"},
		},
	}
	assert.Equal(t, "my-dp", dpResult.GetName())
	assert.Equal(t, "ns-a", dpResult.GetNamespace())

	// Cluster-scoped ClusterDataPlane
	cdpResult := &DataPlaneResult{
		ClusterDataPlane: &openchoreov1alpha1.ClusterDataPlane{
			ObjectMeta: metav1.ObjectMeta{Name: "shared-cdp"},
		},
	}
	assert.Equal(t, "shared-cdp", cdpResult.GetName())
	assert.Equal(t, "", cdpResult.GetNamespace())

	// Empty result
	emptyDP := &DataPlaneResult{}
	assert.Equal(t, "", emptyDP.GetName())
	assert.Equal(t, "", emptyDP.GetNamespace())
}

func TestWorkflowPlaneResult_Methods(t *testing.T) {
	// Namespace-scoped WorkflowPlane
	wpResult := &WorkflowPlaneResult{
		WorkflowPlane: &openchoreov1alpha1.WorkflowPlane{
			ObjectMeta: metav1.ObjectMeta{Name: "my-wp", Namespace: "ns-a"},
		},
	}
	assert.Equal(t, "my-wp", wpResult.GetName())
	assert.Equal(t, "ns-a", wpResult.GetNamespace())

	// Cluster-scoped ClusterWorkflowPlane
	cwpResult := &WorkflowPlaneResult{
		ClusterWorkflowPlane: &openchoreov1alpha1.ClusterWorkflowPlane{
			ObjectMeta: metav1.ObjectMeta{Name: "shared-cwp"},
		},
	}
	assert.Equal(t, "shared-cwp", cwpResult.GetName())
	assert.Equal(t, "", cwpResult.GetNamespace())

	// Empty result
	emptyWP := &WorkflowPlaneResult{}
	assert.Equal(t, "", emptyWP.GetName())
	assert.Equal(t, "", emptyWP.GetNamespace())
}

// ─────────────────────────────────────────────────────────────
// GetK8sClient on Result types
// ─────────────────────────────────────────────────────────────

func TestDataPlaneResult_GetK8sClient(t *testing.T) {
	fakeDP := fake.NewClientBuilder().Build()

	tests := []struct {
		name     string
		result   *DataPlaneResult
		provider *k8sMocks.MockPlaneClientProvider
		wantErr  string
	}{
		{
			name:   "namespace-scoped DataPlane dispatches to DataPlaneClient",
			result: &DataPlaneResult{DataPlane: &openchoreov1alpha1.DataPlane{ObjectMeta: metav1.ObjectMeta{Name: "dp"}}},
			provider: func() *k8sMocks.MockPlaneClientProvider {
				m := &k8sMocks.MockPlaneClientProvider{}
				m.EXPECT().DataPlaneClient(mock.Anything).Return(fakeDP, nil).Once()
				return m
			}(),
		},
		{
			name:   "cluster-scoped ClusterDataPlane dispatches to ClusterDataPlaneClient",
			result: &DataPlaneResult{ClusterDataPlane: &openchoreov1alpha1.ClusterDataPlane{ObjectMeta: metav1.ObjectMeta{Name: "cdp"}}},
			provider: func() *k8sMocks.MockPlaneClientProvider {
				m := &k8sMocks.MockPlaneClientProvider{}
				m.EXPECT().ClusterDataPlaneClient(mock.Anything).Return(fakeDP, nil).Once()
				return m
			}(),
		},
		{
			name:     "empty result returns error",
			result:   &DataPlaneResult{},
			provider: &k8sMocks.MockPlaneClientProvider{},
			wantErr:  "no data plane set in result",
		},
		{
			name:   "provider error is propagated",
			result: &DataPlaneResult{DataPlane: &openchoreov1alpha1.DataPlane{}},
			provider: func() *k8sMocks.MockPlaneClientProvider {
				m := &k8sMocks.MockPlaneClientProvider{}
				m.EXPECT().DataPlaneClient(mock.Anything).Return(nil, fmt.Errorf("connection refused")).Once()
				return m
			}(),
			wantErr: "connection refused",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.result.GetK8sClient(tt.provider)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				assert.Nil(t, got)
				return
			}
			require.NoError(t, err)
			assert.NotNil(t, got)
		})
	}

	t.Run("nil provider returns error", func(t *testing.T) {
		result := &DataPlaneResult{DataPlane: &openchoreov1alpha1.DataPlane{}}
		_, err := result.GetK8sClient(nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no DataPlaneClientProvider configured")
	})
}

func TestObservabilityPlaneResult_GetK8sClient(t *testing.T) {
	fakeOP := fake.NewClientBuilder().Build()

	tests := []struct {
		name     string
		result   *ObservabilityPlaneResult
		provider *k8sMocks.MockPlaneClientProvider
		wantErr  string
	}{
		{
			name:   "namespace-scoped ObservabilityPlane dispatches correctly",
			result: &ObservabilityPlaneResult{ObservabilityPlane: &openchoreov1alpha1.ObservabilityPlane{ObjectMeta: metav1.ObjectMeta{Name: "op"}}},
			provider: func() *k8sMocks.MockPlaneClientProvider {
				m := &k8sMocks.MockPlaneClientProvider{}
				m.EXPECT().ObservabilityPlaneClient(mock.Anything).Return(fakeOP, nil).Once()
				return m
			}(),
		},
		{
			name:   "cluster-scoped ClusterObservabilityPlane dispatches correctly",
			result: &ObservabilityPlaneResult{ClusterObservabilityPlane: &openchoreov1alpha1.ClusterObservabilityPlane{ObjectMeta: metav1.ObjectMeta{Name: "cop"}}},
			provider: func() *k8sMocks.MockPlaneClientProvider {
				m := &k8sMocks.MockPlaneClientProvider{}
				m.EXPECT().ClusterObservabilityPlaneClient(mock.Anything).Return(fakeOP, nil).Once()
				return m
			}(),
		},
		{
			name:     "empty result returns error",
			result:   &ObservabilityPlaneResult{},
			provider: &k8sMocks.MockPlaneClientProvider{},
			wantErr:  "no observability plane set in result",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.result.GetK8sClient(tt.provider)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				assert.Nil(t, got)
				return
			}
			require.NoError(t, err)
			assert.NotNil(t, got)
		})
	}

	t.Run("nil provider returns error", func(t *testing.T) {
		result := &ObservabilityPlaneResult{ObservabilityPlane: &openchoreov1alpha1.ObservabilityPlane{}}
		_, err := result.GetK8sClient(nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no ObservabilityPlaneClientProvider configured")
	})
}

func TestWorkflowPlaneResult_GetK8sClient(t *testing.T) {
	fakeWP := fake.NewClientBuilder().Build()

	tests := []struct {
		name     string
		result   *WorkflowPlaneResult
		provider *k8sMocks.MockPlaneClientProvider
		wantErr  string
	}{
		{
			name:   "namespace-scoped WorkflowPlane dispatches correctly",
			result: &WorkflowPlaneResult{WorkflowPlane: &openchoreov1alpha1.WorkflowPlane{ObjectMeta: metav1.ObjectMeta{Name: "wp"}}},
			provider: func() *k8sMocks.MockPlaneClientProvider {
				m := &k8sMocks.MockPlaneClientProvider{}
				m.EXPECT().WorkflowPlaneClient(mock.Anything).Return(fakeWP, nil).Once()
				return m
			}(),
		},
		{
			name:   "cluster-scoped ClusterWorkflowPlane dispatches correctly",
			result: &WorkflowPlaneResult{ClusterWorkflowPlane: &openchoreov1alpha1.ClusterWorkflowPlane{ObjectMeta: metav1.ObjectMeta{Name: "cwp"}}},
			provider: func() *k8sMocks.MockPlaneClientProvider {
				m := &k8sMocks.MockPlaneClientProvider{}
				m.EXPECT().ClusterWorkflowPlaneClient(mock.Anything).Return(fakeWP, nil).Once()
				return m
			}(),
		},
		{
			name:     "empty result returns error",
			result:   &WorkflowPlaneResult{},
			provider: &k8sMocks.MockPlaneClientProvider{},
			wantErr:  "no workflow plane set in result",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.result.GetK8sClient(tt.provider)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				assert.Nil(t, got)
				return
			}
			require.NoError(t, err)
			assert.NotNil(t, got)
		})
	}

	t.Run("nil provider returns error", func(t *testing.T) {
		result := &WorkflowPlaneResult{WorkflowPlane: &openchoreov1alpha1.WorkflowPlane{}}
		_, err := result.GetK8sClient(nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no WorkflowPlaneClientProvider configured")
	})
}

// ============================================================================

func TestDataPlaneResult_ToDataPlane_WithDataPlane(t *testing.T) {
	dp := &openchoreov1alpha1.DataPlane{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-dp",
			Namespace: "test-ns",
			UID:       "dp-uid-123",
		},
		Spec: openchoreov1alpha1.DataPlaneSpec{
			PlaneID: "plane-1",
		},
	}

	result := &DataPlaneResult{DataPlane: dp}
	got := result.ToDataPlane()

	// Should return the exact same pointer
	assert.Same(t, dp, got)
}

func TestDataPlaneResult_ToDataPlane_WithClusterDataPlane(t *testing.T) {
	cdp := &openchoreov1alpha1.ClusterDataPlane{
		ObjectMeta: metav1.ObjectMeta{
			Name: "cluster-dp",
			UID:  "cdp-uid-456",
		},
		Spec: openchoreov1alpha1.ClusterDataPlaneSpec{
			PlaneID: "shared-plane",
			Gateway: openchoreov1alpha1.GatewaySpec{
				Ingress: &openchoreov1alpha1.GatewayNetworkSpec{
					External: &openchoreov1alpha1.GatewayEndpointSpec{
						Name:      "public-gw",
						Namespace: "gw-ns",
					},
				},
			},
			ObservabilityPlaneRef: &openchoreov1alpha1.ClusterObservabilityPlaneRef{
				Kind: openchoreov1alpha1.ClusterObservabilityPlaneRefKindClusterObservabilityPlane,
				Name: "shared-obs",
			},
		},
	}

	result := &DataPlaneResult{ClusterDataPlane: cdp}
	got := result.ToDataPlane()

	require.NotNil(t, got)
	// Verify ObjectMeta fields are mapped
	assert.Equal(t, "cluster-dp", got.Name)
	assert.Equal(t, "cdp-uid-456", string(got.UID))
	assert.Equal(t, "", got.Namespace) // Cluster-scoped has no namespace

	// Verify Spec fields are mapped
	assert.Equal(t, "shared-plane", got.Spec.PlaneID)
	require.NotNil(t, got.Spec.Gateway.Ingress)
	require.NotNil(t, got.Spec.Gateway.Ingress.External)
	assert.Equal(t, "public-gw", got.Spec.Gateway.Ingress.External.Name)
	assert.Equal(t, "gw-ns", got.Spec.Gateway.Ingress.External.Namespace)

	// Verify ObservabilityPlaneRef is mapped from ClusterObservabilityPlaneRef
	require.NotNil(t, got.Spec.ObservabilityPlaneRef)
	assert.Equal(t, openchoreov1alpha1.ObservabilityPlaneRefKindClusterObservabilityPlane, got.Spec.ObservabilityPlaneRef.Kind)
	assert.Equal(t, "shared-obs", got.Spec.ObservabilityPlaneRef.Name)
}

func TestDataPlaneResult_ToDataPlane_WithClusterDataPlane_NoObsRef(t *testing.T) {
	cdp := &openchoreov1alpha1.ClusterDataPlane{
		ObjectMeta: metav1.ObjectMeta{
			Name: "cluster-dp-no-obs",
			UID:  "cdp-uid-789",
		},
		Spec: openchoreov1alpha1.ClusterDataPlaneSpec{
			PlaneID:               "plane-no-obs",
			ObservabilityPlaneRef: nil,
		},
	}

	result := &DataPlaneResult{ClusterDataPlane: cdp}
	got := result.ToDataPlane()

	require.NotNil(t, got)
	assert.Equal(t, "cluster-dp-no-obs", got.Name)
	assert.Nil(t, got.Spec.ObservabilityPlaneRef)
}

func TestDataPlaneResult_ToDataPlane_NeitherSet(t *testing.T) {
	result := &DataPlaneResult{}
	got := result.ToDataPlane()

	assert.Nil(t, got)
}

// ============================================================================
// Tests for DataPlaneResult.GetObservabilityPlane
// ============================================================================

func TestDataPlaneResult_GetObservabilityPlane(t *testing.T) {
	scheme := newScheme(t)
	ctx := context.Background()

	tests := []struct {
		name     string
		result   *DataPlaneResult
		objects  []client.Object
		wantName string
		wantNS   bool
		wantErr  string
	}{
		{
			name: "DataPlane with explicit obs ref",
			result: &DataPlaneResult{DataPlane: &openchoreov1alpha1.DataPlane{
				ObjectMeta: metav1.ObjectMeta{Name: "my-dp", Namespace: "test-ns"},
				Spec: openchoreov1alpha1.DataPlaneSpec{
					ObservabilityPlaneRef: &openchoreov1alpha1.ObservabilityPlaneRef{
						Kind: openchoreov1alpha1.ObservabilityPlaneRefKindObservabilityPlane, Name: "my-obs",
					},
				},
			}},
			objects:  []client.Object{&openchoreov1alpha1.ObservabilityPlane{ObjectMeta: metav1.ObjectMeta{Name: "my-obs", Namespace: "test-ns"}}},
			wantName: "my-obs",
			wantNS:   true,
		},
		{
			name: "ClusterDataPlane with explicit cluster obs ref",
			result: &DataPlaneResult{ClusterDataPlane: &openchoreov1alpha1.ClusterDataPlane{
				ObjectMeta: metav1.ObjectMeta{Name: "cluster-dp"},
				Spec: openchoreov1alpha1.ClusterDataPlaneSpec{
					ObservabilityPlaneRef: &openchoreov1alpha1.ClusterObservabilityPlaneRef{
						Kind: openchoreov1alpha1.ClusterObservabilityPlaneRefKindClusterObservabilityPlane, Name: "shared-obs",
					},
				},
			}},
			objects:  []client.Object{&openchoreov1alpha1.ClusterObservabilityPlane{ObjectMeta: metav1.ObjectMeta{Name: "shared-obs"}}},
			wantName: "shared-obs",
			wantNS:   false,
		},
		{
			name: "ClusterDataPlane nil ref defaults to cluster default",
			result: &DataPlaneResult{ClusterDataPlane: &openchoreov1alpha1.ClusterDataPlane{
				ObjectMeta: metav1.ObjectMeta{Name: "cluster-dp-no-ref"},
			}},
			objects:  []client.Object{&openchoreov1alpha1.ClusterObservabilityPlane{ObjectMeta: metav1.ObjectMeta{Name: "default"}}},
			wantName: "default",
			wantNS:   false,
		},
		{
			name: "DataPlane with nil obs ref defaults to namespace default",
			result: &DataPlaneResult{DataPlane: &openchoreov1alpha1.DataPlane{
				ObjectMeta: metav1.ObjectMeta{Name: "my-dp", Namespace: "test-ns"},
			}},
			objects:  []client.Object{&openchoreov1alpha1.ObservabilityPlane{ObjectMeta: metav1.ObjectMeta{Name: "default", Namespace: "test-ns"}}},
			wantName: "default",
			wantNS:   true,
		},
		{
			name:    "empty result errors",
			result:  &DataPlaneResult{},
			wantErr: "no data plane set in result",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fc := newFakeClient(t, scheme, tt.objects...)
			obsResult, err := tt.result.GetObservabilityPlane(ctx, fc)

			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				assert.Nil(t, obsResult)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, obsResult)
			assert.Equal(t, tt.wantName, obsResult.GetName())
			if tt.wantNS {
				assert.NotNil(t, obsResult.ObservabilityPlane)
			} else {
				assert.NotNil(t, obsResult.ClusterObservabilityPlane)
			}
		})
	}
}

func TestWorkflowPlaneResult_GetObservabilityPlane(t *testing.T) {
	scheme := newScheme(t)
	ctx := context.Background()

	tests := []struct {
		name     string
		result   *WorkflowPlaneResult
		objects  []client.Object
		wantName string
		wantNS   bool
		wantErr  string
	}{
		{
			name: "WorkflowPlane with explicit obs ref",
			result: &WorkflowPlaneResult{WorkflowPlane: &openchoreov1alpha1.WorkflowPlane{
				ObjectMeta: metav1.ObjectMeta{Name: "my-wp", Namespace: "test-ns"},
				Spec: openchoreov1alpha1.WorkflowPlaneSpec{
					ObservabilityPlaneRef: &openchoreov1alpha1.ObservabilityPlaneRef{
						Kind: openchoreov1alpha1.ObservabilityPlaneRefKindObservabilityPlane, Name: "my-obs",
					},
				},
			}},
			objects:  []client.Object{&openchoreov1alpha1.ObservabilityPlane{ObjectMeta: metav1.ObjectMeta{Name: "my-obs", Namespace: "test-ns"}}},
			wantName: "my-obs",
			wantNS:   true,
		},
		{
			name: "ClusterWorkflowPlane with explicit cluster obs ref",
			result: &WorkflowPlaneResult{ClusterWorkflowPlane: &openchoreov1alpha1.ClusterWorkflowPlane{
				ObjectMeta: metav1.ObjectMeta{Name: "cluster-wp"},
				Spec: openchoreov1alpha1.ClusterWorkflowPlaneSpec{
					ObservabilityPlaneRef: &openchoreov1alpha1.ClusterObservabilityPlaneRef{
						Kind: openchoreov1alpha1.ClusterObservabilityPlaneRefKindClusterObservabilityPlane, Name: "shared-obs",
					},
				},
			}},
			objects:  []client.Object{&openchoreov1alpha1.ClusterObservabilityPlane{ObjectMeta: metav1.ObjectMeta{Name: "shared-obs"}}},
			wantName: "shared-obs",
			wantNS:   false,
		},
		{
			name: "ClusterWorkflowPlane nil ref defaults to cluster default",
			result: &WorkflowPlaneResult{ClusterWorkflowPlane: &openchoreov1alpha1.ClusterWorkflowPlane{
				ObjectMeta: metav1.ObjectMeta{Name: "cluster-wp-no-ref"},
			}},
			objects:  []client.Object{&openchoreov1alpha1.ClusterObservabilityPlane{ObjectMeta: metav1.ObjectMeta{Name: "default"}}},
			wantName: "default",
			wantNS:   false,
		},
		{
			name: "WorkflowPlane with nil obs ref defaults to namespace default",
			result: &WorkflowPlaneResult{WorkflowPlane: &openchoreov1alpha1.WorkflowPlane{
				ObjectMeta: metav1.ObjectMeta{Name: "my-wp", Namespace: "test-ns"},
			}},
			objects:  []client.Object{&openchoreov1alpha1.ObservabilityPlane{ObjectMeta: metav1.ObjectMeta{Name: "default", Namespace: "test-ns"}}},
			wantName: "default",
			wantNS:   true,
		},
		{
			name:    "empty result errors",
			result:  &WorkflowPlaneResult{},
			wantErr: "no workflow plane set in result",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fc := newFakeClient(t, scheme, tt.objects...)
			obsResult, err := tt.result.GetObservabilityPlane(ctx, fc)

			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				assert.Nil(t, obsResult)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, obsResult)
			assert.Equal(t, tt.wantName, obsResult.GetName())
			if tt.wantNS {
				assert.NotNil(t, obsResult.ObservabilityPlane)
			} else {
				assert.NotNil(t, obsResult.ClusterObservabilityPlane)
			}
		})
	}
}

func TestWorkflowPlaneResult_GetSecretStoreName(t *testing.T) {
	tests := []struct {
		name   string
		result *WorkflowPlaneResult
		want   string
	}{
		{
			name: "WorkflowPlane with secret store",
			result: &WorkflowPlaneResult{WorkflowPlane: &openchoreov1alpha1.WorkflowPlane{
				Spec: openchoreov1alpha1.WorkflowPlaneSpec{SecretStoreRef: &openchoreov1alpha1.SecretStoreRef{Name: "my-store"}},
			}},
			want: "my-store",
		},
		{
			name: "ClusterWorkflowPlane with secret store",
			result: &WorkflowPlaneResult{ClusterWorkflowPlane: &openchoreov1alpha1.ClusterWorkflowPlane{
				Spec: openchoreov1alpha1.ClusterWorkflowPlaneSpec{SecretStoreRef: &openchoreov1alpha1.SecretStoreRef{Name: "cluster-store"}},
			}},
			want: "cluster-store",
		},
		{
			name: "WorkflowPlane nil secret store ref",
			result: &WorkflowPlaneResult{WorkflowPlane: &openchoreov1alpha1.WorkflowPlane{
				Spec: openchoreov1alpha1.WorkflowPlaneSpec{SecretStoreRef: nil},
			}},
			want: "",
		},
		{
			name: "ClusterWorkflowPlane nil secret store ref",
			result: &WorkflowPlaneResult{ClusterWorkflowPlane: &openchoreov1alpha1.ClusterWorkflowPlane{
				Spec: openchoreov1alpha1.ClusterWorkflowPlaneSpec{SecretStoreRef: nil},
			}},
			want: "",
		},
		{
			name:   "empty result",
			result: &WorkflowPlaneResult{},
			want:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.result.GetSecretStoreName())
		})
	}
}

func TestResolveWorkflow(t *testing.T) {
	scheme := newScheme(t)
	ctx := context.Background()

	wf := &openchoreov1alpha1.Workflow{ObjectMeta: metav1.ObjectMeta{Name: "docker", Namespace: "test-ns"}}
	cwf := &openchoreov1alpha1.ClusterWorkflow{ObjectMeta: metav1.ObjectMeta{Name: "docker"}}

	tests := []struct {
		name     string
		kind     openchoreov1alpha1.WorkflowRefKind
		wfName   string
		objects  []client.Object
		wantName string
		wantNS   bool
		wantErr  string
	}{
		{
			name:     "empty kind defaults to ClusterWorkflow",
			kind:     "",
			wfName:   "docker",
			objects:  []client.Object{cwf},
			wantName: "docker",
			wantNS:   false,
		},
		{
			name:     "explicit Workflow kind",
			kind:     openchoreov1alpha1.WorkflowRefKindWorkflow,
			wfName:   "docker",
			objects:  []client.Object{wf},
			wantName: "docker",
			wantNS:   true,
		},
		{
			name:     "explicit ClusterWorkflow kind",
			kind:     openchoreov1alpha1.WorkflowRefKindClusterWorkflow,
			wfName:   "docker",
			objects:  []client.Object{cwf},
			wantName: "docker",
			wantNS:   false,
		},
		{
			name:    "Workflow not found",
			kind:    openchoreov1alpha1.WorkflowRefKindWorkflow,
			wfName:  "nonexistent",
			wantErr: "not found",
		},
		{
			name:    "ClusterWorkflow not found",
			kind:    openchoreov1alpha1.WorkflowRefKindClusterWorkflow,
			wfName:  "nonexistent",
			wantErr: "not found",
		},
		{
			name:    "unsupported kind",
			kind:    "UnsupportedKind",
			wfName:  "some-workflow",
			wantErr: "unsupported workflowRef kind",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fc := newFakeClient(t, scheme, tt.objects...)
			result, err := ResolveWorkflow(ctx, fc, "test-ns", tt.kind, tt.wfName)

			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				assert.Nil(t, result)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, result)
			assert.Equal(t, tt.wantName, result.GetName())
			if tt.wantNS {
				assert.NotNil(t, result.Workflow)
			} else {
				assert.NotNil(t, result.ClusterWorkflow)
			}
		})
	}
}

// ============================================================================
// Tests for WorkflowResult methods
// ============================================================================

func TestWorkflowResult_GetWorkflowSpec_FromWorkflow(t *testing.T) {
	result := &WorkflowResult{
		Workflow: &openchoreov1alpha1.Workflow{
			ObjectMeta: metav1.ObjectMeta{Name: "wf"},
			Spec: openchoreov1alpha1.WorkflowSpec{
				TTLAfterCompletion: "1h",
			},
		},
	}

	spec := result.GetWorkflowSpec()
	assert.Equal(t, "1h", spec.TTLAfterCompletion)
}

func TestWorkflowResult_GetWorkflowSpec_FromClusterWorkflow(t *testing.T) {
	result := &WorkflowResult{
		ClusterWorkflow: &openchoreov1alpha1.ClusterWorkflow{
			ObjectMeta: metav1.ObjectMeta{Name: "cwf"},
			Spec: openchoreov1alpha1.ClusterWorkflowSpec{
				TTLAfterCompletion: "2h",
				WorkflowPlaneRef: &openchoreov1alpha1.ClusterWorkflowPlaneRef{
					Kind: openchoreov1alpha1.ClusterWorkflowPlaneRefKindClusterWorkflowPlane,
					Name: "shared-wp",
				},
			},
		},
	}

	spec := result.GetWorkflowSpec()
	assert.Equal(t, "2h", spec.TTLAfterCompletion)
	require.NotNil(t, spec.WorkflowPlaneRef)
	assert.Equal(t, openchoreov1alpha1.WorkflowPlaneRefKind("ClusterWorkflowPlane"), spec.WorkflowPlaneRef.Kind)
	assert.Equal(t, "shared-wp", spec.WorkflowPlaneRef.Name)
}

func TestWorkflowResult_GetWorkflowSpec_FromClusterWorkflow_NilWorkflowPlaneRef(t *testing.T) {
	// When ClusterWorkflow omits WorkflowPlaneRef (e.g., legacy or before defaulting webhook),
	// GetWorkflowSpec defaults to ClusterWorkflowPlane "default" so ResolveWorkflowPlane
	// always receives a non-nil ref.
	result := &WorkflowResult{
		ClusterWorkflow: &openchoreov1alpha1.ClusterWorkflow{
			ObjectMeta: metav1.ObjectMeta{Name: "cwf-no-wp"},
			Spec: openchoreov1alpha1.ClusterWorkflowSpec{
				TTLAfterCompletion: "1h",
			},
		},
	}

	spec := result.GetWorkflowSpec()
	assert.Equal(t, "1h", spec.TTLAfterCompletion)
	require.NotNil(t, spec.WorkflowPlaneRef)
	assert.Equal(t, openchoreov1alpha1.WorkflowPlaneRefKindClusterWorkflowPlane, spec.WorkflowPlaneRef.Kind)
	assert.Equal(t, "default", spec.WorkflowPlaneRef.Name)
}

func TestWorkflowResult_GetWorkflowSpec_Empty(t *testing.T) {
	result := &WorkflowResult{}
	spec := result.GetWorkflowSpec()
	assert.Equal(t, openchoreov1alpha1.WorkflowSpec{}, spec)
}

func TestWorkflowResult_GetName_Empty(t *testing.T) {
	result := &WorkflowResult{}
	assert.Equal(t, "", result.GetName())
	assert.Equal(t, "", result.GetNamespace())
}
