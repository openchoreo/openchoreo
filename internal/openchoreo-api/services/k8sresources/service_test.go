// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package k8sresources

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/controller"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/config"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/models"
)

// The GVRs objects are selected through, which is what sanitizeObject keys off.
var (
	podGVR        = schema.GroupVersionResource{Version: "v1", Resource: "pods"}
	secretGVR     = schema.GroupVersionResource{Version: "v1", Resource: "secrets"}
	configMapGVR  = schema.GroupVersionResource{Version: "v1", Resource: "configmaps"}
	deploymentGVR = schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}
)

func TestBuildK8sGetPath(t *testing.T) {
	tests := []struct {
		name      string
		group     string
		version   string
		plural    string
		namespace string
		resName   string
		want      string
	}{
		{
			name:      "core API namespaced",
			group:     "",
			version:   "v1",
			plural:    "pods",
			namespace: "ns1",
			resName:   "pod1",
			want:      "api/v1/namespaces/ns1/pods/pod1",
		},
		{
			name:      "core API cluster-scoped",
			group:     "",
			version:   "v1",
			plural:    "namespaces",
			namespace: "",
			resName:   "ns1",
			want:      "api/v1/namespaces/ns1",
		},
		{
			name:      "named group namespaced",
			group:     "apps",
			version:   "v1",
			plural:    "deployments",
			namespace: "ns1",
			resName:   "dep1",
			want:      "apis/apps/v1/namespaces/ns1/deployments/dep1",
		},
		{
			name:      "named group cluster-scoped",
			group:     "rbac.authorization.k8s.io",
			version:   "v1",
			plural:    "clusterroles",
			namespace: "",
			resName:   "admin",
			want:      "apis/rbac.authorization.k8s.io/v1/clusterroles/admin",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildK8sGetPath(tt.group, tt.version, tt.plural, tt.namespace, tt.resName)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestBuildK8sListPath(t *testing.T) {
	tests := []struct {
		name      string
		group     string
		version   string
		plural    string
		namespace string
		want      string
	}{
		{
			name:      "core API namespaced",
			group:     "",
			version:   "v1",
			plural:    "pods",
			namespace: "ns1",
			want:      "api/v1/namespaces/ns1/pods",
		},
		{
			name:      "core API cluster-scoped",
			group:     "",
			version:   "v1",
			plural:    "namespaces",
			namespace: "",
			want:      "api/v1/namespaces",
		},
		{
			name:      "named group namespaced",
			group:     "apps",
			version:   "v1",
			plural:    "deployments",
			namespace: "ns1",
			want:      "apis/apps/v1/namespaces/ns1/deployments",
		},
		{
			name:      "named group cluster-scoped",
			group:     "rbac.authorization.k8s.io",
			version:   "v1",
			plural:    "clusterroles",
			namespace: "",
			want:      "apis/rbac.authorization.k8s.io/v1/clusterroles",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildK8sListPath(tt.group, tt.version, tt.plural, tt.namespace)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestBuildResourceNode(t *testing.T) {
	t.Run("valid object", func(t *testing.T) {
		obj := map[string]any{
			"apiVersion": "apps/v1",
			"kind":       "Deployment",
			"metadata": map[string]any{
				"name":              "my-deploy",
				"namespace":         "ns1",
				"uid":               "uid-123",
				"resourceVersion":   "rv-456",
				"creationTimestamp": "2024-01-15T10:30:00Z",
			},
		}

		node, ok := buildResourceNode(obj, deploymentGVR, nil, "Healthy")
		require.True(t, ok)
		assert.Equal(t, "apps", node.Group)
		assert.Equal(t, "v1", node.Version)
		assert.Equal(t, "Deployment", node.Kind)
		assert.Equal(t, "ns1", node.Namespace)
		assert.Equal(t, "my-deploy", node.Name)
		assert.Equal(t, "uid-123", node.UID)
		assert.Equal(t, "rv-456", node.ResourceVersion)
		require.NotNil(t, node.CreatedAt)
		assert.Equal(t, 2024, node.CreatedAt.Year())
		assert.Nil(t, node.ParentRefs)
	})

	t.Run("missing required fields", func(t *testing.T) {
		obj := map[string]any{
			"apiVersion": "v1",
			"kind":       "Pod",
			"metadata": map[string]any{
				"name": "my-pod",
				// missing uid
			},
		}

		node, ok := buildResourceNode(obj, podGVR, nil, "")
		assert.False(t, ok)
		assert.Equal(t, models.ResourceNode{}, node)
	})

	t.Run("with parent ref", func(t *testing.T) {
		obj := map[string]any{
			"apiVersion": "v1",
			"kind":       "Pod",
			"metadata": map[string]any{
				"name":      "my-pod",
				"namespace": "ns1",
				"uid":       "pod-uid",
			},
		}
		parentRef := &models.ResourceRef{
			Group:     "apps",
			Version:   "v1",
			Kind:      "Deployment",
			Namespace: "ns1",
			Name:      "my-deploy",
			UID:       "deploy-uid",
		}

		node, ok := buildResourceNode(obj, podGVR, parentRef, "")
		require.True(t, ok)
		require.Len(t, node.ParentRefs, 1)
		assert.Equal(t, *parentRef, node.ParentRefs[0])
	})

	t.Run("with health status", func(t *testing.T) {
		obj := map[string]any{
			"apiVersion": "apps/v1",
			"kind":       "Deployment",
			"metadata": map[string]any{
				"name": "my-deploy",
				"uid":  "uid-789",
			},
		}

		node, ok := buildResourceNode(obj, deploymentGVR, nil, openchoreov1alpha1.HealthStatusDegraded)
		require.True(t, ok)
		require.NotNil(t, node.Health)
		assert.Equal(t, "Degraded", node.Health.Status)
	})
}

func TestSanitizeObject(t *testing.T) {
	t.Run("removes managedFields", func(t *testing.T) {
		obj := map[string]any{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata": map[string]any{
				"name":          "my-cm",
				"managedFields": []any{"field1", "field2"},
			},
		}

		result := sanitizeObject(obj, configMapGVR)
		metadata := result["metadata"].(map[string]any)
		assert.NotContains(t, metadata, "managedFields")
		assert.Equal(t, "my-cm", metadata["name"])
		// Original should be unmodified
		origMeta := obj["metadata"].(map[string]any)
		assert.Contains(t, origMeta, "managedFields")
	})

	t.Run("removes Secret data", func(t *testing.T) {
		obj := map[string]any{
			"apiVersion": "v1",
			"kind":       "Secret",
			"metadata": map[string]any{
				"name": "my-secret",
			},
			"data":       map[string]any{"key": "dmFsdWU="},
			"stringData": map[string]any{"key": "value"},
			"type":       "Opaque",
		}

		result := sanitizeObject(obj, secretGVR)
		assert.NotContains(t, result, "data")
		assert.NotContains(t, result, "stringData")
		assert.Equal(t, "Opaque", result["type"])
	})

	// The two cases below pin that the GVR decides, not the object's kind. On
	// the legacy fallback path the kind is not the API server's word for what
	// the object is: list items arrive without one and fetchChildKindList
	// backfills it from the operator's rule text.
	t.Run("strips Secret data when the kind is spelled non-canonically", func(t *testing.T) {
		obj := map[string]any{
			"apiVersion": "v1",
			// What a rule saying `kind: secret` backfills onto a list item.
			"kind": "secret",
			"metadata": map[string]any{
				"name": "my-secret",
				"annotations": map[string]any{
					"kubectl.kubernetes.io/last-applied-configuration": `{"data":{"key":"dmFsdWU="}}`,
				},
			},
			"data":       map[string]any{"key": "dmFsdWU="},
			"stringData": map[string]any{"key": "value"},
		}

		result := sanitizeObject(obj, secretGVR)
		assert.NotContains(t, result, "data")
		assert.NotContains(t, result, "stringData")
		metadata := result["metadata"].(map[string]any)
		annotations := metadata["annotations"].(map[string]any)
		assert.NotContains(t, annotations, "kubectl.kubernetes.io/last-applied-configuration",
			"the annotation carries the same data block the deletes above removed")
	})

	t.Run("leaves a CRD named Secret in another group whole", func(t *testing.T) {
		obj := map[string]any{
			"apiVersion": "example.com/v1",
			"kind":       "Secret",
			"metadata":   map[string]any{"name": "not-a-core-secret"},
			"data":       map[string]any{"key": "value"},
		}

		result := sanitizeObject(obj, schema.GroupVersionResource{
			Group: "example.com", Version: "v1", Resource: "secrets",
		})
		assert.Equal(t, map[string]any{"key": "value"}, result["data"],
			"only core v1 Secrets are stripped; another group's Secret is an unrelated CRD")
	})
}

func TestMapEventItem(t *testing.T) {
	t.Run("with firstTimestamp", func(t *testing.T) {
		item := map[string]any{
			"type":           "Warning",
			"reason":         "BackOff",
			"message":        "Back-off restarting failed container",
			"count":          float64(5),
			"firstTimestamp": "2024-01-15T10:00:00Z",
			"lastTimestamp":  "2024-01-15T10:05:00Z",
			"source": map[string]any{
				"component": "kubelet",
			},
		}

		event := mapEventItem(item)
		assert.Equal(t, "Warning", event.Type)
		assert.Equal(t, "BackOff", event.Reason)
		assert.Equal(t, "Back-off restarting failed container", event.Message)
		require.NotNil(t, event.Count)
		assert.Equal(t, int32(5), *event.Count)
		require.NotNil(t, event.FirstTimestamp)
		assert.Equal(t, 2024, event.FirstTimestamp.Year())
		require.NotNil(t, event.LastTimestamp)
		assert.Equal(t, "kubelet", event.Source)
	})

	t.Run("with eventTime only", func(t *testing.T) {
		item := map[string]any{
			"type":      "Normal",
			"reason":    "Scheduled",
			"message":   "Successfully assigned pod",
			"eventTime": "2024-06-01T12:30:45.123456789Z",
			"source":    map[string]any{},
		}

		event := mapEventItem(item)
		require.NotNil(t, event.FirstTimestamp)
		assert.Equal(t, 2024, event.FirstTimestamp.Year())
		assert.Equal(t, 6, int(event.FirstTimestamp.Month()))
		// lastTimestamp should fall back to firstTimestamp
		require.NotNil(t, event.LastTimestamp)
		assert.Equal(t, event.FirstTimestamp, event.LastTimestamp)
	})

	t.Run("source fallback to reportingComponent", func(t *testing.T) {
		item := map[string]any{
			"type":               "Normal",
			"reason":             "Created",
			"message":            "Created container",
			"source":             map[string]any{},
			"reportingComponent": "kube-scheduler",
		}

		event := mapEventItem(item)
		assert.Equal(t, "kube-scheduler", event.Source)
	})
}

func TestParseLogLines(t *testing.T) {
	t.Run("RFC3339 timestamps", func(t *testing.T) {
		raw := "2024-01-01T00:00:00Z log message here\n2024-01-01T00:00:01Z another line"
		entries := parseLogLines(raw)
		require.Len(t, entries, 2)
		assert.Equal(t, "2024-01-01T00:00:00Z", entries[0].Timestamp)
		assert.Equal(t, "log message here", entries[0].Log)
		assert.Equal(t, "2024-01-01T00:00:01Z", entries[1].Timestamp)
		assert.Equal(t, "another line", entries[1].Log)
	})

	t.Run("RFC3339Nano timestamps", func(t *testing.T) {
		raw := "2024-01-01T00:00:00.123456789Z nano log msg"
		entries := parseLogLines(raw)
		require.Len(t, entries, 1)
		assert.Equal(t, "2024-01-01T00:00:00.123456789Z", entries[0].Timestamp)
		assert.Equal(t, "nano log msg", entries[0].Log)
	})

	t.Run("no valid timestamp", func(t *testing.T) {
		raw := "some random log line without timestamp\nanother bad line"
		entries := parseLogLines(raw)
		assert.Empty(t, entries)
	})

	t.Run("empty and blank lines", func(t *testing.T) {
		raw := "\n   \n\n2024-01-01T00:00:00Z valid line\n   \n"
		entries := parseLogLines(raw)
		require.Len(t, entries, 1)
		assert.Equal(t, "valid line", entries[0].Log)
	})
}

func TestHasOwnerReference(t *testing.T) {
	t.Run("match", func(t *testing.T) {
		obj := map[string]any{
			"metadata": map[string]any{
				"ownerReferences": []any{
					map[string]any{"uid": "owner-uid-1"},
					map[string]any{"uid": "owner-uid-2"},
				},
			},
		}
		assert.True(t, hasOwnerReference(obj, "owner-uid-2"))
	})

	t.Run("no match", func(t *testing.T) {
		obj := map[string]any{
			"metadata": map[string]any{
				"ownerReferences": []any{
					map[string]any{"uid": "owner-uid-1"},
				},
			},
		}
		assert.False(t, hasOwnerReference(obj, "different-uid"))
	})

	t.Run("no metadata", func(t *testing.T) {
		obj := map[string]any{}
		assert.False(t, hasOwnerReference(obj, "any-uid"))
	})
}

func TestGetNestedString(t *testing.T) {
	obj := map[string]any{
		"kind": "Pod",
		"metadata": map[string]any{
			"name": "my-pod",
		},
	}

	t.Run("single key", func(t *testing.T) {
		assert.Equal(t, "Pod", getNestedString(obj, "kind"))
	})

	t.Run("nested keys", func(t *testing.T) {
		assert.Equal(t, "my-pod", getNestedString(obj, "metadata", "name"))
	})

	t.Run("missing key", func(t *testing.T) {
		assert.Equal(t, "", getNestedString(obj, "metadata", "nonexistent"))
	})
}

func TestGetAPIGroup(t *testing.T) {
	t.Run("with group", func(t *testing.T) {
		obj := map[string]any{"apiVersion": "apps/v1"}
		assert.Equal(t, "apps", getAPIGroup(obj))
	})

	t.Run("core API", func(t *testing.T) {
		obj := map[string]any{"apiVersion": "v1"}
		assert.Equal(t, "", getAPIGroup(obj))
	})
}

func TestGetAPIVersion(t *testing.T) {
	t.Run("with group", func(t *testing.T) {
		obj := map[string]any{"apiVersion": "apps/v1"}
		assert.Equal(t, "v1", getAPIVersion(obj))
	})

	t.Run("core API", func(t *testing.T) {
		obj := map[string]any{"apiVersion": "v1"}
		assert.Equal(t, "v1", getAPIVersion(obj))
	})
}

// customChildRules compiles a configuration where a core ConfigMap is a child
// of a CRD root, a kind no built-in rule traverses to.
func customChildRules() *compiledRules {
	return compileRules(config.ResourceTreeConfig{Rules: []config.ResourceTreeRule{{
		Root: config.KindRef{Group: "example.com", Version: "v1", Kind: "Widget", Resource: "widgets"},
		Children: []config.ChildRule{
			{Kind: config.KindRef{Version: "v1", Kind: "ConfigMap", Resource: "configmaps"}},
		},
	}}})
}

// gatewayLabelSelectorRules compiles a labelSelector edge, to show attribution
// does not depend on how the child was matched.
func gatewayLabelSelectorRules() *compiledRules {
	return compileRules(config.ResourceTreeConfig{Rules: []config.ResourceTreeRule{{
		Root: config.KindRef{Group: "gateway.networking.k8s.io", Version: "v1", Kind: "Gateway", Resource: "gateways"},
		Children: []config.ChildRule{{
			Kind:    config.KindRef{Group: "apps", Version: "v1", Kind: "Deployment", Resource: "deployments"},
			Matcher: config.MatcherLabelSelector,
			LabelSelector: &config.LabelSelectorSpec{
				MatchLabels: map[string]string{"gateway.networking.k8s.io/gateway-name": config.TokenParentName},
			},
		}},
	}}})
}

func TestIsChildResourceKind(t *testing.T) {
	svc := &k8sResourcesService{rules: testRules()}

	t.Run("kinds the built-in rules reach through a parent", func(t *testing.T) {
		assert.True(t, svc.isChildResourceKind("", "Pod"))
		assert.True(t, svc.isChildResourceKind("apps", "ReplicaSet"))
		assert.True(t, svc.isChildResourceKind("batch", "Job"))
		assert.True(t, svc.isChildResourceKind("", "Secret"))
	})

	t.Run("roots are not children", func(t *testing.T) {
		assert.False(t, svc.isChildResourceKind("apps", "Deployment"))
		assert.False(t, svc.isChildResourceKind("batch", "CronJob"))
		assert.False(t, svc.isChildResourceKind("external-secrets.io", "ExternalSecret"))
	})

	t.Run("the lookup is group aware", func(t *testing.T) {
		// A CRD may reuse a built-in Kind name in its own group; it is a
		// different kind and is not reachable from any configured root.
		assert.False(t, svc.isChildResourceKind("apps", "Pod"))
		assert.False(t, svc.isChildResourceKind("", "ReplicaSet"))
		assert.False(t, svc.isChildResourceKind("", "Job"))
		assert.False(t, svc.isChildResourceKind("example.com", "Job"))
	})

	t.Run("a custom rule contributes its child kinds", func(t *testing.T) {
		assert.False(t, svc.isChildResourceKind("", "ConfigMap"), "no built-in rule traverses to a ConfigMap")

		custom := &k8sResourcesService{rules: customChildRules()}
		assert.True(t, custom.isChildResourceKind("", "ConfigMap"))
		// Configured rules replace the built-in ones rather than extending them.
		assert.False(t, custom.isChildResourceKind("", "Pod"))
	})

	t.Run("a labelSelector edge contributes the same as an ownerRef edge", func(t *testing.T) {
		labeled := &k8sResourcesService{rules: gatewayLabelSelectorRules()}
		assert.True(t, labeled.isChildResourceKind("apps", "Deployment"))
	})
}

func TestHasParentResourceInRelease(t *testing.T) {
	svc := &k8sResourcesService{rules: testRules()}

	t.Run("Pod with Deployment parent in resources", func(t *testing.T) {
		resources := []openchoreov1alpha1.RenderedManifestStatus{
			{Group: "", Kind: "Service", Name: "svc1"},
			{Group: "apps", Kind: "Deployment", Name: "dep1"},
		}
		assert.True(t, svc.hasParentResourceInRelease("", "Pod", resources))
	})

	t.Run("Pod with no parent kinds in resources", func(t *testing.T) {
		resources := []openchoreov1alpha1.RenderedManifestStatus{
			{Group: "", Kind: "Service", Name: "svc1"},
			{Group: "", Kind: "ConfigMap", Name: "cm1"},
		}
		assert.False(t, svc.hasParentResourceInRelease("", "Pod", resources))
	})

	t.Run("Job with CronJob parent in resources", func(t *testing.T) {
		resources := []openchoreov1alpha1.RenderedManifestStatus{
			{Group: "batch", Kind: "CronJob", Name: "cj1"},
		}
		assert.True(t, svc.hasParentResourceInRelease("batch", "Job", resources))
	})

	t.Run("ReplicaSet with Deployment parent in resources", func(t *testing.T) {
		resources := []openchoreov1alpha1.RenderedManifestStatus{
			{Group: "apps", Kind: "Deployment", Name: "dep1"},
		}
		assert.True(t, svc.hasParentResourceInRelease("apps", "ReplicaSet", resources))
	})

	t.Run("a parent in another group does not attribute the child", func(t *testing.T) {
		// A CRD named Deployment owns nothing the built-in rules know about.
		resources := []openchoreov1alpha1.RenderedManifestStatus{
			{Group: "example.com", Kind: "Deployment", Name: "dep1"},
		}
		assert.False(t, svc.hasParentResourceInRelease("", "Pod", resources))
	})

	t.Run("a child kind in another group has no parents", func(t *testing.T) {
		resources := []openchoreov1alpha1.RenderedManifestStatus{
			{Group: "apps", Kind: "Deployment", Name: "dep1"},
		}
		assert.False(t, svc.hasParentResourceInRelease("apps", "Pod", resources), "apps/Pod is not the core Pod")
	})

	t.Run("a kind that is not a child at all", func(t *testing.T) {
		resources := []openchoreov1alpha1.RenderedManifestStatus{
			{Group: "apps", Kind: "Deployment", Name: "dep1"},
		}
		assert.False(t, svc.hasParentResourceInRelease("apps", "StatefulSet", resources))
	})

	t.Run("a labelSelector edge attributes the same as an ownerRef edge", func(t *testing.T) {
		labeled := &k8sResourcesService{rules: gatewayLabelSelectorRules()}
		resources := []openchoreov1alpha1.RenderedManifestStatus{
			{Group: "gateway.networking.k8s.io", Kind: "Gateway", Name: "gw1"},
		}
		assert.True(t, labeled.hasParentResourceInRelease("apps", "Deployment", resources))
	})
}

func TestDeriveNamespace(t *testing.T) {
	t.Run("with resources", func(t *testing.T) {
		release := &openchoreov1alpha1.RenderedRelease{
			Status: openchoreov1alpha1.RenderedReleaseStatus{
				Resources: []openchoreov1alpha1.RenderedManifestStatus{
					{Namespace: "data-ns", Kind: "Deployment", Name: "dep1"},
					{Namespace: "data-ns", Kind: "Service", Name: "svc1"},
				},
			},
		}
		assert.Equal(t, "data-ns", deriveNamespace(release))
	})

	t.Run("empty resources", func(t *testing.T) {
		release := &openchoreov1alpha1.RenderedRelease{}
		assert.Equal(t, "", deriveNamespace(release))
	})
}

func TestResolveDataPlaneInfo(t *testing.T) {
	t.Run("namespace-scoped DataPlane", func(t *testing.T) {
		dpResult := &controller.DataPlaneResult{
			DataPlane: &openchoreov1alpha1.DataPlane{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "dp-cr",
					Namespace: "dp-ns",
				},
				Spec: openchoreov1alpha1.DataPlaneSpec{
					PlaneID: "my-plane-id",
				},
			},
		}

		pi := resolveDataPlaneInfo(dpResult)
		assert.Equal(t, planeTypeDataPlane, pi.planeType)
		assert.Equal(t, "my-plane-id", pi.planeID)
		assert.Equal(t, "dp-ns", pi.crNamespace)
		assert.Equal(t, "dp-cr", pi.crName)
	})

	t.Run("cluster-scoped ClusterDataPlane", func(t *testing.T) {
		dpResult := &controller.DataPlaneResult{
			ClusterDataPlane: &openchoreov1alpha1.ClusterDataPlane{
				ObjectMeta: metav1.ObjectMeta{
					Name: "cdp-cr",
				},
				Spec: openchoreov1alpha1.ClusterDataPlaneSpec{
					PlaneID: "cluster-plane-id",
				},
			},
		}

		pi := resolveDataPlaneInfo(dpResult)
		assert.Equal(t, planeTypeDataPlane, pi.planeType)
		assert.Equal(t, "cluster-plane-id", pi.planeID)
		assert.Equal(t, "_cluster", pi.crNamespace)
		assert.Equal(t, "cdp-cr", pi.crName)
	})

	t.Run("empty PlaneID falls back to Name", func(t *testing.T) {
		dpResult := &controller.DataPlaneResult{
			DataPlane: &openchoreov1alpha1.DataPlane{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "dp-fallback",
					Namespace: "dp-ns",
				},
				Spec: openchoreov1alpha1.DataPlaneSpec{},
			},
		}

		pi := resolveDataPlaneInfo(dpResult)
		assert.Equal(t, "dp-fallback", pi.planeID)
	})
}

func TestResolveObservabilityPlaneInfo(t *testing.T) {
	t.Run("namespace-scoped ObservabilityPlane", func(t *testing.T) {
		obsResult := &controller.ObservabilityPlaneResult{
			ObservabilityPlane: &openchoreov1alpha1.ObservabilityPlane{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "obs-cr",
					Namespace: "obs-ns",
				},
				Spec: openchoreov1alpha1.ObservabilityPlaneSpec{
					PlaneID: "obs-plane-id",
				},
			},
		}

		pi := resolveObservabilityPlaneInfo(obsResult)
		assert.Equal(t, planeTypeObservabilityPlane, pi.planeType)
		assert.Equal(t, "obs-plane-id", pi.planeID)
		assert.Equal(t, "obs-ns", pi.crNamespace)
		assert.Equal(t, "obs-cr", pi.crName)
	})

	t.Run("cluster-scoped ClusterObservabilityPlane", func(t *testing.T) {
		obsResult := &controller.ObservabilityPlaneResult{
			ClusterObservabilityPlane: &openchoreov1alpha1.ClusterObservabilityPlane{
				ObjectMeta: metav1.ObjectMeta{
					Name: "cops-cr",
				},
				Spec: openchoreov1alpha1.ClusterObservabilityPlaneSpec{
					PlaneID: "cluster-obs-id",
				},
			},
		}

		pi := resolveObservabilityPlaneInfo(obsResult)
		assert.Equal(t, planeTypeObservabilityPlane, pi.planeType)
		assert.Equal(t, "cluster-obs-id", pi.planeID)
		assert.Equal(t, "_cluster", pi.crNamespace)
		assert.Equal(t, "cops-cr", pi.crName)
	})
}

func TestComputeHealthFromObject(t *testing.T) {
	t.Run("healthy Deployment", func(t *testing.T) {
		replicas := int32(1)
		dep := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Generation: 1},
			Spec:       appsv1.DeploymentSpec{Replicas: &replicas},
			Status: appsv1.DeploymentStatus{
				ObservedGeneration:  1,
				Replicas:            1,
				UpdatedReplicas:     1,
				ReadyReplicas:       1,
				AvailableReplicas:   1,
				UnavailableReplicas: 0,
				Conditions: []appsv1.DeploymentCondition{
					{Type: appsv1.DeploymentAvailable, Status: corev1.ConditionTrue},
				},
			},
		}
		obj, err := runtime.DefaultUnstructuredConverter.ToUnstructured(dep)
		require.NoError(t, err)
		obj["apiVersion"] = "apps/v1"
		obj["kind"] = "Deployment"

		health := computeHealthFromObject(obj, "apps", "Deployment")
		require.NotNil(t, health)
		assert.Equal(t, "Healthy", health.Status)
		assert.Empty(t, health.Message)
	})

	t.Run("health check error returns Unknown with message", func(t *testing.T) {
		// A Deployment object with invalid structure triggers a conversion error
		obj := map[string]any{
			"apiVersion": "apps/v1",
			"kind":       "Deployment",
			"status": map[string]any{
				"replicas": "not-a-number",
			},
		}

		health := computeHealthFromObject(obj, "apps", "Deployment")
		require.NotNil(t, health)
		assert.Equal(t, "Unknown", health.Status)
		assert.NotEmpty(t, health.Message)
	})

	t.Run("unknown resource kind returns Healthy", func(t *testing.T) {
		obj := map[string]any{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata":   map[string]any{"name": "my-cm"},
		}

		health := computeHealthFromObject(obj, "", "ConfigMap")
		require.NotNil(t, health)
		assert.Equal(t, "Healthy", health.Status)
	})
}

func TestGetStringField(t *testing.T) {
	t.Run("existing key", func(t *testing.T) {
		obj := map[string]any{"kind": "Pod"}
		assert.Equal(t, "Pod", getStringField(obj, "kind"))
	})

	t.Run("missing key", func(t *testing.T) {
		obj := map[string]any{"kind": "Pod"}
		assert.Equal(t, "", getStringField(obj, "apiVersion"))
	})

	t.Run("non-string value", func(t *testing.T) {
		obj := map[string]any{"count": 42}
		assert.Equal(t, "", getStringField(obj, "count"))
	})
}

func TestSanitizeObject_EdgeCases(t *testing.T) {
	t.Run("no metadata key", func(t *testing.T) {
		obj := map[string]any{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
		}

		result := sanitizeObject(obj, configMapGVR)
		assert.Equal(t, "v1", result["apiVersion"])
		assert.Equal(t, "ConfigMap", result["kind"])
	})

	t.Run("non-Secret with data field preserved", func(t *testing.T) {
		obj := map[string]any{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata":   map[string]any{"name": "my-cm"},
			"data":       map[string]any{"key": "value"},
		}

		result := sanitizeObject(obj, configMapGVR)
		assert.Contains(t, result, "data")
		assert.Equal(t, map[string]any{"key": "value"}, result["data"])
	})
}

func TestResolveObservabilityPlaneInfo_EdgeCases(t *testing.T) {
	t.Run("empty PlaneID falls back to Name", func(t *testing.T) {
		obsResult := &controller.ObservabilityPlaneResult{
			ObservabilityPlane: &openchoreov1alpha1.ObservabilityPlane{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "obs-fallback",
					Namespace: "obs-ns",
				},
				Spec: openchoreov1alpha1.ObservabilityPlaneSpec{},
			},
		}

		pi := resolveObservabilityPlaneInfo(obsResult)
		assert.Equal(t, "obs-fallback", pi.planeID)
	})

	t.Run("cluster-scoped empty PlaneID falls back to Name", func(t *testing.T) {
		obsResult := &controller.ObservabilityPlaneResult{
			ClusterObservabilityPlane: &openchoreov1alpha1.ClusterObservabilityPlane{
				ObjectMeta: metav1.ObjectMeta{
					Name: "cops-fallback",
				},
				Spec: openchoreov1alpha1.ClusterObservabilityPlaneSpec{},
			},
		}

		pi := resolveObservabilityPlaneInfo(obsResult)
		assert.Equal(t, "cops-fallback", pi.planeID)
		assert.Equal(t, "_cluster", pi.crNamespace)
	})

	t.Run("both nil returns empty planeInfo", func(t *testing.T) {
		obsResult := &controller.ObservabilityPlaneResult{}
		pi := resolveObservabilityPlaneInfo(obsResult)
		assert.Equal(t, planeInfo{}, pi)
	})
}

func TestResolveDataPlaneInfo_BothNil(t *testing.T) {
	dpResult := &controller.DataPlaneResult{}
	pi := resolveDataPlaneInfo(dpResult)
	assert.Equal(t, planeInfo{}, pi)
}

func TestContextsForPlane(t *testing.T) {
	mk := func(plane string) releaseContext {
		return releaseContext{release: &openchoreov1alpha1.RenderedRelease{
			Spec: openchoreov1alpha1.RenderedReleaseSpec{TargetPlane: plane},
		}}
	}
	contexts := []releaseContext{
		mk(planeTypeObservabilityPlane),
		mk(planeTypeDataPlane),
		mk(planeTypeObservabilityPlane),
	}

	got := contextsForPlane(contexts, planeTypeDataPlane)
	require.Len(t, got, 1, "only the data-plane context must survive the filter")
	assert.Equal(t, planeTypeDataPlane, got[0].release.Spec.TargetPlane)

	assert.Empty(t, contextsForPlane(nil, planeTypeDataPlane))
}
