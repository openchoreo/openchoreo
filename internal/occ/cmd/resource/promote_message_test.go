// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package resource

import (
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/occ/resources/client/mocks"
	"github.com/openchoreo/openchoreo/internal/occ/testutil"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
)

func TestPromote_Message(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetResource(mock.Anything, "my-org", "analytics-db").Return(resourceWithLatestRelease(), nil)
	mc.EXPECT().ListResourceReleaseBindings(mock.Anything, "my-org", mock.Anything).Return(&gen.ResourceReleaseBindingList{
		Items: []gen.ResourceReleaseBinding{bindingForEnv("analytics-db-dev", "dev")},
	}, nil)
	mc.EXPECT().UpdateResourceReleaseBinding(mock.Anything, "my-org", "analytics-db-dev",
		mock.MatchedBy(func(b gen.ResourceReleaseBinding) bool {
			return b.Metadata.Annotations != nil &&
				(*b.Metadata.Annotations)[openchoreov1alpha1.AnnotationKeyChangeMessage] == "Bigger instance"
		})).Return(&gen.ResourceReleaseBinding{Metadata: gen.ObjectMeta{Name: "analytics-db-dev"}}, nil)

	testutil.CaptureStdout(t, func() {
		require.NoError(t, New(mc).Promote(PromoteParams{Namespace: "my-org", ResourceName: "analytics-db",
			Environment: "dev", Message: "Bigger instance"}))
	})
}
