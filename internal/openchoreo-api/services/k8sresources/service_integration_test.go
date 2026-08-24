// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package k8sresources

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	authzmocks "github.com/openchoreo/openchoreo/internal/authz/core/mocks"
	"github.com/openchoreo/openchoreo/internal/clients/gateway"
	"github.com/openchoreo/openchoreo/internal/controller"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/config"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/models"
	"github.com/openchoreo/openchoreo/internal/resourcetree/protocol"
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

// testTreeConfig returns the built-in traversal rules.
func testTreeConfig() config.ResourceTreeConfig {
	return config.ResourceTreeDefaults()
}

// newTestService builds a service on the built-in traversal rules.
func newTestService(k8sClient client.Client, gatewayClient *gateway.Client) Service {
	return NewService(k8sClient, gatewayClient, testTreeConfig(), testLogger())
}

// testRules compiles the built-in traversal rules, for tests that build the
// service struct directly instead of going through the constructor.
func testRules() *compiledRules {
	return compileRules(testTreeConfig())
}

// testGatewayServer creates a non-TLS HTTP test server and a gateway Client pointing to it.
// The handler receives all proxied requests.
func testGatewayServer(t *testing.T, handler http.HandlerFunc) *gateway.Client {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	c, err := gateway.NewClientWithConfig(&gateway.Config{BaseURL: server.URL})
	require.NoError(t, err)
	return c
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
func testRenderedRelease(rb *openchoreov1alpha1.ReleaseBinding, targetPlane string, resources []openchoreov1alpha1.RenderedManifestStatus) *openchoreov1alpha1.RenderedRelease {
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

// namedDataPlaneRelease is testRenderedRelease with a caller-chosen name, for
// tests that need two data-plane releases owned by one binding.
func namedDataPlaneRelease(rb *openchoreov1alpha1.ReleaseBinding, name string,
	resources []openchoreov1alpha1.RenderedManifestStatus) *openchoreov1alpha1.RenderedRelease {
	rr := testRenderedRelease(rb, planeTypeDataPlane, resources)
	rr.Name = name
	return rr
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
	gc, err := gateway.NewClientWithConfig(&gateway.Config{BaseURL: "http://localhost"})
	require.NoError(t, err)
	svc := newTestService(fc, gc)
	require.NotNil(t, svc)
}

// --- resolveReleaseContexts ---

func TestResolveReleaseContexts(t *testing.T) {
	t.Run("release binding not found", func(t *testing.T) {
		fc := newFakeClient()
		svc := &k8sResourcesService{k8sClient: fc, rules: testRules(), logger: testLogger()}

		_, _, err := svc.resolveReleaseContexts(context.Background(), testNamespace, "nonexistent")
		require.ErrorIs(t, err, ErrReleaseBindingNotFound)
	})

	t.Run("no owned releases returns nil", func(t *testing.T) {
		rb := testReleaseBinding()
		env := testEnvironment()
		dp := testDataPlane("default")
		fc := newFakeClient(rb, env, dp)
		svc := &k8sResourcesService{k8sClient: fc, rules: testRules(), logger: testLogger()}

		contexts, _, err := svc.resolveReleaseContexts(context.Background(), testNamespace, "rb-1")
		require.NoError(t, err)
		require.Nil(t, contexts)
	})

	t.Run("environment not found", func(t *testing.T) {
		rb := testReleaseBinding()
		rr := testRenderedRelease(rb, planeTypeDataPlane, []openchoreov1alpha1.RenderedManifestStatus{
			{ID: "dep", Group: "apps", Version: "v1", Kind: "Deployment", Name: "web", Namespace: "dp-ns"},
		})
		fc := newFakeClient(rb, rr)
		svc := &k8sResourcesService{k8sClient: fc, rules: testRules(), logger: testLogger()}

		_, _, err := svc.resolveReleaseContexts(context.Background(), testNamespace, "rb-1")
		require.ErrorIs(t, err, ErrEnvironmentNotFound)
	})

	t.Run("success with dataplane release", func(t *testing.T) {
		rb := testReleaseBinding()
		env := testEnvironment()
		dp := testDataPlane("default")
		rr := testRenderedRelease(rb, planeTypeDataPlane, []openchoreov1alpha1.RenderedManifestStatus{
			{ID: "dep", Group: "apps", Version: "v1", Kind: "Deployment", Name: "web", Namespace: "dp-ns"},
		})
		fc := newFakeClient(rb, env, dp, rr)
		svc := &k8sResourcesService{k8sClient: fc, rules: testRules(), logger: testLogger()}

		contexts, _, err := svc.resolveReleaseContexts(context.Background(), testNamespace, "rb-1")
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
		svc := &k8sResourcesService{k8sClient: newFakeClient(), rules: testRules(), logger: testLogger()}

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
		svc := &k8sResourcesService{k8sClient: fc, rules: testRules(), logger: testLogger()}

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
		svc := newTestService(fc, nil)

		_, err := svc.GetResourceTree(context.Background(), testNamespace, "rb-1")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "gateway client is not configured")
	})

	t.Run("release binding not found", func(t *testing.T) {
		gc := testGatewayServer(t, func(w http.ResponseWriter, r *http.Request) {})
		fc := newFakeClient()
		svc := newTestService(fc, gc)

		_, err := svc.GetResourceTree(context.Background(), testNamespace, "nonexistent")
		require.ErrorIs(t, err, ErrReleaseBindingNotFound)
	})

	t.Run("empty tree when no owned releases", func(t *testing.T) {
		rb := testReleaseBinding()
		env := testEnvironment()
		dp := testDataPlane("default")
		gc := testGatewayServer(t, func(w http.ResponseWriter, r *http.Request) {})
		fc := newFakeClient(rb, env, dp)
		svc := newTestService(fc, gc)

		result, err := svc.GetResourceTree(context.Background(), testNamespace, "rb-1")
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Empty(t, result.RenderedReleases)
	})

	t.Run("success returns resource tree with nodes", func(t *testing.T) {
		rb := testReleaseBinding()
		env := testEnvironment()
		dp := testDataPlane("default")
		rr := testRenderedRelease(rb, planeTypeDataPlane, []openchoreov1alpha1.RenderedManifestStatus{
			{ID: "svc", Group: "", Version: "v1", Kind: "Service", Name: "web-svc", Namespace: "dp-ns"},
		})
		fc := newFakeClient(rb, env, dp, rr)

		svcObj := k8sObject("v1", "Service", "dp-ns", "web-svc", "svc-uid-1")

		gc := testGatewayServer(t, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(jsonMarshal(t, svcObj))
		})

		svc := newTestService(fc, gc)
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
		rr := testRenderedRelease(rb, planeTypeDataPlane, []openchoreov1alpha1.RenderedManifestStatus{
			{ID: "svc", Group: "", Version: "v1", Kind: "Service", Name: "web-svc", Namespace: "dp-ns"},
		})
		fc := newFakeClient(rb, env, dp, rr)

		gc := testGatewayServer(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		})

		svc := newTestService(fc, gc)
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
		svc := newTestService(fc, nil)

		_, err := svc.GetResourceEvents(context.Background(), testNamespace, "rb-1", "apps", "v1", "Deployment", "web")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "gateway client is not configured")
	})

	t.Run("incomplete walk is not reported as not-found", func(t *testing.T) {
		// The gateway fails every request, so the root cannot be fetched and the
		// walk cannot prove or disprove membership. That must surface as an
		// incomplete-tree error, never as a 404 that reads as "the resource does
		// not exist" (finding I2).
		rb := testReleaseBinding()
		env := testEnvironment()
		dp := testDataPlane("default")
		rr := testRenderedRelease(rb, planeTypeDataPlane, []openchoreov1alpha1.RenderedManifestStatus{
			{ID: "dep", Group: "apps", Version: "v1", Kind: "Deployment", Name: "web", Namespace: "dp-ns"},
		})
		fc := newFakeClient(rb, env, dp, rr)
		gc := testGatewayServer(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		})
		svc := newTestService(fc, gc)

		_, err := svc.GetResourceEvents(context.Background(), testNamespace, "rb-1", "", "v1", "ConfigMap", "missing")
		require.ErrorIs(t, err, ErrResourceTreeIncomplete)
		require.NotErrorIs(t, err, ErrResourceNotFound)
	})

	t.Run("success returns events", func(t *testing.T) {
		rb := testReleaseBinding()
		env := testEnvironment()
		dp := testDataPlane("default")
		rr := testRenderedRelease(rb, planeTypeDataPlane, []openchoreov1alpha1.RenderedManifestStatus{
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

		svc := newTestService(fc, gc)
		result, err := svc.GetResourceEvents(context.Background(), testNamespace, "rb-1", "apps", "v1", "Deployment", "web")
		require.NoError(t, err)
		require.Len(t, result.Events, 1)
		assert.Equal(t, "Normal", result.Events[0].Type)
		assert.Equal(t, "ScalingReplicaSet", result.Events[0].Reason)
		assert.Equal(t, "deployment-controller", result.Events[0].Source)
	})

	t.Run("cross-namespace discovered child uses its real namespace", func(t *testing.T) {
		rb := testReleaseBinding()
		env := testEnvironment()
		dp := testDataPlane("default")
		rr := testRenderedRelease(rb, planeTypeDataPlane, []openchoreov1alpha1.RenderedManifestStatus{
			{ID: "dep", Group: "apps", Version: "v1", Kind: "Deployment", Name: "web", Namespace: "dp-ns"},
		})
		fc := newFakeClient(rb, env, dp, rr)
		cfg := config.ResourceTreeConfig{Rules: []config.ResourceTreeRule{{
			Root: config.KindRef{Group: "apps", Version: "v1", Kind: "Deployment", Resource: "deployments"},
			Children: []config.ChildRule{{
				Kind:    config.KindRef{Version: "v1", Kind: "Pod", Resource: "pods"},
				Matcher: config.MatcherLabelSelector,
				LabelSelector: &config.LabelSelectorSpec{
					MatchLabels: map[string]string{"owner": config.TokenParentName}, Namespaces: []string{"other-ns"},
				},
			}},
		}}}
		cluster := &fakeCluster{objects: []map[string]any{
			liveObject("v1", "Pod", "other-ns", "cross-pod", "pod-uid", withLabels(map[string]string{"owner": "web"})),
		}}
		var eventsPath, podPath, logsPath string
		gc := treeGateway(t, &gatewayRecorder{}, cluster.match(t), func(r *http.Request) stubResponse {
			switch {
			case strings.Contains(r.URL.Path, "/deployments/web"):
				return stubResponse{status: http.StatusOK, body: liveObject("apps/v1", "Deployment", "dp-ns", "web", "dep-uid")}
			case strings.HasSuffix(r.URL.Path, "/events"):
				eventsPath = r.URL.Path
				return stubResponse{status: http.StatusOK, body: k8sList()}
			case strings.HasSuffix(r.URL.Path, "/pods/cross-pod"):
				podPath = r.URL.Path
				return stubResponse{status: http.StatusOK, body: podWithContainers("main")}
			case strings.HasSuffix(r.URL.Path, "/pods/cross-pod/log"):
				logsPath = r.URL.Path
				return stubResponse{status: http.StatusOK, raw: "2024-01-15T10:00:00Z hello\n"}
			default:
				return stubResponse{status: http.StatusNotFound}
			}
		})
		svc := &k8sResourcesService{k8sClient: fc, gatewayClient: gc, rules: compileRules(cfg), logger: testLogger()}

		_, err := svc.GetResourceEvents(context.Background(), testNamespace, "rb-1", "", "v1", "Pod", "cross-pod")
		require.NoError(t, err)
		assert.Contains(t, eventsPath, "/namespaces/other-ns/events")
		_, err = svc.GetResourceLogs(context.Background(), testNamespace, "rb-1", "cross-pod", "", nil)
		require.NoError(t, err)
		assert.Contains(t, podPath, "/namespaces/other-ns/pods/cross-pod")
		assert.Contains(t, logsPath, "/namespaces/other-ns/pods/cross-pod/log")
		_, err = svc.GetResourceEvents(context.Background(), testNamespace, "rb-1", "", "v1", "Pod", "unrelated")
		require.ErrorIs(t, err, ErrResourceNotFound)
		_, err = svc.GetResourceLogs(context.Background(), testNamespace, "rb-1", "unrelated", "", nil)
		require.ErrorIs(t, err, ErrResourceNotFound)
	})

	t.Run("identity matching two live members is ambiguous, not an arbitrary pick", func(t *testing.T) {
		rb := testReleaseBinding()
		env := testEnvironment()
		dp := testDataPlane("default")
		rrA := namedDataPlaneRelease(rb, "rr-a", []openchoreov1alpha1.RenderedManifestStatus{
			{ID: "dep", Group: "apps", Version: "v1", Kind: "Deployment", Name: "web-a", Namespace: "ns-a"},
		})
		rrB := namedDataPlaneRelease(rb, "rr-b", []openchoreov1alpha1.RenderedManifestStatus{
			{ID: "dep", Group: "apps", Version: "v1", Kind: "Deployment", Name: "web-b", Namespace: "ns-b"},
		})
		fc := newFakeClient(rb, env, dp, rrA, rrB)
		cfg := config.ResourceTreeConfig{Rules: []config.ResourceTreeRule{{
			Root: config.KindRef{Group: "apps", Version: "v1", Kind: "Deployment", Resource: "deployments"},
			Children: []config.ChildRule{{
				Kind:    config.KindRef{Version: "v1", Kind: "Pod", Resource: "pods"},
				Matcher: config.MatcherLabelSelector,
				LabelSelector: &config.LabelSelectorSpec{
					MatchLabels: map[string]string{"owner": config.TokenParentName},
				},
			}},
		}}}
		// Both releases resolve a Pod named "dup" (in different namespaces); the
		// logs/events identity is group/version/kind/name only, so the two collide.
		cluster := &fakeCluster{objects: []map[string]any{
			liveObject("v1", "Pod", "ns-a", "dup", "dup-a", withLabels(map[string]string{"owner": "web-a"})),
			liveObject("v1", "Pod", "ns-b", "dup", "dup-b", withLabels(map[string]string{"owner": "web-b"})),
		}}
		gc := treeGateway(t, &gatewayRecorder{}, cluster.match(t), func(r *http.Request) stubResponse {
			switch {
			case strings.Contains(r.URL.Path, "/deployments/web-a"):
				return stubResponse{status: http.StatusOK, body: liveObject("apps/v1", "Deployment", "ns-a", "web-a", "dep-a")}
			case strings.Contains(r.URL.Path, "/deployments/web-b"):
				return stubResponse{status: http.StatusOK, body: liveObject("apps/v1", "Deployment", "ns-b", "web-b", "dep-b")}
			default:
				return stubResponse{status: http.StatusNotFound}
			}
		})
		svc := &k8sResourcesService{k8sClient: fc, gatewayClient: gc, rules: compileRules(cfg), logger: testLogger()}

		_, err := svc.GetResourceEvents(context.Background(), testNamespace, "rb-1", "", "v1", "Pod", "dup")
		require.ErrorIs(t, err, ErrResourceMatchAmbiguous)
		require.NotErrorIs(t, err, ErrResourceNotFound)
	})

	t.Run("two releases declaring the same rendered root are ambiguous", func(t *testing.T) {
		rb := testReleaseBinding()
		env := testEnvironment()
		dp := testDataPlane("default")
		rrA := namedDataPlaneRelease(rb, "rr-a", []openchoreov1alpha1.RenderedManifestStatus{
			{ID: "dep", Group: "apps", Version: "v1", Kind: "Deployment", Name: "web", Namespace: "ns-a"},
		})
		rrB := namedDataPlaneRelease(rb, "rr-b", []openchoreov1alpha1.RenderedManifestStatus{
			{ID: "dep", Group: "apps", Version: "v1", Kind: "Deployment", Name: "web", Namespace: "ns-b"},
		})
		fc := newFakeClient(rb, env, dp, rrA, rrB)
		gc := testGatewayServer(t, func(w http.ResponseWriter, r *http.Request) {})
		svc := newTestService(fc, gc)

		// Both releases declare a rendered root "web": the exact-root path must
		// report ambiguity rather than taking the first.
		_, err := svc.GetResourceEvents(context.Background(), testNamespace, "rb-1", "apps", "v1", "Deployment", "web")
		require.ErrorIs(t, err, ErrResourceMatchAmbiguous)
	})

	t.Run("two same-named members within one release are ambiguous", func(t *testing.T) {
		rb := testReleaseBinding()
		env := testEnvironment()
		dp := testDataPlane("default")
		rr := testRenderedRelease(rb, planeTypeDataPlane, []openchoreov1alpha1.RenderedManifestStatus{
			{ID: "dep", Group: "apps", Version: "v1", Kind: "Deployment", Name: "web", Namespace: "ns-a"},
		})
		fc := newFakeClient(rb, env, dp, rr)
		// One cross-namespace selector edge, so a single release's walk can surface
		// two Pods named "dup" in different namespaces — distinct UIDs, one name.
		cfg := config.ResourceTreeConfig{Rules: []config.ResourceTreeRule{{
			Root: config.KindRef{Group: "apps", Version: "v1", Kind: "Deployment", Resource: "deployments"},
			Children: []config.ChildRule{{
				Kind:    config.KindRef{Version: "v1", Kind: "Pod", Resource: "pods"},
				Matcher: config.MatcherLabelSelector,
				LabelSelector: &config.LabelSelectorSpec{
					MatchLabels: map[string]string{"owner": config.TokenParentName},
					Namespaces:  []string{"ns-a", "ns-b"},
				},
			}},
		}}}
		cluster := &fakeCluster{objects: []map[string]any{
			liveObject("v1", "Pod", "ns-a", "dup", "dup-a", withLabels(map[string]string{"owner": "web"})),
			liveObject("v1", "Pod", "ns-b", "dup", "dup-b", withLabels(map[string]string{"owner": "web"})),
		}}
		gc := treeGateway(t, &gatewayRecorder{}, cluster.match(t), func(r *http.Request) stubResponse {
			if strings.Contains(r.URL.Path, "/deployments/web") {
				return stubResponse{status: http.StatusOK, body: liveObject("apps/v1", "Deployment", "ns-a", "web", "dep-a")}
			}
			return stubResponse{status: http.StatusNotFound}
		})
		svc := &k8sResourcesService{k8sClient: fc, gatewayClient: gc, rules: compileRules(cfg), logger: testLogger()}

		_, err := svc.GetResourceEvents(context.Background(), testNamespace, "rb-1", "", "v1", "Pod", "dup")
		require.ErrorIs(t, err, ErrResourceMatchAmbiguous)
	})

	t.Run("events for cluster-scoped resource", func(t *testing.T) {
		rb := testReleaseBinding()
		env := testEnvironment()
		dp := testDataPlane("default")
		rr := testRenderedRelease(rb, planeTypeDataPlane, []openchoreov1alpha1.RenderedManifestStatus{
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

		svc := newTestService(fc, gc)
		result, err := svc.GetResourceEvents(context.Background(), testNamespace, "rb-1", "", "v1", "Namespace", "my-ns")
		require.NoError(t, err)
		assert.Empty(t, result.Events)
		// For cluster-scoped resource (empty namespace), the events path should be cluster-level
		assert.Contains(t, capturedPath, "api/v1/events")
	})
}

// --- GetResourceLogs ---

func TestGetResourceLogs(t *testing.T) {
	dataPlaneRelease := func() []client.Object {
		rb := testReleaseBinding()
		env := testEnvironment()
		dp := testDataPlane("default")
		rr := testRenderedRelease(rb, planeTypeDataPlane, []openchoreov1alpha1.RenderedManifestStatus{
			{ID: "dep", Group: "apps", Version: "v1", Kind: "Deployment", Name: "web", Namespace: "dp-ns"},
			{ID: "pod", Group: "", Version: "v1", Kind: "Pod", Name: "pod-1", Namespace: "dp-ns"},
		})
		return []client.Object{rb, env, dp, rr}
	}

	t.Run("nil gateway client returns error", func(t *testing.T) {
		fc := newFakeClient()
		svc := newTestService(fc, nil)

		_, err := svc.GetResourceLogs(context.Background(), testNamespace, "rb-1", "pod-1", "", nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "gateway client is not configured")
	})

	t.Run("no dataplane release with parent resources returns not found", func(t *testing.T) {
		rb := testReleaseBinding()
		env := testEnvironment()
		dp := testDataPlane("default")
		// Only an observability plane release — no parent for pods
		rr := testRenderedRelease(rb, planeTypeObservabilityPlane, []openchoreov1alpha1.RenderedManifestStatus{
			{ID: "cm", Group: "", Version: "v1", Kind: "ConfigMap", Name: "obs-cfg", Namespace: "obs-ns"},
		})
		fc := newFakeClient(rb, env, dp, rr)
		gc := testGatewayServer(t, func(w http.ResponseWriter, r *http.Request) {})
		svc := newTestService(fc, gc)

		_, err := svc.GetResourceLogs(context.Background(), testNamespace, "rb-1", "pod-1", "", nil)
		require.ErrorIs(t, err, ErrResourceNotFound)
	})

	t.Run("single container returns parsed logs tagged with container", func(t *testing.T) {
		objs := dataPlaneRelease()
		fc := newFakeClient(objs...)

		gc := testGatewayServer(t, podLogsHandler(t, []string{"main"}, map[string]string{
			"main": "2024-01-15T10:00:00Z Starting server\n2024-01-15T10:00:01Z Ready\n",
		}))

		svc := newTestService(fc, gc)
		result, err := svc.GetResourceLogs(context.Background(), testNamespace, "rb-1", "pod-1", "", nil)
		require.NoError(t, err)
		require.Len(t, result.LogEntries, 2)
		assert.Equal(t, "Starting server", result.LogEntries[0].Log)
		assert.Equal(t, "main", result.LogEntries[0].Container)
		assert.Equal(t, "Ready", result.LogEntries[1].Log)
		assert.Equal(t, "main", result.LogEntries[1].Container)
	})

	t.Run("multiple containers aggregate ordered by timestamp and tagged", func(t *testing.T) {
		objs := dataPlaneRelease()
		fc := newFakeClient(objs...)

		gc := testGatewayServer(t, podLogsHandler(t, []string{"main", "daprd"}, map[string]string{
			"main":  "2024-01-15T10:00:00Z main-a\n2024-01-15T10:00:02Z main-b\n",
			"daprd": "2024-01-15T10:00:01Z daprd-a\n2024-01-15T10:00:03Z daprd-b\n",
		}))

		svc := newTestService(fc, gc)
		result, err := svc.GetResourceLogs(context.Background(), testNamespace, "rb-1", "pod-1", "", nil)
		require.NoError(t, err)
		require.Len(t, result.LogEntries, 4)

		logs := make([]string, 0, len(result.LogEntries))
		containers := make([]string, 0, len(result.LogEntries))
		for _, e := range result.LogEntries {
			logs = append(logs, e.Log)
			containers = append(containers, e.Container)
		}
		assert.Equal(t, []string{"main-a", "daprd-a", "main-b", "daprd-b"}, logs)
		assert.Equal(t, []string{"main", "daprd", "main", "daprd"}, containers)
	})

	t.Run("explicit container fetches only that container without listing the pod", func(t *testing.T) {
		objs := dataPlaneRelease()
		fc := newFakeClient(objs...)

		var capturedContainer string
		gc := testGatewayServer(t, func(w http.ResponseWriter, r *http.Request) {
			require.True(t, strings.HasSuffix(r.URL.Path, "/log"),
				"expected only a pod-log request, got %s", r.URL.Path)
			capturedContainer = r.URL.Query().Get("container")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("2024-01-15T10:00:00Z only-daprd\n"))
		})

		svc := newTestService(fc, gc)
		result, err := svc.GetResourceLogs(context.Background(), testNamespace, "rb-1", "pod-1", "daprd", nil)
		require.NoError(t, err)
		require.Len(t, result.LogEntries, 1)
		assert.Equal(t, "daprd", capturedContainer)
		assert.Equal(t, "daprd", result.LogEntries[0].Container)
	})

	t.Run("explicit invalid container returns ErrInvalidContainer", func(t *testing.T) {
		objs := dataPlaneRelease()
		fc := newFakeClient(objs...)

		gc := testGatewayServer(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
		})

		svc := newTestService(fc, gc)
		_, err := svc.GetResourceLogs(context.Background(), testNamespace, "rb-1", "pod-1", "nope", nil)
		require.ErrorIs(t, err, ErrInvalidContainer)
	})

	t.Run("pod enumeration failure returns not found", func(t *testing.T) {
		objs := dataPlaneRelease()
		fc := newFakeClient(objs...)

		gc := testGatewayServer(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		})

		svc := newTestService(fc, gc)
		_, err := svc.GetResourceLogs(context.Background(), testNamespace, "rb-1", "pod-1", "", nil)
		require.ErrorIs(t, err, ErrResourceNotFound)
	})

	t.Run("with sinceSeconds forwards the query param", func(t *testing.T) {
		objs := dataPlaneRelease()
		fc := newFakeClient(objs...)

		var capturedLogsURL string
		gc := testGatewayServer(t, func(w http.ResponseWriter, r *http.Request) {
			if strings.HasSuffix(r.URL.Path, "/log") {
				capturedLogsURL = r.URL.String()
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("2024-01-15T10:00:00Z recent log\n"))
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(jsonMarshal(t, podWithContainers("main")))
		})

		svc := newTestService(fc, gc)
		since := int64(300)
		result, err := svc.GetResourceLogs(context.Background(), testNamespace, "rb-1", "pod-1", "", &since)
		require.NoError(t, err)
		require.Len(t, result.LogEntries, 1)
		assert.Contains(t, capturedLogsURL, "sinceSeconds=300")
	})

	t.Run("explicit container on a missing pod returns not found, not invalid container", func(t *testing.T) {
		objs := dataPlaneRelease()
		fc := newFakeClient(objs...)

		gc := testGatewayServer(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		})

		svc := newTestService(fc, gc)
		_, err := svc.GetResourceLogs(context.Background(), testNamespace, "rb-1", "pod-1", "main", nil)
		require.ErrorIs(t, err, ErrResourceNotFound)
		require.NotErrorIs(t, err, ErrInvalidContainer)
	})

	t.Run("explicit container forbidden surfaces the error instead of 404/400", func(t *testing.T) {
		objs := dataPlaneRelease()
		fc := newFakeClient(objs...)

		gc := testGatewayServer(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusForbidden)
		})

		svc := newTestService(fc, gc)
		_, err := svc.GetResourceLogs(context.Background(), testNamespace, "rb-1", "pod-1", "main", nil)
		require.Error(t, err)
		require.NotErrorIs(t, err, ErrResourceNotFound)
		require.NotErrorIs(t, err, ErrInvalidContainer)
	})

	t.Run("transient pod-resolution failure is not reported as not found", func(t *testing.T) {
		objs := dataPlaneRelease()
		fc := newFakeClient(objs...)

		gc := testGatewayServer(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		})

		svc := newTestService(fc, gc)
		_, err := svc.GetResourceLogs(context.Background(), testNamespace, "rb-1", "pod-1", "", nil)
		require.Error(t, err)
		require.NotErrorIs(t, err, ErrResourceNotFound)
	})

	t.Run("aggregate skips a still-starting container and returns the rest", func(t *testing.T) {
		objs := dataPlaneRelease()
		fc := newFakeClient(objs...)

		gc := testGatewayServer(t, func(w http.ResponseWriter, r *http.Request) {
			if strings.HasSuffix(r.URL.Path, "/log") {
				if r.URL.Query().Get("container") == "sidecar" {
					w.WriteHeader(http.StatusOK)
					_, _ = w.Write([]byte("2024-01-15T10:00:00Z sidecar up\n"))
					return
				}
				// "main" is still starting — Kubernetes returns 400.
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(jsonMarshal(t, podWithContainers("main", "sidecar")))
		})

		svc := newTestService(fc, gc)
		result, err := svc.GetResourceLogs(context.Background(), testNamespace, "rb-1", "pod-1", "", nil)
		require.NoError(t, err)
		require.Len(t, result.LogEntries, 1)
		assert.Equal(t, "sidecar up", result.LogEntries[0].Log)
		assert.Equal(t, "sidecar", result.LogEntries[0].Container)
	})

	t.Run("aggregate surfaces a forbidden container instead of a partial result", func(t *testing.T) {
		objs := dataPlaneRelease()
		fc := newFakeClient(objs...)

		gc := testGatewayServer(t, func(w http.ResponseWriter, r *http.Request) {
			if strings.HasSuffix(r.URL.Path, "/log") {
				w.WriteHeader(http.StatusForbidden)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(jsonMarshal(t, podWithContainers("main", "sidecar")))
		})

		svc := newTestService(fc, gc)
		_, err := svc.GetResourceLogs(context.Background(), testNamespace, "rb-1", "pod-1", "", nil)
		require.Error(t, err)
		require.NotErrorIs(t, err, ErrResourceNotFound)
	})
}

// podWithContainers builds a minimal pod object (as decoded JSON) with the given container names.
func podWithContainers(names ...string) map[string]any {
	cs := make([]any, len(names))
	for i, n := range names {
		cs[i] = map[string]any{"name": n}
	}
	return map[string]any{
		"apiVersion": "v1",
		"kind":       "Pod",
		"metadata":   map[string]any{"name": "pod-1", "namespace": "dp-ns"},
		"spec":       map[string]any{"containers": cs},
	}
}

// podLogsHandler routes a proxied pod GET to a pod carrying the given containers, and each
// pod-log request to the log body for its ?container= value.
func podLogsHandler(t *testing.T, containers []string, logsByContainer map[string]string) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/log") {
			body, ok := logsByContainer[r.URL.Query().Get("container")]
			if !ok {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(body))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(jsonMarshal(t, podWithContainers(containers...)))
	}
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

		svc := &k8sResourcesService{gatewayClient: gc, rules: testRules(), logger: testLogger()}
		pi := planeInfo{planeType: "dataplane", planeID: "dp1", crNamespace: "ns", crName: "dp"}

		result, err := svc.fetchLiveResource(context.Background(), pi, "api/v1/namespaces/ns1/services/svc1")
		require.NoError(t, err)
		assert.Equal(t, "Service", result["kind"])
	})

	t.Run("non-200 returns error", func(t *testing.T) {
		gc := testGatewayServer(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		})

		svc := &k8sResourcesService{gatewayClient: gc, rules: testRules(), logger: testLogger()}
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

		svc := &k8sResourcesService{gatewayClient: gc, rules: testRules(), logger: testLogger()}
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

		svc := &k8sResourcesService{gatewayClient: gc, rules: testRules(), logger: testLogger()}
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

		svc := &k8sResourcesService{gatewayClient: gc, rules: testRules(), logger: testLogger()}
		pi := planeInfo{planeType: "dataplane", planeID: "dp1", crNamespace: "ns", crName: "dp"}

		result, err := svc.fetchK8sList(context.Background(), pi, "api/v1/namespaces/ns1/pods", "")
		require.NoError(t, err)
		assert.Nil(t, result)
	})

	t.Run("non-200 returns error", func(t *testing.T) {
		gc := testGatewayServer(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusForbidden)
		})

		svc := &k8sResourcesService{gatewayClient: gc, rules: testRules(), logger: testLogger()}
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

		svc := &k8sResourcesService{gatewayClient: gc, rules: testRules(), logger: testLogger()}
		pi := planeInfo{planeType: "dataplane", planeID: "dp1", crNamespace: "ns", crName: "dp"}

		_, err := svc.fetchK8sList(context.Background(), pi, "api/v1/events", "fieldSelector=involvedObject.kind=Deployment")
		require.NoError(t, err)
		assert.Contains(t, capturedQuery, "fieldSelector=involvedObject.kind=Deployment")
	})
}

// --- resource tree walk fixtures ---

const (
	treeNamespace = "dp-ns"
	// podChildKind is compared against often enough that the literal reads as a
	// magic string.
	podChildKind = "Pod"
)

// treePlaneInfo is the plane every walk test proxies through.
func treePlaneInfo() planeInfo {
	return planeInfo{planeType: "dataplane", planeID: "dp1", crNamespace: "ns", crName: "dp"}
}

// testKindByPlural maps the REST plurals the walk lists to the kinds the
// fixtures carry, so the legacy list stub can answer a proxied list path.
var testKindByPlural = map[string]string{
	"pods":        "Pod",
	"replicasets": "ReplicaSet",
	"jobs":        "Job",
	"secrets":     "Secret",
	"services":    "Service",
	"deployments": "Deployment",
	"configmaps":  "ConfigMap",
}

type objOption func(map[string]any)

// ownedBy adds owner references, the only thing the ownerRef matcher reads.
func ownedBy(uids ...string) objOption {
	return func(obj map[string]any) {
		refs := make([]any, 0, len(uids))
		for _, uid := range uids {
			refs = append(refs, map[string]any{"uid": uid})
		}
		obj["metadata"].(map[string]any)["ownerReferences"] = refs
	}
}

func withLabels(pairs map[string]string) objOption {
	return func(obj map[string]any) {
		labelMap := make(map[string]any, len(pairs))
		for key, value := range pairs {
			labelMap[key] = value
		}
		obj["metadata"].(map[string]any)["labels"] = labelMap
	}
}

func withAnnotations(pairs map[string]string) objOption {
	return func(obj map[string]any) {
		annotations := make(map[string]any, len(pairs))
		for key, value := range pairs {
			annotations[key] = value
		}
		obj["metadata"].(map[string]any)["annotations"] = annotations
	}
}

// withField sets a top-level field, used to prove whether an object reached the
// node whole or was projected down to its metadata.
func withField(key string, value any) objOption {
	return func(obj map[string]any) { obj[key] = value }
}

func nodeAnnotations(t *testing.T, node models.ResourceNode) map[string]any {
	t.Helper()
	metadata, ok := node.Object["metadata"].(map[string]any)
	require.True(t, ok, "node object has no metadata")
	annotations, ok := metadata["annotations"].(map[string]any)
	require.True(t, ok, "node metadata has no annotations map")
	return annotations
}

func liveObject(apiVersion, kind, namespace, name, uid string, opts ...objOption) map[string]any {
	obj := k8sObject(apiVersion, kind, namespace, name, uid)
	for _, opt := range opts {
		opt(obj)
	}
	return obj
}

// stubResponse is one gateway stub answer. body is marshaled as JSON; raw is
// written verbatim, for responses that are not valid JSON at all; dropConn
// closes the connection without answering, which is what the client sees when a
// request fails in transit.
type stubResponse struct {
	status   int
	body     any
	raw      string
	dropConn bool
}

func writeStub(t *testing.T, w http.ResponseWriter, resp stubResponse) {
	t.Helper()
	if resp.dropConn {
		conn, _, err := http.NewResponseController(w).Hijack()
		require.NoError(t, err)
		require.NoError(t, conn.Close())
		return
	}
	if resp.raw != "" {
		w.WriteHeader(resp.status)
		_, _ = w.Write([]byte(resp.raw))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.status)
	if resp.body != nil {
		_, _ = w.Write(jsonMarshal(t, resp.body))
	}
}

// gatewayRecorder captures what a walk sent: the resource-tree match batches and
// the legacy proxied list URLs. The two are recorded apart because the whole
// point of several tests is that one path was used and the other was not.
type gatewayRecorder struct {
	mu       sync.Mutex
	matches  []protocol.MatchRequest
	listURLs []*url.URL
}

func (g *gatewayRecorder) recordMatch(req protocol.MatchRequest) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.matches = append(g.matches, req)
}

func (g *gatewayRecorder) recordList(u *url.URL) {
	g.mu.Lock()
	defer g.mu.Unlock()
	copied := *u
	g.listURLs = append(g.listURLs, &copied)
}

func (g *gatewayRecorder) matchRequests() []protocol.MatchRequest {
	g.mu.Lock()
	defer g.mu.Unlock()
	return slices.Clone(g.matches)
}

func (g *gatewayRecorder) legacyURLs() []*url.URL {
	g.mu.Lock()
	defer g.mu.Unlock()
	return slices.Clone(g.listURLs)
}

// queryIDs flattens every query ID the walk sent, across all batches.
func (g *gatewayRecorder) queryIDs() []string {
	batches := g.matchRequests()
	ids := make([]string, 0, len(batches))
	for _, req := range batches {
		for _, q := range req.Queries {
			ids = append(ids, q.ID)
		}
	}
	return ids
}

// findQuery returns the query for a child kind across every batch sent.
func (g *gatewayRecorder) findQuery(t *testing.T, childKind string) protocol.MatchQuery {
	t.Helper()
	for _, req := range g.matchRequests() {
		for _, q := range req.Queries {
			if q.Child.Kind == childKind {
				return q
			}
		}
	}
	t.Fatalf("no match query was sent for child kind %q", childKind)
	return protocol.MatchQuery{}
}

// treeGateway builds a gateway client whose server splits resource-tree match
// posts from legacy proxied k8s requests and records both. A nil handler answers
// with an empty result per query / an empty list.
func treeGateway(t *testing.T, rec *gatewayRecorder,
	onMatch func(protocol.MatchRequest) stubResponse,
	onLegacy func(*http.Request) stubResponse,
) *gateway.Client {
	t.Helper()
	return testGatewayServer(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/resource-tree/") {
			body, err := io.ReadAll(r.Body)
			require.NoError(t, err)
			var req protocol.MatchRequest
			require.NoError(t, json.Unmarshal(body, &req))
			rec.recordMatch(req)

			resp := matchResponse(req, func(protocol.MatchQuery) protocol.MatchResult {
				return protocol.MatchResult{}
			})
			if onMatch != nil {
				resp = onMatch(req)
			}
			writeStub(t, w, resp)
			return
		}

		rec.recordList(r.URL)
		resp := stubResponse{status: http.StatusOK, body: k8sList()}
		if onLegacy != nil {
			resp = onLegacy(r)
		}
		writeStub(t, w, resp)
	})
}

// matchResponse answers every query in a batch, stamping each result with the
// query's own ID so the client's coverage check passes.
func matchResponse(req protocol.MatchRequest, answer func(protocol.MatchQuery) protocol.MatchResult) stubResponse {
	resp := protocol.MatchResponse{
		Version: protocol.Version,
		Results: make([]protocol.MatchResult, 0, len(req.Queries)),
	}
	for _, q := range req.Queries {
		result := answer(q)
		result.ID = q.ID
		resp.Results = append(resp.Results, result)
	}
	return stubResponse{status: http.StatusOK, body: resp}
}

// unsupportedTarget is the answer an agent that predates resource-tree support
// produces: its router does not know the target, so the gateway relays a 404.
func unsupportedTarget(protocol.MatchRequest) stubResponse {
	return stubResponse{status: http.StatusNotFound, raw: "unknown target: resource-tree"}
}

// fakeCluster is a set of live objects the gateway stub serves BOTH ways: as
// cluster-agent match results and as legacy proxied list responses. Driving both
// from one fixture is what makes "the fallback yields the same nodes" a real
// comparison instead of two hand-written expectations.
//
// It deliberately does NOT trim metadataOnly matches the way the real agent
// does, so a node that ends up projected proves the API projected it.
type fakeCluster struct {
	objects []map[string]any
	// kindByPlural extends testKindByPlural for fixtures whose kinds are not in
	// the built-in rule set, so the legacy list stub can answer for them too.
	kindByPlural map[string]string
}

func (c *fakeCluster) kindFor(plural string) string {
	if kind, ok := c.kindByPlural[plural]; ok {
		return kind
	}
	return testKindByPlural[plural]
}

func (c *fakeCluster) match(t *testing.T) func(protocol.MatchRequest) stubResponse {
	t.Helper()
	return func(req protocol.MatchRequest) stubResponse {
		return matchResponse(req, func(q protocol.MatchQuery) protocol.MatchResult {
			return protocol.MatchResult{Matches: c.answer(t, q)}
		})
	}
}

func (c *fakeCluster) answer(t *testing.T, q protocol.MatchQuery) []protocol.MatchedObject {
	t.Helper()
	if q.Matcher == protocol.MatcherOwnerRef {
		return c.answerOwnerRef(t, q)
	}
	return c.answerLabelSelector(t, q)
}

func (c *fakeCluster) answerOwnerRef(t *testing.T, q protocol.MatchQuery) []protocol.MatchedObject {
	t.Helper()
	var matches []protocol.MatchedObject
	for _, obj := range c.objects {
		if getStringField(obj, "kind") != q.Child.Kind {
			continue
		}
		namespace := getNestedString(obj, "metadata", "namespace")
		var parents []string
		for _, p := range q.Parents {
			if p.Namespace == namespace && hasOwnerReference(obj, p.UID) {
				parents = append(parents, p.UID)
			}
		}
		if len(parents) == 0 {
			continue
		}
		matches = append(matches, protocol.MatchedObject{ParentUIDs: parents, Object: jsonMarshal(t, obj)})
	}
	return matches
}

func (c *fakeCluster) answerLabelSelector(t *testing.T, q protocol.MatchQuery) []protocol.MatchedObject {
	t.Helper()
	var criteria protocol.LabelSelectorCriteria
	require.NoError(t, json.Unmarshal(q.Criteria, &criteria))

	var matches []protocol.MatchedObject
	indexByUID := map[string]int{}
	for _, p := range q.Parents {
		set := labels.Set{}
		for key, value := range criteria.MatchLabels {
			substituted, err := protocol.SubstituteParentTokens(value, p.Name, p.Namespace)
			require.NoError(t, err)
			set[key] = substituted
		}
		namespaces := criteria.Namespaces
		if len(namespaces) == 0 {
			namespaces = []string{p.Namespace}
		}
		for _, obj := range c.objects {
			if getStringField(obj, "kind") != q.Child.Kind {
				continue
			}
			if !slices.Contains(namespaces, getNestedString(obj, "metadata", "namespace")) {
				continue
			}
			if !labels.SelectorFromSet(set).Matches(objectLabels(obj)) {
				continue
			}
			uid := getNestedString(obj, "metadata", "uid")
			if idx, ok := indexByUID[uid]; ok {
				matches[idx].ParentUIDs = append(matches[idx].ParentUIDs, p.UID)
				continue
			}
			indexByUID[uid] = len(matches)
			matches = append(matches, protocol.MatchedObject{ParentUIDs: []string{p.UID}, Object: jsonMarshal(t, obj)})
		}
	}
	return matches
}

func (c *fakeCluster) list(t *testing.T) func(*http.Request) stubResponse {
	t.Helper()
	return func(r *http.Request) stubResponse {
		plural, namespace := parseListPath(t, r.URL)
		kind := c.kindFor(plural)

		selector := labels.Everything()
		if raw := r.URL.Query().Get("labelSelector"); raw != "" {
			parsed, err := labels.Parse(raw)
			require.NoError(t, err)
			selector = parsed
		}

		var items []map[string]any
		for _, obj := range c.objects {
			if getStringField(obj, "kind") != kind {
				continue
			}
			if getNestedString(obj, "metadata", "namespace") != namespace {
				continue
			}
			if !selector.Matches(objectLabels(obj)) {
				continue
			}
			items = append(items, obj)
		}
		return stubResponse{status: http.StatusOK, body: k8sList(items...)}
	}
}

func objectLabels(obj map[string]any) labels.Set {
	metadata, _ := obj["metadata"].(map[string]any)
	raw, _ := metadata["labels"].(map[string]any)
	set := make(labels.Set, len(raw))
	for key, value := range raw {
		if s, ok := value.(string); ok {
			set[key] = s
		}
	}
	return set
}

// parseListPath pulls the plural and namespace out of a proxied namespaced list
// path such as .../k8s/apis/apps/v1/namespaces/dp-ns/replicasets.
func parseListPath(t *testing.T, u *url.URL) (string, string) {
	t.Helper()
	segments := strings.Split(strings.Trim(u.Path, "/"), "/")
	require.GreaterOrEqual(t, len(segments), 3, "unexpected list path %s", u.Path)
	require.Equal(t, "namespaces", segments[len(segments)-3], "expected a namespaced list path, got %s", u.Path)
	return segments[len(segments)-1], segments[len(segments)-2]
}

// treeService builds a service on a specific rule set.
func treeService(gc *gateway.Client, cfg config.ResourceTreeConfig) *k8sResourcesService {
	return &k8sResourcesService{
		k8sClient:     newFakeClient(),
		gatewayClient: gc,
		rules:         compileRules(cfg),
		logger:        testLogger(),
	}
}

// runWalk seeds the root node the way buildResourceTreeNodes does and expands
// it, returning every node in the accumulator — the root first, then children.
func runWalk(t *testing.T, svc *k8sResourcesService, rootObj map[string]any,
	rs *openchoreov1alpha1.RenderedManifestStatus) []models.ResourceNode {
	t.Helper()
	return runWalkCtx(t, context.Background(), svc, rootObj, rs)
}

func runWalkCtx(t *testing.T, ctx context.Context, svc *k8sResourcesService, rootObj map[string]any,
	rs *openchoreov1alpha1.RenderedManifestStatus) []models.ResourceNode {
	t.Helper()
	acc := newNodeAccumulator(1)
	// The root's plural is resolved the way buildResourceTreeNodes resolves it,
	// so the GVR the sanitizer sees here is the one production would hand it.
	plural, err := svc.rootResourcePlural(rs)
	require.NoError(t, err, "root fixture must resolve a plural")
	rootGVR := schema.GroupVersionResource{Group: rs.Group, Version: rs.Version, Resource: plural}
	root, ok := buildResourceNode(rootObj, rootGVR, nil, "")
	require.True(t, ok, "root fixture must build a node")
	acc.add(root)
	svc.expandChildren(ctx, treePlaneInfo(), acc, root.UID, rootObj, rs)
	return acc.nodes
}

func findNode(t *testing.T, nodes []models.ResourceNode, kind, name string) models.ResourceNode {
	t.Helper()
	for _, node := range nodes {
		if node.Kind == kind && node.Name == name {
			return node
		}
	}
	t.Fatalf("no %s node named %q among %s", kind, name, nodeSummary(nodes))
	return models.ResourceNode{}
}

func nodeSummary(nodes []models.ResourceNode) string {
	parts := make([]string, 0, len(nodes))
	for _, node := range nodes {
		parts = append(parts, fmt.Sprintf("%s/%s", node.Kind, node.Name))
	}
	return "[" + strings.Join(parts, " ") + "]"
}

func deploymentRootStatus() *openchoreov1alpha1.RenderedManifestStatus {
	return &openchoreov1alpha1.RenderedManifestStatus{
		Group: "apps", Version: "v1", Kind: "Deployment", Name: "web", Namespace: treeNamespace,
	}
}

func deploymentRootObject() map[string]any {
	return liveObject("apps/v1", "Deployment", treeNamespace, "web", "deploy-uid")
}

// deploymentTree is the built-in Deployment chain's fixture: a hidden ReplicaSet
// carrying a Pod.
func deploymentTree() *fakeCluster {
	return &fakeCluster{objects: []map[string]any{
		liveObject("apps/v1", "ReplicaSet", treeNamespace, "web-abc", "rs-uid", ownedBy("deploy-uid")),
		liveObject("v1", "Pod", treeNamespace, "web-abc-xyz", "pod-uid", ownedBy("rs-uid")),
	}}
}

// --- test rule sets ---

// twoBranchConfig reaches Pod through two different branches of one root, so the
// same kind edge appears twice and level+kinds cannot identify a query.
func twoBranchConfig() config.ResourceTreeConfig {
	pod := config.ChildRule{Kind: config.KindRef{Version: "v1", Kind: "Pod", Resource: "pods"}}
	return config.ResourceTreeConfig{Rules: []config.ResourceTreeRule{{
		Root: config.KindRef{Group: "apps", Version: "v1", Kind: "Deployment", Resource: "deployments"},
		Children: []config.ChildRule{
			{
				Kind:     config.KindRef{Group: "apps", Version: "v1", Kind: "ReplicaSet", Resource: "replicasets"},
				Hide:     true,
				Children: []config.ChildRule{pod},
			},
			{
				Kind:     config.KindRef{Group: "batch", Version: "v1", Kind: "Job", Resource: "jobs"},
				Children: []config.ChildRule{pod},
			},
		},
	}}}
}

// hiddenBranchConfig puts a visible sibling next to the hidden ReplicaSet, so a
// failure below the hidden node has more than one emitted node it could wrongly
// land on.
func hiddenBranchConfig() config.ResourceTreeConfig {
	return config.ResourceTreeConfig{Rules: []config.ResourceTreeRule{{
		Root: config.KindRef{Group: "apps", Version: "v1", Kind: "Deployment", Resource: "deployments"},
		Children: []config.ChildRule{
			{
				Kind: config.KindRef{Group: "apps", Version: "v1", Kind: "ReplicaSet", Resource: "replicasets"},
				Hide: true,
				Children: []config.ChildRule{
					{Kind: config.KindRef{Version: "v1", Kind: "Pod", Resource: "pods"}},
				},
			},
			{Kind: config.KindRef{Version: "v1", Kind: "ConfigMap", Resource: "configmaps"}},
		},
	}}}
}

// secretConfig hangs a Secret off a Deployment, optionally opting out of the
// core-Secret metadata-only default.
func secretConfig(metadataOnly *bool) config.ResourceTreeConfig {
	return config.ResourceTreeConfig{Rules: []config.ResourceTreeRule{{
		Root: config.KindRef{Group: "apps", Version: "v1", Kind: "Deployment", Resource: "deployments"},
		Children: []config.ChildRule{{
			Kind:         config.KindRef{Version: "v1", Kind: "Secret", Resource: "secrets"},
			MetadataOnly: metadataOnly,
		}},
	}}}
}

// lowercaseSecretConfig declares the core Secret edge with a non-canonical kind
// spelling. Config validation accepts it, because it only checks that kind is
// non-empty, and the listing still works, because the list URL is built from the
// resource. It is the shape that would spell away any Secret defense keyed off
// the kind string. metadata_only is explicitly false so the projection is out of
// the way and only the data strip is under test.
func lowercaseSecretConfig() config.ResourceTreeConfig {
	return config.ResourceTreeConfig{Rules: []config.ResourceTreeRule{{
		Root: config.KindRef{Group: "apps", Version: "v1", Kind: "Deployment", Resource: "deployments"},
		Children: []config.ChildRule{{
			Kind:         config.KindRef{Version: "v1", Kind: "secret", Resource: "secrets"},
			MetadataOnly: boolPtr(false),
		}},
	}}}
}

// gatewayConfig is a dataplane-only root CRD — absent from the control plane's
// RESTMapper — with one ownerRef child and one labelSelector child.
func gatewayConfig() config.ResourceTreeConfig {
	return config.ResourceTreeConfig{Rules: []config.ResourceTreeRule{{
		Root: config.KindRef{Group: "gateway.networking.k8s.io", Version: "v1", Kind: "Gateway", Resource: "gateways"},
		Children: []config.ChildRule{
			{Kind: config.KindRef{Version: "v1", Kind: "Service", Resource: "services"}},
			{
				Kind:    config.KindRef{Group: "apps", Version: "v1", Kind: "Deployment", Resource: "deployments"},
				Matcher: config.MatcherLabelSelector,
				LabelSelector: &config.LabelSelectorSpec{
					MatchLabels: map[string]string{"owner": config.TokenParentName},
					Namespaces:  []string{"gw-system"},
				},
			},
		},
	}}}
}

func gatewayRootStatus() *openchoreov1alpha1.RenderedManifestStatus {
	return &openchoreov1alpha1.RenderedManifestStatus{
		Group: "gateway.networking.k8s.io", Version: "v1", Kind: "Gateway",
		Name: "gw-a", Namespace: treeNamespace,
	}
}

func gatewayRootObject() map[string]any {
	return liveObject("gateway.networking.k8s.io/v1", "Gateway", treeNamespace, "gw-a", "gw-uid")
}

// gatewayTree holds the children of gw-a: an owned Service in the gateway's own
// namespace, and a labeled Deployment in the rule's target namespace.
func gatewayTree() *fakeCluster {
	return &fakeCluster{objects: []map[string]any{
		liveObject("v1", "Service", treeNamespace, "gw-a-svc", "svc-uid", ownedBy("gw-uid")),
		liveObject("apps/v1", "Deployment", "gw-system", "gw-a-deploy", "gwdeploy-uid",
			withLabels(map[string]string{"owner": "gw-a"})),
	}}
}

// --- expandChildren ---

func TestExpandChildren(t *testing.T) {
	t.Run("Deployment yields one Pod parented to the Deployment through a hidden ReplicaSet", func(t *testing.T) {
		rec := &gatewayRecorder{}
		cluster := deploymentTree()
		svc := treeService(treeGateway(t, rec, cluster.match(t), nil), testTreeConfig())

		nodes := runWalk(t, svc, deploymentRootObject(), deploymentRootStatus())

		require.Len(t, nodes, 2, "expected the Deployment root and one Pod, got %s", nodeSummary(nodes))
		pod := nodes[1]
		assert.Equal(t, "Pod", pod.Kind)
		assert.Equal(t, "web-abc-xyz", pod.Name)
		require.Len(t, pod.ParentRefs, 1)
		assert.Equal(t, "Deployment", pod.ParentRefs[0].Kind)
		assert.Equal(t, "deploy-uid", pod.ParentRefs[0].UID)
		assert.Empty(t, pod.MatchedBy, "an ownerRef match is exact and carries no matchedBy badge")
	})

	t.Run("CronJob yields a Job and a Pod parented to the Job", func(t *testing.T) {
		rec := &gatewayRecorder{}
		cluster := &fakeCluster{objects: []map[string]any{
			liveObject("batch/v1", "Job", treeNamespace, "cj-job-1", "job-uid", ownedBy("cj-uid")),
			liveObject("v1", "Pod", treeNamespace, "cj-job-1-pod", "pod-uid", ownedBy("job-uid")),
		}}
		svc := treeService(treeGateway(t, rec, cluster.match(t), nil), testTreeConfig())

		rootObj := liveObject("batch/v1", "CronJob", treeNamespace, "my-cj", "cj-uid")
		rs := &openchoreov1alpha1.RenderedManifestStatus{
			Group: "batch", Version: "v1", Kind: "CronJob", Name: "my-cj", Namespace: treeNamespace,
		}
		nodes := runWalk(t, svc, rootObj, rs)

		require.Len(t, nodes, 3, "expected CronJob, Job and Pod, got %s", nodeSummary(nodes))
		job := findNode(t, nodes, "Job", "cj-job-1")
		require.Len(t, job.ParentRefs, 1)
		assert.Equal(t, "CronJob", job.ParentRefs[0].Kind)

		pod := findNode(t, nodes, "Pod", "cj-job-1-pod")
		require.Len(t, pod.ParentRefs, 1)
		assert.Equal(t, "Job", pod.ParentRefs[0].Kind)
		assert.Equal(t, "job-uid", pod.ParentRefs[0].UID)
	})

	t.Run("Job yields its owned Pods", func(t *testing.T) {
		rec := &gatewayRecorder{}
		cluster := &fakeCluster{objects: []map[string]any{
			liveObject("v1", "Pod", treeNamespace, "job-pod-1", "pod-uid", ownedBy("job-uid")),
		}}
		svc := treeService(treeGateway(t, rec, cluster.match(t), nil), testTreeConfig())

		rootObj := liveObject("batch/v1", "Job", treeNamespace, "my-job", "job-uid")
		rs := &openchoreov1alpha1.RenderedManifestStatus{
			Group: "batch", Version: "v1", Kind: "Job", Name: "my-job", Namespace: treeNamespace,
		}
		nodes := runWalk(t, svc, rootObj, rs)

		require.Len(t, nodes, 2, "expected the Job root and one Pod, got %s", nodeSummary(nodes))
		assert.Equal(t, "job-pod-1", nodes[1].Name)
		require.Len(t, nodes[1].ParentRefs, 1)
		assert.Equal(t, "job-uid", nodes[1].ParentRefs[0].UID)
	})

	t.Run("a kind with no rule makes no calls at all", func(t *testing.T) {
		rec := &gatewayRecorder{}
		gc := treeGateway(t, rec, func(protocol.MatchRequest) stubResponse {
			t.Error("no match request should be sent for a kind with no rule")
			return stubResponse{status: http.StatusOK}
		}, func(*http.Request) stubResponse {
			t.Error("no legacy list should be sent for a kind with no rule")
			return stubResponse{status: http.StatusOK}
		})
		svc := treeService(gc, testTreeConfig())

		rootObj := liveObject("v1", "Service", treeNamespace, "web-svc", "svc-uid")
		rs := &openchoreov1alpha1.RenderedManifestStatus{
			Group: "", Version: "v1", Kind: "Service", Name: "web-svc", Namespace: treeNamespace,
		}
		nodes := runWalk(t, svc, rootObj, rs)

		require.Len(t, nodes, 1, "only the root should be present")
		assert.Empty(t, nodes[0].ChildrenStatus, "no rule means no children and no status")
		assert.Empty(t, rec.matchRequests())
		assert.Empty(t, rec.legacyURLs())
	})

	t.Run("an old agent's 404 falls back to control-plane filtering and yields the same nodes", func(t *testing.T) {
		cluster := deploymentTree()

		agentRec := &gatewayRecorder{}
		agentSvc := treeService(treeGateway(t, agentRec, cluster.match(t), nil), testTreeConfig())
		viaAgent := runWalk(t, agentSvc, deploymentRootObject(), deploymentRootStatus())

		legacyRec := &gatewayRecorder{}
		legacySvc := treeService(treeGateway(t, legacyRec, unsupportedTarget, cluster.list(t)), testTreeConfig())
		viaLegacy := runWalk(t, legacySvc, deploymentRootObject(), deploymentRootStatus())

		require.Len(t, viaLegacy, 2, "expected the Deployment root and one Pod, got %s", nodeSummary(viaLegacy))
		assert.Equal(t, viaAgent, viaLegacy, "the fallback must produce the same tree as the agent path")
		assert.NotEmpty(t, legacyRec.legacyURLs(), "the fallback must have listed through the legacy proxy")
		assert.Empty(t, agentRec.legacyURLs(), "the agent path must not touch the legacy list endpoints")
	})

	t.Run("a forbidden query lands on the parent node and leaves the rest of the tree intact", func(t *testing.T) {
		rec := &gatewayRecorder{}
		cluster := &fakeCluster{objects: []map[string]any{
			liveObject("batch/v1", "Job", treeNamespace, "cj-job-1", "job-uid", ownedBy("cj-uid")),
		}}
		gc := treeGateway(t, rec, func(req protocol.MatchRequest) stubResponse {
			return matchResponse(req, func(q protocol.MatchQuery) protocol.MatchResult {
				if q.Child.Kind == podChildKind {
					return protocol.MatchResult{Error: &protocol.MatchError{
						Code: protocol.CodeForbidden, Message: "pods is forbidden",
					}}
				}
				return protocol.MatchResult{Matches: cluster.answer(t, q)}
			})
		}, nil)
		svc := treeService(gc, testTreeConfig())

		rootObj := liveObject("batch/v1", "CronJob", treeNamespace, "my-cj", "cj-uid")
		rs := &openchoreov1alpha1.RenderedManifestStatus{
			Group: "batch", Version: "v1", Kind: "CronJob", Name: "my-cj", Namespace: treeNamespace,
		}
		nodes := runWalk(t, svc, rootObj, rs)

		require.Len(t, nodes, 2, "the CronJob and its Job must survive the failed Pod query, got %s", nodeSummary(nodes))
		job := findNode(t, nodes, "Job", "cj-job-1")
		require.Len(t, job.ChildrenStatus, 1)
		assert.Equal(t, "forbidden", job.ChildrenStatus[0].State)
		assert.Equal(t, "Pod", job.ChildrenStatus[0].Kind)
		assert.Equal(t, "v1", job.ChildrenStatus[0].Version)
		assert.Contains(t, job.ChildrenStatus[0].Message, "forbidden")
		assert.Empty(t, findNode(t, nodes, "CronJob", "my-cj").ChildrenStatus,
			"the CronJob's own expansion succeeded and must carry no status")
	})

	t.Run("a Pod under two parents is emitted once with both parent refs", func(t *testing.T) {
		rec := &gatewayRecorder{}
		cluster := &fakeCluster{objects: []map[string]any{
			liveObject("batch/v1", "Job", treeNamespace, "job-a", "job-a-uid", ownedBy("cj-uid")),
			liveObject("batch/v1", "Job", treeNamespace, "job-b", "job-b-uid", ownedBy("cj-uid")),
			liveObject("v1", "Pod", treeNamespace, "shared-pod", "pod-uid", ownedBy("job-a-uid", "job-b-uid")),
		}}
		svc := treeService(treeGateway(t, rec, cluster.match(t), nil), testTreeConfig())

		rootObj := liveObject("batch/v1", "CronJob", treeNamespace, "my-cj", "cj-uid")
		rs := &openchoreov1alpha1.RenderedManifestStatus{
			Group: "batch", Version: "v1", Kind: "CronJob", Name: "my-cj", Namespace: treeNamespace,
		}
		nodes := runWalk(t, svc, rootObj, rs)

		require.Len(t, nodes, 4, "expected CronJob, two Jobs and one shared Pod, got %s", nodeSummary(nodes))
		pod := findNode(t, nodes, "Pod", "shared-pod")
		require.Len(t, pod.ParentRefs, 2)
		uids := []string{pod.ParentRefs[0].UID, pod.ParentRefs[1].UID}
		assert.ElementsMatch(t, []string{"job-a-uid", "job-b-uid"}, uids)
	})

	t.Run("metadataOnly projects the node object down to its metadata", func(t *testing.T) {
		rec := &gatewayRecorder{}
		cluster := &fakeCluster{objects: []map[string]any{
			liveObject("v1", "Secret", treeNamespace, "web-secret", "secret-uid",
				ownedBy("deploy-uid"), withField("type", "Opaque"), withField("data", map[string]any{"k": "dg=="})),
		}}
		svc := treeService(treeGateway(t, rec, cluster.match(t), nil), secretConfig(nil))

		nodes := runWalk(t, svc, deploymentRootObject(), deploymentRootStatus())

		require.Len(t, nodes, 2, "expected the Deployment root and one Secret, got %s", nodeSummary(nodes))
		secret := nodes[1]
		assert.True(t, secret.MetadataOnly)
		assert.True(t, rec.findQuery(t, "Secret").MetadataOnly, "the query must ask the agent to trim too")
		assert.ElementsMatch(t, []string{"apiVersion", "kind", "metadata"}, mapKeys(secret.Object),
			"an untrimmed agent response must still be projected API-side")
	})

	t.Run("an explicit metadata_only false keeps the Secret object whole", func(t *testing.T) {
		rec := &gatewayRecorder{}
		cluster := &fakeCluster{objects: []map[string]any{
			liveObject("v1", "Secret", treeNamespace, "web-secret", "secret-uid",
				ownedBy("deploy-uid"), withField("type", "Opaque"), withField("data", map[string]any{"k": "dg=="})),
		}}
		svc := treeService(treeGateway(t, rec, cluster.match(t), nil), secretConfig(boolPtr(false)))

		nodes := runWalk(t, svc, deploymentRootObject(), deploymentRootStatus())

		require.Len(t, nodes, 2)
		secret := nodes[1]
		assert.False(t, secret.MetadataOnly)
		assert.False(t, rec.findQuery(t, "Secret").MetadataOnly)
		assert.Equal(t, "Opaque", secret.Object["type"], "the object must reach the node whole")
		assert.NotContains(t, secret.Object, "data", "sanitizeObject still strips a Secret's contents")
	})

	// The kind on a fallback list item is not the API server's word for what the
	// object is: Kubernetes serves list items without one and fetchChildKindList
	// backfills it from the operator's rule text. This rule spells the kind
	// "secret" and opts out of the metadata-only default, so the unconditional
	// data strip is the only thing left between the Secret's contents and the
	// response — and it must not be keyed off that spelling.
	t.Run("a Secret found through the fallback is stripped even when the rule misspells its kind", func(t *testing.T) {
		const lastApplied = `{"apiVersion":"v1","kind":"Secret","metadata":{"name":"web-secret"},` +
			`"data":{"password":"c3VwZXItc2VjcmV0"}}`

		// A list item exactly as an API server serves one: no kind, no apiVersion.
		item := map[string]any{
			"metadata": map[string]any{
				"name":              "web-secret",
				"namespace":         treeNamespace,
				"uid":               "secret-uid",
				"resourceVersion":   "1",
				"creationTimestamp": "2024-01-15T10:00:00Z",
				"ownerReferences":   []any{map[string]any{"uid": "deploy-uid"}},
				"annotations":       map[string]any{protocol.LastAppliedConfigAnnotation: lastApplied},
			},
			"type":       "Opaque",
			"data":       map[string]any{"password": "c3VwZXItc2VjcmV0"},
			"stringData": map[string]any{"password": "super-secret"},
		}

		rec := &gatewayRecorder{}
		gc := treeGateway(t, rec, unsupportedTarget, func(*http.Request) stubResponse {
			return stubResponse{status: http.StatusOK, body: k8sList(item)}
		})
		svc := treeService(gc, lowercaseSecretConfig())

		nodes := runWalk(t, svc, deploymentRootObject(), deploymentRootStatus())

		require.NotEmpty(t, rec.legacyURLs(), "this must exercise the fallback, not the agent path")
		require.Len(t, nodes, 2, "expected the Deployment root and one Secret, got %s", nodeSummary(nodes))
		secret := nodes[1]
		assert.Equal(t, "secret", secret.Kind, "the node carries the kind the rule declared")
		assert.Equal(t, "Opaque", secret.Object["type"], "the object reaches the node whole apart from the strip")
		assert.NotContains(t, secret.Object, "data", "Secret data must never leave this API")
		assert.NotContains(t, secret.Object, "stringData")
		assert.NotContains(t, nodeAnnotations(t, secret), protocol.LastAppliedConfigAnnotation,
			"the last-applied annotation carries the same data block")
	})

	t.Run("a truncated result reports alongside the matches it did collect", func(t *testing.T) {
		rec := &gatewayRecorder{}
		cluster := &fakeCluster{objects: []map[string]any{
			liveObject("v1", "Pod", treeNamespace, "job-pod-1", "pod-uid", ownedBy("job-uid")),
		}}
		gc := treeGateway(t, rec, func(req protocol.MatchRequest) stubResponse {
			return matchResponse(req, func(q protocol.MatchQuery) protocol.MatchResult {
				// Truncated with no Error: the agent reports this shape when a
				// walk stops early but nothing failed.
				return protocol.MatchResult{Matches: cluster.answer(t, q), Truncated: true}
			})
		}, nil)
		svc := treeService(gc, testTreeConfig())

		rootObj := liveObject("batch/v1", "Job", treeNamespace, "my-job", "job-uid")
		rs := &openchoreov1alpha1.RenderedManifestStatus{
			Group: "batch", Version: "v1", Kind: "Job", Name: "my-job", Namespace: treeNamespace,
		}
		nodes := runWalk(t, svc, rootObj, rs)

		require.Len(t, nodes, 2, "a truncated result still yields the matches it carried, got %s", nodeSummary(nodes))
		findNode(t, nodes, "Pod", "job-pod-1")

		job := findNode(t, nodes, "Job", "my-job")
		require.Len(t, job.ChildrenStatus, 1, "truncation must be reported, not silently accepted as a full answer")
		assert.Equal(t, "error", job.ChildrenStatus[0].State)
		assert.Equal(t, "Pod", job.ChildrenStatus[0].Kind)
		assert.Equal(t, "the result was truncated", job.ChildrenStatus[0].Message)
	})

	t.Run("a metadataOnly node drops the annotation that carries the whole object", func(t *testing.T) {
		const lastApplied = `{"apiVersion":"v1","kind":"Secret","metadata":{"name":"web-secret"},` +
			`"data":{"password":"c3VwZXItc2VjcmV0"}}`

		rec := &gatewayRecorder{}
		cluster := &fakeCluster{objects: []map[string]any{
			liveObject("v1", "Secret", treeNamespace, "web-secret", "secret-uid",
				ownedBy("deploy-uid"),
				withAnnotations(map[string]string{
					protocol.LastAppliedConfigAnnotation: lastApplied,
					"openchoreo.dev/keep":                "displayed",
				}),
				withField("data", map[string]any{"password": "c3VwZXItc2VjcmV0"})),
		}}
		svc := treeService(treeGateway(t, rec, cluster.match(t), nil), secretConfig(nil))

		nodes := runWalk(t, svc, deploymentRootObject(), deploymentRootStatus())

		require.Len(t, nodes, 2)
		secret := nodes[1]
		require.True(t, secret.MetadataOnly)
		assert.NotContains(t, string(jsonMarshal(t, secret)), "c3VwZXItc2VjcmV0",
			"the secret value must not reach the response inside an annotation")

		annotations := nodeAnnotations(t, secret)
		assert.NotContains(t, annotations, protocol.LastAppliedConfigAnnotation)
		assert.Equal(t, "displayed", annotations["openchoreo.dev/keep"],
			"only the one annotation is dropped, not every annotation")
	})

	t.Run("a full Secret node drops the annotation too, since sanitizing data is not enough", func(t *testing.T) {
		const lastApplied = `{"apiVersion":"v1","kind":"Secret","metadata":{"name":"web-secret"},` +
			`"data":{"password":"c3VwZXItc2VjcmV0"}}`

		rec := &gatewayRecorder{}
		cluster := &fakeCluster{objects: []map[string]any{
			liveObject("v1", "Secret", treeNamespace, "web-secret", "secret-uid",
				ownedBy("deploy-uid"),
				withAnnotations(map[string]string{protocol.LastAppliedConfigAnnotation: lastApplied}),
				withField("type", "Opaque"),
				withField("data", map[string]any{"password": "c3VwZXItc2VjcmV0"})),
		}}
		svc := treeService(treeGateway(t, rec, cluster.match(t), nil), secretConfig(boolPtr(false)))

		nodes := runWalk(t, svc, deploymentRootObject(), deploymentRootStatus())

		require.Len(t, nodes, 2)
		secret := nodes[1]
		require.False(t, secret.MetadataOnly)
		assert.Equal(t, "Opaque", secret.Object["type"], "the rest of the object still reaches the node")
		assert.NotContains(t, string(jsonMarshal(t, secret)), "c3VwZXItc2VjcmV0",
			"stripping top-level data is only half the strip on an applied Secret")
		assert.NotContains(t, nodeAnnotations(t, secret), protocol.LastAppliedConfigAnnotation)
	})

	t.Run("a failure below a hidden parent lands on the nearest emitted ancestor", func(t *testing.T) {
		rec := &gatewayRecorder{}
		cluster := &fakeCluster{objects: []map[string]any{
			liveObject("apps/v1", "ReplicaSet", treeNamespace, "web-abc", "rs-uid", ownedBy("deploy-uid")),
			liveObject("v1", "ConfigMap", treeNamespace, "web-cm", "cm-uid", ownedBy("deploy-uid")),
		}}
		gc := treeGateway(t, rec, func(req protocol.MatchRequest) stubResponse {
			return matchResponse(req, func(q protocol.MatchQuery) protocol.MatchResult {
				if q.Child.Kind == podChildKind {
					return protocol.MatchResult{Error: &protocol.MatchError{
						Code: protocol.CodeInternal, Message: "list pods: connection reset",
					}}
				}
				return protocol.MatchResult{Matches: cluster.answer(t, q)}
			})
		}, nil)
		svc := treeService(gc, hiddenBranchConfig())

		nodes := runWalk(t, svc, deploymentRootObject(), deploymentRootStatus())

		require.Len(t, nodes, 2, "the hidden ReplicaSet is never emitted, got %s", nodeSummary(nodes))
		deployment := findNode(t, nodes, "Deployment", "web")
		require.Len(t, deployment.ChildrenStatus, 1, "the hidden parent's failure must surface on the Deployment")
		assert.Equal(t, "error", deployment.ChildrenStatus[0].State)
		assert.Equal(t, "Pod", deployment.ChildrenStatus[0].Kind)
		assert.Empty(t, findNode(t, nodes, "ConfigMap", "web-cm").ChildrenStatus,
			"the sibling ConfigMap did not fail and must carry no status")
	})

	t.Run("one failure shared by several hidden siblings is reported once", func(t *testing.T) {
		rec := &gatewayRecorder{}
		cluster := &fakeCluster{objects: []map[string]any{
			liveObject("apps/v1", "ReplicaSet", treeNamespace, "web-1", "rs-1-uid", ownedBy("deploy-uid")),
			liveObject("apps/v1", "ReplicaSet", treeNamespace, "web-2", "rs-2-uid", ownedBy("deploy-uid")),
			liveObject("apps/v1", "ReplicaSet", treeNamespace, "web-3", "rs-3-uid", ownedBy("deploy-uid")),
		}}
		gc := treeGateway(t, rec, func(req protocol.MatchRequest) stubResponse {
			return matchResponse(req, func(q protocol.MatchQuery) protocol.MatchResult {
				if q.Child.Kind == podChildKind {
					require.Len(t, q.Parents, 3, "all three ReplicaSets must share one query")
					return protocol.MatchResult{Error: &protocol.MatchError{
						Code: protocol.CodeForbidden, Message: "pods is forbidden",
					}}
				}
				return protocol.MatchResult{Matches: cluster.answer(t, q)}
			})
		}, nil)
		svc := treeService(gc, testTreeConfig())

		nodes := runWalk(t, svc, deploymentRootObject(), deploymentRootStatus())

		require.Len(t, nodes, 1, "every ReplicaSet is hidden, got %s", nodeSummary(nodes))
		require.Len(t, nodes[0].ChildrenStatus, 1,
			"three hidden siblings resolve to one anchor and must not stack three identical lines")
		assert.Equal(t, "forbidden", nodes[0].ChildrenStatus[0].State)
	})

	t.Run("a forbidden legacy list during the fallback surfaces as a forbidden status", func(t *testing.T) {
		rec := &gatewayRecorder{}
		cluster := &fakeCluster{objects: []map[string]any{
			liveObject("batch/v1", "Job", treeNamespace, "cj-job-1", "job-uid", ownedBy("cj-uid")),
		}}
		serveList := cluster.list(t)
		gc := treeGateway(t, rec, unsupportedTarget, func(r *http.Request) stubResponse {
			if strings.HasSuffix(r.URL.Path, "/pods") {
				return stubResponse{status: http.StatusForbidden}
			}
			return serveList(r)
		})
		svc := treeService(gc, testTreeConfig())

		rootObj := liveObject("batch/v1", "CronJob", treeNamespace, "my-cj", "cj-uid")
		rs := &openchoreov1alpha1.RenderedManifestStatus{
			Group: "batch", Version: "v1", Kind: "CronJob", Name: "my-cj", Namespace: treeNamespace,
		}
		nodes := runWalk(t, svc, rootObj, rs)

		require.Len(t, nodes, 2, "the Job must still be discovered, got %s", nodeSummary(nodes))
		job := findNode(t, nodes, "Job", "cj-job-1")
		require.Len(t, job.ChildrenStatus, 1, "a swallowed 403 would leave no status at all")
		assert.Equal(t, "forbidden", job.ChildrenStatus[0].State)
		assert.Equal(t, "Pod", job.ChildrenStatus[0].Kind)
	})

	t.Run("a query id set that does not cover the request is a hard error", func(t *testing.T) {
		assertNoFallback(t, func(protocol.MatchRequest) stubResponse {
			return stubResponse{status: http.StatusOK, body: protocol.MatchResponse{
				Version: protocol.Version,
				Results: []protocol.MatchResult{{ID: "not-a-query-we-sent"}},
			}}
		}, context.Background())
	})

	t.Run("a gateway 500 is a hard error with no fallback", func(t *testing.T) {
		assertNoFallback(t, func(protocol.MatchRequest) stubResponse {
			return stubResponse{status: http.StatusInternalServerError, raw: "boom"}
		}, context.Background())
	})

	t.Run("a malformed match response is a hard error with no fallback", func(t *testing.T) {
		assertNoFallback(t, func(protocol.MatchRequest) stubResponse {
			return stubResponse{status: http.StatusOK, raw: "not json"}
		}, context.Background())
	})

	// A request that fails in transit — a dropped connection here, a wall-clock
	// timeout in production — surfaces from the same http.Client.Do call as a
	// *gateway.TransientError. The connection is dropped rather than the context
	// expired because an expired context would also stop the legacy walk, making
	// "the fallback was not taken" indistinguishable from "the fallback failed".
	t.Run("a match request that fails in transit is a hard error with no fallback", func(t *testing.T) {
		assertNoFallback(t, func(protocol.MatchRequest) stubResponse {
			return stubResponse{dropConn: true}
		}, context.Background())
	})

	t.Run("the same kind edge in two branches gets distinct query ids", func(t *testing.T) {
		rec := &gatewayRecorder{}
		cluster := &fakeCluster{objects: []map[string]any{
			liveObject("apps/v1", "ReplicaSet", treeNamespace, "web-abc", "rs-uid", ownedBy("deploy-uid")),
			liveObject("batch/v1", "Job", treeNamespace, "web-job", "job-uid", ownedBy("deploy-uid")),
			liveObject("v1", "Pod", treeNamespace, "rs-pod", "rs-pod-uid", ownedBy("rs-uid")),
			liveObject("v1", "Pod", treeNamespace, "job-pod", "job-pod-uid", ownedBy("job-uid")),
		}}
		svc := treeService(treeGateway(t, rec, cluster.match(t), nil), twoBranchConfig())

		nodes := runWalk(t, svc, deploymentRootObject(), deploymentRootStatus())

		// Deployment + Job + two Pods; the ReplicaSet is hidden.
		require.Len(t, nodes, 4, "got %s", nodeSummary(nodes))

		ids := rec.queryIDs()
		require.Len(t, ids, 4, "two edges at level one and two Pod edges at level two: %v", ids)
		assert.Len(t, slices.Compact(slices.Sorted(slices.Values(ids))), len(ids),
			"query ids must be unique per edge, got %v", ids)

		// Both Pod queries are for the same child kind at the same level; only the
		// structural path tells them apart.
		podIDs := make([]string, 0, 2)
		for _, req := range rec.matchRequests() {
			for _, q := range req.Queries {
				if q.Child.Kind == podChildKind {
					podIDs = append(podIDs, q.ID)
				}
			}
		}
		require.Len(t, podIDs, 2)
		assert.NotEqual(t, podIDs[0], podIDs[1])
	})

	t.Run("a labelSelector edge sends unsubstituted criteria and badges its nodes", func(t *testing.T) {
		rec := &gatewayRecorder{}
		cluster := gatewayTree()
		svc := treeService(treeGateway(t, rec, cluster.match(t), nil), gatewayConfig())

		nodes := runWalk(t, svc, gatewayRootObject(), gatewayRootStatus())

		require.Len(t, nodes, 3, "expected the Gateway, its Service and its Deployment, got %s", nodeSummary(nodes))

		query := rec.findQuery(t, "Deployment")
		assert.Equal(t, protocol.MatcherLabelSelector, query.Matcher)
		var criteria protocol.LabelSelectorCriteria
		require.NoError(t, json.Unmarshal(query.Criteria, &criteria))
		assert.Equal(t, config.TokenParentName, criteria.MatchLabels["owner"],
			"the token must cross the wire unsubstituted; the agent substitutes per parent")
		assert.Equal(t, []string{"gw-system"}, criteria.Namespaces)

		assert.Equal(t, "labelSelector", findNode(t, nodes, "Deployment", "gw-a-deploy").MatchedBy)
		assert.Empty(t, findNode(t, nodes, "Service", "gw-a-svc").MatchedBy,
			"the ownerRef-matched sibling must not be badged")
	})

	t.Run("the fallback substitutes the selector server-side for a labelSelector edge", func(t *testing.T) {
		rec := &gatewayRecorder{}
		cluster := gatewayTree()
		svc := treeService(treeGateway(t, rec, unsupportedTarget, cluster.list(t)), gatewayConfig())

		nodes := runWalk(t, svc, gatewayRootObject(), gatewayRootStatus())

		require.Len(t, nodes, 3, "expected the Gateway, its Service and its Deployment, got %s", nodeSummary(nodes))
		assert.Equal(t, "labelSelector", findNode(t, nodes, "Deployment", "gw-a-deploy").MatchedBy)
		findNode(t, nodes, "Service", "gw-a-svc")

		var selectorURLs, plainURLs []*url.URL
		for _, u := range rec.legacyURLs() {
			if u.Query().Get("labelSelector") != "" {
				selectorURLs = append(selectorURLs, u)
			} else {
				plainURLs = append(plainURLs, u)
			}
		}

		require.Len(t, selectorURLs, 1, "exactly one list must carry a selector: %v", rec.legacyURLs())
		assert.Equal(t, "owner=gw-a", selectorURLs[0].Query().Get("labelSelector"),
			"the control plane must substitute the parent token itself")
		plural, namespace := parseListPath(t, selectorURLs[0])
		assert.Equal(t, "deployments", plural)
		assert.Equal(t, "gw-system", namespace, "the rule's target namespace, not the parent's")

		require.Len(t, plainURLs, 1, "the ownerRef edge lists without a selector")
		plural, namespace = parseListPath(t, plainURLs[0])
		assert.Equal(t, "services", plural)
		assert.Equal(t, treeNamespace, namespace)
	})

	t.Run("a safely scoped labelSelector edge supports a cluster-scoped parent", func(t *testing.T) {
		for _, fallback := range []bool{false, true} {
			t.Run(fmt.Sprintf("legacy=%t", fallback), func(t *testing.T) {
				rec := &gatewayRecorder{}
				cluster := &fakeCluster{objects: []map[string]any{
					liveObject("apps/v1", "Deployment", "gw-system", "gw-a-deploy", "deploy-uid",
						withLabels(map[string]string{"owner": "gw-a"})),
				}}
				cfg := config.ResourceTreeConfig{Rules: []config.ResourceTreeRule{{
					Root: config.KindRef{Group: "gateway.networking.k8s.io", Version: "v1", Kind: "GatewayClass", Resource: "gatewayclasses"},
					Children: []config.ChildRule{{
						Kind:    config.KindRef{Group: "apps", Version: "v1", Kind: "Deployment", Resource: "deployments"},
						Matcher: config.MatcherLabelSelector,
						LabelSelector: &config.LabelSelectorSpec{
							MatchLabels: map[string]string{"owner": config.TokenParentName},
							Namespaces:  []string{"gw-system"},
						},
					}},
				}}}
				onMatch := cluster.match(t)
				var onLegacy func(*http.Request) stubResponse
				if fallback {
					onMatch = unsupportedTarget
					onLegacy = cluster.list(t)
				}
				svc := treeService(treeGateway(t, rec, onMatch, onLegacy), cfg)
				rootObj := liveObject("gateway.networking.k8s.io/v1", "GatewayClass", "", "gw-a", "gatewayclass-uid")
				rs := &openchoreov1alpha1.RenderedManifestStatus{
					Group: "gateway.networking.k8s.io", Version: "v1", Kind: "GatewayClass", Name: "gw-a",
				}

				nodes := runWalk(t, svc, rootObj, rs)
				require.Len(t, nodes, 2, "got %s", nodeSummary(nodes))
				assert.Equal(t, "gw-system", findNode(t, nodes, "Deployment", "gw-a-deploy").Namespace)
			})
		}
	})

	t.Run("a cluster-scoped parent is reported rather than queried", func(t *testing.T) {
		rec := &gatewayRecorder{}
		gc := treeGateway(t, rec, func(protocol.MatchRequest) stubResponse {
			t.Error("a cluster-scoped parent must never reach the agent")
			return stubResponse{status: http.StatusOK}
		}, nil)
		svc := treeService(gc, testTreeConfig())

		rootObj := liveObject("apps/v1", "Deployment", "", "web", "deploy-uid")
		rs := &openchoreov1alpha1.RenderedManifestStatus{
			Group: "apps", Version: "v1", Kind: "Deployment", Name: "web",
		}
		nodes := runWalk(t, svc, rootObj, rs)

		require.Len(t, nodes, 1)
		require.Len(t, nodes[0].ChildrenStatus, 1)
		assert.Equal(t, "error", nodes[0].ChildrenStatus[0].State)
		assert.Equal(t, "ReplicaSet", nodes[0].ChildrenStatus[0].Kind)
		assert.Equal(t, "cluster-scoped parents are not supported", nodes[0].ChildrenStatus[0].Message)
		assert.Empty(t, rec.matchRequests())
		assert.Empty(t, rec.legacyURLs())
	})
}

// assertNoFallback pins the errors that must NOT reach the legacy walk: the
// already-discovered Job stays, its Pod expansion is reported as an error, and
// the legacy list endpoints are never touched. Mapping any of these onto the
// version-skew sentinel would silently downgrade an outage to a slow path.
func assertNoFallback(t *testing.T, onMatch func(protocol.MatchRequest) stubResponse, ctx context.Context) {
	t.Helper()

	rec := &gatewayRecorder{}
	cluster := &fakeCluster{objects: []map[string]any{
		liveObject("batch/v1", "Job", treeNamespace, "cj-job-1", "job-uid", ownedBy("cj-uid")),
	}}
	gc := treeGateway(t, rec, func(req protocol.MatchRequest) stubResponse {
		if req.Queries[0].Child.Kind == podChildKind {
			return onMatch(req)
		}
		return matchResponse(req, func(q protocol.MatchQuery) protocol.MatchResult {
			return protocol.MatchResult{Matches: cluster.answer(t, q)}
		})
	}, func(*http.Request) stubResponse {
		t.Error("a hard discovery error must never fall back to the legacy list endpoints")
		return stubResponse{status: http.StatusOK, body: k8sList()}
	})
	svc := treeService(gc, testTreeConfig())

	rootObj := liveObject("batch/v1", "CronJob", treeNamespace, "my-cj", "cj-uid")
	rs := &openchoreov1alpha1.RenderedManifestStatus{
		Group: "batch", Version: "v1", Kind: "CronJob", Name: "my-cj", Namespace: treeNamespace,
	}
	nodes := runWalkCtx(t, ctx, svc, rootObj, rs)

	require.Len(t, nodes, 2, "already emitted nodes must survive, got %s", nodeSummary(nodes))
	job := findNode(t, nodes, "Job", "cj-job-1")
	require.Len(t, job.ChildrenStatus, 1)
	assert.Equal(t, "error", job.ChildrenStatus[0].State)
	assert.Equal(t, "Pod", job.ChildrenStatus[0].Kind)
	assert.NotEmpty(t, job.ChildrenStatus[0].Message)
	assert.Empty(t, rec.legacyURLs(), "the legacy list endpoints must receive zero calls")
}

// TestFetchSelectedChildrenSkipsEmptyParentField mirrors the agent's skip on
// the fallback path: an empty parent field substitutes to "", a legal empty
// label value that would list and claim objects whose label value is empty. The
// two paths must return the same tree, so the fallback skips the parent too.
func TestFetchSelectedChildrenSkipsEmptyParentField(t *testing.T) {
	rec := &gatewayRecorder{}
	gc := treeGateway(t, rec, unsupportedTarget, func(*http.Request) stubResponse {
		t.Error("a parent with an empty field the selector derives from must not be listed")
		return stubResponse{status: http.StatusOK, body: k8sList()}
	})
	svc := treeService(gc, testTreeConfig())

	edge := &compiledChild{
		Kind:    config.KindRef{Group: "apps", Version: "v1", Kind: "Deployment", Resource: "deployments"},
		Matcher: config.MatcherLabelSelector,
		Criteria: jsonMarshal(t, protocol.LabelSelectorCriteria{
			MatchLabels: map[string]string{"ns": protocol.TokenParentNamespace},
			Namespaces:  []string{"fixed-ns"},
		}),
		EdgeID: "gw/deployments",
	}

	objects, err := svc.fetchSelectedChildren(context.Background(), treePlaneInfo(), edge,
		&walkParent{uid: "gw-uid", name: "gw-a"})

	require.NoError(t, err, "a skipped parent is not an error")
	assert.Empty(t, objects)
	assert.Empty(t, rec.legacyURLs())
}

func mapKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	return keys
}

// --- fetchOwnedResources / fetchLabelSelectedResources ---

func TestFetchOwnedResources(t *testing.T) {
	t.Run("filters by owner UID", func(t *testing.T) {
		ownedPod := liveObject("v1", "Pod", "ns1", "pod-owned", "pod-uid-1", ownedBy("owner-uid"))
		unownedPod := liveObject("v1", "Pod", "ns1", "pod-other", "pod-uid-2", ownedBy("different-uid"))

		gc := testGatewayServer(t, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(jsonMarshal(t, k8sList(ownedPod, unownedPod)))
		})

		svc := treeService(gc, testTreeConfig())
		owned, err := svc.fetchOwnedResources(context.Background(), treePlaneInfo(), "", "v1", "Pod", "pods", "ns1", "owner-uid")
		require.NoError(t, err)
		require.Len(t, owned, 1)
		assert.Equal(t, "Pod", owned[0]["kind"])
		assert.Equal(t, "v1", owned[0]["apiVersion"])
	})

	t.Run("uses the configured version and plural instead of assuming v1", func(t *testing.T) {
		var capturedPath string
		gc := testGatewayServer(t, func(w http.ResponseWriter, r *http.Request) {
			capturedPath = r.URL.Path
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(jsonMarshal(t, k8sList()))
		})

		svc := treeService(gc, testTreeConfig())
		_, err := svc.fetchOwnedResources(context.Background(), treePlaneInfo(),
			"gateway.networking.k8s.io", "v1beta1", "Gateway", "gateways", "ns1", "owner-uid")
		require.NoError(t, err)
		assert.Contains(t, capturedPath, "apis/gateway.networking.k8s.io/v1beta1/namespaces/ns1/gateways")
	})

	t.Run("backfills apiVersion and kind on items served without them", func(t *testing.T) {
		bare := map[string]any{"metadata": map[string]any{
			"name": "rs-1", "namespace": "ns1", "uid": "rs-uid",
			"ownerReferences": []any{map[string]any{"uid": "deploy-uid"}},
		}}

		gc := testGatewayServer(t, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(jsonMarshal(t, k8sList(bare)))
		})

		svc := treeService(gc, testTreeConfig())
		owned, err := svc.fetchOwnedResources(context.Background(), treePlaneInfo(),
			"apps", "v1", "ReplicaSet", "replicasets", "ns1", "deploy-uid")
		require.NoError(t, err)
		require.Len(t, owned, 1)
		assert.Equal(t, "apps/v1", owned[0]["apiVersion"])
		assert.Equal(t, "ReplicaSet", owned[0]["kind"])
	})

	t.Run("a gateway error is returned, not swallowed", func(t *testing.T) {
		gc := testGatewayServer(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusForbidden)
		})

		svc := treeService(gc, testTreeConfig())
		owned, err := svc.fetchOwnedResources(context.Background(), treePlaneInfo(), "", "v1", "Pod", "pods", "ns1", "owner-uid")
		require.Error(t, err, "a forbidden list must be distinguishable from an empty one")
		assert.Empty(t, owned)

		var statusErr *liveResourceStatusError
		require.ErrorAs(t, err, &statusErr)
		assert.Equal(t, http.StatusForbidden, statusErr.statusCode)
	})
}

func TestFetchLabelSelectedResources(t *testing.T) {
	t.Run("pushes the selector to the API server and does not filter by owner", func(t *testing.T) {
		unowned := liveObject("apps/v1", "Deployment", "ns1", "dep-1", "dep-uid",
			withLabels(map[string]string{"owner": "gw-a"}))

		var capturedSelector, capturedPath string
		gc := testGatewayServer(t, func(w http.ResponseWriter, r *http.Request) {
			capturedSelector = r.URL.Query().Get("labelSelector")
			capturedPath = r.URL.Path
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(jsonMarshal(t, k8sList(unowned)))
		})

		svc := treeService(gc, testTreeConfig())
		items, err := svc.fetchLabelSelectedResources(context.Background(), treePlaneInfo(),
			"apps", "v1", "Deployment", "deployments", "ns1", "owner=gw-a")
		require.NoError(t, err)
		require.Len(t, items, 1, "a labelSelector match carries no owner reference and must not be filtered out")
		assert.Equal(t, "owner=gw-a", capturedSelector)
		assert.Contains(t, capturedPath, "apis/apps/v1/namespaces/ns1/deployments")
	})

	t.Run("a gateway error is returned", func(t *testing.T) {
		gc := testGatewayServer(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		})

		svc := treeService(gc, testTreeConfig())
		_, err := svc.fetchLabelSelectedResources(context.Background(), treePlaneInfo(),
			"apps", "v1", "Deployment", "deployments", "ns1", "owner=gw-a")
		require.Error(t, err)
	})
}

// --- resolveResourcePlural ---

func TestResolveResourcePlural(t *testing.T) {
	t.Run("resolves known types via REST mapper", func(t *testing.T) {
		fc := newFakeClient()
		svc := &k8sResourcesService{k8sClient: fc, rules: testRules(), logger: testLogger()}

		// The fake client's REST mapper knows about standard K8s types
		plural, err := svc.resolveResourcePlural("apps", "v1", "Deployment")
		require.NoError(t, err)
		assert.Equal(t, "deployments", plural)
	})

	t.Run("unknown type returns error", func(t *testing.T) {
		fc := newFakeClient()
		svc := &k8sResourcesService{k8sClient: fc, rules: testRules(), logger: testLogger()}

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
				Resources: []openchoreov1alpha1.RenderedManifestStatus{
					{ID: "svc", Group: "", Version: "v1", Kind: "Service", Name: "web-svc", Namespace: "dp-ns",
						HealthStatus: openchoreov1alpha1.HealthStatusHealthy},
				},
			},
		}

		fc := newFakeClient()
		svc := &k8sResourcesService{k8sClient: fc, gatewayClient: gc, rules: testRules(), logger: testLogger()}
		pi := planeInfo{planeType: "dataplane", planeID: "dp1", crNamespace: "ns", crName: "dp"}
		rc := &releaseContext{release: release, plane: pi, namespace: "dp-ns"}

		nodes, _ := svc.buildResourceTreeNodes(context.Background(), rc)
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
				Resources: []openchoreov1alpha1.RenderedManifestStatus{
					{ID: "svc", Group: "", Version: "v1", Kind: "Service", Name: "missing-svc", Namespace: "dp-ns"},
				},
			},
		}

		fc := newFakeClient()
		svc := &k8sResourcesService{k8sClient: fc, gatewayClient: gc, rules: testRules(), logger: testLogger()}
		pi := planeInfo{planeType: "dataplane", planeID: "dp1", crNamespace: "ns", crName: "dp"}
		rc := &releaseContext{release: release, plane: pi, namespace: "dp-ns"}

		nodes, _ := svc.buildResourceTreeNodes(context.Background(), rc)
		assert.Empty(t, nodes)
	})

	t.Run("a dataplane-only root resolves its plural from the rule, not the RESTMapper", func(t *testing.T) {
		// Gateway is deliberately absent from testRESTMapper: resolving it through
		// the control plane would fail before discovery ever started.
		_, err := (&k8sResourcesService{k8sClient: newFakeClient(), rules: testRules()}).
			resolveResourcePlural("gateway.networking.k8s.io", "v1", "Gateway")
		require.Error(t, err, "the fixture RESTMapper must not know this kind")

		rec := &gatewayRecorder{}
		var capturedPath string
		gc := treeGateway(t, rec, nil, func(r *http.Request) stubResponse {
			capturedPath = r.URL.Path
			return stubResponse{status: http.StatusOK, body: gatewayRootObject()}
		})

		release := &openchoreov1alpha1.RenderedRelease{
			Status: openchoreov1alpha1.RenderedReleaseStatus{
				Resources: []openchoreov1alpha1.RenderedManifestStatus{*gatewayRootStatus()},
			},
		}
		svc := treeService(gc, gatewayConfig())
		rc := &releaseContext{release: release, plane: treePlaneInfo(), namespace: treeNamespace}

		nodes, _ := svc.buildResourceTreeNodes(context.Background(), rc)
		require.Len(t, nodes, 1)
		assert.Equal(t, "Gateway", nodes[0].Kind)
		assert.Contains(t, capturedPath,
			"apis/gateway.networking.k8s.io/v1/namespaces/dp-ns/gateways/gw-a")
	})

	t.Run("a child shared by two roots is merged into the node the caller receives", func(t *testing.T) {
		// Regression test for holding a *models.ResourceNode across appends. The
		// shared Pod is emitted while expanding the first Deployment, then merged
		// while expanding the last resource in the release. The accumulator starts
		// at exactly len(Resources) capacity, so emitting MORE nodes than there are
		// resources guarantees the backing array was reallocated between the two —
		// a stored pointer would then write the merge into the abandoned array and
		// the returned slice would still show a single parent.
		const sharedPodUID = "shared-pod-uid"
		cluster := &fakeCluster{objects: []map[string]any{
			liveObject("apps/v1", "ReplicaSet", treeNamespace, "web-a-rs", "rs-a-uid", ownedBy("deploy-a-uid")),
			liveObject("apps/v1", "ReplicaSet", treeNamespace, "web-b-rs", "rs-b-uid", ownedBy("deploy-b-uid")),
			liveObject("v1", "Pod", treeNamespace, "shared-pod", sharedPodUID, ownedBy("rs-a-uid", "rs-b-uid")),
		}}

		resources := make([]openchoreov1alpha1.RenderedManifestStatus, 0, 22)
		resources = append(resources, openchoreov1alpha1.RenderedManifestStatus{
			Group: "apps", Version: "v1", Kind: "Deployment", Name: "web-a", Namespace: treeNamespace,
		})
		liveByName := map[string]map[string]any{
			"web-a": liveObject("apps/v1", "Deployment", treeNamespace, "web-a", "deploy-a-uid"),
			"web-b": liveObject("apps/v1", "Deployment", treeNamespace, "web-b", "deploy-b-uid"),
		}
		// Twenty childless Services separate the first emit from the merge.
		for i := range 20 {
			name := fmt.Sprintf("filler-%d", i)
			resources = append(resources, openchoreov1alpha1.RenderedManifestStatus{
				Group: "", Version: "v1", Kind: "Service", Name: name, Namespace: treeNamespace,
			})
			liveByName[name] = liveObject("v1", "Service", treeNamespace, name, "svc-uid-"+name)
		}
		resources = append(resources, openchoreov1alpha1.RenderedManifestStatus{
			Group: "apps", Version: "v1", Kind: "Deployment", Name: "web-b", Namespace: treeNamespace,
		})

		rec := &gatewayRecorder{}
		gc := treeGateway(t, rec, cluster.match(t), func(r *http.Request) stubResponse {
			segments := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
			obj, ok := liveByName[segments[len(segments)-1]]
			require.True(t, ok, "unexpected live GET %s", r.URL.Path)
			return stubResponse{status: http.StatusOK, body: obj}
		})

		release := &openchoreov1alpha1.RenderedRelease{
			Status: openchoreov1alpha1.RenderedReleaseStatus{Resources: resources},
		}
		svc := treeService(gc, testTreeConfig())
		rc := &releaseContext{release: release, plane: treePlaneInfo(), namespace: treeNamespace}

		nodes, _ := svc.buildResourceTreeNodes(context.Background(), rc)

		require.Greater(t, len(nodes), len(resources),
			"the emitted nodes must outnumber the seed capacity, or no reallocation happened")
		pod := findNode(t, nodes, "Pod", "shared-pod")
		require.Len(t, pod.ParentRefs, 2, "the shared Pod must carry both Deployments")
		assert.ElementsMatch(t, []string{"deploy-a-uid", "deploy-b-uid"},
			[]string{pod.ParentRefs[0].UID, pod.ParentRefs[1].UID})

		seen := map[string]int{}
		for _, node := range nodes {
			seen[node.UID]++
		}
		assert.Equal(t, 1, seen[sharedPodUID], "the shared Pod must be emitted exactly once")
	})
}

// --- NewServiceWithAuthz ---

func TestNewServiceWithAuthz(t *testing.T) {
	fc := newFakeClient()
	gc, err := gateway.NewClientWithConfig(&gateway.Config{BaseURL: "http://localhost"})
	require.NoError(t, err)
	pdp := authzmocks.NewMockPDP(t)
	svc := NewServiceWithAuthz(fc, gc, pdp, testTreeConfig(), testLogger())
	require.NotNil(t, svc)
}
