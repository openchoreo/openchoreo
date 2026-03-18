// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package component

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openchoreo/openchoreo/internal/occ/resources/client"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
)

func TestReverseLogs(t *testing.T) {
	tests := []struct {
		name string
		logs []client.LogEntry
		want []string
	}{
		{
			name: "multiple entries",
			logs: []client.LogEntry{
				{Timestamp: "t1", Log: "a"},
				{Timestamp: "t2", Log: "b"},
				{Timestamp: "t3", Log: "c"},
			},
			want: []string{"c", "b", "a"},
		},
		{
			name: "single entry",
			logs: []client.LogEntry{{Timestamp: "t1", Log: "only"}},
			want: []string{"only"},
		},
		{
			name: "two entries",
			logs: []client.LogEntry{
				{Timestamp: "t1", Log: "first"},
				{Timestamp: "t2", Log: "second"},
			},
			want: []string{"second", "first"},
		},
		{
			name: "empty slice",
			logs: []client.LogEntry{},
			want: []string{},
		},
		{
			name: "nil slice",
			logs: nil,
			want: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reverseLogs(tt.logs)
			require.Len(t, tt.logs, len(tt.want))
			for i, w := range tt.want {
				assert.Equal(t, w, tt.logs[i].Log)
			}
		})
	}
}

func makePipeline(paths []gen.PromotionPath) *gen.DeploymentPipeline {
	p := &gen.DeploymentPipeline{
		Metadata: gen.ObjectMeta{Name: "test-pipeline"},
	}
	if paths != nil {
		p.Spec = &gen.DeploymentPipelineSpec{
			PromotionPaths: &paths,
		}
	}
	return p
}

func promotionPath(source string, targets ...string) gen.PromotionPath {
	refs := make([]gen.TargetEnvironmentRef, len(targets))
	for i, t := range targets {
		refs[i] = gen.TargetEnvironmentRef{Name: t}
	}
	pp := gen.PromotionPath{
		TargetEnvironmentRefs: refs,
	}
	pp.SourceEnvironmentRef.Name = source
	return pp
}

func TestFindRootEnvironment(t *testing.T) {
	tests := []struct {
		name     string
		pipeline *gen.DeploymentPipeline
		want     string
		wantErr  bool
	}{
		{
			name: "linear dev->staging->prod",
			pipeline: makePipeline([]gen.PromotionPath{
				promotionPath("dev", "staging"),
				promotionPath("staging", "prod"),
			}),
			want: "dev",
		},
		{
			name: "diamond: dev->staging, dev->qa, staging->prod, qa->prod",
			pipeline: makePipeline([]gen.PromotionPath{
				promotionPath("dev", "staging", "qa"),
				promotionPath("staging", "prod"),
				promotionPath("qa", "prod"),
			}),
			want: "dev",
		},
		{
			name: "single path",
			pipeline: makePipeline([]gen.PromotionPath{
				promotionPath("dev", "prod"),
			}),
			want: "dev",
		},
		{
			name:     "nil spec",
			pipeline: &gen.DeploymentPipeline{Metadata: gen.ObjectMeta{Name: "p"}},
			wantErr:  true,
		},
		{
			name:     "empty promotion paths",
			pipeline: makePipeline([]gen.PromotionPath{}),
			wantErr:  true,
		},
		{
			name: "all sources are also targets",
			pipeline: makePipeline([]gen.PromotionPath{
				promotionPath("a", "b"),
				promotionPath("b", "a"),
			}),
			wantErr: true,
		},
		{
			name: "skips empty source name",
			pipeline: makePipeline([]gen.PromotionPath{
				promotionPath("", "staging"),
				promotionPath("dev", "staging"),
			}),
			want: "dev",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := findRootEnvironment(tt.pipeline)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}
