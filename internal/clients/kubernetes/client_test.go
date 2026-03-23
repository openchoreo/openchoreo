// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package kubernetes

import (
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

// ────────────────── KubeMultiClientManager tests ──────────────────

func TestNewManager(t *testing.T) {
	mgr := NewManager()
	require.NotNil(t, mgr)
	assert.NotNil(t, mgr.clients)
	assert.Empty(t, mgr.clients)
	assert.Nil(t, mgr.ProxyTLSConfig)
}

func TestNewManagerWithProxyTLS(t *testing.T) {
	tlsCfg := &ProxyTLSConfig{
		CACertPath:     "/path/to/ca.crt",
		ClientCertPath: "/path/to/client.crt",
		ClientKeyPath:  "/path/to/client.key",
	}
	mgr := NewManagerWithProxyTLS(tlsCfg)
	require.NotNil(t, mgr)
	assert.Same(t, tlsCfg, mgr.ProxyTLSConfig)
	assert.Empty(t, mgr.clients)
}

func TestGetOrAddClient_CacheMiss(t *testing.T) {
	mgr := NewManager()
	callCount := 0

	cl, err := mgr.GetOrAddClient("key1", func() (client.Client, error) {
		callCount++
		return &ProxyClient{}, nil
	})

	require.NoError(t, err)
	assert.NotNil(t, cl)
	assert.Equal(t, 1, callCount)
}

func TestGetOrAddClient_CacheHit(t *testing.T) {
	mgr := NewManager()
	callCount := 0

	createFunc := func() (client.Client, error) {
		callCount++
		return &ProxyClient{}, nil
	}

	cl1, err := mgr.GetOrAddClient("key1", createFunc)
	require.NoError(t, err)

	cl2, err := mgr.GetOrAddClient("key1", createFunc)
	require.NoError(t, err)

	assert.Equal(t, 1, callCount, "createFunc should only be called once on cache hit")
	assert.Same(t, cl1.(*ProxyClient), cl2.(*ProxyClient))
}

func TestGetOrAddClient_ErrorPropagated(t *testing.T) {
	mgr := NewManager()

	_, err := mgr.GetOrAddClient("key1", func() (client.Client, error) {
		return nil, errTest("create failed")
	})

	require.Error(t, err)
	assert.Equal(t, "create failed", err.Error())

	// Failed creation should NOT be cached
	callCount := 0
	_, _ = mgr.GetOrAddClient("key1", func() (client.Client, error) {
		callCount++
		return &ProxyClient{}, nil
	})
	assert.Equal(t, 1, callCount, "after failed creation, createFunc should be called again")
}

type errTest string

func (e errTest) Error() string { return string(e) }

func TestGetOrAddClient_Concurrent(t *testing.T) {
	mgr := NewManager()
	var callCount int64

	createFunc := func() (client.Client, error) { //nolint:unparam // always returns nil error to satisfy the factory signature
		atomic.AddInt64(&callCount, 1)
		return &ProxyClient{}, nil
	}

	var wg sync.WaitGroup
	const goroutines = 50
	for range goroutines {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = mgr.GetOrAddClient("shared-key", createFunc)
		}()
	}
	wg.Wait()

	assert.Equal(t, int64(1), atomic.LoadInt64(&callCount), "createFunc should be called exactly once across all goroutines")
}

func TestRemoveClient(t *testing.T) {
	t.Run("remove existing client causes re-creation", func(t *testing.T) {
		mgr := NewManager()
		callCount := 0
		createFunc := func() (client.Client, error) {
			callCount++
			return &ProxyClient{}, nil
		}

		_, _ = mgr.GetOrAddClient("key1", createFunc)
		require.Equal(t, 1, callCount)

		mgr.RemoveClient("key1")

		_, _ = mgr.GetOrAddClient("key1", createFunc)
		assert.Equal(t, 2, callCount, "after RemoveClient, createFunc should be called again")
	})

	t.Run("remove non-existent key does not panic", func(t *testing.T) {
		mgr := NewManager()
		// Should not panic
		assert.NotPanics(t, func() { mgr.RemoveClient("nonexistent") })
	})
}

// ──────────────────────── Plane factory function tests ────────────────────────

const testGatewayURL = "https://gateway.example.com"

func TestGetK8sClientFromDataPlane(t *testing.T) {
	t.Run("empty gatewayURL returns error", func(t *testing.T) {
		mgr := NewManager()
		dp := &openchoreov1alpha1.DataPlane{
			ObjectMeta: metav1.ObjectMeta{Name: "my-dp", Namespace: "default"},
		}
		_, err := GetK8sClientFromDataPlane(mgr, dp, "")
		require.Error(t, err)
	})

	t.Run("uses PlaneID from spec", func(t *testing.T) {
		mgr := NewManager()
		dp := &openchoreov1alpha1.DataPlane{
			ObjectMeta: metav1.ObjectMeta{Name: "my-dp", Namespace: "default"},
			Spec:       openchoreov1alpha1.DataPlaneSpec{PlaneID: "custom-id"},
		}
		cl, err := GetK8sClientFromDataPlane(mgr, dp, testGatewayURL)
		require.NoError(t, err)
		require.NotNil(t, cl)
		pc, ok := cl.(*ProxyClient)
		require.True(t, ok, "expected *ProxyClient")
		assert.Equal(t, "custom-id", pc.planeID)
		assert.Equal(t, "dataplane", pc.planeType)
		assert.Equal(t, "default", pc.crNamespace)
		assert.Equal(t, "my-dp", pc.crName)
	})

	t.Run("PlaneID defaults to CR name", func(t *testing.T) {
		mgr := NewManager()
		dp := &openchoreov1alpha1.DataPlane{
			ObjectMeta: metav1.ObjectMeta{Name: "my-dp", Namespace: "default"},
		}
		cl, err := GetK8sClientFromDataPlane(mgr, dp, testGatewayURL)
		require.NoError(t, err)
		require.NotNil(t, cl)
		pc, ok := cl.(*ProxyClient)
		require.True(t, ok, "expected *ProxyClient")
		assert.Equal(t, "my-dp", pc.planeID)
		assert.Equal(t, "dataplane", pc.planeType)
		assert.Equal(t, "default", pc.crNamespace)
		assert.Equal(t, "my-dp", pc.crName)
	})

	t.Run("caches client on repeated calls", func(t *testing.T) {
		mgr := NewManager()
		dp := &openchoreov1alpha1.DataPlane{
			ObjectMeta: metav1.ObjectMeta{Name: "my-dp", Namespace: "default"},
			Spec:       openchoreov1alpha1.DataPlaneSpec{PlaneID: "prod"},
		}
		cl1, err := GetK8sClientFromDataPlane(mgr, dp, testGatewayURL)
		require.NoError(t, err)

		cl2, err := GetK8sClientFromDataPlane(mgr, dp, testGatewayURL)
		require.NoError(t, err)

		assert.Same(t, cl1.(*ProxyClient), cl2.(*ProxyClient))
	})
}

func TestGetK8sClientFromWorkflowPlane(t *testing.T) {
	t.Run("empty gatewayURL returns error", func(t *testing.T) {
		mgr := NewManager()
		wp := &openchoreov1alpha1.WorkflowPlane{
			ObjectMeta: metav1.ObjectMeta{Name: "my-wp", Namespace: "default"},
		}
		_, err := GetK8sClientFromWorkflowPlane(mgr, wp, "")
		require.Error(t, err)
	})

	t.Run("PlaneID defaults to CR name", func(t *testing.T) {
		mgr := NewManager()
		wp := &openchoreov1alpha1.WorkflowPlane{
			ObjectMeta: metav1.ObjectMeta{Name: "ci-wp", Namespace: "default"},
		}
		cl, err := GetK8sClientFromWorkflowPlane(mgr, wp, testGatewayURL)
		require.NoError(t, err)
		require.NotNil(t, cl)
		pc, ok := cl.(*ProxyClient)
		require.True(t, ok, "expected *ProxyClient")
		assert.Equal(t, "ci-wp", pc.planeID)
		assert.Equal(t, "workflowplane", pc.planeType)
		assert.Equal(t, "default", pc.crNamespace)
		assert.Equal(t, "ci-wp", pc.crName)
	})

	t.Run("caches client on repeated calls", func(t *testing.T) {
		mgr := NewManager()
		wp := &openchoreov1alpha1.WorkflowPlane{
			ObjectMeta: metav1.ObjectMeta{Name: "ci-wp", Namespace: "default"},
			Spec:       openchoreov1alpha1.WorkflowPlaneSpec{PlaneID: "ci"},
		}
		cl1, _ := GetK8sClientFromWorkflowPlane(mgr, wp, testGatewayURL)
		cl2, _ := GetK8sClientFromWorkflowPlane(mgr, wp, testGatewayURL)
		assert.Same(t, cl1.(*ProxyClient), cl2.(*ProxyClient))
	})
}

func TestGetK8sClientFromClusterWorkflowPlane(t *testing.T) {
	t.Run("empty gatewayURL returns error", func(t *testing.T) {
		mgr := NewManager()
		cwp := &openchoreov1alpha1.ClusterWorkflowPlane{
			ObjectMeta: metav1.ObjectMeta{Name: "ci-cwp"},
		}
		_, err := GetK8sClientFromClusterWorkflowPlane(mgr, cwp, "")
		require.Error(t, err)
	})

	t.Run("cluster-scoped uses _cluster namespace", func(t *testing.T) {
		mgr := NewManager()
		cwp := &openchoreov1alpha1.ClusterWorkflowPlane{
			ObjectMeta: metav1.ObjectMeta{Name: "global-wp"},
			Spec:       openchoreov1alpha1.ClusterWorkflowPlaneSpec{PlaneID: "global"},
		}
		cl, err := GetK8sClientFromClusterWorkflowPlane(mgr, cwp, testGatewayURL)
		require.NoError(t, err)
		require.NotNil(t, cl)

		pc, ok := cl.(*ProxyClient)
		require.True(t, ok, "expected *ProxyClient")
		assert.Equal(t, "_cluster", pc.crNamespace)
	})
}

func TestGetK8sClientFromClusterDataPlane(t *testing.T) {
	t.Run("empty gatewayURL returns error", func(t *testing.T) {
		mgr := NewManager()
		cdp := &openchoreov1alpha1.ClusterDataPlane{
			ObjectMeta: metav1.ObjectMeta{Name: "global-dp"},
		}
		_, err := GetK8sClientFromClusterDataPlane(mgr, cdp, "")
		require.Error(t, err)
	})

	t.Run("cluster-scoped uses _cluster namespace", func(t *testing.T) {
		mgr := NewManager()
		cdp := &openchoreov1alpha1.ClusterDataPlane{
			ObjectMeta: metav1.ObjectMeta{Name: "global-dp"},
			Spec:       openchoreov1alpha1.ClusterDataPlaneSpec{PlaneID: "global"},
		}
		cl, err := GetK8sClientFromClusterDataPlane(mgr, cdp, testGatewayURL)
		require.NoError(t, err)
		require.NotNil(t, cl)

		pc, ok := cl.(*ProxyClient)
		require.True(t, ok, "expected *ProxyClient")
		assert.Equal(t, "_cluster", pc.crNamespace)
	})

	t.Run("caches client on repeated calls", func(t *testing.T) {
		mgr := NewManager()
		cdp := &openchoreov1alpha1.ClusterDataPlane{
			ObjectMeta: metav1.ObjectMeta{Name: "global-dp"},
			Spec:       openchoreov1alpha1.ClusterDataPlaneSpec{PlaneID: "global"},
		}
		cl1, _ := GetK8sClientFromClusterDataPlane(mgr, cdp, testGatewayURL)
		cl2, _ := GetK8sClientFromClusterDataPlane(mgr, cdp, testGatewayURL)
		assert.Same(t, cl1.(*ProxyClient), cl2.(*ProxyClient))
	})
}

func TestGetK8sClientFromObservabilityPlane(t *testing.T) {
	t.Run("no ClusterAgent.ClientCA.Value returns error", func(t *testing.T) {
		mgr := NewManager()
		op := &openchoreov1alpha1.ObservabilityPlane{
			ObjectMeta: metav1.ObjectMeta{Name: "obs", Namespace: "default"},
		}
		_, err := GetK8sClientFromObservabilityPlane(mgr, op, testGatewayURL)
		require.Error(t, err)
	})

	t.Run("with ClientCA.Value but empty gatewayURL returns error", func(t *testing.T) {
		mgr := NewManager()
		op := &openchoreov1alpha1.ObservabilityPlane{
			ObjectMeta: metav1.ObjectMeta{Name: "obs", Namespace: "default"},
			Spec: openchoreov1alpha1.ObservabilityPlaneSpec{
				ClusterAgent: openchoreov1alpha1.ClusterAgentConfig{
					ClientCA: openchoreov1alpha1.ValueFrom{Value: "some-ca"},
				},
			},
		}
		_, err := GetK8sClientFromObservabilityPlane(mgr, op, "")
		require.Error(t, err)
	})

	t.Run("with ClientCA.Value and valid gatewayURL returns client", func(t *testing.T) {
		mgr := NewManager()
		op := &openchoreov1alpha1.ObservabilityPlane{
			ObjectMeta: metav1.ObjectMeta{Name: "obs", Namespace: "default"},
			Spec: openchoreov1alpha1.ObservabilityPlaneSpec{
				ClusterAgent: openchoreov1alpha1.ClusterAgentConfig{
					ClientCA: openchoreov1alpha1.ValueFrom{Value: "some-ca"},
				},
			},
		}
		cl, err := GetK8sClientFromObservabilityPlane(mgr, op, testGatewayURL)
		require.NoError(t, err)
		assert.NotNil(t, cl)
	})

	t.Run("PlaneID defaults to CR name", func(t *testing.T) {
		mgr := NewManager()
		op := &openchoreov1alpha1.ObservabilityPlane{
			ObjectMeta: metav1.ObjectMeta{Name: "my-obs", Namespace: "default"},
			Spec: openchoreov1alpha1.ObservabilityPlaneSpec{
				ClusterAgent: openchoreov1alpha1.ClusterAgentConfig{
					ClientCA: openchoreov1alpha1.ValueFrom{Value: "ca"},
				},
			},
		}
		cl, err := GetK8sClientFromObservabilityPlane(mgr, op, testGatewayURL)
		require.NoError(t, err)
		require.NotNil(t, cl)
		pc, ok := cl.(*ProxyClient)
		require.True(t, ok, "expected *ProxyClient")
		assert.Equal(t, "my-obs", pc.planeID)
		assert.Equal(t, "observabilityplane", pc.planeType)
		assert.Equal(t, "default", pc.crNamespace)
		assert.Equal(t, "my-obs", pc.crName)
	})
}

func TestGetK8sClientFromClusterObservabilityPlane(t *testing.T) {
	t.Run("no ClusterAgent.ClientCA.Value returns error", func(t *testing.T) {
		mgr := NewManager()
		cop := &openchoreov1alpha1.ClusterObservabilityPlane{
			ObjectMeta: metav1.ObjectMeta{Name: "cluster-obs"},
		}
		_, err := GetK8sClientFromClusterObservabilityPlane(mgr, cop, testGatewayURL)
		require.Error(t, err)
	})

	t.Run("with ClientCA.Value but empty gatewayURL returns error", func(t *testing.T) {
		mgr := NewManager()
		cop := &openchoreov1alpha1.ClusterObservabilityPlane{
			ObjectMeta: metav1.ObjectMeta{Name: "cluster-obs"},
			Spec: openchoreov1alpha1.ClusterObservabilityPlaneSpec{
				ClusterAgent: openchoreov1alpha1.ClusterAgentConfig{
					ClientCA: openchoreov1alpha1.ValueFrom{Value: "ca"},
				},
			},
		}
		_, err := GetK8sClientFromClusterObservabilityPlane(mgr, cop, "")
		require.Error(t, err)
	})

	t.Run("with ClientCA.Value and valid gatewayURL returns client", func(t *testing.T) {
		mgr := NewManager()
		cop := &openchoreov1alpha1.ClusterObservabilityPlane{
			ObjectMeta: metav1.ObjectMeta{Name: "cluster-obs"},
			Spec: openchoreov1alpha1.ClusterObservabilityPlaneSpec{
				ClusterAgent: openchoreov1alpha1.ClusterAgentConfig{
					ClientCA: openchoreov1alpha1.ValueFrom{Value: "some-ca"},
				},
			},
		}
		cl, err := GetK8sClientFromClusterObservabilityPlane(mgr, cop, testGatewayURL)
		require.NoError(t, err)
		require.NotNil(t, cl)

		// Cluster-scoped uses _cluster namespace
		pc, ok := cl.(*ProxyClient)
		require.True(t, ok, "expected *ProxyClient")
		assert.Equal(t, "_cluster", pc.crNamespace)
	})

	t.Run("caches client on repeated calls", func(t *testing.T) {
		mgr := NewManager()
		cop := &openchoreov1alpha1.ClusterObservabilityPlane{
			ObjectMeta: metav1.ObjectMeta{Name: "cluster-obs"},
			Spec: openchoreov1alpha1.ClusterObservabilityPlaneSpec{
				ClusterAgent: openchoreov1alpha1.ClusterAgentConfig{
					ClientCA: openchoreov1alpha1.ValueFrom{Value: "ca"},
				},
			},
		}
		cl1, _ := GetK8sClientFromClusterObservabilityPlane(mgr, cop, testGatewayURL)
		cl2, _ := GetK8sClientFromClusterObservabilityPlane(mgr, cop, testGatewayURL)
		assert.Same(t, cl1.(*ProxyClient), cl2.(*ProxyClient))
	})
}
