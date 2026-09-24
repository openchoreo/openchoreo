// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package environment

import (
	"context"
	"fmt"
	"sort"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	openchoreodevv1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/controller"
	"github.com/openchoreo/openchoreo/internal/validation/hooks"
)

// nolint:unused
// log is for logging in this package.
var environmentlog = logf.Log.WithName("environment-resource")

// SetupEnvironmentWebhookWithManager registers the webhook for Environment in the manager.
func SetupEnvironmentWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr, &openchoreodevv1alpha1.Environment{}).
		WithCustomValidator(&Validator{Client: mgr.GetClient()}).
		Complete()
}

// +kubebuilder:webhook:path=/validate-openchoreo-dev-v1alpha1-environment,mutating=false,failurePolicy=fail,sideEffects=None,groups=openchoreo.dev,resources=environments,verbs=create;update,versions=v1alpha1,name=venvironment-v1alpha1.kb.io,admissionReviewVersions=v1

// Validator validates Environment resources
// +kubebuilder:object:generate=false
type Validator struct {
	Client client.Client
}

var _ webhook.CustomValidator = &Validator{}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type Environment.
func (v *Validator) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	env, ok := obj.(*openchoreodevv1alpha1.Environment)
	if !ok {
		return nil, fmt.Errorf("expected an Environment object but got %T", obj)
	}
	environmentlog.Info("Validation for Environment upon creation", "name", env.GetName())

	return v.validate(ctx, env)
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type Environment.
func (v *Validator) ValidateUpdate(ctx context.Context, _, newObj runtime.Object) (admission.Warnings, error) {
	env, ok := newObj.(*openchoreodevv1alpha1.Environment)
	if !ok {
		return nil, fmt.Errorf("expected an Environment object for the newObj but got %T", newObj)
	}
	environmentlog.Info("Validation for Environment upon update", "name", env.GetName())

	return v.validate(ctx, env)
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type Environment.
func (v *Validator) ValidateDelete(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	env, ok := obj.(*openchoreodevv1alpha1.Environment)
	if !ok {
		return nil, fmt.Errorf("expected an Environment object but got %T", obj)
	}
	environmentlog.Info("Validation for Environment upon deletion", "name", env.GetName())

	// No special validation needed for deletion
	return nil, nil
}

func (v *Validator) validate(ctx context.Context, env *openchoreodevv1alpha1.Environment) (admission.Warnings, error) {
	warnings, allErrs := ValidateEnvironmentHooks(ctx, v.Client, env)
	if len(allErrs) > 0 {
		return warnings, apierrors.NewInvalid(env.GroupVersionKind().GroupKind(), env.GetName(), allErrs)
	}
	return warnings, nil
}

// ValidateEnvironmentHooks validates the environment's hook set and warns when
// a bound Workflow hook has no workflow plane to run on.
// An environment without hooks produces no errors and no warnings.
func ValidateEnvironmentHooks(
	ctx context.Context,
	c client.Client,
	env *openchoreodevv1alpha1.Environment,
) (admission.Warnings, field.ErrorList) {
	if env.Spec.Hooks == nil {
		return nil, nil
	}
	resolver := newResolver(ctx, c, env.Namespace)

	allErrs, warns := hooks.ValidateHookSet(env.Spec.Hooks, field.NewPath("spec", "hooks"), resolver.resolve)
	var warnings admission.Warnings
	for _, w := range warns {
		warnings = append(warnings, w)
	}
	warnings = append(warnings, resolver.planeWarnings()...)
	return warnings, allErrs
}

func normalizeRef(ref openchoreodevv1alpha1.HookRef) openchoreodevv1alpha1.HookRef {
	if ref.Kind == "" {
		ref.Kind = openchoreodevv1alpha1.HookRefKindHook
	}
	return ref
}

// resolver fetches hooks once per ref and remembers which resolved Workflow hooks
// have no reachable workflow plane.
type resolver struct {
	ctx       context.Context
	client    client.Client
	namespace string
	cache     map[openchoreodevv1alpha1.HookRef]*openchoreodevv1alpha1.HookSpec
	noPlane   map[string]string // "<kind>/<name>" -> reason
}

func newResolver(ctx context.Context, c client.Client, namespace string) *resolver {
	return &resolver{
		ctx:       ctx,
		client:    c,
		namespace: namespace,
		cache:     map[openchoreodevv1alpha1.HookRef]*openchoreodevv1alpha1.HookSpec{},
		noPlane:   map[string]string{},
	}
}

func (r *resolver) resolve(ref openchoreodevv1alpha1.HookRef) (*openchoreodevv1alpha1.HookSpec, error) {
	if r.client == nil {
		return nil, nil
	}
	ref = normalizeRef(ref)
	if spec, ok := r.cache[ref]; ok {
		return spec, nil
	}

	var spec *openchoreodevv1alpha1.HookSpec
	switch ref.Kind {
	case openchoreodevv1alpha1.HookRefKindClusterHook:
		chk := &openchoreodevv1alpha1.ClusterHook{}
		if err := r.client.Get(r.ctx, client.ObjectKey{Name: ref.Name}, chk); err != nil {
			if apierrors.IsNotFound(err) {
				r.cache[ref] = nil
				return nil, nil
			}
			return nil, err
		}
		spec = &chk.Spec
	default:
		hk := &openchoreodevv1alpha1.Hook{}
		if err := r.client.Get(r.ctx, client.ObjectKey{Namespace: r.namespace, Name: ref.Name}, hk); err != nil {
			if apierrors.IsNotFound(err) {
				r.cache[ref] = nil
				return nil, nil
			}
			return nil, err
		}
		spec = &hk.Spec
	}
	r.cache[ref] = spec
	r.checkPlane(ref, spec)
	return spec, nil
}

// checkPlane records a warning when the hook's workflow, or the workflow plane
// it targets, cannot be found. The gate fails closed on this at deployment time,
// so surfacing it at admission saves a blocked release.
func (r *resolver) checkPlane(ref openchoreodevv1alpha1.HookRef, spec *openchoreodevv1alpha1.HookSpec) {
	if spec.WorkflowRef == nil {
		return
	}
	key := fmt.Sprintf("%s/%s", ref.Kind, ref.Name)
	wf, err := controller.ResolveWorkflow(r.ctx, r.client, r.namespace, spec.WorkflowRef.Kind, spec.WorkflowRef.Name)
	if err != nil {
		r.noPlane[key] = err.Error()
		return
	}
	wfSpec := wf.GetWorkflowSpec()
	if _, err := controller.GetWorkflowPlaneFromRef(r.ctx, r.client, wf.GetNamespace(), wfSpec.WorkflowPlaneRef); err != nil {
		r.noPlane[key] = err.Error()
	}
}

func (r *resolver) planeWarnings() admission.Warnings {
	keys := make([]string, 0, len(r.noPlane))
	for k := range r.noPlane {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var warnings admission.Warnings
	for _, k := range keys {
		warnings = append(warnings, fmt.Sprintf("hook %s has no workflow plane to run on: %s; a Sync pre-deploy binding will block with PlaneUnavailable", k, r.noPlane[k]))
	}
	return warnings
}
