// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package k8sresources

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	authzmocks "github.com/openchoreo/openchoreo/internal/authz/core/mocks"
	"github.com/openchoreo/openchoreo/internal/clients/gateway"
	"github.com/openchoreo/openchoreo/internal/controller"
)

// testScheme returns a scheme with OpenChoreo and standard K8s types registered.
func testScheme() *runtime.Scheme {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = appsv1.AddToScheme(scheme)
	_ = batchv1.AddToScheme(scheme)
	_ = openchoreov1alpha1.AddToScheme(scheme)
	return scheme
}

// testLogger returns a discard logger.
func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError + 1}))
}

// testGatewayServer creates a non-TLS HTTP test server and a gateway Client pointing to it.
// The handler receives all proxied requests.
func testGatewayServer(t *testing.T, handler http.HandlerFunc) *gateway.Client {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	return gateway.NewClient(server.URL)
}

// testRESTMapper creates a REST mapper with standard K8s type mappings.
func testRESTMapper() meta.RESTMapper {
	mapper := meta.NewDefaultRESTMapper([]schema.GroupVersion{
		{Group: "", Version: "v1"},
		{Group: "apps", Version: "v1"},
		{Group: "batch", Version: "v1"},
	})
	mapper.Add(schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Pod"}, meta.RESTScopeNamespace)
	mapper.Add(schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Service"}, meta.RESTScopeNamespace)
	mapper.Add(schema.GroupVersionKind{Group: "", Version: "v1", Kind: "ConfigMap"}, meta.RESTScopeNamespace)
	mapper.Add(schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Namespace"}, meta.RESTScopeRoot)
	mapper.Add(schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"}, meta.RESTScopeNamespace)
	mapper.Add(schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "ReplicaSet"}, meta.RESTScopeNamespace)
	mapper.Add(schema.GroupVersionKind{Group: "batch", Version: "v1", Kind: "Job"}, meta.RESTScopeNamespace)
	mapper.Add(schema.GroupVersionKind{Group: "batch", Version: "v1", Kind: "CronJob"}, meta.RESTScopeNamespace)
	return mapper
}

// newFakeClient creates a fake K8s client with the given objects.
func newFakeClient(objects ...client.Object) client.Client {
	return fake.NewClientBuilder().
		WithScheme(testScheme()).
		WithRESTMapper(testRESTMapper()).
		WithObjects(objects...).
		WithStatusSubresource(&openchoreov1alpha1.RenderedRelease{}).
		Build()
}

const testNamespace = "ns-1"

// testReleaseBinding creates a ReleaseBinding fixture with an owner ref UID for matching.
func testReleaseBinding() *openchoreov1alpha1.ReleaseBinding {
	return &openchoreov1alpha1.ReleaseBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "rb-1",
			Namespace: testNamespace,
			UID:       "rb-1-uid",
		},
		Spec: openchoreov1alpha1.ReleaseBindingSpec{
			Owner: openchoreov1alpha1.ReleaseBindingOwner{
				ProjectName:   "proj-1",
				ComponentName: "comp-1",
			},
			Environment: "dev",
		},
	}
}

// testRenderedRelease creates a RenderedRelease owned by the given ReleaseBinding.
func testRenderedRelease(rb *openchoreov1alpha1.ReleaseBinding, targetPlane string, resources []openchoreov1alpha1.ResourceStatus) *openchoreov1alpha1.RenderedRelease {
	return &openchoreov1alpha1.RenderedRelease{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "rr-1",
			Namespace: testNamespace,
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: "core.choreo.dev/v1alpha1",
					Kind:       "ReleaseBinding",
					Name:       rb.Name,
					UID:        rb.UID,
					Controller: boolPtr(true),
				},
			},
		},
		Spec: openchoreov1alpha1.RenderedReleaseSpec{
			Owner: openchoreov1alpha1.RenderedReleaseOwner{
				ProjectName:   rb.Spec.Owner.ProjectName,
				ComponentName: rb.Spec.Owner.ComponentName,
			},
			EnvironmentName: rb.Spec.Environment,
			TargetPlane:     targetPlane,
		},
		Status: openchoreov1alpha1.RenderedReleaseStatus{
			Resources: resources,
		},
	}
}

func testDataPlane(name string) *openchoreov1alpha1.DataPlane {
	return &openchoreov1alpha1.DataPlane{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: testNamespace,
		},
		Spec: openchoreov1alpha1.DataPlaneSpec{
			PlaneID: name + "-id",
			ClusterAgent: openchoreov1alpha1.ClusterAgentConfig{
				ClientCA: openchoreov1alpha1.ValueFrom{Value: "test-ca"},
			},
		},
	}
}

func testEnvironment() *openchoreov1alpha1.Environment {
	return &openchoreov1alpha1.Environment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "dev",
			Namespace: testNamespace,
		},
	}
}

func boolPtr(b bool) *bool { return &b }

// k8sObject builds a minimal unstructured K8s object map for test responses.
func k8sObject(apiVersion, kind, namespace, name, uid string) map[string]any {
	obj := map[string]any{
		"apiVersion": apiVersion,
		"kind":       kind,
		"metadata": map[string]any{
			"name":              name,
			"namespace":         namespace,
			"uid":               uid,
			"resourceVersion":   "1",
			"creationTimestamp": "2024-01-15T10:00:00Z",
		},
	}
	return obj
}

// k8sList wraps items into a Kubernetes list response.
func k8sList(items ...map[string]any) map[string]any {
	anyItems := make([]any, len(items))
	for i, item := range items {
		anyItems[i] = item
	}
	return map[string]any{
		"apiVersion": "v1",
		"kind":       "List",
		"items":      anyItems,
	}
}

func jsonMarshal(t *testing.T, v any) []byte {
	t.Helper()
	data, err := json.Marshal(v)
	require.NoError(t, err)
	return data
}

// --- NewService ---

func TestNewService(t *testing.T) {
	fc := newFakeClient()
	gc := gateway.NewClient("http://localhost")
	svc := NewService(fc, gc, testLogger())
	require.NotNil(t, svc)
}

// --- resolveReleaseContexts ---

func TestResolveReleaseContexts(t *testing.T) {
	t.Run("release binding not found", func(t *testing.T) {
		fc := newFakeClient()
		svc := &k8sResourcesService{k8sClient: fc, logger: testLogger()}

		_, err := svc.resolveReleaseContexts(context.Background(), testNamespace, "nonexistent")
		require.ErrorIs(t, err, ErrReleaseBindingNotFound)
	})

	t.Run("no owned releases returns nil", func(t *testing.T) {
		rb := testReleaseBinding()
		env := testEnvironment()
		dp := testDataPlane("default")
		fc := newFakeClient(rb, env, dp)
		svc := &k8sResourcesService{k8sClient: fc, logger: testLogger()}

		contexts, err := svc.resolveReleaseContexts(context.Background(), testNamespace, "rb-1")
		require.NoError(t, err)
		require.Nil(t, contexts)
	})

	t.Run("environment not found", func(t *testing.T) {
		rb := testReleaseBinding()
		rr := testRenderedRelease(rb, planeTypeDataPlane, []openchoreov1alpha1.ResourceStatus{
			{ID: "dep", Group: "apps", Version: "v1", Kind: "Deployment", Name: "web", Namespace: "dp-ns"},
		})
		fc := newFakeClient(rb, rr)
		svc := &k8sResourcesService{k8sClient: fc, logger: testLogger()}

		_, err := svc.resolveReleaseContexts(context.Background(), testNamespace, "rb-1")
		require.ErrorIs(t, err, ErrEnvironmentNotFound)
	})

	t.Run("success with dataplane release", func(t *testing.T) {
		rb := testReleaseBinding()
		env := testEnvironment()
		dp := testDataPlane("default")
		rr := testRenderedRelease(rb, planeTypeDataPlane, []openchoreov1alpha1.ResourceStatus{
			{ID: "dep", Group: "apps", Version: "v1", Kind: "Deployment", Name: "web", Namespace: "dp-ns"},
		})
		fc := newFakeClient(rb, env, dp, rr)
		svc := &k8sResourcesService{k8sClient: fc, logger: testLogger()}

		contexts, err := svc.resolveReleaseContexts(context.Background(), testNamespace, "rb-1")
		require.NoError(t, err)
		require.Len(t, contexts, 1)
		assert.Equal(t, "rr-1", contexts[0].release.Name)
		assert.Equal(t, planeTypeDataPlane, contexts[0].plane.planeType)
		assert.Equal(t, "default-id", contexts[0].plane.planeID)
		assert.Equal(t, "dp-ns", contexts[0].namespace)
	})
}

// --- resolvePlaneInfo ---

func TestResolvePlaneInfo(t *testing.T) {
	t.Run("dataplane target returns dataplane info", func(t *testing.T) {
		dp := testDataPlane("my-dp")
		dpResult := &controller.DataPlaneResult{DataPlane: dp}

		release := &openchoreov1alpha1.RenderedRelease{
			Spec: openchoreov1alpha1.RenderedReleaseSpec{TargetPlane: planeTypeDataPlane},
		}
		svc := &k8sResourcesService{k8sClient: newFakeClient(), logger: testLogger()}

		pi, err := svc.resolvePlaneInfo(context.Background(), release, dpResult)
		require.NoError(t, err)
		assert.Equal(t, planeTypeDataPlane, pi.planeType)
		assert.Equal(t, "my-dp-id", pi.planeID)
	})

	t.Run("observabilityplane target resolves obs plane", func(t *testing.T) {
		dp := testDataPlane("my-dp")
		obs := &openchoreov1alpha1.ObservabilityPlane{
			ObjectMeta: metav1.ObjectMeta{Name: "default", Namespace: testNamespace},
			Spec: openchoreov1alpha1.ObservabilityPlaneSpec{
				PlaneID:     "obs-id",
				ObserverURL: "https://observer.test",
				ClusterAgent: openchoreov1alpha1.ClusterAgentConfig{
					ClientCA: openchoreov1alpha1.ValueFrom{Value: "test-ca"},
				},
			},
		}
		fc := newFakeClient(dp, obs)
		dpResult := &controller.DataPlaneResult{DataPlane: dp}

		release := &openchoreov1alpha1.RenderedRelease{
			Spec: openchoreov1alpha1.RenderedReleaseSpec{TargetPlane: planeTypeObservabilityPlane},
		}
		svc := &k8sResourcesService{k8sClient: fc, logger: testLogger()}

		pi, err := svc.resolvePlaneInfo(context.Background(), release, dpResult)
		require.NoError(t, err)
		assert.Equal(t, planeTypeObservabilityPlane, pi.planeType)
		assert.Equal(t, "obs-id", pi.planeID)
	})
}

// --- GetResourceTree ---

func TestGetResourceTree(t *testing.T) {
	t.Run("nil gateway client returns error", func(t *testing.T) {
		fc := newFakeClient()
		svc := NewService(fc, nil, testLogger())

		_, err := svc.GetResourceTree(context.Background(), testNamespace, "rb-1")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "gateway client is not configured")
	})

	t.Run("release binding not found", func(t *testing.T) {
		gc := testGatewayServer(t, func(w http.ResponseWriter, r *http.Request) {})
		fc := newFakeClient()
		svc := NewService(fc, gc, testLogger())

		_, err := svc.GetResourceTree(context.Background(), testNamespace, "nonexistent")
		require.ErrorIs(t, err, ErrReleaseBindingNotFound)
	})

	t.Run("empty tree when no owned releases", func(t *testing.T) {
		rb := testReleaseBinding()
		env := testEnvironment()
		dp := testDataPlane("default")
		gc := testGatewayServer(t, func(w http.ResponseWriter, r *http.Request) {})
		fc := newFakeClient(rb, env, dp)
		svc := NewService(fc, gc, testLogger())

		result, err := svc.GetResourceTree(context.Background(), testNamespace, "rb-1")
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Empty(t, result.RenderedReleases)
	})

	t.Run("success returns resource tree with nodes", func(t *testing.T) {
		rb := testReleaseBinding()
		env := testEnvironment()
		dp := testDataPlane("default")
		rr := testRenderedRelease(rb, planeTypeDataPlane, []openchoreov1alpha1.ResourceStatus{
			{ID: "svc", Group: "", Version: "v1", Kind: "Service", Name: "web-svc", Namespace: "dp-ns"},
		})
		fc := newFakeClient(rb, env, dp, rr)

		svcObj := k8sObject("v1", "Service", "dp-ns", "web-svc", "svc-uid-1")

		gc := testGatewayServer(t, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(jsonMarshal(t, svcObj))
		})

		svc := NewService(fc, gc, testLogger())
		result, err := svc.GetResourceTree(context.Background(), testNamespace, "rb-1")
		require.NoError(t, err)
		require.NotNil(t, result)
		require.Len(t, result.RenderedReleases, 1)
		assert.Equal(t, "rr-1", result.RenderedReleases[0].Name)
		assert.Equal(t, planeTypeDataPlane, result.RenderedReleases[0].TargetPlane)
		require.Len(t, result.RenderedReleases[0].Nodes, 1)
		assert.Equal(t, "Service", result.RenderedReleases[0].Nodes[0].Kind)
		assert.Equal(t, "web-svc", result.RenderedReleases[0].Nodes[0].Name)
	})

	t.Run("gateway 500 skips resource gracefully", func(t *testing.T) {
		rb := testReleaseBinding()
		env := testEnvironment()
		dp := testDataPlane("default")
		rr := testRenderedRelease(rb, planeTypeDataPlane, []openchoreov1alpha1.ResourceStatus{
			{ID: "svc", Group: "", Version: "v1", Kind: "Service", Name: "web-svc", Namespace: "dp-ns"},
		})
		fc := newFakeClient(rb, env, dp, rr)

		gc := testGatewayServer(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		})

		svc := NewService(fc, gc, testLogger())
		result, err := svc.GetResourceTree(context.Background(), testNamespace, "rb-1")
		require.NoError(t, err)
		require.Len(t, result.RenderedReleases, 1)
		assert.Empty(t, result.RenderedReleases[0].Nodes)
	})
}

// --- GetResourceEvents ---

func TestGetResourceEvents(t *testing.T) {
	t.Run("nil gateway client returns error", func(t *testing.T) {
		fc := newFakeClient()
		svc := NewService(fc, nil, testLogger())

		_, err := svc.GetResourceEvents(context.Background(), testNamespace, "rb-1", "apps", "v1", "Deployment", "web")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "gateway client is not configured")
	})

	t.Run("resource not found returns error", func(t *testing.T) {
		rb := testReleaseBinding()
		env := testEnvironment()
		dp := testDataPlane("default")
		rr := testRenderedRelease(rb, planeTypeDataPlane, []openchoreov1alpha1.ResourceStatus{
			{ID: "dep", Group: "apps", Version: "v1", Kind: "Deployment", Name: "web", Namespace: "dp-ns"},
		})
		fc := newFakeClient(rb, env, dp, rr)
		gc := testGatewayServer(t, func(w http.ResponseWriter, r *http.Request) {})
		svc := NewService(fc, gc, testLogger())

		_, err := svc.GetResourceEvents(context.Background(), testNamespace, "rb-1", "", "v1", "ConfigMap", "missing")
		require.ErrorIs(t, err, ErrResourceNotFound)
	})

	t.Run("success returns events", func(t *testing.T) {
		rb := testReleaseBinding()
		env := testEnvironment()
		dp := testDataPlane("default")
		rr := testRenderedRelease(rb, planeTypeDataPlane, []openchoreov1alpha1.ResourceStatus{
			{ID: "dep", Group: "apps", Version: "v1", Kind: "Deployment", Name: "web", Namespace: "dp-ns"},
		})
		fc := newFakeClient(rb, env, dp, rr)

		eventItem := map[string]any{
			"type":           "Normal",
			"reason":         "ScalingReplicaSet",
			"message":        "Scaled up replica set web-abc to 1",
			"firstTimestamp": "2024-01-15T10:00:00Z",
			"lastTimestamp":  "2024-01-15T10:00:00Z",
			"source":         map[string]any{"component": "deployment-controller"},
		}
		eventList := k8sList(eventItem)

		gc := testGatewayServer(t, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(jsonMarshal(t, eventList))
		})

		svc := NewService(fc, gc, testLogger())
		result, err := svc.GetResourceEvents(context.Background(), testNamespace, "rb-1", "apps", "v1", "Deployment", "web")
		require.NoError(t, err)
		require.Len(t, result.Events, 1)
		assert.Equal(t, "Normal", result.Events[0].Type)
		assert.Equal(t, "ScalingReplicaSet", result.Events[0].Reason)
		assert.Equal(t, "deployment-controller", result.Events[0].Source)
	})

	t.Run("events for cluster-scoped resource", func(t *testing.T) {
		rb := testReleaseBinding()
		env := testEnvironment()
		dp := testDataPlane("default")
		rr := testRenderedRelease(rb, planeTypeDataPlane, []openchoreov1alpha1.ResourceStatus{
			{ID: "ns", Group: "", Version: "v1", Kind: "Namespace", Name: "my-ns"},
		})
		fc := newFakeClient(rb, env, dp, rr)

		var capturedPath string
		gc := testGatewayServer(t, func(w http.ResponseWriter, r *http.Request) {
			capturedPath = r.URL.Path
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(jsonMarshal(t, k8sList()))
		})

		svc := NewService(fc, gc, testLogger())
		result, err := svc.GetResourceEvents(context.Background(), testNamespace, "rb-1", "", "v1", "Namespace", "my-ns")
		require.NoError(t, err)
		assert.Empty(t, result.Events)
		// For cluster-scoped resource (empty namespace), the events path should be cluster-level
		assert.Contains(t, capturedPath, "api/v1/events")
	})
}

// --- GetResourceLogs ---

func TestGetResourceLogs(t *testing.T) {
	t.Run("nil gateway client returns error", func(t *testing.T) {
		fc := newFakeClient()
		svc := NewService(fc, nil, testLogger())

		_, err := svc.GetResourceLogs(context.Background(), testNamespace, "rb-1", "pod-1", nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "gateway client is not configured")
	})

	t.Run("no dataplane release with parent resources returns not found", func(t *testing.T) {
		rb := testReleaseBinding()
		env := testEnvironment()
		dp := testDataPlane("default")
		// Only an observability plane release — no parent for pods
		rr := testRenderedRelease(rb, planeTypeObservabilityPlane, []openchoreov1alpha1.ResourceStatus{
			{ID: "cm", Group: "", Version: "v1", Kind: "ConfigMap", Name: "obs-cfg", Namespace: "obs-ns"},
		})
		fc := newFakeClient(rb, env, dp, rr)
		gc := testGatewayServer(t, func(w http.ResponseWriter, r *http.Request) {})
		svc := NewService(fc, gc, testLogger())

		_, err := svc.GetResourceLogs(context.Background(), testNamespace, "rb-1", "pod-1", nil)
		require.ErrorIs(t, err, ErrResourceNotFound)
	})

	t.Run("success returns parsed logs", func(t *testing.T) {
		rb := testReleaseBinding()
		env := testEnvironment()
		dp := testDataPlane("default")
		rr := testRenderedRelease(rb, planeTypeDataPlane, []openchoreov1alpha1.ResourceStatus{
			{ID: "dep", Group: "apps", Version: "v1", Kind: "Deployment", Name: "web", Namespace: "dp-ns"},
		})
		fc := newFakeClient(rb, env, dp, rr)

		gc := testGatewayServer(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("2024-01-15T10:00:00Z Starting server\n2024-01-15T10:00:01Z Ready\n"))
		})

		svc := NewService(fc, gc, testLogger())
		result, err := svc.GetResourceLogs(context.Background(), testNamespace, "rb-1", "pod-1", nil)
		require.NoError(t, err)
		require.Len(t, result.LogEntries, 2)
		assert.Equal(t, "Starting server", result.LogEntries[0].Log)
		assert.Equal(t, "Ready", result.LogEntries[1].Log)
	})

	t.Run("permanent gateway error returns not found", func(t *testing.T) {
		rb := testReleaseBinding()
		env := testEnvironment()
		dp := testDataPlane("default")
		rr := testRenderedRelease(rb, planeTypeDataPlane, []openchoreov1alpha1.ResourceStatus{
			{ID: "dep", Group: "apps", Version: "v1", Kind: "Deployment", Name: "web", Namespace: "dp-ns"},
		})
		fc := newFakeClient(rb, env, dp, rr)

		gc := testGatewayServer(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		})

		svc := NewService(fc, gc, testLogger())
		_, err := svc.GetResourceLogs(context.Background(), testNamespace, "rb-1", "pod-1", nil)
		require.ErrorIs(t, err, ErrResourceNotFound)
	})

	t.Run("with sinceSeconds", func(t *testing.T) {
		rb := testReleaseBinding()
		env := testEnvironment()
		dp := testDataPlane("default")
		rr := testRenderedRelease(rb, planeTypeDataPlane, []openchoreov1alpha1.ResourceStatus{
			{ID: "dep", Group: "apps", Version: "v1", Kind: "Deployment", Name: "web", Namespace: "dp-ns"},
		})
		fc := newFakeClient(rb, env, dp, rr)

		var capturedURL string
		gc := testGatewayServer(t, func(w http.ResponseWriter, r *http.Request) {
			capturedURL = r.URL.String()
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("2024-01-15T10:00:00Z recent log\n"))
		})

		svc := NewService(fc, gc, testLogger())
		since := int64(300)
		result, err := svc.GetResourceLogs(context.Background(), testNamespace, "rb-1", "pod-1", &since)
		require.NoError(t, err)
		require.Len(t, result.LogEntries, 1)
		assert.Contains(t, capturedURL, "sinceSeconds=300")
	})
}

// --- fetchLiveResource ---

func TestFetchLiveResource(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		obj := k8sObject("v1", "Service", "ns1", "svc1", "uid-1")
		gc := testGatewayServer(t, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(jsonMarshal(t, obj))
		})

		svc := &k8sResourcesService{gatewayClient: gc, logger: testLogger()}
		pi := planeInfo{planeType: "dataplane", planeID: "dp1", crNamespace: "ns", crName: "dp"}

		result, err := svc.fetchLiveResource(context.Background(), pi, "api/v1/namespaces/ns1/services/svc1")
		require.NoError(t, err)
		assert.Equal(t, "Service", result["kind"])
	})

	t.Run("non-200 returns error", func(t *testing.T) {
		gc := testGatewayServer(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		})

		svc := &k8sResourcesService{gatewayClient: gc, logger: testLogger()}
		pi := planeInfo{planeType: "dataplane", planeID: "dp1", crNamespace: "ns", crName: "dp"}

		_, err := svc.fetchLiveResource(context.Background(), pi, "api/v1/namespaces/ns1/services/missing")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unexpected status 404")
	})

	t.Run("invalid JSON returns error", func(t *testing.T) {
		gc := testGatewayServer(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("not json"))
		})

		svc := &k8sResourcesService{gatewayClient: gc, logger: testLogger()}
		pi := planeInfo{planeType: "dataplane", planeID: "dp1", crNamespace: "ns", crName: "dp"}

		_, err := svc.fetchLiveResource(context.Background(), pi, "api/v1/namespaces/ns1/pods/p1")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to unmarshal response")
	})
}

// --- fetchK8sList ---

func TestFetchK8sList(t *testing.T) {
	t.Run("success returns items", func(t *testing.T) {
		items := k8sList(
			k8sObject("v1", "Pod", "ns1", "pod-1", "uid-1"),
			k8sObject("v1", "Pod", "ns1", "pod-2", "uid-2"),
		)
		gc := testGatewayServer(t, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(jsonMarshal(t, items))
		})

		svc := &k8sResourcesService{gatewayClient: gc, logger: testLogger()}
		pi := planeInfo{planeType: "dataplane", planeID: "dp1", crNamespace: "ns", crName: "dp"}

		result, err := svc.fetchK8sList(context.Background(), pi, "api/v1/namespaces/ns1/pods", "")
		require.NoError(t, err)
		require.Len(t, result, 2)
	})

	t.Run("empty items returns nil", func(t *testing.T) {
		gc := testGatewayServer(t, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(jsonMarshal(t, map[string]any{"items": nil}))
		})

		svc := &k8sResourcesService{gatewayClient: gc, logger: testLogger()}
		pi := planeInfo{planeType: "dataplane", planeID: "dp1", crNamespace: "ns", crName: "dp"}

		result, err := svc.fetchK8sList(context.Background(), pi, "api/v1/namespaces/ns1/pods", "")
		require.NoError(t, err)
		assert.Nil(t, result)
	})

	t.Run("non-200 returns error", func(t *testing.T) {
		gc := testGatewayServer(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusForbidden)
		})

		svc := &k8sResourcesService{gatewayClient: gc, logger: testLogger()}
		pi := planeInfo{planeType: "dataplane", planeID: "dp1", crNamespace: "ns", crName: "dp"}

		_, err := svc.fetchK8sList(context.Background(), pi, "api/v1/namespaces/ns1/pods", "")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unexpected status 403")
	})

	t.Run("with query params", func(t *testing.T) {
		var capturedQuery string
		gc := testGatewayServer(t, func(w http.ResponseWriter, r *http.Request) {
			capturedQuery = r.URL.RawQuery
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(jsonMarshal(t, k8sList()))
		})

		svc := &k8sResourcesService{gatewayClient: gc, logger: testLogger()}
		pi := planeInfo{planeType: "dataplane", planeID: "dp1", crNamespace: "ns", crName: "dp"}

		_, err := svc.fetchK8sList(context.Background(), pi, "api/v1/events", "fieldSelector=involvedObject.kind=Deployment")
		require.NoError(t, err)
		assert.Contains(t, capturedQuery, "fieldSelector=involvedObject.kind=Deployment")
	})
}

// --- fetchChildResources ---

func TestFetchChildResources(t *testing.T) {
	t.Run("Deployment fetches ReplicaSets then Pods", func(t *testing.T) {
		rsObj := k8sObject("apps/v1", "ReplicaSet", "dp-ns", "web-abc", "rs-uid-1")
		rsObj["metadata"].(map[string]any)["ownerReferences"] = []any{
			map[string]any{"uid": "deploy-uid-1"},
		}

		podObj := k8sObject("v1", "Pod", "dp-ns", "web-abc-xyz", "pod-uid-1")
		podObj["metadata"].(map[string]any)["ownerReferences"] = []any{
			map[string]any{"uid": "rs-uid-1"},
		}

		callCount := 0
		gc := testGatewayServer(t, func(w http.ResponseWriter, r *http.Request) {
			callCount++
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			path := r.URL.Path
			if strings.Contains(path, "replicasets") {
				_, _ = w.Write(jsonMarshal(t, k8sList(rsObj)))
			} else if strings.Contains(path, "pods") {
				_, _ = w.Write(jsonMarshal(t, k8sList(podObj)))
			} else {
				_, _ = w.Write(jsonMarshal(t, k8sList()))
			}
		})

		fc := newFakeClient()
		svc := &k8sResourcesService{k8sClient: fc, gatewayClient: gc, logger: testLogger()}

		parentObj := k8sObject("apps/v1", "Deployment", "dp-ns", "web", "deploy-uid-1")
		rs := &openchoreov1alpha1.ResourceStatus{
			Group: "apps", Version: "v1", Kind: "Deployment", Name: "web", Namespace: "dp-ns",
		}
		pi := planeInfo{planeType: "dataplane", planeID: "dp1", crNamespace: "ns", crName: "dp"}

		nodes := svc.fetchChildResources(context.Background(), pi, parentObj, rs)
		// Should find the pod through replicaset chain
		require.Len(t, nodes, 1)
		assert.Equal(t, "Pod", nodes[0].Kind)
		assert.Equal(t, "web-abc-xyz", nodes[0].Name)
	})

	t.Run("Job fetches owned Pods", func(t *testing.T) {
		podObj := k8sObject("v1", "Pod", "dp-ns", "job-pod-1", "pod-uid-1")
		podObj["metadata"].(map[string]any)["ownerReferences"] = []any{
			map[string]any{"uid": "job-uid-1"},
		}

		gc := testGatewayServer(t, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(jsonMarshal(t, k8sList(podObj)))
		})

		fc := newFakeClient()
		svc := &k8sResourcesService{k8sClient: fc, gatewayClient: gc, logger: testLogger()}

		parentObj := k8sObject("batch/v1", "Job", "dp-ns", "my-job", "job-uid-1")
		rs := &openchoreov1alpha1.ResourceStatus{
			Group: "batch", Version: "v1", Kind: "Job", Name: "my-job", Namespace: "dp-ns",
		}
		pi := planeInfo{planeType: "dataplane", planeID: "dp1", crNamespace: "ns", crName: "dp"}

		nodes := svc.fetchChildResources(context.Background(), pi, parentObj, rs)
		require.Len(t, nodes, 1)
		assert.Equal(t, "Pod", nodes[0].Kind)
		assert.Equal(t, "job-pod-1", nodes[0].Name)
	})

	t.Run("CronJob fetches Jobs then Pods", func(t *testing.T) {
		jobObj := k8sObject("batch/v1", "Job", "dp-ns", "cj-job-1", "job-uid-1")
		jobObj["metadata"].(map[string]any)["ownerReferences"] = []any{
			map[string]any{"uid": "cj-uid-1"},
		}

		podObj := k8sObject("v1", "Pod", "dp-ns", "cj-job-1-pod", "pod-uid-1")
		podObj["metadata"].(map[string]any)["ownerReferences"] = []any{
			map[string]any{"uid": "job-uid-1"},
		}

		gc := testGatewayServer(t, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			path := r.URL.Path
			if strings.Contains(path, "jobs") {
				_, _ = w.Write(jsonMarshal(t, k8sList(jobObj)))
			} else if strings.Contains(path, "pods") {
				_, _ = w.Write(jsonMarshal(t, k8sList(podObj)))
			} else {
				_, _ = w.Write(jsonMarshal(t, k8sList()))
			}
		})

		fc := newFakeClient()
		svc := &k8sResourcesService{k8sClient: fc, gatewayClient: gc, logger: testLogger()}

		parentObj := k8sObject("batch/v1", "CronJob", "dp-ns", "my-cj", "cj-uid-1")
		rs := &openchoreov1alpha1.ResourceStatus{
			Group: "batch", Version: "v1", Kind: "CronJob", Name: "my-cj", Namespace: "dp-ns",
		}
		pi := planeInfo{planeType: "dataplane", planeID: "dp1", crNamespace: "ns", crName: "dp"}

		nodes := svc.fetchChildResources(context.Background(), pi, parentObj, rs)
		// Should find both the Job and the Pod
		require.Len(t, nodes, 2)
		kinds := []string{nodes[0].Kind, nodes[1].Kind}
		assert.Contains(t, kinds, "Job")
		assert.Contains(t, kinds, "Pod")
	})

	t.Run("Service has no child resources", func(t *testing.T) {
		gc := testGatewayServer(t, func(w http.ResponseWriter, r *http.Request) {
			t.Fatal("should not be called for Service kind")
		})

		fc := newFakeClient()
		svc := &k8sResourcesService{k8sClient: fc, gatewayClient: gc, logger: testLogger()}

		parentObj := k8sObject("v1", "Service", "dp-ns", "web-svc", "svc-uid-1")
		rs := &openchoreov1alpha1.ResourceStatus{
			Group: "", Version: "v1", Kind: "Service", Name: "web-svc", Namespace: "dp-ns",
		}
		pi := planeInfo{planeType: "dataplane", planeID: "dp1", crNamespace: "ns", crName: "dp"}

		nodes := svc.fetchChildResources(context.Background(), pi, parentObj, rs)
		assert.Empty(t, nodes)
	})
}

// --- fetchOwnedResources ---

func TestFetchOwnedResources(t *testing.T) {
	t.Run("filters by owner UID", func(t *testing.T) {
		ownedPod := k8sObject("v1", "Pod", "ns1", "pod-owned", "pod-uid-1")
		ownedPod["metadata"].(map[string]any)["ownerReferences"] = []any{
			map[string]any{"uid": "owner-uid"},
		}
		unownedPod := k8sObject("v1", "Pod", "ns1", "pod-other", "pod-uid-2")
		unownedPod["metadata"].(map[string]any)["ownerReferences"] = []any{
			map[string]any{"uid": "different-uid"},
		}

		gc := testGatewayServer(t, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(jsonMarshal(t, k8sList(ownedPod, unownedPod)))
		})

		fc := newFakeClient()
		svc := &k8sResourcesService{k8sClient: fc, gatewayClient: gc, logger: testLogger()}
		pi := planeInfo{planeType: "dataplane", planeID: "dp1", crNamespace: "ns", crName: "dp"}

		owned := svc.fetchOwnedResources(context.Background(), pi, "", "Pod", "ns1", "owner-uid")
		require.Len(t, owned, 1)
		assert.Equal(t, "Pod", owned[0]["kind"])
		assert.Equal(t, "v1", owned[0]["apiVersion"])
	})

	t.Run("with API group", func(t *testing.T) {
		rsObj := k8sObject("apps/v1", "ReplicaSet", "ns1", "rs-1", "rs-uid-1")
		rsObj["metadata"].(map[string]any)["ownerReferences"] = []any{
			map[string]any{"uid": "deploy-uid"},
		}

		gc := testGatewayServer(t, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(jsonMarshal(t, k8sList(rsObj)))
		})

		fc := newFakeClient()
		svc := &k8sResourcesService{k8sClient: fc, gatewayClient: gc, logger: testLogger()}
		pi := planeInfo{planeType: "dataplane", planeID: "dp1", crNamespace: "ns", crName: "dp"}

		owned := svc.fetchOwnedResources(context.Background(), pi, "apps", "ReplicaSet", "ns1", "deploy-uid")
		require.Len(t, owned, 1)
		assert.Equal(t, "apps/v1", owned[0]["apiVersion"])
	})

	t.Run("gateway error returns nil gracefully", func(t *testing.T) {
		gc := testGatewayServer(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		})

		fc := newFakeClient()
		svc := &k8sResourcesService{k8sClient: fc, gatewayClient: gc, logger: testLogger()}
		pi := planeInfo{planeType: "dataplane", planeID: "dp1", crNamespace: "ns", crName: "dp"}

		owned := svc.fetchOwnedResources(context.Background(), pi, "", "Pod", "ns1", "owner-uid")
		assert.Nil(t, owned)
	})
}

// --- resolveResourcePlural ---

func TestResolveResourcePlural(t *testing.T) {
	t.Run("resolves known types via REST mapper", func(t *testing.T) {
		fc := newFakeClient()
		svc := &k8sResourcesService{k8sClient: fc, logger: testLogger()}

		// The fake client's REST mapper knows about standard K8s types
		plural, err := svc.resolveResourcePlural("apps", "v1", "Deployment")
		require.NoError(t, err)
		assert.Equal(t, "deployments", plural)
	})

	t.Run("unknown type returns error", func(t *testing.T) {
		fc := newFakeClient()
		svc := &k8sResourcesService{k8sClient: fc, logger: testLogger()}

		_, err := svc.resolveResourcePlural("nonexistent.group", "v1", "Unknown")
		require.Error(t, err)
	})
}

// --- buildResourceTreeNodes ---

func TestBuildResourceTreeNodes(t *testing.T) {
	t.Run("builds nodes from release resources", func(t *testing.T) {
		svcObj := k8sObject("v1", "Service", "dp-ns", "web-svc", "svc-uid-1")

		gc := testGatewayServer(t, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(jsonMarshal(t, svcObj))
		})

		release := &openchoreov1alpha1.RenderedRelease{
			Status: openchoreov1alpha1.RenderedReleaseStatus{
				Resources: []openchoreov1alpha1.ResourceStatus{
					{ID: "svc", Group: "", Version: "v1", Kind: "Service", Name: "web-svc", Namespace: "dp-ns",
						HealthStatus: openchoreov1alpha1.HealthStatusHealthy},
				},
			},
		}

		fc := newFakeClient()
		svc := &k8sResourcesService{k8sClient: fc, gatewayClient: gc, logger: testLogger()}
		pi := planeInfo{planeType: "dataplane", planeID: "dp1", crNamespace: "ns", crName: "dp"}
		rc := &releaseContext{release: release, plane: pi, namespace: "dp-ns"}

		nodes := svc.buildResourceTreeNodes(context.Background(), rc)
		require.Len(t, nodes, 1)
		assert.Equal(t, "Service", nodes[0].Kind)
		assert.Equal(t, "web-svc", nodes[0].Name)
		require.NotNil(t, nodes[0].Health)
		assert.Equal(t, "Healthy", nodes[0].Health.Status)
	})

	t.Run("skips resources that fail to fetch", func(t *testing.T) {
		gc := testGatewayServer(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		})

		release := &openchoreov1alpha1.RenderedRelease{
			Status: openchoreov1alpha1.RenderedReleaseStatus{
				Resources: []openchoreov1alpha1.ResourceStatus{
					{ID: "svc", Group: "", Version: "v1", Kind: "Service", Name: "missing-svc", Namespace: "dp-ns"},
				},
			},
		}

		fc := newFakeClient()
		svc := &k8sResourcesService{k8sClient: fc, gatewayClient: gc, logger: testLogger()}
		pi := planeInfo{planeType: "dataplane", planeID: "dp1", crNamespace: "ns", crName: "dp"}
		rc := &releaseContext{release: release, plane: pi, namespace: "dp-ns"}

		nodes := svc.buildResourceTreeNodes(context.Background(), rc)
		assert.Empty(t, nodes)
	})
}

// --- NewServiceWithAuthz ---

func TestNewServiceWithAuthz(t *testing.T) {
	fc := newFakeClient()
	gc := gateway.NewClient("http://localhost")
	pdp := authzmocks.NewMockPDP(t)
	svc := NewServiceWithAuthz(fc, gc, pdp, testLogger())
	require.NotNil(t, svc)
}
