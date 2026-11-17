// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package component

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/occ/resources/client/mocks"
	"github.com/openchoreo/openchoreo/internal/occ/testutil"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
)

// hasMessage matches a binding carrying msg as its change message.
func hasMessage(msg string) any {
	return mock.MatchedBy(func(rb gen.ReleaseBinding) bool {
		return rb.Metadata.Annotations != nil &&
			(*rb.Metadata.Annotations)[openchoreov1alpha1.AnnotationKeyChangeMessage] == msg
	})
}

func TestDeploy_MessageOnANewBinding(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GenerateRelease(mock.Anything, "ns", "my-comp", mock.Anything).Return(&gen.ComponentRelease{
		Metadata: gen.ObjectMeta{Name: testReleaseName},
	}, nil)
	mc.EXPECT().GetProjectDeploymentPipeline(mock.Anything, "ns", "my-project").Return(makeLinearPipeline(), nil)
	mc.EXPECT().GetReleaseBinding(mock.Anything, "ns", "my-comp-dev").Return(nil, nil)
	mc.EXPECT().CreateReleaseBinding(mock.Anything, "ns", hasMessage("First deploy")).Return(&gen.ReleaseBinding{
		Metadata: gen.ObjectMeta{Name: "my-comp-dev"},
		Spec:     &gen.ReleaseBindingSpec{Environment: "dev", ReleaseName: makeReleaseName(testReleaseName)},
	}, nil)

	testutil.CaptureStdout(t, func() {
		require.NoError(t, New(mc).Deploy(DeployParams{Namespace: "ns", Project: "my-project", ComponentName: "my-comp",
			Message: "First deploy"}))
	})
}

func TestDeploy_MessageOnAnExistingBinding(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GenerateRelease(mock.Anything, "ns", "my-comp", mock.Anything).Return(&gen.ComponentRelease{
		Metadata: gen.ObjectMeta{Name: "rel-2"},
	}, nil)
	mc.EXPECT().GetProjectDeploymentPipeline(mock.Anything, "ns", "my-project").Return(makeLinearPipeline(), nil)
	mc.EXPECT().GetReleaseBinding(mock.Anything, "ns", "my-comp-dev").Return(&gen.ReleaseBinding{
		Metadata: gen.ObjectMeta{Name: "my-comp-dev"},
		Spec:     &gen.ReleaseBindingSpec{Environment: "dev", ReleaseName: makeReleaseName(testReleaseName)},
	}, nil)
	mc.EXPECT().UpdateReleaseBinding(mock.Anything, "ns", "my-comp-dev", hasMessage("Fix checkout timeout")).
		Return(&gen.ReleaseBinding{
			Metadata: gen.ObjectMeta{Name: "my-comp-dev"},
			Spec:     &gen.ReleaseBindingSpec{Environment: "dev", ReleaseName: makeReleaseName("rel-2")},
		}, nil)

	testutil.CaptureStdout(t, func() {
		require.NoError(t, New(mc).Deploy(DeployParams{Namespace: "ns", Project: "my-project", ComponentName: "my-comp",
			Message: "Fix checkout timeout"}))
	})
}

func TestDeploy_PromoteKeepsThePromoteCauseAndAddsTheMessage(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetProjectDeploymentPipeline(mock.Anything, "ns", "my-project").Return(makeLinearPipeline(), nil)
	mc.EXPECT().ListReleaseBindings(mock.Anything, "ns", mock.Anything).Return(&gen.ReleaseBindingList{
		Items: []gen.ReleaseBinding{{
			Metadata: gen.ObjectMeta{Name: "my-comp-dev"},
			Spec: &gen.ReleaseBindingSpec{
				Environment: "dev",
				Owner: struct {
					ComponentName string  `json:"componentName"`
					ProjectName   string  `json:"projectName"`
					ReleaseLine   *string `json:"releaseLine,omitempty"`
				}{ComponentName: "my-comp"},
				ReleaseName: makeReleaseName(testReleaseName),
			},
		}},
	}, nil)
	mc.EXPECT().GetReleaseBinding(mock.Anything, "ns", "my-comp-staging").Return(nil, nil)
	both := mock.MatchedBy(func(rb gen.ReleaseBinding) bool {
		a := rb.Metadata.Annotations
		return a != nil && (*a)[openchoreov1alpha1.AnnotationKeyRevisionCause] == string(openchoreov1alpha1.RevisionCausePromote) &&
			(*a)[openchoreov1alpha1.AnnotationKeyChangeMessage] == "Ready for QA"
	})
	mc.EXPECT().CreateReleaseBinding(mock.Anything, "ns", both).Return(&gen.ReleaseBinding{
		Metadata: gen.ObjectMeta{Name: "my-comp-staging"},
		Spec:     &gen.ReleaseBindingSpec{Environment: "staging", ReleaseName: makeReleaseName(testReleaseName)},
	}, nil)

	testutil.CaptureStdout(t, func() {
		require.NoError(t, New(mc).Deploy(DeployParams{Namespace: "ns", Project: "my-project", ComponentName: "my-comp",
			To: "staging", Message: "Ready for QA"}))
	})
}

func TestDeploy_MessageTooLongIsRefusedBeforeAnyCall(t *testing.T) {
	err := New(mocks.NewMockInterface(t)).Deploy(DeployParams{Namespace: "ns", Project: "my-project",
		ComponentName: "my-comp", Message: strings.Repeat("x", 1025)})
	assert.EqualError(t, err, "--message must be at most 1024 characters (got 1025)")
}
