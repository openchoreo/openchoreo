// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package releasebinding

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/labels"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/testutil"
)

const (
	testPromotionRelease = "release-a"
	testOtherName        = "other"
)

func promotionPipeline() *openchoreov1alpha1.DeploymentPipeline {
	return &openchoreov1alpha1.DeploymentPipeline{
		ObjectMeta: metav1.ObjectMeta{Name: "default", Namespace: testNamespace},
		Spec: openchoreov1alpha1.DeploymentPipelineSpec{
			PromotionPaths: []openchoreov1alpha1.PromotionPath{
				{SourceEnvironmentRef: openchoreov1alpha1.EnvironmentRef{Name: "dev"},
					TargetEnvironmentRefs: []openchoreov1alpha1.TargetEnvironmentRef{{Name: "staging"}}},
				{SourceEnvironmentRef: openchoreov1alpha1.EnvironmentRef{Name: "staging"},
					TargetEnvironmentRefs: []openchoreov1alpha1.TargetEnvironmentRef{{Name: "production"}}},
			},
		},
	}
}

func TestReleaseBindingPromotion(t *testing.T) {
	tests := []struct {
		name            string
		environment     string
		state           openchoreov1alpha1.ReleaseState
		release         string
		source          bool
		mutateSource    func(*openchoreov1alpha1.ReleaseBinding)
		mutatePipeline  func(*openchoreov1alpha1.DeploymentPipeline)
		missingProject  bool
		missingPipeline bool
		wantError       string
	}{
		{name: "cannot skip staging", release: testPromotionRelease, wantError: "must be deployed to staging"},
		{name: "configured source", release: testPromotionRelease, source: true},
		{name: "source has another release", release: "release-b", source: true, wantError: "must be deployed to staging"},
		{name: "wrong source environment", release: testPromotionRelease, source: true,
			mutateSource: func(rb *openchoreov1alpha1.ReleaseBinding) { rb.Spec.Environment = "dev" }, wantError: "must be deployed to staging"},
		{name: "other component", release: testPromotionRelease, source: true,
			mutateSource: func(rb *openchoreov1alpha1.ReleaseBinding) { rb.Spec.Owner.ComponentName = testOtherName }, wantError: "must be deployed to staging"},
		{name: "other project", release: testPromotionRelease, source: true,
			mutateSource: func(rb *openchoreov1alpha1.ReleaseBinding) { rb.Spec.Owner.ProjectName = testOtherName }, wantError: "must be deployed to staging"},
		{name: "other namespace", release: testPromotionRelease, source: true,
			mutateSource: func(rb *openchoreov1alpha1.ReleaseBinding) { rb.Namespace = testOtherName }, wantError: "must be deployed to staging"},
		{name: "source spec without labels", release: testPromotionRelease, source: true,
			mutateSource: func(rb *openchoreov1alpha1.ReleaseBinding) { rb.Labels = nil }},
		{name: "root environment", environment: "dev", release: testPromotionRelease},
		{name: "environment outside pipeline", environment: "sandbox", release: testPromotionRelease},
		{name: "empty pipeline", release: testPromotionRelease,
			mutatePipeline: func(p *openchoreov1alpha1.DeploymentPipeline) { p.Spec.PromotionPaths = nil }},
		{name: "explicit direct promotion", release: testPromotionRelease, source: true,
			mutateSource: func(rb *openchoreov1alpha1.ReleaseBinding) { rb.Spec.Environment = "dev" },
			mutatePipeline: func(p *openchoreov1alpha1.DeploymentPipeline) {
				p.Spec.PromotionPaths[0].TargetEnvironmentRefs = append(p.Spec.PromotionPaths[0].TargetEnvironmentRefs,
					openchoreov1alpha1.TargetEnvironmentRef{Name: "production"})
			}},
		{name: "any configured source", release: testPromotionRelease, source: true,
			mutateSource: func(rb *openchoreov1alpha1.ReleaseBinding) { rb.Spec.Environment = "qa" },
			mutatePipeline: func(p *openchoreov1alpha1.DeploymentPipeline) {
				p.Spec.PromotionPaths = append(p.Spec.PromotionPaths, openchoreov1alpha1.PromotionPath{
					SourceEnvironmentRef:  openchoreov1alpha1.EnvironmentRef{Name: "qa"},
					TargetEnvironmentRefs: []openchoreov1alpha1.TargetEnvironmentRef{{Name: "production"}},
				})
			}},
		{name: "all source names in error", release: testPromotionRelease,
			mutatePipeline: func(p *openchoreov1alpha1.DeploymentPipeline) {
				p.Spec.PromotionPaths = append(p.Spec.PromotionPaths, openchoreov1alpha1.PromotionPath{
					SourceEnvironmentRef:  openchoreov1alpha1.EnvironmentRef{Name: "qa"},
					TargetEnvironmentRefs: []openchoreov1alpha1.TargetEnvironmentRef{{Name: "production"}},
				})
			}, wantError: "must be deployed to staging or qa"},
		{name: "missing project", release: testPromotionRelease, missingProject: true, wantError: "Project \"test-project\" not found"},
		{name: "missing pipeline", release: testPromotionRelease, missingPipeline: true, wantError: "DeploymentPipeline \"default\" not found"},
		{name: "undeploy", release: testPromotionRelease, state: openchoreov1alpha1.ReleaseStateUndeploy},
		{name: "undeploy with missing pipeline", release: testPromotionRelease, state: openchoreov1alpha1.ReleaseStateUndeploy, missingPipeline: true},
		{name: "auto deploy pending", missingProject: true, missingPipeline: true},
	}

	for _, tt := range tests {
		for _, operation := range []string{"create", "update"} {
			t.Run(tt.name+"/"+operation, func(t *testing.T) {
				ctx := context.Background()
				pipeline := promotionPipeline()
				if tt.mutatePipeline != nil {
					tt.mutatePipeline(pipeline)
				}
				objects := []client.Object{testutil.NewComponent(testNamespace, testProjectName, testComponentName)}
				if !tt.missingProject {
					objects = append(objects, testutil.NewProject(testNamespace, testProjectName))
				}
				if !tt.missingPipeline {
					objects = append(objects, pipeline)
				}
				if tt.source {
					source := testutil.NewReleaseBinding(testNamespace, testProjectName, testComponentName, "staging", "source")
					source.Spec.ReleaseName = testPromotionRelease
					if tt.mutateSource != nil {
						tt.mutateSource(source)
					}
					objects = append(objects, source)
				}
				environment := tt.environment
				if environment == "" {
					environment = "production"
				}
				existing := testutil.NewReleaseBinding(testNamespace, testProjectName, testComponentName, environment, testRBName)
				existing.Spec.ReleaseName = "old-release"
				if operation == "update" {
					objects = append(objects, existing)
				}
				k8sClient := testutil.NewFakeClient(objects...)
				svc := NewService(k8sClient, testutil.TestLogger())
				rb := existing.DeepCopy()
				rb.Namespace = "untrusted-namespace"
				rb.Spec.ReleaseName = tt.release
				rb.Spec.State = tt.state
				var err error
				if operation == "create" {
					_, err = svc.CreateReleaseBinding(ctx, testNamespace, rb)
				} else {
					_, err = svc.UpdateReleaseBinding(ctx, testNamespace, rb)
				}
				stored := &openchoreov1alpha1.ReleaseBinding{}
				getErr := k8sClient.Get(ctx, client.ObjectKey{Namespace: testNamespace, Name: testRBName}, stored)
				if tt.wantError == "" {
					require.NoError(t, err)
					require.NoError(t, getErr)
					assert.Equal(t, tt.release, stored.Spec.ReleaseName)
					return
				}
				var validationErr *services.ValidationError
				require.ErrorAs(t, err, &validationErr)
				assert.Contains(t, validationErr.Msg, tt.wantError)
				if operation == "create" {
					assert.True(t, apierrors.IsNotFound(getErr), "rejected creates must not persist")
				} else {
					require.NoError(t, getErr)
					assert.Equal(t, existing.Spec, stored.Spec, "rejected updates must not change the stored spec")
				}
			})
		}
	}
}

func TestUpdateReleaseBindingRenderedPromotion(t *testing.T) {
	tests := []struct {
		name            string
		rendered        bool
		ownerUID        types.UID
		renderedRelease string
		release         string
		state           openchoreov1alpha1.ReleaseState
		wantError       bool
	}{
		{name: "keeps rendered release after source advances", rendered: true, ownerUID: "stored-uid", renderedRelease: testPromotionRelease, release: testPromotionRelease},
		{name: "config edit on blocked binding", release: testPromotionRelease, wantError: true},
		{name: "new release needs source", rendered: true, ownerUID: "stored-uid", renderedRelease: testPromotionRelease, release: "release-b", wantError: true},
		{name: "another binding owns rendered release", rendered: true, ownerUID: "other-uid", renderedRelease: testPromotionRelease, release: testPromotionRelease, wantError: true},
		{name: "rendered release label differs", rendered: true, ownerUID: "stored-uid", renderedRelease: "release-b", release: testPromotionRelease, wantError: true},
		{name: "reactivation needs source", release: testPromotionRelease, state: openchoreov1alpha1.ReleaseStateActive, wantError: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			existing := testutil.NewReleaseBinding(testNamespace, testProjectName, testComponentName, "production", testRBName)
			existing.UID = "stored-uid"
			existing.Spec.ReleaseName = testPromotionRelease
			if tt.state != "" {
				existing.Spec.State = openchoreov1alpha1.ReleaseStateUndeploy
			}
			objects := []client.Object{existing, promotionPipeline(), testutil.NewProject(testNamespace, testProjectName)}
			if tt.rendered {
				owner := existing.DeepCopy()
				owner.UID = tt.ownerUID
				objects = append(objects, &openchoreov1alpha1.RenderedRelease{
					ObjectMeta: metav1.ObjectMeta{
						Name: testComponentName + "-production", Namespace: testNamespace,
						OwnerReferences: []metav1.OwnerReference{*metav1.NewControllerRef(owner, openchoreov1alpha1.GroupVersion.WithKind("ReleaseBinding"))},
						Labels:          map[string]string{labels.LabelKeyComponentReleaseName: tt.renderedRelease},
					},
				})
			}
			k8sClient := testutil.NewFakeClient(objects...)
			svc := NewService(k8sClient, testutil.TestLogger())
			rb := existing.DeepCopy()
			rb.UID = "other-uid"
			rb.Spec.ReleaseName = tt.release
			rb.Spec.State = tt.state
			rb.Annotations = map[string]string{"edited": "true"}
			_, err := svc.UpdateReleaseBinding(ctx, testNamespace, rb)
			if tt.wantError {
				var validationErr *services.ValidationError
				require.ErrorAs(t, err, &validationErr)
				stored, getErr := svc.GetReleaseBinding(ctx, testNamespace, testRBName)
				require.NoError(t, getErr)
				assert.Equal(t, existing.Spec, stored.Spec)
				assert.Empty(t, stored.Annotations)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestReleaseBindingPromotionReadErrors(t *testing.T) {
	for _, resource := range []string{"project", "pipeline", "bindings", "rendered"} {
		t.Run(resource, func(t *testing.T) {
			ctx := context.Background()
			existing := testutil.NewReleaseBinding(testNamespace, testProjectName, testComponentName, "production", testRBName)
			existing.Spec.ReleaseName = "old-release"
			base := fake.NewClientBuilder().WithScheme(testutil.NewScheme()).
				WithObjects(existing, promotionPipeline(), testutil.NewProject(testNamespace, testProjectName)).Build()
			readErr := errors.New("read failed")
			k8sClient := interceptor.NewClient(base, interceptor.Funcs{
				Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
					switch obj.(type) {
					case *openchoreov1alpha1.Project:
						if resource == "project" {
							return readErr
						}
					case *openchoreov1alpha1.DeploymentPipeline:
						if resource == "pipeline" {
							return readErr
						}
					case *openchoreov1alpha1.RenderedRelease:
						if resource == "rendered" {
							return readErr
						}
					}
					return c.Get(ctx, key, obj, opts...)
				},
				List: func(ctx context.Context, c client.WithWatch, list client.ObjectList, opts ...client.ListOption) error {
					if resource == "bindings" {
						return readErr
					}
					return c.List(ctx, list, opts...)
				},
			})
			rb := existing.DeepCopy()
			rb.Spec.ReleaseName = testPromotionRelease
			_, err := NewService(k8sClient, testutil.TestLogger()).UpdateReleaseBinding(ctx, testNamespace, rb)
			require.ErrorIs(t, err, readErr)
			stored := &openchoreov1alpha1.ReleaseBinding{}
			require.NoError(t, base.Get(ctx, client.ObjectKeyFromObject(existing), stored))
			assert.Equal(t, existing.Spec, stored.Spec)
		})
	}
}

func TestCreateReleaseBindingCannotClaimRenderedPromotion(t *testing.T) {
	ctx := context.Background()
	rb := testutil.NewReleaseBinding(testNamespace, testProjectName, testComponentName, "production", testRBName)
	rb.UID = "claimed-uid"
	rb.Spec.ReleaseName = testPromotionRelease
	rb.Status.Conditions = []metav1.Condition{{Type: "ReleaseSynced", Status: metav1.ConditionTrue}}
	rendered := &openchoreov1alpha1.RenderedRelease{
		ObjectMeta: metav1.ObjectMeta{
			Name: testComponentName + "-production", Namespace: testNamespace,
			OwnerReferences: []metav1.OwnerReference{*metav1.NewControllerRef(rb, openchoreov1alpha1.GroupVersion.WithKind("ReleaseBinding"))},
			Labels:          map[string]string{labels.LabelKeyComponentReleaseName: rb.Spec.ReleaseName},
		},
	}
	svc := newService(t, promotionPipeline(), testutil.NewProject(testNamespace, testProjectName),
		testutil.NewComponent(testNamespace, testProjectName, testComponentName), rendered)
	_, err := svc.CreateReleaseBinding(ctx, testNamespace, rb)
	var validationErr *services.ValidationError
	require.ErrorAs(t, err, &validationErr)
	_, err = svc.GetReleaseBinding(ctx, testNamespace, testRBName)
	require.ErrorIs(t, err, ErrReleaseBindingNotFound)
}
