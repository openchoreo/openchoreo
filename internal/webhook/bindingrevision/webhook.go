// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

// Package bindingrevision validates updates to ReleaseBindingRevisions and
// ResourceReleaseBindingRevisions: a revision records one binding spec, and
// that record must not change after it is created.
//
// The CRDs carry a `self == oldSelf` CEL rule on spec, but CEL cannot compare
// the free-form objects in a binding spec in full: they are declared
// `type: object` with preserved unknown fields, and CEL equality on them misses
// a key replaced by another key, for example {"replicas":2} becoming
// {"count":2}. This webhook compares the whole spec.
//
// It compares the specs as the apiserver admitted them, not as Go types:
// decoding into the typed revision would drop the unknown fields the CRD
// preserves (for example under bindingSpec.workloadOverrides), and a change to
// them would go unseen.
package bindingrevision

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"

	admissionv1 "k8s.io/api/admission/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

// ImmutableSpecMessage is the message an update that changes a revision's
// spec is rejected with. It matches the CRDs' CEL rule.
const ImmutableSpecMessage = "revision spec is immutable; only status may change"

// Paths the webhooks are served at. They match the kubebuilder markers below,
// from which config/webhook/manifests.yaml is generated.
const (
	releaseBindingRevisionPath         = "/validate-openchoreo-dev-v1alpha1-releasebindingrevision"
	resourceReleaseBindingRevisionPath = "/validate-openchoreo-dev-v1alpha1-resourcereleasebindingrevision"
)

// Only updates can change a spec. Status is written through the status
// subresource, which these rules do not match.

// +kubebuilder:webhook:path=/validate-openchoreo-dev-v1alpha1-releasebindingrevision,mutating=false,failurePolicy=fail,sideEffects=None,groups=openchoreo.dev,resources=releasebindingrevisions,verbs=update,versions=v1alpha1,name=vreleasebindingrevision-v1alpha1.kb.io,admissionReviewVersions=v1

// SetupReleaseBindingRevisionWebhookWithManager registers the validating
// webhook for ReleaseBindingRevision in the manager.
func SetupReleaseBindingRevisionWebhookWithManager(mgr ctrl.Manager) error {
	mgr.GetWebhookServer().Register(releaseBindingRevisionPath, &webhook.Admission{
		Handler: NewSpecGuard(openchoreov1alpha1.GroupVersion.WithKind("ReleaseBindingRevision").GroupKind()),
	})
	return nil
}

// +kubebuilder:webhook:path=/validate-openchoreo-dev-v1alpha1-resourcereleasebindingrevision,mutating=false,failurePolicy=fail,sideEffects=None,groups=openchoreo.dev,resources=resourcereleasebindingrevisions,verbs=update,versions=v1alpha1,name=vresourcereleasebindingrevision-v1alpha1.kb.io,admissionReviewVersions=v1

// SetupResourceReleaseBindingRevisionWebhookWithManager registers the
// validating webhook for ResourceReleaseBindingRevision in the manager.
func SetupResourceReleaseBindingRevisionWebhookWithManager(mgr ctrl.Manager) error {
	mgr.GetWebhookServer().Register(resourceReleaseBindingRevisionPath, &webhook.Admission{
		Handler: NewSpecGuard(openchoreov1alpha1.GroupVersion.WithKind("ResourceReleaseBindingRevision").GroupKind()),
	})
	return nil
}

// SpecGuard rejects an update that changes a revision's spec.
type SpecGuard struct {
	kind schema.GroupKind
}

var _ admission.Handler = &SpecGuard{}

// NewSpecGuard returns a SpecGuard for revisions of the given kind.
func NewSpecGuard(kind schema.GroupKind) *SpecGuard {
	return &SpecGuard{kind: kind}
}

// Handle allows every request except an update whose spec differs from the
// stored one.
func (g *SpecGuard) Handle(_ context.Context, req admission.Request) admission.Response {
	if req.Operation != admissionv1.Update {
		return admission.Allowed("")
	}
	same, err := sameSpec(req.OldObject.Raw, req.Object.Raw)
	if err != nil {
		return admission.Errored(http.StatusBadRequest, fmt.Errorf("compare revision specs: %w", err))
	}
	if same {
		return admission.Allowed("")
	}
	invalid := apierrors.NewInvalid(g.kind, req.Name, field.ErrorList{
		field.Forbidden(field.NewPath("spec"), ImmutableSpecMessage),
	})
	return admission.Response{AdmissionResponse: admissionv1.AdmissionResponse{
		Allowed: false,
		Result:  &invalid.ErrStatus,
	}}
}

// sameSpec reports whether two serialized objects carry the same spec. Both
// are decoded into generic values, keeping every field and the exact spelling
// of every number, so key order and formatting do not matter, while any key
// added, removed or renamed at any depth does.
func sameSpec(oldRaw, newRaw []byte) (bool, error) {
	oldSpec, err := specOf(oldRaw)
	if err != nil {
		return false, fmt.Errorf("decode old object: %w", err)
	}
	newSpec, err := specOf(newRaw)
	if err != nil {
		return false, fmt.Errorf("decode new object: %w", err)
	}
	return reflect.DeepEqual(oldSpec, newSpec), nil
}

func specOf(raw []byte) (any, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var obj struct {
		Spec any `json:"spec"`
	}
	if err := dec.Decode(&obj); err != nil {
		return nil, err
	}
	return obj.Spec, nil
}
