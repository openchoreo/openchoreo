// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package scheduledtask

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
)

// Reconciler reconciles a ScheduledTask object
type Reconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=openchoreo.dev,resources=scheduledtasks,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=openchoreo.dev,resources=scheduledtasks/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=openchoreo.dev,resources=scheduledtasks/finalizers,verbs=update
// +kubebuilder:rbac:groups=openchoreo.dev,resources=workloads,verbs=get;list;watch
// +kubebuilder:rbac:groups=openchoreo.dev,resources=scheduledtaskbindings,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// The ScheduledTask controller creates a ScheduledTaskBinding that combines the ScheduledTask
// specification with the referenced Workload specification.
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("Reconciling ScheduledTask")

	// Fetch the ScheduledTask instance
	scheduledTask := &openchoreov1alpha1.ScheduledTask{}
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
func (r *Reconciler) reconcileScheduledTaskBinding(ctx context.Context, scheduledTask *openchoreov1alpha1.ScheduledTask) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Find the associated Workload
	workload := &openchoreov1alpha1.Workload{}
	if err := r.Get(ctx, client.ObjectKey{
		Name:      scheduledTask.Spec.WorkloadName,
		Namespace: scheduledTask.Namespace,
	}, workload); err != nil {
		logger.Error(err, "Failed to get Workload",
			"workloadName", scheduledTask.Spec.WorkloadName)
		return ctrl.Result{}, err
	}

	scheduledTaskBinding := &openchoreov1alpha1.ScheduledTaskBinding{
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

func (r *Reconciler) makeScheduledTaskBinding(scheduledTask *openchoreov1alpha1.ScheduledTask, workload *openchoreov1alpha1.Workload) *openchoreov1alpha1.ScheduledTaskBinding {
	stb := &openchoreov1alpha1.ScheduledTaskBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      scheduledTask.Name,
			Namespace: scheduledTask.Namespace,
		},
		Spec: openchoreov1alpha1.ScheduledTaskBindingSpec{
			Owner: openchoreov1alpha1.ScheduledTaskOwner{
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
	// Set up the index for workload reference
	if err := r.setupWorkloadRefIndex(context.Background(), mgr); err != nil {
		return fmt.Errorf("failed to setup workload reference index: %w", err)
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&openchoreov1alpha1.ScheduledTask{}).
		Owns(&openchoreov1alpha1.ScheduledTaskBinding{}).
		Watches(
			&openchoreov1alpha1.Workload{},
			handler.EnqueueRequestsFromMapFunc(r.listScheduledTasksForWorkload),
		).
		Named("scheduledtask").
		Complete(r)
}
