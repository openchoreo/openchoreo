// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package endpointreleasebinding

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/controller"
	dpkubernetes "github.com/openchoreo/openchoreo/internal/dataplane/kubernetes"
	"github.com/openchoreo/openchoreo/internal/labels"
	endpointpipeline "github.com/openchoreo/openchoreo/internal/pipeline/endpoint"
)

const ownershipConflictMarker = "RenderedRelease exists but is not owned by this binding"

// renderAndEmit drives the endpoint pipeline against the release snapshot and
// writes the resulting RenderedRelease. Returns the live RenderedRelease on
// success (created, updated, or already in sync) or nil when no further
// evaluation should run (render error, ownership conflict). Transient API
// errors are returned to the caller for requeue.
func (r *Reconciler) renderAndEmit(
	ctx context.Context,
	binding *openchoreov1alpha1.EndpointReleaseBinding,
	input *endpointpipeline.RenderInput,
) (*openchoreov1alpha1.RenderedRelease, error) {
	logger := log.FromContext(ctx)

	output, err := r.Pipeline.Render(ctx, input)
	if err != nil {
		markSyncedFalse(binding, ReasonRenderingFailed, fmt.Sprintf("Failed to render manifests: %v", err))
		logger.Info("Pipeline render failed", "error", err)
		return nil, nil
	}

	manifests, err := convertEntriesToManifests(output.Entries)
	if err != nil {
		markSyncedFalse(binding, ReasonRenderingFailed, fmt.Sprintf("Failed to encode rendered manifests: %v", err))
		return nil, err
	}

	rrName := makeRenderedReleaseName(binding)
	rr := &openchoreov1alpha1.RenderedRelease{
		ObjectMeta: metav1.ObjectMeta{Name: rrName, Namespace: binding.Namespace},
	}

	op, err := controllerutil.CreateOrUpdate(ctx, r.Client, rr, func() error {
		if rr.UID != "" {
			hasOwner, ownerErr := controllerutil.HasOwnerReference(rr.GetOwnerReferences(), binding, r.Scheme)
			if ownerErr != nil {
				return fmt.Errorf("check owner reference: %w", ownerErr)
			}
			if !hasOwner {
				return fmt.Errorf("%s: %q", ownershipConflictMarker, rrName)
			}
		}

		rr.Labels = map[string]string{
			labels.LabelKeyNamespaceName:   binding.Namespace,
			labels.LabelKeyProjectName:     binding.Spec.Owner.ProjectName,
			labels.LabelKeyEndpointCRName:  binding.Spec.Owner.EndpointName,
			labels.LabelKeyEnvironmentName: binding.Spec.Environment,
		}
		rr.Spec = openchoreov1alpha1.RenderedReleaseSpec{
			Owner: openchoreov1alpha1.RenderedReleaseOwner{
				ProjectName:  binding.Spec.Owner.ProjectName,
				EndpointName: binding.Spec.Owner.EndpointName,
			},
			EnvironmentName: binding.Spec.Environment,
			TargetPlane:     openchoreov1alpha1.TargetPlaneDataPlane,
			Resources:       manifests,
		}
		return controllerutil.SetControllerReference(binding, rr, r.Scheme)
	})
	if err != nil {
		if strings.Contains(err.Error(), ownershipConflictMarker) {
			markSyncedFalse(binding, ReasonReleaseOwnershipConflict, err.Error())
			return nil, nil
		}
		markSyncedFalse(binding, ReasonReleaseUpdateFailed,
			fmt.Sprintf("Failed to reconcile RenderedRelease %q: %v", rrName, err))
		return nil, err
	}

	switch op {
	case controllerutil.OperationResultCreated, controllerutil.OperationResultUpdated:
		controller.MarkTrueCondition(binding, ConditionSynced, ReasonReleaseCreated,
			fmt.Sprintf("RenderedRelease %q %s with %d resource(s)", rrName, op, len(manifests)))
	case controllerutil.OperationResultNone:
		controller.MarkTrueCondition(binding, ConditionSynced, ReasonReleaseSynced,
			fmt.Sprintf("RenderedRelease %q is up to date", rrName))
	}

	return rr, nil
}

// buildRenderInput assembles the pipeline input shared by rendering and URL
// resolution, so both see exactly the same CEL context.
func buildRenderInput(
	binding *openchoreov1alpha1.EndpointReleaseBinding,
	in *reconcileInputs,
	md endpointpipeline.MetadataContext,
	backends []endpointpipeline.Backend,
) *endpointpipeline.RenderInput {
	return &endpointpipeline.RenderInput{
		EndpointTypeSpec:   &in.release.Spec.EndpointType.Spec,
		Parameters:         in.release.Spec.Parameters,
		EnvironmentConfigs: binding.Spec.EndpointTypeEnvironmentConfigs,
		Visibility:         in.release.Spec.Visibility,
		Backends:           backends,
		Metadata:           md,
		DataPlane:          endpointpipeline.BuildDataPlaneContext(in.dataPlane),
		Environment:        endpointpipeline.BuildEnvironmentContext(in.environment, in.dataPlane),
	}
}

// resolveURLs evaluates the EndpointType's URLs against input and writes them
// to status.urls with the URLsResolved condition. It runs only after a
// successful render, so status.urls always describes the RenderedRelease that
// was just emitted. A failing URL is reported but does not affect the
// RenderedRelease: routing keeps working while the type is fixed.
func (r *Reconciler) resolveURLs(
	ctx context.Context,
	binding *openchoreov1alpha1.EndpointReleaseBinding,
	input *endpointpipeline.RenderInput,
) {
	urls, err := r.Pipeline.ResolveURLs(ctx, input)
	binding.Status.URLs = urls

	if err != nil {
		controller.MarkFalseCondition(binding, ConditionURLsResolved, ReasonURLResolutionFailed,
			fmt.Sprintf("Failed to resolve URLs: %v", err))
		log.FromContext(ctx).Info("URL resolution failed", "error", err)
		return
	}
	controller.MarkTrueCondition(binding, ConditionURLsResolved, ReasonURLsResolved, "URLs resolved")
}

// makeRenderedReleaseName returns the RenderedRelease name for a binding. The
// "e_" discriminator keeps it collision-safe against component (""), resource
// ("r_") and project ("p_") RenderedReleases: the underscore is sanitized to
// "-" in the visible name but drives the hash, and K8s names cannot contain
// "_", so a Component named "e-{endpoint}" hashes a different input.
func makeRenderedReleaseName(binding *openchoreov1alpha1.EndpointReleaseBinding) string {
	return dpkubernetes.GenerateK8sName("e_"+binding.Spec.Owner.EndpointName, binding.Spec.Environment)
}

// convertEntriesToManifests marshals each pipeline-rendered map into a
// RenderedManifest with its ID preserved verbatim.
func convertEntriesToManifests(entries []endpointpipeline.RenderedEntry) ([]openchoreov1alpha1.RenderedManifest, error) {
	manifests := make([]openchoreov1alpha1.RenderedManifest, 0, len(entries))
	for i := range entries {
		raw, err := json.Marshal(entries[i].Object)
		if err != nil {
			return nil, fmt.Errorf("marshal rendered manifest %q: %w", entries[i].ID, err)
		}
		manifests = append(manifests, openchoreov1alpha1.RenderedManifest{
			ID:     entries[i].ID,
			Object: &runtime.RawExtension{Raw: raw},
		})
	}
	return manifests, nil
}

// dataPlaneNamespace returns the dp-{ns}-{project}-{env}-{hash} namespace the
// project's components render into for this environment. Routes render there
// too, so they reference backend Services in their own namespace.
func dataPlaneNamespace(binding *openchoreov1alpha1.EndpointReleaseBinding) string {
	return dpkubernetes.GenerateK8sNameWithLengthLimit(
		dpkubernetes.MaxNamespaceNameLength,
		"dp", binding.Namespace, binding.Spec.Owner.ProjectName, binding.Spec.Environment,
	)
}

// buildMetadataContext computes the platform-injected metadata surface exposed
// to CEL templates.
func buildMetadataContext(binding *openchoreov1alpha1.EndpointReleaseBinding, in *reconcileInputs) endpointpipeline.MetadataContext {
	endpointName := binding.Spec.Owner.EndpointName
	projectName := binding.Spec.Owner.ProjectName
	envName := binding.Spec.Environment

	return endpointpipeline.MetadataContext{
		Name:              makeRenderedReleaseName(binding),
		Namespace:         dataPlaneNamespace(binding),
		EndpointNamespace: binding.Namespace,
		EndpointName:      endpointName,
		EndpointUID:       string(in.endpoint.UID),
		ProjectName:       projectName,
		ProjectUID:        string(in.project.UID),
		EnvironmentName:   envName,
		EnvironmentUID:    string(in.environment.UID),
		DataPlaneName:     in.dataPlane.Name,
		DataPlaneUID:      string(in.dataPlane.UID),
		Labels: map[string]string{
			labels.LabelKeyNamespaceName:   binding.Namespace,
			labels.LabelKeyProjectName:     projectName,
			labels.LabelKeyProjectUID:      string(in.project.UID),
			labels.LabelKeyEndpointCRName:  endpointName,
			labels.LabelKeyEndpointCRUID:   string(in.endpoint.UID),
			labels.LabelKeyEnvironmentName: envName,
			labels.LabelKeyEnvironmentUID:  string(in.environment.UID),
		},
		Annotations: binding.Annotations,
	}
}
