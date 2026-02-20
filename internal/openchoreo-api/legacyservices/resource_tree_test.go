// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package legacyservices

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	gatewayClient "github.com/openchoreo/openchoreo/internal/clients/gateway"
	"github.com/openchoreo/openchoreo/internal/labels"
)

func TestGetReleaseResourceTree(t *testing.T) {
	const (
		namespace   = "test-ns"
		project     = "test-project"
		component   = "test-component"
		environment = "test-env"
		dpName      = "default"
	)

	// newComponent creates a Component object for tests.
	newComponent := func(ns, name, projectName string) *openchoreov1alpha1.Component {
		return &openchoreov1alpha1.Component{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: ns,
				Name:      name,
			},
			Spec: openchoreov1alpha1.ComponentSpec{
				Owner: openchoreov1alpha1.ComponentOwner{
					ProjectName: projectName,
				},
			},
		}
	}

	// newRelease creates a Release with given resource statuses.
	newRelease := func(
		ns, proj, comp, env string,
		resources []openchoreov1alpha1.ResourceStatus,
	) *openchoreov1alpha1.Release {
		return &openchoreov1alpha1.Release{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: ns,
				Name:      comp + "-" + env + "-release",
				Labels: map[string]string{
					labels.LabelKeyNamespaceName:   ns,
					labels.LabelKeyProjectName:     proj,
					labels.LabelKeyComponentName:   comp,
					labels.LabelKeyEnvironmentName: env,
				},
			},
			Status: openchoreov1alpha1.ReleaseStatus{
				Resources: resources,
			},
		}
	}

	// newEnvironment creates an Environment that references a DataPlane.
	newEnvironment := func(ns, name string) *openchoreov1alpha1.Environment {
		return &openchoreov1alpha1.Environment{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: ns,
				Name:      name,
			},
			Spec: openchoreov1alpha1.EnvironmentSpec{
				DataPlaneRef: &openchoreov1alpha1.DataPlaneRef{
					Kind: openchoreov1alpha1.DataPlaneRefKindDataPlane,
					Name: dpName,
				},
			},
		}
	}

	// newDataPlane creates a DataPlane object.
	newDataPlane := func(ns, name string) *openchoreov1alpha1.DataPlane {
		return &openchoreov1alpha1.DataPlane{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: ns,
				Name:      name,
			},
			Spec: openchoreov1alpha1.DataPlaneSpec{
				PlaneID: name,
			},
		}
	}

	// newGatewayServer creates an httptest server that returns a JSON object
	// for any request, simulating the gateway proxy.
	newGatewayServer := func(obj map[string]any) *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(obj)
		}))
	}

	t.Run("success with single resource", func(t *testing.T) {
		liveObj := map[string]any{
			"apiVersion": "apps/v1",
			"kind":       "Deployment",
			"metadata": map[string]any{
				"namespace":       "workload-ns",
				"name":            "my-deploy",
				"uid":             "uid-1",
				"resourceVersion": "100",
			},
		}
		server := newGatewayServer(liveObj)
		defer server.Close()

		resources := []openchoreov1alpha1.ResourceStatus{
			{
				Group:        "apps",
				Version:      "v1",
				Kind:         "Deployment",
				Namespace:    "workload-ns",
				Name:         "my-deploy",
				HealthStatus: openchoreov1alpha1.HealthStatusHealthy,
			},
		}

		k8s := fake.NewClientBuilder().
			WithScheme(newTestScheme(t)).
			WithRESTMapper(newTestRESTMapper()).
			WithObjects(
				newComponent(namespace, component, project),
				newRelease(namespace, project, component, environment, resources),
				newEnvironment(namespace, environment),
				newDataPlane(namespace, dpName),
			).
			Build()

		// For child resource fetch (ReplicaSets), return empty list
		mux := http.NewServeMux()
		mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			// If this is a list request (no specific resource name at end),
			// return an empty list; otherwise return the live object.
			_ = json.NewEncoder(w).Encode(map[string]any{
				"apiVersion": "apps/v1",
				"kind":       "Deployment",
				"metadata": map[string]any{
					"namespace":       "workload-ns",
					"name":            "my-deploy",
					"uid":             "uid-1",
					"resourceVersion": "100",
				},
				"items": []any{},
			})
		})
		serverMux := httptest.NewServer(mux)
		defer serverMux.Close()

		svc := &ComponentService{
			k8sClient:     k8s,
			logger:        slog.Default(),
			authzPDP:      &allowAllPDP{},
			gatewayClient: gatewayClient.NewClient(serverMux.URL),
		}

		resp, err := svc.GetReleaseResourceTree(
			context.Background(), namespace, project, component, environment,
		)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp == nil {
			t.Fatal("expected non-nil response")
		}
		if len(resp.Nodes) == 0 {
			t.Fatal("expected at least one node")
		}
		node := resp.Nodes[0]
		if node.Kind != "Deployment" {
			t.Errorf("expected kind Deployment, got %s", node.Kind)
		}
		if node.Name != "my-deploy" {
			t.Errorf("expected name my-deploy, got %s", node.Name)
		}
		if node.Health == nil || node.Health.Status != string(openchoreov1alpha1.HealthStatusHealthy) {
			t.Errorf("expected health status Healthy, got %v", node.Health)
		}
	})

	t.Run("component not found returns ErrComponentNotFound", func(t *testing.T) {
		k8s := fake.NewClientBuilder().
			WithScheme(newTestScheme(t)).
			Build()

		svc := &ComponentService{
			k8sClient:     k8s,
			logger:        slog.Default(),
			authzPDP:      &allowAllPDP{},
			gatewayClient: gatewayClient.NewClient("http://unused"),
		}

		_, err := svc.GetReleaseResourceTree(
			context.Background(), namespace, project, component, environment,
		)
		if !errors.Is(err, ErrComponentNotFound) {
			t.Errorf("expected ErrComponentNotFound, got %v", err)
		}
	})

	t.Run("project name mismatch returns ErrComponentNotFound", func(t *testing.T) {
		// Component exists but belongs to a different project.
		k8s := fake.NewClientBuilder().
			WithScheme(newTestScheme(t)).
			WithObjects(
				newComponent(namespace, component, "other-project"),
			).
			Build()

		svc := &ComponentService{
			k8sClient:     k8s,
			logger:        slog.Default(),
			authzPDP:      &allowAllPDP{},
			gatewayClient: gatewayClient.NewClient("http://unused"),
		}

		_, err := svc.GetReleaseResourceTree(
			context.Background(), namespace, project, component, environment,
		)
		if !errors.Is(err, ErrComponentNotFound) {
			t.Errorf("expected ErrComponentNotFound, got %v", err)
		}
	})

	t.Run("empty release list returns ErrReleaseNotFound", func(t *testing.T) {
		k8s := fake.NewClientBuilder().
			WithScheme(newTestScheme(t)).
			WithObjects(
				newComponent(namespace, component, project),
				// No releases created.
			).
			Build()

		svc := &ComponentService{
			k8sClient:     k8s,
			logger:        slog.Default(),
			authzPDP:      &allowAllPDP{},
			gatewayClient: gatewayClient.NewClient("http://unused"),
		}

		_, err := svc.GetReleaseResourceTree(
			context.Background(), namespace, project, component, environment,
		)
		if !errors.Is(err, ErrReleaseNotFound) {
			t.Errorf("expected ErrReleaseNotFound, got %v", err)
		}
	})

	t.Run("environment not found returns ErrEnvironmentNotFound", func(t *testing.T) {
		resources := []openchoreov1alpha1.ResourceStatus{
			{
				Group:   "apps",
				Version: "v1",
				Kind:    "Deployment",
				Name:    "my-deploy",
			},
		}

		k8s := fake.NewClientBuilder().
			WithScheme(newTestScheme(t)).
			WithObjects(
				newComponent(namespace, component, project),
				newRelease(namespace, project, component, environment, resources),
				// No environment created.
			).
			Build()

		svc := &ComponentService{
			k8sClient:     k8s,
			logger:        slog.Default(),
			authzPDP:      &allowAllPDP{},
			gatewayClient: gatewayClient.NewClient("http://unused"),
		}

		_, err := svc.GetReleaseResourceTree(
			context.Background(), namespace, project, component, environment,
		)
		if !errors.Is(err, ErrEnvironmentNotFound) {
			t.Errorf("expected ErrEnvironmentNotFound, got %v", err)
		}
	})

	t.Run("fetchLiveResource error skips resource", func(t *testing.T) {
		resources := []openchoreov1alpha1.ResourceStatus{
			{
				Group:     "apps",
				Version:   "v1",
				Kind:      "Deployment",
				Namespace: "workload-ns",
				Name:      "deploy-ok",
			},
			{
				Group:     "apps",
				Version:   "v1",
				Kind:      "Deployment",
				Namespace: "workload-ns",
				Name:      "deploy-fail",
			},
		}

		// Gateway server that returns 200 for deploy-ok and 404 for deploy-fail.
		callCount := 0
		server := httptest.NewServer(
			http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				callCount++
				w.Header().Set("Content-Type", "application/json")
				if callCount == 1 {
					// First call: live resource fetch for deploy-ok
					_ = json.NewEncoder(w).Encode(map[string]any{
						"apiVersion": "apps/v1",
						"kind":       "Deployment",
						"metadata": map[string]any{
							"namespace":       "workload-ns",
							"name":            "deploy-ok",
							"uid":             "uid-ok",
							"resourceVersion": "1",
						},
					})
				} else if callCount == 2 {
					// Second call: child resources (ReplicaSets) for deploy-ok
					_ = json.NewEncoder(w).Encode(map[string]any{
						"items": []any{},
					})
				} else if callCount == 3 {
					// Third call: live resource fetch for deploy-fail -> 404
					w.WriteHeader(http.StatusNotFound)
					_, _ = w.Write([]byte(`{"kind":"Status","code":404}`))
				}
			}),
		)
		defer server.Close()

		k8s := fake.NewClientBuilder().
			WithScheme(newTestScheme(t)).
			WithRESTMapper(newTestRESTMapper()).
			WithObjects(
				newComponent(namespace, component, project),
				newRelease(namespace, project, component, environment, resources),
				newEnvironment(namespace, environment),
				newDataPlane(namespace, dpName),
			).
			Build()

		svc := &ComponentService{
			k8sClient:     k8s,
			logger:        slog.Default(),
			authzPDP:      &allowAllPDP{},
			gatewayClient: gatewayClient.NewClient(server.URL),
		}

		resp, err := svc.GetReleaseResourceTree(
			context.Background(), namespace, project, component, environment,
		)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// Only the first resource (deploy-ok) should be in the tree.
		// The second (deploy-fail) should have been skipped.
		found := false
		for _, n := range resp.Nodes {
			if n.Name == "deploy-fail" {
				t.Error("deploy-fail should have been skipped")
			}
			if n.Name == "deploy-ok" {
				found = true
			}
		}
		if !found {
			t.Error("expected deploy-ok node in response")
		}
	})

	t.Run("authz denial returns ErrForbidden", func(t *testing.T) {
		k8s := fake.NewClientBuilder().
			WithScheme(newTestScheme(t)).
			Build()

		svc := &ComponentService{
			k8sClient:     k8s,
			logger:        slog.Default(),
			authzPDP:      &denyAllPDP{},
			gatewayClient: gatewayClient.NewClient("http://unused"),
		}

		_, err := svc.GetReleaseResourceTree(
			context.Background(), namespace, project, component, environment,
		)
		if !errors.Is(err, ErrForbidden) {
			t.Errorf("expected ErrForbidden, got %v", err)
		}
	})

	t.Run("nil gateway client returns error", func(t *testing.T) {
		k8s := fake.NewClientBuilder().
			WithScheme(newTestScheme(t)).
			WithObjects(
				newComponent(namespace, component, project),
			).
			Build()

		svc := &ComponentService{
			k8sClient:     k8s,
			logger:        slog.Default(),
			authzPDP:      &allowAllPDP{},
			gatewayClient: nil,
		}

		_, err := svc.GetReleaseResourceTree(
			context.Background(), namespace, project, component, environment,
		)
		if err == nil {
			t.Fatal("expected error for nil gateway client")
		}
	})
}
