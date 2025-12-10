// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package releasebinding

import (
	"context"
	"encoding/json"
	"net/http"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	openchoreodevv1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

// nolint:unused
// log is for logging in this package.
var releasebindinglog = logf.Log.WithName("releasebinding-resource")

// SetupReleaseBindingWebhookWithManager registers the webhook for ReleaseBinding in the manager.
func SetupReleaseBindingWebhookWithManager(mgr ctrl.Manager) error {
	// Register mutating webhook manually because we implement admission.Handler
	// instead of webhook.CustomDefaulter. This is necessary to access both old
	// and new objects during updates, enabling preserve-when-empty logic for
	// releaseName (e.g., preserving auto-populated values when users update
	// other fields). The path must match the kubebuilder webhook marker below.
	mutatingWebhook := &webhook.Admission{
		Handler: &Defaulter{
			decoder: admission.NewDecoder(mgr.GetScheme()),
		},
	}
	mgr.GetWebhookServer().Register(
		"/mutate-openchoreo-dev-v1alpha1-releasebinding",
		mutatingWebhook,
	)

	// Register validating webhook
	return ctrl.NewWebhookManagedBy(mgr).
		For(&openchoreodevv1alpha1.ReleaseBinding{}).
		WithValidator(&Validator{Client: mgr.GetClient()}).
		Complete()
}

// +kubebuilder:webhook:path=/mutate-openchoreo-dev-v1alpha1-releasebinding,mutating=true,failurePolicy=fail,sideEffects=None,groups=openchoreo.dev,resources=releasebindings,verbs=create;update,versions=v1alpha1,name=mreleasebinding-v1alpha1.kb.io,admissionReviewVersions=v1

// Defaulter struct is responsible for setting default values on the custom resource of the
// Kind ReleaseBinding when those are created or updated.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as it is used only for temporary operations and does not need to be deeply copied.
type Defaulter struct {
	decoder admission.Decoder
}

var _ admission.Handler = &Defaulter{}

// Handle implements admission.Handler for custom defaulting logic with access to old object.
func (d *Defaulter) Handle(ctx context.Context, req admission.Request) admission.Response {
	releasebinding := &openchoreodevv1alpha1.ReleaseBinding{}

	err := d.decoder.Decode(req, releasebinding)
	if err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}

	// For updates, preserve releaseName from old object if not specified in new object
	if req.Operation == "UPDATE" && len(req.OldObject.Raw) > 0 {
		oldBinding := &openchoreodevv1alpha1.ReleaseBinding{}
		if err := d.decoder.DecodeRaw(req.OldObject, oldBinding); err != nil {
			return admission.Errored(http.StatusBadRequest, err)
		}

		// Preserve old releaseName if update is missing it
		// e.g: applying sample resources with auto deploy enabled, release auto created
		if releasebinding.Spec.ReleaseName == "" && oldBinding.Spec.ReleaseName != "" {
			releasebinding.Spec.ReleaseName = oldBinding.Spec.ReleaseName
		}
	}

	// Marshal the modified object
	marshaledBinding, err := json.Marshal(releasebinding)
	if err != nil {
		return admission.Errored(http.StatusInternalServerError, err)
	}

	return admission.PatchResponseFromRaw(req.Object.Raw, marshaledBinding)
}

// TODO(user): change verbs to "verbs=create;update;delete" if you want to enable deletion component.
// NOTE: The 'path' attribute must follow a specific pattern and should not be modified directly here.
// Modifying the path for an invalid path can cause API server errors; failing to locate the webhook.
// +kubebuilder:webhook:path=/validate-openchoreo-dev-v1alpha1-releasebinding,mutating=false,failurePolicy=fail,sideEffects=None,groups=openchoreo.dev,resources=releasebindings,verbs=create;update,versions=v1alpha1,name=vreleasebinding-v1alpha1.kb.io,admissionReviewVersions=v1

// Validator struct is responsible for validating the ReleaseBinding resource
// when it is created, updated, or deleted.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as this struct is used only for temporary operations and does not need to be deeply copied.
type Validator struct {
	Client client.Client
}

var _ webhook.CustomValidator = &Validator{}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type ReleaseBinding.
func (v *Validator) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	// Note: Required field validations (owner, environment) are enforced by the CRD schema
	// Note: spec.environment, spec.owner immutability is enforced by CEL rules in the CRD schema
	// Note: Cross-resource validation (ComponentRelease, schema validation) is handled by the controller

	return nil, nil
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type ReleaseBinding.
func (v *Validator) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	// Note: Required field validations (owner, environment) are enforced by the CRD schema
	// Note: spec.environment, spec.owner immutability is enforced by CEL rules in the CRD schema
	// Note: Cross-resource validation (ComponentRelease, schema validation) is handled by the controller

	return nil, nil
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type ReleaseBinding.
func (v *Validator) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	// No special validation needed for deletion
	return nil, nil
}
