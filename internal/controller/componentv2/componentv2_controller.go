// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package componentv2

import (
	"context"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

// Reconciler reconciles a ComponentV2 object
type Reconciler struct {
	client.Client
	// IsGitOpsMode indicates whether the controller is running in GitOps mode
	// In GitOps mode, the controller will not create or update resources directly in the cluster,
	// but will instead generate the necessary manifests and creates GitCommitRequests to update the Git repository.
	IsGitOpsMode bool
	Scheme       *runtime.Scheme
}

// +kubebuilder:rbac:groups=openchoreo.dev,resources=componentv2s,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=openchoreo.dev,resources=componentv2s/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=openchoreo.dev,resources=componentv2s/finalizers,verbs=update
// +kubebuilder:rbac:groups=openchoreo.dev,resources=workloads,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=openchoreo.dev,resources=gitcommitrequests,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the ComponentV2 object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.19.1/pkg/reconcile
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Fetch the ComponentV2 instance for this reconcile request
	comp := &openchoreov1alpha1.ComponentV2{}
	if err := r.Get(ctx, req.NamespacedName, comp); err != nil {
		if apierrors.IsNotFound(err) {
			// The ComponentV2 resource may have been deleted since it triggered the reconcile
			logger.Info("ComponentV2 resource not found. Ignoring since it must be deleted.")
			return ctrl.Result{}, nil
		}
		// Error reading the object
		logger.Error(err, "Failed to get ComponentV2")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.IsGitOpsMode = true
	return ctrl.NewControllerManagedBy(mgr).
		For(&openchoreov1alpha1.ComponentV2{}).
		Named("componentv2").
		Complete(r)
}

// reconcileWorkload reconciles the workload associated with the ComponentV2.
/*
func (r *Reconciler) reconcileWorkload(ctx context.Context, comp *openchoreov1alpha1.ComponentV2) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	if r.IsGitOpsMode {
		// GitOps mode: Create GitCommitRequest instead of applying directly
		workload := r.makeWorkload(comp)
		gitCommitReq := &openchoreov1alpha1.GitCommitRequest{
			ObjectMeta: metav1.ObjectMeta{
				// Name:      makeWorkloadGitCommitRequestName(comp, workload.Spec.EnvironmentName),
				Namespace: comp.Namespace,
			},
		}

		op, err := controllerutil.CreateOrUpdate(ctx, r.Client, gitCommitReq, func() error {
			gitCommitReq.Spec = r.makeGitCommitRequestForWorkload(comp, workload).Spec
			return controllerutil.SetControllerReference(comp, gitCommitReq, r.Scheme)
		})
		if err != nil {
			logger.Error(err, "Failed to reconcile GitCommitRequest for Workload", "GitCommitRequest", gitCommitReq.Name)
			return ctrl.Result{}, err
		}
		if op == controllerutil.OperationResultCreated ||
			op == controllerutil.OperationResultUpdated {
			logger.Info("Successfully reconciled GitCommitRequest for Workload", "GitCommitRequest", gitCommitReq.Name, "Operation", op)
			return ctrl.Result{Requeue: true}, nil
		}
		return ctrl.Result{}, nil
	} else {
		workload := &openchoreov1alpha1.Workload{
			ObjectMeta: metav1.ObjectMeta{
				Name:      comp.Name,
				Namespace: comp.Namespace,
			},
		}
		op, err := controllerutil.CreateOrUpdate(ctx, r.Client, workload, func() error {
			workload.Spec = r.makeWorkload(comp).Spec
			return controllerutil.SetControllerReference(comp, workload, r.Scheme)
		})
		if err != nil {
			logger.Error(err, "Failed to reconcile Workload", "Workload", workload.Name)
			return ctrl.Result{}, err
		}
		if op == controllerutil.OperationResultCreated ||
			op == controllerutil.OperationResultUpdated {
			logger.Info("Successfully reconciled Workload", "Workload", workload.Name, "Operation", op)
			return ctrl.Result{Requeue: true}, nil
		}
		return ctrl.Result{}, nil
	}
}
*/

/*
func makeWorkloadGitCommitRequestName(comp *openchoreov1alpha1.ComponentV2, envName string) string {
	return fmt.Sprintf("%s-%s-workload", comp.Name, envName)
}
*/

/*
func (r *Reconciler) makeWorkload(comp *openchoreov1alpha1.ComponentV2) *openchoreov1alpha1.Workload {
	return &openchoreov1alpha1.Workload{
		ObjectMeta: metav1.ObjectMeta{
			Name:      comp.Name,
			Namespace: comp.Namespace,
		},
		Spec: openchoreov1alpha1.WorkloadSpec{
			Owner: openchoreov1alpha1.WorkloadOwner{
				ProjectName:   comp.Spec.Owner.ProjectName,
				ComponentName: comp.Name,
			},
			// EnvironmentName:      "development",
			// WorkloadTemplateSpec: comp.Spec.Workload,
		},
	}
}
*/

// makeGitCommitRequestForWorkload creates a GitCommitRequest for the given workload
/*
func (r *Reconciler) makeGitCommitRequestForWorkload(comp *openchoreov1alpha1.ComponentV2, workload *openchoreov1alpha1.Workload) *openchoreov1alpha1.GitCommitRequest {
	workloadYAML, err := r.generateWorkloadYAML(workload)
	if err != nil {
		// In a real implementation, we'd handle this error better
		workloadYAML = fmt.Sprintf("# Error generating YAML: %v", err)
	}

	envName := "development" // TODO: Replace with actual environment name from component spec or context
	filePath := fmt.Sprintf("applications/projects/%s/components/%s/overlays/%s/workload.yaml",
		comp.Spec.Owner.ProjectName, comp.Name, envName)
	message := fmt.Sprintf("Update workload for component %s in %s environment", comp.Name, envName)

	files := []openchoreov1alpha1.FileEdit{
		{
			Path:    filePath,
			Content: workloadYAML,
		},
	}

	return &openchoreov1alpha1.GitCommitRequest{
		ObjectMeta: metav1.ObjectMeta{
			Name:      makeWorkloadGitCommitRequestName(comp, envName),
			Namespace: comp.Namespace,
		},
		Spec: r.makeCommitRequestSpec(message, files),
	}
}
*/

// makeCommitRequestSpec creates a GitCommitRequestSpec with the given message and files
/*
func (r *Reconciler) makeCommitRequestSpec(message string, files []openchoreov1alpha1.FileEdit) openchoreov1alpha1.GitCommitRequestSpec {
	return openchoreov1alpha1.GitCommitRequestSpec{
		RepoURL: "https://github.com/Mirage20/openchoreo-gitops", // TODO: Configure GitOps repository URL
		Branch:  "controller-test",                               // TODO: Configure target branch (e.g., "main")
		Message: message,
		Author: openchoreov1alpha1.GitCommitAuthor{
			Name:  "OpenChoreo",
			Email: "controller@openchoreo.dev",
		},
		AuthSecretRef: "git-credentials",
		Files:         files,
	}
}
*/

// generateWorkloadYAML converts a Workload resource to YAML format
/*
func (r *Reconciler) generateWorkloadYAML(workload *openchoreov1alpha1.Workload) (string, error) {
	// Create a clean copy for serialization
	workloadCopy := workload.DeepCopy()

	// Ensure TypeMeta is set for proper YAML generation
	workloadCopy.TypeMeta = metav1.TypeMeta{
		APIVersion: "openchoreo.dev/v1alpha1",
		Kind:       "Workload",
	}

	// Clear fields that shouldn't be in GitOps
	workloadCopy.ObjectMeta.ResourceVersion = ""
	workloadCopy.ObjectMeta.UID = ""
	workloadCopy.ObjectMeta.Generation = 0
	workloadCopy.ObjectMeta.CreationTimestamp = metav1.Time{}
	workloadCopy.ObjectMeta.ManagedFields = nil
	workloadCopy.ObjectMeta.OwnerReferences = nil
	workloadCopy.Status = openchoreov1alpha1.WorkloadStatus{}

	yamlBytes, err := yaml.Marshal(workloadCopy)
	if err != nil {
		return "", fmt.Errorf("failed to marshal workload to YAML: %w", err)
	}

	return string(yamlBytes), nil
}
*/
