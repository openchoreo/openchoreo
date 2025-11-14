// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package scheduledtaskbinding

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/controller/scheduledtaskbinding/render"
	"github.com/openchoreo/openchoreo/internal/labels"
)

// Reconciler reconciles a ScheduledTaskBinding object
type Reconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=openchoreo.dev,resources=scheduledtaskbindings,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=openchoreo.dev,resources=scheduledtaskbindings/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=openchoreo.dev,resources=scheduledtaskbindings/finalizers,verbs=update
// +kubebuilder:rbac:groups=openchoreo.dev,resources=scheduledtaskclasses,verbs=get;list;watch
// +kubebuilder:rbac:groups=openchoreo.dev,resources=releases,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=openchoreo.dev,resources=environments,verbs=get;list;watch
// +kubebuilder:rbac:groups=openchoreo.dev,resources=dataplanes,verbs=get;list;watch
// +kubebuilder:rbac:groups=openchoreo.dev,resources=secretreferences,verbs=get;list;watch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Fetch the ScheduledTaskBinding instance
	scheduledTaskBinding := &openchoreov1alpha1.ScheduledTaskBinding{}
	if err := r.Get(ctx, req.NamespacedName, scheduledTaskBinding); err != nil {
		if client.IgnoreNotFound(err) != nil {
			logger.Error(err, "Failed to get ScheduledTaskBinding")
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	// Fetch the associated ScheduledTaskClass
	scheduledTaskClass := &openchoreov1alpha1.ScheduledTaskClass{}
	if err := r.Get(ctx, client.ObjectKey{
		Namespace: scheduledTaskBinding.Namespace,
		Name:      scheduledTaskBinding.Spec.ClassName,
	}, scheduledTaskClass); err != nil {
		logger.Error(err, "Failed to get ScheduledTaskClass", "scheduledTaskClassName", scheduledTaskBinding.Spec.ClassName)
		return ctrl.Result{}, err
	}

	// Fetch Environment to get DataPlane reference
	environment := &openchoreov1alpha1.Environment{}
	if err := r.Get(ctx, client.ObjectKey{
		Namespace: scheduledTaskBinding.Namespace,
		Name:      scheduledTaskBinding.Spec.Environment,
	}, environment); err != nil {
		logger.Error(err, "Failed to get Environment", "Environment", scheduledTaskBinding.Spec.Environment)
		return ctrl.Result{}, err
	}

	// Fetch DataPlane and image pull SecretReferences if configured
	var dataPlane *openchoreov1alpha1.DataPlane
	imagePullSecretReferences := make(map[string]*openchoreov1alpha1.SecretReference)

	if environment.Spec.DataPlaneRef != "" {
		dataPlane = &openchoreov1alpha1.DataPlane{}
		if err := r.Get(ctx, client.ObjectKey{
			Namespace: scheduledTaskBinding.Namespace,
			Name:      environment.Spec.DataPlaneRef,
		}, dataPlane); err != nil {
			logger.Error(err, "Failed to get DataPlane", "DataPlane", environment.Spec.DataPlaneRef)
			return ctrl.Result{}, err
		}

		// Fetch image pull SecretReferences from DataPlane's ImagePullSecretRefs
		for _, secretRefName := range dataPlane.Spec.ImagePullSecretRefs {
			secretRef := &openchoreov1alpha1.SecretReference{}
			if err := r.Get(ctx, client.ObjectKey{
				Namespace: scheduledTaskBinding.Namespace,
				Name:      secretRefName,
			}, secretRef); err != nil {
				logger.Error(err, "Failed to get image pull SecretReference", "SecretReference", secretRefName)
				return ctrl.Result{}, err
			}
			imagePullSecretReferences[secretRefName] = secretRef
		}
	}

	if res, err := r.reconcileRelease(ctx, scheduledTaskBinding, scheduledTaskClass, dataPlane, imagePullSecretReferences); err != nil || res.Requeue {
		return res, err
	}

	return ctrl.Result{}, nil
}

// reconcileRelease reconciles the Release associated with the ScheduledTaskBinding.
func (r *Reconciler) reconcileRelease(ctx context.Context, scheduledTaskBinding *openchoreov1alpha1.ScheduledTaskBinding,
	scheduledTaskClass *openchoreov1alpha1.ScheduledTaskClass, dataPlane *openchoreov1alpha1.DataPlane,
	imagePullSecretReferences map[string]*openchoreov1alpha1.SecretReference) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	release := &openchoreov1alpha1.Release{
		ObjectMeta: metav1.ObjectMeta{
			Name:      scheduledTaskBinding.Name,
			Namespace: scheduledTaskBinding.Namespace,
		},
	}

	op, err := controllerutil.CreateOrUpdate(ctx, r.Client, release, func() error {
		rCtx := render.Context{
			ScheduledTaskBinding:      scheduledTaskBinding,
			ScheduledTaskClass:        scheduledTaskClass,
			DataPlane:                 dataPlane,
			ImagePullSecretReferences: imagePullSecretReferences,
		}
		desired := r.makeRelease(rCtx)
		release.Labels = desired.Labels
		release.Spec = desired.Spec
		if len(rCtx.Errors()) > 0 {
			err := rCtx.Error()
			return err
		}
		return controllerutil.SetControllerReference(scheduledTaskBinding, release, r.Scheme)
	})
	if err != nil {
		logger.Error(err, "Failed to reconcile Release", "Release", release.Name)
		return ctrl.Result{}, err
	}
	if op == controllerutil.OperationResultCreated ||
		op == controllerutil.OperationResultUpdated {
		logger.Info("Successfully reconciled Release", "Release", release.Name, "Operation", op)
		// TODO: Update ScheduledTaskBinding status and requeue for further processing
		return ctrl.Result{Requeue: true}, nil
	}
	return ctrl.Result{}, nil
}

func (r *Reconciler) makeRelease(rCtx render.Context) *openchoreov1alpha1.Release {
	release := &openchoreov1alpha1.Release{
		ObjectMeta: metav1.ObjectMeta{
			Name:      rCtx.ScheduledTaskBinding.Name,
			Namespace: rCtx.ScheduledTaskBinding.Namespace,
			Labels:    r.makeLabels(rCtx.ScheduledTaskBinding),
		},
		Spec: openchoreov1alpha1.ReleaseSpec{
			Owner: openchoreov1alpha1.ReleaseOwner{
				ProjectName:   rCtx.ScheduledTaskBinding.Spec.Owner.ProjectName,
				ComponentName: rCtx.ScheduledTaskBinding.Spec.Owner.ComponentName,
			},
			EnvironmentName: rCtx.ScheduledTaskBinding.Spec.Environment,
		},
	}

	var resources []openchoreov1alpha1.Resource

	// Add ExternalSecret resources FIRST (so secrets exist when job tries to pull images)
	if res := render.ExternalSecrets(rCtx); res != nil {
		for _, externalSecret := range res {
			resources = append(resources, *externalSecret)
		}
	}

	// Add CronJob resource for scheduled execution
	if res := render.CronJob(rCtx); res != nil {
		resources = append(resources, *res)
	}

	release.Spec.Resources = resources
	return release
}

// SetupWithManager sets up the controller with the Manager.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	// Set up the index for scheduled task class reference
	if err := r.setupScheduledTaskClassRefIndex(context.Background(), mgr); err != nil {
		return fmt.Errorf("failed to setup scheduled task class reference index: %w", err)
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&openchoreov1alpha1.ScheduledTaskBinding{}).
		Watches(
			&openchoreov1alpha1.ScheduledTaskClass{},
			handler.EnqueueRequestsFromMapFunc(r.listScheduledTaskBindingsForScheduledTaskClass),
		).
		Named("scheduledtaskbinding").
		Complete(r)
}

// makeLabels creates standard labels for Release resources, merging with ScheduledTaskBinding labels.
func (r *Reconciler) makeLabels(scheduledTaskBinding *openchoreov1alpha1.ScheduledTaskBinding) map[string]string {
	// Start with ScheduledTaskBinding's existing labels
	result := make(map[string]string)
	for k, v := range scheduledTaskBinding.Labels {
		result[k] = v
	}

	// Add/overwrite component-specific labels
	result[labels.LabelKeyOrganizationName] = scheduledTaskBinding.Namespace
	result[labels.LabelKeyProjectName] = scheduledTaskBinding.Spec.Owner.ProjectName
	result[labels.LabelKeyComponentName] = scheduledTaskBinding.Spec.Owner.ComponentName
	result[labels.LabelKeyEnvironmentName] = scheduledTaskBinding.Spec.Environment

	return result
}
