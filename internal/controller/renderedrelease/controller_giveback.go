// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package renderedrelease

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/openchoreo/openchoreo/internal/labels"
)

// takeoverFieldManager is the field manager of the claimed label. It is not ControllerName,
// because an apply drops what its manager applied before and no longer does, and the
// release's manifests don't contain the label.
const takeoverFieldManager = "renderedrelease-takeover"

// trackingLabels are the labels a release puts on every object it applies.
var trackingLabels = []string{
	labels.LabelKeyManagedBy,
	labels.LabelKeyRenderedReleaseResourceID,
	labels.LabelKeyRenderedReleaseUID,
	labels.LabelKeyRenderedReleaseName,
	labels.LabelKeyRenderedReleaseNamespace,
}

// hasClaimedLabel reports whether an object carries the claimed label, whatever its value.
func hasClaimedLabel(obj *unstructured.Unstructured) bool {
	_, found := obj.GetLabels()[labels.LabelKeyClaimed]
	return found
}

// addClaimedLabel adds the claimed label to a live object, and returns the object as it is
// then. The request names the object by UID: without it, the apply would create an object
// with nothing but the label in place of one that has just been deleted.
func addClaimedLabel(ctx context.Context, planeClient client.Client, live *unstructured.Unstructured) (*unstructured.Unstructured, error) {
	labeled := sameObject(live)
	labeled.SetUID(live.GetUID())
	labeled.SetLabels(map[string]string{labels.LabelKeyClaimed: "true"})
	if err := planeClient.Patch(ctx, labeled, client.Apply, client.FieldOwner(takeoverFieldManager)); err != nil {
		return nil, fmt.Errorf("failed to label %s as claimed: %w", describeObject(live), err)
	}
	return labeled, nil
}

// giveBack erases the marks of a release from a live object: its tracking labels, the
// claimed label and its field manager's entry. Nothing else changes, not the spec and not
// the generation, so nothing rolls out. What the release set stays, without an owner.
//
// An apply without the fields can't do this: it removes what only the release set, and is
// rejected where that is required.
func giveBack(ctx context.Context, planeClient client.Client, live *unstructured.Unstructured) error {
	// The server ignores the removal of the only managedFields entry. The label has an entry
	// of its own, which goes with it, so there is another one to leave until then.
	if !hasClaimedLabel(live) {
		labeled, err := addClaimedLabel(ctx, planeClient, live)
		if err != nil {
			return err
		}
		live = labeled
	}
	patch, err := giveBackPatch(live)
	if err != nil {
		return fmt.Errorf("failed to give back %s: %w", describeObject(live), err)
	}
	err = planeClient.Patch(ctx, sameObject(live), client.RawPatch(types.JSONPatchType, patch))
	if client.IgnoreNotFound(err) != nil {
		return fmt.Errorf("failed to give back %s: %w", describeObject(live), err)
	}
	return nil
}

// giveBackPatch returns the JSON patch that gives a live object back. It tests that the
// object is the one that was read, and that each mark is still there as read, so a mark
// that changed in between is never erased and the next try reads it again. It doesn't test
// the resourceVersion, which an object whose status changes all the time never keeps.
func giveBackPatch(live *unstructured.Unstructured) ([]byte, error) {
	ops := []map[string]any{{"op": "test", "path": "/metadata/uid", "value": live.GetUID()}}
	remove := func(path string, value any) {
		ops = append(ops, map[string]any{"op": "test", "path": path, "value": value}, map[string]any{"op": "remove", "path": path})
	}

	// The raw entries are tested, as read. From the last one, so that removing one does not
	// move the others.
	raw, _, err := unstructured.NestedSlice(live.Object, "metadata", "managedFields")
	if err != nil {
		return nil, err
	}
	for i, entry := range slices.Backward(live.GetManagedFields()) {
		if entry.Manager == ControllerName && entry.Operation == metav1.ManagedFieldsOperationApply {
			remove(fmt.Sprintf("/metadata/managedFields/%d", i), raw[i])
		}
	}
	escape := strings.NewReplacer("~", "~0", "/", "~1")
	for _, key := range append(slices.Clone(trackingLabels), labels.LabelKeyClaimed) {
		if value, found := live.GetLabels()[key]; found {
			remove("/metadata/labels/"+escape.Replace(key), value)
		}
	}
	return json.Marshal(ops)
}

// sameObject returns an object that only names the given one.
func sameObject(obj *unstructured.Unstructured) *unstructured.Unstructured {
	same := &unstructured.Unstructured{}
	same.SetGroupVersionKind(obj.GroupVersionKind())
	same.SetNamespace(obj.GetNamespace())
	same.SetName(obj.GetName())
	return same
}
