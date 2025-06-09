// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package build

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	"github.com/google/go-github/v69/github"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	choreov1 "github.com/openchoreo/openchoreo/api/v1"
	"github.com/openchoreo/openchoreo/internal/controller"
	"github.com/openchoreo/openchoreo/internal/controller/build/integrations"
	"github.com/openchoreo/openchoreo/internal/controller/build/integrations/kubernetes"
	argointegrations "github.com/openchoreo/openchoreo/internal/controller/build/integrations/kubernetes/ci/argo"
	"github.com/openchoreo/openchoreo/internal/controller/build/integrations/source"
	sourcegithub "github.com/openchoreo/openchoreo/internal/controller/build/integrations/source/github"
	"github.com/openchoreo/openchoreo/internal/controller/build/resources"
	"github.com/openchoreo/openchoreo/internal/dataplane"
	dpKubernetes "github.com/openchoreo/openchoreo/internal/dataplane/kubernetes"
	argoproj "github.com/openchoreo/openchoreo/internal/dataplane/kubernetes/types/argoproj.io/workflow/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/labels"
)

// Reconciler reconciles a Build object
type Reconciler struct {
	client.Client
	DpClientMgr  *dpKubernetes.KubeClientManager
	Scheme       *runtime.Scheme
	GithubClient *github.Client
	recorder     record.EventRecorder
}

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Build object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.19.1/pkg/reconcile
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Fetch the build resource
	build := &choreov1.Build{}
	if err := r.Get(ctx, req.NamespacedName, build); err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("Build resource not found, ignoring since object must be deleted")
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Failed to get Build")
		return ctrl.Result{}, err
	}

	if build.Spec.BuildConfiguration.Docker == nil && build.Spec.BuildConfiguration.Buildpack == nil {
		r.recorder.Eventf(build, corev1.EventTypeWarning, "BuildConfigsNotFound",
			"Build configurations are not found: %s", build.Name)
		logger.Error(errors.New("BuildConfigsNotFound"), "build cannot proceed without build configs")
		return ctrl.Result{}, nil
	}

	oldBuild := build.DeepCopy()

	if len(build.Status.Conditions) == 0 {
		setInitialBuildConditions(build)
		return controller.UpdateStatusConditionsAndRequeue(ctx, r.Client, oldBuild, build)
	}

	// Handle the deletion of the build
	if !build.DeletionTimestamp.IsZero() {
		logger.Info("Finalizing build")
		return r.finalize(ctx, oldBuild, build)
	}

	// Ensure the finalizer is added to the build
	if finalizerAdded, err := r.ensureFinalizer(ctx, build); err != nil || finalizerAdded {
		return ctrl.Result{}, err
	}

	if shouldIgnoreReconcile(build) {
		return ctrl.Result{}, nil
	}

	// Create a new build context for the build with required hierarchy objects
	buildCtx, err := r.makeBuildContext(ctx, build)
	if err != nil {
		logger.Error(err, "Error creating build context")
		r.recorder.Eventf(build, corev1.EventTypeWarning, "ContextResolutionFailed",
			"Context resolution failed: %s", err)
		return ctrl.Result{}, controller.IgnoreHierarchyNotFoundError(err)
	}

	dpClient, err := r.getBPClient(ctx, buildCtx.Build)
	if err != nil {
		logger.Error(err, "Error in getting build plane client")
		return ctrl.Result{}, err
	}

	externalResourceHandlers := r.makeExternalResourceHandlers(dpClient)
	if err := r.reconcileExternalResources(ctx, externalResourceHandlers, buildCtx); err != nil {
		logger.Error(err, "Error reconciling external resources")
		r.recorder.Eventf(build, corev1.EventTypeWarning, "ExternalResourceReconciliationFailed",
			"External resource reconciliation failed: %s", err)
		return ctrl.Result{}, err
	}

	existingWorkflow, err := r.ensureWorkflow(ctx, buildCtx, dpClient)

	if err != nil {
		logger.Error(err, "Failed to ensure workflow")
		r.recorder.Eventf(build, corev1.EventTypeWarning, "WorkflowReconciliationFailed",
			"Build workflow reconciliation failed: %s", err)
		return ctrl.Result{}, err
	}

	if isBuildWorkflowRunning(build) {
		requeue := r.handleBuildSteps(build, existingWorkflow.Status.Nodes)

		if requeue {
			return r.handleRequeueAfterBuild(ctx, oldBuild, build)
		}

		// When ci workflow is completed, it is required to update conditions
		if oldBuild.Status.ImageStatus.Image != buildCtx.Build.Status.ImageStatus.Image ||
			controller.NeedConditionUpdate(oldBuild.Status.Conditions, buildCtx.Build.Status.Conditions) {
			if err := r.Status().Update(ctx, build); err != nil {
				logger.Error(err, "Failed to update build status")
				return ctrl.Result{}, err
			}
		}
	}

	if shouldCreateDeployableArtifact(buildCtx.Build) {
		requeue, err := r.createDeployableArtifact(ctx, buildCtx)
		if requeue {
			return controller.UpdateStatusConditionsAndRequeue(ctx, r.Client, oldBuild, build)
		}
		if err != nil {
			return controller.UpdateStatusConditionsAndReturn(ctx, r.Client, oldBuild, build)
		}
		meta.SetStatusCondition(&buildCtx.Build.Status.Conditions, NewDeployableArtifactCreatedCondition(buildCtx.Build.Generation))
		r.recorder.Event(build, corev1.EventTypeNormal, "DeployableArtifactReady", "Deployable artifact created")
		return controller.UpdateStatusConditionsAndRequeue(ctx, r.Client, oldBuild, build)
	}

	requeue, err := r.handleAutoDeployment(ctx, buildCtx)
	if requeue {
		return controller.UpdateStatusConditionsAndRequeue(ctx, r.Client, oldBuild, build)
	} else if err != nil {
		meta.SetStatusCondition(&buildCtx.Build.Status.Conditions, NewBuildFailedCondition(buildCtx.Build.Generation))
		return controller.UpdateStatusConditionsAndReturn(ctx, r.Client, oldBuild, build)
	}
	meta.SetStatusCondition(&buildCtx.Build.Status.Conditions, NewBuildCompletedCondition(buildCtx.Build.Generation))
	return controller.UpdateStatusConditionsAndReturn(ctx, r.Client, oldBuild, build)
}

// SetupWithManager sets up the controller with the Manager.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	if r.recorder == nil {
		r.recorder = mgr.GetEventRecorderFor("build-controller")
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&choreov1.Build{}).
		Named("build").
		Owns(&choreov1.DeployableArtifact{}).
		Complete(r)
}

func (r *Reconciler) retrieveEnvsOfPipeline(ctx context.Context, dp *choreov1.DeploymentPipeline) ([]*choreov1.Environment, error) {
	envNamesSet := make(map[string]struct{})

	for _, path := range dp.Spec.PromotionPaths {
		envNamesSet[path.SourceEnvironmentRef] = struct{}{}
		for _, target := range path.TargetEnvironmentRefs {
			envNamesSet[target.Name] = struct{}{}
		}
	}

	envs := make([]*choreov1.Environment, 0, len(envNamesSet))
	for name := range envNamesSet {
		env, err := controller.GetEnvironmentByName(ctx, r.Client, dp, name)
		if err != nil {
			return nil, err
		}
		envs = append(envs, env)
	}

	return envs, nil
}

func (r *Reconciler) retrieveDataplanes(ctx context.Context, envs []*choreov1.Environment) ([]*choreov1.DataPlane, error) {
	uniqueRefs := make(map[string]*choreov1.Environment)

	for _, env := range envs {
		if env == nil || env.Spec.DataPlaneRef == "" {
			continue
		}
		uniqueRefs[env.Spec.DataPlaneRef] = env
	}

	dataplanes := make([]*choreov1.DataPlane, 0, len(uniqueRefs))
	for _, env := range uniqueRefs {
		dp, err := controller.GetDataplaneOfEnv(ctx, r.Client, env)
		if err != nil {
			return nil, fmt.Errorf("failed to get DataPlane for environment %q: %w", env.Name, err)
		}
		dataplanes = append(dataplanes, dp)
	}

	return dataplanes, nil
}

func getRegistriesForPush(dataplanes []*choreov1.DataPlane) (map[string]string, []string) {
	registriesWithSecrets := make(map[string]string)
	noAuthRegistriesSet := make(map[string]struct{})

	for _, dp := range dataplanes {
		pushSecrets := dp.Spec.Registry.ImagePushSecrets
		noAuthRegistries := dp.Spec.Registry.Unauthenticated

		for _, pushSecret := range pushSecrets {
			registriesWithSecrets[pushSecret.Name] = pushSecret.Prefix
		}

		for _, registryPrefix := range noAuthRegistries {
			noAuthRegistriesSet[registryPrefix] = struct{}{}
		}
	}

	noAuthRegistriesList := make([]string, 0, len(noAuthRegistriesSet))
	for registry := range noAuthRegistriesSet {
		noAuthRegistriesList = append(noAuthRegistriesList, registry)
	}

	return registriesWithSecrets, noAuthRegistriesList
}

func convertToImagePushSecrets(registriesWithSecrets map[string]string) []choreov1.ImagePushSecret {
	imagePushSecrets := make([]choreov1.ImagePushSecret, 0, len(registriesWithSecrets))

	for name, prefix := range registriesWithSecrets {
		imagePushSecrets = append(imagePushSecrets, choreov1.ImagePushSecret{
			Name:   name,
			Prefix: prefix,
		})
	}

	return imagePushSecrets
}

func (r *Reconciler) makeBuildContext(ctx context.Context, build *choreov1.Build) (*integrations.BuildContext, error) {
	// makeBuildContext creates a build context for the given build by retrieving the
	// parent objects that this build is required to continue its work.
	component, err := controller.GetComponent(ctx, r.Client, build)
	if err != nil {
		return nil, fmt.Errorf("cannot retrieve the component: %w", err)
	}
	deploymentTrack, err := controller.GetDeploymentTrack(ctx, r.Client, build)
	if err != nil {
		return nil, fmt.Errorf("cannot retrieve the deployment track: %w", err)
	}

	project, err := controller.GetProject(ctx, r.Client, build)
	if err != nil {
		return nil, fmt.Errorf("cannot retrieve the project: %w", err)
	}
	deploymentPipeline, err := controller.GetDeploymentPipeline(ctx, r.Client, build, project.Spec.DeploymentPipelineRef)
	if err != nil {
		return nil, fmt.Errorf("cannot retrieve the deployment pipeline: %w", err)
	}
	envs, err := r.retrieveEnvsOfPipeline(ctx, deploymentPipeline)
	if err != nil {
		return nil, fmt.Errorf("cannot retrieve the environments of the pipeline: %w", err)
	}
	dataplanes, err := r.retrieveDataplanes(ctx, envs)
	if err != nil {
		return nil, fmt.Errorf("cannot retrieve the dataplanes: %w", err)
	}
	registriesWithSecrets, registries := getRegistriesForPush(dataplanes)
	imagePushSecrets := convertToImagePushSecrets(registriesWithSecrets)

	return &integrations.BuildContext{
		Registry: choreov1.Registry{
			ImagePushSecrets: imagePushSecrets,
			Unauthenticated:  registries,
		},
		Component:       component,
		DeploymentTrack: deploymentTrack,
		Build:           build,
	}, nil
}

func (r *Reconciler) getBPClient(ctx context.Context, build *choreov1.Build) (client.Client, error) {
	buildplane, err := r.getDataPlaneMarkedAsBuildPlane(ctx, r.Client, build)
	if err != nil {
		// Return an error if dataplane retrieval fails
		return nil, fmt.Errorf("failed to get build plane: %w", err)
	}

	dpClient, err := dpKubernetes.GetDPClient(r.DpClientMgr, buildplane)
	if err != nil {
		// Return an error if client creation fails
		return nil, fmt.Errorf("failed to get build plane client: %w", err)
	}

	return dpClient, nil
}

func (r *Reconciler) getDataPlaneMarkedAsBuildPlane(ctx context.Context, c client.Client, build *choreov1.Build) (*choreov1.DataPlane, error) {
	orgName := controller.GetOrganizationName(build)
	labelSelector := client.MatchingLabels{
		labels.LabelKeyOrganizationName: orgName,
		labels.LabelKeyBuildPlane:       "true",
	}

	var dataPlaneList choreov1.DataPlaneList
	if err := c.List(ctx, &dataPlaneList, client.InNamespace(build.GetNamespace()), labelSelector); err != nil {
		return nil, fmt.Errorf("failed to list dataplanes: %w", err)
	}

	count := len(dataPlaneList.Items)
	switch {
	case count == 0:
		r.recorder.Eventf(build, corev1.EventTypeWarning, "NoBuildPlaneFound",
			"No dataplane is configured as build plane for organization: %s", orgName)
		return nil, fmt.Errorf("no dataplane configured as build plane for organization: %s", orgName)

	case count > 1:
		r.recorder.Eventf(build, corev1.EventTypeWarning, "MultipleBuildPlanesFound",
			"Multiple dataplanes are configured as build planes for organization: %s", orgName)
		return nil, fmt.Errorf("multiple dataplanes configured as build planes for organization: %s", orgName)

	default:
		return &dataPlaneList.Items[0], nil
	}
}

// makeExternalResourceHandlers creates the chain of external resource handlers that are used to
// create the build namespace and other resources required for argo workflows.
func (r *Reconciler) makeExternalResourceHandlers(dpClient client.Client) []dataplane.ResourceHandler[integrations.BuildContext] {
	var handlers []dataplane.ResourceHandler[integrations.BuildContext]

	handlers = append(handlers, kubernetes.NewNamespaceHandler(dpClient))
	handlers = append(handlers, argointegrations.NewServiceAccountHandler(dpClient))
	handlers = append(handlers, argointegrations.NewRoleHandler(dpClient))
	handlers = append(handlers, argointegrations.NewRoleBindingHandler(dpClient))

	return handlers
}

// ReconcileResource handles the reconciliation logic for a single resource.
func (r *Reconciler) ReconcileResource(
	ctx context.Context,
	resourceHandler dataplane.ResourceHandler[integrations.BuildContext],
	buildCtx *integrations.BuildContext,
	logger logr.Logger) error {
	// Check if the resource exists
	currentState, err := resourceHandler.GetCurrentState(ctx, buildCtx)
	if err != nil {
		logger.Error(err, "Error retrieving current state of the resource")
		return err
	}

	exists := currentState != nil
	if !exists {
		// Create the resource if it does not exist
		if err := resourceHandler.Create(ctx, buildCtx); err != nil && !apierrors.IsAlreadyExists(err) {
			logger.Error(err, "Error creating the resource")
			return err
		}
	}
	// TODO: Test this flow
	// else {
	//	// Update the resource if it exists
	//	if err := resourceHandler.Update(ctx, buildCtx, currentState); err != nil {
	//		logger.Error(err, "Error updating resource")
	//		return err
	//	}
	// }
	return nil
}

// reconcileExternalResources reconciles the provided external resources based on the build context.
func (r *Reconciler) reconcileExternalResources(
	ctx context.Context,
	resourceHandlers []dataplane.ResourceHandler[integrations.BuildContext],
	buildCtx *integrations.BuildContext) error {
	handlerNameLogKey := "resourceHandler"
	for _, resourceHandler := range resourceHandlers {
		logger := log.FromContext(ctx).WithValues(handlerNameLogKey, resourceHandler.Name())

		if err := r.ReconcileResource(ctx, resourceHandler, buildCtx, logger); err != nil {
			logger.Error(err, "Error reconciling resource")
			return err
		}
	}

	return nil
}

func (r *Reconciler) ensureWorkflow(ctx context.Context, buildCtx *integrations.BuildContext, dpClient client.Client) (*argoproj.Workflow, error) {
	logger := log.FromContext(ctx).WithValues("workflowHandler", "Workflow")
	workflowHandler := argointegrations.NewWorkflowHandler(dpClient)
	existingWorkflow, err := workflowHandler.GetCurrentState(ctx, buildCtx)
	if err != nil {
		logger.Error(err, "Error retrieving current state of the workflow resource")
		return nil, err
	}

	exists := existingWorkflow != nil

	if !exists {
		// Create the external resource if it does not exist
		if err := workflowHandler.Create(ctx, buildCtx); err != nil {
			logger.Error(err, "Error creating workflow resource")
			return nil, err
		}
		r.recorder.Eventf(buildCtx.Build, corev1.EventTypeNormal, "NewWorkflowCreated",
			"New build workflow created: %s", buildCtx.Build.Name)
		existingWorkflow, err = workflowHandler.GetCurrentState(ctx, buildCtx)
		if err != nil {
			logger.Error(err, "Error retrieving current state of the workflow resource")
			return nil, err
		}
	}

	// Use the two-value form of type assertion for safety
	existing, ok := existingWorkflow.(argoproj.Workflow)
	if !ok {
		return nil, fmt.Errorf("could not convert workflow to expected type")
	}
	return &existing, nil
}

// shouldIgnoreReconcile checks whether the reconcile loop should be continued.
// Reconciliation should be avoided if the build is in a final state.
func shouldIgnoreReconcile(build *choreov1.Build) bool {
	if completedCondition := meta.FindStatusCondition(build.Status.Conditions, string(ConditionCompleted)); completedCondition != nil && completedCondition.Reason != string(ReasonBuildInProgress) {
		return true
	}
	return false
}

// shouldCreateDeployableArtifact represents whether the deployable artifact should be created.
// Deployable artifact should be created when the workflow is completed successfully and when the deployable artifact
// does not exist.
func shouldCreateDeployableArtifact(build *choreov1.Build) bool {
	return meta.FindStatusCondition(build.Status.Conditions, string(ConditionDeployableArtifactCreated)) == nil
}

func isBuildWorkflowRunning(build *choreov1.Build) bool {
	stepConditions := []controller.ConditionType{
		ConditionCloneStepSucceeded,
		ConditionBuildStepSucceeded,
		ConditionPushStepSucceeded,
	}

	for _, conditionType := range stepConditions {
		condition := meta.FindStatusCondition(build.Status.Conditions, string(conditionType))
		if condition == nil {
			continue
		}

		switch condition.Reason {
		case string(ReasonStepFailed):
			// A failed step means the workflow is not running
			return false
		case string(ReasonStepQueued), string(ReasonStepInProgress):
			// At least one step is running/scheduled to run
			return true
		}
	}

	return false
}

func isBuildStepRunning(build *choreov1.Build) bool {
	condition := meta.FindStatusCondition(build.Status.Conditions, string(ConditionBuildStepSucceeded))
	return condition != nil && condition.Reason == string(ReasonStepInProgress)
}

// handleRequeueAfterBuild manages the requeue process after a build step.
func (r *Reconciler) handleRequeueAfterBuild(ctx context.Context, old, build *choreov1.Build) (ctrl.Result, error) {
	// Check if the build step is running and has not yet succeeded.
	if isBuildStepRunning(build) {
		// Requeue after 20 seconds to provide a controlled interval instead of exponential backoff.
		return controller.UpdateStatusConditionsAndRequeueAfter(ctx, r.Client, old, build, 20*time.Second)
	}
	// Default requeue without a delay if the build step is not there or already succeeded.
	return controller.UpdateStatusConditionsAndRequeue(ctx, r.Client, old, build)
}

func (r *Reconciler) handleBuildSteps(build *choreov1.Build, nodes argoproj.Nodes) bool {
	steps := []struct {
		stepName      integrations.BuildWorkflowStep
		conditionType controller.ConditionType
	}{
		{integrations.CloneStep, ConditionCloneStepSucceeded},
		{integrations.BuildStep, ConditionBuildStepSucceeded},
		{integrations.PushStep, ConditionPushStepSucceeded},
	}

	for _, step := range steps {
		stepInfo, isFound := argointegrations.GetStepByTemplateName(nodes, step.stepName)
		if !isFound || meta.IsStatusConditionPresentAndEqual(build.Status.Conditions, string(step.conditionType), metav1.ConditionTrue) {
			continue
		}
		switch argointegrations.GetStepPhase(stepInfo.Phase) {
		case integrations.Running:
			markStepInProgress(build, step.conditionType)
			return true
		case integrations.Succeeded:
			markStepSucceeded(build, step.conditionType)
			r.recorder.Event(build, corev1.EventTypeNormal, string(step.conditionType), "Workflow step succeeded")
			isFinalStep := step.stepName == integrations.PushStep
			if isFinalStep {
				image := argointegrations.GetImageNameFromWorkflow(*stepInfo.Outputs)
				if image == "" {
					meta.SetStatusCondition(&build.Status.Conditions, NewImageMissingBuildFailedCondition(build.Generation))
				} else {
					build.Status.ImageStatus.Image = image
				}
				return false
			}
			return true
		case integrations.Failed:
			markStepFailed(build, step.conditionType)
			r.recorder.Event(build, corev1.EventTypeWarning, string(step.conditionType), "Workflow step failed")
			meta.SetStatusCondition(&build.Status.Conditions, NewBuildFailedCondition(build.Generation))
			return false
		}
	}
	return true
}

func (r *Reconciler) createDeployableArtifact(ctx context.Context, buildCtx *integrations.BuildContext) (bool, error) {
	deployableArtifact := resources.MakeDeployableArtifact(buildCtx.Build)

	if buildCtx.Component.Spec.Type == choreov1.ComponentTypeService {
		endpoints, err := r.getEndpointConfigs(ctx, buildCtx)
		if err != nil {
			return true, fmt.Errorf("error getting endpoint configs: %w", err)
		}
		resources.AddComponentSpecificConfigs(buildCtx, deployableArtifact, &endpoints)
	} else {
		resources.AddComponentSpecificConfigs(buildCtx, deployableArtifact, nil)
	}

	if err := ctrl.SetControllerReference(buildCtx.Build, deployableArtifact, r.Scheme); err != nil {
		return true, err
	}

	if err := r.Client.Create(ctx, deployableArtifact); err != nil && !apierrors.IsAlreadyExists(err) {
		r.recorder.Event(buildCtx.Build, corev1.EventTypeWarning, "DeployableArtifactCreationFailed", "Deployable artifact creation failed.")
		return true, fmt.Errorf("failed to create deployable artifact: %w", err)
	}
	return false, nil
}

func (r *Reconciler) handleAutoDeployment(ctx context.Context, buildCtx *integrations.BuildContext) (bool, error) {
	if buildCtx.DeploymentTrack.Spec.AutoDeploy && meta.IsStatusConditionPresentAndEqual(buildCtx.Build.Status.Conditions, string(ConditionDeployableArtifactCreated), metav1.ConditionTrue) {
		requeue, err := r.updateOrCreateDeployment(ctx, buildCtx)
		if requeue {
			return true, nil
		} else if err != nil {
			meta.SetStatusCondition(&buildCtx.Build.Status.Conditions, NewAutoDeploymentFailedCondition(buildCtx.Build.Generation))
			return false, err
		}
		meta.SetStatusCondition(&buildCtx.Build.Status.Conditions, NewAutoDeploymentSuccessfulCondition(buildCtx.Build.Generation))
	}
	return false, nil
}

func (r *Reconciler) updateOrCreateDeployment(ctx context.Context, buildCtx *integrations.BuildContext) (bool, error) {
	logger := log.FromContext(ctx)

	environment, err := r.getFirstEnvironmentFromDeploymentPipeline(ctx, buildCtx.Build)
	if err != nil {
		if apierrors.IsNotFound(err) {
			// Environment not found, no need to requeue
			return false, nil
		}
		// Other errors should be retried
		return true, err
	}

	// Retrieve the existing deployment
	deployment, err := controller.GetDeploymentByEnvironment(ctx, r.Client, buildCtx.Build, environment.Labels[labels.LabelKeyName])
	if err != nil {
		var hierarchyErr *controller.HierarchyNotFoundError
		if errors.As(err, &hierarchyErr) {
			// Deployment does not exist, create a new one
			deployment = resources.MakeDeployment(buildCtx, environment.Labels[labels.LabelKeyName])
			if err := r.Client.Create(ctx, deployment); err != nil {
				logger.Error(err, "Failed to create deployment", "Build.name", buildCtx.Build.Name)
				return true, err
			}
			logger.Info("Created deployment", "Build.name", buildCtx.Build.Name)
			r.recorder.Event(buildCtx.Build, corev1.EventTypeNormal, "DeploymentReady", "Deployment created")
			return false, nil
		}
		// Return if the error is not a "Not Found" error
		logger.Error(err, "Failed to get deployment", "Build.name", buildCtx.Build.Name)
		return true, err
	}

	// If deployment exists, update the DeploymentArtifactRef if necessary
	if deployment.Spec.DeploymentArtifactRef != buildCtx.Build.Name {
		deployment.Spec.DeploymentArtifactRef = buildCtx.Build.Name
		if err = r.Update(ctx, deployment); err != nil {
			logger.Error(err, "Failed to update deployment", "Deployment.name", deployment.Name)
			return true, err
		}
		r.recorder.Event(buildCtx.Build, corev1.EventTypeNormal, "DeploymentReady", "Deployment updated")
	}

	// No further reconciliation needed
	return false, nil
}

func (r *Reconciler) getFirstEnvironmentFromDeploymentPipeline(ctx context.Context, build *choreov1.Build) (*choreov1.Environment, error) {
	// Get the deployment pipeline of the project
	deploymentPipeline, err := r.getDeploymentPipelineOfProject(ctx, r.Client, build)
	if err != nil {
		return nil, err
	}

	// Get the environment name from the first promotion path
	environmentName := deploymentPipeline.Spec.PromotionPaths[0].SourceEnvironmentRef

	// Retrieve the environment by name
	environment, err := controller.GetEnvironmentByName(ctx, r.Client, build, environmentName)
	if err != nil {
		return nil, err
	}
	return environment, nil
}

func (r *Reconciler) getDeploymentPipelineOfProject(ctx context.Context, c client.Client, obj client.Object) (*choreov1.DeploymentPipeline, error) {
	project, err := controller.GetProject(ctx, c, obj)
	if err != nil {
		return nil, err
	}

	dp, err := controller.GetDeploymentPipeline(ctx, c, obj, project.Spec.DeploymentPipelineRef)
	if err != nil {
		return nil, err
	}

	return dp, nil
}

func (r *Reconciler) fetchComponentConfigs(ctx context.Context, buildCtx *integrations.BuildContext) (*source.Config, error) {
	logger := log.FromContext(ctx)
	sourceHandler := sourcegithub.NewGithubHandler(r.GithubClient)
	config, err := sourceHandler.FetchComponentDescriptor(ctx, buildCtx)
	if err != nil {
		logger.Error(err, "Failed to fetch component descriptor")
		r.recorder.Eventf(buildCtx.Build, corev1.EventTypeWarning, "RetrievingComponentDescriptorFailed", "Retrieving component descriptor failed: %s", err)
		return nil, fmt.Errorf("failed to get component.yaml from the repository buildName:%s;%w", buildCtx.Build.Name, err)
	}
	return config, nil
}

func (r *Reconciler) getEndpointConfigs(ctx context.Context, buildCtx *integrations.BuildContext) ([]choreov1.EndpointTemplate, error) {
	config, err := r.fetchComponentConfigs(ctx, buildCtx)
	if err != nil {
		return nil, err
	}
	endpointTemplates := []choreov1.EndpointTemplate{}
	for _, endpoint := range config.Endpoints {
		endpointTemplates = append(endpointTemplates, createEndpointTemplate(endpoint))
	}

	return endpointTemplates, nil
}

func createEndpointTemplate(endpoint source.Endpoint) choreov1.EndpointTemplate {
	basePath := endpoint.Service.BasePath
	if basePath == "" {
		basePath = "/"
	}
	return choreov1.EndpointTemplate{
		Spec: choreov1.EndpointSpec{
			Type:                endpoint.Type,
			NetworkVisibilities: parseNetworkVisibilities(endpoint.NetworkVisibilities),
			BackendRef: choreov1.BackendRef{
				BasePath: basePath,
				Type:     choreov1.BackendRefTypeComponentRef,
				ComponentRef: &choreov1.ComponentRef{
					Port: endpoint.Service.Port,
				},
			},
		},
	}
}

func parseNetworkVisibilities(visibilities []source.NetworkVisibilityLevel) *choreov1.NetworkVisibility {
	nv := choreov1.NetworkVisibility{}

	for _, visibility := range visibilities {
		switch visibility {
		case source.NetworkVisibilityLevelOrganization:
			nv.Organization = &choreov1.VisibilityConfig{Enable: true}
		case source.NetworkVisibilityLevelPublic:
			nv.Public = &choreov1.VisibilityConfig{Enable: true}
		}
	}

	return &nv
}
