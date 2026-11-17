// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package mcphandlers

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/bindingrevision"
	releasebindingmocks "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/releasebinding/mocks"
	resourcereleasebindingmocks "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/resourcereleasebinding/mocks"
	"github.com/openchoreo/openchoreo/internal/server/middleware/audit"
	"github.com/openchoreo/openchoreo/pkg/mcp/tools"
)

const testReleaseBindingName = "test-component-dev"

func sampleReleaseBindingRevision(n int64, release string, active, knownGood bool) openchoreov1alpha1.ReleaseBindingRevision {
	return openchoreov1alpha1.ReleaseBindingRevision{
		ObjectMeta: metav1.ObjectMeta{
			Name: testReleaseBindingName + "-r" + strconv.FormatInt(n, 10), Namespace: testNS,
			CreationTimestamp: metav1.NewTime(time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)),
		},
		Spec: openchoreov1alpha1.ReleaseBindingRevisionSpec{
			Revision: n, Cause: openchoreov1alpha1.RevisionCauseDeploy, ComponentReleaseName: release, SnapshotHash: "abc",
			BindingSpec: openchoreov1alpha1.ReleaseBindingSpec{
				Owner:       openchoreov1alpha1.ReleaseBindingOwner{ProjectName: testProject, ComponentName: testComponent},
				Environment: testEnvironmentName, ReleaseName: release, State: openchoreov1alpha1.ReleaseStateActive,
			},
		},
		Status: openchoreov1alpha1.BindingRevisionStatus{
			Health: openchoreov1alpha1.RevisionHealthHealthy, Active: active, KnownGood: knownGood,
		},
	}
}

func sampleRollbackResult(dryRun bool) *bindingrevision.RollbackResult {
	from := int64(3)
	return &bindingrevision.RollbackResult{
		RevisionName: testReleaseBindingName + "-r4", Revision: 4, TargetRevision: 1, RevertedFrom: &from, DryRun: dryRun,
		Changes:  []bindingrevision.SpecChange{{Field: "releaseName", Current: "rel-c", Target: "rel-a"}},
		Warnings: []bindingrevision.Warning{{Code: bindingrevision.WarningProjectCellDrift, Message: "the cell moved"}},
	}
}

// --- ReleaseBindingRevision ---

func TestListReleaseBindingRevisions(t *testing.T) {
	ctx := context.Background()

	t.Run("returns wrapped list with the fields an agent picks a target by", func(t *testing.T) {
		svc := releasebindingmocks.NewMockService(t)
		svc.EXPECT().ListReleaseBindingRevisions(mock.Anything, testNS, testComponent, testEnvironmentName, "", mock.Anything).
			Return(&services.ListResult[openchoreov1alpha1.ReleaseBindingRevision]{
				Items: []openchoreov1alpha1.ReleaseBindingRevision{
					sampleReleaseBindingRevision(2, "rel-b", true, false),
					sampleReleaseBindingRevision(1, "rel-a", false, true),
				},
				NextCursor: "next-token",
			}, nil)

		h := newTestHandler(withReleaseBindingService(svc))
		result, err := h.ListReleaseBindingRevisions(ctx, testNS, testComponent, testEnvironmentName, "", tools.ListOpts{Limit: 10})
		require.NoError(t, err)

		m, ok := result.(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "next-token", m["next_cursor"])
		items, ok := m["release_binding_revisions"].([]map[string]any)
		require.True(t, ok)
		require.Len(t, items, 2)

		assert.Equal(t, int64(2), items[0]["revision"])
		assert.Equal(t, true, items[0]["active"])
		assert.Equal(t, false, items[0]["knownGood"], "false is reported, not omitted")
		assert.Equal(t, "rel-a", items[1]["componentRelease"])
		assert.Equal(t, testComponent+"-r1", items[1]["displayName"], "shown as {component}-r{N}")
		assert.Equal(t, true, items[1]["knownGood"])
		assert.Equal(t, "Healthy", items[1]["health"])
		assert.Equal(t, "Deploy", items[1]["cause"])
		assert.Equal(t, testEnvironmentName, items[1]["environment"])
		assert.Equal(t, "2026-10-01T09:00:00Z", items[1]["createdAt"])
	})

	t.Run("service error propagates", func(t *testing.T) {
		svc := releasebindingmocks.NewMockService(t)
		svc.EXPECT().ListReleaseBindingRevisions(mock.Anything, testNS, testComponent, "", "", mock.Anything).
			Return(nil, errors.New("list failed"))

		h := newTestHandler(withReleaseBindingService(svc))
		_, err := h.ListReleaseBindingRevisions(ctx, testNS, testComponent, "", "", tools.ListOpts{})
		assert.EqualError(t, err, "list failed")
	})
}

func TestGetReleaseBindingRevision(t *testing.T) {
	ctx := context.Background()

	t.Run("returns the summary plus the recorded binding spec", func(t *testing.T) {
		rev := sampleReleaseBindingRevision(1, "rel-a", false, true)
		svc := releasebindingmocks.NewMockService(t)
		svc.EXPECT().GetReleaseBindingRevision(mock.Anything, testNS, rev.Name).Return(&rev, nil)

		h := newTestHandler(withReleaseBindingService(svc))
		result, err := h.GetReleaseBindingRevision(ctx, testNS, rev.Name)
		require.NoError(t, err)

		m, ok := result.(map[string]any)
		require.True(t, ok)
		assert.Equal(t, rev.Name, m["name"])
		assert.Equal(t, "abc", m["snapshotHash"])
		spec, ok := m["bindingSpec"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "rel-a", spec["releaseName"])
	})

	t.Run("not found propagates", func(t *testing.T) {
		svc := releasebindingmocks.NewMockService(t)
		svc.EXPECT().GetReleaseBindingRevision(mock.Anything, testNS, "nope").Return(nil, bindingrevision.ErrRevisionNotFound)

		h := newTestHandler(withReleaseBindingService(svc))
		_, err := h.GetReleaseBindingRevision(ctx, testNS, "nope")
		assert.ErrorIs(t, err, bindingrevision.ErrRevisionNotFound)
	})
}

func TestRollbackReleaseBinding(t *testing.T) {
	ctx := context.Background()

	t.Run("dry run reports changes, warnings and the next step", func(t *testing.T) {
		svc := releasebindingmocks.NewMockService(t)
		svc.EXPECT().RollbackReleaseBinding(mock.Anything, testNS, testReleaseBindingName,
			bindingrevision.RollbackRequest{Revision: 1, DryRun: true}).Return(sampleRollbackResult(true), nil)

		h := newTestHandler(withReleaseBindingService(svc))
		result, err := h.RollbackReleaseBinding(ctx, testNS, testReleaseBindingName, 1, true, "")
		require.NoError(t, err)

		m, ok := result.(map[string]any)
		require.True(t, ok)
		assert.Equal(t, true, m["dryRun"])
		assert.Equal(t, false, m["applied"])
		assert.Equal(t, int64(1), m["targetRevision"])
		assert.Equal(t, int64(3), m["revertedFrom"])
		assert.Equal(t, testReleaseBindingName+"-r4", m["revisionName"])
		assert.Equal(t, []map[string]any{{"field": "releaseName", "current": "rel-c", "target": "rel-a"}}, m["changes"])
		assert.Equal(t, []map[string]any{{"code": "ProjectCellDrift", "message": "the cell moved"}}, m["warnings"])
		assert.Contains(t, m["nextStep"], "dry_run=false", "the dry run tells the agent how to proceed")
	})

	t.Run("applied rollback has no next step", func(t *testing.T) {
		svc := releasebindingmocks.NewMockService(t)
		svc.EXPECT().RollbackReleaseBinding(mock.Anything, testNS, testReleaseBindingName,
			bindingrevision.RollbackRequest{Revision: 1, DryRun: false}).Return(sampleRollbackResult(false), nil)

		h := newTestHandler(withReleaseBindingService(svc))
		result, err := h.RollbackReleaseBinding(ctx, testNS, testReleaseBindingName, 1, false, "")
		require.NoError(t, err)

		m := result.(map[string]any)
		assert.Equal(t, true, m["applied"])
		assert.NotContains(t, m, "nextStep")
	})

	t.Run("a message is trimmed, passed on and audited", func(t *testing.T) {
		svc := releasebindingmocks.NewMockService(t)
		svc.EXPECT().RollbackReleaseBinding(mock.Anything, testNS, testReleaseBindingName,
			bindingrevision.RollbackRequest{Revision: 1, Message: "Payments failing since r4"}).
			Return(sampleRollbackResult(false), nil)

		h := newTestHandler(withReleaseBindingService(svc))
		actx, data := audit.NewAuditContext(ctx,
			&audit.Resource{Namespace: testNS, Name: testReleaseBindingName}, audit.RequestInfo{})
		_, err := h.RollbackReleaseBinding(actx, testNS, testReleaseBindingName, 1, false, "  Payments failing since r4 ")
		require.NoError(t, err)
		assert.Equal(t, map[string]any{"dryRun": false, "message": "Payments failing since r4"}, data.Resource.Metadata)
	})

	t.Run("a message over the limit is refused before the service is called", func(t *testing.T) {
		h := newTestHandler(withReleaseBindingService(releasebindingmocks.NewMockService(t)))
		_, err := h.RollbackReleaseBinding(ctx, testNS, testReleaseBindingName, 1, true, strings.Repeat("x", 1025))
		var vErr *services.ValidationError
		require.ErrorAs(t, err, &vErr)
		assert.Equal(t, "message must be at most 1024 characters", vErr.Msg)
	})

	t.Run("conflict propagates", func(t *testing.T) {
		svc := releasebindingmocks.NewMockService(t)
		svc.EXPECT().RollbackReleaseBinding(mock.Anything, testNS, testReleaseBindingName, mock.Anything).
			Return(nil, bindingrevision.ErrReleaseDeleted)

		h := newTestHandler(withReleaseBindingService(svc))
		_, err := h.RollbackReleaseBinding(ctx, testNS, testReleaseBindingName, 1, true, "")
		assert.ErrorIs(t, err, bindingrevision.ErrReleaseDeleted)
	})
}

// --- ResourceReleaseBindingRevision ---

func sampleResourceReleaseBindingRevision(n int64, release string) openchoreov1alpha1.ResourceReleaseBindingRevision {
	return openchoreov1alpha1.ResourceReleaseBindingRevision{
		ObjectMeta: metav1.ObjectMeta{Name: testResourceReleaseBindingName + "-r1", Namespace: testNS},
		Spec: openchoreov1alpha1.ResourceReleaseBindingRevisionSpec{
			Revision: n, Cause: openchoreov1alpha1.RevisionCausePromote, ResourceReleaseName: release, SnapshotHash: "def",
			BindingSpec: openchoreov1alpha1.ResourceReleaseBindingSpec{
				Owner:           openchoreov1alpha1.ResourceReleaseBindingOwner{ProjectName: testProject, ResourceName: testResourceName},
				Environment:     testResourceEnvironment,
				ResourceRelease: release,
			},
		},
		Status: openchoreov1alpha1.BindingRevisionStatus{Health: openchoreov1alpha1.RevisionHealthHealthy, KnownGood: true},
	}
}

func TestResourceReleaseBindingRevisionTools(t *testing.T) {
	ctx := context.Background()

	t.Run("list", func(t *testing.T) {
		svc := resourcereleasebindingmocks.NewMockService(t)
		svc.EXPECT().ListResourceReleaseBindingRevisions(mock.Anything, testNS, testResourceName, "", "", mock.Anything).
			Return(&services.ListResult[openchoreov1alpha1.ResourceReleaseBindingRevision]{
				Items: []openchoreov1alpha1.ResourceReleaseBindingRevision{sampleResourceReleaseBindingRevision(1, testResourceReleaseName)},
			}, nil)

		h := newTestHandler(withResourceReleaseBindingService(svc))
		result, err := h.ListResourceReleaseBindingRevisions(ctx, testNS, testResourceName, "", "", tools.ListOpts{})
		require.NoError(t, err)

		items := result.(map[string]any)["resource_release_binding_revisions"].([]map[string]any)
		require.Len(t, items, 1)
		assert.Equal(t, testResourceReleaseName, items[0]["resourceRelease"])
		assert.Equal(t, testResourceName, items[0]["resourceName"])
		assert.Equal(t, "Promote", items[0]["cause"])
		assert.Equal(t, true, items[0]["knownGood"])
	})

	t.Run("get", func(t *testing.T) {
		rev := sampleResourceReleaseBindingRevision(1, testResourceReleaseName)
		svc := resourcereleasebindingmocks.NewMockService(t)
		svc.EXPECT().GetResourceReleaseBindingRevision(mock.Anything, testNS, rev.Name).Return(&rev, nil)

		h := newTestHandler(withResourceReleaseBindingService(svc))
		result, err := h.GetResourceReleaseBindingRevision(ctx, testNS, rev.Name)
		require.NoError(t, err)
		spec := result.(map[string]any)["bindingSpec"].(map[string]any)
		assert.Equal(t, testResourceReleaseName, spec["resourceRelease"])
	})

	t.Run("rollback passes dry_run through", func(t *testing.T) {
		svc := resourcereleasebindingmocks.NewMockService(t)
		svc.EXPECT().RollbackResourceReleaseBinding(mock.Anything, testNS, testResourceReleaseBindingName,
			bindingrevision.RollbackRequest{Revision: 1, DryRun: true}).Return(sampleRollbackResult(true), nil)

		h := newTestHandler(withResourceReleaseBindingService(svc))
		result, err := h.RollbackResourceReleaseBinding(ctx, testNS, testResourceReleaseBindingName, 1, true, "")
		require.NoError(t, err)
		m := result.(map[string]any)
		assert.Equal(t, testResourceReleaseBindingName, m["binding"])
		assert.Equal(t, false, m["applied"])
	})
}

// Every rollback tool call is audited, dry runs included: the MCP audit
// adapter seeds the record from the tool arguments before the handler runs.
// The record must say whether the call was a dry run (the default), so a
// preview is not mistaken for an applied rollback.
func TestRollbackAuditRecordsDryRun(t *testing.T) {
	failure := errors.New("etcd is on fire")
	for _, dryRun := range []bool{true, false} {
		for _, svcErr := range []error{nil, failure} {
			name := fmt.Sprintf("dryRun=%t err=%v", dryRun, svcErr)
			result := sampleRollbackResult(dryRun)
			if svcErr != nil {
				result = nil
			}

			t.Run("ReleaseBinding "+name, func(t *testing.T) {
				svc := releasebindingmocks.NewMockService(t)
				svc.EXPECT().RollbackReleaseBinding(mock.Anything, testNS, testReleaseBindingName,
					bindingrevision.RollbackRequest{Revision: 1, DryRun: dryRun}).Return(result, svcErr)
				h := newTestHandler(withReleaseBindingService(svc))

				ctx, data := audit.NewAuditContext(context.Background(),
					&audit.Resource{Namespace: testNS, Name: testReleaseBindingName}, audit.RequestInfo{})
				_, _ = h.RollbackReleaseBinding(ctx, testNS, testReleaseBindingName, 1, dryRun, "")

				require.NotNil(t, data.Resource)
				assert.Equal(t, testNS, data.Resource.Namespace)
				assert.Equal(t, testReleaseBindingName, data.Resource.Name)
				assert.Equal(t, map[string]any{"dryRun": dryRun}, data.Resource.Metadata)
			})

			t.Run("ResourceReleaseBinding "+name, func(t *testing.T) {
				svc := resourcereleasebindingmocks.NewMockService(t)
				svc.EXPECT().RollbackResourceReleaseBinding(mock.Anything, testNS, "db-dev",
					bindingrevision.RollbackRequest{Revision: 1, DryRun: dryRun}).Return(result, svcErr)
				h := newTestHandler(withResourceReleaseBindingService(svc))

				ctx, data := audit.NewAuditContext(context.Background(),
					&audit.Resource{Namespace: testNS, Name: "db-dev"}, audit.RequestInfo{})
				_, _ = h.RollbackResourceReleaseBinding(ctx, testNS, "db-dev", 1, dryRun, "")

				require.NotNil(t, data.Resource)
				assert.Equal(t, "db-dev", data.Resource.Name)
				assert.Equal(t, map[string]any{"dryRun": dryRun}, data.Resource.Metadata)
			})
		}
	}
}
