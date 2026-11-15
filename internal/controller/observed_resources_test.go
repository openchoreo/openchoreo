// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

func TestObservedResourceID(t *testing.T) {
	t.Run("core group is kind and name", func(t *testing.T) {
		assert.Equal(t, "service-payments-api", ObservedResourceID("v1", "Service", "payments-api"))
	})

	t.Run("other groups add a hash of the group", func(t *testing.T) {
		id := ObservedResourceID("apps/v1", "Deployment", "payments-api")
		assert.Regexp(t, `^deployment-payments-api-[0-9a-f]{8}$`, id)
	})

	t.Run("same kind and name in two groups differ", func(t *testing.T) {
		core := ObservedResourceID("v1", "Service", "web")
		knative := ObservedResourceID("serving.knative.dev/v1", "Service", "web")
		assert.NotEqual(t, core, knative)
	})

	t.Run("one object under two versions is the same id", func(t *testing.T) {
		assert.Equal(t,
			ObservedResourceID("autoscaling/v1", "HorizontalPodAutoscaler", "web"),
			ObservedResourceID("autoscaling/v2", "HorizontalPodAutoscaler", "web"))
	})

	t.Run("long ids are shortened and stay distinct", func(t *testing.T) {
		name := strings.Repeat("a", 100)
		for _, apiVersion := range []string{"v1", "apps/v1"} {
			id := ObservedResourceID(apiVersion, "ConfigMap", name)
			assert.LessOrEqual(t, len(id), 63, apiVersion)
			assert.Equal(t, id, ObservedResourceID(apiVersion, "ConfigMap", name), apiVersion)
		}
		assert.NotEqual(t,
			ObservedResourceID("v1", "ConfigMap", name),
			ObservedResourceID("v1", "ConfigMap", name+"b"))
		assert.NotEqual(t,
			ObservedResourceID("v1", "ConfigMap", name),
			ObservedResourceID("example.com/v1", "ConfigMap", name))
	})
}

func TestObservedResources(t *testing.T) {
	base := []openchoreov1alpha1.ObservedResource{{
		ID: ObservedResourceID("v1", "Namespace", "payments"), APIVersion: "v1", Kind: "Namespace", Name: "payments",
	}}
	claims := []openchoreov1alpha1.ObservedResourceClaim{
		{APIVersion: "apps/v1", Kind: "Deployment", Name: "api"},
		{APIVersion: "autoscaling/v1", Kind: "HorizontalPodAutoscaler", Name: "api"},
		{APIVersion: "autoscaling/v2", Kind: "HorizontalPodAutoscaler", Name: "api"},
		{APIVersion: "v1", Kind: "Namespace", Name: "payments"},
	}

	resources, skipped := ObservedResources(base, "payments", claims)

	assert.Equal(t, []openchoreov1alpha1.ObservedResource{
		base[0],
		{ID: ObservedResourceID("apps/v1", "Deployment", "api"), APIVersion: "apps/v1", Kind: "Deployment", Name: "api", Namespace: "payments"},
		{ID: ObservedResourceID("autoscaling/v1", "HorizontalPodAutoscaler", "api"), APIVersion: "autoscaling/v1", Kind: "HorizontalPodAutoscaler", Name: "api", Namespace: "payments"},
	}, resources)
	assert.Equal(t, claims[2:], skipped)
}
