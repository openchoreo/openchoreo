// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package k8sresources

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/controller"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/models"
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

		node, ok := buildResourceNode(obj, nil, "Healthy")
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

		node, ok := buildResourceNode(obj, nil, "")
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

		node, ok := buildResourceNode(obj, parentRef, "")
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

		node, ok := buildResourceNode(obj, nil, openchoreov1alpha1.HealthStatusDegraded)
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

		result := sanitizeObject(obj, "ConfigMap")
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

		result := sanitizeObject(obj, "Secret")
		assert.NotContains(t, result, "data")
		assert.NotContains(t, result, "stringData")
		assert.Equal(t, "Opaque", result["type"])
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

func TestIsChildResourceKind(t *testing.T) {
	t.Run("Pod", func(t *testing.T) {
		assert.True(t, isChildResourceKind("Pod"))
	})

	t.Run("Deployment", func(t *testing.T) {
		assert.False(t, isChildResourceKind("Deployment"))
	})

	t.Run("Job", func(t *testing.T) {
		assert.True(t, isChildResourceKind("Job"))
	})
}

func TestHasParentResourceInRelease(t *testing.T) {
	t.Run("Pod with Deployment parent in resources", func(t *testing.T) {
		resources := []openchoreov1alpha1.ResourceStatus{
			{Kind: "Service", Name: "svc1"},
			{Kind: "Deployment", Name: "dep1"},
		}
		assert.True(t, hasParentResourceInRelease("Pod", resources))
	})

	t.Run("Pod with no parent kinds in resources", func(t *testing.T) {
		resources := []openchoreov1alpha1.ResourceStatus{
			{Kind: "Service", Name: "svc1"},
			{Kind: "ConfigMap", Name: "cm1"},
		}
		assert.False(t, hasParentResourceInRelease("Pod", resources))
	})

	t.Run("Job with CronJob parent in resources", func(t *testing.T) {
		resources := []openchoreov1alpha1.ResourceStatus{
			{Kind: "CronJob", Name: "cj1"},
		}
		assert.True(t, hasParentResourceInRelease("Job", resources))
	})

	t.Run("ReplicaSet with Deployment parent in resources", func(t *testing.T) {
		resources := []openchoreov1alpha1.ResourceStatus{
			{Kind: "Deployment", Name: "dep1"},
		}
		assert.True(t, hasParentResourceInRelease("ReplicaSet", resources))
	})
}

func TestDeriveNamespace(t *testing.T) {
	t.Run("with resources", func(t *testing.T) {
		release := &openchoreov1alpha1.RenderedRelease{
			Status: openchoreov1alpha1.RenderedReleaseStatus{
				Resources: []openchoreov1alpha1.ResourceStatus{
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

func TestFindResourceRelease(t *testing.T) {
	svc := &k8sResourcesService{}

	dpRelease := releaseContext{
		release: &openchoreov1alpha1.RenderedRelease{
			Spec: openchoreov1alpha1.RenderedReleaseSpec{TargetPlane: planeTypeDataPlane},
			Status: openchoreov1alpha1.RenderedReleaseStatus{
				Resources: []openchoreov1alpha1.ResourceStatus{
					{Group: "apps", Version: "v1", Kind: "Deployment", Name: "web", Namespace: "app-ns"},
					{Group: "", Version: "v1", Kind: "Service", Name: "web-svc", Namespace: "app-ns"},
				},
			},
		},
		namespace: "app-ns",
	}

	obsRelease := releaseContext{
		release: &openchoreov1alpha1.RenderedRelease{
			Spec: openchoreov1alpha1.RenderedReleaseSpec{TargetPlane: planeTypeObservabilityPlane},
			Status: openchoreov1alpha1.RenderedReleaseStatus{
				Resources: []openchoreov1alpha1.ResourceStatus{
					{Group: "", Version: "v1", Kind: "ConfigMap", Name: "obs-cfg", Namespace: "obs-ns"},
				},
			},
		},
		namespace: "obs-ns",
	}

	contexts := []releaseContext{dpRelease, obsRelease}

	t.Run("exact match returns resource namespace", func(t *testing.T) {
		rc, ns := svc.findResourceRelease(contexts, "apps", "v1", "Deployment", "web")
		require.NotNil(t, rc)
		assert.Equal(t, "app-ns", ns)
		assert.Equal(t, planeTypeDataPlane, rc.release.Spec.TargetPlane)
	})

	t.Run("exact match in second context", func(t *testing.T) {
		rc, ns := svc.findResourceRelease(contexts, "", "v1", "ConfigMap", "obs-cfg")
		require.NotNil(t, rc)
		assert.Equal(t, "obs-ns", ns)
		assert.Equal(t, planeTypeObservabilityPlane, rc.release.Spec.TargetPlane)
	})

	t.Run("child kind falls back to parent lookup", func(t *testing.T) {
		// Pod is not in status resources, but its parent kind Deployment is
		rc, ns := svc.findResourceRelease(contexts, "", "v1", "Pod", "web-abc-xyz")
		require.NotNil(t, rc)
		assert.Equal(t, "app-ns", ns)
	})

	t.Run("child kind with no parent in any context", func(t *testing.T) {
		// ReplicaSet's parent is Deployment — only dpRelease has it
		// But Job's parent is CronJob — neither context has a CronJob
		noParentContexts := []releaseContext{obsRelease}
		rc, ns := svc.findResourceRelease(noParentContexts, "", "v1", "Pod", "orphan-pod")
		assert.Nil(t, rc)
		assert.Equal(t, "", ns)
	})

	t.Run("Job child falls back to CronJob parent", func(t *testing.T) {
		cronJobCtx := releaseContext{
			release: &openchoreov1alpha1.RenderedRelease{
				Status: openchoreov1alpha1.RenderedReleaseStatus{
					Resources: []openchoreov1alpha1.ResourceStatus{
						{Group: "batch", Version: "v1", Kind: "CronJob", Name: "cj1", Namespace: "batch-ns"},
					},
				},
			},
			namespace: "batch-ns",
		}
		rc, ns := svc.findResourceRelease([]releaseContext{cronJobCtx}, "batch", "v1", "Job", "cj1-abc")
		require.NotNil(t, rc)
		assert.Equal(t, "batch-ns", ns)
	})

	t.Run("exact child match preferred over parent fallback", func(t *testing.T) {
		// Release has both a Deployment (parent) and a Pod listed directly with a different namespace
		mixedCtx := releaseContext{
			release: &openchoreov1alpha1.RenderedRelease{
				Status: openchoreov1alpha1.RenderedReleaseStatus{
					Resources: []openchoreov1alpha1.ResourceStatus{
						{Group: "apps", Version: "v1", Kind: "Deployment", Name: "web", Namespace: "app-ns"},
						{Group: "", Version: "v1", Kind: "Pod", Name: "special-pod", Namespace: "pod-ns"},
					},
				},
			},
			namespace: "app-ns",
		}
		// Exact match should return the Pod's own namespace ("pod-ns"), not the context namespace ("app-ns")
		rc, ns := svc.findResourceRelease([]releaseContext{mixedCtx}, "", "v1", "Pod", "special-pod")
		require.NotNil(t, rc)
		assert.Equal(t, "pod-ns", ns)
	})

	t.Run("non-child kind not found", func(t *testing.T) {
		rc, ns := svc.findResourceRelease(contexts, "apps", "v1", "StatefulSet", "missing")
		assert.Nil(t, rc)
		assert.Equal(t, "", ns)
	})

	t.Run("empty contexts", func(t *testing.T) {
		rc, ns := svc.findResourceRelease(nil, "apps", "v1", "Deployment", "web")
		assert.Nil(t, rc)
		assert.Equal(t, "", ns)
	})
}
