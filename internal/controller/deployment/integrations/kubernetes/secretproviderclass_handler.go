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
	"gopkg.in/yaml.v3"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	choreov1 "github.com/openchoreo/openchoreo/api/v1"
	"github.com/openchoreo/openchoreo/internal/dataplane"
	dpkubernetes "github.com/openchoreo/openchoreo/internal/dataplane/kubernetes"
	csisecretv1 "github.com/openchoreo/openchoreo/internal/dataplane/kubernetes/types/secretstorecsi/v1"
)

const (
	// TODO: Make this configurable
	hashicorpVaultProvider = "vault"
	hashicorpVaultRoleName = "choreo-secret-reader-role"
	hashicorpVaultAddress  = "http://choreo-dp-vault:8200"
)

type secretProviderClassHandler struct {
	kubernetesClient client.Client
}

var _ dataplane.ResourceHandler[dataplane.DeploymentContext] = (*secretProviderClassHandler)(nil)

func NewSecretProviderClassHandler(kubernetesClient client.Client) dataplane.ResourceHandler[dataplane.DeploymentContext] {
	return &secretProviderClassHandler{
		kubernetesClient: kubernetesClient,
	}
}

func (h *secretProviderClassHandler) Name() string {
	return "KubernetesSecretProviderClassHandler"
}

func (h *secretProviderClassHandler) IsRequired(deployCtx *dataplane.DeploymentContext) bool {
	return len(deployCtx.ConfigurationGroups) > 0
}

func (h *secretProviderClassHandler) GetCurrentState(ctx context.Context, deployCtx *dataplane.DeploymentContext) (interface{}, error) {
	namespace := makeNamespaceName(deployCtx)
	labels := makeWorkloadLabels(deployCtx)
	spcList := &csisecretv1.SecretProviderClassList{}
	listOpts := []client.ListOption{
		client.InNamespace(namespace),
		client.MatchingLabels(labels),
	}
	err := h.kubernetesClient.List(ctx, spcList, listOpts...)
	if err != nil {
		return nil, err
	}
	if len(spcList.Items) == 0 {
		return nil, nil
	}
	// Convert the list to a slice of pointers so that it can be compared with the new state
	// during the update operation
	secretProviderClasses := make([]*csisecretv1.SecretProviderClass, 0, len(spcList.Items))
	for i := range spcList.Items {
		secretProviderClasses = append(secretProviderClasses, &spcList.Items[i])
	}
	return secretProviderClasses, nil
}

func (h *secretProviderClassHandler) Create(ctx context.Context, deployCtx *dataplane.DeploymentContext) error {
	secretProviderClasses := makeSecretProviderClasses(deployCtx)
	for _, spc := range secretProviderClasses {
		err := h.kubernetesClient.Create(ctx, spc)
		if err != nil {
			return fmt.Errorf("error while creating SecretProviderClass %s: %w", spc.Name, err)
		}
	}
	return nil
}

func (h *secretProviderClassHandler) Update(ctx context.Context, deployCtx *dataplane.DeploymentContext, currentState interface{}) error {
	currentSPCs, ok := currentState.([]*csisecretv1.SecretProviderClass)
	if !ok {
		return errors.New("failed to cast current state to a slice of SecretProviderClass")
	}

	desiredSPCs := makeSecretProviderClasses(deployCtx)

	// Build a map for quick lookups of current and desired SecretProviderClasses
	// Using "name" as the key
	currentMap := make(map[string]*csisecretv1.SecretProviderClass, len(currentSPCs))
	for _, spc := range currentSPCs {
		currentMap[spc.Name] = spc
	}

	desiredMap := make(map[string]*csisecretv1.SecretProviderClass, len(desiredSPCs))
	for _, spc := range desiredSPCs {
		desiredMap[spc.Name] = spc
	}

	// Create or update the SecretProviderClasses that are not present in the current state
	for name, desiredSPC := range desiredMap {
		existingSPC, found := currentMap[name]
		if !found {
			// Create the SecretProviderClass if it is not present in the current state
			if err := h.kubernetesClient.Create(ctx, desiredSPC); err != nil {
				return fmt.Errorf("error while creating SecretProviderClass %s: %w", desiredSPC.Name, err)
			}
			continue
		}

		// Update the SecretProviderClass that are present in the current state
		if !cmp.Equal(existingSPC.Spec, desiredSPC.Spec) ||
			!cmp.Equal(extractManagedLabels(existingSPC.Labels), extractManagedLabels(desiredSPC.Labels)) {
			// TODO: Auto restart the pods that are using the SecretProviderClass
			updatedSPC := existingSPC.DeepCopy()
			updatedSPC.Spec = desiredSPC.Spec
			updatedSPC.Labels = desiredSPC.Labels

			if err := h.kubernetesClient.Update(ctx, updatedSPC); err != nil {
				return fmt.Errorf("error while updating SecretProviderClass %s: %w", desiredSPC.Name, err)
			}
		}
	}

	// Delete the SecretProviderClass that are not present in the desired state
	for name, existingSPC := range currentMap {
		if _, found := desiredMap[name]; !found {
			if err := h.kubernetesClient.Delete(ctx, existingSPC); err != nil {
				return fmt.Errorf("error while deleting SecretProviderClass %s: %w", existingSPC.Name, err)
			}
		}
	}

	return nil
}

func (h *secretProviderClassHandler) Delete(ctx context.Context, deployCtx *dataplane.DeploymentContext) error {
	namespace := makeNamespaceName(deployCtx)
	labels := makeWorkloadLabels(deployCtx)
	deleteAllOpt := []client.DeleteAllOfOption{
		// Make sure the correct labels are used, otherwise, it might delete unwanted SecretProviderClasses
		client.InNamespace(namespace),
		client.MatchingLabels(labels),
	}
	err := h.kubernetesClient.DeleteAllOf(ctx, &csisecretv1.SecretProviderClass{}, deleteAllOpt...)
	if err != nil {
		return fmt.Errorf("error while deleting SecretProviderClasses: %w", err)
	}
	return nil
}

// This struct is used to marshal the YAML configuration required for the SecretProviderClass
// Object. This will be specific to the HashiCorp Vault provider.
// Example:
// objects: |
//   - objectName: "password"
//     secretPath: "secret/data/redis"
//     secretKey: "password"
//
// TODO: This needs to be extended to support other providers.
type secretProviderObject struct {
	ObjectName string `yaml:"objectName,omitempty"`
	SecretPath string `yaml:"secretPath,omitempty"`
	SecretKey  string `yaml:"secretKey,omitempty"`
}

// makeSecretProviderClasses creates the SecretProviderClass resources for the given deployment context.
// Currently, it only supports HashiCorp Vault as the provider.
// Ref: https://developer.hashicorp.com/vault/docs/platform/k8s/csi/configurations#secret-provider-class-parameters
func makeSecretProviderClasses(deployCtx *dataplane.DeploymentContext) []*csisecretv1.SecretProviderClass {
	secretProviderClasses := make([]*csisecretv1.SecretProviderClass, 0)
	for _, cg := range deployCtx.ConfigurationGroups {
		cgConfigs := cg.Spec.Configurations
		if len(cgConfigs) == 0 {
			continue
		}

		spc := &csisecretv1.SecretProviderClass{
			ObjectMeta: metav1.ObjectMeta{
				Name:      makeSecretProviderClassName(deployCtx, cg),
				Namespace: makeNamespaceName(deployCtx),
				Labels:    makeWorkloadLabels(deployCtx),
			},
			Spec: csisecretv1.SecretProviderClassSpec{
				Provider: hashicorpVaultProvider,
				Parameters: map[string]string{ // TODO: make this vendor specific
					"vaultAddress": hashicorpVaultAddress,
					"roleName":     hashicorpVaultRoleName,
				},
			},
		}
		spObjects := make([]secretProviderObject, 0)
		for _, cgConfig := range cgConfigs {
			cgv := findConfigGroupValueForEnv(cgConfig.Values, cg.Spec.EnvironmentGroups, deployCtx.Environment)
			if cgv == nil || cgv.VaultKey == "" {
				continue
			}
			// TODO: Improvement: filter the values that are only used in deployable artifact
			spObjects = append(spObjects, secretProviderObject{
				ObjectName: cgConfig.Key,
				SecretPath: cgv.VaultKey,
				// The KV engine version 2 has multiple keys, but here we are using only one key to keep it compatible
				// with other providers.
				// In short, we avoid the support of single secret supporting multiple key values.
				SecretKey: "value",
			})
		}

		// If there are no configuration values to add to the SecretProviderClass, skip creating it
		if len(spObjects) == 0 {
			continue
		}
		objYAML, _ := yaml.Marshal(spObjects) // Only primitive types are used, so no error is expected
		spc.Spec.Parameters["objects"] = string(objYAML)
		spc.Spec.SecretObjects = []*csisecretv1.SecretObject{
			makeSecretObject(deployCtx, cg, spObjects),
		}
		secretProviderClasses = append(secretProviderClasses, spc)
	}
	return secretProviderClasses
}

func makeSecretProviderClassName(deployCtx *dataplane.DeploymentContext, cg *choreov1.ConfigurationGroup) string {
	// TODO: Ideally, this should be choreo name instead of kubernetes name
	componentName := deployCtx.Component.Name
	deploymentTrackName := deployCtx.DeploymentTrack.Name
	configGroupName := cg.Name
	// Limit the name to 253 characters to comply with the K8s name length limit for SecretProviderClasses
	return dpkubernetes.GenerateK8sName(componentName, deploymentTrackName, configGroupName)
}

func makeSecretObject(deployCtx *dataplane.DeploymentContext, cg *choreov1.ConfigurationGroup,
	spObjects []secretProviderObject) *csisecretv1.SecretObject {
	// Here, we use the configuration key (objectName) for both the object name and the key for simplicity
	data := make([]*csisecretv1.SecretObjectData, 0, len(spObjects))
	for _, spObject := range spObjects {
		data = append(data, &csisecretv1.SecretObjectData{
			ObjectName: spObject.ObjectName,
			Key:        spObject.ObjectName,
		})
	}
	return &csisecretv1.SecretObject{
		SecretName: makeSecretProviderClassName(deployCtx, cg),
		Type:       "Opaque",
		Labels:     makeWorkloadLabels(deployCtx),
		Data:       data,
	}
}
