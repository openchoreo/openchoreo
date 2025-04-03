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

package resources

import (
	"context"
	"fmt"
	"reflect"
	"sort"

	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	"github.com/openchoreo/openchoreo/internal/dataplane/kubernetes"
	"github.com/openchoreo/openchoreo/pkg/cli/common/constants"
)

type OutputFormat string

const (
	OutputFormatTable OutputFormat = "table"
	OutputFormatYAML  OutputFormat = "yaml"
	OutputFormatJSON  OutputFormat = "json"
)

// ResourceFilter defines criteria for filtering resources
type ResourceFilter struct {
	Name      string
	Labels    map[string]string
	Namespace string
}

// ResourceOperation is the interface for any resource operation.
type ResourceOperation[T client.Object] interface {
	List() ([]ResourceWrapper[T], error)
	Create(obj T) error
	Update(obj T) error
	Delete(name string) error

	GetNames() ([]string, error)
	Exists(name string) (bool, error)

	GetNamespace() string
	GetLabels() map[string]string
	GetConfig() constants.CRDConfig
	SetNamespace(namespace string)

	Print(format OutputFormat, filter *ResourceFilter) error
	PrintItems(items []ResourceWrapper[T], format OutputFormat) error
}

// BaseResource implements the shared logic for resource operations.
type BaseResource[T client.Object, L client.ObjectList] struct {
	client    client.Client
	scheme    *runtime.Scheme
	namespace string
	labels    map[string]string
	config    constants.CRDConfig
}

// NewBaseResource constructs a BaseResource given ResourceOption callbacks.
func NewBaseResource[T client.Object, L client.ObjectList](opts ...ResourceOption[T, L]) *BaseResource[T, L] {
	b := &BaseResource[T, L]{labels: map[string]string{}}
	for _, opt := range opts {
		opt(b)
	}
	return b
}

// List lists objects matching namespace/labels.
func (b *BaseResource[T, L]) List() ([]ResourceWrapper[T], error) {
	var zero []ResourceWrapper[T]

	list := newPtrTypeOf[L]()

	if err := b.client.List(context.Background(), list,
		client.InNamespace(b.namespace),
		client.MatchingLabels(b.labels),
	); err != nil {
		return zero, fmt.Errorf("failed to list resources: %w", err)
	}

	itemsVal := reflect.ValueOf(list).Elem().FieldByName("Items")
	if !itemsVal.IsValid() {
		return zero, fmt.Errorf("invalid list type: Items field not found")
	}

	results := make([]ResourceWrapper[T], 0, itemsVal.Len())
	for i := 0; i < itemsVal.Len(); i++ {
		rawAddr := itemsVal.Index(i).Addr().Interface()
		item, ok := rawAddr.(T)
		if !ok {
			return zero, fmt.Errorf("item is not of type T")
		}

		wrapper := ResourceWrapper[T]{
			Resource:       item,
			KubernetesName: item.GetName(),
			LogicalName:    item.GetName(),
		}

		// If resource name is stored in a label, set the logical name from that label
		if choreoName, ok := item.GetLabels()[constants.LabelName]; ok {
			wrapper.LogicalName = choreoName
		}

		results = append(results, wrapper)
	}
	return results, nil
}

// Create creates a K8s resource.
func (b *BaseResource[T, L]) Create(obj T) error {
	return b.client.Create(context.Background(), obj)
}

// Update updates a K8s resource.
func (b *BaseResource[T, L]) Update(obj T) error {
	return b.client.Update(context.Background(), obj)
}

// Delete removes one or more matching resources by name.
func (b *BaseResource[T, L]) Delete(name string) error {
	items, err := b.List()
	if err != nil {
		return fmt.Errorf("failed to list before delete: %w", err)
	}
	for _, item := range items {
		if item.Resource.GetName() == name {
			if err := b.client.Delete(context.Background(), item.Resource); err != nil {
				return fmt.Errorf("failed to delete resource: %w", err)
			}
		}
	}
	return nil
}

// GetNames returns sorted names of resources.
func (b *BaseResource[T, L]) GetNames() ([]string, error) {
	items, err := b.List()
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(items))
	for _, i := range items {
		names = append(names, i.GetName())
	}
	sort.Strings(names)
	return names, nil
}

// Exists returns true if a resource with the given name exists.
func (b *BaseResource[T, L]) Exists(name string) (bool, error) {
	items, err := b.List()
	if err != nil {
		return false, err
	}
	for _, i := range items {
		if i.GetName() == name {
			return true, nil
		}
	}
	return false, nil
}

func (b *BaseResource[T, L]) GetNamespace() string {
	return b.namespace
}

func (b *BaseResource[T, L]) GetConfig() constants.CRDConfig {
	return b.config
}

// WithNamespace sets the namespace on the resource
func (b *BaseResource[T, L]) WithNamespace(namespace string) {
	b.namespace = namespace
}

// Print outputs resources in the specified format with optional filtering
func (b *BaseResource[T, L]) Print(format OutputFormat, filter *ResourceFilter) error {
	items, err := b.List()
	if err != nil {
		return err
	}

	if filter != nil && filter.Name != "" {
		filtered, err := FilterByName(items, filter.Name)
		if err != nil {
			return err
		}
		items = filtered
	}

	if filter != nil && len(filter.Labels) > 0 {
		var filtered []ResourceWrapper[T]
		for _, item := range items {
			matches := true
			itemLabels := item.Resource.GetLabels()
			for k, v := range filter.Labels {
				if itemLabels[k] != v {
					matches = false
					break
				}
			}
			if matches {
				filtered = append(filtered, item)
			}
		}
		items = filtered
	}

	return b.PrintItems(items, format)
}

// PrintItems outputs pre-filtered items in the specified format
func (b *BaseResource[T, L]) PrintItems(items []ResourceWrapper[T], format OutputFormat) error {
	switch format {
	case OutputFormatTable:
		return b.PrintTableItems(items)
	case OutputFormatYAML:
		return b.printYAMLItems(items)
	default:
		return fmt.Errorf("unsupported output format: %s", format)
	}
}

func (b *BaseResource[T, L]) PrintTableItems(items []ResourceWrapper[T]) error {
	if len(items) == 0 {
		fmt.Println("No resources found")
		return nil
	}

	// Basic table implementation for any client.Object
	headers := []string{"NAME", "ORGANIZATION", "AGE"}
	rows := make([][]string, 0, len(items))

	for _, wrapper := range items {
		resource := wrapper.GetResource()
		name := wrapper.GetName()
		namespace := resource.GetNamespace()
		creationTime := resource.GetCreationTimestamp().Time
		age := FormatAge(creationTime)

		rows = append(rows, []string{
			name,
			namespace,
			age,
		})
	}

	return PrintTable(headers, rows)
}

// printYAMLItems outputs the provided items in YAML format
func (b *BaseResource[T, L]) printYAMLItems(items []ResourceWrapper[T]) error {
	if len(items) == 0 {
		return nil
	}

	for _, item := range items {
		clean := item.Resource.DeepCopyObject().(T)
		clean.SetManagedFields(nil)
		clean.SetResourceVersion("")
		clean.SetUID("")
		clean.SetGeneration(0)

		yamlBytes, err := yaml.Marshal(clean)
		if err != nil {
			return fmt.Errorf("failed to marshal resource to YAML: %w", err)
		}
		fmt.Printf("---\n%s\n", string(yamlBytes))
	}
	return nil
}

// newPtrTypeOf returns a fresh pointer for lists (e.g. &choreov1.BuildList{})
func newPtrTypeOf[U any]() U {
	t := reflect.TypeOf((*U)(nil)).Elem()
	if t.Kind() != reflect.Pointer {
		panic("U must be a pointer type, e.g. *BuildList")
	}
	elem := t.Elem()
	v := reflect.New(elem).Interface()
	return v.(U)
}

type ResourceKind[T client.Object, L client.ObjectList] struct {
	client    client.Client
	namespace string
	labels    map[string]string
	config    constants.CRDConfig
}

func NewResourceKind[T client.Object, L client.ObjectList]() *ResourceKind[T, L] {
	return &ResourceKind[T, L]{}
}

func (k *ResourceKind[T, L]) WithClient() ResourceOption[T, L] {
	return func(br *BaseResource[T, L]) {
		br.client = k.client
	}
}

func (k *ResourceKind[T, L]) WithNamespace() ResourceOption[T, L] {
	return func(br *BaseResource[T, L]) {
		br.namespace = k.namespace
	}
}

func (k *ResourceKind[T, L]) WithLabels() ResourceOption[T, L] {
	return func(br *BaseResource[T, L]) {
		br.labels = k.labels
	}
}

func (k *ResourceKind[T, L]) WithConfig() ResourceOption[T, L] {
	return func(br *BaseResource[T, L]) {
		br.config = k.config
	}
}

// FilterByName returns only items matching the given logical name (or all if name == "").
func FilterByName[T client.Object](items []ResourceWrapper[T], name string) ([]ResourceWrapper[T], error) {
	if name == "" {
		return items, nil
	}
	var filtered []ResourceWrapper[T]
	for _, wrapper := range items {
		if wrapper.GetName() == name {
			filtered = append(filtered, wrapper)
		}
	}
	if len(filtered) == 0 {
		return nil, fmt.Errorf("%T named %q not found", new(T), name)
	}
	return filtered, nil
}

func GenerateResourceName(parts ...string) string {
	return kubernetes.GenerateK8sName(parts...)
}

func (b *BaseResource[T, L]) GetClient() client.Client {
	return b.client
}

func DefaultIfEmpty(value, defaultValue string) string {
	if value == "" {
		return defaultValue
	}
	return value
}

func (b *BaseResource[T, L]) GetLabels() map[string]string {
	return b.labels
}
