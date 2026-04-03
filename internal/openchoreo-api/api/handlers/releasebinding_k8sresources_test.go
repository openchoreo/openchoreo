// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/models"
	svcpkg "github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/handlerservices"
	k8sresourcessvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/k8sresources"
)

type mockK8sResourcesService struct {
	treeFn   func(ctx context.Context, namespace, rb string) (*k8sresourcessvc.K8sResourceTreeResult, error)
	eventsFn func(ctx context.Context, namespace, rb, group, version, kind, name string) (*models.ResourceEventsResponse, error)
	logsFn   func(ctx context.Context, namespace, rb, pod string, sinceSeconds *int64) (*models.ResourcePodLogsResponse, error)
}

var _ k8sresourcessvc.Service = (*mockK8sResourcesService)(nil)

func (m *mockK8sResourcesService) GetResourceTree(ctx context.Context, namespaceName, releaseBindingName string) (*k8sresourcessvc.K8sResourceTreeResult, error) {
	if m.treeFn == nil {
		panic("GetResourceTree not configured")
	}
	return m.treeFn(ctx, namespaceName, releaseBindingName)
}
func (m *mockK8sResourcesService) GetResourceEvents(ctx context.Context, namespaceName, releaseBindingName, group, version, kind, name string) (*models.ResourceEventsResponse, error) {
	if m.eventsFn == nil {
		panic("GetResourceEvents not configured")
	}
	return m.eventsFn(ctx, namespaceName, releaseBindingName, group, version, kind, name)
}
func (m *mockK8sResourcesService) GetResourceLogs(ctx context.Context, namespaceName, releaseBindingName, podName string, sinceSeconds *int64) (*models.ResourcePodLogsResponse, error) {
	if m.logsFn == nil {
		panic("GetResourceLogs not configured")
	}
	return m.logsFn(ctx, namespaceName, releaseBindingName, podName, sinceSeconds)
}

func TestGetReleaseBindingK8sResourceTreeHandler_MapsNotFoundAndForbidden(t *testing.T) {
	ctx := testContext()

	tests := []struct {
		name    string
		svcErr  error
		wantTyp any
	}{
		{"forbidden -> 403", svcpkg.ErrForbidden, gen.GetReleaseBindingK8sResourceTree403JSONResponse{}},
		{"releasebinding not found -> 404", k8sresourcessvc.ErrReleaseBindingNotFound, gen.GetReleaseBindingK8sResourceTree404JSONResponse{}},
		{"rendered release not found -> 404", k8sresourcessvc.ErrRenderedReleaseNotFound, gen.GetReleaseBindingK8sResourceTree404JSONResponse{}},
		{"environment not found -> 404", k8sresourcessvc.ErrEnvironmentNotFound, gen.GetReleaseBindingK8sResourceTree404JSONResponse{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := &Handler{
				services: &handlerservices.Services{
					K8sResourcesService: &mockK8sResourcesService{
						treeFn: func(context.Context, string, string) (*k8sresourcessvc.K8sResourceTreeResult, error) {
							return nil, tt.svcErr
						},
					},
				},
				logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
			}

			resp, err := h.GetReleaseBindingK8sResourceTree(ctx, gen.GetReleaseBindingK8sResourceTreeRequestObject{
				NamespaceName:      "test-ns",
				ReleaseBindingName: "rb-1",
			})
			require.NoError(t, err)
			assert.IsType(t, tt.wantTyp, resp)
		})
	}
}

func TestGetReleaseBindingK8sResourceTreeHandler_ConvertsReleasesAndOptionalRenderedRelease(t *testing.T) {
	ctx := testContext()

	rendered := &openchoreov1alpha1.RenderedRelease{}
	rendered.Name = "rr-1"

	h := &Handler{
		services: &handlerservices.Services{
			K8sResourcesService: &mockK8sResourcesService{
				treeFn: func(context.Context, string, string) (*k8sresourcessvc.K8sResourceTreeResult, error) {
					return &k8sresourcessvc.K8sResourceTreeResult{
						RenderedReleases: []k8sresourcessvc.ReleaseResourceTree{
							{
								Name:        "rel-a",
								TargetPlane: "dataplane",
								Nodes: []models.ResourceNode{
									{Kind: "Deployment", Name: "dep-a"},
								},
								Release: rendered,
							},
							{
								Name:        "rel-b",
								TargetPlane: "observabilityplane",
								Nodes:       nil,
								Release:     nil,
							},
						},
					}, nil
				},
			},
		},
		logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}

	resp, err := h.GetReleaseBindingK8sResourceTree(ctx, gen.GetReleaseBindingK8sResourceTreeRequestObject{
		NamespaceName:      "test-ns",
		ReleaseBindingName: "rb-1",
	})
	require.NoError(t, err)

	typed, ok := resp.(gen.GetReleaseBindingK8sResourceTree200JSONResponse)
	require.True(t, ok, "expected 200 response, got %T", resp)
	require.Len(t, typed.RenderedReleases, 2)
	assert.Equal(t, gen.ReleaseResourceTreeTargetPlaneDataplane, typed.RenderedReleases[0].TargetPlane)
	require.Len(t, typed.RenderedReleases[0].Nodes, 1)
	require.NotNil(t, typed.RenderedReleases[0].RenderedRelease)
	assert.Equal(t, "rr-1", typed.RenderedReleases[0].RenderedRelease.Metadata.Name)

	assert.Equal(t, gen.ReleaseResourceTreeTargetPlaneObservabilityplane, typed.RenderedReleases[1].TargetPlane)
	assert.Nil(t, typed.RenderedReleases[1].RenderedRelease)
}

func TestGetReleaseBindingK8sResourceEventsHandler_DefaultsGroupToEmptyString(t *testing.T) {
	ctx := testContext()

	h := &Handler{
		services: &handlerservices.Services{
			K8sResourcesService: &mockK8sResourcesService{
				eventsFn: func(_ context.Context, namespace, rb, group, version, kind, name string) (*models.ResourceEventsResponse, error) {
					assert.Equal(t, "", group)
					assert.Equal(t, "v1", version)
					assert.Equal(t, "Pod", kind)
					assert.Equal(t, "p1", name)
					return &models.ResourceEventsResponse{Events: nil}, nil
				},
			},
		},
		logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}

	resp, err := h.GetReleaseBindingK8sResourceEvents(ctx, gen.GetReleaseBindingK8sResourceEventsRequestObject{
		NamespaceName:      "test-ns",
		ReleaseBindingName: "rb-1",
		Params: gen.GetReleaseBindingK8sResourceEventsParams{
			Version: "v1",
			Kind:    "Pod",
			Name:    "p1",
		},
	})
	require.NoError(t, err)
	assert.IsType(t, gen.GetReleaseBindingK8sResourceEvents200JSONResponse{}, resp)
}

func TestGetReleaseBindingK8sResourceEventsHandler_MapsErrors(t *testing.T) {
	ctx := testContext()

	tests := []struct {
		name    string
		svcErr  error
		wantTyp any
	}{
		{"forbidden -> 403", svcpkg.ErrForbidden, gen.GetReleaseBindingK8sResourceEvents403JSONResponse{}},
		{"releasebinding not found -> 404", k8sresourcessvc.ErrReleaseBindingNotFound, gen.GetReleaseBindingK8sResourceEvents404JSONResponse{}},
		{"rendered release not found -> 404", k8sresourcessvc.ErrRenderedReleaseNotFound, gen.GetReleaseBindingK8sResourceEvents404JSONResponse{}},
		{"environment not found -> 404", k8sresourcessvc.ErrEnvironmentNotFound, gen.GetReleaseBindingK8sResourceEvents404JSONResponse{}},
		{"resource not found -> 404", k8sresourcessvc.ErrResourceNotFound, gen.GetReleaseBindingK8sResourceEvents404JSONResponse{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := &Handler{
				services: &handlerservices.Services{
					K8sResourcesService: &mockK8sResourcesService{
						eventsFn: func(context.Context, string, string, string, string, string, string) (*models.ResourceEventsResponse, error) {
							return nil, tt.svcErr
						},
					},
				},
				logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
			}
			resp, err := h.GetReleaseBindingK8sResourceEvents(ctx, gen.GetReleaseBindingK8sResourceEventsRequestObject{
				NamespaceName:      "test-ns",
				ReleaseBindingName: "rb-1",
				Params:             gen.GetReleaseBindingK8sResourceEventsParams{Version: "v1", Kind: "Pod", Name: "p1"},
			})
			require.NoError(t, err)
			assert.IsType(t, tt.wantTyp, resp)
		})
	}
}

func TestGetReleaseBindingK8sResourceEventsHandler_SuccessReturnsEvents(t *testing.T) {
	ctx := testContext()

	h := &Handler{
		services: &handlerservices.Services{
			K8sResourcesService: &mockK8sResourcesService{
				eventsFn: func(context.Context, string, string, string, string, string, string) (*models.ResourceEventsResponse, error) {
					return &models.ResourceEventsResponse{Events: []models.ResourceEvent{
						{Type: "Normal", Reason: "Started", Message: "container started"},
					}}, nil
				},
			},
		},
		logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}

	resp, err := h.GetReleaseBindingK8sResourceEvents(ctx, gen.GetReleaseBindingK8sResourceEventsRequestObject{
		NamespaceName:      "test-ns",
		ReleaseBindingName: "rb-1",
		Params:             gen.GetReleaseBindingK8sResourceEventsParams{Version: "v1", Kind: "Pod", Name: "p1"},
	})
	require.NoError(t, err)

	typed, ok := resp.(gen.GetReleaseBindingK8sResourceEvents200JSONResponse)
	require.True(t, ok, "expected 200 response, got %T", resp)
	require.Len(t, typed.Events, 1)
	assert.Equal(t, "Normal", typed.Events[0].Type)
	assert.Equal(t, "Started", typed.Events[0].Reason)
}

func TestGetReleaseBindingK8sResourceLogsHandler_SuccessReturnsLogs(t *testing.T) {
	ctx := testContext()

	h := &Handler{
		services: &handlerservices.Services{
			K8sResourcesService: &mockK8sResourcesService{
				logsFn: func(context.Context, string, string, string, *int64) (*models.ResourcePodLogsResponse, error) {
					return &models.ResourcePodLogsResponse{LogEntries: []models.PodLogEntry{
						{Timestamp: "2026-01-02T03:04:05Z", Log: "hello"},
					}}, nil
				},
			},
		},
		logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}

	resp, err := h.GetReleaseBindingK8sResourceLogs(ctx, gen.GetReleaseBindingK8sResourceLogsRequestObject{
		NamespaceName:      "test-ns",
		ReleaseBindingName: "rb-1",
		Params:             gen.GetReleaseBindingK8sResourceLogsParams{PodName: "pod-1"},
	})
	require.NoError(t, err)

	typed, ok := resp.(gen.GetReleaseBindingK8sResourceLogs200JSONResponse)
	require.True(t, ok, "expected 200 response, got %T", resp)
	require.Len(t, typed.LogEntries, 1)
	assert.Equal(t, "hello", typed.LogEntries[0].Log)
}

func TestGetReleaseBindingK8sResourceLogsHandler_MapsErrors(t *testing.T) {
	ctx := testContext()

	tests := []struct {
		name    string
		svcErr  error
		wantTyp any
	}{
		{"forbidden -> 403", svcpkg.ErrForbidden, gen.GetReleaseBindingK8sResourceLogs403JSONResponse{}},
		{"releasebinding not found -> 404", k8sresourcessvc.ErrReleaseBindingNotFound, gen.GetReleaseBindingK8sResourceLogs404JSONResponse{}},
		{"rendered release not found -> 404", k8sresourcessvc.ErrRenderedReleaseNotFound, gen.GetReleaseBindingK8sResourceLogs404JSONResponse{}},
		{"environment not found -> 404", k8sresourcessvc.ErrEnvironmentNotFound, gen.GetReleaseBindingK8sResourceLogs404JSONResponse{}},
		{"resource not found -> 404", k8sresourcessvc.ErrResourceNotFound, gen.GetReleaseBindingK8sResourceLogs404JSONResponse{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := &Handler{
				services: &handlerservices.Services{
					K8sResourcesService: &mockK8sResourcesService{
						logsFn: func(context.Context, string, string, string, *int64) (*models.ResourcePodLogsResponse, error) {
							return nil, tt.svcErr
						},
					},
				},
				logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
			}
			resp, err := h.GetReleaseBindingK8sResourceLogs(ctx, gen.GetReleaseBindingK8sResourceLogsRequestObject{
				NamespaceName:      "test-ns",
				ReleaseBindingName: "rb-1",
				Params:             gen.GetReleaseBindingK8sResourceLogsParams{PodName: "pod-1"},
			})
			require.NoError(t, err)
			assert.IsType(t, tt.wantTyp, resp)
		})
	}
}
