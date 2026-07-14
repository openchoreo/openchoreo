// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package mcphandlers

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
	projectreleasemocks "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/projectrelease/mocks"
	"github.com/openchoreo/openchoreo/pkg/mcp/tools"
)

const testProjectReleaseName = "standard-project-abc123"

func sampleProjectRelease() *openchoreov1alpha1.ProjectRelease {
	return &openchoreov1alpha1.ProjectRelease{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testProjectReleaseName,
			Namespace: testNS,
		},
		Spec: openchoreov1alpha1.ProjectReleaseSpec{
			Owner: openchoreov1alpha1.ProjectReleaseOwner{ProjectName: testProject},
			ProjectType: openchoreov1alpha1.ProjectReleaseProjectType{
				Kind: openchoreov1alpha1.ProjectTypeRefKindProjectType,
				Name: testProjectTypeName,
			},
		},
	}
}

// genProjectReleaseBody builds a create request body from JSON to avoid
// depending on the generated anonymous struct shape of ProjectReleaseSpec.
func genProjectReleaseBody(t *testing.T) *gen.CreateProjectReleaseJSONRequestBody {
	t.Helper()
	var body gen.CreateProjectReleaseJSONRequestBody
	raw := `{
		"metadata": {"name": "` + testProjectReleaseName + `"},
		"spec": {
			"owner": {"projectName": "` + testProject + `"},
			"projectType": {
				"kind": "ProjectType",
				"name": "` + testProjectTypeName + `",
				"spec": {"resources": [{"id": "cell-namespace", "template": {}}]}
			}
		}
	}`
	require.NoError(t, json.Unmarshal([]byte(raw), &body))
	return &body
}

func TestListProjectReleases(t *testing.T) {
	ctx := context.Background()

	t.Run("returns wrapped list with pagination cursor", func(t *testing.T) {
		prSvc := projectreleasemocks.NewMockService(t)
		prSvc.EXPECT().
			ListProjectReleases(mock.Anything, testNS, testProject, mock.Anything).
			Return(&services.ListResult[openchoreov1alpha1.ProjectRelease]{
				Items:      []openchoreov1alpha1.ProjectRelease{*sampleProjectRelease()},
				NextCursor: "next-token",
			}, nil)

		h := newTestHandler(withProjectReleaseService(prSvc))
		result, err := h.ListProjectReleases(ctx, testNS, testProject, tools.ListOpts{Limit: 10})
		require.NoError(t, err)

		m, ok := result.(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "next-token", m["next_cursor"])
		items, ok := m["project_releases"].([]map[string]any)
		require.True(t, ok)
		require.Len(t, items, 1)
		assert.Equal(t, testProjectReleaseName, items[0]["name"])
		assert.Equal(t, testProject, items[0]["projectName"])
	})

	t.Run("service error propagates", func(t *testing.T) {
		expected := errors.New("list failed")
		prSvc := projectreleasemocks.NewMockService(t)
		prSvc.EXPECT().
			ListProjectReleases(mock.Anything, testNS, testProject, mock.Anything).
			Return(nil, expected)

		h := newTestHandler(withProjectReleaseService(prSvc))
		_, err := h.ListProjectReleases(ctx, testNS, testProject, tools.ListOpts{})
		require.ErrorIs(t, err, expected)
	})
}

func TestGetProjectRelease(t *testing.T) {
	ctx := context.Background()

	t.Run("returns detail map with embedded type snapshot", func(t *testing.T) {
		prSvc := projectreleasemocks.NewMockService(t)
		prSvc.EXPECT().
			GetProjectRelease(mock.Anything, testNS, testProjectReleaseName).
			Return(sampleProjectRelease(), nil)

		h := newTestHandler(withProjectReleaseService(prSvc))
		result, err := h.GetProjectRelease(ctx, testNS, testProjectReleaseName)
		require.NoError(t, err)

		m, ok := result.(map[string]any)
		require.True(t, ok)
		assert.Equal(t, testProjectReleaseName, m["name"])
		assert.Equal(t, testProject, m["projectName"])
	})

	t.Run("service error propagates", func(t *testing.T) {
		expected := errors.New("not found")
		prSvc := projectreleasemocks.NewMockService(t)
		prSvc.EXPECT().
			GetProjectRelease(mock.Anything, testNS, testProjectReleaseName).
			Return(nil, expected)

		h := newTestHandler(withProjectReleaseService(prSvc))
		_, err := h.GetProjectRelease(ctx, testNS, testProjectReleaseName)
		require.ErrorIs(t, err, expected)
	})
}

func TestCreateProjectRelease(t *testing.T) {
	ctx := context.Background()

	t.Run("converts gen body to CRD and sets namespace", func(t *testing.T) {
		prSvc := projectreleasemocks.NewMockService(t)
		prSvc.EXPECT().
			CreateProjectRelease(mock.Anything, testNS, mock.MatchedBy(func(pr *openchoreov1alpha1.ProjectRelease) bool {
				return pr.Name == testProjectReleaseName &&
					pr.Namespace == testNS &&
					pr.Spec.Owner.ProjectName == testProject &&
					pr.Spec.ProjectType.Name == testProjectTypeName &&
					pr.Spec.ProjectType.Kind == openchoreov1alpha1.ProjectTypeRefKindProjectType &&
					len(pr.Spec.ProjectType.Spec.Resources) == 1
			})).
			Return(sampleProjectRelease(), nil)

		h := newTestHandler(withProjectReleaseService(prSvc))
		result, err := h.CreateProjectRelease(ctx, testNS, genProjectReleaseBody(t))
		require.NoError(t, err)
		m, ok := result.(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "created", m["action"])
	})

	t.Run("service error propagates", func(t *testing.T) {
		expected := errors.New("create failed")
		prSvc := projectreleasemocks.NewMockService(t)
		prSvc.EXPECT().
			CreateProjectRelease(mock.Anything, testNS, mock.Anything).
			Return(nil, expected)

		h := newTestHandler(withProjectReleaseService(prSvc))
		_, err := h.CreateProjectRelease(ctx, testNS, genProjectReleaseBody(t))
		require.ErrorIs(t, err, expected)
	})

	t.Run("rejects nil body", func(t *testing.T) {
		h := newTestHandler()
		_, err := h.CreateProjectRelease(ctx, testNS, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "request body is required")
	})
}

func TestDeleteProjectRelease(t *testing.T) {
	ctx := context.Background()

	t.Run("returns action deleted", func(t *testing.T) {
		prSvc := projectreleasemocks.NewMockService(t)
		prSvc.EXPECT().
			DeleteProjectRelease(mock.Anything, testNS, testProjectReleaseName).
			Return(nil)

		h := newTestHandler(withProjectReleaseService(prSvc))
		result, err := h.DeleteProjectRelease(ctx, testNS, testProjectReleaseName)
		require.NoError(t, err)
		m, ok := result.(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "deleted", m["action"])
		assert.Equal(t, testProjectReleaseName, m["name"])
		assert.Equal(t, testNS, m["namespace"])
	})

	t.Run("service error propagates", func(t *testing.T) {
		expected := errors.New("delete failed")
		prSvc := projectreleasemocks.NewMockService(t)
		prSvc.EXPECT().
			DeleteProjectRelease(mock.Anything, testNS, testProjectReleaseName).
			Return(expected)

		h := newTestHandler(withProjectReleaseService(prSvc))
		_, err := h.DeleteProjectRelease(ctx, testNS, testProjectReleaseName)
		require.ErrorIs(t, err, expected)
	})
}
