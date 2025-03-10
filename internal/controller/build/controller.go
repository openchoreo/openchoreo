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
	"path"
	"strings"
	"time"

	"github.com/go-logr/logr"
	"github.com/google/go-github/v69/github"
	"gopkg.in/yaml.v3"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	choreov1 "github.com/choreo-idp/choreo/api/v1"
	"github.com/choreo-idp/choreo/internal/controller"
	"github.com/choreo-idp/choreo/internal/controller/build/descriptor"
	dpkubernetes "github.com/choreo-idp/choreo/internal/dataplane/kubernetes"
	argo "github.com/choreo-idp/choreo/internal/dataplane/kubernetes/types/argoproj.io/workflow/v1alpha1"
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

	buildNamespace := "choreo-ci-" + build.Labels[labels.LabelKeyOrganizationName]
	if err := r.ensureNamespaceResources(ctx, buildNamespace, logger); err != nil {
		logger.Error(err, "Failed to ensure choreo ci namespace resources")
		return ctrl.Result{}, err
	}

	existingWorkflow, err := r.ensureWorkflow(ctx, build, buildNamespace, logger)

	if err != nil {
		logger.Error(err, "Failed to ensure workflow")
		return ctrl.Result{}, err
	}

	// If a new workflow was created, update status and requeue
	if existingWorkflow == nil {
		return controller.UpdateStatusConditionsAndRequeue(ctx, r.Client, oldBuild, build)
	}

	if meta.FindStatusCondition(build.Status.Conditions, string(Completed)) == nil {
		requeue := r.handleBuildSteps(build, existingWorkflow.Status.Nodes)

		if requeue {
			return r.handleRequeueAfterBuild(ctx, oldBuild, build, existingWorkflow)
		}

		// When build is completed, it is required to update conditions
		if oldBuild.Status.ImageStatus.Image != build.Status.ImageStatus.Image ||
			controller.NeedConditionUpdate(oldBuild.Status.Conditions, build.Status.Conditions) {
			if err := r.Status().Update(ctx, build); err != nil {
				logger.Error(err, "Failed to update build status")
				return ctrl.Result{Requeue: true}, err
			}
		}
	}

	if r.shouldCreateDeployableArtifact(build) {
		requeue, err := r.createDeployableArtifact(ctx, build)
		if requeue {
			return controller.UpdateStatusConditionsAndRequeue(ctx, r.Client, oldBuild, build)
		}
		if err != nil {
			return controller.UpdateStatusConditionsAndReturn(ctx, r.Client, oldBuild, build)
		}
		meta.SetStatusCondition(&build.Status.Conditions, controller.NewCondition(
			DeployableArtifactCreated,
			metav1.ConditionTrue,
			"ArtifactCreationSuccessful",
			"Successfully created a deployable artifact for the build",
			build.Generation,
		))
		return controller.UpdateStatusConditionsAndRequeue(ctx, r.Client, oldBuild, build)
	}

	requeue, err := r.handleAutoDeployment(ctx, build)
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

func (r *Reconciler) shouldIgnoreReconcile(build *choreov1.Build) bool {
	return meta.FindStatusCondition(build.Status.Conditions, string(DeploymentApplied)) != nil ||
		meta.IsStatusConditionPresentAndEqual(build.Status.Conditions, string(Completed), metav1.ConditionFalse)
}

func (r *Reconciler) handleRequeueAfterBuild(
	ctx context.Context, old, build *choreov1.Build, workflow *argo.Workflow,
) (ctrl.Result, error) {
	// If the build step is still running, requeue the reconciliation after 1 minute.
	// This provides a controlled requeue interval instead of relying on exponential backoff.
	stepInfo, isFound := GetStepByTemplateName(workflow.Status.Nodes, BuildStep)
	if isFound && meta.FindStatusCondition(build.Status.Conditions, string(BuildSucceeded)) == nil {
		if getStepPhase(stepInfo.Phase) == Running {
			return controller.UpdateStatusConditionsAndRequeueAfter(ctx, r.Client, old, build, 20*time.Second)
		}
	}
	return controller.UpdateStatusConditionsAndRequeue(ctx, r.Client, old, build)
}

func (r *Reconciler) shouldCreateDeployableArtifact(build *choreov1.Build) bool {
	return meta.IsStatusConditionPresentAndEqual(build.Status.Conditions, string(Completed), metav1.ConditionTrue) &&
		meta.FindStatusCondition(build.Status.Conditions, string(DeployableArtifactCreated)) == nil
}

func (r *Reconciler) handleAutoDeployment(ctx context.Context, build *choreov1.Build) (bool, error) {
	logger := log.FromContext(ctx)
	dt, err := controller.GetDeploymentTrack(ctx, r.Client, build)
	if apierrors.IsNotFound(err) {
		logger.Error(err, "Deployment resource not found")
		meta.SetStatusCondition(&build.Status.Conditions, controller.NewCondition(
			DeploymentApplied,
			metav1.ConditionFalse,
			"DeploymentFailed",
			"Deployment configuration failed.",
			build.Generation,
		))
		return false, nil
	} else if err != nil {
		return true, err
	}

	if dt.Spec.AutoDeploy && meta.IsStatusConditionPresentAndEqual(build.Status.Conditions, string(DeployableArtifactCreated), metav1.ConditionTrue) {
		requeue, err := r.updateOrCreateDeployment(ctx, build)
		if requeue {
			return true, nil
		} else if err != nil {
			meta.SetStatusCondition(&build.Status.Conditions, controller.NewCondition(
				DeploymentApplied,
				metav1.ConditionFalse,
				"DeploymentFailed",
				"Deployment configuration failed.",
				build.Generation,
			))
			return false, err
		}
		meta.SetStatusCondition(&build.Status.Conditions, controller.NewCondition(
			DeploymentApplied,
			metav1.ConditionTrue,
			"DeploymentAppliedSuccessfully",
			"Successfully configured the deployment.",
			build.Generation,
		))
	}
	return false, nil
}

// ensureNamespaceResources ensures that the namespace, service account, role, and role binding are created.
func (r *Reconciler) ensureNamespaceResources(ctx context.Context, namespaceName string, logger logr.Logger) error {
	// Step 1: Create Namespace if it doesn't exist
	namespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: namespaceName,
		},
	}
	if err := r.Client.Create(ctx, namespace); err != nil && !apierrors.IsAlreadyExists(err) {
		logger.Error(err, "Failed to create choreo ci namespace", "Namespace", namespaceName)
		return err
	}

	// Step 2: Create ServiceAccount if it doesn't exist
	sa := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "argo-workflow-sa",
			Namespace: namespaceName,
		},
	}
	if err := r.Client.Create(ctx, sa); err != nil && !apierrors.IsAlreadyExists(err) {
		logger.Error(err, "Failed to create ServiceAccount", "Namespace", namespaceName)
		return err
	}

	// Step 3: Create Role if it doesn't exist
	role := &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "argo-workflow-role",
			Namespace: namespaceName,
		},
		Rules: []rbacv1.PolicyRule{
			{
				APIGroups: []string{"argoproj.io"},
				Resources: []string{"workflowtaskresults"},
				Verbs:     []string{"create", "get", "list", "watch", "update", "patch"},
			},
		},
	}
	if err := r.Client.Create(ctx, role); err != nil && !apierrors.IsAlreadyExists(err) {
		logger.Error(err, "Failed to create Role", "Namespace", namespaceName)
		return err
	}

	// Step 4: Create RoleBinding if it doesn't exist
	roleBinding := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "argo-workflow-role-binding",
			Namespace: namespaceName,
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      "argo-workflow-sa",
				Namespace: namespaceName,
			},
		},
		RoleRef: rbacv1.RoleRef{
			Kind:     "Role",
			Name:     "argo-workflow-role",
			APIGroup: "rbac.authorization.k8s.io",
		},
	}
	if err := r.Client.Create(ctx, roleBinding); err != nil && !apierrors.IsAlreadyExists(err) {
		logger.Error(err, "Failed to create RoleBinding", "Namespace", namespaceName)
		return err
	}
	return nil
}

func (r *Reconciler) ensureWorkflow(ctx context.Context, build *choreov1.Build, buildNamespace string, logger logr.Logger) (*argo.Workflow, error) {
	component, err := controller.GetComponent(ctx, r.Client, build)
	if err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("Component of the build is not found", "Build.Name", build.Name)
			return nil, err
		}
		logger.Info("Error occurred while retrieving the component of the build", "Build.Name", build.Name)
		return nil, err
	}
	existingWorkflow := argo.Workflow{}
	err = r.Get(ctx, client.ObjectKey{Name: dpkubernetes.GenerateK8sNameWithLengthLimit(63, build.ObjectMeta.Name), Namespace: buildNamespace}, &existingWorkflow)
	if err != nil {
		// Create the workflow
		if apierrors.IsNotFound(err) {
			workflow := makeArgoWorkflow(build, component.Spec.Source.GitRepository.URL, buildNamespace)

			if err := r.Create(ctx, workflow); err != nil {
				return nil, err
			}
			meta.SetStatusCondition(&build.Status.Conditions, controller.NewCondition(
				Initialized,
				metav1.ConditionTrue,
				"WorkflowCreated",
				"Workflow was created in the cluster",
				build.Generation,
			))
			return nil, nil
		}
		return nil, err
	}
	return &existingWorkflow, nil
}

func (r *Reconciler) handleBuildSteps(build *choreov1.Build, nodes argo.Nodes) bool {
	steps := []struct {
		stepName      WorkflowStep
		conditionType controller.ConditionType
	}{
		{CloneStep, CloneSucceeded},
		{BuildStep, BuildSucceeded},
		{PushStep, PushSucceeded},
	}

	stepInfo, isFound := GetStepByTemplateName(nodes, steps[0].stepName)
	if isFound && meta.FindStatusCondition(build.Status.Conditions, string(steps[0].conditionType)) == nil {
		switch getStepPhase(stepInfo.Phase) {
		case Running:
			return true
		case Succeeded:
			r.markStepAsSucceeded(build, steps[0].conditionType)
			return true
		case Failed:
			r.markStepAsFailed(build, steps[0].conditionType)
			return false
		}
	}

	stepInfo, isFound = GetStepByTemplateName(nodes, steps[1].stepName)
	if isFound && meta.FindStatusCondition(build.Status.Conditions, string(steps[1].conditionType)) == nil {
		switch getStepPhase(stepInfo.Phase) {
		case Running:
			return true
		case Succeeded:
			r.markStepAsSucceeded(build, steps[1].conditionType)
			return true
		case Failed:
			r.markStepAsFailed(build, steps[1].conditionType)
			return false
		}
	}

	stepInfo, isFound = GetStepByTemplateName(nodes, steps[2].stepName)
	if isFound && meta.FindStatusCondition(build.Status.Conditions, string(steps[2].conditionType)) == nil {
		switch getStepPhase(stepInfo.Phase) {
		case Running:
			return true
		case Succeeded:
			r.markStepAsSucceeded(build, steps[2].conditionType)
			r.markWorkflowCompleted(build, stepInfo.Outputs)
			return false
		case Failed:
			r.markStepAsFailed(build, steps[2].conditionType)
			return false
		}
	}
	return true
}

func (r *Reconciler) markWorkflowCompleted(build *choreov1.Build, argoPushStepOutput *argo.Outputs) {
	newCondition := metav1.Condition{
		Type:               string(Completed),
		Status:             metav1.ConditionTrue,
		LastTransitionTime: metav1.Now(),
		Reason:             "BuildCompleted",
		Message:            "Build completed successfully.",
	}
	image := getImageNameFromWorkflow(*argoPushStepOutput)
	if image == "" {
		newCondition.Status = metav1.ConditionFalse
		newCondition.Reason = "BuildFailed"
		newCondition.Message = "Image name is not found in the workflow"
	} else {
		build.Status.ImageStatus.Image = image
	}
	meta.SetStatusCondition(&build.Status.Conditions, newCondition)
}

func (r *Reconciler) markStepAsSucceeded(build *choreov1.Build, conditionType controller.ConditionType) {
	successDescriptiors := map[controller.ConditionType]struct {
		Reason  controller.ConditionReason
		Message string
	}{
		CloneSucceeded: {
			Reason:  "CloneSourceCodeSucceeded",
			Message: "Source code cloning was successful.",
		},
		BuildSucceeded: {
			Reason:  "BuildImageSucceeded",
			Message: "Building the source code was successful.",
		},
		PushSucceeded: {
			Reason:  "PushImageSucceeded",
			Message: "Pushing the built image to the registry was successful.",
		},
	}

	meta.SetStatusCondition(&build.Status.Conditions, controller.NewCondition(
		conditionType,
		metav1.ConditionTrue,
		successDescriptiors[conditionType].Reason,
		successDescriptiors[conditionType].Message,
		build.Generation,
	))
}

func (r *Reconciler) markStepAsFailed(build *choreov1.Build, conditionType controller.ConditionType) {
	failureDescriptors := map[controller.ConditionType]struct {
		Reason  controller.ConditionReason
		Message string
	}{
		CloneSucceeded: {
			Reason:  "CloneSourceCodeFailed",
			Message: "Source code cloning failed.",
		},
		BuildSucceeded: {
			Reason:  "BuildImageFailed",
			Message: "Building the source code failed.",
		},
		PushSucceeded: {
			Reason:  "PushImageFailed",
			Message: "Pushing the built image to the registry failed.",
		},
	}

	meta.SetStatusCondition(&build.Status.Conditions, controller.NewCondition(
		conditionType,
		metav1.ConditionTrue,
		failureDescriptors[conditionType].Reason,
		failureDescriptors[conditionType].Message,
		build.Generation,
	))

	meta.SetStatusCondition(&build.Status.Conditions, controller.NewCondition(
		Completed,
		metav1.ConditionFalse,
		"BuildFailed",
		"Build completed with a failure status",
		build.Generation,
	))
}

func getImageNameFromWorkflow(output argo.Outputs) string {
	for _, param := range output.Parameters {
		if param.Name == "image" {
			return *param.Value
		}
	}
	return ""
}

// This doesn't include git revision. It is added from the workflow.
func constructImageNameWithTag(build *choreov1.Build) string {
	componentName := build.ObjectMeta.Labels["core.choreo.dev/component"]
	orgName := build.ObjectMeta.Labels["core.choreo.dev/organization"]
	projName := build.ObjectMeta.Labels["core.choreo.dev/project"]
	dtName := build.ObjectMeta.Labels["core.choreo.dev/deployment-track"]

	// To prevent excessively long image names, we limit them to 128 characters for the name and 128 characters for the tag.
	imageName := dpkubernetes.GenerateK8sNameWithLengthLimit(128, orgName, projName, componentName)
	// The maximum recommended tag length is 128 characters, with 8 characters reserved for the commit SHA.
	return fmt.Sprintf(
		"%s:%s",
		imageName,
		dpkubernetes.GenerateK8sNameWithLengthLimit(119, dtName),
	)
}

func (r *Reconciler) createDeployableArtifact(ctx context.Context, build *choreov1.Build) (bool, error) {
	logger := log.FromContext(ctx)
	deployableArtifact := &choreov1.DeployableArtifact{
		TypeMeta: metav1.TypeMeta{
			Kind:       "DeployableArtifact",
			APIVersion: "core.choreo.dev/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      build.ObjectMeta.Name,
			Namespace: build.ObjectMeta.Namespace,
			Annotations: map[string]string{
				"core.choreo.dev/display-name": build.ObjectMeta.Name,
				"core.choreo.dev/description":  "Deployable Artifact was created by the build.",
			},
			Labels: map[string]string{
				"core.choreo.dev/name":             build.ObjectMeta.Name,
				"core.choreo.dev/build":            build.ObjectMeta.Name,
				"core.choreo.dev/deployment-track": build.ObjectMeta.Labels["core.choreo.dev/deployment-track"],
				"core.choreo.dev/component":        build.ObjectMeta.Labels["core.choreo.dev/component"],
				"core.choreo.dev/project":          build.ObjectMeta.Labels["core.choreo.dev/project"],
				"core.choreo.dev/organization":     build.ObjectMeta.Labels["core.choreo.dev/organization"],
			},
		},
		Spec: choreov1.DeployableArtifactSpec{
			TargetArtifact: choreov1.TargetArtifact{
				FromBuildRef: &choreov1.FromBuildRef{
					Name: build.ObjectMeta.Name,
				},
			},
		},
	}
	// We only create the deployable artifact if it doesn't exist
	existing := &choreov1.DeployableArtifact{}
	err := r.Get(ctx, client.ObjectKeyFromObject(deployableArtifact), existing)
	if err == nil {
		return false, nil
	} else if !apierrors.IsNotFound(err) {
		logger.Error(err, "failed to get deployable artifact")
		return true, fmt.Errorf("failed to get deployable artifact: %w", err)
	}
	if err := ctrl.SetControllerReference(build, deployableArtifact, r.Scheme); err != nil {
		logger.Error(err, "Failed to set controller reference", "DeployableArtifact", deployableArtifact.Labels)
		return false, err
	}
	component, err := controller.GetComponent(ctx, r.Client, build)
	if err != nil {
		if apierrors.IsNotFound(err) {
			logger.Error(err, "Component doesn't exist")
			return false, err
		}
		return true, fmt.Errorf("failed to get component: %w ", err)
	}
	r.addComponentSpecificConfigs(ctx, logger, component, deployableArtifact, build)

	if err := r.Create(ctx, deployableArtifact); err != nil {
		return true, fmt.Errorf("failed to create deployable artifact: %w", err)
	}
	return false, nil
}
func (r *Reconciler) updateOrCreateDeployment(ctx context.Context, build *choreov1.Build) (bool, error) {
	logger := log.FromContext(ctx)

	environment, err := r.getFirstEnvironmentFromDeploymentPipeline(ctx, build)
	if err != nil {
		if apierrors.IsNotFound(err) {
			// Environment not found, no need to requeue
			return false, nil
		}
		// Other errors should be retried
		return true, err
	}

	// Retrieve the existing deployment
	deployment, err := controller.GetDeploymentByEnvironment(ctx, r.Client, build, environment.Labels[labels.LabelKeyName])
	if err != nil {
		var hierarchyErr *controller.HierarchyNotFoundError
		if errors.As(err, &hierarchyErr) {
			// Deployment does not exist, create a new one
			logger.Info("Hierarchy not found", "error", hierarchyErr)
			// Call your deployment creation handler
			return r.createDeployment(ctx, build, environment.Labels[labels.LabelKeyName])
		}
		// Return if the error is not a "Not Found" error
		logger.Error(err, "Failed to get deployment", "Build.name", build.Name)
		return true, err
	}

	// If deployment exists, update the DeploymentArtifactRef if necessary
	if deployment.Spec.DeploymentArtifactRef != build.Name {
		deployment.Spec.DeploymentArtifactRef = build.Name
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

func (r *Reconciler) createDeployment(ctx context.Context, build *choreov1.Build, environmentName string) (bool, error) {
	logger := log.FromContext(ctx)

	// Generate the deployment name
	deploymentName := dpkubernetes.GenerateK8sNameWithLengthLimit(
		dpkubernetes.MaxResourceNameLength,
		controller.GetOrganizationName(build),
		controller.GetProjectName(build),
		controller.GetComponentName(build),
		controller.GetDeploymentTrackName(build),
		environmentName,
	)

	deploymentLabelName := dpkubernetes.GenerateK8sNameWithLengthLimit(63, environmentName, "deployment")

	// Create the deployment object
	deployment := &choreov1.Deployment{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "core.choreo.dev/v1",
			Kind:       "Deployment",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      deploymentName,
			Namespace: build.Namespace,
			Labels: map[string]string{
				labels.LabelKeyOrganizationName:    controller.GetOrganizationName(build),
				labels.LabelKeyProjectName:         controller.GetProjectName(build),
				labels.LabelKeyComponentName:       controller.GetComponentName(build),
				labels.LabelKeyDeploymentTrackName: controller.GetDeploymentTrackName(build),
				labels.LabelKeyEnvironmentName:     environmentName,
				labels.LabelKeyName:                deploymentLabelName,
			},
		},
		Spec: choreov1.DeploymentSpec{
			DeploymentArtifactRef: build.Name,
		},
	}

	// Create the deployment
	if err := r.Create(ctx, deployment); err != nil {
		logger.Error(err, "Failed to create deployment", "Deployment.name", deploymentName)
		return true, err
	}
	logger.Info("Created deployment", "Deployment.Name", deploymentName, "Deployment.Label.Name", deploymentLabelName)
	return false, nil
}

func (r *Reconciler) addComponentSpecificConfigs(ctx context.Context, logger logr.Logger, component *choreov1.Component, deployableArtifact *choreov1.DeployableArtifact, build *choreov1.Build) {
	componentType := component.Spec.Type
	if componentType == choreov1.ComponentTypeService {
		endpointTemplates, err := r.getEndpointConfigs(ctx, build)
		if err != nil {
			logger.Error(err, "Failed to get endpoint configurations", "Build.Name", build.Name)
		}
		deployableArtifact.Spec.Configuration = &choreov1.Configuration{
			EndpointTemplates: endpointTemplates,
		}
	} else if componentType == choreov1.ComponentTypeScheduledTask {
		deployableArtifact.Spec.Configuration = &choreov1.Configuration{
			Application: &choreov1.Application{
				Task: &choreov1.TaskConfig{
					Disabled: false,
					Schedule: &choreov1.TaskSchedule{
						Cron:     "*/5 * * * *",
						Timezone: "Asia/Colombo",
					},
				},
			},
		}
	} else if componentType == choreov1.ComponentTypeWebApplication {
		deployableArtifact.Spec.Configuration = &choreov1.Configuration{
			EndpointTemplates: []choreov1.EndpointTemplate{
				{
					// TODO: This should come from the component descriptor in source code.
					ObjectMeta: metav1.ObjectMeta{
						Name: "webapp",
					},
					Spec: choreov1.EndpointSpec{
						Type: "HTTP",
						Service: choreov1.EndpointServiceSpec{
							BasePath: "/",
							Port:     80,
						},
					},
				},
			},
		}
	}
}

func (r *Reconciler) getEndpointConfigs(ctx context.Context, build *choreov1.Build) ([]choreov1.EndpointTemplate, error) {
	component, err := controller.GetComponent(ctx, r.Client, build)
	if err != nil {
		return nil, err
	}
	componentManifestPath := "./choreo/component.yaml"
	if build.Spec.Path != "" {
		componentManifestPath = path.Clean(fmt.Sprintf(".%s/.choreo/component.yaml", build.Spec.Path))
	}

	owner, repositoryName, err := extractRepositoryInfo(component.Spec.Source.GitRepository.URL)
	if err != nil {
		return nil, fmt.Errorf("bad git repository url: %w", err)
	}
	// If the build has a specific git revision, use it. Otherwise, use the default branch.
	ref := build.Spec.Branch
	if build.Spec.GitRevision != "" {
		ref = build.Spec.GitRevision
	}

	componentYaml, _, _, err := r.GithubClient.Repositories.GetContents(ctx, owner, repositoryName, componentManifestPath, &github.RepositoryContentGetOptions{Ref: ref})
	if err != nil {
		return nil, fmt.Errorf("failed to get component.yaml from the repository buildName:%s;owner:%s;repo:%s;%w", build.Name, owner, repositoryName, err)
	}
	componentYamlContent, err := componentYaml.GetContent()
	if err != nil {
		return nil, fmt.Errorf("failed to get content of component.yaml from the repository  buildName:%s;owner:%s;repo:%s;%w", build.Name, owner, repositoryName, err)
	}
	config := descriptor.Config{}
	err = yaml.Unmarshal([]byte(componentYamlContent), &config)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal component.yaml from the repository buildName:%s;owner:%s;repo:%s;%w", build.Name, owner, repositoryName, err)
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

func extractRepositoryInfo(repoURL string) (string, string, error) {
	if repoURL == "" {
		return "", "", fmt.Errorf("repository URL is empty")
	}
	if strings.Split(repoURL, "/")[0] != "https:" {
		return "", "", fmt.Errorf("invalid repository URL")
	}
	urlSegments := strings.Split(repoURL, "/")
	start := 0
	len := len(urlSegments)
	if len > 2 {
		start = len - 2
	}
	owner := urlSegments[start]
	repo := urlSegments[start+1]
	return owner, repo, nil
}
