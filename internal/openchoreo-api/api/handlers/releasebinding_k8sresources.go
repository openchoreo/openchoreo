// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"regexp"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/models"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
	k8sresourcessvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/k8sresources"
)

// GetReleaseBindingK8sResourceTree returns all live Kubernetes resources deployed by the releases
// owned by a release binding.
func (h *Handler) GetReleaseBindingK8sResourceTree(
	ctx context.Context,
	request gen.GetReleaseBindingK8sResourceTreeRequestObject,
) (gen.GetReleaseBindingK8sResourceTreeResponseObject, error) {
	h.logger.Debug("GetReleaseBindingK8sResourceTree called",
		"namespace", request.NamespaceName,
		"releaseBinding", request.ReleaseBindingName)

	result, err := h.services.K8sResourcesService.GetResourceTree(ctx, request.NamespaceName, request.ReleaseBindingName)
	if err != nil {
		return h.handleK8sResourceTreeError(err)
	}

	genReleases := make([]gen.ReleaseResourceTree, 0, len(result.RenderedReleases))
	for _, r := range result.RenderedReleases {
		nodes, err := convertList[models.ResourceNode, gen.ResourceNode](r.Nodes)
		if err != nil {
			h.logger.Error("Failed to convert resource nodes", "error", err)
			return gen.GetReleaseBindingK8sResourceTree500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
		}
		entry := gen.ReleaseResourceTree{
			Name:        r.Name,
			TargetPlane: gen.ReleaseResourceTreeTargetPlane(r.TargetPlane),
			Nodes:       nodes,
		}

		if r.Release != nil {
			genRelease, err := convert[openchoreov1alpha1.RenderedRelease, gen.RenderedRelease](*r.Release)
			if err != nil {
				h.logger.Error("Failed to convert rendered release", "error", err)
				return gen.GetReleaseBindingK8sResourceTree500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
			}
			entry.RenderedRelease = &genRelease
		}

		genReleases = append(genReleases, entry)
	}

	return gen.GetReleaseBindingK8sResourceTree200JSONResponse{
		RenderedReleases: genReleases,
	}, nil
}

// GetReleaseBindingK8sResourceEvents returns Kubernetes events for a specific resource
// in the release binding's resource tree.
func (h *Handler) GetReleaseBindingK8sResourceEvents(
	ctx context.Context,
	request gen.GetReleaseBindingK8sResourceEventsRequestObject,
) (gen.GetReleaseBindingK8sResourceEventsResponseObject, error) {
	h.logger.Debug("GetReleaseBindingK8sResourceEvents called",
		"namespace", request.NamespaceName,
		"releaseBinding", request.ReleaseBindingName,
		"kind", request.Params.Kind,
		"name", request.Params.Name)

	group := ""
	if request.Params.Group != nil {
		group = *request.Params.Group
	}

	resp, err := h.services.K8sResourcesService.GetResourceEvents(
		ctx,
		request.NamespaceName,
		request.ReleaseBindingName,
		group,
		request.Params.Version,
		request.Params.Kind,
		request.Params.Name,
	)
	if err != nil {
		return h.handleK8sResourceEventsError(err)
	}

	result, err := convert[models.ResourceEventsResponse, gen.ResourceEventsResponse](*resp)
	if err != nil {
		h.logger.Error("Failed to convert resource events response", "error", err)
		return gen.GetReleaseBindingK8sResourceEvents500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	return gen.GetReleaseBindingK8sResourceEvents200JSONResponse(result), nil
}

// GetReleaseBindingK8sResourceLogs returns logs for a specific pod in the release binding's resource tree.
func (h *Handler) GetReleaseBindingK8sResourceLogs(
	ctx context.Context,
	request gen.GetReleaseBindingK8sResourceLogsRequestObject,
) (gen.GetReleaseBindingK8sResourceLogsResponseObject, error) {
	container := ""
	if request.Params.Container != nil {
		container = *request.Params.Container
	}

	h.logger.Debug("GetReleaseBindingK8sResourceLogs called",
		"namespace", request.NamespaceName,
		"releaseBinding", request.ReleaseBindingName,
		"podName", request.Params.PodName,
		"container", container)

	resp, err := h.services.K8sResourcesService.GetResourceLogs(
		ctx,
		request.NamespaceName,
		request.ReleaseBindingName,
		request.Params.PodName,
		container,
		request.Params.SinceSeconds,
	)
	if err != nil {
		return h.handleK8sResourceLogsError(err)
	}

	result, err := convert[models.ResourcePodLogsResponse, gen.ResourcePodLogsResponse](*resp)
	if err != nil {
		h.logger.Error("Failed to convert resource pod logs response", "error", err)
		return gen.GetReleaseBindingK8sResourceLogs500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	return gen.GetReleaseBindingK8sResourceLogs200JSONResponse(result), nil
}

const maxTriggerPayloadSize = 64 << 10

// triggerPathPattern matches the cronjob trigger subresource:
// /api/v1alpha1/namespaces/{namespaceName}/releasebindings/{releaseBindingName}/trigger
var triggerPathPattern = regexp.MustCompile(
	`^/api/v1alpha1/namespaces/[^/]+/releasebindings/[^/]+/trigger$`)

// OptionalTriggerBodyMiddleware lets the cronjob trigger endpoint be called with no request body.
//
// The endpoint declares an optional requestBody so callers can pass args, but oapi-codegen's strict
// handler always decodes the body and rejects an empty one with "can't decode JSON body: EOF"
// (the generated template has no branch for requestBody.required). Substituting an empty JSON
// object keeps the original bodyless `POST .../trigger` working, which existing clients rely on.
func OptionalTriggerBodyMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && triggerPathPattern.MatchString(r.URL.Path) && r.Body != nil {
			r.Body = http.MaxBytesReader(w, r.Body, maxTriggerPayloadSize)
			body, err := io.ReadAll(r.Body)
			if err != nil {
				var maxBytesErr *http.MaxBytesError
				if errors.As(err, &maxBytesErr) {
					http.Error(w, "request body too large", http.StatusRequestEntityTooLarge)
				} else {
					http.Error(w, "failed to read request body", http.StatusBadRequest)
				}
				return
			}
			if len(bytes.TrimSpace(body)) == 0 {
				body = []byte("{}")
			}
			r.Body = io.NopCloser(bytes.NewReader(body))
			r.ContentLength = int64(len(body))
		}
		next.ServeHTTP(w, r)
	})
}

// TriggerReleaseBindingCronJob creates a Job from the deployed CronJob's jobTemplate for a
// cronjob workload component, matching `kubectl create job --from=cronjob/<name>`.
func (h *Handler) TriggerReleaseBindingCronJob(
	ctx context.Context,
	request gen.TriggerReleaseBindingCronJobRequestObject,
) (gen.TriggerReleaseBindingCronJobResponseObject, error) {
	h.logger.Debug("TriggerReleaseBindingCronJob called",
		"namespace", request.NamespaceName,
		"releaseBinding", request.ReleaseBindingName)

	// An omitted args field or an explicit null leaves Args nil, which keeps the jobTemplate's
	// args; a bodyless request reaches here as "{}" via OptionalTriggerBodyMiddleware. A present
	// `"args": []` decodes to a non-nil empty slice and is a deliberate "run with no args", so it
	// must stay distinguishable from nil.
	var overrides *models.CronJobTriggerRequest
	if request.Body != nil && request.Body.Args != nil {
		overrides = &models.CronJobTriggerRequest{Args: *request.Body.Args}
	}

	resp, err := h.services.K8sResourcesService.TriggerCronJob(ctx, request.NamespaceName, request.ReleaseBindingName, overrides)
	if err != nil {
		return h.handleTriggerCronJobError(err)
	}

	result, err := convert[models.CronJobTriggerResponse, gen.CronJobTriggerResponse](*resp)
	if err != nil {
		h.logger.Error("Failed to convert cronjob trigger response", "error", err)
		return gen.TriggerReleaseBindingCronJob500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
	}

	return gen.TriggerReleaseBindingCronJob200JSONResponse(result), nil
}

func (h *Handler) handleTriggerCronJobError(err error) (gen.TriggerReleaseBindingCronJobResponseObject, error) {
	if errors.Is(err, services.ErrForbidden) {
		return gen.TriggerReleaseBindingCronJob403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
	}
	if errors.Is(err, k8sresourcessvc.ErrNotCronJobWorkload) {
		return gen.TriggerReleaseBindingCronJob400JSONResponse{
			BadRequestJSONResponse: badRequest("release binding component is not a cronjob workload"),
		}, nil
	}
	if errors.Is(err, k8sresourcessvc.ErrTriggerContainerAmbiguous) {
		return gen.TriggerReleaseBindingCronJob400JSONResponse{
			BadRequestJSONResponse: badRequest(
				"cannot apply args: expected a container named \"main\" or a single-container pod"),
		}, nil
	}
	if errors.Is(err, k8sresourcessvc.ErrTriggerConflict) {
		return gen.TriggerReleaseBindingCronJob400JSONResponse{
			BadRequestJSONResponse: badRequest("a job with the same name already exists, retry the trigger"),
		}, nil
	}
	if errors.Is(err, k8sresourcessvc.ErrReleaseBindingNotFound) {
		return gen.TriggerReleaseBindingCronJob404JSONResponse{NotFoundJSONResponse: notFound("ReleaseBinding")}, nil
	}
	if errors.Is(err, k8sresourcessvc.ErrComponentReleaseNotFound) {
		return gen.TriggerReleaseBindingCronJob404JSONResponse{NotFoundJSONResponse: notFound("ComponentRelease")}, nil
	}
	if errors.Is(err, k8sresourcessvc.ErrRenderedReleaseNotFound) {
		return gen.TriggerReleaseBindingCronJob404JSONResponse{NotFoundJSONResponse: notFound("RenderedRelease")}, nil
	}
	if errors.Is(err, k8sresourcessvc.ErrCronJobNotFound) {
		return gen.TriggerReleaseBindingCronJob404JSONResponse{NotFoundJSONResponse: notFound("CronJob")}, nil
	}
	if errors.Is(err, k8sresourcessvc.ErrEnvironmentNotFound) {
		return gen.TriggerReleaseBindingCronJob404JSONResponse{NotFoundJSONResponse: notFound("Environment")}, nil
	}
	h.logger.Error("Failed to trigger cronjob", "error", err)
	return gen.TriggerReleaseBindingCronJob500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
}

func (h *Handler) handleK8sResourceTreeError(err error) (gen.GetReleaseBindingK8sResourceTreeResponseObject, error) {
	if errors.Is(err, services.ErrForbidden) {
		return gen.GetReleaseBindingK8sResourceTree403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
	}
	if errors.Is(err, k8sresourcessvc.ErrReleaseBindingNotFound) {
		return gen.GetReleaseBindingK8sResourceTree404JSONResponse{NotFoundJSONResponse: notFound("ReleaseBinding")}, nil
	}
	if errors.Is(err, k8sresourcessvc.ErrRenderedReleaseNotFound) {
		return gen.GetReleaseBindingK8sResourceTree404JSONResponse{NotFoundJSONResponse: notFound("RenderedRelease")}, nil
	}
	if errors.Is(err, k8sresourcessvc.ErrEnvironmentNotFound) {
		return gen.GetReleaseBindingK8sResourceTree404JSONResponse{NotFoundJSONResponse: notFound("Environment")}, nil
	}
	h.logger.Error("Failed to get k8s resource tree", "error", err)
	return gen.GetReleaseBindingK8sResourceTree500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
}

func (h *Handler) handleK8sResourceEventsError(err error) (gen.GetReleaseBindingK8sResourceEventsResponseObject, error) {
	if errors.Is(err, services.ErrForbidden) {
		return gen.GetReleaseBindingK8sResourceEvents403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
	}
	if errors.Is(err, k8sresourcessvc.ErrReleaseBindingNotFound) {
		return gen.GetReleaseBindingK8sResourceEvents404JSONResponse{NotFoundJSONResponse: notFound("ReleaseBinding")}, nil
	}
	if errors.Is(err, k8sresourcessvc.ErrRenderedReleaseNotFound) {
		return gen.GetReleaseBindingK8sResourceEvents404JSONResponse{NotFoundJSONResponse: notFound("RenderedRelease")}, nil
	}
	if errors.Is(err, k8sresourcessvc.ErrEnvironmentNotFound) {
		return gen.GetReleaseBindingK8sResourceEvents404JSONResponse{NotFoundJSONResponse: notFound("Environment")}, nil
	}
	if errors.Is(err, k8sresourcessvc.ErrResourceNotFound) {
		return gen.GetReleaseBindingK8sResourceEvents404JSONResponse{NotFoundJSONResponse: notFound("Resource")}, nil
	}
	h.logger.Error("Failed to get k8s resource events", "error", err)
	return gen.GetReleaseBindingK8sResourceEvents500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
}

func (h *Handler) handleK8sResourceLogsError(err error) (gen.GetReleaseBindingK8sResourceLogsResponseObject, error) {
	if errors.Is(err, services.ErrForbidden) {
		return gen.GetReleaseBindingK8sResourceLogs403JSONResponse{ForbiddenJSONResponse: forbidden()}, nil
	}
	if errors.Is(err, k8sresourcessvc.ErrReleaseBindingNotFound) {
		return gen.GetReleaseBindingK8sResourceLogs404JSONResponse{NotFoundJSONResponse: notFound("ReleaseBinding")}, nil
	}
	if errors.Is(err, k8sresourcessvc.ErrRenderedReleaseNotFound) {
		return gen.GetReleaseBindingK8sResourceLogs404JSONResponse{NotFoundJSONResponse: notFound("RenderedRelease")}, nil
	}
	if errors.Is(err, k8sresourcessvc.ErrEnvironmentNotFound) {
		return gen.GetReleaseBindingK8sResourceLogs404JSONResponse{NotFoundJSONResponse: notFound("Environment")}, nil
	}
	if errors.Is(err, k8sresourcessvc.ErrResourceNotFound) {
		return gen.GetReleaseBindingK8sResourceLogs404JSONResponse{NotFoundJSONResponse: notFound("Resource")}, nil
	}
	if errors.Is(err, k8sresourcessvc.ErrInvalidContainer) {
		return gen.GetReleaseBindingK8sResourceLogs400JSONResponse{
			BadRequestJSONResponse: badRequest("Container not found in pod"),
		}, nil
	}
	h.logger.Error("Failed to get k8s resource logs", "error", err)
	return gen.GetReleaseBindingK8sResourceLogs500JSONResponse{InternalErrorJSONResponse: internalError()}, nil
}
