// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package deployment

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8slabels "k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/controller"
	"github.com/openchoreo/openchoreo/internal/dataplane"
	"github.com/openchoreo/openchoreo/internal/labels"
)

// makeDeploymentContext creates a deployment context for the given deployment by retrieving the
// parent objects that this deployment is associated with.
func (r *Reconciler) makeDeploymentContext(ctx context.Context, deployment *openchoreov1alpha1.Deployment) (*dataplane.DeploymentContext, error) {
	project, err := controller.GetProject(ctx, r.Client, deployment)
	if err != nil {
		return nil, fmt.Errorf("cannot retrieve the project: %w", err)
	}

	component, err := controller.GetComponent(ctx, r.Client, deployment)
	if err != nil {
		return nil, fmt.Errorf("cannot retrieve the component: %w", err)
	}

	deploymentTrack, err := controller.GetDeploymentTrack(ctx, r.Client, deployment)
	if err != nil {
		return nil, fmt.Errorf("cannot retrieve the deployment track: %w", err)
	}

	environment, err := controller.GetEnvironment(ctx, r.Client, deployment)
	if err != nil {
		return nil, fmt.Errorf("cannot retrieve the environment: %w", err)
	}

	targetDeployableArtifact, err := r.findDeployableArtifact(ctx, deployment)
	if err != nil {
		meta.SetStatusCondition(&deployment.Status.Conditions,
			NewArtifactNotFoundCondition(deployment.Spec.DeploymentArtifactRef, deployment.Generation))
		return nil, fmt.Errorf("cannot retrieve the deployable artifact: %w", err)
	}

	containerImage, err := r.findContainerImage(ctx, component, targetDeployableArtifact, deployment)
	if err != nil {
		return nil, fmt.Errorf("cannot retrieve the container image: %w", err)
	}

	configurationGroups, err := r.findConfigurationGroups(ctx, targetDeployableArtifact)
	if err != nil {
		return nil, fmt.Errorf("cannot retrieve the referenced configuration groups: %w", err)
	}

	meta.SetStatusCondition(&deployment.Status.Conditions, NewArtifactResolvedCondition(deployment.Generation))

	return &dataplane.DeploymentContext{
		Project:             project,
		Component:           component,
		DeploymentTrack:     deploymentTrack,
		DeployableArtifact:  targetDeployableArtifact,
		Deployment:          deployment,
		Environment:         environment,
		ConfigurationGroups: configurationGroups,
		ContainerImage:      containerImage,
	}, nil
}

func (r *Reconciler) findDeployableArtifact(ctx context.Context, deployment *openchoreov1alpha1.Deployment) (*openchoreov1alpha1.DeployableArtifact, error) {
	// Find the DeployableArtifact that the Deployment is referring to within the hierarchy
	deployableArtifactList := &openchoreov1alpha1.DeployableArtifactList{}
	listOpts := []client.ListOption{
		client.InNamespace(deployment.Namespace),
		client.MatchingLabels(makeHierarchyLabelsForDeploymentTrack(deployment.ObjectMeta)),
	}
	if err := r.Client.List(ctx, deployableArtifactList, listOpts...); err != nil {
		return nil, err
	}

	// Find the target deployable artifact
	var targetDeployableArtifact *openchoreov1alpha1.DeployableArtifact
	for _, deployableArtifact := range deployableArtifactList.Items {
		if deployableArtifact.Name == deployment.Spec.DeploymentArtifactRef {
			targetDeployableArtifact = &deployableArtifact
			break
		}
	}

	if targetDeployableArtifact == nil {
		return nil, fmt.Errorf("deployable artifact %q is not found for deployment: %s/%s", deployment.Spec.DeploymentArtifactRef, deployment.Namespace, deployment.Name)
	}

	return targetDeployableArtifact, nil
}

func makeHierarchyLabelsForDeploymentTrack(objMeta metav1.ObjectMeta) map[string]string {
	// Hierarchical labels to be used for DeploymentTrack
	keys := []string{
		labels.LabelKeyOrganizationName,
		labels.LabelKeyProjectName,
		labels.LabelKeyComponentName,
		labels.LabelKeyDeploymentTrackName,
	}

	// Prepare a new map to hold the extracted labels.
	hierarchyLabelMap := make(map[string]string, len(keys))

	for _, key := range keys {
		// We need to assign an empty string if the label is not present.
		// Otherwise, the k8s listing will return all the objects.
		val := ""
		if objMeta.Labels != nil {
			val = objMeta.Labels[key]
		}
		hierarchyLabelMap[key] = val
	}

	return hierarchyLabelMap
}

func (r *Reconciler) findContainerImage(ctx context.Context, component *openchoreov1alpha1.Component,
	deployableArtifact *openchoreov1alpha1.DeployableArtifact, deployment *openchoreov1alpha1.Deployment) (string, error) {
	if buildRef := deployableArtifact.Spec.TargetArtifact.FromBuildRef; buildRef != nil {
		if buildRef.Name != "" {
			// Find the build that the deployable artifact is referring to
			buildList := &openchoreov1alpha1.BuildV2List{}
			listOpts := []client.ListOption{
				client.InNamespace(deployableArtifact.Namespace),
				client.MatchingLabels(makeHierarchyLabelsForDeploymentTrack(deployableArtifact.ObjectMeta)),
			}
			if err := r.Client.List(ctx, buildList, listOpts...); err != nil {
				return "", fmt.Errorf("findContainerImage: failed to list builds: %w", err)
			}

			for _, build := range buildList.Items {
				if build.Name == buildRef.Name {
					// TODO: Make local registry configurable and move to build controller
					return fmt.Sprintf("%s/%s", "localhost:30003", build.Status.ImageStatus.Image), nil
				}
			}
			meta.SetStatusCondition(&deployment.Status.Conditions,
				NewArtifactBuildNotFoundCondition(deployment.Spec.DeploymentArtifactRef, buildRef.Name, deployment.Generation))
			return "", fmt.Errorf("build %q is not found for deployable artifact: %s/%s", buildRef.Name, deployableArtifact.Namespace, deployableArtifact.Name)
		} else if buildRef.GitRevision != "" {
			// TODO: Search for the build by git revision
			return "", fmt.Errorf("search by git revision is not supported")
		}
		return "", fmt.Errorf("one of the build name or git revision should be provided")
	} else if imageRef := deployableArtifact.Spec.TargetArtifact.FromImageRef; imageRef != nil {
		if imageRef.Tag == "" {
			return "", fmt.Errorf("image tag is not provided")
		}
		containerRegistry := component.Spec.Source.ContainerRegistry
		if containerRegistry == nil {
			return "", fmt.Errorf("container registry is not provided for the component %s/%s", component.Namespace, component.Name)
		}
		return fmt.Sprintf("%s:%s", containerRegistry.ImageName, imageRef.Tag), nil
	}
	return "", fmt.Errorf("one of the build or image reference should be provided")
}

func (r *Reconciler) findConfigurationGroups(ctx context.Context, deployableArtifact *openchoreov1alpha1.DeployableArtifact) ([]*openchoreov1alpha1.ConfigurationGroup, error) {
	// Find all the ConfigurationGroups that the deployable artifact is referring to
	if deployableArtifact.Spec.Configuration == nil || deployableArtifact.Spec.Configuration.Application == nil {
		return nil, nil
	}

	appCfg := deployableArtifact.Spec.Configuration.Application
	configGroupNameSet := make(map[string]struct{})

	// The following individual loops will build the referenced configuration group names
	// from the Env, EnvFrom, FileMounts, and FileMountsFrom sections of the application configuration.

	// Find configuration groups in the Env section
	for _, ev := range appCfg.Env {
		if ev.ValueFrom == nil {
			continue
		}
		if ev.ValueFrom.ConfigurationGroupRef == nil {
			continue
		}
		if ev.ValueFrom.ConfigurationGroupRef.Name == "" {
			continue // TODO: This will be validated by the admission controller
		}
		configGroupNameSet[ev.ValueFrom.ConfigurationGroupRef.Name] = struct{}{}
	}

	// Find configuration groups in the EnvFrom section
	for _, evf := range appCfg.EnvFrom {
		if evf.ConfigurationGroupRef == nil {
			continue
		}
		if evf.ConfigurationGroupRef.Name == "" {
			continue // TODO: This will be validated by the admission controller
		}
		configGroupNameSet[evf.ConfigurationGroupRef.Name] = struct{}{}
	}

	// Find configuration groups in the FileMounts section
	for _, fm := range appCfg.FileMounts {
		if fm.ValueFrom == nil {
			continue
		}
		if fm.ValueFrom.ConfigurationGroupRef == nil {
			continue
		}
		if fm.ValueFrom.ConfigurationGroupRef.Name == "" {
			continue // TODO: This will be validated by the admission controller
		}
		configGroupNameSet[fm.ValueFrom.ConfigurationGroupRef.Name] = struct{}{}
	}

	// Find configuration groups in the FileMountsFrom section
	for _, fmf := range appCfg.FileMountsFrom {
		if fmf.ConfigurationGroupRef == nil {
			continue
		}
		if fmf.ConfigurationGroupRef.Name == "" {
			continue // TODO: This will be validated by the admission controller
		}
		configGroupNameSet[fmf.ConfigurationGroupRef.Name] = struct{}{}
	}

	// Build the label selector to find the configuration groups
	configGroupNames := make([]string, 0, len(configGroupNameSet))
	for name := range configGroupNameSet {
		configGroupNames = append(configGroupNames, name)
	}

	if len(configGroupNames) == 0 {
		return nil, nil
	}

	req, err := k8slabels.NewRequirement(labels.LabelKeyName, selection.In, configGroupNames)
	if err != nil {
		return nil, fmt.Errorf("failed to build the label selector: %w", err)
	}
	selector := k8slabels.NewSelector().Add(*req)

	configurationGroupList := &openchoreov1alpha1.ConfigurationGroupList{}
	listOpts := []client.ListOption{
		client.InNamespace(deployableArtifact.Namespace),
		client.MatchingLabels(map[string]string{
			labels.LabelKeyOrganizationName: deployableArtifact.Labels[labels.LabelKeyOrganizationName],
		}),
		client.MatchingLabelsSelector{
			Selector: selector,
		},
	}
	if err := r.Client.List(ctx, configurationGroupList, listOpts...); err != nil {
		return nil, err
	}

	cgs := make([]*openchoreov1alpha1.ConfigurationGroup, 0, len(configurationGroupList.Items))
	for _, item := range configurationGroupList.Items {
		cgs = append(cgs, &item)
	}

	return cgs, nil
}
