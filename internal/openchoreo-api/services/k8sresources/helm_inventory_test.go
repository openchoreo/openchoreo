// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package k8sresources

import (
	"testing"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"k8s.io/apimachinery/pkg/runtime"
)

func TestParseFluxHelmInventoryEntryID(t *testing.T) {
	t.Parallel()

	deployment := parseFluxHelmInventoryEntryID("obt-dev_smollm2_apps_Deployment")
	if deployment == nil {
		t.Fatal("expected deployment parse")
	}
	if deployment.Namespace != "obt-dev" || deployment.Name != "smollm2" || deployment.Group != "apps" {
		t.Fatalf("unexpected deployment: %+v", deployment)
	}

	pvc := parseFluxHelmInventoryEntryID("obt-dev_smollm2-model-storage__PersistentVolumeClaim")
	if pvc == nil {
		t.Fatal("expected pvc parse")
	}
	if pvc.Namespace != "obt-dev" || pvc.Name != "smollm2-model-storage" {
		t.Fatalf("unexpected pvc: %+v", pvc)
	}
}

func TestHelmManifestStatusHasInventoryKind(t *testing.T) {
	t.Parallel()

	entry := &openchoreov1alpha1.RenderedManifestStatus{
		Kind: "HelmRelease",
		Status: &runtime.RawExtension{
			Raw: []byte(`{"inventory":{"entries":[{"id":"obt-dev_smollm2_apps_Deployment"}]}}`),
		},
	}
	if !helmManifestStatusHasInventoryKind(entry, "Deployment") {
		t.Fatal("expected Deployment in inventory")
	}
	if helmManifestStatusHasInventoryKind(entry, "Pod") {
		t.Fatal("did not expect Pod in inventory")
	}
}

func TestHelmInventoryResourceMatch(t *testing.T) {
	t.Parallel()

	entry := &openchoreov1alpha1.RenderedManifestStatus{
		Kind: "HelmRelease",
		Status: &runtime.RawExtension{
			Raw: []byte(`{"inventory":{"entries":[{"id":"obt-dev_smollm2_apps_Deployment"}]}}`),
		},
	}
	ns, ok := helmInventoryResourceMatch(entry, "apps", "v1", "Deployment", "smollm2")
	if !ok || ns != "obt-dev" {
		t.Fatalf("expected match in obt-dev, got ok=%v ns=%q", ok, ns)
	}
}

func TestHelmReleaseResourceRef(t *testing.T) {
	t.Parallel()

	ref := helmReleaseResourceRef(map[string]any{
		"apiVersion": "helm.toolkit.fluxcd.io/v2",
		"kind":       "HelmRelease",
		"metadata": map[string]any{
			"name":      "smollm2",
			"namespace": "obt-dev",
			"uid":       "hr-uid-1",
		},
	})
	if ref == nil {
		t.Fatal("expected parent ref")
	}
	if ref.Kind != "HelmRelease" || ref.Name != "smollm2" || ref.UID != "hr-uid-1" {
		t.Fatalf("unexpected ref: %+v", ref)
	}
	if ref.Group != "helm.toolkit.fluxcd.io" || ref.Version != "v2" {
		t.Fatalf("unexpected GVK: %+v", ref)
	}
	if helmReleaseResourceRef(map[string]any{"metadata": map[string]any{"name": "x"}}) != nil {
		t.Fatal("expected nil without uid")
	}
}
