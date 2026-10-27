// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package releasebinding

import (
	"context"
	"fmt"
	"sort"
	"strings"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/controller"
	"github.com/openchoreo/openchoreo/internal/labels"
)

// validateReleaseLine checks the binding's release line. A binding without a line belongs to its
// component's implicit default line; one with a line requires an existing ReleaseLine of the component.
// Returns the binding's ReleaseLine, nil for the default line, and false with a condition set when the
// binding must not be rendered.
func (r *Reconciler) validateReleaseLine(
	ctx context.Context,
	releaseBinding *openchoreov1alpha1.ReleaseBinding,
) (*openchoreov1alpha1.ReleaseLine, bool, error) {
	lineName := releaseBinding.Spec.Owner.ReleaseLine
	if lineName == "" {
		return nil, true, nil
	}

	line := &openchoreov1alpha1.ReleaseLine{}
	if err := r.Get(ctx, types.NamespacedName{Name: lineName, Namespace: releaseBinding.Namespace}, line); err != nil {
		if apierrors.IsNotFound(err) {
			controller.MarkFalseCondition(releaseBinding, ConditionReleaseSynced, ReasonReleaseLineNotFound,
				fmt.Sprintf("ReleaseLine %q not found", lineName))
			return nil, false, nil
		}
		log.FromContext(ctx).Error(err, "Failed to get ReleaseLine", "releaseLine", lineName)
		return nil, false, err
	}

	if line.Spec.Owner.ProjectName != releaseBinding.Spec.Owner.ProjectName ||
		line.Spec.Owner.ComponentName != releaseBinding.Spec.Owner.ComponentName {
		controller.MarkFalseCondition(releaseBinding, ConditionReleaseSynced, ReasonInvalidReleaseConfiguration,
			fmt.Sprintf("ReleaseLine %q belongs to component %s/%s, not %s/%s", lineName,
				line.Spec.Owner.ProjectName, line.Spec.Owner.ComponentName,
				releaseBinding.Spec.Owner.ProjectName, releaseBinding.Spec.Owner.ComponentName))
		return nil, false, nil
	}
	return line, true, nil
}

// findReleaseBindingsForReleaseLine maps a ReleaseLine to the bindings on that line.
func (r *Reconciler) findReleaseBindingsForReleaseLine(ctx context.Context, obj client.Object) []ctrl.Request {
	line := obj.(*openchoreov1alpha1.ReleaseLine)

	var bindings openchoreov1alpha1.ReleaseBindingList
	if err := r.List(ctx, &bindings,
		client.InNamespace(line.Namespace),
		client.MatchingFields{controller.IndexKeyReleaseBindingOwnerComponentName: line.Spec.Owner.ComponentName}); err != nil {
		return nil
	}

	var requests []ctrl.Request
	for _, b := range controller.FilterReleaseBindingsByLine(bindings.Items, line.Name) {
		requests = append(requests, ctrl.Request{NamespacedName: types.NamespacedName{Name: b.Name, Namespace: b.Namespace}})
	}
	return requests
}

// findReleaseLineCollision reports objects the binding would render on the target plane that another
// RenderedRelease of the same component and environment, including the default line's, already renders
// there. Returns an empty string when there is no collision.
func (r *Reconciler) findReleaseLineCollision(
	ctx context.Context,
	releaseBinding *openchoreov1alpha1.ReleaseBinding,
	ownReleaseName string,
	plane string,
	resources []openchoreov1alpha1.RenderedManifest,
) (string, error) {
	if len(resources) == 0 {
		return "", nil
	}
	desired, err := manifestKeys(resources)
	if err != nil {
		return "", err
	}

	var releases openchoreov1alpha1.RenderedReleaseList
	if err := r.List(ctx, &releases, client.InNamespace(releaseBinding.Namespace), client.MatchingLabels{
		labels.LabelKeyProjectName:     releaseBinding.Spec.Owner.ProjectName,
		labels.LabelKeyComponentName:   releaseBinding.Spec.Owner.ComponentName,
		labels.LabelKeyEnvironmentName: releaseBinding.Spec.Environment,
	}); err != nil {
		return "", fmt.Errorf("failed to list RenderedReleases: %w", err)
	}

	for i := range releases.Items {
		other := &releases.Items[i]
		if other.Name == ownReleaseName || other.Spec.TargetPlane != plane {
			continue
		}
		existing, err := manifestKeys(other.Spec.Resources)
		if err != nil {
			return "", err
		}
		var clashes []string
		for key := range desired {
			if existing[key] {
				clashes = append(clashes, key)
			}
		}
		if len(clashes) > 0 {
			sort.Strings(clashes)
			return fmt.Sprintf("Release line %q renders %s, which RenderedRelease %q also renders; "+
				"name these objects from metadata.name or metadata.componentLineName in the ComponentType and Traits",
				releaseBinding.Spec.Owner.ReleaseLine, strings.Join(clashes, ", "), other.Name), nil
		}
	}
	return "", nil
}

// manifestKeys returns the "Kind.group namespace/name" identity of each manifest.
func manifestKeys(resources []openchoreov1alpha1.RenderedManifest) (map[string]bool, error) {
	keys := make(map[string]bool, len(resources))
	for _, res := range resources {
		if res.Object == nil {
			continue
		}
		obj := &unstructured.Unstructured{}
		if err := obj.UnmarshalJSON(res.Object.Raw); err != nil {
			return nil, fmt.Errorf("failed to decode rendered resource %q: %w", res.ID, err)
		}
		kind := obj.GetKind()
		if group := obj.GroupVersionKind().Group; group != "" {
			kind += "." + group
		}
		keys[fmt.Sprintf("%s %s/%s", kind, obj.GetNamespace(), obj.GetName())] = true
	}
	return keys, nil
}

const kindPod = "Pod"

// findDefaultLinePodLabel reports a release line object that would share pods with the default line:
// a pod template carrying the component UID label, which the default line's selectors match, or a
// selector matching on that label, which the default line's pods carry. Returns an empty string when
// there is none.
func findDefaultLinePodLabel(line string, resources []openchoreov1alpha1.RenderedManifest) (string, error) {
	for _, res := range resources {
		if res.Object == nil {
			continue
		}
		obj := &unstructured.Unstructured{}
		if err := obj.UnmarshalJSON(res.Object.Raw); err != nil {
			return "", fmt.Errorf("failed to decode rendered resource %q: %w", res.ID, err)
		}
		var podLabels map[string]string
		switch obj.GetKind() {
		case kindPod:
			podLabels = obj.GetLabels()
		case "CronJob":
			podLabels, _, _ = unstructured.NestedStringMap(obj.Object, "spec", "jobTemplate", "spec", "template", "metadata", "labels")
		default:
			podLabels, _, _ = unstructured.NestedStringMap(obj.Object, "spec", "template", "metadata", "labels")
		}
		if _, ok := podLabels[labels.LabelKeyComponentUID]; ok {
			return fmt.Sprintf("Release line %q renders %s %q with pod label %s, which the default line's selectors also match; "+
				"label pods with metadata.podSelectors in the ComponentType and Traits",
				line, obj.GetKind(), obj.GetName(), labels.LabelKeyComponentUID), nil
		}
		if selectsComponentUID(obj) {
			return fmt.Sprintf("Release line %q renders %s %q selecting on %s, which the default line's pods carry; "+
				"select pods with metadata.podSelectors in the ComponentType and Traits",
				line, obj.GetKind(), obj.GetName(), labels.LabelKeyComponentUID), nil
		}
	}
	return "", nil
}

// selectsComponentUID reports whether an object's own selector, spec.selector or spec.podSelector,
// matches pods on the component UID label. Selectors of other pods, such as network policy peers,
// are not the object's own and are not checked.
func selectsComponentUID(obj *unstructured.Unstructured) bool {
	for _, field := range []string{"selector", "podSelector"} {
		selector, _, _ := unstructured.NestedMap(obj.Object, "spec", field)
		if _, ok := selector[labels.LabelKeyComponentUID]; ok {
			return true
		}
		matchLabels, _, _ := unstructured.NestedMap(selector, "matchLabels")
		if _, ok := matchLabels[labels.LabelKeyComponentUID]; ok {
			return true
		}
		expressions, _, _ := unstructured.NestedSlice(selector, "matchExpressions")
		for _, e := range expressions {
			expr, _ := e.(map[string]any)
			if expr["key"] == labels.LabelKeyComponentUID && (expr["operator"] == "In" || expr["operator"] == "Exists") {
				return true
			}
		}
	}
	return false
}
