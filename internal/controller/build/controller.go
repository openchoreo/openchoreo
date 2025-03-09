/*
 * Copyright (c) 2025, WSO2 Inc. (http://www.wso2.org) All Rights Reserved.
 *
 * WSO2 Inc. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License.
 * You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied. See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

package build

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	"github.com/google/go-github/v69/github"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	choreov1 "github.com/choreo-idp/choreo/api/v1"
	"github.com/choreo-idp/choreo/internal/controller"
	"github.com/choreo-idp/choreo/internal/controller/build/integrations/kubernetes"
	"github.com/choreo-idp/choreo/internal/controller/build/integrations/source"
	"github.com/choreo-idp/choreo/internal/controller/build/resources"
	argoproj "github.com/choreo-idp/choreo/internal/dataplane/kubernetes/types/argoproj.io/workflow/v1alpha1"
	"github.com/choreo-idp/choreo/internal/labels"
)

// Reconciler reconciles a Build object
type Reconciler struct {
	client.Client
	Scheme       *runtime.Scheme
	GithubClient *github.Client
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

	if r.shouldIgnoreReconcile(build) {
		return ctrl.Result{}, nil
	}

	oldBuild := build.DeepCopy()

	// Create a new build context for the build with required hierarchy objects
	buildCtx, err := r.makeBuildContext(ctx, build)
	if err != nil {
		logger.Error(err, "Error creating build context")
		return ctrl.Result{}, controller.IgnoreHierarchyNotFoundError(err)
	}

	externalResourceHandlers := r.makeExternalResourceHandlers()
	if err := r.reconcileExternalResources(ctx, externalResourceHandlers, buildCtx); err != nil {
		logger.Error(err, "Error reconciling external resources")
		return ctrl.Result{}, err
	}

	existingWorkflow, err := r.ensureWorkflow(ctx, buildCtx)

	if err != nil {
		logger.Error(err, "Failed to ensure workflow")
		return ctrl.Result{}, err
	}

	// If a new workflow was created, update status and requeue
	if existingWorkflow == nil {
		return controller.UpdateStatusConditionsAndRequeue(ctx, r.Client, oldBuild, build)
	}

	if meta.FindStatusCondition(buildCtx.Build.Status.Conditions, string(ConditionCompleted)) == nil {
		requeue := r.handleBuildSteps(build, existingWorkflow.Status.Nodes)

		if requeue {
			return r.handleRequeueAfterBuild(ctx, oldBuild, build, existingWorkflow)
		}

		// When build is completed, it is required to update conditions
		if oldBuild.Status.ImageStatus.Image != buildCtx.Build.Status.ImageStatus.Image ||
			controller.NeedConditionUpdate(oldBuild.Status.Conditions, buildCtx.Build.Status.Conditions) {
			if err := r.Status().Update(ctx, build); err != nil {
				logger.Error(err, "Failed to update build status")
				return ctrl.Result{Requeue: true}, err
			}
		}
	}

	if r.shouldCreateDeployableArtifact(buildCtx.Build) {
		requeue, err := r.createDeployableArtifact(ctx, buildCtx)
		if requeue {
			return controller.UpdateStatusConditionsAndRequeue(ctx, r.Client, oldBuild, build)
		}
		if err != nil {
			return controller.UpdateStatusConditionsAndReturn(ctx, r.Client, oldBuild, build)
		}
		meta.SetStatusCondition(&buildCtx.Build.Status.Conditions, NewDeployableArtifactCreatedCondition(buildCtx.Build.Generation))
		return controller.UpdateStatusConditionsAndRequeue(ctx, r.Client, oldBuild, build)
	}

	requeue, err := r.handleAutoDeployment(ctx, buildCtx)
	if requeue {
		return controller.UpdateStatusConditionsAndRequeue(ctx, r.Client, oldBuild, build)
	} else if err != nil {
		return controller.UpdateStatusConditionsAndReturn(ctx, r.Client, oldBuild, build)
	}

	return controller.UpdateStatusConditionsAndReturn(ctx, r.Client, oldBuild, build)
}

// SetupWithManager sets up the controller with the Manager.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&choreov1.Build{}).
		Named("build").
		Complete(r)
}

func (r *Reconciler) makeBuildContext(ctx context.Context, build *choreov1.Build) (*common.BuildContext, error) {
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
	return &common.BuildContext{
		Component:       component,
		DeploymentTrack: deploymentTrack,
		Build:           build,
	}, nil
}

// makeExternalResourceHandlers creates the chain of external resource handlers that are used to
// create the build namespace and other resources required for argo workflows.
func (r *Reconciler) makeExternalResourceHandlers() []common.ResourceHandler[common.BuildContext] {
	var handlers []common.ResourceHandler[common.BuildContext]

	handlers = append(handlers, kubernetes.NewNamespaceHandler(r.Client))
	handlers = append(handlers, argointegrations.NewServiceAccountHandler(r.Client))
	handlers = append(handlers, argointegrations.NewRoleHandler(r.Client))
	handlers = append(handlers, argointegrations.NewRoleBindingHandler(r.Client))

	return handlers
}

// ReconcileResource handles the reconciliation logic for a single resource.
func (r *Reconciler) ReconcileResource(
	ctx context.Context,
	resourceHandler common.ResourceHandler[common.BuildContext],
	buildCtx *common.BuildContext,
	logger logr.Logger) error {
	// Check if the resource exists
	currentState, err := resourceHandler.Get(ctx, buildCtx)
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
	resourceHandlers []common.ResourceHandler[common.BuildContext],
	buildCtx *common.BuildContext) error {
	handlerNameLogKey := "resourceHandler"
	for _, resourceHandler := range resourceHandlers {
		resourceName := resourceHandler.KindName() + " - " + resourceHandler.Name(ctx, buildCtx)
		logger := log.FromContext(ctx).WithValues(handlerNameLogKey, resourceName)

		if err := r.ReconcileResource(ctx, resourceHandler, buildCtx, logger); err != nil {
			logger.Error(err, "Error reconciling resource")
			return err
		}
	}

	return nil
}

func (r *Reconciler) ensureWorkflow(ctx context.Context, buildCtx *common.BuildContext) (*argoproj.Workflow, error) {
	logger := log.FromContext(ctx).WithValues("workflowHandler", "Workflow")
	workflowHandler := argointegrations.NewWorkflowHandler(r.Client)
	existingWorkflow, err := workflowHandler.Get(ctx, buildCtx)
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
		meta.SetStatusCondition(&buildCtx.Build.Status.Conditions, NewWorkflowInitializedCondition(buildCtx.Build.Generation))
		return nil, nil
	}
	existing := existingWorkflow.(argoproj.Workflow)
	return &existing, nil
}

// shouldIgnoreReconcile checks whether the reconcile loop should be continued.
// Reconciliation should be avoided if the build is in a final state.
func (r *Reconciler) shouldIgnoreReconcile(build *choreov1.Build) bool {
	return meta.FindStatusCondition(build.Status.Conditions, string(ConditionDeploymentApplied)) != nil ||
		meta.IsStatusConditionPresentAndEqual(build.Status.Conditions, string(ConditionCompleted), metav1.ConditionFalse)
}

// shouldCreateDeployableArtifact represents whether the deployable artifact should be created.
// Deployable artifact should be created when the workflow is completed successfully and when the deployable artifact
// does not exist.
func (r *Reconciler) shouldCreateDeployableArtifact(build *choreov1.Build) bool {
	return meta.IsStatusConditionPresentAndEqual(build.Status.Conditions, string(ConditionCompleted), metav1.ConditionTrue) &&
		meta.FindStatusCondition(build.Status.Conditions, string(ConditionDeployableArtifactCreated)) == nil
}

func (r *Reconciler) handleRequeueAfterBuild(
	ctx context.Context, old, build *choreov1.Build, workflow *argoproj.Workflow,
) (ctrl.Result, error) {
	// If the build step is still running, requeue the reconciliation after 1 minute.
	// This provides a controlled requeue interval instead of relying on exponential backoff.
	stepInfo, isFound := argointegrations.GetStepByTemplateName(workflow.Status.Nodes, common.BuildStep)
	if isFound && meta.FindStatusCondition(build.Status.Conditions, string(ConditionBuildSucceeded)) == nil {
		if argointegrations.GetStepPhase(stepInfo.Phase) == common.Running {
			return controller.UpdateStatusConditionsAndRequeueAfter(ctx, r.Client, old, build, 20*time.Second)
		}
	}
	return controller.UpdateStatusConditionsAndRequeue(ctx, r.Client, old, build)
}

func (r *Reconciler) handleBuildSteps(build *choreov1.Build, nodes argoproj.Nodes) bool {
	steps := []struct {
		stepName      common.BuildWorkflowStep
		conditionType controller.ConditionType
	}{
		{common.CloneStep, ConditionCloneSucceeded},
		{common.BuildStep, ConditionBuildSucceeded},
		{common.PushStep, ConditionPushSucceeded},
	}

	for _, step := range steps {
		stepInfo, isFound := argointegrations.GetStepByTemplateName(nodes, step.stepName)
		if !isFound || meta.FindStatusCondition(build.Status.Conditions, string(step.conditionType)) != nil {
			continue
		}
		switch argointegrations.GetStepPhase(stepInfo.Phase) {
		case common.Running:
			return true
		case common.Succeeded:
			r.markStepAsSucceeded(build, step.conditionType)
			isFinalStep := step.stepName == common.PushStep
			if isFinalStep {
				r.markWorkflowCompleted(build, stepInfo.Outputs)
				return false
			}
			return true
		case common.Failed:
			r.markStepAsFailed(build, step.conditionType)
			return false
		}
	}
	return true
}

func (r *Reconciler) createDeployableArtifact(ctx context.Context, buildCtx *common.BuildContext) (bool, error) {
	deployableArtifactHandler := resources.NewDeployableArtifactHandler(r.Client)

	if buildCtx.Component.Spec.Type == choreov1.ComponentTypeService {
		endpoints, err := r.getEndpointConfigs(ctx, buildCtx)
		if err != nil {
			return true, fmt.Errorf("error getting endpoint configs: %w", err)
		}

		// Build Context: set endpoints
		buildCtx.Endpoints = &endpoints
	}

	if err := deployableArtifactHandler.Create(ctx, buildCtx); err != nil && !apierrors.IsAlreadyExists(err) {
		return true, fmt.Errorf("failed to create deployable artifact: %w", err)
	}
	return false, nil
}

func (r *Reconciler) handleAutoDeployment(ctx context.Context, buildCtx *common.BuildContext) (bool, error) {
	if buildCtx.DeploymentTrack.Spec.AutoDeploy &&
		meta.IsStatusConditionPresentAndEqual(buildCtx.Build.Status.Conditions, string(ConditionDeployableArtifactCreated), metav1.ConditionTrue) {
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

func (r *Reconciler) updateOrCreateDeployment(ctx context.Context, buildCtx *common.BuildContext) (bool, error) {
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

	// Build Context: set initial environment of the deployment pipeline
	buildCtx.InitialEnvironment = environment

	deploymentHandler := resources.NewDeploymentHandler(r.Client)

	// Retrieve the existing deployment
	deployment, err := controller.GetDeploymentByEnvironment(ctx, r.Client, buildCtx.Build, environment.Labels[labels.LabelKeyName])
	if err != nil {
		var hierarchyErr *controller.HierarchyNotFoundError
		if errors.As(err, &hierarchyErr) {
			// Deployment does not exist, create a new one
			if err := deploymentHandler.Create(ctx, buildCtx); err != nil {
				logger.Error(err, "Failed to create deployment", "Build.name", buildCtx.Build.Name)
				return true, err
			}
			logger.Info("Created deployment", "Build.name", buildCtx.Build.Name)
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

func (r *Reconciler) fetchComponentConfigs(ctx context.Context, buildCtx *common.BuildContext) (*source.Config, error) {
	logger := log.FromContext(ctx)
	sourceHandler := sourcegithub.NewGithubHandler(r.GithubClient)
	config, err := sourceHandler.FetchComponentDescriptor(ctx, buildCtx)
	if err != nil {
		logger.Error(err, "Failed to fetch component descriptor")
		return nil, fmt.Errorf("failed to get component.yaml from the repository buildName:%s;%w", buildCtx.Build.Name, err)
	}
	return config.(*source.Config), nil
}

func (r *Reconciler) getEndpointConfigs(ctx context.Context, buildCtx *common.BuildContext) ([]choreov1.EndpointTemplate, error) {
	config, err := r.fetchComponentConfigs(ctx, buildCtx)
	if err != nil {
		return nil, err
	}
	endpointTemplates := []choreov1.EndpointTemplate{}
	for _, endpoint := range config.Endpoints {
		basePath := endpoint.Service.BasePath
		if basePath == "" {
			basePath = "/"
		}
		endpointTemplates = append(endpointTemplates, choreov1.EndpointTemplate{
			Spec: choreov1.EndpointSpec{
				Type:                endpoint.Type,
				NetworkVisibilities: endpoint.NetworkVisibilities,
				Service: choreov1.EndpointServiceSpec{
					Port:     endpoint.Service.Port,
					BasePath: basePath,
				},
			},
		})
	}
	return endpointTemplates, nil
}
