// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package releasebinding

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"sigs.k8s.io/controller-runtime/pkg/event"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

func promotionPath(source string, targets ...string) openchoreov1alpha1.PromotionPath {
	path := openchoreov1alpha1.PromotionPath{
		SourceEnvironmentRef: openchoreov1alpha1.EnvironmentRef{Name: source},
	}
	for _, target := range targets {
		path.TargetEnvironmentRefs = append(path.TargetEnvironmentRefs, openchoreov1alpha1.TargetEnvironmentRef{Name: target})
	}
	return path
}

func TestPromotionSources(t *testing.T) {
	pipeline := &openchoreov1alpha1.DeploymentPipeline{
		Spec: openchoreov1alpha1.DeploymentPipelineSpec{
			PromotionPaths: []openchoreov1alpha1.PromotionPath{
				promotionPath("development", "staging", "qa"),
				promotionPath("staging", "production"),
				promotionPath("qa", "production"),
			},
		},
	}

	tests := []struct {
		name string
		env  string
		want []string
	}{
		{name: "root environment", env: "development", want: nil},
		{name: "single source", env: "staging", want: []string{"development"}},
		{name: "one of several targets", env: "qa", want: []string{"development"}},
		{name: "several sources", env: "production", want: []string{"staging", "qa"}},
		{name: "environment outside the pipeline", env: "sandbox", want: nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, promotionSources(pipeline, tt.env))
		})
	}

	assert.Nil(t, promotionSources(&openchoreov1alpha1.DeploymentPipeline{}, "staging"),
		"a pipeline without promotion paths has no promotion targets")
}

func TestReleaseNameChangedPredicate(t *testing.T) {
	p := releaseNameChangedPredicate()
	binding := func(release string) *openchoreov1alpha1.ReleaseBinding {
		return &openchoreov1alpha1.ReleaseBinding{Spec: openchoreov1alpha1.ReleaseBindingSpec{ReleaseName: release}}
	}

	assert.True(t, p.Create(event.CreateEvent{Object: binding("a")}))
	assert.True(t, p.Update(event.UpdateEvent{ObjectOld: binding("a"), ObjectNew: binding("b")}))
	assert.False(t, p.Update(event.UpdateEvent{ObjectOld: binding("a"), ObjectNew: binding("a")}))
	assert.False(t, p.Delete(event.DeleteEvent{Object: binding("a")}))
}
