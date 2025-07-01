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

	choreov1 "github.com/openchoreo/openchoreo/api/v1"
	"github.com/openchoreo/openchoreo/internal/controller/scheduledtaskbinding/render"
)

// Reconciler reconciles a ScheduledTaskBinding object
type Reconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=core.choreo.dev,resources=scheduledtaskbindings,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core.choreo.dev,resources=scheduledtaskbindings/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=core.choreo.dev,resources=scheduledtaskbindings/finalizers,verbs=update
// +kubebuilder:rbac:groups=core.choreo.dev,resources=scheduledtaskclasses,verbs=get;list;watch
// +kubebuilder:rbac:groups=core.choreo.dev,resources=scheduledtaskreleases,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Fetch the ScheduledTaskBinding instance
	scheduledTaskBinding := &choreov1.ScheduledTaskBinding{}
	if err := r.Get(ctx, req.NamespacedName, scheduledTaskBinding); err != nil {
		if client.IgnoreNotFound(err) != nil {
			logger.Error(err, "Failed to get ScheduledTaskBinding")
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	// Fetch the associated ScheduledTaskClass
	scheduledTaskClass := &choreov1.ScheduledTaskClass{}
	if err := r.Get(ctx, client.ObjectKey{
		Namespace: scheduledTaskBinding.Namespace,
		Name:      scheduledTaskBinding.Spec.ClassName,
	}, scheduledTaskClass); err != nil {
		logger.Error(err, "Failed to get ScheduledTaskClass", "scheduledTaskClassName", scheduledTaskBinding.Spec.ClassName)
		return ctrl.Result{}, err
	}

	if res, err := r.reconcileScheduledTaskRelease(ctx, scheduledTaskBinding, scheduledTaskClass); err != nil || res.Requeue {
		return res, err
	}

	return ctrl.Result{}, nil
}

// reconcileScheduledTaskRelease reconciles the ScheduledTaskRelease associated with the ScheduledTaskBinding.
func (r *Reconciler) reconcileScheduledTaskRelease(ctx context.Context, scheduledTaskBinding *choreov1.ScheduledTaskBinding, scheduledTaskClass *choreov1.ScheduledTaskClass) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	scheduledTaskRelease := &choreov1.ScheduledTaskRelease{
		ObjectMeta: metav1.ObjectMeta{
			Name:      scheduledTaskBinding.Name,
			Namespace: scheduledTaskBinding.Namespace,
		},
	}

	op, err := controllerutil.CreateOrUpdate(ctx, r.Client, scheduledTaskRelease, func() error {
		rCtx := render.Context{
			ScheduledTaskBinding: scheduledTaskBinding,
			ScheduledTaskClass:   scheduledTaskClass,
		}
		scheduledTaskRelease.Spec = r.makeScheduledTaskRelease(rCtx).Spec
		if len(rCtx.Errors()) > 0 {
			err := rCtx.Error()
			return err
		}
		return controllerutil.SetControllerReference(scheduledTaskBinding, scheduledTaskRelease, r.Scheme)
	})
	if err != nil {
		logger.Error(err, "Failed to reconcile ScheduledTaskRelease", "ScheduledTaskRelease", scheduledTaskRelease.Name)
		return ctrl.Result{}, err
	}
	if op == controllerutil.OperationResultCreated ||
		op == controllerutil.OperationResultUpdated {
		logger.Info("Successfully reconciled ScheduledTaskRelease", "ScheduledTaskRelease", scheduledTaskRelease.Name, "Operation", op)
		return ctrl.Result{}, nil
	}
	return ctrl.Result{}, nil
}

func (r *Reconciler) makeScheduledTaskRelease(rCtx render.Context) *choreov1.ScheduledTaskRelease {
	str := &choreov1.ScheduledTaskRelease{
		ObjectMeta: metav1.ObjectMeta{
			Name:      rCtx.ScheduledTaskBinding.Name,
			Namespace: rCtx.ScheduledTaskBinding.Namespace,
		},
		Spec: choreov1.ScheduledTaskReleaseSpec{
			Owner: choreov1.ScheduledTaskOwner{
				ProjectName:   rCtx.ScheduledTaskBinding.Spec.Owner.ProjectName,
				ComponentName: rCtx.ScheduledTaskBinding.Spec.Owner.ComponentName,
			},
			EnvironmentName: rCtx.ScheduledTaskBinding.Spec.Environment,
		},
	}

	var resources []choreov1.Resource

	// Add CronJob resource for scheduled execution
	if res := render.CronJob(rCtx); res != nil {
		resources = append(resources, *res)
	}

	str.Spec.Resources = resources
	return str
}

// SetupWithManager sets up the controller with the Manager.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	// Set up the index for scheduled task class reference
	if err := r.setupScheduledTaskClassRefIndex(context.Background(), mgr); err != nil {
		return fmt.Errorf("failed to setup scheduled task class reference index: %w", err)
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&choreov1.ScheduledTaskBinding{}).
		Watches(
			&choreov1.ScheduledTaskClass{},
			handler.EnqueueRequestsFromMapFunc(r.listScheduledTaskBindingsForScheduledTaskClass),
		).
		Named("scheduledtaskbinding").
		Complete(r)
}
