// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package component

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/labels"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
	projectsvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/project"
)

// --- Test helpers ---

func newScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	require.NoError(t, openchoreov1alpha1.AddToScheme(s))
	return s
}

func newService(t *testing.T, objs ...client.Object) Service {
	t.Helper()
	scheme := newScheme(t)
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...).Build()
	return NewService(k8sClient, testLogger())
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(nopWriter{}, nil))
}

type nopWriter struct{}

func (nopWriter) Write(p []byte) (int, error) { return len(p), nil }

const (
	testNamespace     = "test-ns"
	testProjectName   = "test-project"
	testComponentName = "test-comp"
	testPipelineName  = "test-pipeline"
)

func testProject() *openchoreov1alpha1.Project {
	return &openchoreov1alpha1.Project{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testProjectName,
			Namespace: testNamespace,
		},
		Spec: openchoreov1alpha1.ProjectSpec{
			DeploymentPipelineRef: openchoreov1alpha1.DeploymentPipelineRef{
				Name: testPipelineName,
			},
		},
	}
}

func testComponent() *openchoreov1alpha1.Component {
	return &openchoreov1alpha1.Component{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testComponentName,
			Namespace: testNamespace,
			Labels: map[string]string{
				labels.LabelKeyProjectName: testProjectName,
			},
		},
		Spec: openchoreov1alpha1.ComponentSpec{
			Owner: openchoreov1alpha1.ComponentOwner{
				ProjectName: testProjectName,
			},
			ComponentType: openchoreov1alpha1.ComponentTypeRef{
				Name: "deployment/web-app",
			},
		},
	}
}

func testDeploymentPipeline() *openchoreov1alpha1.DeploymentPipeline {
	return &openchoreov1alpha1.DeploymentPipeline{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testPipelineName,
			Namespace: testNamespace,
		},
		Spec: openchoreov1alpha1.DeploymentPipelineSpec{
			PromotionPaths: []openchoreov1alpha1.PromotionPath{
				{
					SourceEnvironmentRef: openchoreov1alpha1.EnvironmentRef{Name: "dev"},
					TargetEnvironmentRefs: []openchoreov1alpha1.TargetEnvironmentRef{
						{Name: "staging"},
					},
				},
				{
					SourceEnvironmentRef: openchoreov1alpha1.EnvironmentRef{Name: "staging"},
					TargetEnvironmentRefs: []openchoreov1alpha1.TargetEnvironmentRef{
						{Name: "prod"},
					},
				},
			},
		},
	}
}

func testComponentType() *openchoreov1alpha1.ComponentType {
	return &openchoreov1alpha1.ComponentType{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "web-app",
			Namespace: testNamespace,
		},
		Spec: openchoreov1alpha1.ComponentTypeSpec{
			WorkloadType: "deployment",
		},
	}
}

func testWorkload() *openchoreov1alpha1.Workload {
	return &openchoreov1alpha1.Workload{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testComponentName + "-workload",
			Namespace: testNamespace,
		},
		Spec: openchoreov1alpha1.WorkloadSpec{
			Owner: openchoreov1alpha1.WorkloadOwner{
				ProjectName:   testProjectName,
				ComponentName: testComponentName,
			},
			WorkloadTemplateSpec: openchoreov1alpha1.WorkloadTemplateSpec{
				Container: openchoreov1alpha1.Container{
					Image: "nginx:latest",
				},
			},
		},
	}
}

func rawJSON(t *testing.T, v any) *runtime.RawExtension {
	t.Helper()
	data, err := json.Marshal(v)
	require.NoError(t, err)
	return &runtime.RawExtension{Raw: data}
}

// hasReleaseTrait returns true if traits contains an entry with the given name (any kind).
func hasReleaseTrait(traits []openchoreov1alpha1.ComponentReleaseTrait, name string) bool {
	for _, t := range traits {
		if t.Name == name {
			return true
		}
	}
	return false
}

// --- Pure Functions ---

func TestParseComponentTypeName(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		expected  string
		expectErr bool
	}{
		{name: "valid", input: "deployment/web-app", expected: "web-app"},
		{name: "no slash", input: "web-app", expectErr: true},
		{name: "empty name after slash", input: "deployment/", expectErr: true},
		{name: "leading slash - empty workload type", input: "/web-app", expected: "web-app"},
		{name: "multiple slashes", input: "a/b/c", expectErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseComponentTypeName(tc.input)
			if tc.expectErr {
				require.Error(t, err)
				assert.Empty(t, got)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tc.expected, got)
			}
		})
	}
}

func TestBuildTraitEnvironmentConfigsSchema(t *testing.T) {
	tests := []struct {
		name      string
		traitSpec openchoreov1alpha1.TraitSpec
		expectNil bool
		expectErr bool
	}{
		{
			name:      "nil environmentConfigs",
			traitSpec: openchoreov1alpha1.TraitSpec{},
			expectNil: true,
		},
		{
			name: "empty raw",
			traitSpec: openchoreov1alpha1.TraitSpec{
				EnvironmentConfigs: &openchoreov1alpha1.SchemaSection{},
			},
			expectNil: true,
		},
		{
			name: "valid schema",
			traitSpec: openchoreov1alpha1.TraitSpec{
				EnvironmentConfigs: &openchoreov1alpha1.SchemaSection{
					OpenAPIV3Schema: rawJSON(t, map[string]any{"replicas": "integer"}),
				},
			},
			expectNil: false,
		},
		{
			name: "invalid schema",
			traitSpec: openchoreov1alpha1.TraitSpec{
				EnvironmentConfigs: &openchoreov1alpha1.SchemaSection{
					OpenAPIV3Schema: &runtime.RawExtension{Raw: []byte(`{not valid json}`)},
				},
			},
			expectErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := buildTraitEnvironmentConfigsSchema(tc.traitSpec, "test-trait")
			if tc.expectErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			if tc.expectNil {
				assert.Nil(t, got)
			} else {
				assert.NotNil(t, got)
			}
		})
	}
}

// --- CRUD Operations ---

func TestCreateComponent(t *testing.T) {
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		svc := newService(t, testProject())
		comp := &openchoreov1alpha1.Component{
			ObjectMeta: metav1.ObjectMeta{Name: "new-comp"},
			Spec: openchoreov1alpha1.ComponentSpec{
				Owner:         openchoreov1alpha1.ComponentOwner{ProjectName: testProjectName},
				ComponentType: openchoreov1alpha1.ComponentTypeRef{Name: "deployment/web-app"},
			},
		}

		result, err := svc.CreateComponent(ctx, testNamespace, comp)
		require.NoError(t, err)
		assert.Equal(t, componentTypeMeta, result.TypeMeta)
		assert.Equal(t, testProjectName, result.Labels[labels.LabelKeyProjectName])
	})

	t.Run("nil input", func(t *testing.T) {
		svc := newService(t, testProject())
		_, err := svc.CreateComponent(ctx, testNamespace, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cannot be nil")
	})

	t.Run("project not found", func(t *testing.T) {
		svc := newService(t) // no project seeded
		comp := &openchoreov1alpha1.Component{
			ObjectMeta: metav1.ObjectMeta{Name: "comp"},
			Spec: openchoreov1alpha1.ComponentSpec{
				Owner: openchoreov1alpha1.ComponentOwner{ProjectName: "nonexistent"},
			},
		}

		_, err := svc.CreateComponent(ctx, testNamespace, comp)
		require.Error(t, err)
		assert.ErrorIs(t, err, projectsvc.ErrProjectNotFound)
	})

	t.Run("already exists", func(t *testing.T) {
		existing := testComponent()
		svc := newService(t, testProject(), existing)
		dup := &openchoreov1alpha1.Component{
			ObjectMeta: metav1.ObjectMeta{Name: testComponentName},
			Spec: openchoreov1alpha1.ComponentSpec{
				Owner:         openchoreov1alpha1.ComponentOwner{ProjectName: testProjectName},
				ComponentType: openchoreov1alpha1.ComponentTypeRef{Name: "deployment/web-app"},
			},
		}

		_, err := svc.CreateComponent(ctx, testNamespace, dup)
		require.ErrorIs(t, err, ErrComponentAlreadyExists)
	})

	t.Run("sets project label when labels are nil", func(t *testing.T) {
		svc := newService(t, testProject())
		comp := &openchoreov1alpha1.Component{
			ObjectMeta: metav1.ObjectMeta{Name: "label-test"},
			Spec: openchoreov1alpha1.ComponentSpec{
				Owner:         openchoreov1alpha1.ComponentOwner{ProjectName: testProjectName},
				ComponentType: openchoreov1alpha1.ComponentTypeRef{Name: "deployment/web-app"},
			},
		}

		result, err := svc.CreateComponent(ctx, testNamespace, comp)
		require.NoError(t, err)
		assert.Equal(t, testProjectName, result.Labels[labels.LabelKeyProjectName])
	})
}

func TestGetComponent(t *testing.T) {
	ctx := context.Background()

	t.Run("found", func(t *testing.T) {
		svc := newService(t, testComponent())
		result, err := svc.GetComponent(ctx, testNamespace, testComponentName)
		require.NoError(t, err)
		assert.Equal(t, componentTypeMeta, result.TypeMeta)
		assert.Equal(t, testComponentName, result.Name)
	})

	t.Run("not found", func(t *testing.T) {
		svc := newService(t)
		_, err := svc.GetComponent(ctx, testNamespace, "nonexistent")
		require.ErrorIs(t, err, ErrComponentNotFound)
	})
}

func TestUpdateComponent(t *testing.T) {
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		existing := testComponent()
		svc := newService(t, testProject(), existing)

		update := &openchoreov1alpha1.Component{
			ObjectMeta: metav1.ObjectMeta{Name: testComponentName},
			Spec: openchoreov1alpha1.ComponentSpec{
				Owner:         openchoreov1alpha1.ComponentOwner{ProjectName: testProjectName},
				ComponentType: openchoreov1alpha1.ComponentTypeRef{Name: "deployment/web-app"},
				AutoDeploy:    true,
			},
		}

		result, err := svc.UpdateComponent(ctx, testNamespace, update)
		require.NoError(t, err)
		assert.Equal(t, componentTypeMeta, result.TypeMeta)
		assert.True(t, result.Spec.AutoDeploy)
	})

	t.Run("not found", func(t *testing.T) {
		svc := newService(t, testProject())
		update := &openchoreov1alpha1.Component{
			ObjectMeta: metav1.ObjectMeta{Name: "nonexistent"},
			Spec: openchoreov1alpha1.ComponentSpec{
				Owner: openchoreov1alpha1.ComponentOwner{ProjectName: testProjectName},
			},
		}
		_, err := svc.UpdateComponent(ctx, testNamespace, update)
		require.ErrorIs(t, err, ErrComponentNotFound)
	})

	t.Run("nil input", func(t *testing.T) {
		svc := newService(t, testProject())
		_, err := svc.UpdateComponent(ctx, testNamespace, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cannot be nil")
	})

	t.Run("immutable projectName", func(t *testing.T) {
		existing := testComponent()
		svc := newService(t, testProject(), existing)

		update := &openchoreov1alpha1.Component{
			ObjectMeta: metav1.ObjectMeta{Name: testComponentName},
			Spec: openchoreov1alpha1.ComponentSpec{
				Owner:         openchoreov1alpha1.ComponentOwner{ProjectName: "different-project"},
				ComponentType: openchoreov1alpha1.ComponentTypeRef{Name: "deployment/web-app"},
			},
		}

		_, err := svc.UpdateComponent(ctx, testNamespace, update)
		require.Error(t, err)
		var validationErr *services.ValidationError
		require.True(t, errors.As(err, &validationErr))
		assert.Equal(t, "spec.owner.projectName is immutable", validationErr.Msg)
	})

	t.Run("preserves project label", func(t *testing.T) {
		existing := testComponent()
		svc := newService(t, testProject(), existing)

		update := &openchoreov1alpha1.Component{
			ObjectMeta: metav1.ObjectMeta{Name: testComponentName},
			Spec: openchoreov1alpha1.ComponentSpec{
				Owner:         openchoreov1alpha1.ComponentOwner{ProjectName: testProjectName},
				ComponentType: openchoreov1alpha1.ComponentTypeRef{Name: "deployment/web-app"},
			},
		}

		result, err := svc.UpdateComponent(ctx, testNamespace, update)
		require.NoError(t, err)
		assert.Equal(t, testProjectName, result.Labels[labels.LabelKeyProjectName])
	})
}

func TestDeleteComponent(t *testing.T) {
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		svc := newService(t, testComponent())
		err := svc.DeleteComponent(ctx, testNamespace, testComponentName)
		require.NoError(t, err)

		_, err = svc.GetComponent(ctx, testNamespace, testComponentName)
		require.ErrorIs(t, err, ErrComponentNotFound)
	})

	t.Run("not found", func(t *testing.T) {
		svc := newService(t)
		err := svc.DeleteComponent(ctx, testNamespace, "nonexistent")
		require.ErrorIs(t, err, ErrComponentNotFound)
	})
}

// --- Orchestration Flows ---

func tier3SeedObjects() []client.Object {
	return []client.Object{
		testProject(),
		testDeploymentPipeline(),
		testComponent(),
		testComponentType(),
		testWorkload(),
	}
}

func TestGenerateRelease(t *testing.T) {
	ctx := context.Background()

	t.Run("success with explicit name", func(t *testing.T) {
		svc := newService(t, tier3SeedObjects()...)

		result, err := svc.GenerateRelease(ctx, testNamespace, testComponentName, &GenerateReleaseRequest{ReleaseName: "v1"})
		require.NoError(t, err)
		assert.Equal(t, componentReleaseTypeMeta, result.TypeMeta)
		assert.Equal(t, "v1", result.Name)
		assert.Equal(t, testProjectName, result.Spec.Owner.ProjectName)
		assert.Equal(t, testComponentName, result.Spec.Owner.ComponentName)
		assert.Equal(t, "deployment", result.Spec.ComponentType.WorkloadType)
		assert.Equal(t, "nginx:latest", result.Spec.Workload.Container.Image)
		assert.Equal(t, testProjectName, result.Labels[labels.LabelKeyProjectName])
		assert.Equal(t, testComponentName, result.Labels[labels.LabelKeyComponentName])
	})

	t.Run("success with auto-generated name", func(t *testing.T) {
		svc := newService(t, tier3SeedObjects()...)

		result, err := svc.GenerateRelease(ctx, testNamespace, testComponentName, &GenerateReleaseRequest{ReleaseName: ""})
		require.NoError(t, err)
		assert.Contains(t, result.Name, testComponentName+"-")
		// Name should match pattern: test-comp-YYYYMMDD-N
		assert.Regexp(t, `^test-comp-\d{8}-\d+$`, result.Name)
	})

	t.Run("with traits", func(t *testing.T) {
		trait := &openchoreov1alpha1.Trait{
			ObjectMeta: metav1.ObjectMeta{Name: "ingress", Namespace: testNamespace},
			Spec: openchoreov1alpha1.TraitSpec{
				Parameters: &openchoreov1alpha1.SchemaSection{
					OpenAPIV3Schema: rawJSON(t, map[string]any{"host": "string"}),
				},
			},
		}
		compWithTrait := testComponent()
		compWithTrait.Spec.Traits = []openchoreov1alpha1.ComponentTrait{
			{Kind: openchoreov1alpha1.TraitRefKindTrait, Name: "ingress", InstanceName: "my-ingress"},
		}
		ct := testComponentType()
		ct.Spec.AllowedTraits = []openchoreov1alpha1.TraitRef{
			{Kind: openchoreov1alpha1.TraitRefKindTrait, Name: "ingress"},
		}

		objs := []client.Object{testProject(), testDeploymentPipeline(), compWithTrait, ct, testWorkload(), trait}
		svc := newService(t, objs...)

		result, err := svc.GenerateRelease(ctx, testNamespace, testComponentName, &GenerateReleaseRequest{ReleaseName: "v1"})
		require.NoError(t, err)
		require.Len(t, result.Spec.Traits, 1)
		assert.Equal(t, openchoreov1alpha1.TraitRefKindTrait, result.Spec.Traits[0].Kind)
		assert.Equal(t, "ingress", result.Spec.Traits[0].Name)
		assert.NotNil(t, result.Spec.ComponentProfile)
		assert.Len(t, result.Spec.ComponentProfile.Traits, 1)
		assert.Equal(t, openchoreov1alpha1.TraitRefKindTrait, result.Spec.ComponentProfile.Traits[0].Kind)
		assert.Equal(t, "ingress", result.Spec.ComponentProfile.Traits[0].Name)
	})

	t.Run("component not found", func(t *testing.T) {
		svc := newService(t, testProject())
		_, err := svc.GenerateRelease(ctx, testNamespace, "nonexistent", &GenerateReleaseRequest{ReleaseName: "v1"})
		require.ErrorIs(t, err, ErrComponentNotFound)
	})

	t.Run("workload not found", func(t *testing.T) {
		// Seed component but no workload
		objs := []client.Object{testProject(), testComponent(), testComponentType()}
		svc := newService(t, objs...)

		_, err := svc.GenerateRelease(ctx, testNamespace, testComponentName, &GenerateReleaseRequest{ReleaseName: "v1"})
		require.ErrorIs(t, err, ErrWorkloadNotFound)
	})

	t.Run("ClusterComponentType", func(t *testing.T) {
		cct := &openchoreov1alpha1.ClusterComponentType{
			ObjectMeta: metav1.ObjectMeta{Name: "cluster-web"},
			Spec: openchoreov1alpha1.ClusterComponentTypeSpec{
				WorkloadType: "deployment",
			},
		}
		comp := testComponent()
		comp.Spec.ComponentType = openchoreov1alpha1.ComponentTypeRef{
			Kind: openchoreov1alpha1.ComponentTypeRefKindClusterComponentType,
			Name: "deployment/cluster-web",
		}
		objs := []client.Object{testProject(), testDeploymentPipeline(), comp, cct, testWorkload()}
		svc := newService(t, objs...)

		result, err := svc.GenerateRelease(ctx, testNamespace, testComponentName, &GenerateReleaseRequest{ReleaseName: "v1"})
		require.NoError(t, err)
		assert.Equal(t, "deployment", result.Spec.ComponentType.WorkloadType)
	})

	t.Run("with embedded traits from ComponentType", func(t *testing.T) {
		embeddedTrait := &openchoreov1alpha1.Trait{
			ObjectMeta: metav1.ObjectMeta{Name: "storage", Namespace: testNamespace},
			Spec: openchoreov1alpha1.TraitSpec{
				Parameters: &openchoreov1alpha1.SchemaSection{
					OpenAPIV3Schema: rawJSON(t, map[string]any{"mountPath": "string"}),
				},
			},
		}
		ct := testComponentType()
		ct.Spec.Traits = []openchoreov1alpha1.ComponentTypeTrait{
			{Kind: openchoreov1alpha1.TraitRefKindTrait, Name: "storage", InstanceName: "app-storage"},
		}

		objs := []client.Object{testProject(), testDeploymentPipeline(), testComponent(), ct, testWorkload(), embeddedTrait}
		svc := newService(t, objs...)

		result, err := svc.GenerateRelease(ctx, testNamespace, testComponentName, &GenerateReleaseRequest{ReleaseName: "v1"})
		require.NoError(t, err)
		assert.True(t, hasReleaseTrait(result.Spec.Traits, "storage"), "expected trait 'storage' in Spec.Traits")
	})

	t.Run("with embedded ClusterTrait from ComponentType", func(t *testing.T) {
		clusterTrait := &openchoreov1alpha1.ClusterTrait{
			ObjectMeta: metav1.ObjectMeta{Name: "observability"},
			Spec: openchoreov1alpha1.ClusterTraitSpec{
				Parameters: &openchoreov1alpha1.SchemaSection{
					OpenAPIV3Schema: rawJSON(t, map[string]any{"enabled": "boolean"}),
				},
			},
		}
		ct := testComponentType()
		ct.Spec.Traits = []openchoreov1alpha1.ComponentTypeTrait{
			{Kind: openchoreov1alpha1.TraitRefKindClusterTrait, Name: "observability", InstanceName: "obs"},
		}

		objs := []client.Object{testProject(), testDeploymentPipeline(), testComponent(), ct, testWorkload(), clusterTrait}
		svc := newService(t, objs...)

		result, err := svc.GenerateRelease(ctx, testNamespace, testComponentName, &GenerateReleaseRequest{ReleaseName: "v1"})
		require.NoError(t, err)
		assert.True(t, hasReleaseTrait(result.Spec.Traits, "observability"), "expected trait 'observability' in Spec.Traits")
	})

	t.Run("with both embedded and component-level traits", func(t *testing.T) {
		embeddedTrait := &openchoreov1alpha1.Trait{
			ObjectMeta: metav1.ObjectMeta{Name: "storage", Namespace: testNamespace},
			Spec: openchoreov1alpha1.TraitSpec{
				Parameters: &openchoreov1alpha1.SchemaSection{
					OpenAPIV3Schema: rawJSON(t, map[string]any{"mountPath": "string"}),
				},
			},
		}
		componentTrait := &openchoreov1alpha1.Trait{
			ObjectMeta: metav1.ObjectMeta{Name: "ingress", Namespace: testNamespace},
			Spec: openchoreov1alpha1.TraitSpec{
				Parameters: &openchoreov1alpha1.SchemaSection{
					OpenAPIV3Schema: rawJSON(t, map[string]any{"host": "string"}),
				},
			},
		}
		ct := testComponentType()
		ct.Spec.Traits = []openchoreov1alpha1.ComponentTypeTrait{
			{Kind: openchoreov1alpha1.TraitRefKindTrait, Name: "storage", InstanceName: "app-storage"},
		}
		ct.Spec.AllowedTraits = []openchoreov1alpha1.TraitRef{
			{Kind: openchoreov1alpha1.TraitRefKindTrait, Name: "ingress"},
		}
		comp := testComponent()
		comp.Spec.Traits = []openchoreov1alpha1.ComponentTrait{
			{Kind: openchoreov1alpha1.TraitRefKindTrait, Name: "ingress", InstanceName: "my-ingress"},
		}

		objs := []client.Object{testProject(), testDeploymentPipeline(), comp, ct, testWorkload(), embeddedTrait, componentTrait}
		svc := newService(t, objs...)

		result, err := svc.GenerateRelease(ctx, testNamespace, testComponentName, &GenerateReleaseRequest{ReleaseName: "v1"})
		require.NoError(t, err)
		assert.True(t, hasReleaseTrait(result.Spec.Traits, "storage"), "expected trait 'storage' in Spec.Traits")
		assert.True(t, hasReleaseTrait(result.Spec.Traits, "ingress"), "expected trait 'ingress' in Spec.Traits")
		assert.Len(t, result.Spec.Traits, 2)
	})

	t.Run("ClusterComponentType with embedded ClusterTrait", func(t *testing.T) {
		clusterTrait := &openchoreov1alpha1.ClusterTrait{
			ObjectMeta: metav1.ObjectMeta{Name: "networking"},
			Spec: openchoreov1alpha1.ClusterTraitSpec{
				Parameters: &openchoreov1alpha1.SchemaSection{
					OpenAPIV3Schema: rawJSON(t, map[string]any{"port": "integer"}),
				},
			},
		}
		cct := &openchoreov1alpha1.ClusterComponentType{
			ObjectMeta: metav1.ObjectMeta{Name: "cluster-web"},
			Spec: openchoreov1alpha1.ClusterComponentTypeSpec{
				WorkloadType: "deployment",
				Traits: []openchoreov1alpha1.ClusterComponentTypeTrait{
					{Kind: openchoreov1alpha1.ClusterTraitRefKindClusterTrait, Name: "networking", InstanceName: "net"},
				},
			},
		}
		comp := testComponent()
		comp.Spec.ComponentType = openchoreov1alpha1.ComponentTypeRef{
			Kind: openchoreov1alpha1.ComponentTypeRefKindClusterComponentType,
			Name: "deployment/cluster-web",
		}

		objs := []client.Object{testProject(), testDeploymentPipeline(), comp, cct, testWorkload(), clusterTrait}
		svc := newService(t, objs...)

		result, err := svc.GenerateRelease(ctx, testNamespace, testComponentName, &GenerateReleaseRequest{ReleaseName: "v1"})
		require.NoError(t, err)
		assert.True(t, hasReleaseTrait(result.Spec.Traits, "networking"), "expected trait 'networking' in Spec.Traits")
	})

	t.Run("embedded trait not found returns error", func(t *testing.T) {
		ct := testComponentType()
		ct.Spec.Traits = []openchoreov1alpha1.ComponentTypeTrait{
			{Kind: openchoreov1alpha1.TraitRefKindTrait, Name: "nonexistent", InstanceName: "missing"},
		}

		objs := []client.Object{testProject(), testDeploymentPipeline(), testComponent(), ct, testWorkload()}
		svc := newService(t, objs...)

		_, err := svc.GenerateRelease(ctx, testNamespace, testComponentName, &GenerateReleaseRequest{ReleaseName: "v1"})
		require.ErrorIs(t, err, ErrTraitNotFound)
	})
}

func TestGetComponentSchema(t *testing.T) {
	ctx := context.Background()

	t.Run("basic - no env configs", func(t *testing.T) {
		svc := newService(t, testComponent(), testComponentType())

		schema, err := svc.GetComponentSchema(ctx, testNamespace, testComponentName)
		require.NoError(t, err)
		assert.Equal(t, "object", schema.Type)
		// No componentTypeEnvironmentConfigs expected
		_, hasEnvConfigs := schema.Properties["componentTypeEnvironmentConfigs"]
		assert.False(t, hasEnvConfigs)
	})

	t.Run("with environmentConfigs", func(t *testing.T) {
		ct := testComponentType()
		ct.Spec.EnvironmentConfigs = &openchoreov1alpha1.SchemaSection{
			OpenAPIV3Schema: rawJSON(t, map[string]any{"replicas": "integer"}),
		}
		svc := newService(t, testComponent(), ct)

		schema, err := svc.GetComponentSchema(ctx, testNamespace, testComponentName)
		require.NoError(t, err)
		_, hasEnvConfigs := schema.Properties["componentTypeEnvironmentConfigs"]
		assert.True(t, hasEnvConfigs)
	})

	t.Run("with trait envConfigs", func(t *testing.T) {
		trait := &openchoreov1alpha1.Trait{
			ObjectMeta: metav1.ObjectMeta{Name: "ingress", Namespace: testNamespace},
			Spec: openchoreov1alpha1.TraitSpec{
				EnvironmentConfigs: &openchoreov1alpha1.SchemaSection{
					OpenAPIV3Schema: rawJSON(t, map[string]any{"hostname": "string"}),
				},
			},
		}
		comp := testComponent()
		comp.Spec.Traits = []openchoreov1alpha1.ComponentTrait{
			{Kind: openchoreov1alpha1.TraitRefKindTrait, Name: "ingress", InstanceName: "my-ingress"},
		}
		svc := newService(t, comp, testComponentType(), trait)

		schema, err := svc.GetComponentSchema(ctx, testNamespace, testComponentName)
		require.NoError(t, err)
		traitEnvironmentConfigs, has := schema.Properties["traitEnvironmentConfigs"]
		require.True(t, has)
		_, hasInstance := traitEnvironmentConfigs.Properties["my-ingress"]
		assert.True(t, hasInstance)
	})

	t.Run("component not found", func(t *testing.T) {
		svc := newService(t)
		_, err := svc.GetComponentSchema(ctx, testNamespace, "nonexistent")
		require.ErrorIs(t, err, ErrComponentNotFound)
	})

	t.Run("componentType not found", func(t *testing.T) {
		svc := newService(t, testComponent()) // no ComponentType seeded
		_, err := svc.GetComponentSchema(ctx, testNamespace, testComponentName)
		require.ErrorIs(t, err, ErrComponentTypeNotFound)
	})

	t.Run("ClusterComponentType", func(t *testing.T) {
		cct := &openchoreov1alpha1.ClusterComponentType{
			ObjectMeta: metav1.ObjectMeta{Name: "cluster-web"},
			Spec: openchoreov1alpha1.ClusterComponentTypeSpec{
				WorkloadType: "deployment",
				EnvironmentConfigs: &openchoreov1alpha1.SchemaSection{
					OpenAPIV3Schema: rawJSON(t, map[string]any{"replicas": "integer"}),
				},
			},
		}
		comp := testComponent()
		comp.Spec.ComponentType = openchoreov1alpha1.ComponentTypeRef{
			Kind: openchoreov1alpha1.ComponentTypeRefKindClusterComponentType,
			Name: "deployment/cluster-web",
		}
		svc := newService(t, comp, cct)

		schema, err := svc.GetComponentSchema(ctx, testNamespace, testComponentName)
		require.NoError(t, err)
		_, hasEnvConfigs := schema.Properties["componentTypeEnvironmentConfigs"]
		assert.True(t, hasEnvConfigs)
	})
}

func TestGetComponentReleaseSchema(t *testing.T) {
	ctx := context.Background()

	t.Run("basic - no env configs", func(t *testing.T) {
		release := &openchoreov1alpha1.ComponentRelease{
			ObjectMeta: metav1.ObjectMeta{Name: "rel-1", Namespace: testNamespace},
			Spec: openchoreov1alpha1.ComponentReleaseSpec{
				Owner: openchoreov1alpha1.ComponentReleaseOwner{
					ProjectName:   testProjectName,
					ComponentName: testComponentName,
				},
				ComponentType: openchoreov1alpha1.ComponentTypeSpec{
					WorkloadType: "deployment",
				},
			},
		}
		svc := newService(t, release)

		schema, err := svc.GetComponentReleaseSchema(ctx, testNamespace, "rel-1", testComponentName)
		require.NoError(t, err)
		assert.Equal(t, "object", schema.Type)
		_, hasEnvConfigs := schema.Properties["componentTypeEnvironmentConfigs"]
		assert.False(t, hasEnvConfigs)
	})

	t.Run("with componentType envConfigs", func(t *testing.T) {
		release := &openchoreov1alpha1.ComponentRelease{
			ObjectMeta: metav1.ObjectMeta{Name: "rel-2", Namespace: testNamespace},
			Spec: openchoreov1alpha1.ComponentReleaseSpec{
				Owner: openchoreov1alpha1.ComponentReleaseOwner{
					ProjectName:   testProjectName,
					ComponentName: testComponentName,
				},
				ComponentType: openchoreov1alpha1.ComponentTypeSpec{
					WorkloadType: "deployment",
					EnvironmentConfigs: &openchoreov1alpha1.SchemaSection{
						OpenAPIV3Schema: rawJSON(t, map[string]any{"replicas": "integer"}),
					},
				},
			},
		}
		svc := newService(t, release)

		schema, err := svc.GetComponentReleaseSchema(ctx, testNamespace, "rel-2", testComponentName)
		require.NoError(t, err)
		_, hasEnvConfigs := schema.Properties["componentTypeEnvironmentConfigs"]
		assert.True(t, hasEnvConfigs)
	})

	t.Run("with trait envConfigs", func(t *testing.T) {
		release := &openchoreov1alpha1.ComponentRelease{
			ObjectMeta: metav1.ObjectMeta{Name: "rel-3", Namespace: testNamespace},
			Spec: openchoreov1alpha1.ComponentReleaseSpec{
				Owner: openchoreov1alpha1.ComponentReleaseOwner{
					ProjectName:   testProjectName,
					ComponentName: testComponentName,
				},
				ComponentType: openchoreov1alpha1.ComponentTypeSpec{WorkloadType: "deployment"},
				Traits: []openchoreov1alpha1.ComponentReleaseTrait{
					{
						Kind: openchoreov1alpha1.TraitRefKindTrait,
						Name: "ingress",
						Spec: openchoreov1alpha1.TraitSpec{
							EnvironmentConfigs: &openchoreov1alpha1.SchemaSection{
								OpenAPIV3Schema: rawJSON(t, map[string]any{"hostname": "string"}),
							},
						},
					},
				},
				ComponentProfile: &openchoreov1alpha1.ComponentProfile{
					Traits: []openchoreov1alpha1.ComponentProfileTrait{
						{Kind: openchoreov1alpha1.TraitRefKindTrait, Name: "ingress", InstanceName: "my-ingress"},
					},
				},
			},
		}
		svc := newService(t, release)

		schema, err := svc.GetComponentReleaseSchema(ctx, testNamespace, "rel-3", testComponentName)
		require.NoError(t, err)
		traitEnvironmentConfigs, has := schema.Properties["traitEnvironmentConfigs"]
		require.True(t, has)
		_, hasInstance := traitEnvironmentConfigs.Properties["my-ingress"]
		assert.True(t, hasInstance)
	})

	t.Run("release not found", func(t *testing.T) {
		svc := newService(t)
		_, err := svc.GetComponentReleaseSchema(ctx, testNamespace, "nonexistent", testComponentName)
		require.ErrorIs(t, err, ErrComponentReleaseNotFound)
	})

	t.Run("release owned by different component", func(t *testing.T) {
		release := &openchoreov1alpha1.ComponentRelease{
			ObjectMeta: metav1.ObjectMeta{Name: "rel-other", Namespace: testNamespace},
			Spec: openchoreov1alpha1.ComponentReleaseSpec{
				Owner: openchoreov1alpha1.ComponentReleaseOwner{
					ProjectName:   testProjectName,
					ComponentName: "other-comp",
				},
			},
		}
		svc := newService(t, release)

		_, err := svc.GetComponentReleaseSchema(ctx, testNamespace, "rel-other", testComponentName)
		require.ErrorIs(t, err, ErrComponentReleaseNotFound)
	})

	t.Run("empty releaseName", func(t *testing.T) {
		svc := newService(t)
		_, err := svc.GetComponentReleaseSchema(ctx, testNamespace, "", testComponentName)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrValidation)
	})

	t.Run("empty componentName", func(t *testing.T) {
		svc := newService(t)
		_, err := svc.GetComponentReleaseSchema(ctx, testNamespace, "rel-1", "")
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrValidation)
	})
}

// --- ListComponents ---

func TestListComponents(t *testing.T) {
	ctx := context.Background()

	t.Run("list all without project filter", func(t *testing.T) {
		projA := testProject()
		projB := &openchoreov1alpha1.Project{
			ObjectMeta: metav1.ObjectMeta{Name: "proj-b", Namespace: testNamespace},
			Spec:       openchoreov1alpha1.ProjectSpec{DeploymentPipelineRef: openchoreov1alpha1.DeploymentPipelineRef{Name: testPipelineName}},
		}
		compA := &openchoreov1alpha1.Component{
			ObjectMeta: metav1.ObjectMeta{Name: "comp-a", Namespace: testNamespace, Labels: map[string]string{labels.LabelKeyProjectName: testProjectName}},
			Spec:       openchoreov1alpha1.ComponentSpec{Owner: openchoreov1alpha1.ComponentOwner{ProjectName: testProjectName}, ComponentType: openchoreov1alpha1.ComponentTypeRef{Name: "deployment/web-app"}},
		}
		compB := &openchoreov1alpha1.Component{
			ObjectMeta: metav1.ObjectMeta{Name: "comp-b", Namespace: testNamespace, Labels: map[string]string{labels.LabelKeyProjectName: "proj-b"}},
			Spec:       openchoreov1alpha1.ComponentSpec{Owner: openchoreov1alpha1.ComponentOwner{ProjectName: "proj-b"}, ComponentType: openchoreov1alpha1.ComponentTypeRef{Name: "deployment/web-app"}},
		}
		svc := newService(t, projA, projB, compA, compB)

		result, err := svc.ListComponents(ctx, testNamespace, "", services.ListOptions{})
		require.NoError(t, err)
		assert.Len(t, result.Items, 2)
		for _, item := range result.Items {
			assert.Equal(t, componentTypeMeta, item.TypeMeta)
		}
	})

	t.Run("filter by project", func(t *testing.T) {
		projA := testProject()
		projB := &openchoreov1alpha1.Project{
			ObjectMeta: metav1.ObjectMeta{Name: "proj-b", Namespace: testNamespace},
			Spec:       openchoreov1alpha1.ProjectSpec{DeploymentPipelineRef: openchoreov1alpha1.DeploymentPipelineRef{Name: testPipelineName}},
		}
		compA := &openchoreov1alpha1.Component{
			ObjectMeta: metav1.ObjectMeta{Name: "comp-a", Namespace: testNamespace, Labels: map[string]string{labels.LabelKeyProjectName: testProjectName}},
			Spec:       openchoreov1alpha1.ComponentSpec{Owner: openchoreov1alpha1.ComponentOwner{ProjectName: testProjectName}, ComponentType: openchoreov1alpha1.ComponentTypeRef{Name: "deployment/web-app"}},
		}
		compB := &openchoreov1alpha1.Component{
			ObjectMeta: metav1.ObjectMeta{Name: "comp-b", Namespace: testNamespace, Labels: map[string]string{labels.LabelKeyProjectName: "proj-b"}},
			Spec:       openchoreov1alpha1.ComponentSpec{Owner: openchoreov1alpha1.ComponentOwner{ProjectName: "proj-b"}, ComponentType: openchoreov1alpha1.ComponentTypeRef{Name: "deployment/web-app"}},
		}
		svc := newService(t, projA, projB, compA, compB)

		result, err := svc.ListComponents(ctx, testNamespace, testProjectName, services.ListOptions{})
		require.NoError(t, err)
		assert.Len(t, result.Items, 1)
		assert.Equal(t, "comp-a", result.Items[0].Name)
		assert.Equal(t, testProjectName, result.Items[0].Spec.Owner.ProjectName)
	})

	t.Run("project not found", func(t *testing.T) {
		svc := newService(t)
		_, err := svc.ListComponents(ctx, testNamespace, "nonexistent", services.ListOptions{})
		require.ErrorIs(t, err, projectsvc.ErrProjectNotFound)
	})

	t.Run("empty result", func(t *testing.T) {
		svc := newService(t, testProject())
		result, err := svc.ListComponents(ctx, testNamespace, testProjectName, services.ListOptions{})
		require.NoError(t, err)
		assert.Empty(t, result.Items)
	})

	t.Run("with limit and project filter", func(t *testing.T) {
		proj := testProject()
		comps := make([]client.Object, 0, 4)
		comps = append(comps, proj)
		for i := range 3 {
			comp := &openchoreov1alpha1.Component{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fmt.Sprintf("comp-%d", i),
					Namespace: testNamespace,
					Labels:    map[string]string{labels.LabelKeyProjectName: testProjectName},
				},
				Spec: openchoreov1alpha1.ComponentSpec{
					Owner:         openchoreov1alpha1.ComponentOwner{ProjectName: testProjectName},
					ComponentType: openchoreov1alpha1.ComponentTypeRef{Name: "deployment/web-app"},
				},
			}
			comps = append(comps, comp)
		}
		svc := newService(t, comps...)

		result, err := svc.ListComponents(ctx, testNamespace, testProjectName, services.ListOptions{Limit: 2})
		require.NoError(t, err)
		assert.LessOrEqual(t, len(result.Items), 2)
	})

	t.Run("skips project validation when projectName is empty", func(t *testing.T) {
		comp := testComponent()
		svc := newService(t, comp) // no project seeded

		result, err := svc.ListComponents(ctx, testNamespace, "", services.ListOptions{})
		require.NoError(t, err)
		assert.Len(t, result.Items, 1)
	})

	t.Run("TypeMeta is set on each item", func(t *testing.T) {
		svc := newService(t, testProject(), testComponent())

		result, err := svc.ListComponents(ctx, testNamespace, "", services.ListOptions{})
		require.NoError(t, err)
		require.NotEmpty(t, result.Items)
		for _, item := range result.Items {
			assert.Equal(t, componentTypeMeta, item.TypeMeta)
		}
	})

	t.Run("valid label selector filters results", func(t *testing.T) {
		comp := testComponent()
		comp.Labels["env"] = "prod"
		compNoLabel := &openchoreov1alpha1.Component{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "comp-no-label",
				Namespace: testNamespace,
				Labels:    map[string]string{labels.LabelKeyProjectName: testProjectName},
			},
			Spec: openchoreov1alpha1.ComponentSpec{
				Owner:         openchoreov1alpha1.ComponentOwner{ProjectName: testProjectName},
				ComponentType: openchoreov1alpha1.ComponentTypeRef{Name: "deployment/web-app"},
			},
		}
		svc := newService(t, comp, compNoLabel)

		result, err := svc.ListComponents(ctx, testNamespace, "", services.ListOptions{LabelSelector: "env=prod"})
		require.NoError(t, err)
		assert.Len(t, result.Items, 1)
		assert.Equal(t, testComponentName, result.Items[0].Name)
	})

	t.Run("invalid label selector returns error", func(t *testing.T) {
		svc := newService(t, testProject(), testComponent())

		_, err := svc.ListComponents(ctx, testNamespace, "", services.ListOptions{LabelSelector: "===invalid"})
		require.Error(t, err)
		var validationErr *services.ValidationError
		assert.ErrorAs(t, err, &validationErr)
	})

	t.Run("namespace isolation", func(t *testing.T) {
		compInNs := testComponent()
		compInOtherNs := &openchoreov1alpha1.Component{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "comp-other",
				Namespace: "other-ns",
				Labels:    map[string]string{labels.LabelKeyProjectName: testProjectName},
			},
			Spec: openchoreov1alpha1.ComponentSpec{
				Owner:         openchoreov1alpha1.ComponentOwner{ProjectName: testProjectName},
				ComponentType: openchoreov1alpha1.ComponentTypeRef{Name: "deployment/web-app"},
			},
		}
		svc := newService(t, compInNs, compInOtherNs)

		result, err := svc.ListComponents(ctx, testNamespace, "", services.ListOptions{})
		require.NoError(t, err)
		assert.Len(t, result.Items, 1)
		assert.Equal(t, testComponentName, result.Items[0].Name)
		assert.Equal(t, testNamespace, result.Items[0].Namespace)
	})
}
