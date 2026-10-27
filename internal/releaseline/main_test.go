// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package releaseline

import (
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

func TestNormalize(t *testing.T) {
	assert.Equal(t, "", Normalize(""))
	assert.Equal(t, "", Normalize("main"))
	assert.Equal(t, "", Normalize(" main "))
	assert.Equal(t, "next", Normalize("next"))
	assert.Equal(t, "default", Normalize("default"))
}

func binding(name, project, line, env, release string, ready bool) openchoreov1alpha1.ReleaseBinding {
	b := openchoreov1alpha1.ReleaseBinding{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec: openchoreov1alpha1.ReleaseBindingSpec{
			Owner:       openchoreov1alpha1.ReleaseBindingOwner{ProjectName: project, ComponentName: "hello", ReleaseLine: line},
			Environment: env,
			ReleaseName: release,
		},
	}
	if ready {
		b.Status.Conditions = []metav1.Condition{{Type: "Ready", Status: metav1.ConditionTrue}}
	}
	return b
}

func TestEnvironments(t *testing.T) {
	bindings := []openchoreov1alpha1.ReleaseBinding{
		binding("hello-next-staging", "p", "next", "staging", "r2", false),
		binding("hello-next-development", "p", "next", "development", "r3", true),
		binding("hello-development", "p", "", "development", "r1", true),
		binding("hello-next-development-other-project", "other", "next", "development", "r9", true),
	}

	assert.Equal(t, []openchoreov1alpha1.ReleaseLineEnvironment{
		{Environment: "development", ReleaseBinding: "hello-next-development", Release: "r3", Ready: true},
		{Environment: "staging", ReleaseBinding: "hello-next-staging", Release: "r2"},
	}, Environments(bindings, "p", "hello", "next"))
	assert.Equal(t, []openchoreov1alpha1.ReleaseLineEnvironment{
		{Environment: "development", ReleaseBinding: "hello-development", Release: "r1", Ready: true},
	}, Environments(bindings, "p", "hello", ""))
	assert.Empty(t, Environments(bindings, "p", "hello", "v3"))
}

func TestMainLine(t *testing.T) {
	comp := &openchoreov1alpha1.Component{
		ObjectMeta: metav1.ObjectMeta{Name: "hello", Namespace: "default"},
		Spec:       openchoreov1alpha1.ComponentSpec{Owner: openchoreov1alpha1.ComponentOwner{ProjectName: "p"}},
		Status: openchoreov1alpha1.ComponentStatus{
			LatestRelease: &openchoreov1alpha1.LatestRelease{Name: "hello-r1"},
		},
	}
	bindings := []openchoreov1alpha1.ReleaseBinding{
		binding("hello-development", "p", "", "development", "hello-r1", true),
		binding("hello-next-development", "p", "next", "development", "hello-next-r1", true),
	}

	line := Main(comp, bindings)
	assert.Equal(t, MainLine, line.Name)
	assert.Equal(t, "default", line.Namespace)
	assert.Equal(t, openchoreov1alpha1.ReleaseLineOwner{ProjectName: "p", ComponentName: "hello"}, line.Spec.Owner)
	assert.Equal(t, "hello-r1", line.Status.LatestRelease.Name)
	assert.Equal(t, []openchoreov1alpha1.ReleaseLineEnvironment{
		{Environment: "development", ReleaseBinding: "hello-development", Release: "hello-r1", Ready: true},
	}, line.Status.Environments)
}
