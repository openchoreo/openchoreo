// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package services

import (
	"context"
	"errors"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

func TestValidateReleaseLineOwner(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := openchoreov1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	component := func(name string, releaseLines bool) *openchoreov1alpha1.Component {
		return &openchoreov1alpha1.Component{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "ns"},
			Spec: openchoreov1alpha1.ComponentSpec{
				Owner:               openchoreov1alpha1.ComponentOwner{ProjectName: "proj"},
				ReleaseLinesEnabled: releaseLines,
			},
		}
	}
	line := func(name, componentName string) *openchoreov1alpha1.ReleaseLine {
		return &openchoreov1alpha1.ReleaseLine{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "ns"},
			Spec: openchoreov1alpha1.ReleaseLineSpec{
				Owner: openchoreov1alpha1.ReleaseLineOwner{ProjectName: "proj", ComponentName: componentName},
			},
		}
	}
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(
		component("plain", false),
		component("lined", true),
		component("other", true),
		line("next", "lined"),
		line("other-next", "other"),
	).Build()

	tests := []struct {
		name          string
		project       string
		component     string
		releaseLine   string
		wantRejection bool
	}{
		{name: "no component", component: ""},
		{name: "component not found", component: "missing", releaseLine: "next"},
		{name: "component without release lines", project: "proj", component: "plain"},
		{name: "line on a component without release lines", project: "proj", component: "plain", releaseLine: "next", wantRejection: true},
		{name: "component with release lines needs a line", project: "proj", component: "lined", wantRejection: true},
		{name: "line of the component", project: "proj", component: "lined", releaseLine: "next"},
		{name: "line without a project", component: "lined", releaseLine: "next"},
		{name: "line that doesn't exist", project: "proj", component: "lined", releaseLine: "ghost", wantRejection: true},
		{name: "line of another component", project: "proj", component: "lined", releaseLine: "other-next", wantRejection: true},
		{name: "line of another project", project: "elsewhere", component: "lined", releaseLine: "next", wantRejection: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateReleaseLineOwner(context.Background(), c, "ns", tt.project, tt.component, tt.releaseLine)
			var vErr *ValidationError
			switch {
			case tt.wantRejection && !errors.As(err, &vErr):
				t.Errorf("expected a validation error, got %v", err)
			case !tt.wantRejection && err != nil:
				t.Errorf("expected no error, got %v", err)
			}
		})
	}
}
