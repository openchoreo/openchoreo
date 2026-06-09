// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clustergateway

import (
	"context"
	"log/slog"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

// testLogger returns a logger for tests
func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
}

// testScheme creates a scheme with the OpenChoreo types registered
func testScheme() *runtime.Scheme {
	scheme := runtime.NewScheme()
	_ = openchoreov1alpha1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)
	return scheme
}

// TestDataPlaneInfo tests the dataPlaneInfo extractor function
func TestDataPlaneInfo(t *testing.T) {
	tests := []struct {
		name            string
		obj             client.Object
		expectOK        bool
		expectedName    string
		expectedNS      string
		expectedPlaneID string
	}{
		{
			name: "valid DataPlane with explicit planeID",
			obj: &openchoreov1alpha1.DataPlane{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-dp",
					Namespace: "test-ns",
				},
				Spec: openchoreov1alpha1.DataPlaneSpec{
					PlaneID: "custom-plane-id",
				},
			},
			expectOK:        true,
			expectedName:    "my-dp",
			expectedNS:      "test-ns",
			expectedPlaneID: "custom-plane-id",
		},
		{
			name: "valid DataPlane without planeID (defaults to name)",
			obj: &openchoreov1alpha1.DataPlane{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "default-dp",
					Namespace: "test-ns",
				},
				Spec: openchoreov1alpha1.DataPlaneSpec{
					PlaneID: "",
				},
			},
			expectOK:        true,
			expectedName:    "default-dp",
			expectedNS:      "test-ns",
			expectedPlaneID: "default-dp", // Defaults to name
		},
		{
			name: "wrong type returns false",
			obj: &openchoreov1alpha1.WorkflowPlane{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "wp",
					Namespace: "test-ns",
				},
			},
			expectOK: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info, ok := dataPlaneInfo(tt.obj)
			assert.Equal(t, tt.expectOK, ok)
			if tt.expectOK {
				assert.Equal(t, tt.expectedName, info.name)
				assert.Equal(t, tt.expectedNS, info.namespace)
				assert.Equal(t, tt.expectedPlaneID, info.planeID)
			}
		})
	}
}

// TestClusterDataPlaneInfo tests the clusterDataPlaneInfo extractor function
func TestClusterDataPlaneInfo(t *testing.T) {
	tests := []struct {
		name            string
		obj             client.Object
		expectOK        bool
		expectedName    string
		expectedNS      string
		expectedPlaneID string
	}{
		{
			name: "valid ClusterDataPlane",
			obj: &openchoreov1alpha1.ClusterDataPlane{
				ObjectMeta: metav1.ObjectMeta{
					Name: "shared-dp",
				},
				Spec: openchoreov1alpha1.ClusterDataPlaneSpec{
					PlaneID: "shared-plane-id",
				},
			},
			expectOK:        true,
			expectedName:    "shared-dp",
			expectedNS:      "", // Cluster-scoped has empty namespace
			expectedPlaneID: "shared-plane-id",
		},
		{
			name: "wrong type returns false",
			obj: &openchoreov1alpha1.DataPlane{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "dp",
					Namespace: "test-ns",
				},
			},
			expectOK: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info, ok := clusterDataPlaneInfo(tt.obj)
			assert.Equal(t, tt.expectOK, ok)
			if tt.expectOK {
				assert.Equal(t, tt.expectedName, info.name)
				assert.Equal(t, tt.expectedNS, info.namespace)
				assert.Equal(t, tt.expectedPlaneID, info.planeID)
			}
		})
	}
}

// TestClusterWorkflowPlaneInfo tests the clusterWorkflowPlaneInfo extractor function
func TestClusterWorkflowPlaneInfo(t *testing.T) {
	tests := []struct {
		name            string
		obj             client.Object
		expectOK        bool
		expectedName    string
		expectedNS      string
		expectedPlaneID string
	}{
		{
			name: "valid ClusterWorkflowPlane",
			obj: &openchoreov1alpha1.ClusterWorkflowPlane{
				ObjectMeta: metav1.ObjectMeta{
					Name: "shared-wp",
				},
				Spec: openchoreov1alpha1.ClusterWorkflowPlaneSpec{
					PlaneID: "shared-workflow-plane",
				},
			},
			expectOK:        true,
			expectedName:    "shared-wp",
			expectedNS:      "", // Cluster-scoped has empty namespace
			expectedPlaneID: "shared-workflow-plane",
		},
		{
			name: "wrong type returns false",
			obj: &openchoreov1alpha1.WorkflowPlane{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "wp",
					Namespace: "test-ns",
				},
			},
			expectOK: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info, ok := clusterWorkflowPlaneInfo(tt.obj)
			assert.Equal(t, tt.expectOK, ok)
			if tt.expectOK {
				assert.Equal(t, tt.expectedName, info.name)
				assert.Equal(t, tt.expectedNS, info.namespace)
				assert.Equal(t, tt.expectedPlaneID, info.planeID)
			}
		})
	}
}

// TestClusterObsPlaneInfo tests the clusterObsPlaneInfo extractor function
func TestClusterObsPlaneInfo(t *testing.T) {
	tests := []struct {
		name            string
		obj             client.Object
		expectOK        bool
		expectedName    string
		expectedNS      string
		expectedPlaneID string
	}{
		{
			name: "valid ClusterObservabilityPlane",
			obj: &openchoreov1alpha1.ClusterObservabilityPlane{
				ObjectMeta: metav1.ObjectMeta{
					Name: "shared-obs",
				},
				Spec: openchoreov1alpha1.ClusterObservabilityPlaneSpec{
					PlaneID: "shared-obs-plane",
				},
			},
			expectOK:        true,
			expectedName:    "shared-obs",
			expectedNS:      "", // Cluster-scoped has empty namespace
			expectedPlaneID: "shared-obs-plane",
		},
		{
			name: "wrong type returns false",
			obj: &openchoreov1alpha1.ObservabilityPlane{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "obs",
					Namespace: "test-ns",
				},
			},
			expectOK: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info, ok := clusterObsPlaneInfo(tt.obj)
			assert.Equal(t, tt.expectOK, ok)
			if tt.expectOK {
				assert.Equal(t, tt.expectedName, info.name)
				assert.Equal(t, tt.expectedNS, info.namespace)
				assert.Equal(t, tt.expectedPlaneID, info.planeID)
			}
		})
	}
}

// TestExtractListItems tests the extractListItems function for all list types
func TestExtractListItems(t *testing.T) {
	tests := []struct {
		name        string
		list        client.ObjectList
		expectedLen int
		expectError bool
	}{
		{
			name: "DataPlaneList with items",
			list: &openchoreov1alpha1.DataPlaneList{
				Items: []openchoreov1alpha1.DataPlane{
					{ObjectMeta: metav1.ObjectMeta{Name: "dp1"}},
					{ObjectMeta: metav1.ObjectMeta{Name: "dp2"}},
				},
			},
			expectedLen: 2,
			expectError: false,
		},
		{
			name:        "empty DataPlaneList",
			list:        &openchoreov1alpha1.DataPlaneList{},
			expectedLen: 0,
			expectError: false,
		},
		{
			name: "WorkflowPlaneList with items",
			list: &openchoreov1alpha1.WorkflowPlaneList{
				Items: []openchoreov1alpha1.WorkflowPlane{
					{ObjectMeta: metav1.ObjectMeta{Name: "wp1"}},
				},
			},
			expectedLen: 1,
			expectError: false,
		},
		{
			name: "ObservabilityPlaneList with items",
			list: &openchoreov1alpha1.ObservabilityPlaneList{
				Items: []openchoreov1alpha1.ObservabilityPlane{
					{ObjectMeta: metav1.ObjectMeta{Name: "obs1"}},
				},
			},
			expectedLen: 1,
			expectError: false,
		},
		{
			name: "ClusterDataPlaneList with items",
			list: &openchoreov1alpha1.ClusterDataPlaneList{
				Items: []openchoreov1alpha1.ClusterDataPlane{
					{ObjectMeta: metav1.ObjectMeta{Name: "cdp1"}},
					{ObjectMeta: metav1.ObjectMeta{Name: "cdp2"}},
					{ObjectMeta: metav1.ObjectMeta{Name: "cdp3"}},
				},
			},
			expectedLen: 3,
			expectError: false,
		},
		{
			name: "ClusterWorkflowPlaneList with items",
			list: &openchoreov1alpha1.ClusterWorkflowPlaneList{
				Items: []openchoreov1alpha1.ClusterWorkflowPlane{
					{ObjectMeta: metav1.ObjectMeta{Name: "cwp1"}},
				},
			},
			expectedLen: 1,
			expectError: false,
		},
		{
			name: "ClusterObservabilityPlaneList with items",
			list: &openchoreov1alpha1.ClusterObservabilityPlaneList{
				Items: []openchoreov1alpha1.ClusterObservabilityPlane{
					{ObjectMeta: metav1.ObjectMeta{Name: "cop1"}},
					{ObjectMeta: metav1.ObjectMeta{Name: "cop2"}},
				},
			},
			expectedLen: 2,
			expectError: false,
		},
		{
			name:        "unsupported list type returns error",
			list:        &corev1.PodList{},
			expectedLen: 0,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			items, err := extractListItems(tt.list)
			if tt.expectError {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Len(t, items, tt.expectedLen)
			}
		})
	}
}

// TestGetAllPlaneClientCAs_DataPlane tests getAllPlaneClientCAs for dataplane type
func TestGetAllPlaneClientCAs_DataPlane(t *testing.T) {
	scheme := testScheme()

	// Create test secrets with CA data
	caSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ca-secret",
			Namespace: "test-ns",
		},
		Data: map[string][]byte{
			"ca.crt": []byte("test-ca-data"),
		},
	}

	clusterCASecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cluster-ca-secret",
			Namespace: "default",
		},
		Data: map[string][]byte{
			"ca.crt": []byte("cluster-ca-data"),
		},
	}

	// Create namespace-scoped DataPlane
	dataPlane := &openchoreov1alpha1.DataPlane{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ns-dp",
			Namespace: "test-ns",
		},
		Spec: openchoreov1alpha1.DataPlaneSpec{
			PlaneID: "shared-plane",
			ClusterAgent: openchoreov1alpha1.ClusterAgentConfig{
				ClientCA: openchoreov1alpha1.ValueFrom{
					SecretKeyRef: &openchoreov1alpha1.SecretKeyReference{
						Name: "ca-secret",
						Key:  "ca.crt",
					},
				},
			},
		},
	}

	// Create cluster-scoped ClusterDataPlane with the same planeID
	clusterDataPlane := &openchoreov1alpha1.ClusterDataPlane{
		ObjectMeta: metav1.ObjectMeta{
			Name: "cluster-dp",
		},
		Spec: openchoreov1alpha1.ClusterDataPlaneSpec{
			PlaneID: "shared-plane",
			ClusterAgent: openchoreov1alpha1.ClusterAgentConfig{
				ClientCA: openchoreov1alpha1.ValueFrom{
					SecretKeyRef: &openchoreov1alpha1.SecretKeyReference{
						Name:      "cluster-ca-secret",
						Namespace: "default",
						Key:       "ca.crt",
					},
				},
			},
		},
	}

	// Create fake client with objects
	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(caSecret, clusterCASecret, dataPlane, clusterDataPlane).
		Build()

	// Create server with fake client
	server := &Server{
		k8sClient: fakeClient,
		logger:    testLogger(),
	}

	// Test getAllPlaneClientCAs
	result, err := server.getAllPlaneClientCAs(planeTypeDataPlane, "shared-plane")
	require.NoError(t, err)

	// Should find both namespace-scoped and cluster-scoped CRs
	assert.Len(t, result, 2)
	assert.Contains(t, result, "test-ns/ns-dp") // Namespace-scoped key: "namespace/name"
	assert.Contains(t, result, "/cluster-dp")   // Cluster-scoped key: "/name"
	assert.Equal(t, []byte("test-ca-data"), result["test-ns/ns-dp"])
	assert.Equal(t, []byte("cluster-ca-data"), result["/cluster-dp"])
}

// TestGetAllPlaneClientCAs_OnlyNamespaceScoped tests with only namespace-scoped CRs
func TestGetAllPlaneClientCAs_OnlyNamespaceScoped(t *testing.T) {
	scheme := testScheme()

	caSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ca-secret",
			Namespace: "org-a",
		},
		Data: map[string][]byte{
			"ca.crt": []byte("org-a-ca"),
		},
	}

	dataPlane := &openchoreov1alpha1.DataPlane{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "default",
			Namespace: "org-a",
		},
		Spec: openchoreov1alpha1.DataPlaneSpec{
			PlaneID: "prod-cluster",
			ClusterAgent: openchoreov1alpha1.ClusterAgentConfig{
				ClientCA: openchoreov1alpha1.ValueFrom{
					SecretKeyRef: &openchoreov1alpha1.SecretKeyReference{
						Name: "ca-secret",
						Key:  "ca.crt",
					},
				},
			},
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(caSecret, dataPlane).
		Build()

	server := &Server{
		k8sClient: fakeClient,
		logger:    testLogger(),
	}

	result, err := server.getAllPlaneClientCAs(planeTypeDataPlane, "prod-cluster")
	require.NoError(t, err)

	assert.Len(t, result, 1)
	assert.Contains(t, result, "org-a/default")
}

// TestGetAllPlaneClientCAs_OnlyClusterScoped tests with only cluster-scoped CRs
func TestGetAllPlaneClientCAs_OnlyClusterScoped(t *testing.T) {
	scheme := testScheme()

	caSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "shared-ca",
			Namespace: "cert-manager",
		},
		Data: map[string][]byte{
			"tls.crt": []byte("shared-ca-data"),
		},
	}

	clusterDataPlane := &openchoreov1alpha1.ClusterDataPlane{
		ObjectMeta: metav1.ObjectMeta{
			Name: "global-dp",
		},
		Spec: openchoreov1alpha1.ClusterDataPlaneSpec{
			PlaneID: "global-plane",
			ClusterAgent: openchoreov1alpha1.ClusterAgentConfig{
				ClientCA: openchoreov1alpha1.ValueFrom{
					SecretKeyRef: &openchoreov1alpha1.SecretKeyReference{
						Name:      "shared-ca",
						Namespace: "cert-manager",
						Key:       "tls.crt",
					},
				},
			},
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(caSecret, clusterDataPlane).
		Build()

	server := &Server{
		k8sClient: fakeClient,
		logger:    testLogger(),
	}

	result, err := server.getAllPlaneClientCAs(planeTypeDataPlane, "global-plane")
	require.NoError(t, err)

	assert.Len(t, result, 1)
	assert.Contains(t, result, "/global-dp")
}

// TestGetAllPlaneClientCAs_NoMatchingPlaneID tests when no CRs match the planeID
func TestGetAllPlaneClientCAs_NoMatchingPlaneID(t *testing.T) {
	scheme := testScheme()

	// Create CRs with different planeIDs
	dataPlane := &openchoreov1alpha1.DataPlane{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "dp1",
			Namespace: "test-ns",
		},
		Spec: openchoreov1alpha1.DataPlaneSpec{
			PlaneID: "different-plane",
		},
	}

	clusterDataPlane := &openchoreov1alpha1.ClusterDataPlane{
		ObjectMeta: metav1.ObjectMeta{
			Name: "cdp1",
		},
		Spec: openchoreov1alpha1.ClusterDataPlaneSpec{
			PlaneID: "another-plane",
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(dataPlane, clusterDataPlane).
		Build()

	server := &Server{
		k8sClient: fakeClient,
		logger:    testLogger(),
	}

	result, err := server.getAllPlaneClientCAs(planeTypeDataPlane, "non-existent-plane")
	require.NoError(t, err)

	assert.Len(t, result, 0)
}

// TestGetAllPlaneClientCAs_WorkflowPlane tests getAllPlaneClientCAs for workflowplane type
func TestGetAllPlaneClientCAs_WorkflowPlane(t *testing.T) {
	scheme := testScheme()

	nsCASecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ns-ca",
			Namespace: "build-ns",
		},
		Data: map[string][]byte{
			"ca.crt": []byte("ns-build-ca"),
		},
	}

	clusterCASecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cluster-ca",
			Namespace: "default",
		},
		Data: map[string][]byte{
			"ca.crt": []byte("cluster-build-ca"),
		},
	}

	workflowPlane := &openchoreov1alpha1.WorkflowPlane{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "workflow-plane",
			Namespace: "build-ns",
		},
		Spec: openchoreov1alpha1.WorkflowPlaneSpec{
			PlaneID: "ci-cluster",
			ClusterAgent: openchoreov1alpha1.ClusterAgentConfig{
				ClientCA: openchoreov1alpha1.ValueFrom{
					SecretKeyRef: &openchoreov1alpha1.SecretKeyReference{
						Name: "ns-ca",
						Key:  "ca.crt",
					},
				},
			},
		},
	}

	clusterWorkflowPlane := &openchoreov1alpha1.ClusterWorkflowPlane{
		ObjectMeta: metav1.ObjectMeta{
			Name: "shared-build",
		},
		Spec: openchoreov1alpha1.ClusterWorkflowPlaneSpec{
			PlaneID: "ci-cluster",
			ClusterAgent: openchoreov1alpha1.ClusterAgentConfig{
				ClientCA: openchoreov1alpha1.ValueFrom{
					SecretKeyRef: &openchoreov1alpha1.SecretKeyReference{
						Name:      "cluster-ca",
						Namespace: "default",
						Key:       "ca.crt",
					},
				},
			},
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(nsCASecret, clusterCASecret, workflowPlane, clusterWorkflowPlane).
		Build()

	server := &Server{
		k8sClient: fakeClient,
		logger:    testLogger(),
	}

	result, err := server.getAllPlaneClientCAs(planeTypeWorkflowPlane, "ci-cluster")
	require.NoError(t, err)

	assert.Len(t, result, 2)
	assert.Contains(t, result, "build-ns/workflow-plane")
	assert.Contains(t, result, "/shared-build")
}

// TestGetAllPlaneClientCAs_ObservabilityPlane tests getAllPlaneClientCAs for observabilityplane type
func TestGetAllPlaneClientCAs_ObservabilityPlane(t *testing.T) {
	scheme := testScheme()

	nsCASecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "obs-ca",
			Namespace: "monitoring",
		},
		Data: map[string][]byte{
			"ca.crt": []byte("ns-obs-ca"),
		},
	}

	clusterCASecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "shared-obs-ca",
			Namespace: "default",
		},
		Data: map[string][]byte{
			"ca.crt": []byte("cluster-obs-ca"),
		},
	}

	obsPlane := &openchoreov1alpha1.ObservabilityPlane{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "obs-plane",
			Namespace: "monitoring",
		},
		Spec: openchoreov1alpha1.ObservabilityPlaneSpec{
			PlaneID: "monitoring-cluster",
			ClusterAgent: openchoreov1alpha1.ClusterAgentConfig{
				ClientCA: openchoreov1alpha1.ValueFrom{
					SecretKeyRef: &openchoreov1alpha1.SecretKeyReference{
						Name: "obs-ca",
						Key:  "ca.crt",
					},
				},
			},
		},
	}

	clusterObsPlane := &openchoreov1alpha1.ClusterObservabilityPlane{
		ObjectMeta: metav1.ObjectMeta{
			Name: "shared-obs",
		},
		Spec: openchoreov1alpha1.ClusterObservabilityPlaneSpec{
			PlaneID: "monitoring-cluster",
			ClusterAgent: openchoreov1alpha1.ClusterAgentConfig{
				ClientCA: openchoreov1alpha1.ValueFrom{
					SecretKeyRef: &openchoreov1alpha1.SecretKeyReference{
						Name:      "shared-obs-ca",
						Namespace: "default",
						Key:       "ca.crt",
					},
				},
			},
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(nsCASecret, clusterCASecret, obsPlane, clusterObsPlane).
		Build()

	server := &Server{
		k8sClient: fakeClient,
		logger:    testLogger(),
	}

	result, err := server.getAllPlaneClientCAs(planeTypeObservabilityPlane, "monitoring-cluster")
	require.NoError(t, err)

	assert.Len(t, result, 2)
	assert.Contains(t, result, "monitoring/obs-plane")
	assert.Contains(t, result, "/shared-obs")
}

// TestGetAllPlaneClientCAs_UnsupportedPlaneType tests error handling for unknown plane types
func TestGetAllPlaneClientCAs_UnsupportedPlaneType(t *testing.T) {
	scheme := testScheme()

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		Build()

	server := &Server{
		k8sClient: fakeClient,
		logger:    testLogger(),
	}

	_, err := server.getAllPlaneClientCAs("unknownplane", "some-id")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported plane type")
}

// TestGetAllPlaneClientCAs_WithInlineCA tests CRs with inline CA value
func TestGetAllPlaneClientCAs_WithInlineCA(t *testing.T) {
	scheme := testScheme()

	dataPlane := &openchoreov1alpha1.DataPlane{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "dp-inline",
			Namespace: "test-ns",
		},
		Spec: openchoreov1alpha1.DataPlaneSpec{
			PlaneID: "inline-plane",
			ClusterAgent: openchoreov1alpha1.ClusterAgentConfig{
				ClientCA: openchoreov1alpha1.ValueFrom{
					Value: "inline-ca-certificate-data",
				},
			},
		},
	}

	clusterDataPlane := &openchoreov1alpha1.ClusterDataPlane{
		ObjectMeta: metav1.ObjectMeta{
			Name: "cdp-inline",
		},
		Spec: openchoreov1alpha1.ClusterDataPlaneSpec{
			PlaneID: "inline-plane",
			ClusterAgent: openchoreov1alpha1.ClusterAgentConfig{
				ClientCA: openchoreov1alpha1.ValueFrom{
					Value: "cluster-inline-ca-certificate-data",
				},
			},
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(dataPlane, clusterDataPlane).
		Build()

	server := &Server{
		k8sClient: fakeClient,
		logger:    testLogger(),
	}

	result, err := server.getAllPlaneClientCAs(planeTypeDataPlane, "inline-plane")
	require.NoError(t, err)

	assert.Len(t, result, 2)
	assert.Equal(t, []byte("inline-ca-certificate-data"), result["test-ns/dp-inline"])
	assert.Equal(t, []byte("cluster-inline-ca-certificate-data"), result["/cdp-inline"])
}

// TestExtractPlaneClientCAs tests the extractPlaneClientCAs helper function
func TestExtractPlaneClientCAs(t *testing.T) {
	scheme := testScheme()

	caSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "agent-ca",
			Namespace: "ns1",
		},
		Data: map[string][]byte{
			"ca.crt": []byte("ca-data-1"),
		},
	}

	dp1 := &openchoreov1alpha1.DataPlane{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "dp1",
			Namespace: "ns1",
		},
		Spec: openchoreov1alpha1.DataPlaneSpec{
			PlaneID: "target-plane",
			ClusterAgent: openchoreov1alpha1.ClusterAgentConfig{
				ClientCA: openchoreov1alpha1.ValueFrom{
					SecretKeyRef: &openchoreov1alpha1.SecretKeyReference{
						Name: "agent-ca",
						Key:  "ca.crt",
					},
				},
			},
		},
	}

	dp2 := &openchoreov1alpha1.DataPlane{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "dp2",
			Namespace: "ns2",
		},
		Spec: openchoreov1alpha1.DataPlaneSpec{
			PlaneID: "other-plane", // Different planeID
			ClusterAgent: openchoreov1alpha1.ClusterAgentConfig{
				ClientCA: openchoreov1alpha1.ValueFrom{
					Value: "should-not-be-included",
				},
			},
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(caSecret, dp1, dp2).
		Build()

	server := &Server{
		k8sClient: fakeClient,
		logger:    testLogger(),
	}

	ctx := context.Background()
	result, err := server.extractPlaneClientCAs(ctx, planeTypeDataPlane, "target-plane",
		&openchoreov1alpha1.DataPlaneList{}, dataPlaneInfo)

	require.NoError(t, err)
	assert.Len(t, result, 1)
	assert.Contains(t, result, "ns1/dp1")
	assert.Equal(t, []byte("ca-data-1"), result["ns1/dp1"])
}

// TestCRKeyFormat verifies the key format for namespace vs cluster-scoped resources
func TestCRKeyFormat(t *testing.T) {
	tests := []struct {
		name        string
		namespace   string
		crName      string
		expectedKey string
	}{
		{
			name:        "namespace-scoped CR",
			namespace:   "org-a",
			crName:      "my-dataplane",
			expectedKey: "org-a/my-dataplane",
		},
		{
			name:        "cluster-scoped CR (empty namespace)",
			namespace:   "",
			crName:      "shared-dataplane",
			expectedKey: "/shared-dataplane",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info := planeInfo{
				name:      tt.crName,
				namespace: tt.namespace,
			}
			// This is the key format used in extractPlaneClientCAs
			key := info.namespace + "/" + info.name
			assert.Equal(t, tt.expectedKey, key)
		})
	}
}

// --- parseCACertificates tests ---

func TestParseCACertificates_SingleCert(t *testing.T) {
	caCert, _ := generateTestCA(t)
	pemData := encodeCertToPEM(t, caCert)

	certs, err := parseCACertificates(pemData)
	require.NoError(t, err)
	assert.Len(t, certs, 1)
	assert.Equal(t, "Test CA", certs[0].Subject.CommonName)
}

func TestParseCACertificates_MultipleCerts(t *testing.T) {
	ca1, _ := generateTestCA(t)
	ca2, _ := generateTestCA(t)
	pemData := append(encodeCertToPEM(t, ca1), encodeCertToPEM(t, ca2)...)

	certs, err := parseCACertificates(pemData)
	require.NoError(t, err)
	assert.Len(t, certs, 2)
}

func TestParseCACertificates_InvalidPEM(t *testing.T) {
	_, err := parseCACertificates([]byte("not valid pem data"))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no certificates found")
}

func TestParseCACertificates_InvalidCertDER(t *testing.T) {
	// Valid PEM structure but invalid DER content
	invalidPEM := []byte("-----BEGIN CERTIFICATE-----\nYWJj\n-----END CERTIFICATE-----\n")
	_, err := parseCACertificates(invalidPEM)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse certificate")
}

// --- workflowPlaneInfo / obsPlaneInfo default planeID tests ---

func TestWorkflowPlaneInfo_DefaultPlaneID(t *testing.T) {
	obj := &openchoreov1alpha1.WorkflowPlane{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-wp",
			Namespace: "test-ns",
		},
		Spec: openchoreov1alpha1.WorkflowPlaneSpec{
			PlaneID: "", // Empty - should default to name
		},
	}

	info, ok := workflowPlaneInfo(obj)
	assert.True(t, ok)
	assert.Equal(t, "my-wp", info.planeID)
}

func TestObsPlaneInfo_DefaultPlaneID(t *testing.T) {
	obj := &openchoreov1alpha1.ObservabilityPlane{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-obs",
			Namespace: "test-ns",
		},
		Spec: openchoreov1alpha1.ObservabilityPlaneSpec{
			PlaneID: "", // Empty - should default to name
		},
	}

	info, ok := obsPlaneInfo(obj)
	assert.True(t, ok)
	assert.Equal(t, "my-obs", info.planeID)
}

// --- extractCAFromPlane tests ---

func TestExtractCAFromPlane_NilAgent(t *testing.T) {
	server := &Server{
		k8sClient: fake.NewClientBuilder().WithScheme(testScheme()).Build(),
		logger:    testLogger(),
	}

	data, err := server.extractCAFromPlane(nil, "test-ns")
	assert.NoError(t, err)
	assert.Nil(t, data)
}

// --- extractCADataWithNamespace tests ---

func TestExtractCADataWithNamespace_NoValueNoSecret(t *testing.T) {
	server := &Server{
		k8sClient: fake.NewClientBuilder().WithScheme(testScheme()).Build(),
		logger:    testLogger(),
	}

	valueFrom := &openchoreov1alpha1.ValueFrom{
		// Neither Value nor SecretKeyRef set
	}

	_, err := server.extractCADataWithNamespace(valueFrom, "test-ns")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no valid CA data found")
}

// --- extractCAFromSecret edge case tests ---

func TestExtractCAFromSecret_EmptyName(t *testing.T) {
	server := &Server{
		k8sClient: fake.NewClientBuilder().WithScheme(testScheme()).Build(),
		logger:    testLogger(),
	}

	secretRef := &openchoreov1alpha1.SecretKeyReference{
		Name: "",
		Key:  "ca.crt",
	}

	_, err := server.extractCAFromSecret(secretRef, "test-ns")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "secret name is required")
}

func TestExtractCAFromSecret_EmptyKey(t *testing.T) {
	server := &Server{
		k8sClient: fake.NewClientBuilder().WithScheme(testScheme()).Build(),
		logger:    testLogger(),
	}

	secretRef := &openchoreov1alpha1.SecretKeyReference{
		Name: "my-secret",
		Key:  "",
	}

	_, err := server.extractCAFromSecret(secretRef, "test-ns")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "secret key is required")
}

func TestExtractCAFromSecret_MissingKeyInSecret(t *testing.T) {
	scheme := testScheme()
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "my-secret", Namespace: "test-ns"},
		Data: map[string][]byte{
			"other-key": []byte("some-data"),
		},
	}

	server := &Server{
		k8sClient: fake.NewClientBuilder().WithScheme(scheme).WithObjects(secret).Build(),
		logger:    testLogger(),
	}

	secretRef := &openchoreov1alpha1.SecretKeyReference{
		Name: "my-secret",
		Key:  "ca.crt",
	}

	_, err := server.extractCAFromSecret(secretRef, "test-ns")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "key ca.crt not found")
}

func TestExtractCAFromSecret_EmptyCAData(t *testing.T) {
	scheme := testScheme()
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "my-secret", Namespace: "test-ns"},
		Data: map[string][]byte{
			"ca.crt": {}, // Empty
		},
	}

	server := &Server{
		k8sClient: fake.NewClientBuilder().WithScheme(scheme).WithObjects(secret).Build(),
		logger:    testLogger(),
	}

	secretRef := &openchoreov1alpha1.SecretKeyReference{
		Name: "my-secret",
		Key:  "ca.crt",
	}

	_, err := server.extractCAFromSecret(secretRef, "test-ns")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "CA data is empty")
}

// TestExtractPlaneClientCAs_CAExtractionError tests the continue branch
// when extractCAFromPlane returns an error for a specific CR.
func TestExtractPlaneClientCAs_CAExtractionError(t *testing.T) {
	scheme := testScheme()

	// DataPlane with a secret ref that doesn't exist → extractCAFromPlane will error
	dp := &openchoreov1alpha1.DataPlane{
		ObjectMeta: metav1.ObjectMeta{Name: "dp-bad", Namespace: "ns"},
		Spec: openchoreov1alpha1.DataPlaneSpec{
			PlaneID: "target-plane",
			ClusterAgent: openchoreov1alpha1.ClusterAgentConfig{
				ClientCA: openchoreov1alpha1.ValueFrom{
					SecretKeyRef: &openchoreov1alpha1.SecretKeyReference{
						Name: "nonexistent-secret",
						Key:  "ca.crt",
					},
				},
			},
		},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(dp).Build()
	server := &Server{k8sClient: fakeClient, logger: testLogger()}

	ctx := context.Background()
	result, err := server.extractPlaneClientCAs(ctx, planeTypeDataPlane, "target-plane",
		&openchoreov1alpha1.DataPlaneList{}, dataPlaneInfo)

	// Should not error — just skips the bad CR
	require.NoError(t, err)
	assert.Len(t, result, 0) // The CR with bad secret is skipped
}

// TestExtractPlaneClientCAs_MixedGoodAndBadCRs tests that good CRs are included
// while CRs with CA extraction errors are skipped.
func TestExtractPlaneClientCAs_MixedGoodAndBadCRs(t *testing.T) {
	scheme := testScheme()

	caSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "good-ca", Namespace: "ns"},
		Data:       map[string][]byte{"ca.crt": []byte("good-ca-data")},
	}

	dpGood := &openchoreov1alpha1.DataPlane{
		ObjectMeta: metav1.ObjectMeta{Name: "dp-good", Namespace: "ns"},
		Spec: openchoreov1alpha1.DataPlaneSpec{
			PlaneID: "shared",
			ClusterAgent: openchoreov1alpha1.ClusterAgentConfig{
				ClientCA: openchoreov1alpha1.ValueFrom{
					SecretKeyRef: &openchoreov1alpha1.SecretKeyReference{Name: "good-ca", Key: "ca.crt"},
				},
			},
		},
	}

	dpBad := &openchoreov1alpha1.DataPlane{
		ObjectMeta: metav1.ObjectMeta{Name: "dp-bad", Namespace: "ns"},
		Spec: openchoreov1alpha1.DataPlaneSpec{
			PlaneID: "shared",
			ClusterAgent: openchoreov1alpha1.ClusterAgentConfig{
				ClientCA: openchoreov1alpha1.ValueFrom{
					SecretKeyRef: &openchoreov1alpha1.SecretKeyReference{Name: "missing", Key: "ca.crt"},
				},
			},
		},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(caSecret, dpGood, dpBad).Build()
	server := &Server{k8sClient: fakeClient, logger: testLogger()}

	ctx := context.Background()
	result, err := server.extractPlaneClientCAs(ctx, planeTypeDataPlane, "shared",
		&openchoreov1alpha1.DataPlaneList{}, dataPlaneInfo)

	require.NoError(t, err)
	assert.Len(t, result, 1)
	assert.Contains(t, result, "ns/dp-good")
	assert.Equal(t, []byte("good-ca-data"), result["ns/dp-good"])
}

func TestGetAllPlaneClientCAs_EmptyResult(t *testing.T) {
	scheme := testScheme()
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	server := &Server{k8sClient: fakeClient, logger: testLogger()}

	// No CRs exist for any plane type
	for _, pt := range []string{planeTypeDataPlane, planeTypeWorkflowPlane, planeTypeObservabilityPlane} {
		result, err := server.getAllPlaneClientCAs(pt, "nonexistent")
		require.NoError(t, err)
		assert.Len(t, result, 0)
	}
}
