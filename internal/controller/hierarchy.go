// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/labels"
)

// This file contains the helper functions to get the Choreo specific parent objects from the Kubernetes objects.

// HierarchyNotFoundError is an error type that is used to indicate that a parent object in the hierarchy is not found.
type HierarchyNotFoundError struct {
	objInfo    string
	parentInfo string

	parentHierarchyInfos []string
}

func (e *HierarchyNotFoundError) Error() string {
	return fmt.Sprintf("%s refers to a non-existent %s on %s", e.objInfo, e.parentInfo, strings.Join(e.parentHierarchyInfos, " -> "))
}

// NewHierarchyNotFoundError creates a new error with the given object and parent object details.
// The parentObj is the immediate parent of the obj
// The parentHierarchyObjs are the hierarchy of objects from the parentObj to the top level object starting from the top level object.
// Example: NewHierarchyNotFoundError(deployment, deploymentTrack, namespace, project, component)
func NewHierarchyNotFoundError(obj client.Object, parentObj client.Object, parentHierarchyObjs ...client.Object) error {
	getKindFn := func(obj client.Object) string {
		if !obj.GetObjectKind().GroupVersionKind().Empty() {
			return obj.GetObjectKind().GroupVersionKind().Kind
		}
		// If the object is initialized without setting the GVK, use the type name.
		return reflect.TypeOf(obj).Elem().Name()
	}

	genInfoFn := func(obj client.Object) string {
		return fmt.Sprintf("%s '%s'", strings.ToLower(getKindFn(obj)), obj.GetName())
	}

	parentHierarchyInfos := make([]string, 0, len(parentHierarchyObjs))
	for _, parentHierarchyObj := range parentHierarchyObjs {
		parentHierarchyInfos = append(parentHierarchyInfos, genInfoFn(parentHierarchyObj))
	}

	return &HierarchyNotFoundError{
		objInfo:              genInfoFn(obj),
		parentInfo:           genInfoFn(parentObj),
		parentHierarchyInfos: parentHierarchyInfos,
	}
}

// IgnoreHierarchyNotFoundError returns nil if the given error is a HierarchyNotFoundError.
// This is useful during the reconciliation process to ignore the error if the parent object is not found and avoid retrying.
func IgnoreHierarchyNotFoundError(err error) error {
	if err == nil {
		return nil
	}
	var notFoundErr *HierarchyNotFoundError
	if errors.As(err, &notFoundErr) {
		return nil
	}
	return err
}

// HierarchyFunc is a generic function type that takes a context, client, and object as input and
// returns an object of type T and an error.
// This is used for MakeHierarchyWatchHandler to define the function that will be called to get the target object.
type HierarchyFunc[T any] func(ctx context.Context, c client.Client, obj client.Object) (T, error)

// objWithName is a helper functions to set the name of the object.
// Use this function to only set the name of a newly created object as it directly modifies the object.
func objWithName(obj client.Object, name string) client.Object {
	obj.SetName(name)
	return obj
}

func GetDeploymentPipeline(ctx context.Context, c client.Client, obj client.Object, dpName string) (*openchoreov1alpha1.DeploymentPipeline, error) {
	deploymentPipelineList := &openchoreov1alpha1.DeploymentPipelineList{}
	listOpts := []client.ListOption{
		client.InNamespace(obj.GetNamespace()),
		client.MatchingLabels{
			labels.LabelKeyNamespaceName: GetNamespaceName(obj),
			labels.LabelKeyName:          dpName,
		},
	}

	if err := c.List(ctx, deploymentPipelineList, listOpts...); err != nil {
		return nil, fmt.Errorf("failed to list projects: %w", err)
	}

	if len(deploymentPipelineList.Items) > 0 {
		return &deploymentPipelineList.Items[0], nil
	}

	return nil, NewHierarchyNotFoundError(obj, objWithName(&openchoreov1alpha1.DeploymentPipeline{}, dpName),
		objWithName(&corev1.Namespace{}, obj.GetNamespace()),
	)
}

func GetProject(ctx context.Context, c client.Client, obj client.Object) (*openchoreov1alpha1.Project, error) {
	projectList := &openchoreov1alpha1.ProjectList{}
	listOpts := []client.ListOption{
		client.InNamespace(obj.GetNamespace()),
		client.MatchingLabels{
			labels.LabelKeyNamespaceName: GetNamespaceName(obj),
			labels.LabelKeyName:          GetProjectName(obj),
		},
	}

	if err := c.List(ctx, projectList, listOpts...); err != nil {
		return nil, fmt.Errorf("failed to list projects: %w", err)
	}

	if len(projectList.Items) > 0 {
		return &projectList.Items[0], nil
	}

	return nil, NewHierarchyNotFoundError(obj, objWithName(&openchoreov1alpha1.Project{}, GetProjectName(obj)),
		objWithName(&corev1.Namespace{}, obj.GetNamespace()),
	)
}

func GetComponent(ctx context.Context, c client.Client, obj client.Object) (*openchoreov1alpha1.Component, error) {
	componentList := &openchoreov1alpha1.ComponentList{}
	listOpts := []client.ListOption{
		client.InNamespace(obj.GetNamespace()),
		client.MatchingLabels{
			labels.LabelKeyNamespaceName: GetNamespaceName(obj),
			labels.LabelKeyProjectName:   GetProjectName(obj),
			labels.LabelKeyName:          GetComponentName(obj),
		},
	}

	if err := c.List(ctx, componentList, listOpts...); err != nil {
		return nil, fmt.Errorf("failed to list components: %w", err)
	}

	if len(componentList.Items) > 0 {
		return &componentList.Items[0], nil
	}

	return nil, NewHierarchyNotFoundError(obj, objWithName(&openchoreov1alpha1.Component{}, GetComponentName(obj)),
		objWithName(&corev1.Namespace{}, obj.GetNamespace()),
		objWithName(&openchoreov1alpha1.Project{}, GetProjectName(obj)),
	)
}

func GetDeploymentTrack(ctx context.Context, c client.Client, obj client.Object) (*openchoreov1alpha1.DeploymentTrack, error) {
	deploymentTrackList := &openchoreov1alpha1.DeploymentTrackList{}
	listOpts := []client.ListOption{
		client.InNamespace(obj.GetNamespace()),
		client.MatchingLabels{
			labels.LabelKeyNamespaceName: GetNamespaceName(obj),
			labels.LabelKeyProjectName:   GetProjectName(obj),
			labels.LabelKeyComponentName: GetComponentName(obj),
			labels.LabelKeyName:          GetDeploymentTrackName(obj),
		},
	}

	if err := c.List(ctx, deploymentTrackList, listOpts...); err != nil {
		return nil, fmt.Errorf("failed to list deployment tracks: %w", err)
	}

	if len(deploymentTrackList.Items) > 0 {
		return &deploymentTrackList.Items[0], nil
	}

	return nil, NewHierarchyNotFoundError(obj, objWithName(&openchoreov1alpha1.DeploymentTrack{}, GetDeploymentTrackName(obj)),
		objWithName(&corev1.Namespace{}, obj.GetNamespace()),
		objWithName(&openchoreov1alpha1.Project{}, GetProjectName(obj)),
		objWithName(&openchoreov1alpha1.Component{}, GetComponentName(obj)),
	)
}

func GetEnvironment(ctx context.Context, c client.Client, obj client.Object) (*openchoreov1alpha1.Environment, error) {
	environmentList := &openchoreov1alpha1.EnvironmentList{}
	listOpts := []client.ListOption{
		client.InNamespace(obj.GetNamespace()),
		client.MatchingLabels{
			labels.LabelKeyNamespaceName: GetNamespaceName(obj),
			labels.LabelKeyName:          GetEnvironmentName(obj),
		},
	}

	if err := c.List(ctx, environmentList, listOpts...); err != nil {
		return nil, fmt.Errorf("failed to list environments: %w", err)
	}

	if len(environmentList.Items) > 0 {
		return &environmentList.Items[0], nil
	}

	return nil, NewHierarchyNotFoundError(obj, objWithName(&openchoreov1alpha1.Environment{}, GetEnvironmentName(obj)),
		objWithName(&corev1.Namespace{}, obj.GetNamespace()),
	)
}

func GetEnvironmentByName(ctx context.Context, c client.Client, obj client.Object, envName string) (*openchoreov1alpha1.Environment, error) {
	environmentList := &openchoreov1alpha1.EnvironmentList{}
	listOpts := []client.ListOption{
		client.InNamespace(obj.GetNamespace()),
		client.MatchingLabels{
			labels.LabelKeyNamespaceName: GetNamespaceName(obj),
			labels.LabelKeyName:          envName,
		},
	}

	if err := c.List(ctx, environmentList, listOpts...); err != nil {
		return nil, fmt.Errorf("failed to list environments: %w", err)
	}

	if len(environmentList.Items) > 0 {
		return &environmentList.Items[0], nil
	}

	return nil, NewHierarchyNotFoundError(obj, objWithName(&openchoreov1alpha1.Environment{}, envName),
		objWithName(&corev1.Namespace{}, obj.GetNamespace()),
	)
}

// GetDataPlaneByEnvironment retrieves the DataPlane object for the given Environment.
// It uses the DataPlaneRef field in the Environment to find the DataPlane object.
// Note: This function only returns DataPlane, not ClusterDataPlane. For environments
// referencing ClusterDataPlane, use GetDataPlaneOrClusterDataPlaneOfEnv from reference.go.
func GetDataPlaneByEnvironment(ctx context.Context, c client.Client, env *openchoreov1alpha1.Environment) (*openchoreov1alpha1.DataPlane, error) {
	ref := env.Spec.DataPlaneRef

	// Determine the plane name and kind
	var planeName string
	var isClusterScope bool

	if ref == nil {
		// Default to DataPlane named "default" in the same namespace
		planeName = DefaultPlaneName
		isClusterScope = false
	} else {
		planeName = ref.Name
		isClusterScope = ref.Kind == openchoreov1alpha1.DataPlaneRefKindClusterDataPlane
	}

	if isClusterScope {
		// For ClusterDataPlane references, this function cannot return the correct type
		return nil, NewHierarchyNotFoundError(env, objWithName(&openchoreov1alpha1.DataPlane{}, planeName),
			objWithName(&corev1.Namespace{}, env.GetNamespace()),
		)
	}

	dataPlane := &openchoreov1alpha1.DataPlane{}
	key := client.ObjectKey{Namespace: env.Namespace, Name: planeName}

	if err := c.Get(ctx, key, dataPlane); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, NewHierarchyNotFoundError(env, objWithName(&openchoreov1alpha1.DataPlane{}, planeName),
				objWithName(&corev1.Namespace{}, env.GetNamespace()),
			)
		}
		return nil, fmt.Errorf("failed to get data plane: %w", err)
	}

	return dataPlane, nil
}

func GetDataPlane(ctx context.Context, c client.Client, obj client.Object) (*openchoreov1alpha1.DataPlane, error) {
	dataPlaneList := &openchoreov1alpha1.DataPlaneList{}
	listOpts := []client.ListOption{
		client.InNamespace(obj.GetNamespace()),
		client.MatchingLabels{
			labels.LabelKeyNamespaceName: GetNamespaceName(obj),
		},
	}

	if err := c.List(ctx, dataPlaneList, listOpts...); err != nil {
		return nil, fmt.Errorf("failed to list data planes: %w", err)
	}

	if len(dataPlaneList.Items) > 0 {
		return &dataPlaneList.Items[0], nil
	}

	return nil, NewHierarchyNotFoundError(obj, objWithName(&openchoreov1alpha1.DataPlane{}, GetDataPlaneName(obj)),
		objWithName(&corev1.Namespace{}, obj.GetNamespace()),
	)
}

func GetBuildPlane(ctx context.Context, c client.Client, obj client.Object) (*openchoreov1alpha1.BuildPlane, error) {
	// 1. Try to get the project to check for buildPlaneRef
	project, projectErr := GetProject(ctx, c, obj)
	if projectErr != nil {
		// Only fall through if project is not found (HierarchyNotFoundError)
		// For other errors (e.g., network errors), propagate the error
		var hierarchyNotFoundErr *HierarchyNotFoundError
		if !errors.As(projectErr, &hierarchyNotFoundErr) {
			return nil, fmt.Errorf("failed to get project for build plane lookup: %w", projectErr)
		}
		// Project not found - fall through to default BuildPlane lookup
	} else if project.Spec.BuildPlaneRef != nil {
		ref := project.Spec.BuildPlaneRef

		// Handle ClusterBuildPlane case - this function only returns BuildPlane
		if ref.Kind == openchoreov1alpha1.BuildPlaneRefKindClusterBuildPlane {
			return nil, NewHierarchyNotFoundError(obj,
				objWithName(&openchoreov1alpha1.BuildPlane{}, ref.Name),
				objWithName(&corev1.Namespace{}, obj.GetNamespace()),
			)
		}

		// Use explicit buildPlaneRef
		buildPlane := &openchoreov1alpha1.BuildPlane{}
		key := client.ObjectKey{Namespace: obj.GetNamespace(), Name: ref.Name}
		if err := c.Get(ctx, key, buildPlane); err != nil {
			if apierrors.IsNotFound(err) {
				return nil, NewHierarchyNotFoundError(obj,
					objWithName(&openchoreov1alpha1.BuildPlane{}, ref.Name),
					objWithName(&corev1.Namespace{}, obj.GetNamespace()),
					project,
				)
			}
			return nil, fmt.Errorf("failed to get build plane: %w", err)
		}
		return buildPlane, nil
	}

	// 2. Try "default" BuildPlane
	buildPlane := &openchoreov1alpha1.BuildPlane{}
	key := client.ObjectKey{Namespace: obj.GetNamespace(), Name: DefaultPlaneName}
	if err := c.Get(ctx, key, buildPlane); err == nil {
		return buildPlane, nil
	}

	// 3. Fallback: first BuildPlane in namespace with matching namespace label
	buildPlaneList := &openchoreov1alpha1.BuildPlaneList{}
	listOpts := []client.ListOption{
		client.InNamespace(obj.GetNamespace()),
		client.MatchingLabels{labels.LabelKeyNamespaceName: obj.GetNamespace()},
	}

	if err := c.List(ctx, buildPlaneList, listOpts...); err != nil {
		return nil, fmt.Errorf("failed to list build planes: %w", err)
	}

	if len(buildPlaneList.Items) > 0 {
		return &buildPlaneList.Items[0], nil
	}

	return nil, NewHierarchyNotFoundError(obj, objWithName(&openchoreov1alpha1.BuildPlane{}, DefaultPlaneName),
		objWithName(&corev1.Namespace{}, obj.GetNamespace()),
	)
}
