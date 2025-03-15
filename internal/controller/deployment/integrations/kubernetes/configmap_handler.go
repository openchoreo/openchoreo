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

package kubernetes

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/go-cmp/cmp"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	choreov1 "github.com/choreo-idp/choreo/api/v1"
	"github.com/choreo-idp/choreo/internal/dataplane"
	dpkubernetes "github.com/choreo-idp/choreo/internal/dataplane/kubernetes"
)

type configMapHandler struct {
	kubernetesClient client.Client
}

var _ dataplane.ResourceHandler[dataplane.DeploymentContext] = (*configMapHandler)(nil)

func NewConfigMapHandler(kubernetesClient client.Client) dataplane.ResourceHandler[dataplane.DeploymentContext] {
	return &configMapHandler{
		kubernetesClient: kubernetesClient,
	}
}

func (h *configMapHandler) Name() string {
	return "KubernetesConfigMapHandler"
}

func (h *configMapHandler) IsRequired(deployCtx *dataplane.DeploymentContext) bool {
	return len(deployCtx.ConfigurationGroups) > 0
}

func (h *configMapHandler) GetCurrentState(ctx context.Context, deployCtx *dataplane.DeploymentContext) (interface{}, error) {
	namespace := makeNamespaceName(deployCtx)
	labels := makeWorkloadLabels(deployCtx)
	cmList := &corev1.ConfigMapList{}
	listOpts := []client.ListOption{
		client.InNamespace(namespace),
		client.MatchingLabels(labels),
	}
	err := h.kubernetesClient.List(ctx, cmList, listOpts...)
	if err != nil {
		return nil, err
	}
	if len(cmList.Items) == 0 {
		return nil, nil
	}
	// Convert the list to a slice of pointers so that it can be compared with the new state
	// during the update operation
	configMaps := make([]*corev1.ConfigMap, 0, len(cmList.Items))
	for i := range cmList.Items {
		configMaps = append(configMaps, &cmList.Items[i])
	}
	return configMaps, nil
}

func (h *configMapHandler) Create(ctx context.Context, deployCtx *dataplane.DeploymentContext) error {
	configMaps := makeConfigMaps(deployCtx)
	for _, cm := range configMaps {
		err := h.kubernetesClient.Create(ctx, cm)
		if err != nil {
			return fmt.Errorf("error while creating configmap %s: %w", cm.Name, err)
		}
	}
	return nil
}

func (h *configMapHandler) Update(ctx context.Context, deployCtx *dataplane.DeploymentContext, currentState interface{}) error {
	currentConfigMaps, ok := currentState.([]*corev1.ConfigMap)
	if !ok {
		return errors.New("failed to cast current state to a slice of ConfigMaps")
	}

	desiredConfigMaps := makeConfigMaps(deployCtx)

	// Build a map for quick lookups of current and desired ConfigMaps
	// Using "name" as the key
	currentMap := make(map[string]*corev1.ConfigMap, len(currentConfigMaps))
	for _, cm := range currentConfigMaps {
		currentMap[cm.Name] = cm
	}

	desiredMap := make(map[string]*corev1.ConfigMap, len(desiredConfigMaps))
	for _, cm := range desiredConfigMaps {
		desiredMap[cm.Name] = cm
	}

	// Create or update the ConfigMaps that are not present in the current state
	for name, desiredConfigMap := range desiredMap {
		existingConfigMap, found := currentMap[name]
		if !found {
			// Create the ConfigMap if it is not present in the current state
			if err := h.kubernetesClient.Create(ctx, desiredConfigMap); err != nil {
				return fmt.Errorf("error while creating configmap %s: %w", desiredConfigMap.Name, err)
			}
			continue
		}

		// Update the ConfigMaps that are present in the current state
		if !cmp.Equal(existingConfigMap.Data, desiredConfigMap.Data) ||
			!cmp.Equal(extractManagedLabels(existingConfigMap.Labels), extractManagedLabels(desiredConfigMap.Labels)) {
			// TODO: Auto restart the pods that are using the ConfigMap
			updatedConfigMap := existingConfigMap.DeepCopy()
			updatedConfigMap.Data = desiredConfigMap.Data
			updatedConfigMap.Labels = desiredConfigMap.Labels

			if err := h.kubernetesClient.Update(ctx, updatedConfigMap); err != nil {
				return fmt.Errorf("error while updating configmap %s: %w", desiredConfigMap.Name, err)
			}
		}
	}

	// Delete the ConfigMaps that are not present in the desired state
	for name, existingConfigMap := range currentMap {
		if _, found := desiredMap[name]; !found {
			if err := h.kubernetesClient.Delete(ctx, existingConfigMap); err != nil {
				return fmt.Errorf("error while deleting configmap %s: %w", existingConfigMap.Name, err)
			}
		}
	}

	return nil
}

func (h *configMapHandler) Delete(ctx context.Context, deployCtx *dataplane.DeploymentContext) error {
	namespace := makeNamespaceName(deployCtx)
	labels := makeWorkloadLabels(deployCtx)
	deleteAllOpt := []client.DeleteAllOfOption{
		// Make sure the correct labels are used, otherwise, it might delete unwanted ConfigMaps
		client.InNamespace(namespace),
		client.MatchingLabels(labels),
	}
	err := h.kubernetesClient.DeleteAllOf(ctx, &corev1.ConfigMap{}, deleteAllOpt...)
	if err != nil {
		return fmt.Errorf("error while deleting configmaps: %w", err)
	}
	return nil
}

func makeConfigMaps(deployCtx *dataplane.DeploymentContext) []*corev1.ConfigMap {
	configMaps := make([]*corev1.ConfigMap, 0)
	for _, cg := range deployCtx.ConfigurationGroups {
		cgConfigs := cg.Spec.Configurations
		if len(cgConfigs) == 0 {
			continue
		}

		cm := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      makeConfigMapName(deployCtx, cg),
				Namespace: makeNamespaceName(deployCtx),
				Labels:    makeWorkloadLabels(deployCtx),
			},
		}
		cmData := make(map[string]string)
		for _, cgConfig := range cgConfigs {
			cgv := findConfigGroupValueForEnv(cgConfig.Values, cg.Spec.EnvironmentGroups, deployCtx.Environment)
			if cgv == nil || cgv.Value == "" {
				continue
			}
			// TODO: Improvement: filter the values that are only used in deployable artifact
			cmData[cgConfig.Key] = cgv.Value
		}

		// If there are no configuration values to add to the ConfigMap, skip creating it
		if len(cmData) == 0 {
			continue
		}
		cm.Data = cmData
		configMaps = append(configMaps, cm)
	}
	return configMaps
}

func makeConfigMapName(deployCtx *dataplane.DeploymentContext, cg *choreov1.ConfigurationGroup) string {
	// TODO: Ideally, this should be choreo name instead of kubernetes name
	componentName := deployCtx.Component.Name
	deploymentTrackName := deployCtx.DeploymentTrack.Name
	configGroupName := cg.Name
	// Limit the name to 253 characters to comply with the K8s name length limit for ConfigMaps
	return dpkubernetes.GenerateK8sName(componentName, deploymentTrackName, configGroupName)
}
