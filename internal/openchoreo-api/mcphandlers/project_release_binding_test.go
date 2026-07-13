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
	projectreleasebindingmocks "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/projectreleasebinding/mocks"
	"github.com/openchoreo/openchoreo/pkg/mcp/tools"
)

const (
	testProjectReleaseBindingName = "standard-project-dev"
	testBindingEnvironment        = "development"
)

func sampleProjectReleaseBinding() *openchoreov1alpha1.ProjectReleaseBinding {
	return &openchoreov1alpha1.ProjectReleaseBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testProjectReleaseBindingName,
			Namespace: testNS,
		},
		Spec: openchoreov1alpha1.ProjectReleaseBindingSpec{
			Owner:          openchoreov1alpha1.ProjectReleaseBindingOwner{ProjectName: testProject},
			Environment:    testBindingEnvironment,
			ProjectRelease: testProjectReleaseName,
		},
	}
}

// genProjectReleaseBindingBody builds a create/update request body from JSON to
// avoid depending on the generated anonymous struct shape.
func genProjectReleaseBindingBody(t *testing.T) *gen.CreateProjectReleaseBindingJSONRequestBody {
	t.Helper()
	var body gen.CreateProjectReleaseBindingJSONRequestBody
	raw := `{
		"metadata": {"name": "` + testProjectReleaseBindingName + `"},
		"spec": {
			"owner": {"projectName": "` + testProject + `"},
			"environment": "` + testBindingEnvironment + `",
			"projectRelease": "` + testProjectReleaseName + `"
		}
	}`
	require.NoError(t, json.Unmarshal([]byte(raw), &body))
	return &body
}

func TestListProjectReleaseBindings(t *testing.T) {
	ctx := context.Background()

	t.Run("returns wrapped list with pagination cursor", func(t *testing.T) {
		rbSvc := projectreleasebindingmocks.NewMockService(t)
		rbSvc.EXPECT().
			ListProjectReleaseBindings(mock.Anything, testNS, testProject, mock.Anything).
			Return(&services.ListResult[openchoreov1alpha1.ProjectReleaseBinding]{
				Items:      []openchoreov1alpha1.ProjectReleaseBinding{*sampleProjectReleaseBinding()},
				NextCursor: "next-token",
			}, nil)

		h := newTestHandler(withProjectReleaseBindingService(rbSvc))
		result, err := h.ListProjectReleaseBindings(ctx, testNS, testProject, tools.ListOpts{Limit: 10})
		require.NoError(t, err)

		m, ok := result.(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "next-token", m["next_cursor"])
		items, ok := m["project_release_bindings"].([]map[string]any)
		require.True(t, ok)
		require.Len(t, items, 1)
		assert.Equal(t, testProjectReleaseBindingName, items[0]["name"])
		assert.Equal(t, testBindingEnvironment, items[0]["environment"])
	})

	t.Run("service error propagates", func(t *testing.T) {
		expected := errors.New("list failed")
		rbSvc := projectreleasebindingmocks.NewMockService(t)
		rbSvc.EXPECT().
			ListProjectReleaseBindings(mock.Anything, testNS, testProject, mock.Anything).
			Return(nil, expected)

		h := newTestHandler(withProjectReleaseBindingService(rbSvc))
		_, err := h.ListProjectReleaseBindings(ctx, testNS, testProject, tools.ListOpts{})
		require.ErrorIs(t, err, expected)
	})
}

func TestGetProjectReleaseBinding(t *testing.T) {
	ctx := context.Background()

	t.Run("returns detail map", func(t *testing.T) {
		rbSvc := projectreleasebindingmocks.NewMockService(t)
		rbSvc.EXPECT().
			GetProjectReleaseBinding(mock.Anything, testNS, testProjectReleaseBindingName).
			Return(sampleProjectReleaseBinding(), nil)

		h := newTestHandler(withProjectReleaseBindingService(rbSvc))
		result, err := h.GetProjectReleaseBinding(ctx, testNS, testProjectReleaseBindingName)
		require.NoError(t, err)

		m, ok := result.(map[string]any)
		require.True(t, ok)
		assert.Equal(t, testProjectReleaseBindingName, m["name"])
		assert.Equal(t, testProjectReleaseName, m["projectRelease"])
	})

	t.Run("service error propagates", func(t *testing.T) {
		expected := errors.New("not found")
		rbSvc := projectreleasebindingmocks.NewMockService(t)
		rbSvc.EXPECT().
			GetProjectReleaseBinding(mock.Anything, testNS, testProjectReleaseBindingName).
			Return(nil, expected)

		h := newTestHandler(withProjectReleaseBindingService(rbSvc))
		_, err := h.GetProjectReleaseBinding(ctx, testNS, testProjectReleaseBindingName)
		require.ErrorIs(t, err, expected)
	})
}

func TestCreateProjectReleaseBinding(t *testing.T) {
	ctx := context.Background()

	t.Run("converts gen body to CRD and sets namespace", func(t *testing.T) {
		rbSvc := projectreleasebindingmocks.NewMockService(t)
		rbSvc.EXPECT().
			CreateProjectReleaseBinding(mock.Anything, testNS, mock.MatchedBy(func(rb *openchoreov1alpha1.ProjectReleaseBinding) bool {
				return rb.Name == testProjectReleaseBindingName &&
					rb.Namespace == testNS &&
					rb.Spec.Owner.ProjectName == testProject &&
					rb.Spec.Environment == testBindingEnvironment &&
					rb.Spec.ProjectRelease == testProjectReleaseName
			})).
			Return(sampleProjectReleaseBinding(), nil)

		h := newTestHandler(withProjectReleaseBindingService(rbSvc))
		result, err := h.CreateProjectReleaseBinding(ctx, testNS, genProjectReleaseBindingBody(t))
		require.NoError(t, err)
		m, ok := result.(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "created", m["action"])
	})

	t.Run("service error propagates", func(t *testing.T) {
		expected := errors.New("create failed")
		rbSvc := projectreleasebindingmocks.NewMockService(t)
		rbSvc.EXPECT().
			CreateProjectReleaseBinding(mock.Anything, testNS, mock.Anything).
			Return(nil, expected)

		h := newTestHandler(withProjectReleaseBindingService(rbSvc))
		_, err := h.CreateProjectReleaseBinding(ctx, testNS, genProjectReleaseBindingBody(t))
		require.ErrorIs(t, err, expected)
	})

	t.Run("rejects nil body", func(t *testing.T) {
		h := newTestHandler()
		_, err := h.CreateProjectReleaseBinding(ctx, testNS, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "request body is required")
	})
}

func TestUpdateProjectReleaseBinding(t *testing.T) {
	ctx := context.Background()

	const newRelease = "standard-project-def456"

	// partialUpdateBody mirrors what the update_project_release_binding tool
	// actually produces: only the mutable fields are set. spec.owner and
	// spec.environment are absent, so the handler must fill them from the
	// existing binding rather than submit them empty.
	partialUpdateBody := func() *gen.UpdateProjectReleaseBindingJSONRequestBody {
		pin := newRelease
		return &gen.UpdateProjectReleaseBindingJSONRequestBody{
			Metadata: gen.ObjectMeta{Name: testProjectReleaseBindingName},
			Spec:     &gen.ProjectReleaseBindingSpec{ProjectRelease: &pin},
		}
	}

	// Regression guard: the tool sends no owner/environment, so a naive convert
	// submitted them empty and the CRD rejected the update as an immutability
	// violation. The handler must preserve them from the existing binding.
	t.Run("re-pins release and preserves immutable owner and environment", func(t *testing.T) {
		rbSvc := projectreleasebindingmocks.NewMockService(t)
		rbSvc.EXPECT().
			GetProjectReleaseBinding(mock.Anything, testNS, testProjectReleaseBindingName).
			Return(sampleProjectReleaseBinding(), nil)

		var sent *openchoreov1alpha1.ProjectReleaseBinding
		rbSvc.EXPECT().
			UpdateProjectReleaseBinding(mock.Anything, testNS, mock.Anything).
			Run(func(_ context.Context, _ string, rb *openchoreov1alpha1.ProjectReleaseBinding) {
				sent = rb
			}).
			Return(sampleProjectReleaseBinding(), nil)

		h := newTestHandler(withProjectReleaseBindingService(rbSvc))
		result, err := h.UpdateProjectReleaseBinding(ctx, testNS, partialUpdateBody())
		require.NoError(t, err)

		require.NotNil(t, sent)
		assert.Equal(t, newRelease, sent.Spec.ProjectRelease, "new pin must be applied")
		assert.Equal(t, testBindingEnvironment, sent.Spec.Environment, "environment must be preserved")
		assert.Equal(t, testProject, sent.Spec.Owner.ProjectName, "owner must be preserved")

		m, ok := result.(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "updated", m["action"])
	})

	t.Run("applies environmentConfigs and preserves the existing pin", func(t *testing.T) {
		rbSvc := projectreleasebindingmocks.NewMockService(t)
		rbSvc.EXPECT().
			GetProjectReleaseBinding(mock.Anything, testNS, testProjectReleaseBindingName).
			Return(sampleProjectReleaseBinding(), nil)

		var sent *openchoreov1alpha1.ProjectReleaseBinding
		rbSvc.EXPECT().
			UpdateProjectReleaseBinding(mock.Anything, testNS, mock.Anything).
			Run(func(_ context.Context, _ string, rb *openchoreov1alpha1.ProjectReleaseBinding) {
				sent = rb
			}).
			Return(sampleProjectReleaseBinding(), nil)

		body := &gen.UpdateProjectReleaseBindingJSONRequestBody{
			Metadata: gen.ObjectMeta{Name: testProjectReleaseBindingName},
			Spec: &gen.ProjectReleaseBindingSpec{
				EnvironmentConfigs: &map[string]interface{}{"replicas": float64(3)},
			},
		}

		h := newTestHandler(withProjectReleaseBindingService(rbSvc))
		_, err := h.UpdateProjectReleaseBinding(ctx, testNS, body)
		require.NoError(t, err)

		require.NotNil(t, sent)
		assert.Equal(t, testBindingEnvironment, sent.Spec.Environment)
		assert.Equal(t, testProject, sent.Spec.Owner.ProjectName)
		assert.Equal(t, testProjectReleaseName, sent.Spec.ProjectRelease, "unset pin must keep existing value")
		require.NotNil(t, sent.Spec.EnvironmentConfigs)
		assert.JSONEq(t, `{"replicas":3}`, string(sent.Spec.EnvironmentConfigs.Raw))
	})

	t.Run("propagates get error before attempting update", func(t *testing.T) {
		expected := errors.New("not found")
		rbSvc := projectreleasebindingmocks.NewMockService(t)
		rbSvc.EXPECT().
			GetProjectReleaseBinding(mock.Anything, testNS, testProjectReleaseBindingName).
			Return(nil, expected)

		h := newTestHandler(withProjectReleaseBindingService(rbSvc))
		_, err := h.UpdateProjectReleaseBinding(ctx, testNS, partialUpdateBody())
		require.ErrorIs(t, err, expected)
	})

	t.Run("propagates update error", func(t *testing.T) {
		expected := errors.New("update failed")
		rbSvc := projectreleasebindingmocks.NewMockService(t)
		rbSvc.EXPECT().
			GetProjectReleaseBinding(mock.Anything, testNS, testProjectReleaseBindingName).
			Return(sampleProjectReleaseBinding(), nil)
		rbSvc.EXPECT().
			UpdateProjectReleaseBinding(mock.Anything, testNS, mock.Anything).
			Return(nil, expected)

		h := newTestHandler(withProjectReleaseBindingService(rbSvc))
		_, err := h.UpdateProjectReleaseBinding(ctx, testNS, partialUpdateBody())
		require.ErrorIs(t, err, expected)
	})

	t.Run("rejects nil body", func(t *testing.T) {
		h := newTestHandler()
		_, err := h.UpdateProjectReleaseBinding(ctx, testNS, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "request body is required")
	})
}

func TestDeleteProjectReleaseBinding(t *testing.T) {
	ctx := context.Background()

	t.Run("returns action deleted", func(t *testing.T) {
		rbSvc := projectreleasebindingmocks.NewMockService(t)
		rbSvc.EXPECT().
			DeleteProjectReleaseBinding(mock.Anything, testNS, testProjectReleaseBindingName).
			Return(nil)

		h := newTestHandler(withProjectReleaseBindingService(rbSvc))
		result, err := h.DeleteProjectReleaseBinding(ctx, testNS, testProjectReleaseBindingName)
		require.NoError(t, err)
		m, ok := result.(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "deleted", m["action"])
		assert.Equal(t, testProjectReleaseBindingName, m["name"])
		assert.Equal(t, testNS, m["namespace"])
	})

	t.Run("service error propagates", func(t *testing.T) {
		expected := errors.New("delete failed")
		rbSvc := projectreleasebindingmocks.NewMockService(t)
		rbSvc.EXPECT().
			DeleteProjectReleaseBinding(mock.Anything, testNS, testProjectReleaseBindingName).
			Return(expected)

		h := newTestHandler(withProjectReleaseBindingService(rbSvc))
		_, err := h.DeleteProjectReleaseBinding(ctx, testNS, testProjectReleaseBindingName)
		require.ErrorIs(t, err, expected)
	})
}
