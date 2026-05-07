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
	"k8s.io/apimachinery/pkg/runtime"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/controller"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
	clustercomponenttypesvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/clustercomponenttype"
	clustercomponenttypemocks "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/clustercomponenttype/mocks"
	componentmocks "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/component/mocks"
	componentreleasemocks "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/componentrelease/mocks"
	componenttypesvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/componenttype"
	componenttypemocks "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/componenttype/mocks"
	releasebindingmocks "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/releasebinding/mocks"
	workflowrunmocks "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/workflowrun/mocks"
	workloadmocks "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/workload/mocks"
)

const (
	testNS              = "test-ns"
	testProject         = "test-project"
	testComponent       = "test-component"
	testComponentType   = "deployment/my-type"
	testEnvironmentName = "dev"
	testExistingVal     = "existing-val"
	testNewVal          = "new-val"
	testDisplayName     = "My Component"
	testDescription     = "A test component"
)

// ---------------------------------------------------------------------------
// parseComponentTypeFormat
// ---------------------------------------------------------------------------

func TestParseComponentTypeFormat(t *testing.T) {
	tests := []struct {
		name          string
		input         string
		wantWorkload  string
		wantTypeName  string
		wantErrSubstr string
	}{
		{
			name:         "valid format",
			input:        "deployment/web-app",
			wantWorkload: "deployment",
			wantTypeName: "web-app",
		},
		{
			name:         "valid with complex name",
			input:        "statefulset/my-db",
			wantWorkload: "statefulset",
			wantTypeName: "my-db",
		},
		{
			name:          "no slash",
			input:         "invalid",
			wantErrSubstr: "invalid componentType format",
		},
		{
			name:          "empty workload type",
			input:         "/name",
			wantErrSubstr: "invalid componentType format",
		},
		{
			name:          "empty name",
			input:         "type/",
			wantErrSubstr: "invalid componentType format",
		},
		{
			name:          "empty string",
			input:         "",
			wantErrSubstr: "invalid componentType format",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wt, tn, err := parseComponentTypeFormat(tt.input)
			if tt.wantErrSubstr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErrSubstr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantWorkload, wt)
			assert.Equal(t, tt.wantTypeName, tn)
		})
	}
}

// ---------------------------------------------------------------------------
// CreateComponent
// ---------------------------------------------------------------------------

func TestCreateComponent(t *testing.T) {
	ctx := context.Background()

	makeCreated := func(name string) *openchoreov1alpha1.Component {
		return &openchoreov1alpha1.Component{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: testNS}}
	}

	t.Run("happy path minimal", func(t *testing.T) {
		compSvc := componentmocks.NewMockService(t)
		compSvc.EXPECT().
			CreateComponent(mock.Anything, testNS, mock.MatchedBy(func(c *openchoreov1alpha1.Component) bool {
				return c.Name == "my-comp" &&
					c.Namespace == testNS &&
					c.Spec.Owner.ProjectName == testProject
			})).
			Return(makeCreated("my-comp"), nil)

		h := newTestHandler(withComponentService(compSvc))
		req := &gen.CreateComponentRequest{Name: "my-comp"}
		result, err := h.CreateComponent(ctx, testNS, testProject, req)
		require.NoError(t, err)
		assert.NotNil(t, result)
	})

	t.Run("happy path with display name and description", func(t *testing.T) {
		compSvc := componentmocks.NewMockService(t)
		displayName := testDisplayName
		description := testDescription
		compSvc.EXPECT().
			CreateComponent(mock.Anything, testNS, mock.MatchedBy(func(c *openchoreov1alpha1.Component) bool {
				return c.Annotations["openchoreo.dev/display-name"] == displayName &&
					c.Annotations["openchoreo.dev/description"] == description
			})).
			Return(makeCreated("my-comp"), nil)

		h := newTestHandler(withComponentService(compSvc))
		req := &gen.CreateComponentRequest{Name: "my-comp", DisplayName: &displayName, Description: &description}
		_, err := h.CreateComponent(ctx, testNS, testProject, req)
		require.NoError(t, err)
	})

	t.Run("empty display name and description not set", func(t *testing.T) {
		compSvc := componentmocks.NewMockService(t)
		empty := ""
		compSvc.EXPECT().
			CreateComponent(mock.Anything, testNS, mock.MatchedBy(func(c *openchoreov1alpha1.Component) bool {
				_, hasDisplay := c.Annotations["openchoreo.dev/display-name"]
				_, hasDesc := c.Annotations["openchoreo.dev/description"]
				return !hasDisplay && !hasDesc
			})).
			Return(makeCreated("my-comp"), nil)

		h := newTestHandler(withComponentService(compSvc))
		req := &gen.CreateComponentRequest{Name: "my-comp", DisplayName: &empty, Description: &empty}
		_, err := h.CreateComponent(ctx, testNS, testProject, req)
		require.NoError(t, err)
	})

	t.Run("auto deploy true", func(t *testing.T) {
		compSvc := componentmocks.NewMockService(t)
		autoDeploy := true
		compSvc.EXPECT().
			CreateComponent(mock.Anything, testNS, mock.MatchedBy(func(c *openchoreov1alpha1.Component) bool {
				return c.Spec.AutoDeploy == true
			})).
			Return(makeCreated("my-comp"), nil)

		h := newTestHandler(withComponentService(compSvc))
		req := &gen.CreateComponentRequest{Name: "my-comp", AutoDeploy: &autoDeploy}
		_, err := h.CreateComponent(ctx, testNS, testProject, req)
		require.NoError(t, err)
	})

	t.Run("with parameters", func(t *testing.T) {
		compSvc := componentmocks.NewMockService(t)
		params := map[string]interface{}{"key": "value"}
		expectedRaw, _ := json.Marshal(params)
		compSvc.EXPECT().
			CreateComponent(mock.Anything, testNS, mock.MatchedBy(func(c *openchoreov1alpha1.Component) bool {
				return c.Spec.Parameters != nil &&
					string(c.Spec.Parameters.Raw) == string(expectedRaw)
			})).
			Return(makeCreated("my-comp"), nil)

		h := newTestHandler(withComponentService(compSvc))
		req := &gen.CreateComponentRequest{Name: "my-comp", Parameters: &params}
		_, err := h.CreateComponent(ctx, testNS, testProject, req)
		require.NoError(t, err)
	})

	t.Run("with workflow without parameters", func(t *testing.T) {
		compSvc := componentmocks.NewMockService(t)
		compSvc.EXPECT().
			CreateComponent(mock.Anything, testNS, mock.MatchedBy(func(c *openchoreov1alpha1.Component) bool {
				return c.Spec.Workflow != nil &&
					c.Spec.Workflow.Name == "build-wf" &&
					c.Spec.Workflow.Parameters == nil
			})).
			Return(makeCreated("my-comp"), nil)

		h := newTestHandler(withComponentService(compSvc))
		req := &gen.CreateComponentRequest{
			Name:     "my-comp",
			Workflow: &gen.ComponentWorkflowInput{Name: "build-wf"},
		}
		_, err := h.CreateComponent(ctx, testNS, testProject, req)
		require.NoError(t, err)
	})

	t.Run("with workflow with parameters", func(t *testing.T) {
		compSvc := componentmocks.NewMockService(t)
		wfParams := map[string]interface{}{"branch": "main"}
		expectedRaw, _ := json.Marshal(wfParams)
		compSvc.EXPECT().
			CreateComponent(mock.Anything, testNS, mock.MatchedBy(func(c *openchoreov1alpha1.Component) bool {
				return c.Spec.Workflow != nil &&
					c.Spec.Workflow.Parameters != nil &&
					string(c.Spec.Workflow.Parameters.Raw) == string(expectedRaw)
			})).
			Return(makeCreated("my-comp"), nil)

		h := newTestHandler(withComponentService(compSvc))
		req := &gen.CreateComponentRequest{
			Name: "my-comp",
			Workflow: &gen.ComponentWorkflowInput{
				Name:       "build-wf",
				Parameters: &wfParams,
			},
		}
		_, err := h.CreateComponent(ctx, testNS, testProject, req)
		require.NoError(t, err)
	})

	t.Run("componentType resolves to namespace-scoped", func(t *testing.T) {
		compSvc := componentmocks.NewMockService(t)
		ctSvc := componenttypemocks.NewMockService(t)
		ct := testComponentType
		ctSvc.EXPECT().GetComponentType(mock.Anything, testNS, "my-type").Return(&openchoreov1alpha1.ComponentType{}, nil)
		compSvc.EXPECT().
			CreateComponent(mock.Anything, testNS, mock.MatchedBy(func(c *openchoreov1alpha1.Component) bool {
				return c.Spec.ComponentType.Kind == openchoreov1alpha1.ComponentTypeRefKindComponentType &&
					c.Spec.ComponentType.Name == ct
			})).
			Return(makeCreated("my-comp"), nil)

		h := newTestHandler(withComponentService(compSvc), withComponentTypeService(ctSvc))
		req := &gen.CreateComponentRequest{Name: "my-comp", ComponentType: &ct}
		_, err := h.CreateComponent(ctx, testNS, testProject, req)
		require.NoError(t, err)
	})

	t.Run("componentType falls back to cluster-scoped", func(t *testing.T) {
		compSvc := componentmocks.NewMockService(t)
		ctSvc := componenttypemocks.NewMockService(t)
		cctSvc := clustercomponenttypemocks.NewMockService(t)
		ct := "deployment/cluster-type"
		ctSvc.EXPECT().GetComponentType(mock.Anything, testNS, "cluster-type").Return(nil, componenttypesvc.ErrComponentTypeNotFound)
		cctSvc.EXPECT().GetClusterComponentType(mock.Anything, "cluster-type").Return(&openchoreov1alpha1.ClusterComponentType{}, nil)
		compSvc.EXPECT().
			CreateComponent(mock.Anything, testNS, mock.MatchedBy(func(c *openchoreov1alpha1.Component) bool {
				return c.Spec.ComponentType.Kind == openchoreov1alpha1.ComponentTypeRefKindClusterComponentType
			})).
			Return(makeCreated("my-comp"), nil)

		h := newTestHandler(withComponentService(compSvc), withComponentTypeService(ctSvc), withClusterComponentTypeService(cctSvc))
		req := &gen.CreateComponentRequest{Name: "my-comp", ComponentType: &ct}
		_, err := h.CreateComponent(ctx, testNS, testProject, req)
		require.NoError(t, err)
	})

	t.Run("componentType not found in either", func(t *testing.T) {
		ctSvc := componenttypemocks.NewMockService(t)
		cctSvc := clustercomponenttypemocks.NewMockService(t)
		ct := "deployment/unknown"
		ctSvc.EXPECT().GetComponentType(mock.Anything, testNS, "unknown").Return(nil, componenttypesvc.ErrComponentTypeNotFound)
		cctSvc.EXPECT().GetClusterComponentType(mock.Anything, "unknown").Return(nil, clustercomponenttypesvc.ErrClusterComponentTypeNotFound)

		h := newTestHandler(withComponentTypeService(ctSvc), withClusterComponentTypeService(cctSvc))
		req := &gen.CreateComponentRequest{Name: "my-comp", ComponentType: &ct}
		_, err := h.CreateComponent(ctx, testNS, testProject, req)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})

	t.Run("componentType invalid format", func(t *testing.T) {
		h := newTestHandler()
		ct := "invalid-no-slash"
		req := &gen.CreateComponentRequest{Name: "my-comp", ComponentType: &ct}
		_, err := h.CreateComponent(ctx, testNS, testProject, req)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid componentType format")
	})

	t.Run("unexpected error from ComponentTypeService", func(t *testing.T) {
		ctSvc := componenttypemocks.NewMockService(t)
		ct := testComponentType
		unexpectedErr := errors.New("internal error")
		ctSvc.EXPECT().GetComponentType(mock.Anything, testNS, "my-type").Return(nil, unexpectedErr)

		h := newTestHandler(withComponentTypeService(ctSvc))
		req := &gen.CreateComponentRequest{Name: "my-comp", ComponentType: &ct}
		_, err := h.CreateComponent(ctx, testNS, testProject, req)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "internal error")
	})

	t.Run("unexpected error from ClusterComponentTypeService", func(t *testing.T) {
		ctSvc := componenttypemocks.NewMockService(t)
		cctSvc := clustercomponenttypemocks.NewMockService(t)
		ct := testComponentType
		ctSvc.EXPECT().GetComponentType(mock.Anything, testNS, "my-type").Return(nil, componenttypesvc.ErrComponentTypeNotFound)
		cctSvc.EXPECT().GetClusterComponentType(mock.Anything, "my-type").Return(nil, errors.New("cct internal error"))

		h := newTestHandler(withComponentTypeService(ctSvc), withClusterComponentTypeService(cctSvc))
		req := &gen.CreateComponentRequest{Name: "my-comp", ComponentType: &ct}
		_, err := h.CreateComponent(ctx, testNS, testProject, req)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cct internal error")
	})

	t.Run("service error propagated", func(t *testing.T) {
		compSvc := componentmocks.NewMockService(t)
		compSvc.EXPECT().CreateComponent(mock.Anything, testNS, mock.Anything).Return(nil, errors.New("create failed"))

		h := newTestHandler(withComponentService(compSvc))
		req := &gen.CreateComponentRequest{Name: "my-comp"}
		_, err := h.CreateComponent(ctx, testNS, testProject, req)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "create failed")
	})
}

// ---------------------------------------------------------------------------
// PatchComponent
// ---------------------------------------------------------------------------

func TestPatchComponent(t *testing.T) {
	ctx := context.Background()

	// Use a factory to avoid mutations leaking between subtests
	freshComponent := func() *openchoreov1alpha1.Component {
		return &openchoreov1alpha1.Component{
			ObjectMeta: metav1.ObjectMeta{Name: testComponent, Namespace: testNS},
			Spec:       openchoreov1alpha1.ComponentSpec{AutoDeploy: false},
		}
	}

	t.Run("updates AutoDeploy", func(t *testing.T) {
		compSvc := componentmocks.NewMockService(t)
		compSvc.EXPECT().GetComponent(mock.Anything, testNS, testComponent).Return(freshComponent(), nil)
		compSvc.EXPECT().
			UpdateComponent(mock.Anything, testNS, mock.MatchedBy(func(c *openchoreov1alpha1.Component) bool {
				return c.Spec.AutoDeploy == true
			})).
			Return(freshComponent(), nil)

		autoDeploy := true
		h := newTestHandler(withComponentService(compSvc))
		_, err := h.PatchComponent(ctx, testNS, testComponent, &gen.PatchComponentRequest{AutoDeploy: &autoDeploy})
		require.NoError(t, err)
	})

	t.Run("updates Parameters", func(t *testing.T) {
		compSvc := componentmocks.NewMockService(t)
		compSvc.EXPECT().GetComponent(mock.Anything, testNS, testComponent).Return(freshComponent(), nil)
		params := map[string]interface{}{"env": "prod"}
		expectedRaw, _ := json.Marshal(params)
		compSvc.EXPECT().
			UpdateComponent(mock.Anything, testNS, mock.MatchedBy(func(c *openchoreov1alpha1.Component) bool {
				return c.Spec.Parameters != nil &&
					string(c.Spec.Parameters.Raw) == string(expectedRaw)
			})).
			Return(freshComponent(), nil)

		h := newTestHandler(withComponentService(compSvc))
		_, err := h.PatchComponent(ctx, testNS, testComponent, &gen.PatchComponentRequest{Parameters: &params})
		require.NoError(t, err)
	})

	t.Run("nil fields leave component unchanged", func(t *testing.T) {
		compSvc := componentmocks.NewMockService(t)
		compSvc.EXPECT().GetComponent(mock.Anything, testNS, testComponent).Return(freshComponent(), nil)
		compSvc.EXPECT().
			UpdateComponent(mock.Anything, testNS, mock.MatchedBy(func(c *openchoreov1alpha1.Component) bool {
				return c.Spec.AutoDeploy == false && c.Spec.Parameters == nil
			})).
			Return(freshComponent(), nil)

		h := newTestHandler(withComponentService(compSvc))
		_, err := h.PatchComponent(ctx, testNS, testComponent, &gen.PatchComponentRequest{})
		require.NoError(t, err)
	})

	t.Run("GetComponent error propagated", func(t *testing.T) {
		compSvc := componentmocks.NewMockService(t)
		compSvc.EXPECT().GetComponent(mock.Anything, testNS, testComponent).Return(nil, errors.New("not found"))

		h := newTestHandler(withComponentService(compSvc))
		_, err := h.PatchComponent(ctx, testNS, testComponent, &gen.PatchComponentRequest{})
		require.Error(t, err)
	})

	t.Run("updates DisplayName and Description annotations", func(t *testing.T) {
		compSvc := componentmocks.NewMockService(t)
		compSvc.EXPECT().GetComponent(mock.Anything, testNS, testComponent).Return(freshComponent(), nil)
		compSvc.EXPECT().
			UpdateComponent(mock.Anything, testNS, mock.MatchedBy(func(c *openchoreov1alpha1.Component) bool {
				return c.Annotations[controller.AnnotationKeyDisplayName] == testDisplayName &&
					c.Annotations[controller.AnnotationKeyDescription] == testDescription
			})).
			Return(freshComponent(), nil)

		dn, desc := testDisplayName, testDescription
		h := newTestHandler(withComponentService(compSvc))
		_, err := h.PatchComponent(ctx, testNS, testComponent, &gen.PatchComponentRequest{
			DisplayName: &dn,
			Description: &desc,
		})
		require.NoError(t, err)
	})

	t.Run("empty DisplayName is treated as no-change", func(t *testing.T) {
		existing := freshComponent()
		existing.Annotations = map[string]string{
			controller.AnnotationKeyDisplayName: "Original Name",
		}
		compSvc := componentmocks.NewMockService(t)
		compSvc.EXPECT().GetComponent(mock.Anything, testNS, testComponent).Return(existing, nil)
		compSvc.EXPECT().
			UpdateComponent(mock.Anything, testNS, mock.MatchedBy(func(c *openchoreov1alpha1.Component) bool {
				return c.Annotations[controller.AnnotationKeyDisplayName] == "Original Name"
			})).
			Return(existing, nil)

		empty := ""
		h := newTestHandler(withComponentService(compSvc))
		_, err := h.PatchComponent(ctx, testNS, testComponent, &gen.PatchComponentRequest{DisplayName: &empty})
		require.NoError(t, err)
	})

	t.Run("replaces Traits with new list", func(t *testing.T) {
		existing := freshComponent()
		existing.Spec.Traits = []openchoreov1alpha1.ComponentTrait{
			{Name: "old-trait", InstanceName: "old"},
		}
		compSvc := componentmocks.NewMockService(t)
		compSvc.EXPECT().GetComponent(mock.Anything, testNS, testComponent).Return(existing, nil)

		params := map[string]interface{}{"min": 1.0, "max": 5.0}
		expectedRaw, _ := json.Marshal(params)
		compSvc.EXPECT().
			UpdateComponent(mock.Anything, testNS, mock.MatchedBy(func(c *openchoreov1alpha1.Component) bool {
				if len(c.Spec.Traits) != 1 {
					return false
				}
				tr := c.Spec.Traits[0]
				return tr.Name == "autoscaler" &&
					tr.InstanceName == "api-autoscaler" &&
					tr.Kind == openchoreov1alpha1.TraitRefKind("ClusterTrait") &&
					tr.Parameters != nil && string(tr.Parameters.Raw) == string(expectedRaw)
			})).
			Return(existing, nil)

		clusterKind := gen.ComponentTraitInputKindClusterTrait
		traits := []gen.ComponentTraitInput{
			{
				Name:         "autoscaler",
				InstanceName: "api-autoscaler",
				Kind:         &clusterKind,
				Parameters:   &params,
			},
		}
		h := newTestHandler(withComponentService(compSvc))
		_, err := h.PatchComponent(ctx, testNS, testComponent, &gen.PatchComponentRequest{Traits: &traits})
		require.NoError(t, err)
	})

	t.Run("empty Traits clears all traits", func(t *testing.T) {
		existing := freshComponent()
		existing.Spec.Traits = []openchoreov1alpha1.ComponentTrait{
			{Name: "to-be-removed", InstanceName: "x"},
		}
		compSvc := componentmocks.NewMockService(t)
		compSvc.EXPECT().GetComponent(mock.Anything, testNS, testComponent).Return(existing, nil)
		compSvc.EXPECT().
			UpdateComponent(mock.Anything, testNS, mock.MatchedBy(func(c *openchoreov1alpha1.Component) bool {
				return c.Spec.Traits != nil && len(c.Spec.Traits) == 0
			})).
			Return(existing, nil)

		empty := []gen.ComponentTraitInput{}
		h := newTestHandler(withComponentService(compSvc))
		_, err := h.PatchComponent(ctx, testNS, testComponent, &gen.PatchComponentRequest{Traits: &empty})
		require.NoError(t, err)
	})

	t.Run("nil Traits leaves existing traits unchanged", func(t *testing.T) {
		existing := freshComponent()
		existing.Spec.Traits = []openchoreov1alpha1.ComponentTrait{
			{Name: "keep-me", InstanceName: "k"},
		}
		compSvc := componentmocks.NewMockService(t)
		compSvc.EXPECT().GetComponent(mock.Anything, testNS, testComponent).Return(existing, nil)
		compSvc.EXPECT().
			UpdateComponent(mock.Anything, testNS, mock.MatchedBy(func(c *openchoreov1alpha1.Component) bool {
				return len(c.Spec.Traits) == 1 && c.Spec.Traits[0].Name == "keep-me"
			})).
			Return(existing, nil)

		h := newTestHandler(withComponentService(compSvc))
		_, err := h.PatchComponent(ctx, testNS, testComponent, &gen.PatchComponentRequest{})
		require.NoError(t, err)
	})

	t.Run("replaces Workflow", func(t *testing.T) {
		existing := freshComponent()
		existing.Spec.Workflow = &openchoreov1alpha1.ComponentWorkflowConfig{Name: "old-builder"}
		compSvc := componentmocks.NewMockService(t)
		compSvc.EXPECT().GetComponent(mock.Anything, testNS, testComponent).Return(existing, nil)

		params := map[string]interface{}{"branch": "main"}
		expectedRaw, _ := json.Marshal(params)
		compSvc.EXPECT().
			UpdateComponent(mock.Anything, testNS, mock.MatchedBy(func(c *openchoreov1alpha1.Component) bool {
				return c.Spec.Workflow != nil &&
					c.Spec.Workflow.Name == "docker-build" &&
					c.Spec.Workflow.Kind == openchoreov1alpha1.WorkflowRefKind("ClusterWorkflow") &&
					c.Spec.Workflow.Parameters != nil &&
					string(c.Spec.Workflow.Parameters.Raw) == string(expectedRaw)
			})).
			Return(existing, nil)

		clusterKind := gen.ComponentWorkflowInputKindClusterWorkflow
		h := newTestHandler(withComponentService(compSvc))
		_, err := h.PatchComponent(ctx, testNS, testComponent, &gen.PatchComponentRequest{
			Workflow: &gen.ComponentWorkflowInput{
				Name:       "docker-build",
				Kind:       &clusterKind,
				Parameters: &params,
			},
		})
		require.NoError(t, err)
	})
}

// ---------------------------------------------------------------------------
// CreateWorkload
// ---------------------------------------------------------------------------

func TestCreateWorkload(t *testing.T) {
	ctx := context.Background()

	makeComponent := func(projectName string) *openchoreov1alpha1.Component {
		return &openchoreov1alpha1.Component{
			ObjectMeta: metav1.ObjectMeta{Name: testComponent},
			Spec: openchoreov1alpha1.ComponentSpec{
				Owner: openchoreov1alpha1.ComponentOwner{ProjectName: projectName},
			},
		}
	}
	makeWorkload := func() *openchoreov1alpha1.Workload {
		return &openchoreov1alpha1.Workload{ObjectMeta: metav1.ObjectMeta{Name: testComponent + "-workload"}}
	}

	t.Run("happy path: sets name and owner from component", func(t *testing.T) {
		compSvc := componentmocks.NewMockService(t)
		wlSvc := workloadmocks.NewMockService(t)
		compSvc.EXPECT().GetComponent(mock.Anything, testNS, testComponent).Return(makeComponent("proj1"), nil)
		wlSvc.EXPECT().
			CreateWorkload(mock.Anything, testNS, mock.MatchedBy(func(w *openchoreov1alpha1.Workload) bool {
				return w.Name == testComponent+"-workload" &&
					w.Spec.Owner.ProjectName == "proj1" &&
					w.Spec.Owner.ComponentName == testComponent
			})).
			Return(makeWorkload(), nil)

		h := newTestHandler(withComponentService(compSvc), withWorkloadService(wlSvc))
		spec := map[string]interface{}{"container": map[string]interface{}{"image": "nginx"}}
		_, err := h.CreateWorkload(ctx, testNS, testComponent, spec)
		require.NoError(t, err)
	})

	t.Run("component not found propagates error", func(t *testing.T) {
		compSvc := componentmocks.NewMockService(t)
		compSvc.EXPECT().GetComponent(mock.Anything, testNS, testComponent).Return(nil, errors.New("component not found"))

		h := newTestHandler(withComponentService(compSvc))
		_, err := h.CreateWorkload(ctx, testNS, testComponent, map[string]interface{}{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "component not found")
	})
}

// ---------------------------------------------------------------------------
// UpdateWorkload
// ---------------------------------------------------------------------------

func TestUpdateWorkload(t *testing.T) {
	ctx := context.Background()

	existingOwner := openchoreov1alpha1.WorkloadOwner{ProjectName: "proj1", ComponentName: "comp1"}
	existingWorkload := &openchoreov1alpha1.Workload{
		ObjectMeta: metav1.ObjectMeta{Name: "comp1-workload", Namespace: testNS},
		Spec: openchoreov1alpha1.WorkloadSpec{
			Owner: existingOwner,
		},
	}

	t.Run("preserves owner from existing workload", func(t *testing.T) {
		wlSvc := workloadmocks.NewMockService(t)
		wlSvc.EXPECT().GetWorkload(mock.Anything, testNS, "comp1-workload").Return(existingWorkload, nil)
		wlSvc.EXPECT().
			UpdateWorkload(mock.Anything, testNS, mock.MatchedBy(func(w *openchoreov1alpha1.Workload) bool {
				return w.Spec.Owner.ProjectName == "proj1" &&
					w.Spec.Owner.ComponentName == "comp1"
			})).
			Return(existingWorkload, nil)

		h := newTestHandler(withWorkloadService(wlSvc))
		// spec without owner — it should be injected from existing
		spec := map[string]interface{}{"container": map[string]interface{}{"image": "nginx:latest"}}
		_, err := h.UpdateWorkload(ctx, testNS, "comp1-workload", spec)
		require.NoError(t, err)
	})

	t.Run("GetWorkload error propagated", func(t *testing.T) {
		wlSvc := workloadmocks.NewMockService(t)
		wlSvc.EXPECT().GetWorkload(mock.Anything, testNS, "comp1-workload").Return(nil, errors.New("not found"))

		h := newTestHandler(withWorkloadService(wlSvc))
		_, err := h.UpdateWorkload(ctx, testNS, "comp1-workload", map[string]interface{}{})
		require.Error(t, err)
	})
}

// ---------------------------------------------------------------------------
// Delete handlers (Component, Workload, ComponentRelease)
// ---------------------------------------------------------------------------

func TestDeleteComponent(t *testing.T) {
	ctx := context.Background()

	t.Run("delete returns action: deleted", func(t *testing.T) {
		compSvc := componentmocks.NewMockService(t)
		compSvc.EXPECT().DeleteComponent(mock.Anything, testNS, testComponent).Return(nil)

		h := newTestHandler(withComponentService(compSvc))
		result, err := h.DeleteComponent(ctx, testNS, testComponent)
		require.NoError(t, err)
		m, ok := result.(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "deleted", m["action"])
		assert.Equal(t, testComponent, m["name"])
		assert.Equal(t, testNS, m["namespace"])
	})

	t.Run("service delete error propagated", func(t *testing.T) {
		expected := errors.New("not found")
		compSvc := componentmocks.NewMockService(t)
		compSvc.EXPECT().DeleteComponent(mock.Anything, testNS, testComponent).Return(expected)

		h := newTestHandler(withComponentService(compSvc))
		_, err := h.DeleteComponent(ctx, testNS, testComponent)
		require.ErrorIs(t, err, expected)
	})
}

func TestDeleteWorkload(t *testing.T) {
	ctx := context.Background()

	t.Run("delete returns action: deleted", func(t *testing.T) {
		wlSvc := workloadmocks.NewMockService(t)
		wlSvc.EXPECT().DeleteWorkload(mock.Anything, testNS, "comp1-workload").Return(nil)

		h := newTestHandler(withWorkloadService(wlSvc))
		result, err := h.DeleteWorkload(ctx, testNS, "comp1-workload")
		require.NoError(t, err)
		m, ok := result.(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "deleted", m["action"])
		assert.Equal(t, "comp1-workload", m["name"])
	})

	t.Run("service delete error propagated", func(t *testing.T) {
		expected := errors.New("not found")
		wlSvc := workloadmocks.NewMockService(t)
		wlSvc.EXPECT().DeleteWorkload(mock.Anything, testNS, "comp1-workload").Return(expected)

		h := newTestHandler(withWorkloadService(wlSvc))
		_, err := h.DeleteWorkload(ctx, testNS, "comp1-workload")
		require.ErrorIs(t, err, expected)
	})
}

func TestDeleteComponentRelease(t *testing.T) {
	ctx := context.Background()

	t.Run("delete returns action: deleted", func(t *testing.T) {
		crSvc := componentreleasemocks.NewMockService(t)
		crSvc.EXPECT().DeleteComponentRelease(mock.Anything, testNS, "release-1").Return(nil)

		h := newTestHandler(withComponentReleaseService(crSvc))
		result, err := h.DeleteComponentRelease(ctx, testNS, "release-1")
		require.NoError(t, err)
		m, ok := result.(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "deleted", m["action"])
		assert.Equal(t, "release-1", m["name"])
	})

	t.Run("service delete error propagated", func(t *testing.T) {
		expected := errors.New("not found")
		crSvc := componentreleasemocks.NewMockService(t)
		crSvc.EXPECT().DeleteComponentRelease(mock.Anything, testNS, "release-1").Return(expected)

		h := newTestHandler(withComponentReleaseService(crSvc))
		_, err := h.DeleteComponentRelease(ctx, testNS, "release-1")
		require.ErrorIs(t, err, expected)
	})
}

// ---------------------------------------------------------------------------
// CreateReleaseBinding
// ---------------------------------------------------------------------------

func TestCreateReleaseBinding(t *testing.T) {
	ctx := context.Background()

	makeRB := func() *openchoreov1alpha1.ReleaseBinding {
		return &openchoreov1alpha1.ReleaseBinding{ObjectMeta: metav1.ObjectMeta{Name: testComponent + "-dev"}}
	}

	baseReq := &gen.ReleaseBindingSpec{
		Environment: testEnvironmentName,
		Owner: struct {
			ComponentName string `json:"componentName"`
			ProjectName   string `json:"projectName"`
		}{ComponentName: testComponent, ProjectName: testProject},
	}

	t.Run("happy path: name, owner, environment set correctly", func(t *testing.T) {
		rbSvc := releasebindingmocks.NewMockService(t)
		rbSvc.EXPECT().
			CreateReleaseBinding(mock.Anything, testNS, mock.MatchedBy(func(rb *openchoreov1alpha1.ReleaseBinding) bool {
				return rb.Name == testComponent+"-"+testEnvironmentName &&
					rb.Spec.Environment == testEnvironmentName &&
					rb.Spec.Owner.ProjectName == testProject &&
					rb.Spec.Owner.ComponentName == testComponent
			})).
			Return(makeRB(), nil)

		h := newTestHandler(withReleaseBindingService(rbSvc))
		_, err := h.CreateReleaseBinding(ctx, testNS, baseReq)
		require.NoError(t, err)
	})

	t.Run("with ComponentTypeEnvironmentConfigs", func(t *testing.T) {
		rbSvc := releasebindingmocks.NewMockService(t)
		configs := map[string]interface{}{"key": "value"}
		expectedRaw, _ := json.Marshal(configs)
		rbSvc.EXPECT().
			CreateReleaseBinding(mock.Anything, testNS, mock.MatchedBy(func(rb *openchoreov1alpha1.ReleaseBinding) bool {
				return rb.Spec.ComponentTypeEnvironmentConfigs != nil &&
					string(rb.Spec.ComponentTypeEnvironmentConfigs.Raw) == string(expectedRaw)
			})).
			Return(makeRB(), nil)

		req := *baseReq
		req.ComponentTypeEnvironmentConfigs = &configs
		h := newTestHandler(withReleaseBindingService(rbSvc))
		_, err := h.CreateReleaseBinding(ctx, testNS, &req)
		require.NoError(t, err)
	})

	t.Run("with TraitEnvironmentConfigs", func(t *testing.T) {
		rbSvc := releasebindingmocks.NewMockService(t)
		traitVal := map[string]interface{}{"param": "val"}
		traitConfigs := map[string]interface{}{"my-trait": traitVal}
		rbSvc.EXPECT().
			CreateReleaseBinding(mock.Anything, testNS, mock.MatchedBy(func(rb *openchoreov1alpha1.ReleaseBinding) bool {
				return len(rb.Spec.TraitEnvironmentConfigs) == 1
			})).
			Return(makeRB(), nil)

		req := *baseReq
		req.TraitEnvironmentConfigs = &traitConfigs
		h := newTestHandler(withReleaseBindingService(rbSvc))
		_, err := h.CreateReleaseBinding(ctx, testNS, &req)
		require.NoError(t, err)
	})

	t.Run("service error propagated", func(t *testing.T) {
		rbSvc := releasebindingmocks.NewMockService(t)
		rbSvc.EXPECT().CreateReleaseBinding(mock.Anything, testNS, mock.Anything).Return(nil, errors.New("create failed"))

		h := newTestHandler(withReleaseBindingService(rbSvc))
		_, err := h.CreateReleaseBinding(ctx, testNS, baseReq)
		require.Error(t, err)
	})
}

// ---------------------------------------------------------------------------
// UpdateReleaseBinding
// ---------------------------------------------------------------------------

func TestUpdateReleaseBinding(t *testing.T) {
	ctx := context.Background()

	existingRB := &openchoreov1alpha1.ReleaseBinding{
		ObjectMeta: metav1.ObjectMeta{Name: "comp-dev", Namespace: testNS},
		Spec: openchoreov1alpha1.ReleaseBindingSpec{
			Environment: testEnvironmentName,
		},
	}

	t.Run("environment immutability: different value returns error", func(t *testing.T) {
		rbSvc := releasebindingmocks.NewMockService(t)
		rbSvc.EXPECT().GetReleaseBinding(mock.Anything, testNS, "comp-dev").Return(existingRB, nil)

		h := newTestHandler(withReleaseBindingService(rbSvc))
		req := &gen.ReleaseBindingSpec{Environment: "staging"}
		_, err := h.UpdateReleaseBinding(ctx, testNS, "comp-dev", req)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "release binding environment is immutable")
	})

	t.Run("same environment: no error", func(t *testing.T) {
		rbSvc := releasebindingmocks.NewMockService(t)
		rbSvc.EXPECT().GetReleaseBinding(mock.Anything, testNS, "comp-dev").Return(existingRB, nil)
		rbSvc.EXPECT().UpdateReleaseBinding(mock.Anything, testNS, mock.Anything).Return(existingRB, nil)

		h := newTestHandler(withReleaseBindingService(rbSvc))
		req := &gen.ReleaseBindingSpec{Environment: testEnvironmentName}
		_, err := h.UpdateReleaseBinding(ctx, testNS, "comp-dev", req)
		require.NoError(t, err)
	})

	t.Run("updates ReleaseName", func(t *testing.T) {
		rbSvc := releasebindingmocks.NewMockService(t)
		rbSvc.EXPECT().GetReleaseBinding(mock.Anything, testNS, "comp-dev").Return(existingRB, nil)
		releaseName := "v1.2.0"
		rbSvc.EXPECT().
			UpdateReleaseBinding(mock.Anything, testNS, mock.MatchedBy(func(rb *openchoreov1alpha1.ReleaseBinding) bool {
				return rb.Spec.ReleaseName == releaseName
			})).
			Return(existingRB, nil)

		h := newTestHandler(withReleaseBindingService(rbSvc))
		req := &gen.ReleaseBindingSpec{Environment: testEnvironmentName, ReleaseName: &releaseName}
		_, err := h.UpdateReleaseBinding(ctx, testNS, "comp-dev", req)
		require.NoError(t, err)
	})

	t.Run("GetReleaseBinding error propagated", func(t *testing.T) {
		rbSvc := releasebindingmocks.NewMockService(t)
		rbSvc.EXPECT().GetReleaseBinding(mock.Anything, testNS, "comp-dev").Return(nil, errors.New("not found"))

		h := newTestHandler(withReleaseBindingService(rbSvc))
		req := &gen.ReleaseBindingSpec{Environment: testEnvironmentName}
		_, err := h.UpdateReleaseBinding(ctx, testNS, "comp-dev", req)
		require.Error(t, err)
	})

	t.Run("sets release state when provided", func(t *testing.T) {
		rbSvc := releasebindingmocks.NewMockService(t)
		rbSvc.EXPECT().GetReleaseBinding(mock.Anything, testNS, "comp-dev").Return(existingRB, nil)
		rbSvc.EXPECT().
			UpdateReleaseBinding(mock.Anything, testNS, mock.MatchedBy(func(rb *openchoreov1alpha1.ReleaseBinding) bool {
				return rb.Spec.State == openchoreov1alpha1.ReleaseState("Undeploy")
			})).
			Return(existingRB, nil)

		h := newTestHandler(withReleaseBindingService(rbSvc))
		state := gen.ReleaseBindingSpecState("Undeploy")
		req := &gen.ReleaseBindingSpec{Environment: testEnvironmentName, State: &state}
		_, err := h.UpdateReleaseBinding(ctx, testNS, "comp-dev", req)
		require.NoError(t, err)
	})

	t.Run("delete returns action: deleted", func(t *testing.T) {
		rbSvc := releasebindingmocks.NewMockService(t)
		rbSvc.EXPECT().DeleteReleaseBinding(mock.Anything, testNS, "comp-dev").Return(nil)

		h := newTestHandler(withReleaseBindingService(rbSvc))
		result, err := h.DeleteReleaseBinding(ctx, testNS, "comp-dev")
		require.NoError(t, err)
		m, ok := result.(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "deleted", m["action"])
		assert.Equal(t, "comp-dev", m["name"])
	})

	t.Run("delete error propagated", func(t *testing.T) {
		expected := errors.New("conflict")
		rbSvc := releasebindingmocks.NewMockService(t)
		rbSvc.EXPECT().DeleteReleaseBinding(mock.Anything, testNS, "comp-dev").Return(expected)

		h := newTestHandler(withReleaseBindingService(rbSvc))
		_, err := h.DeleteReleaseBinding(ctx, testNS, "comp-dev")
		require.ErrorIs(t, err, expected)
	})

	t.Run("nil state leaves existing state unchanged", func(t *testing.T) {
		rbWithState := &openchoreov1alpha1.ReleaseBinding{
			ObjectMeta: metav1.ObjectMeta{Name: "comp-dev"},
			Spec: openchoreov1alpha1.ReleaseBindingSpec{
				Environment: testEnvironmentName,
				State:       openchoreov1alpha1.ReleaseState("Active"),
			},
		}
		rbSvc := releasebindingmocks.NewMockService(t)
		rbSvc.EXPECT().GetReleaseBinding(mock.Anything, testNS, "comp-dev").Return(rbWithState, nil)
		rbSvc.EXPECT().
			UpdateReleaseBinding(mock.Anything, testNS, mock.MatchedBy(func(rb *openchoreov1alpha1.ReleaseBinding) bool {
				return rb.Spec.State == openchoreov1alpha1.ReleaseState("Active")
			})).
			Return(rbWithState, nil)

		h := newTestHandler(withReleaseBindingService(rbSvc))
		req := &gen.ReleaseBindingSpec{Environment: testEnvironmentName}
		_, err := h.UpdateReleaseBinding(ctx, testNS, "comp-dev", req)
		require.NoError(t, err)
	})
}

// ---------------------------------------------------------------------------
// TriggerWorkflowRun
// ---------------------------------------------------------------------------

func TestTriggerWorkflowRun(t *testing.T) {
	ctx := context.Background()

	makeComponentWithWorkflow := func(wfName string, params *runtime.RawExtension) *openchoreov1alpha1.Component {
		return &openchoreov1alpha1.Component{
			ObjectMeta: metav1.ObjectMeta{Name: testComponent},
			Spec: openchoreov1alpha1.ComponentSpec{
				Owner: openchoreov1alpha1.ComponentOwner{ProjectName: testProject},
				Workflow: &openchoreov1alpha1.ComponentWorkflowConfig{
					Name:       wfName,
					Parameters: params,
				},
			},
		}
	}
	makeCreatedRun := func() *openchoreov1alpha1.WorkflowRun {
		return &openchoreov1alpha1.WorkflowRun{ObjectMeta: metav1.ObjectMeta{Name: testComponent + "-workflow-abc"}}
	}

	t.Run("with commit and existing params: commit injected", func(t *testing.T) {
		existingParams := map[string]interface{}{"branch": "main"}
		raw, _ := json.Marshal(existingParams)
		comp := makeComponentWithWorkflow("build-wf", &runtime.RawExtension{Raw: raw})

		compSvc := componentmocks.NewMockService(t)
		wfrSvc := workflowrunmocks.NewMockService(t)
		compSvc.EXPECT().GetComponent(mock.Anything, testNS, testComponent).Return(comp, nil)
		wfrSvc.EXPECT().
			CreateWorkflowRun(mock.Anything, testNS, mock.MatchedBy(func(wr *openchoreov1alpha1.WorkflowRun) bool {
				if wr.Spec.Workflow.Parameters == nil {
					return false
				}
				var params map[string]interface{}
				_ = json.Unmarshal(wr.Spec.Workflow.Parameters.Raw, &params)
				return params["commit"] == "abc123" && params["branch"] == "main"
			})).
			Return(makeCreatedRun(), nil)

		h := newTestHandler(withComponentService(compSvc), withWorkflowRunService(wfrSvc))
		_, err := h.TriggerWorkflowRun(ctx, testNS, testProject, testComponent, "abc123")
		require.NoError(t, err)
	})

	t.Run("with commit but no existing params", func(t *testing.T) {
		comp := makeComponentWithWorkflow("build-wf", nil)

		compSvc := componentmocks.NewMockService(t)
		wfrSvc := workflowrunmocks.NewMockService(t)
		compSvc.EXPECT().GetComponent(mock.Anything, testNS, testComponent).Return(comp, nil)
		wfrSvc.EXPECT().
			CreateWorkflowRun(mock.Anything, testNS, mock.MatchedBy(func(wr *openchoreov1alpha1.WorkflowRun) bool {
				if wr.Spec.Workflow.Parameters == nil {
					return false
				}
				var params map[string]interface{}
				_ = json.Unmarshal(wr.Spec.Workflow.Parameters.Raw, &params)
				return params["commit"] == "abc123" && len(params) == 1
			})).
			Return(makeCreatedRun(), nil)

		h := newTestHandler(withComponentService(compSvc), withWorkflowRunService(wfrSvc))
		_, err := h.TriggerWorkflowRun(ctx, testNS, testProject, testComponent, "abc123")
		require.NoError(t, err)
	})

	t.Run("without commit: params passed through", func(t *testing.T) {
		existingParams := map[string]interface{}{"branch": "main"}
		raw, _ := json.Marshal(existingParams)
		comp := makeComponentWithWorkflow("build-wf", &runtime.RawExtension{Raw: raw})

		compSvc := componentmocks.NewMockService(t)
		wfrSvc := workflowrunmocks.NewMockService(t)
		compSvc.EXPECT().GetComponent(mock.Anything, testNS, testComponent).Return(comp, nil)
		wfrSvc.EXPECT().
			CreateWorkflowRun(mock.Anything, testNS, mock.MatchedBy(func(wr *openchoreov1alpha1.WorkflowRun) bool {
				if wr.Spec.Workflow.Parameters == nil {
					return false
				}
				var params map[string]interface{}
				_ = json.Unmarshal(wr.Spec.Workflow.Parameters.Raw, &params)
				_, hasCommit := params["commit"]
				return params["branch"] == "main" && !hasCommit
			})).
			Return(makeCreatedRun(), nil)

		h := newTestHandler(withComponentService(compSvc), withWorkflowRunService(wfrSvc))
		_, err := h.TriggerWorkflowRun(ctx, testNS, testProject, testComponent, "")
		require.NoError(t, err)
	})

	t.Run("no workflow configured: nil Workflow", func(t *testing.T) {
		comp := &openchoreov1alpha1.Component{
			ObjectMeta: metav1.ObjectMeta{Name: testComponent},
			Spec:       openchoreov1alpha1.ComponentSpec{},
		}
		compSvc := componentmocks.NewMockService(t)
		compSvc.EXPECT().GetComponent(mock.Anything, testNS, testComponent).Return(comp, nil)

		h := newTestHandler(withComponentService(compSvc))
		_, err := h.TriggerWorkflowRun(ctx, testNS, testProject, testComponent, "abc123")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "does not have a workflow configured")
	})

	t.Run("no workflow configured: empty Name", func(t *testing.T) {
		comp := makeComponentWithWorkflow("", nil)
		compSvc := componentmocks.NewMockService(t)
		compSvc.EXPECT().GetComponent(mock.Anything, testNS, testComponent).Return(comp, nil)

		h := newTestHandler(withComponentService(compSvc))
		_, err := h.TriggerWorkflowRun(ctx, testNS, testProject, testComponent, "abc123")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "does not have a workflow configured")
	})

	t.Run("labels and GenerateName set correctly", func(t *testing.T) {
		comp := makeComponentWithWorkflow("build-wf", nil)
		compSvc := componentmocks.NewMockService(t)
		wfrSvc := workflowrunmocks.NewMockService(t)
		compSvc.EXPECT().GetComponent(mock.Anything, testNS, testComponent).Return(comp, nil)
		wfrSvc.EXPECT().
			CreateWorkflowRun(mock.Anything, testNS, mock.MatchedBy(func(wr *openchoreov1alpha1.WorkflowRun) bool {
				return wr.Labels["openchoreo.dev/project"] == testProject &&
					wr.Labels["openchoreo.dev/component"] == testComponent &&
					wr.GenerateName == testComponent+"-workflow-"
			})).
			Return(makeCreatedRun(), nil)

		h := newTestHandler(withComponentService(compSvc), withWorkflowRunService(wfrSvc))
		_, err := h.TriggerWorkflowRun(ctx, testNS, testProject, testComponent, "")
		require.NoError(t, err)
	})
}
