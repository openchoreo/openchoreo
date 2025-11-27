// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package kubernetes

import (
	"context"
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	k8sscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/openchoreo/openchoreo/internal/cluster-agent/messaging"
	clustergateway "github.com/openchoreo/openchoreo/internal/cluster-gateway"
)

const (
	patchTypeApply = "apply"
)

type AgentClient struct {
	planeName      string
	server         clustergateway.Dispatcher
	requestTimeout time.Duration
	scheme         *runtime.Scheme
}

func NewAgentClient(planeName string, agentSrv clustergateway.Dispatcher, requestTimeout time.Duration) (client.Client, error) {
	return &AgentClient{
		planeName:      planeName,
		server:         agentSrv,
		requestTimeout: requestTimeout,
		scheme:         k8sscheme.Scheme, // Use the global scheme from kubernetes package
	}, nil
}

func (ac *AgentClient) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	gvk, err := getGVK(obj, ac.scheme)
	if err != nil {
		return err
	}

	payload := map[string]interface{}{
		"gvk": map[string]interface{}{
			"group":   gvk.Group,
			"version": gvk.Version,
			"kind":    gvk.Kind,
		},
		"name":      key.Name,
		"namespace": key.Namespace,
	}

	response, err := ac.server.SendClusterAgentRequest(ac.planeName, messaging.TypeQuery, "get-resource", payload, ac.requestTimeout)
	if err != nil {
		return fmt.Errorf("failed to send get request: %w", err)
	}

	if response.Status != messaging.StatusSuccess {
		return convertResponseError(response, gvk, key.Name)
	}

	resourceMap, ok := response.Payload["resource"].(map[string]interface{})
	if !ok {
		return fmt.Errorf("invalid response: resource is not an object")
	}

	unstructuredObj := &unstructured.Unstructured{Object: resourceMap}
	return runtime.DefaultUnstructuredConverter.FromUnstructured(unstructuredObj.Object, obj)
}

func (ac *AgentClient) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	gvk, err := getGVKForList(list, ac.scheme)
	if err != nil {
		return err
	}

	listOpts := &client.ListOptions{}
	for _, opt := range opts {
		opt.ApplyToList(listOpts)
	}

	// Create request payload
	payload := map[string]interface{}{
		"gvk": map[string]interface{}{
			"group":   gvk.Group,
			"version": gvk.Version,
			"kind":    gvk.Kind,
		},
	}

	if listOpts.Namespace != "" {
		payload["namespace"] = listOpts.Namespace
	}

	if listOpts.LabelSelector != nil {
		// Convert label selector to map
		labelMap := make(map[string]string)
		requirements, _ := listOpts.LabelSelector.Requirements()
		for _, req := range requirements {
			if req.Operator() == "=" || req.Operator() == "==" {
				values := req.Values()
				if values.Len() > 0 {
					labelMap[req.Key()] = values.List()[0]
				}
			}
		}
		if len(labelMap) > 0 {
			payload["labelSelector"] = labelMap
		}
	}

	response, err := ac.server.SendClusterAgentRequest(ac.planeName, messaging.TypeQuery, "list-resources", payload, ac.requestTimeout)
	if err != nil {
		return fmt.Errorf("failed to send list request: %w", err)
	}

	if response.Status != messaging.StatusSuccess {
		return convertResponseError(response, gvk, "")
	}

	itemsInterface, ok := response.Payload["items"].([]interface{})
	if !ok {
		return fmt.Errorf("invalid response: items is not an array")
	}

	resources := make([]*unstructured.Unstructured, 0, len(itemsInterface))
	for _, itemInterface := range itemsInterface {
		itemMap, ok := itemInterface.(map[string]interface{})
		if !ok {
			continue
		}
		resources = append(resources, &unstructured.Unstructured{Object: itemMap})
	}

	return setListItems(list, resources)
}

func (ac *AgentClient) Create(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {
	gvk, err := getGVK(obj, ac.scheme)
	if err != nil {
		return fmt.Errorf("failed to get GVK: %w", err)
	}

	unstructuredObj, err := runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
	if err != nil {
		return fmt.Errorf("failed to convert object to unstructured: %w", err)
	}

	unstructured := &unstructured.Unstructured{Object: unstructuredObj}
	unstructured.SetGroupVersionKind(gvk)

	payload := map[string]interface{}{
		"manifest": unstructured.Object,
	}

	response, err := ac.server.SendClusterAgentRequest(ac.planeName, messaging.TypeCommand, "apply-resource", payload, ac.requestTimeout)
	if err != nil {
		return fmt.Errorf("failed to send create request: %w", err)
	}

	if response.Status != messaging.StatusSuccess {
		gvk, _ := getGVK(obj, ac.scheme)
		return convertResponseError(response, gvk, obj.GetName())
	}

	// Update object with metadata from response (resourceVersion, UID, etc.)
	if resourceVersion, ok := response.Payload["resourceVersion"].(string); ok {
		obj.SetResourceVersion(resourceVersion)
	}
	if uid, ok := response.Payload["uid"].(string); ok {
		obj.SetUID(types.UID(uid))
	}

	return nil
}

func (ac *AgentClient) Update(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
	gvk, err := getGVK(obj, ac.scheme)
	if err != nil {
		return fmt.Errorf("failed to get GVK: %w", err)
	}

	unstructuredObj, err := runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
	if err != nil {
		return fmt.Errorf("failed to convert object to unstructured: %w", err)
	}

	unstructured := &unstructured.Unstructured{Object: unstructuredObj}
	unstructured.SetGroupVersionKind(gvk)

	payload := map[string]interface{}{
		"manifest": unstructured.Object,
	}

	response, err := ac.server.SendClusterAgentRequest(ac.planeName, messaging.TypeCommand, "apply-resource", payload, ac.requestTimeout)
	if err != nil {
		return fmt.Errorf("failed to send update request: %w", err)
	}

	if response.Status != messaging.StatusSuccess {
		gvk, _ := getGVK(obj, ac.scheme)
		return convertResponseError(response, gvk, obj.GetName())
	}

	if resourceVersion, ok := response.Payload["resourceVersion"].(string); ok {
		obj.SetResourceVersion(resourceVersion)
	}

	return nil
}

func (ac *AgentClient) Patch(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
	patchOpts := &client.PatchOptions{}
	for _, opt := range opts {
		opt.ApplyToPatch(patchOpts)
	}

	fieldManager := "openchoreo-agent-client"
	if patchOpts.FieldManager != "" {
		fieldManager = patchOpts.FieldManager
	}

	gvk, err := getGVK(obj, ac.scheme)
	if err != nil {
		return fmt.Errorf("failed to get GVK: %w", err)
	}

	unstructuredObj, err := runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
	if err != nil {
		return fmt.Errorf("failed to convert object to unstructured: %w", err)
	}

	unstructured := &unstructured.Unstructured{Object: unstructuredObj}
	unstructured.SetGroupVersionKind(gvk)

	patchType := patchTypeApply
	identifier := "apply-resource"

	switch patch.Type() {
	case types.ApplyPatchType:
		patchType = patchTypeApply
		identifier = "apply-resource"
	case types.MergePatchType:
		patchType = "merge"
		identifier = "patch-resource"
	case types.StrategicMergePatchType:
		patchType = "strategic"
		identifier = "patch-resource"
	}

	payload := map[string]interface{}{
		"manifest":     unstructured.Object,
		"fieldManager": fieldManager,
	}

	if patchType != patchTypeApply {
		payload["patchType"] = patchType
	}

	response, err := ac.server.SendClusterAgentRequest(ac.planeName, messaging.TypeCommand, identifier, payload, ac.requestTimeout)
	if err != nil {
		return fmt.Errorf("failed to send patch request: %w", err)
	}

	if response.Status != messaging.StatusSuccess {
		gvk, _ := getGVK(obj, ac.scheme)
		return convertResponseError(response, gvk, obj.GetName())
	}

	if resourceVersion, ok := response.Payload["resourceVersion"].(string); ok {
		obj.SetResourceVersion(resourceVersion)
	}

	return nil
}

func (ac *AgentClient) Delete(ctx context.Context, obj client.Object, opts ...client.DeleteOption) error {
	gvk, err := getGVK(obj, ac.scheme)
	if err != nil {
		return fmt.Errorf("failed to get GVK: %w", err)
	}

	unstructuredObj, err := runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
	if err != nil {
		return fmt.Errorf("failed to convert object to unstructured: %w", err)
	}

	unstructured := &unstructured.Unstructured{Object: unstructuredObj}
	unstructured.SetGroupVersionKind(gvk)

	payload := map[string]interface{}{
		"manifest": unstructured.Object,
	}

	response, err := ac.server.SendClusterAgentRequest(ac.planeName, messaging.TypeCommand, "delete-resource", payload, ac.requestTimeout)
	if err != nil {
		return fmt.Errorf("failed to send delete request: %w", err)
	}

	if response.Status != messaging.StatusSuccess {
		gvk, _ := getGVK(obj, ac.scheme)
		return convertResponseError(response, gvk, obj.GetName())
	}

	return nil
}

func (ac *AgentClient) DeleteAllOf(ctx context.Context, obj client.Object, opts ...client.DeleteAllOfOption) error {
	deleteAllOfOpts := &client.DeleteAllOfOptions{}
	for _, opt := range opts {
		opt.ApplyToDeleteAllOf(deleteAllOfOpts)
	}

	list := &unstructured.UnstructuredList{}
	if err := ac.List(ctx, list, &deleteAllOfOpts.ListOptions); err != nil {
		return fmt.Errorf("failed to list resources for delete all: %w", err)
	}

	for _, item := range list.Items {
		if err := ac.Delete(ctx, &item); err != nil {
			return fmt.Errorf("failed to delete resource %s/%s: %w", item.GetNamespace(), item.GetName(), err)
		}
	}

	return nil
}

// Status returns a status writer for updating resource status
func (ac *AgentClient) Status() client.StatusWriter {
	return &agentStatusWriter{client: ac}
}

// Scheme returns the scheme this client is using
func (ac *AgentClient) Scheme() *runtime.Scheme {
	return ac.scheme
}

// RESTMapper returns the REST mapper
func (ac *AgentClient) RESTMapper() meta.RESTMapper {
	// For agent client, we don't need a REST mapper as we work with unstructured objects
	return nil
}

// GroupVersionKindFor returns the GVK for an object
func (ac *AgentClient) GroupVersionKindFor(obj runtime.Object) (schema.GroupVersionKind, error) {
	return getGVK(obj, ac.scheme)
}

func (ac *AgentClient) IsObjectNamespaced(obj runtime.Object) (bool, error) {
	gvk, err := ac.GroupVersionKindFor(obj)
	if err != nil {
		return false, err
	}

	namespacedKinds := []string{
		"Pod", "Service", "Deployment", "StatefulSet", "DaemonSet",
		"ConfigMap", "Secret", "PersistentVolumeClaim", "Job", "CronJob",
		"Ingress", "NetworkPolicy", "ServiceAccount", "Role", "RoleBinding",
	}

	for _, kind := range namespacedKinds {
		if gvk.Kind == kind {
			return true, nil
		}
	}

	clusterKinds := []string{
		"Namespace", "Node", "PersistentVolume", "StorageClass",
		"ClusterRole", "ClusterRoleBinding", "CustomResourceDefinition",
	}

	for _, kind := range clusterKinds {
		if gvk.Kind == kind {
			return false, nil
		}
	}

	return true, nil
}

func (ac *AgentClient) SubResource(subResource string) client.SubResourceClient {
	return &agentSubResourceClient{
		client:      ac,
		subResource: subResource,
	}
}

type agentStatusWriter struct {
	client *AgentClient
}

func (asw *agentStatusWriter) Update(ctx context.Context, obj client.Object, opts ...client.SubResourceUpdateOption) error {
	return asw.client.Update(ctx, obj)
}

func (asw *agentStatusWriter) Patch(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.SubResourcePatchOption) error {
	return asw.client.Patch(ctx, obj, patch)
}

func (asw *agentStatusWriter) Create(ctx context.Context, obj client.Object, subResource client.Object, opts ...client.SubResourceCreateOption) error {
	return fmt.Errorf("status create not supported by agent client")
}

type agentSubResourceClient struct {
	client      *AgentClient
	subResource string
}

func (asrc *agentSubResourceClient) Get(ctx context.Context, obj client.Object, subResource client.Object, opts ...client.SubResourceGetOption) error {
	return fmt.Errorf("subresource get not yet implemented")
}

func (asrc *agentSubResourceClient) Create(ctx context.Context, obj client.Object, subResource client.Object, opts ...client.SubResourceCreateOption) error {
	return fmt.Errorf("subresource create not yet implemented")
}

func (asrc *agentSubResourceClient) Update(ctx context.Context, obj client.Object, opts ...client.SubResourceUpdateOption) error {
	return asrc.client.Update(ctx, obj)
}

func (asrc *agentSubResourceClient) Patch(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.SubResourcePatchOption) error {
	return asrc.client.Patch(ctx, obj, patch)
}

// convertResponseError converts an agent response error to a Kubernetes API error
func convertResponseError(response *messaging.ClusterAgentResponse, gvk schema.GroupVersionKind, name string) error {
	if response.Error == nil {
		return fmt.Errorf("request failed with unknown error")
	}

	errMsg := response.Error.Message
	statusCode := response.Error.Code

	// Convert HTTP status codes to appropriate Kubernetes API errors
	gr := schema.GroupResource{
		Group:    gvk.Group,
		Resource: gvk.Kind, // Using Kind as Resource (good enough for error reporting)
	}

	switch statusCode {
	case 404:
		return errors.NewNotFound(gr, name)
	case 409:
		return errors.NewAlreadyExists(gr, name)
	case 403:
		return errors.NewForbidden(gr, name, fmt.Errorf("%s", errMsg))
	case 401:
		return errors.NewUnauthorized(errMsg)
	case 400:
		return errors.NewBadRequest(errMsg)
	case 500, 503:
		return errors.NewInternalError(fmt.Errorf("%s", errMsg))
	default:
		return fmt.Errorf("request failed: %s", errMsg)
	}
}

func getGVK(obj runtime.Object, scheme *runtime.Scheme) (schema.GroupVersionKind, error) {
	gvks, _, err := scheme.ObjectKinds(obj)
	if err != nil {
		return schema.GroupVersionKind{}, fmt.Errorf("failed to get GVK: %w", err)
	}

	if len(gvks) == 0 {
		return schema.GroupVersionKind{}, fmt.Errorf("no GVK found for object")
	}

	return gvks[0], nil
}

func getGVKForList(list client.ObjectList, scheme *runtime.Scheme) (schema.GroupVersionKind, error) {
	gvks, _, err := scheme.ObjectKinds(list)
	if err != nil {
		return schema.GroupVersionKind{}, fmt.Errorf("failed to get GVK for list: %w", err)
	}

	if len(gvks) == 0 {
		return schema.GroupVersionKind{}, fmt.Errorf("no GVK found for list")
	}

	// Convert List GVK to item GVK (e.g., PodList -> Pod)
	gvk := gvks[0]
	if len(gvk.Kind) > 4 && gvk.Kind[len(gvk.Kind)-4:] == "List" {
		gvk.Kind = gvk.Kind[:len(gvk.Kind)-4]
	}

	return gvk, nil
}

func setListItems(list client.ObjectList, items []*unstructured.Unstructured) error {
	listPtr, ok := list.(*unstructured.UnstructuredList)
	if !ok {
		return fmt.Errorf("list is not an UnstructuredList")
	}

	listPtr.Items = make([]unstructured.Unstructured, len(items))
	for i, item := range items {
		listPtr.Items[i] = *item
	}

	return nil
}
