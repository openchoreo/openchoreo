// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package releaseline

import (
	"testing"

	"github.com/stretchr/testify/assert"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

func TestIndexTraits(t *testing.T) {
	line := &openchoreov1alpha1.ReleaseLine{Spec: openchoreov1alpha1.ReleaseLineSpec{Traits: []openchoreov1alpha1.ComponentTrait{
		{Name: "alert", InstanceName: "a"},
		{Kind: openchoreov1alpha1.TraitRefKindClusterTrait, Name: "health-probes", InstanceName: "h"},
	}}}
	assert.Equal(t, []string{"Trait:alert", "ClusterTrait:health-probes"}, indexTraits(line))
}
