// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	dpkubernetes "github.com/openchoreo/openchoreo/internal/dataplane/kubernetes"
)

func TestObservedResourceID(t *testing.T) {
	t.Run("kind and name followed by a hash", func(t *testing.T) {
		assert.Regexp(t, `^deployment-payments-api-[0-9a-f]{8}$`,
			ObservedResourceID("apps/v1", "Deployment", "payments", "payments-api"))
		assert.Regexp(t, `^service-payments-api-[0-9a-f]{8}$`,
			ObservedResourceID("v1", "Service", "payments", "payments-api"))
	})

	t.Run("the same kind and name in two groups differ", func(t *testing.T) {
		knative := ObservedResourceID("serving.knative.dev/v1", "Service", "payments", "web")
		assert.NotEqual(t, ObservedResourceID("v1", "Service", "payments", "web"), knative)

		// A core Service named like the Knative id must not take that id.
		suffix := knative[strings.LastIndex(knative, "-")+1:]
		assert.NotEqual(t, knative, ObservedResourceID("v1", "Service", "payments", "web-"+suffix))
	})

	t.Run("one object under two API versions differs", func(t *testing.T) {
		assert.NotEqual(t,
			ObservedResourceID("autoscaling/v1", "HorizontalPodAutoscaler", "payments", "web"),
			ObservedResourceID("autoscaling/v2", "HorizontalPodAutoscaler", "payments", "web"))
	})

	t.Run("the namespace counts", func(t *testing.T) {
		assert.NotEqual(t,
			ObservedResourceID("v1", "Namespace", "", "payments"),
			ObservedResourceID("v1", "Namespace", "payments", "payments"))
	})

	t.Run("long ids are shortened and stay distinct", func(t *testing.T) {
		name := strings.Repeat("a", 100)
		id := ObservedResourceID("v1", "ConfigMap", "payments", name)
		assert.LessOrEqual(t, len(id), dpkubernetes.MaxLabelNameLength)
		assert.Equal(t, id, ObservedResourceID("v1", "ConfigMap", "payments", name))
		assert.NotEqual(t, id, ObservedResourceID("v1", "ConfigMap", "payments", name+"b"))
		assert.NotEqual(t, id, ObservedResourceID("example.com/v1", "ConfigMap", "payments", name))
	})
}

func TestObservedResources(t *testing.T) {
	claims := []openchoreov1alpha1.ObservedResourceClaim{
		{APIVersion: "apps/v1", Kind: "Deployment", Name: "api"},
		{APIVersion: "autoscaling/v1", Kind: "HorizontalPodAutoscaler", Name: "api"},
		{APIVersion: "autoscaling/v2", Kind: "HorizontalPodAutoscaler", Name: "api"},
	}

	resources := ObservedResources("payments", claims)

	assert.Len(t, resources, len(claims))
	ids := map[string]bool{}
	for i, r := range resources {
		assert.Equal(t, claims[i].APIVersion, r.APIVersion)
		assert.Equal(t, claims[i].Kind, r.Kind)
		assert.Equal(t, claims[i].Name, r.Name)
		assert.Equal(t, "payments", r.Namespace)
		assert.Equal(t, ObservedResourceID(r.APIVersion, r.Kind, "payments", r.Name), r.ID)
		ids[r.ID] = true
	}
	assert.Len(t, ids, len(claims), "every claim gets its own id")
}
