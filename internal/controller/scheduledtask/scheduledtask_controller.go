// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package scheduledtask

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	choreov1 "github.com/openchoreo/openchoreo/api/v1"
)

// Reconciler reconciles a ScheduledTask object
type Reconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=core.choreo.dev,resources=scheduledtasks,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core.choreo.dev,resources=scheduledtasks/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=core.choreo.dev,resources=scheduledtasks/finalizers,verbs=update
// +kubebuilder:rbac:groups=core.choreo.dev,resources=workloads,verbs=get;list;watch
// +kubebuilder:rbac:groups=core.choreo.dev,resources=scheduledtaskbindings,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// The ScheduledTask controller creates a ScheduledTaskBinding that combines the ScheduledTask
// specification with the referenced Workload specification.
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("Reconciling ScheduledTask")

	// Fetch the ScheduledTask instance
	scheduledTask := &choreov1.ScheduledTask{}
	if err := r.Get(ctx, req.NamespacedName, scheduledTask); err != nil {
		if client.IgnoreNotFound(err) != nil {
			logger.Error(err, "Failed to get ScheduledTask")
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	if res, err := r.reconcileScheduledTaskBinding(ctx, scheduledTask); err != nil || res.Requeue {
		return res, err
	}

	return ctrl.Result{}, nil
}

// reconcileScheduledTaskBinding reconciles the ScheduledTaskBinding with the given ScheduledTask.
func (r *Reconciler) reconcileScheduledTaskBinding(ctx context.Context, scheduledTask *choreov1.ScheduledTask) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Find the associated Workload
	workload := &choreov1.Workload{}
	if err := r.Get(ctx, client.ObjectKey{
		Name:      scheduledTask.Spec.WorkloadName,
		Namespace: scheduledTask.Namespace,
	}, workload); err != nil {
		logger.Error(err, "Failed to get Workload",
			"workloadName", scheduledTask.Spec.WorkloadName)
		return ctrl.Result{}, err
	}

	scheduledTaskBinding := &choreov1.ScheduledTaskBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      scheduledTask.Name,
			Namespace: scheduledTask.Namespace,
		},
	}
	op, err := controllerutil.CreateOrUpdate(ctx, r.Client, scheduledTaskBinding, func() error {
		scheduledTaskBinding.Spec = r.makeScheduledTaskBinding(scheduledTask, workload).Spec
		return controllerutil.SetControllerReference(scheduledTask, scheduledTaskBinding, r.Scheme)
	})
	if err != nil {
		logger.Error(err, "Failed to reconcile ScheduledTaskBinding", "ScheduledTask", scheduledTask.Name)
		return ctrl.Result{}, err
	}
	if op == controllerutil.OperationResultCreated ||
		op == controllerutil.OperationResultUpdated {
		logger.Info("Successfully reconciled ScheduledTaskBinding", "ScheduledTask", scheduledTask.Name, "Operation", op)
		return ctrl.Result{Requeue: true}, nil
	}
	return ctrl.Result{}, nil
}

func (r *Reconciler) makeScheduledTaskBinding(scheduledTask *choreov1.ScheduledTask, workload *choreov1.Workload) *choreov1.ScheduledTaskBinding {
	stb := &choreov1.ScheduledTaskBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      scheduledTask.Name,
			Namespace: scheduledTask.Namespace,
		},
		Spec: choreov1.ScheduledTaskBindingSpec{
			Owner: choreov1.ScheduledTaskOwner{
				ProjectName:   scheduledTask.Spec.Owner.ProjectName,
				ComponentName: scheduledTask.Spec.Owner.ComponentName,
			},
			Environment:  "development", // This should come from the actual environment when creating bindings
			ClassName:    scheduledTask.Spec.ClassName,
			WorkloadSpec: workload.Spec.WorkloadTemplateSpec,
			Overrides:    scheduledTask.Spec.Overrides,
		},
	}
	return stb
}

// SetupWithManager sets up the controller with the Manager.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&choreov1.ScheduledTask{}).
		Named("scheduledtask").
		Complete(r)
}
