// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"k8s.io/utils/ptr"

	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/bindingrevision"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/handlerservices"
	releasebindingmocks "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/releasebinding/mocks"
	resourcereleasebindingmocks "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/resourcereleasebinding/mocks"
	"github.com/openchoreo/openchoreo/internal/server/middleware/audit"
)

func TestClassifyRollbackError(t *testing.T) {
	bindingNotFound := errors.New("binding not found")

	tests := []struct {
		name        string
		err         error
		wantOutcome rollbackOutcome
		wantCode    gen.ErrorResponseCode
	}{
		{name: "forbidden", err: fmt.Errorf("check: %w", services.ErrForbidden), wantOutcome: rollbackForbidden},
		{name: "binding not found", err: bindingNotFound, wantOutcome: rollbackNotFound, wantCode: gen.NOTFOUND},
		{name: "revision not found", err: fmt.Errorf("%w: revision 7", bindingrevision.ErrRevisionNotFound),
			wantOutcome: rollbackNotFound, wantCode: gen.NOTFOUND},
		{name: "release deleted", err: fmt.Errorf("%w: ComponentRelease x", bindingrevision.ErrReleaseDeleted),
			wantOutcome: rollbackConflict, wantCode: gen.RELEASEDELETED},
		{name: "concurrent modification", err: fmt.Errorf("%w: binding b", bindingrevision.ErrConcurrentModification),
			wantOutcome: rollbackConflict, wantCode: gen.CONCURRENTMODIFICATION},
		{name: "no previous revision", err: fmt.Errorf("%w: none", bindingrevision.ErrNoPreviousRevision),
			wantOutcome: rollbackConflict, wantCode: gen.NOPREVIOUSREVISION},
		{name: "auto-deploy would undo it", err: fmt.Errorf("%w: component x", bindingrevision.ErrAutoDeployEnabled),
			wantOutcome: rollbackConflict, wantCode: gen.AUTODEPLOYENABLED},
		{name: "managed by GitOps", err: fmt.Errorf("%w: release binding x", bindingrevision.ErrGitOpsManaged),
			wantOutcome: rollbackConflict, wantCode: gen.GITOPSMANAGED},
		{name: "validation", err: &services.ValidationError{Msg: "revision must be at least 1"}, wantOutcome: rollbackBadRequest},
		{name: "anything else", err: errors.New("etcd is on fire"), wantOutcome: rollbackInternalError},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			outcome, body, msg := classifyRollbackError(tt.err, bindingNotFound)
			assert.Equal(t, tt.wantOutcome, outcome)
			assert.Equal(t, tt.wantCode, body.Code)
			if tt.wantOutcome == rollbackBadRequest {
				assert.Equal(t, "revision must be at least 1", msg)
			}
			if tt.wantOutcome == rollbackInternalError {
				assert.Empty(t, body.Error, "internal errors must not leak their message")
			}
		})
	}
}

func TestToRollbackResponse(t *testing.T) {
	resp := toRollbackResponse(&bindingrevision.RollbackResult{
		RevisionName: "api-dev-r6", Revision: 6, TargetRevision: 3, RevertedFrom: ptr.To(int64(5)), DryRun: true,
		Changes:  []bindingrevision.SpecChange{{Field: "releaseName", Current: "api-b", Target: "api-a"}},
		Warnings: []bindingrevision.Warning{{Code: bindingrevision.WarningProjectCellDrift, Message: "cell moved"}},
	})

	assert.Equal(t, "api-dev-r6", resp.RevisionName)
	assert.Equal(t, int64(6), resp.Revision)
	assert.Equal(t, int64(3), resp.TargetRevision)
	assert.Equal(t, ptr.To(int64(5)), resp.RevertedFrom)
	assert.True(t, resp.DryRun)
	assert.Equal(t, []gen.RollbackSpecChange{{Field: "releaseName", Current: "api-b", Target: "api-a"}}, resp.Changes)
	assert.Equal(t, []gen.RollbackWarning{{Code: gen.ProjectCellDrift, Message: "cell moved"}}, resp.Warnings)

	empty := toRollbackResponse(&bindingrevision.RollbackResult{})
	assert.NotNil(t, empty.Changes, "changes is required in the API, so it is [] rather than null")
	assert.NotNil(t, empty.Warnings)
}

// Every call to a rollback route is audited, dry runs included: the audit
// middleware seeds the record from the request path before the handler runs.
// The record must therefore say whether the call was a dry run, which changes
// nothing, so a preview is not mistaken for an applied rollback.
func TestRollbackAuditRecordsDryRun(t *testing.T) {
	const ns, binding = "test-ns", "api-dev"
	failure := errors.New("etcd is on fire")

	for _, dryRun := range []bool{true, false} {
		for _, svcErr := range []error{nil, failure} {
			name := fmt.Sprintf("dryRun=%t err=%v", dryRun, svcErr)
			result := &bindingrevision.RollbackResult{RevisionName: binding + "-r1", Revision: 1, DryRun: dryRun}
			if svcErr != nil {
				result = nil
			}
			body := &gen.RollbackRequest{Revision: ptr.To(int64(1)), DryRun: ptr.To(dryRun)}

			t.Run("ReleaseBinding "+name, func(t *testing.T) {
				svc := releasebindingmocks.NewMockService(t)
				svc.EXPECT().RollbackReleaseBinding(mock.Anything, ns, binding,
					bindingrevision.RollbackRequest{Revision: 1, DryRun: dryRun}).Return(result, svcErr)
				h := &Handler{services: &handlerservices.Services{ReleaseBindingService: svc}, logger: slog.Default()}

				ctx, data := audit.NewAuditContext(context.Background(),
					&audit.Resource{Namespace: ns, Name: binding}, audit.RequestInfo{})
				_, err := h.RollbackReleaseBinding(ctx, gen.RollbackReleaseBindingRequestObject{
					NamespaceName: ns, ReleaseBindingName: binding, Body: body})
				require.NoError(t, err)

				require.NotNil(t, data.Resource)
				assert.Equal(t, ns, data.Resource.Namespace)
				assert.Equal(t, binding, data.Resource.Name)
				assert.Equal(t, map[string]any{"dryRun": dryRun}, data.Resource.Metadata)
			})

			t.Run("ResourceReleaseBinding "+name, func(t *testing.T) {
				svc := resourcereleasebindingmocks.NewMockService(t)
				svc.EXPECT().RollbackResourceReleaseBinding(mock.Anything, ns, binding,
					bindingrevision.RollbackRequest{Revision: 1, DryRun: dryRun}).Return(result, svcErr)
				h := &Handler{services: &handlerservices.Services{ResourceReleaseBindingService: svc}, logger: slog.Default()}

				ctx, data := audit.NewAuditContext(context.Background(),
					&audit.Resource{Namespace: ns, Name: binding}, audit.RequestInfo{})
				_, err := h.RollbackResourceReleaseBinding(ctx, gen.RollbackResourceReleaseBindingRequestObject{
					NamespaceName: ns, ResourceReleaseBindingName: binding, Body: body})
				require.NoError(t, err)

				require.NotNil(t, data.Resource)
				assert.Equal(t, binding, data.Resource.Name)
				assert.Equal(t, map[string]any{"dryRun": dryRun}, data.Resource.Metadata)
			})
		}
	}
}

func TestClassifyDeleteRevisionError(t *testing.T) {
	bindingNotFound := errors.New("binding not found")

	tests := []struct {
		name        string
		err         error
		wantOutcome rollbackOutcome
		wantCode    gen.ErrorResponseCode
		wantMsg     string
	}{
		{name: "forbidden", err: fmt.Errorf("check: %w", services.ErrForbidden), wantOutcome: rollbackForbidden},
		{name: "binding not found", err: bindingNotFound, wantOutcome: rollbackNotFound, wantCode: gen.NOTFOUND},
		{name: "revision not found", err: fmt.Errorf("%w: \"api-dev-r9\"", bindingrevision.ErrRevisionNotFound),
			wantOutcome: rollbackNotFound, wantCode: gen.NOTFOUND, wantMsg: `Revision not found: "api-dev-r9"`},
		{name: "protected", err: &bindingrevision.ProtectedError{Revision: 4, Reason: "it is currently active"},
			wantOutcome: rollbackConflict, wantCode: gen.REVISIONPROTECTED,
			wantMsg: "Cannot delete revision 4 because it is currently active"},
		{name: "concurrent modification", err: fmt.Errorf("%w: revision changed", bindingrevision.ErrConcurrentModification),
			wantOutcome: rollbackConflict, wantCode: gen.CONCURRENTMODIFICATION},
		{name: "anything else", err: errors.New("etcd is on fire"), wantOutcome: rollbackInternalError},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			outcome, body := classifyDeleteRevisionError(tt.err, bindingNotFound)
			assert.Equal(t, tt.wantOutcome, outcome)
			assert.Equal(t, tt.wantCode, body.Code)
			if tt.wantMsg != "" {
				assert.Equal(t, tt.wantMsg, body.Error)
			}
			if tt.wantOutcome == rollbackInternalError {
				assert.Empty(t, body.Error, "internal errors must not leak their message")
			}
		})
	}
}

func TestDeleteRevisionHandlers(t *testing.T) {
	const ns, binding, rev = "test-ns", "api-dev", "api-dev-r2"
	result := &bindingrevision.DeleteResult{RevisionName: rev, RevisionDisplayName: "api-v2-r2", Revision: 2,
		Binding: binding, Environment: "development", DryRun: true}
	want := gen.DeleteBindingRevisionResponse{RevisionName: rev, RevisionDisplayName: ptr.To("api-v2-r2"), Revision: 2,
		Binding: binding, Environment: "development", DryRun: true}
	req := bindingrevision.DeleteRequest{Binding: binding, DryRun: true}
	params := struct {
		Binding *string
		DryRun  *bool
	}{ptr.To(binding), ptr.To(true)}

	t.Run("ReleaseBinding", func(t *testing.T) {
		svc := releasebindingmocks.NewMockService(t)
		svc.EXPECT().DeleteReleaseBindingRevision(mock.Anything, ns, rev, req).Return(result, nil).Once()
		svc.EXPECT().DeleteReleaseBindingRevision(mock.Anything, ns, rev, bindingrevision.DeleteRequest{}).
			Return(nil, &bindingrevision.ProtectedError{Revision: 2, Reason: "it is currently active"}).Once()
		h := &Handler{services: &handlerservices.Services{ReleaseBindingService: svc}, logger: slog.Default()}

		ctx, data := audit.NewAuditContext(context.Background(), &audit.Resource{Namespace: ns, Name: rev}, audit.RequestInfo{})
		resp, err := h.DeleteReleaseBindingRevision(ctx, gen.DeleteReleaseBindingRevisionRequestObject{
			NamespaceName: ns, ReleaseBindingRevisionName: rev,
			Params: gen.DeleteReleaseBindingRevisionParams{Binding: params.Binding, DryRun: params.DryRun}})
		require.NoError(t, err)
		assert.Equal(t, gen.DeleteReleaseBindingRevision200JSONResponse(want), resp)
		assert.Equal(t, map[string]any{"dryRun": true}, data.Resource.Metadata, "a dry run is marked in the audit record")

		resp, err = h.DeleteReleaseBindingRevision(context.Background(), gen.DeleteReleaseBindingRevisionRequestObject{
			NamespaceName: ns, ReleaseBindingRevisionName: rev})
		require.NoError(t, err)
		assert.Equal(t, gen.DeleteReleaseBindingRevision409JSONResponse{RevisionDeleteConflictJSONResponse: gen.RevisionDeleteConflictJSONResponse{
			Code: gen.REVISIONPROTECTED, Error: "Cannot delete revision 2 because it is currently active"}}, resp)
	})

	t.Run("ResourceReleaseBinding", func(t *testing.T) {
		svc := resourcereleasebindingmocks.NewMockService(t)
		svc.EXPECT().DeleteResourceReleaseBindingRevision(mock.Anything, ns, rev, req).Return(result, nil).Once()
		svc.EXPECT().DeleteResourceReleaseBindingRevision(mock.Anything, ns, rev, bindingrevision.DeleteRequest{}).
			Return(nil, fmt.Errorf("%w: %q", bindingrevision.ErrRevisionNotFound, rev)).Once()
		h := &Handler{services: &handlerservices.Services{ResourceReleaseBindingService: svc}, logger: slog.Default()}

		resp, err := h.DeleteResourceReleaseBindingRevision(context.Background(), gen.DeleteResourceReleaseBindingRevisionRequestObject{
			NamespaceName: ns, ResourceReleaseBindingRevisionName: rev,
			Params: gen.DeleteResourceReleaseBindingRevisionParams{Binding: params.Binding, DryRun: params.DryRun}})
		require.NoError(t, err)
		assert.Equal(t, gen.DeleteResourceReleaseBindingRevision200JSONResponse(want), resp)

		resp, err = h.DeleteResourceReleaseBindingRevision(context.Background(), gen.DeleteResourceReleaseBindingRevisionRequestObject{
			NamespaceName: ns, ResourceReleaseBindingRevisionName: rev})
		require.NoError(t, err)
		assert.IsType(t, gen.DeleteResourceReleaseBindingRevision404JSONResponse{}, resp)
	})
}

func TestToRollbackRequest(t *testing.T) {
	req, invalid := toRollbackRequest(&gen.RollbackRequest{})
	assert.Empty(t, invalid)
	assert.Equal(t, bindingrevision.RollbackRequest{}, req, "no revision: the previously active revision (0)")

	req, invalid = toRollbackRequest(&gen.RollbackRequest{Revision: ptr.To(int64(3)), DryRun: ptr.To(true)})
	assert.Empty(t, invalid)
	assert.Equal(t, bindingrevision.RollbackRequest{Revision: 3, DryRun: true}, req)

	for _, n := range []int64{0, -2} {
		_, invalid = toRollbackRequest(&gen.RollbackRequest{Revision: ptr.To(n)})
		assert.Contains(t, invalid, "revision must be at least 1", "an explicit revision %d is not a request for the previous one", n)
	}

	_, invalid = toRollbackRequest(nil)
	assert.Equal(t, "Request body is required", invalid)

	req, invalid = toRollbackRequest(&gen.RollbackRequest{Message: ptr.To("  Payments failing  ")})
	assert.Empty(t, invalid)
	assert.Equal(t, "Payments failing", req.Message, "surrounding whitespace is dropped")

	_, invalid = toRollbackRequest(&gen.RollbackRequest{Message: ptr.To(strings.Repeat("é", 1025))})
	assert.Equal(t, "message must be at most 1024 characters", invalid, "counted in characters, not bytes")

	_, invalid = toRollbackRequest(&gen.RollbackRequest{Message: ptr.To(strings.Repeat("é", 1024))})
	assert.Empty(t, invalid)
}

func TestRollbackAuditRecordsTheMessage(t *testing.T) {
	const ns, binding = "test-ns", "api-dev"
	svc := releasebindingmocks.NewMockService(t)
	svc.EXPECT().RollbackReleaseBinding(mock.Anything, ns, binding,
		bindingrevision.RollbackRequest{Revision: 1, Message: "Payments failing"}).
		Return(&bindingrevision.RollbackResult{RevisionName: binding + "-r1", Revision: 1}, nil)
	h := &Handler{services: &handlerservices.Services{ReleaseBindingService: svc}, logger: slog.Default()}

	ctx, data := audit.NewAuditContext(context.Background(),
		&audit.Resource{Namespace: ns, Name: binding}, audit.RequestInfo{})
	_, err := h.RollbackReleaseBinding(ctx, gen.RollbackReleaseBindingRequestObject{
		NamespaceName: ns, ReleaseBindingName: binding,
		Body: &gen.RollbackRequest{Revision: ptr.To(int64(1)), Message: ptr.To("Payments failing")}})
	require.NoError(t, err)
	assert.Equal(t, map[string]any{"dryRun": false, "message": "Payments failing"}, data.Resource.Metadata)
}

func TestRollbackWithoutRevision(t *testing.T) {
	const ns, binding = "test-ns", "api-dev"
	noPrevious := fmt.Errorf("%w: the previously active revision has been deleted", bindingrevision.ErrNoPreviousRevision)

	t.Run("ReleaseBinding: the service resolves the previous revision", func(t *testing.T) {
		svc := releasebindingmocks.NewMockService(t)
		svc.EXPECT().RollbackReleaseBinding(mock.Anything, ns, binding, bindingrevision.RollbackRequest{DryRun: true}).
			Return(&bindingrevision.RollbackResult{RevisionName: binding + "-r1", Revision: 1, TargetRevision: 1, DryRun: true}, nil).Once()
		svc.EXPECT().RollbackReleaseBinding(mock.Anything, ns, binding, bindingrevision.RollbackRequest{}).
			Return(nil, noPrevious).Once()
		h := &Handler{services: &handlerservices.Services{ReleaseBindingService: svc}, logger: slog.Default()}

		resp, err := h.RollbackReleaseBinding(context.Background(), gen.RollbackReleaseBindingRequestObject{
			NamespaceName: ns, ReleaseBindingName: binding, Body: &gen.RollbackRequest{DryRun: ptr.To(true)}})
		require.NoError(t, err)
		assert.Equal(t, int64(1), resp.(gen.RollbackReleaseBinding200JSONResponse).TargetRevision)

		resp, err = h.RollbackReleaseBinding(context.Background(), gen.RollbackReleaseBindingRequestObject{
			NamespaceName: ns, ReleaseBindingName: binding, Body: &gen.RollbackRequest{}})
		require.NoError(t, err)
		conflict := resp.(gen.RollbackReleaseBinding409JSONResponse)
		assert.Equal(t, gen.NOPREVIOUSREVISION, conflict.Code)
		assert.Equal(t, "No previous revision to roll back to: the previously active revision has been deleted", conflict.Error)
	})

	t.Run("ResourceReleaseBinding: an explicit revision 0 is a 400", func(t *testing.T) {
		svc := resourcereleasebindingmocks.NewMockService(t)
		h := &Handler{services: &handlerservices.Services{ResourceReleaseBindingService: svc}, logger: slog.Default()}

		resp, err := h.RollbackResourceReleaseBinding(context.Background(), gen.RollbackResourceReleaseBindingRequestObject{
			NamespaceName: ns, ResourceReleaseBindingName: binding, Body: &gen.RollbackRequest{Revision: ptr.To(int64(0))}})
		require.NoError(t, err)
		assert.IsType(t, gen.RollbackResourceReleaseBinding400JSONResponse{}, resp)
	})
}
