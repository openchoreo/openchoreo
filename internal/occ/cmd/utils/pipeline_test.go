// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
)

func makePipeline(paths ...gen.PromotionPath) *gen.DeploymentPipeline {
	return &gen.DeploymentPipeline{
		Metadata: gen.ObjectMeta{Name: "test-pipeline"},
		Spec:     &gen.DeploymentPipelineSpec{PromotionPaths: &paths},
	}
}

func promotionPath(source string, targets ...string) gen.PromotionPath {
	refs := make([]gen.TargetEnvironmentRef, len(targets))
	for i, target := range targets {
		refs[i] = gen.TargetEnvironmentRef{Name: target}
	}
	pp := gen.PromotionPath{TargetEnvironmentRefs: refs}
	pp.SourceEnvironmentRef.Name = source
	return pp
}

func TestExpandEnvironments_Linear(t *testing.T) {
	envs := ExpandEnvironments(makePipeline(
		promotionPath("dev", "staging"),
		promotionPath("staging", "prod"),
	))
	// staging appears as both a target and a source; it must not be duplicated.
	assert.Equal(t, []string{"dev", "staging", "prod"}, envs)
}

func TestExpandEnvironments_Diamond(t *testing.T) {
	envs := ExpandEnvironments(makePipeline(
		promotionPath("dev", "staging-a", "staging-b"),
		promotionPath("staging-a", "prod"),
		promotionPath("staging-b", "prod"),
	))
	assert.Equal(t, []string{"dev", "staging-a", "staging-b", "prod"}, envs)
}

func TestExpandEnvironments_SinglePath(t *testing.T) {
	envs := ExpandEnvironments(makePipeline(promotionPath("dev", "prod")))
	assert.Equal(t, []string{"dev", "prod"}, envs)
}

// A pipeline without promotion paths is a valid pipeline with no environments.

func TestExpandEnvironments_Nil(t *testing.T) {
	assert.Empty(t, ExpandEnvironments(nil))
}

func TestExpandEnvironments_NilSpec(t *testing.T) {
	assert.Empty(t, ExpandEnvironments(&gen.DeploymentPipeline{}))
}

func TestExpandEnvironments_EmptyPaths(t *testing.T) {
	assert.Empty(t, ExpandEnvironments(&gen.DeploymentPipeline{
		Spec: &gen.DeploymentPipelineSpec{PromotionPaths: &[]gen.PromotionPath{}},
	}))
}

func TestExpandEnvironments_SkipsEmptyNames(t *testing.T) {
	envs := ExpandEnvironments(makePipeline(promotionPath("", "staging")))
	assert.Equal(t, []string{"staging"}, envs)
}
