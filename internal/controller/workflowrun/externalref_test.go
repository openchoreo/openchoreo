// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package workflowrun

import (
	"strings"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	openchoreodevv1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

func TestResolveExternalRefs(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := openchoreodevv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add scheme: %v", err)
	}

	secretRef := &openchoreodevv1alpha1.SecretReference{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "repo-git-secret",
			Namespace: "default",
		},
		Spec: openchoreodevv1alpha1.SecretReferenceSpec{
			Template: openchoreodevv1alpha1.SecretTemplate{
				Type: "kubernetes.io/basic-auth",
			},
			Data: []openchoreodevv1alpha1.SecretDataSource{
				{
					SecretKey: "username",
					RemoteRef: openchoreodevv1alpha1.RemoteReference{
						Key:      "secret/data/repo-creds",
						Property: "username",
					},
				},
			},
		},
	}

	pushSecretRef := &openchoreodevv1alpha1.SecretReference{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "registry-push-secret",
			Namespace: "default",
		},
		Spec: openchoreodevv1alpha1.SecretReferenceSpec{
			Template: openchoreodevv1alpha1.SecretTemplate{
				Type: "kubernetes.io/dockerconfigjson",
			},
			Data: []openchoreodevv1alpha1.SecretDataSource{
				{
					SecretKey: "config",
					RemoteRef: openchoreodevv1alpha1.RemoteReference{
						Key: "secret/data/registry",
					},
				},
			},
		},
	}

	celContext := map[string]any{
		"metadata": map[string]any{
			"namespaceName":   "default",
			"workflowRunName": "test-run",
		},
		"parameters": map[string]any{
			"repository": map[string]any{
				"secretRef": "repo-git-secret",
			},
			"registry": map[string]any{
				"secretRef": "registry-push-secret",
			},
		},
	}

	t.Run("resolves single SecretReference externalRef", func(t *testing.T) {
		reconciler := &Reconciler{
			Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(secretRef).Build(),
		}

		refs := []openchoreodevv1alpha1.ExternalRef{
			{
				ID:         "git-secret-reference",
				APIVersion: "openchoreo.dev/v1alpha1",
				Kind:       "SecretReference",
				Name:       "${parameters.repository.secretRef}",
			},
		}

		result, err := reconciler.resolveExternalRefs(t.Context(), refs, celContext, "default")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(result) != 1 {
			t.Fatalf("expected 1 result, got %d", len(result))
		}

		refData, ok := result["git-secret-reference"].(map[string]any)
		if !ok {
			t.Fatalf("expected map for git-secret-reference, got %T", result["git-secret-reference"])
		}

		spec, ok := refData["spec"].(map[string]any)
		if !ok {
			t.Fatalf("expected spec map, got %T", refData["spec"])
		}

		template, ok := spec["template"].(map[string]any)
		if !ok {
			t.Fatalf("expected template map, got %T", spec["template"])
		}

		if template["type"] != "kubernetes.io/basic-auth" {
			t.Errorf("expected type kubernetes.io/basic-auth, got %v", template["type"])
		}
	})

	t.Run("resolves multiple externalRefs", func(t *testing.T) {
		reconciler := &Reconciler{
			Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(secretRef, pushSecretRef).Build(),
		}

		refs := []openchoreodevv1alpha1.ExternalRef{
			{
				ID:         "git-secret-reference",
				APIVersion: "openchoreo.dev/v1alpha1",
				Kind:       "SecretReference",
				Name:       "${parameters.repository.secretRef}",
			},
			{
				ID:         "push-secret-reference",
				APIVersion: "openchoreo.dev/v1alpha1",
				Kind:       "SecretReference",
				Name:       "${parameters.registry.secretRef}",
			},
		}

		result, err := reconciler.resolveExternalRefs(t.Context(), refs, celContext, "default")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(result) != 2 {
			t.Fatalf("expected 2 results, got %d", len(result))
		}

		if _, ok := result["git-secret-reference"]; !ok {
			t.Error("expected git-secret-reference in result")
		}
		if _, ok := result["push-secret-reference"]; !ok {
			t.Error("expected push-secret-reference in result")
		}
	})

	t.Run("skips externalRef when name evaluates to empty", func(t *testing.T) {
		reconciler := &Reconciler{
			Client: fake.NewClientBuilder().WithScheme(scheme).Build(),
		}

		emptyContext := map[string]any{
			"metadata": map[string]any{},
			"parameters": map[string]any{
				"repository": map[string]any{
					"secretRef": "",
				},
			},
		}

		refs := []openchoreodevv1alpha1.ExternalRef{
			{
				ID:         "git-secret-reference",
				APIVersion: "openchoreo.dev/v1alpha1",
				Kind:       "SecretReference",
				Name:       "${parameters.repository.secretRef}",
			},
		}

		result, err := reconciler.resolveExternalRefs(t.Context(), refs, emptyContext, "default")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(result) != 0 {
			t.Fatalf("expected 0 results for empty name, got %d", len(result))
		}
	})

	t.Run("returns error when SecretReference not found", func(t *testing.T) {
		reconciler := &Reconciler{
			Client: fake.NewClientBuilder().WithScheme(scheme).Build(),
		}

		refs := []openchoreodevv1alpha1.ExternalRef{
			{
				ID:         "git-secret-reference",
				APIVersion: "openchoreo.dev/v1alpha1",
				Kind:       "SecretReference",
				Name:       "${parameters.repository.secretRef}",
			},
		}

		_, err := reconciler.resolveExternalRefs(t.Context(), refs, celContext, "default")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to get SecretReference") {
			t.Fatalf("expected get SecretReference error, got %v", err)
		}
	})

	t.Run("returns error for unsupported kind", func(t *testing.T) {
		reconciler := &Reconciler{
			Client: fake.NewClientBuilder().WithScheme(scheme).Build(),
		}

		refs := []openchoreodevv1alpha1.ExternalRef{
			{
				ID:         "some-ref",
				APIVersion: "v1",
				Kind:       "ConfigMap",
				Name:       "my-config",
			},
		}

		// Use a context where the name is a literal (no CEL expression)
		literalContext := map[string]any{
			"metadata":   map[string]any{},
			"parameters": map[string]any{},
		}

		_, err := reconciler.resolveExternalRefs(t.Context(), refs, literalContext, "default")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "unsupported externalRef kind") {
			t.Fatalf("expected unsupported kind error, got %v", err)
		}
	})

	t.Run("returns nil for empty externalRefs", func(t *testing.T) {
		reconciler := &Reconciler{
			Client: fake.NewClientBuilder().WithScheme(scheme).Build(),
		}

		result, err := reconciler.resolveExternalRefs(t.Context(), nil, celContext, "default")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result != nil {
			t.Fatalf("expected nil result for empty externalRefs, got %v", result)
		}
	})

	t.Run("exposes entire spec including data array", func(t *testing.T) {
		reconciler := &Reconciler{
			Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(secretRef).Build(),
		}

		refs := []openchoreodevv1alpha1.ExternalRef{
			{
				ID:         "git-secret-reference",
				APIVersion: "openchoreo.dev/v1alpha1",
				Kind:       "SecretReference",
				Name:       "${parameters.repository.secretRef}",
			},
		}

		result, err := reconciler.resolveExternalRefs(t.Context(), refs, celContext, "default")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		refData := result["git-secret-reference"].(map[string]any)
		spec := refData["spec"].(map[string]any)

		// Verify data array is present
		data, ok := spec["data"].([]any)
		if !ok {
			t.Fatalf("expected data array, got %T", spec["data"])
		}
		if len(data) != 1 {
			t.Fatalf("expected 1 data entry, got %d", len(data))
		}

		dataEntry := data[0].(map[string]any)
		if dataEntry["secretKey"] != "username" {
			t.Errorf("expected secretKey 'username', got %v", dataEntry["secretKey"])
		}

		remoteRef := dataEntry["remoteRef"].(map[string]any)
		if remoteRef["key"] != "secret/data/repo-creds" {
			t.Errorf("expected remote key 'secret/data/repo-creds', got %v", remoteRef["key"])
		}
	})
}
