// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package releaseline

import (
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

func TestLineEnvironments(t *testing.T) {
	rb := func(name, project, line, env, release string, ready bool) openchoreov1alpha1.ReleaseBinding {
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
	bindings := []openchoreov1alpha1.ReleaseBinding{
		rb("hello-next-staging", "p", "next", "staging", "r2", false),
		rb("hello-next-development", "p", "next", "development", "r3", true),
		rb("hello-main-development", "p", "main", "development", "r1", true),
		rb("hello-next-development-other-project", "other", "next", "development", "r9", true),
	}

	assert.Equal(t, []openchoreov1alpha1.ReleaseLineEnvironment{
		{Environment: "development", ReleaseBinding: "hello-next-development", Release: "r3", Ready: true},
		{Environment: "staging", ReleaseBinding: "hello-next-staging", Release: "r2"},
	}, lineEnvironments(bindings, "p", "next"))
	assert.Empty(t, lineEnvironments(bindings, "p", "v3"))
}

func TestIndexTraits(t *testing.T) {
	line := &openchoreov1alpha1.ReleaseLine{Spec: openchoreov1alpha1.ReleaseLineSpec{Traits: []openchoreov1alpha1.ComponentTrait{
		{Name: "alert", InstanceName: "a"},
		{Kind: openchoreov1alpha1.TraitRefKindClusterTrait, Name: "health-probes", InstanceName: "h"},
	}}}
	assert.Equal(t, []string{"Trait:alert", "ClusterTrait:health-probes"}, indexTraits(line))
}
