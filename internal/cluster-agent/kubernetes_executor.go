// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clusteragent

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	"github.com/openchoreo/openchoreo/internal/cluster-agent/messaging"
)

type KubernetesExecutor struct {
	client client.Client
}

func NewKubernetesExecutor(client client.Client) *KubernetesExecutor {
	return &KubernetesExecutor{
		client: client,
	}
}

func (ke *KubernetesExecutor) ExecuteClusterAgentRequest(ctx context.Context, req *messaging.ClusterAgentRequest) (*messaging.ClusterAgentResponse, error) {
	action := messaging.Action(req.Identifier)

	switch action {
	case messaging.ActionApplyResource:
		return ke.applyResource(ctx, req)
	case messaging.ActionListResources:
		return ke.listResources(ctx, req)
	case messaging.ActionGetResource:
		return ke.getResource(ctx, req)
	case messaging.ActionDeleteResource:
		return ke.deleteResource(ctx, req)
	case messaging.ActionPatchResource:
		return ke.patchResource(ctx, req)
	case messaging.ActionCreateNamespace:
		return ke.createNamespace(ctx, req)
	default:
		return messaging.NewClusterAgentFailResponse(req, 400, fmt.Sprintf("unsupported action: %s", req.Identifier), nil), nil
	}
}

func (ke *KubernetesExecutor) applyResource(ctx context.Context, req *messaging.ClusterAgentRequest) (*messaging.ClusterAgentResponse, error) {
	manifestData, ok := req.Payload["manifest"]
	if !ok {
		return messaging.NewClusterAgentFailResponse(req, 400, "missing manifest in payload", nil), nil
	}

	obj := &unstructured.Unstructured{}
	manifestMap, ok := manifestData.(map[string]interface{})
	if ok {
		obj.Object = manifestMap
	} else {
		manifestYAML, ok := manifestData.(string)
		if !ok {
			return messaging.NewClusterAgentFailResponse(req, 400, "manifest must be a YAML string or object", nil), nil
		}
		if err := yaml.Unmarshal([]byte(manifestYAML), obj); err != nil {
			return messaging.NewClusterAgentFailResponse(req, 400, fmt.Sprintf("failed to parse manifest: %v", err), nil), nil
		}
	}

	// Get field manager (default to "openchoreo-agent")
	fieldManager := "openchoreo-agent"
	if fm, ok := req.Payload["fieldManager"].(string); ok && fm != "" {
		fieldManager = fm
	}

	if err := ke.client.Patch(ctx, obj, client.Apply, client.ForceOwnership, client.FieldOwner(fieldManager)); err != nil {
		return messaging.NewClusterAgentFailResponse(req, 500, fmt.Sprintf("failed to apply resource: %v", err), map[string]interface{}{
			"retryable": true,
		}), nil
	}

	return messaging.NewClusterAgentSuccessResponse(req, map[string]interface{}{
		"applied":           true,
		"resourceVersion":   obj.GetResourceVersion(),
		"uid":               obj.GetUID(),
		"creationTimestamp": obj.GetCreationTimestamp().Format("2006-01-02T15:04:05Z"),
	}), nil
}

func (ke *KubernetesExecutor) listResources(ctx context.Context, req *messaging.ClusterAgentRequest) (*messaging.ClusterAgentResponse, error) {
	gvkMap, ok := req.Payload["gvk"].(map[string]interface{})
	if !ok {
		return messaging.NewClusterAgentFailResponse(req, 400, "missing or invalid gvk in payload", nil), nil
	}

	gvk := schema.GroupVersionKind{
		Group:   getStringFromMap(gvkMap, "group"),
		Version: getStringFromMap(gvkMap, "version"),
		Kind:    getStringFromMap(gvkMap, "kind"),
	}

	list := &unstructured.UnstructuredList{}
	list.SetGroupVersionKind(gvk)

	listOpts := []client.ListOption{}
	if namespace, ok := req.Payload["namespace"].(string); ok && namespace != "" {
		listOpts = append(listOpts, client.InNamespace(namespace))
	}
	if labelsMap, ok := req.Payload["labelSelector"].(map[string]interface{}); ok {
		labels := make(map[string]string)
		for k, v := range labelsMap {
			if strVal, ok := v.(string); ok {
				labels[k] = strVal
			}
		}
		if len(labels) > 0 {
			listOpts = append(listOpts, client.MatchingLabels(labels))
		}
	}

	if err := ke.client.List(ctx, list, listOpts...); err != nil {
		return messaging.NewClusterAgentFailResponse(req, 500, fmt.Sprintf("failed to list resources: %v", err), nil), nil
	}

	items := make([]map[string]interface{}, len(list.Items))
	for i, item := range list.Items {
		items[i] = item.Object
	}

	return messaging.NewClusterAgentSuccessResponse(req, map[string]interface{}{
		"items": items,
		"count": len(items),
	}), nil
}

func (ke *KubernetesExecutor) getResource(ctx context.Context, req *messaging.ClusterAgentRequest) (*messaging.ClusterAgentResponse, error) {
	gvkMap, ok := req.Payload["gvk"].(map[string]interface{})
	if !ok {
		return messaging.NewClusterAgentFailResponse(req, 400, "missing or invalid gvk in payload", nil), nil
	}

	gvk := schema.GroupVersionKind{
		Group:   getStringFromMap(gvkMap, "group"),
		Version: getStringFromMap(gvkMap, "version"),
		Kind:    getStringFromMap(gvkMap, "kind"),
	}

	name, ok := req.Payload["name"].(string)
	if !ok || name == "" {
		return messaging.NewClusterAgentFailResponse(req, 400, "missing or invalid name in payload", nil), nil
	}

	namespace, _ := req.Payload["namespace"].(string)

	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(gvk)

	if err := ke.client.Get(ctx, client.ObjectKey{Name: name, Namespace: namespace}, obj); err != nil {
		return messaging.NewClusterAgentFailResponse(req, 404, fmt.Sprintf("resource not found: %v", err), nil), nil
	}

	return messaging.NewClusterAgentSuccessResponse(req, map[string]interface{}{
		"resource": obj.Object,
	}), nil
}

func (ke *KubernetesExecutor) deleteResource(ctx context.Context, req *messaging.ClusterAgentRequest) (*messaging.ClusterAgentResponse, error) {
	manifestData, ok := req.Payload["manifest"]
	if !ok {
		return messaging.NewClusterAgentFailResponse(req, 400, "missing manifest in payload", nil), nil
	}

	obj := &unstructured.Unstructured{}
	manifestMap, ok := manifestData.(map[string]interface{})
	if ok {
		obj.Object = manifestMap
	} else {
		manifestYAML, ok := manifestData.(string)
		if !ok {
			return messaging.NewClusterAgentFailResponse(req, 400, "manifest must be a YAML string or object", nil), nil
		}
		if err := yaml.Unmarshal([]byte(manifestYAML), obj); err != nil {
			return messaging.NewClusterAgentFailResponse(req, 400, fmt.Sprintf("failed to parse manifest: %v", err), nil), nil
		}
	}

	if err := ke.client.Delete(ctx, obj); err != nil {
		return messaging.NewClusterAgentFailResponse(req, 500, fmt.Sprintf("failed to delete resource: %v", err), nil), nil
	}

	return messaging.NewClusterAgentSuccessResponse(req, map[string]interface{}{
		"deleted": true,
		"name":    obj.GetName(),
	}), nil
}

func (ke *KubernetesExecutor) patchResource(ctx context.Context, req *messaging.ClusterAgentRequest) (*messaging.ClusterAgentResponse, error) {
	manifestData, ok := req.Payload["manifest"]
	if !ok {
		return messaging.NewClusterAgentFailResponse(req, 400, "missing manifest in payload", nil), nil
	}

	obj := &unstructured.Unstructured{}
	manifestMap, ok := manifestData.(map[string]interface{})
	if ok {
		obj.Object = manifestMap
	} else {
		manifestYAML, ok := manifestData.(string)
		if !ok {
			return messaging.NewClusterAgentFailResponse(req, 400, "manifest must be a YAML string or object", nil), nil
		}
		if err := yaml.Unmarshal([]byte(manifestYAML), obj); err != nil {
			return messaging.NewClusterAgentFailResponse(req, 400, fmt.Sprintf("failed to parse manifest: %v", err), nil), nil
		}
	}

	if err := ke.client.Patch(ctx, obj, client.Merge); err != nil {
		return messaging.NewClusterAgentFailResponse(req, 500, fmt.Sprintf("failed to patch resource: %v", err), nil), nil
	}

	return messaging.NewClusterAgentSuccessResponse(req, map[string]interface{}{
		"patched":         true,
		"resourceVersion": obj.GetResourceVersion(),
	}), nil
}

func (ke *KubernetesExecutor) createNamespace(ctx context.Context, req *messaging.ClusterAgentRequest) (*messaging.ClusterAgentResponse, error) {
	namespace, ok := req.Payload["namespace"].(string)
	if !ok || namespace == "" {
		return messaging.NewClusterAgentFailResponse(req, 400, "missing or invalid namespace in payload", nil), nil
	}

	ns := &unstructured.Unstructured{}
	ns.SetAPIVersion("v1")
	ns.SetKind("Namespace")
	ns.SetName(namespace)

	if err := ke.client.Create(ctx, ns); err != nil {
		if client.IgnoreAlreadyExists(err) != nil {
			return messaging.NewClusterAgentFailResponse(req, 500, fmt.Sprintf("failed to create namespace: %v", err), nil), nil
		}
	}

	return messaging.NewClusterAgentSuccessResponse(req, map[string]interface{}{
		"namespace": namespace,
		"created":   true,
	}), nil
}
