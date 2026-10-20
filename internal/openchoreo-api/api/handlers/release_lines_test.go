// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
	svcpkg "github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/handlerservices"
	releaselinesvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/releaseline"
	releaselinesvcmocks "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/releaseline/mocks"
)

func releaseLineHandler(svc *releaselinesvcmocks.MockService) *Handler {
	return &Handler{
		services: &handlerservices.Services{ReleaseLineService: svc},
		logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
}

func TestGetComponentReleaseLineHandler(t *testing.T) {
	ctx := testContext()

	t.Run("returns the line", func(t *testing.T) {
		svc := releaselinesvcmocks.NewMockService(t)
		svc.EXPECT().GetReleaseLine(mock.Anything, "test-ns", "hello", "next").
			Return(&openchoreov1alpha1.ReleaseLine{ObjectMeta: metav1.ObjectMeta{Name: "next"}}, nil)
		resp, err := releaseLineHandler(svc).GetComponentReleaseLine(ctx, gen.GetComponentReleaseLineRequestObject{
			NamespaceName: "test-ns", ComponentName: "hello", ReleaseLineName: "next",
		})
		require.NoError(t, err)
		typed, ok := resp.(gen.GetComponentReleaseLine200JSONResponse)
		require.True(t, ok, "expected 200 response, got %T", resp)
		assert.Equal(t, "next", typed.Metadata.Name)
	})

	for name, tc := range map[string]struct {
		err  error
		want any
	}{
		"forbidden":         {svcpkg.ErrForbidden, gen.GetComponentReleaseLine403JSONResponse{}},
		"unknown component": {releaselinesvc.ErrComponentNotFound, gen.GetComponentReleaseLine404JSONResponse{}},
		"unknown line":      {releaselinesvc.ErrReleaseLineNotFound, gen.GetComponentReleaseLine404JSONResponse{}},
	} {
		t.Run(name, func(t *testing.T) {
			svc := releaselinesvcmocks.NewMockService(t)
			svc.EXPECT().GetReleaseLine(mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil, tc.err)
			resp, err := releaseLineHandler(svc).GetComponentReleaseLine(ctx, gen.GetComponentReleaseLineRequestObject{
				NamespaceName: "test-ns", ComponentName: "hello", ReleaseLineName: "next",
			})
			require.NoError(t, err)
			assert.IsType(t, tc.want, resp)
		})
	}
}

func TestCreateComponentReleaseLineHandler(t *testing.T) {
	ctx := testContext()

	t.Run("creates the line", func(t *testing.T) {
		svc := releaselinesvcmocks.NewMockService(t)
		svc.EXPECT().CreateReleaseLine(mock.Anything, "test-ns", "hello", mock.Anything).
			RunAndReturn(func(_ context.Context, _, _ string, rl *openchoreov1alpha1.ReleaseLine) (*openchoreov1alpha1.ReleaseLine, error) {
				assert.Equal(t, "next", rl.Name)
				return rl, nil
			})
		resp, err := releaseLineHandler(svc).CreateComponentReleaseLine(ctx, gen.CreateComponentReleaseLineRequestObject{
			NamespaceName: "test-ns", ComponentName: "hello",
			Body: &gen.ReleaseLine{Metadata: gen.ObjectMeta{Name: "next"}},
		})
		require.NoError(t, err)
		assert.IsType(t, gen.CreateComponentReleaseLine201JSONResponse{}, resp)
	})

	t.Run("namespace mismatch", func(t *testing.T) {
		svc := releaselinesvcmocks.NewMockService(t)
		resp, err := releaseLineHandler(svc).CreateComponentReleaseLine(ctx, gen.CreateComponentReleaseLineRequestObject{
			NamespaceName: "test-ns", ComponentName: "hello",
			Body: &gen.ReleaseLine{Metadata: gen.ObjectMeta{Name: "next", Namespace: ptr.To("other-ns")}},
		})
		require.NoError(t, err)
		assert.IsType(t, gen.CreateComponentReleaseLine400JSONResponse{}, resp)
	})

	for name, tc := range map[string]struct {
		err  error
		want any
	}{
		"forbidden":          {svcpkg.ErrForbidden, gen.CreateComponentReleaseLine403JSONResponse{}},
		"unknown component":  {releaselinesvc.ErrComponentNotFound, gen.CreateComponentReleaseLine404JSONResponse{}},
		"lines not enabled":  {releaselinesvc.ErrReleaseLinesNotEnabled, gen.CreateComponentReleaseLine400JSONResponse{}},
		"name taken":         {releaselinesvc.ErrReleaseLineAlreadyExists, gen.CreateComponentReleaseLine409JSONResponse{}},
		"owner of other one": {&svcpkg.ValidationError{Msg: "spec.owner.componentName must be \"hello\""}, gen.CreateComponentReleaseLine400JSONResponse{}},
	} {
		t.Run(name, func(t *testing.T) {
			svc := releaselinesvcmocks.NewMockService(t)
			svc.EXPECT().CreateReleaseLine(mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil, tc.err)
			resp, err := releaseLineHandler(svc).CreateComponentReleaseLine(ctx, gen.CreateComponentReleaseLineRequestObject{
				NamespaceName: "test-ns", ComponentName: "hello",
				Body: &gen.ReleaseLine{Metadata: gen.ObjectMeta{Name: "next"}},
			})
			require.NoError(t, err)
			assert.IsType(t, tc.want, resp)
		})
	}
}

func TestUpdateComponentReleaseLineHandler(t *testing.T) {
	ctx := testContext()

	t.Run("uses the path name", func(t *testing.T) {
		svc := releaselinesvcmocks.NewMockService(t)
		svc.EXPECT().UpdateReleaseLine(mock.Anything, "test-ns", "hello", mock.Anything).
			RunAndReturn(func(_ context.Context, _, _ string, rl *openchoreov1alpha1.ReleaseLine) (*openchoreov1alpha1.ReleaseLine, error) {
				assert.Equal(t, "next", rl.Name, "path must override body name")
				return rl, nil
			})
		resp, err := releaseLineHandler(svc).UpdateComponentReleaseLine(ctx, gen.UpdateComponentReleaseLineRequestObject{
			NamespaceName: "test-ns", ComponentName: "hello", ReleaseLineName: "next",
			Body: &gen.ReleaseLine{Metadata: gen.ObjectMeta{Name: "from-body"}},
		})
		require.NoError(t, err)
		assert.IsType(t, gen.UpdateComponentReleaseLine200JSONResponse{}, resp)
	})

	t.Run("unknown line", func(t *testing.T) {
		svc := releaselinesvcmocks.NewMockService(t)
		svc.EXPECT().UpdateReleaseLine(mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil, releaselinesvc.ErrReleaseLineNotFound)
		resp, err := releaseLineHandler(svc).UpdateComponentReleaseLine(ctx, gen.UpdateComponentReleaseLineRequestObject{
			NamespaceName: "test-ns", ComponentName: "hello", ReleaseLineName: "next",
			Body: &gen.ReleaseLine{Metadata: gen.ObjectMeta{Name: "next"}},
		})
		require.NoError(t, err)
		assert.IsType(t, gen.UpdateComponentReleaseLine404JSONResponse{}, resp)
	})
}

func TestDeleteComponentReleaseLineHandler(t *testing.T) {
	ctx := testContext()

	for name, tc := range map[string]struct {
		err  error
		want any
	}{
		"deleted":      {nil, gen.DeleteComponentReleaseLine204Response{}},
		"forbidden":    {svcpkg.ErrForbidden, gen.DeleteComponentReleaseLine403JSONResponse{}},
		"unknown line": {releaselinesvc.ErrReleaseLineNotFound, gen.DeleteComponentReleaseLine404JSONResponse{}},
	} {
		t.Run(name, func(t *testing.T) {
			svc := releaselinesvcmocks.NewMockService(t)
			svc.EXPECT().DeleteReleaseLine(mock.Anything, "test-ns", "hello", "next").Return(tc.err)
			resp, err := releaseLineHandler(svc).DeleteComponentReleaseLine(ctx, gen.DeleteComponentReleaseLineRequestObject{
				NamespaceName: "test-ns", ComponentName: "hello", ReleaseLineName: "next",
			})
			require.NoError(t, err)
			assert.IsType(t, tc.want, resp)
		})
	}
}
