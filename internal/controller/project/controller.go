// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package project

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/controller"
)

// Reconciler reconciles a Project object
type Reconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
}

// +kubebuilder:rbac:groups=openchoreo.dev,resources=projects,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=openchoreo.dev,resources=projects/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=openchoreo.dev,resources=projects/finalizers,verbs=update
// +kubebuilder:rbac:groups=openchoreo.dev,resources=projecttypes,verbs=get;list;watch
// +kubebuilder:rbac:groups=openchoreo.dev,resources=clusterprojecttypes,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=namespaces,verbs=get;list;watch;delete
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Project object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.19.1/pkg/reconcile
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Fetch the Project instance
	project := &openchoreov1alpha1.Project{}
	if err := r.Get(ctx, req.NamespacedName, project); err != nil {
		if apierrors.IsNotFound(err) {
			// The Project resource may have been deleted since it triggered the reconcile
			logger.Info("Project resource not found. Ignoring since it must be deleted.")
			return ctrl.Result{}, nil
		}
		// Error reading the object
		logger.Error(err, "Failed to get Project")
		return ctrl.Result{}, err
	}

	// Keep a copy of the original object for comparison
	old := project.DeepCopy()

	// Handle the deletion of the project
	if !project.DeletionTimestamp.IsZero() {
		logger.Info("Finalizing project")
		return r.finalize(ctx, old, project)
	}

	// Ensure the finalizer is added to the project
	if finalizerAdded, err := r.ensureFinalizer(ctx, project); err != nil || finalizerAdded {
		// Return after adding the finalizer to ensure the finalizer is persisted
		return ctrl.Result{}, err
	}

	// Handle creation of the project
	// Check if a condition exists already to determine if this is a first-time creation
	existingCondition := meta.FindStatusCondition(old.Status.Conditions, controller.TypeCreated)
	isNewResource := existingCondition == nil

	// Set the observed generation
	project.Status.ObservedGeneration = project.Generation

	// Update the status condition to indicate the project is created/ready
	meta.SetStatusCondition(
		&project.Status.Conditions,
		NewProjectCreatedCondition(project.Generation),
	)

	// Surface project release lifecycle health on the Ready condition.
	// Projects with no spec.type stay in the legacy mode (Ready=Unknown);
	// Projects with spec.type set go through the resolveType path which
	// reports ProjectTypeNotFound on a missing reference and Reconciled
	// once the (Cluster)ProjectType snapshot is in hand. Release creation
	// itself lands in a follow-up commit.
	if err := r.evaluateProjectTypeReady(ctx, project); err != nil {
		// Status-write below still happens; return the transient error so
		// the controller-runtime requeues us.
		if statusErr := controller.UpdateStatusConditions(ctx, r.Client, old, project); statusErr != nil {
			logger.Error(statusErr, "Failed to update Project status after resolveType error")
		}
		return ctrl.Result{}, err
	}

	// Update status if needed
	if err := controller.UpdateStatusConditions(ctx, r.Client, old, project); err != nil {
		return ctrl.Result{}, err
	}

	if isNewResource {
		r.Recorder.Event(project, corev1.EventTypeNormal, "ReconcileComplete", "Successfully created "+project.Name)
	}

	return ctrl.Result{}, nil
}

// evaluateProjectTypeReady is the soft-Type-aware entry into the project
// release lifecycle reconcile. When spec.type is unset the Project stays in
// the legacy mode (Ready=Unknown / NoProjectType). When spec.type is set
// the controller fetches the referenced (Cluster)ProjectType and surfaces
// either ProjectTypeNotFound (False) or Reconciled (True) on Ready. Release
// creation against the resolved snapshot lands in a follow-up commit.
func (r *Reconciler) evaluateProjectTypeReady(ctx context.Context, project *openchoreov1alpha1.Project) error {
	if project.Spec.Type == nil {
		controller.MarkUnknownCondition(project, ConditionReady, ReasonNoProjectType,
			"spec.type is unset; set spec.type to enable automatic ProjectRelease creation")
		return nil
	}

	if _, err := r.resolveType(ctx, project); err != nil {
		if apierrors.IsNotFound(err) {
			controller.MarkFalseCondition(project, ConditionReady, ReasonProjectTypeNotFound,
				fmt.Sprintf("%s %q not found", projectTypeKind(project.Spec.Type.Kind), project.Spec.Type.Name))
			return nil
		}
		return err
	}

	controller.MarkTrueCondition(project, ConditionReady, ReasonReconciled,
		fmt.Sprintf("%s %q resolved", projectTypeKind(project.Spec.Type.Kind), project.Spec.Type.Name))
	return nil
}

// resolveType fetches the (Cluster)ProjectType referenced by project.Spec.Type
// and returns the snapshot to embed in a ProjectRelease. Returns an
// apierrors.IsNotFound error when the referenced template is missing.
// Mirrors internal/controller/resource/controller.go:resolveType.
func (r *Reconciler) resolveType(ctx context.Context, project *openchoreov1alpha1.Project) (
	openchoreov1alpha1.ProjectReleaseProjectType, error,
) {
	kind := projectTypeKind(project.Spec.Type.Kind)
	name := project.Spec.Type.Name

	switch kind {
	case openchoreov1alpha1.ProjectTypeRefKindClusterProjectType:
		cpt := &openchoreov1alpha1.ClusterProjectType{}
		if err := r.Get(ctx, types.NamespacedName{Name: name}, cpt); err != nil {
			return openchoreov1alpha1.ProjectReleaseProjectType{}, err
		}
		// ClusterProjectTypeSpec is structurally identical to ProjectTypeSpec
		// today; if it ever diverges, this cast breaks at compile time and
		// ProjectReleaseProjectType.Spec needs a kind discriminator. Mirrors
		// the (Cluster)ResourceType precedent.
		return openchoreov1alpha1.ProjectReleaseProjectType{
			Kind: kind,
			Name: name,
			Spec: openchoreov1alpha1.ProjectTypeSpec(cpt.Spec),
		}, nil
	default:
		pt := &openchoreov1alpha1.ProjectType{}
		if err := r.Get(ctx, types.NamespacedName{Name: name, Namespace: project.Namespace}, pt); err != nil {
			return openchoreov1alpha1.ProjectReleaseProjectType{}, err
		}
		return openchoreov1alpha1.ProjectReleaseProjectType{
			Kind: kind,
			Name: name,
			Spec: pt.Spec,
		}, nil
	}
}

// projectTypeKind returns the Kind to use for type resolution, defaulting an
// empty Kind to ProjectType (namespaced) per the Project CRD's stated
// default on spec.type.kind.
func projectTypeKind(k openchoreov1alpha1.ProjectTypeRefKind) openchoreov1alpha1.ProjectTypeRefKind {
	if k == "" {
		return openchoreov1alpha1.ProjectTypeRefKindProjectType
	}
	return k
}

// SetupWithManager sets up the controller with the Manager.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	if r.Recorder == nil {
		r.Recorder = mgr.GetEventRecorderFor("project-controller")
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&openchoreov1alpha1.Project{}).
		Named("project").
		Watches(&openchoreov1alpha1.Component{},
			handler.EnqueueRequestsFromMapFunc(r.findProjectForComponent)).
		Complete(r)
}
