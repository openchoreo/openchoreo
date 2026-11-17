// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package workload

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"

	admissionv1 "k8s.io/api/admission/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	openchoreodevv1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

// SetupWorkloadWebhookWithManager registers the webhooks for Workload in the manager.
func SetupWorkloadWebhookWithManager(mgr ctrl.Manager) error {
	// The mutating webhook is an admission.Handler rather than a
	// webhook.CustomDefaulter: it needs the old object to tell which of
	// protocol and type an update changed, and it returns deprecation warnings,
	// which a CustomDefaulter cannot. The path must match the marker below.
	mgr.GetWebhookServer().Register(
		"/mutate-openchoreo-dev-v1alpha1-workload",
		&webhook.Admission{Handler: &Defaulter{decoder: admission.NewDecoder(mgr.GetScheme())}},
	)

	return ctrl.NewWebhookManagedBy(mgr, &openchoreodevv1alpha1.Workload{}).
		WithCustomValidator(&Validator{}).
		Complete()
}

// +kubebuilder:webhook:path=/mutate-openchoreo-dev-v1alpha1-workload,mutating=true,failurePolicy=fail,sideEffects=None,groups=openchoreo.dev,resources=workloads,verbs=create;update,versions=v1alpha1,name=mworkload-v1alpha1.kb.io,admissionReviewVersions=v1

// Defaulter keeps each workload endpoint's protocol and deprecated type equal,
// so readers of either field see the same value, and warns about deprecated
// fields the request uses.
type Defaulter struct {
	decoder admission.Decoder
}

var _ admission.Handler = &Defaulter{}

// Handle implements admission.Handler.
func (d *Defaulter) Handle(_ context.Context, req admission.Request) admission.Response {
	workload := &openchoreodevv1alpha1.Workload{}
	if err := d.decoder.Decode(req, workload); err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}

	var old *openchoreodevv1alpha1.Workload
	if req.Operation == admissionv1.Update && len(req.OldObject.Raw) > 0 {
		old = &openchoreodevv1alpha1.Workload{}
		if err := d.decoder.DecodeRaw(req.OldObject, old); err != nil {
			return admission.Errored(http.StatusBadRequest, err)
		}
	}

	warnings := syncEndpointProtocols(workload, old)

	marshaled, err := json.Marshal(workload)
	if err != nil {
		return admission.Errored(http.StatusInternalServerError, err)
	}
	return admission.PatchResponseFromRaw(req.Object.Raw, marshaled).WithWarnings(warnings...)
}

// syncEndpointProtocols copies each workload endpoint's protocol to its
// deprecated type or the other way round, and returns a warning for every
// deprecated field the request sets.
//
// On update, a field the request changed wins over one it left as it was, so
// editing either field alone never trips the mismatch check. When both
// changed to different values, nothing is copied and validation rejects the
// request.
func syncEndpointProtocols(workload, old *openchoreodevv1alpha1.Workload) []string {
	names := make([]string, 0, len(workload.Spec.Endpoints))
	for name := range workload.Spec.Endpoints {
		names = append(names, name)
	}
	sort.Strings(names)

	var warnings []string
	for _, name := range names {
		ep := workload.Spec.Endpoints[name]
		path := field.NewPath("spec", "endpoints").Key(name)
		//nolint:staticcheck // deprecated fields are synced and reported here
		typ, visibility := ep.Type, ep.Visibility

		if ep.Protocol == "" && typ != "" {
			warnings = append(warnings, fmt.Sprintf(
				"%s is deprecated; use %s", path.Child("type"), path.Child("protocol")))
		}
		if len(visibility) > 0 {
			warnings = append(warnings, fmt.Sprintf(
				"%s is deprecated; expose the endpoint through an Endpoint, whose spec.visibility carries it",
				path.Child("visibility")))
		}

		var oldEP *openchoreodevv1alpha1.WorkloadEndpoint
		if old != nil {
			if prev, ok := old.Spec.Endpoints[name]; ok {
				oldEP = &prev
			}
		}
		ep.Protocol, ep.Type = syncedProtocol(ep.Protocol, typ, oldEP) //nolint:staticcheck // see above
		workload.Spec.Endpoints[name] = ep
	}
	return warnings
}

// syncedProtocol returns the protocol and type to store for one endpoint.
func syncedProtocol(
	protocol, typ openchoreodevv1alpha1.EndpointProtocol,
	old *openchoreodevv1alpha1.WorkloadEndpoint,
) (openchoreodevv1alpha1.EndpointProtocol, openchoreodevv1alpha1.EndpointProtocol) {
	switch {
	case protocol == typ:
		return protocol, typ
	case protocol == "":
		return typ, typ
	case typ == "":
		return protocol, protocol
	}
	if old != nil {
		//nolint:staticcheck // the deprecated field is compared to detect what changed
		protocolChanged, typeChanged := protocol != old.Protocol, typ != old.Type
		switch {
		case protocolChanged && !typeChanged:
			return protocol, protocol
		case typeChanged && !protocolChanged:
			return typ, typ
		}
	}
	return protocol, typ
}

// +kubebuilder:webhook:path=/validate-openchoreo-dev-v1alpha1-workload,mutating=false,failurePolicy=fail,sideEffects=None,groups=openchoreo.dev,resources=workloads,verbs=create;update,versions=v1alpha1,name=vworkload-v1alpha1.kb.io,admissionReviewVersions=v1

// Validator rejects workload endpoints whose protocol is missing or whose
// protocol and deprecated type disagree. It runs after the Defaulter, so a
// mismatch here means a request changed both fields to different values.
type Validator struct{}

var _ webhook.CustomValidator = &Validator{}

// ValidateCreate implements webhook.CustomValidator.
func (v *Validator) ValidateCreate(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	return nil, validate(obj)
}

// ValidateUpdate implements webhook.CustomValidator.
func (v *Validator) ValidateUpdate(_ context.Context, _, newObj runtime.Object) (admission.Warnings, error) {
	return nil, validate(newObj)
}

// ValidateDelete implements webhook.CustomValidator.
func (v *Validator) ValidateDelete(_ context.Context, _ runtime.Object) (admission.Warnings, error) {
	return nil, nil
}

func validate(obj runtime.Object) error {
	workload, ok := obj.(*openchoreodevv1alpha1.Workload)
	if !ok {
		return fmt.Errorf("expected a Workload object but got %T", obj)
	}
	allErrs := ValidateEndpointProtocols(workload.Spec.Endpoints, field.NewPath("spec", "endpoints"))
	if len(allErrs) == 0 {
		return nil
	}
	return apierrors.NewInvalid(
		schema.GroupKind{Group: openchoreodevv1alpha1.GroupVersion.Group, Kind: "Workload"},
		workload.Name, allErrs)
}

// ValidateEndpointProtocols checks that every workload endpoint has a protocol,
// through protocol or the deprecated type, and that the two agree when both
// are set.
func ValidateEndpointProtocols(
	endpoints map[string]openchoreodevv1alpha1.WorkloadEndpoint, path *field.Path,
) field.ErrorList {
	names := make([]string, 0, len(endpoints))
	for name := range endpoints {
		names = append(names, name)
	}
	sort.Strings(names)

	allErrs := field.ErrorList{}
	for _, name := range names {
		ep := endpoints[name]
		epPath := path.Key(name)
		typ := ep.Type //nolint:staticcheck // the deprecated field is validated against protocol
		switch {
		case ep.Protocol == "" && typ == "":
			allErrs = append(allErrs, field.Required(epPath.Child("protocol"), "protocol is required"))
		case ep.Protocol != "" && typ != "" && ep.Protocol != typ:
			allErrs = append(allErrs, field.Invalid(epPath.Child("type"), typ, fmt.Sprintf(
				"deprecated type must equal protocol %q; set only protocol", ep.Protocol)))
		}
	}
	return allErrs
}
