// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package project

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
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
// +kubebuilder:rbac:groups=openchoreo.dev,resources=projectreleases,verbs=get;list;watch;create;update;patch;delete
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

	return r.reconcile(ctx, old, project)
}

// reconcile drives the Project's release lifecycle. Deferred whole-status
// writer (mirrors internal/controller/resource/controller.go) so status.
// LatestRelease, status.Conditions, and status.ObservedGeneration land
// atomically and only when something actually changed. Mirrors the
// resourcerelease pattern; the simpler conditions-only update helper used
// previously can't carry status.LatestRelease forward.
func (r *Reconciler) reconcile(ctx context.Context, old, project *openchoreov1alpha1.Project) (result ctrl.Result, rErr error) {
	logger := log.FromContext(ctx)

	// First-time observation check: surfaced as a Recorder event below.
	// Use the existing TypeCreated condition presence as the "I've been
	// reconciled before" sentinel so the event semantics match prior
	// behavior.
	isNewResource := meta.FindStatusCondition(old.Status.Conditions, controller.TypeCreated) == nil

	defer func() {
		project.Status.ObservedGeneration = project.Generation
		if apiequality.Semantic.DeepEqual(old.Status, project.Status) {
			return
		}
		if err := r.Status().Update(ctx, project); err != nil {
			logger.Error(err, "Failed to update Project status")
			rErr = kerrors.NewAggregate([]error{rErr, err})
		}
	}()

	// Set the legacy Created condition (always True after the first
	// reconcile; preserved for backward compatibility with consumers
	// watching this signal).
	meta.SetStatusCondition(
		&project.Status.Conditions,
		NewProjectCreatedCondition(project.Generation),
	)

	if err := r.reconcileProjectRelease(ctx, project); err != nil {
		return ctrl.Result{}, err
	}

	if isNewResource {
		r.Recorder.Event(project, corev1.EventTypeNormal, "ReconcileComplete", "Successfully created "+project.Name)
	}

	return ctrl.Result{}, nil
}

// reconcileProjectRelease is the soft-Type-aware entry into the project
// release lifecycle reconcile. When spec.type is unset the Project stays in
// the legacy mode (Ready=Unknown / NoProjectType). When spec.type is set
// the controller fetches the referenced (Cluster)ProjectType, computes a
// hash over the inlined snapshot + parameters, and cuts a new
// ProjectRelease whenever the hash drifts from status.latestRelease.Hash.
// Surfaces ProjectTypeNotFound on a missing reference and Reconciled when
// the latest release is in place.
func (r *Reconciler) reconcileProjectRelease(ctx context.Context, project *openchoreov1alpha1.Project) error {
	if project.Spec.Type == nil {
		controller.MarkUnknownCondition(project, ConditionReady, ReasonNoProjectType,
			"spec.type is unset; set spec.type to enable automatic ProjectRelease creation")
		return nil
	}

	snapshot, err := r.resolveType(ctx, project)
	if err != nil {
		if apierrors.IsNotFound(err) {
			controller.MarkFalseCondition(project, ConditionReady, ReasonProjectTypeNotFound,
				fmt.Sprintf("%s %q not found", projectTypeKind(project.Spec.Type.Kind), project.Spec.Type.Name))
			return nil
		}
		return err
	}

	releaseHash := computeReleaseHash(ReleaseSpec{
		ProjectType: snapshot,
		Parameters:  project.Spec.Parameters,
	}, nil)

	if project.Status.LatestRelease == nil || project.Status.LatestRelease.Hash != releaseHash {
		prName := fmt.Sprintf("%s-%s", project.Name, releaseHash)
		if err := r.ensureProjectRelease(ctx, project, snapshot, prName); err != nil {
			return err
		}
		project.Status.LatestRelease = &openchoreov1alpha1.LatestProjectRelease{
			Name: prName,
			Hash: releaseHash,
		}
	}

	controller.MarkTrueCondition(project, ConditionReady, ReasonReconciled,
		fmt.Sprintf("ProjectRelease %s in place", project.Status.LatestRelease.Name))
	return nil
}

// ensureProjectRelease creates a ProjectRelease with the given name if it
// doesn't already exist. On AlreadyExists, verify the existing release
// belongs to this Project before claiming it; refuse to take over a release
// owned by a different Project (defends against hash collisions and
// external creates landing on the same name). Mirrors
// resource.ensureResourceRelease.
func (r *Reconciler) ensureProjectRelease(
	ctx context.Context,
	project *openchoreov1alpha1.Project,
	pt openchoreov1alpha1.ProjectReleaseProjectType,
	name string,
) error {
	pr := &openchoreov1alpha1.ProjectRelease{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: project.Namespace,
		},
		Spec: openchoreov1alpha1.ProjectReleaseSpec{
			Owner: openchoreov1alpha1.ProjectReleaseOwner{
				ProjectName: project.Name,
			},
			ProjectType: pt,
			Parameters:  project.Spec.Parameters,
		},
	}
	err := r.Create(ctx, pr)
	if err == nil {
		return nil
	}
	if !apierrors.IsAlreadyExists(err) {
		return fmt.Errorf("create ProjectRelease %q: %w", name, err)
	}

	existing := &openchoreov1alpha1.ProjectRelease{}
	if getErr := r.Get(ctx, types.NamespacedName{Name: name, Namespace: project.Namespace}, existing); getErr != nil {
		return fmt.Errorf("verify existing ProjectRelease %q: %w", name, getErr)
	}
	if existing.Spec.Owner.ProjectName != project.Name {
		return fmt.Errorf("ProjectRelease %q already exists with owner %q, refusing to claim it for %q",
			name, existing.Spec.Owner.ProjectName, project.Name)
	}
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

// SetupWithManager sets up the controller with the Manager. The watch
// surface re-enqueues Projects when:
//   - a Component owned by the Project changes (legacy hierarchy hook)
//   - a (Cluster)ProjectType referenced via spec.type changes — so PE
//     edits to the template drive a new ProjectRelease cut on the
//     referencing Projects
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	if r.Recorder == nil {
		r.Recorder = mgr.GetEventRecorderFor("project-controller")
	}

	if err := r.setupProjectTypeRefIndex(context.Background(), mgr); err != nil {
		return fmt.Errorf("setup project type reference index: %w", err)
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&openchoreov1alpha1.Project{}).
		Named("project").
		Watches(&openchoreov1alpha1.Component{},
			handler.EnqueueRequestsFromMapFunc(r.findProjectForComponent)).
		Watches(&openchoreov1alpha1.ProjectType{},
			handler.EnqueueRequestsFromMapFunc(r.listProjectsForProjectType)).
		Watches(&openchoreov1alpha1.ClusterProjectType{},
			handler.EnqueueRequestsFromMapFunc(r.listProjectsForClusterProjectType)).
		Complete(r)
}
