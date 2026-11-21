// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package bindingrevision

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

func bindingWith(labels, annotations map[string]string) *openchoreov1alpha1.ReleaseBinding {
	return &openchoreov1alpha1.ReleaseBinding{ObjectMeta: metav1.ObjectMeta{
		Name: "api-development", Namespace: "default", Labels: labels, Annotations: annotations,
	}}
}

func TestGitOpsManager(t *testing.T) {
	tests := []struct {
		name        string
		labels      map[string]string
		annotations map[string]string
		want        string
	}{
		{name: "no metadata: not managed by GitOps"},
		{
			name: "unrelated labels and annotations: not managed by GitOps",
			labels: map[string]string{
				"app.kubernetes.io/managed-by": "Helm",
				"app.kubernetes.io/instance":   "openchoreo",
				"openchoreo.dev/component":     "api",
			},
			annotations: map[string]string{
				openchoreov1alpha1.AnnotationKeyChangeMessage: "hotfix",
				"fluxcd.io/sync":                        "ts",
				"kustomize.toolkit.fluxcd.io/reconcile": "disabled",
			},
		},
		{
			name:   "Flux Kustomization labels",
			labels: map[string]string{labelKeyFluxKustomizationName: "apps", labelKeyFluxKustomizationNamespace: "flux-system"},
			want:   `Flux Kustomization "flux-system/apps"`,
		},
		{
			name:   "only the Flux Kustomization name label",
			labels: map[string]string{labelKeyFluxKustomizationName: "apps"},
			want:   "Flux",
		},
		{
			name:   "only the Flux Kustomization namespace label",
			labels: map[string]string{labelKeyFluxKustomizationNamespace: "flux-system"},
			want:   "Flux",
		},
		{
			name:   "empty Flux Kustomization labels",
			labels: map[string]string{labelKeyFluxKustomizationName: "", labelKeyFluxKustomizationNamespace: ""},
			want:   "Flux",
		},
		{
			name:   "Flux HelmRelease labels",
			labels: map[string]string{labelKeyFluxHelmReleaseName: "platform", labelKeyFluxHelmReleaseNamespace: "flux-system"},
			want:   `Flux HelmRelease "flux-system/platform"`,
		},
		{
			name:   "only the Flux HelmRelease namespace label",
			labels: map[string]string{labelKeyFluxHelmReleaseNamespace: "flux-system"},
			want:   "Flux",
		},
		{
			name:        "Flux tracking keys as annotations are not Flux's tracking labels",
			annotations: map[string]string{labelKeyFluxKustomizationName: "apps", labelKeyFluxKustomizationNamespace: "flux-system"},
		},
		{
			name:        "Argo CD tracking annotation",
			annotations: map[string]string{annotationKeyArgoCDTrackingID: "guestbook:openchoreo.dev/ReleaseBinding:default/api-development"},
			want:        `Argo CD application "guestbook"`,
		},
		{
			name:        "malformed Argo CD tracking annotation",
			annotations: map[string]string{annotationKeyArgoCDTrackingID: "not-a-tracking-id"},
			want:        "Argo CD",
		},
		{
			name:        "empty Argo CD tracking annotation",
			annotations: map[string]string{annotationKeyArgoCDTrackingID: ""},
			want:        "Argo CD",
		},
		{
			name:        "Flux labels win over an Argo CD annotation",
			labels:      map[string]string{labelKeyFluxKustomizationName: "apps", labelKeyFluxKustomizationNamespace: "flux-system"},
			annotations: map[string]string{annotationKeyArgoCDTrackingID: "guestbook:x"},
			want:        `Flux Kustomization "flux-system/apps"`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, GitOpsManager(bindingWith(tt.labels, tt.annotations)))
		})
	}
}

func TestRejectGitOpsManaged(t *testing.T) {
	t.Run("a binding not managed by GitOps is allowed", func(t *testing.T) {
		assert.NoError(t, RejectGitOpsManaged(bindingWith(map[string]string{"team": "payments"}, nil), "release binding"))
	})

	t.Run("a GitOps-managed binding is refused with what to do instead", func(t *testing.T) {
		err := RejectGitOpsManaged(bindingWith(map[string]string{
			labelKeyFluxKustomizationName: "apps", labelKeyFluxKustomizationNamespace: "flux-system",
		}, nil), "release binding")
		require.Error(t, err)
		assert.True(t, errors.Is(err, ErrGitOpsManaged))
		assert.Equal(t, `rollback blocked: the binding is managed by GitOps: release binding "api-development" is `+
			`applied from Git by Flux Kustomization "flux-system/apps". Rollback through OpenChoreo revision history `+
			`is not supported for GitOps-managed bindings, because GitOps would restore the configuration in Git on `+
			`its next sync. Revert the binding's configuration in Git and let GitOps reconcile the change`, err.Error())
	})

	t.Run("partial tracking metadata is refused, never taken as not managed", func(t *testing.T) {
		err := RejectGitOpsManaged(bindingWith(map[string]string{labelKeyFluxKustomizationName: ""}, nil),
			"resource release binding")
		require.ErrorIs(t, err, ErrGitOpsManaged)
		assert.ErrorContains(t, err, `resource release binding "api-development" is applied from Git by Flux.`)
	})
}
