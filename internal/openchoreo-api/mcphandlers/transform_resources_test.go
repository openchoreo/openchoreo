// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package mcphandlers

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openchoreo/openchoreo/internal/openchoreo-api/models"
	k8sresourcessvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/k8sresources"
)

func TestResourceTreeDetail_ProjectsChildDiscoveryFields(t *testing.T) {
	result := &k8sresourcessvc.K8sResourceTreeResult{
		RenderedReleases: []k8sresourcessvc.ReleaseResourceTree{
			{
				Name:        "rel-a",
				TargetPlane: "dataplane",
				Nodes: []models.ResourceNode{
					{
						Version:      "v1",
						Kind:         "Secret",
						Name:         "sec-a",
						MetadataOnly: true,
						MatchedBy:    "labelSelector",
						ChildrenStatus: []models.ChildDiscoveryStatus{
							{Group: "apps", Version: "v1", Kind: "ReplicaSet", State: "forbidden", Message: "no list permission"},
						},
					},
					{Version: "v1", Kind: "Service", Name: "svc-a"},
				},
			},
		},
	}

	detail := resourceTreeDetail(result)

	releases, ok := detail["rendered_releases"].([]map[string]any)
	require.True(t, ok, "rendered_releases should be a list of maps")
	require.Len(t, releases, 1)
	nodes, ok := releases[0]["nodes"].([]map[string]any)
	require.True(t, ok, "nodes should be a list of maps")
	require.Len(t, nodes, 2)

	annotated := nodes[0]
	assert.Equal(t, true, annotated["metadata_only"])
	assert.Equal(t, "labelSelector", annotated["matched_by"])
	childrenStatus, ok := annotated["children_status"].([]map[string]any)
	require.True(t, ok, "children_status should be a list of maps")
	require.Len(t, childrenStatus, 1)
	assert.Equal(t, "apps", childrenStatus[0]["group"])
	assert.Equal(t, "v1", childrenStatus[0]["version"])
	assert.Equal(t, "ReplicaSet", childrenStatus[0]["kind"])
	assert.Equal(t, "forbidden", childrenStatus[0]["state"])
	assert.Equal(t, "no list permission", childrenStatus[0]["message"])

	// An exact ownerRef match is not badged, and an unremarkable node stays as
	// small as it was before child discovery existed.
	plain := nodes[1]
	assert.NotContains(t, plain, "metadata_only")
	assert.NotContains(t, plain, "matched_by")
	assert.NotContains(t, plain, "children_status")
}

func TestResourceTreeDetail_OmitsEmptyGroupAndMessageInChildStatus(t *testing.T) {
	result := &k8sresourcessvc.K8sResourceTreeResult{
		RenderedReleases: []k8sresourcessvc.ReleaseResourceTree{
			{
				Name:        "rel-a",
				TargetPlane: "dataplane",
				Nodes: []models.ResourceNode{
					{
						Version: "v1",
						Kind:    "Job",
						Name:    "job-a",
						ChildrenStatus: []models.ChildDiscoveryStatus{
							{Version: "v1", Kind: "Pod", State: "error"},
						},
					},
				},
			},
		},
	}

	nodes := resourceTreeDetail(result)["rendered_releases"].([]map[string]any)[0]["nodes"].([]map[string]any)
	status := nodes[0]["children_status"].([]map[string]any)[0]
	assert.NotContains(t, status, "group")
	assert.NotContains(t, status, "message")
	assert.Equal(t, "error", status["state"])
}
