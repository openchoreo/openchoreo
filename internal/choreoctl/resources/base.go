// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package resources

import (
	"context"
	"fmt"
	"reflect"
	"sort"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	"github.com/openchoreo/openchoreo/internal/dataplane/kubernetes"
	"github.com/openchoreo/openchoreo/pkg/cli/common/constants"
	pkgconstants "github.com/openchoreo/openchoreo/pkg/constants"
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
	Limit     int // Maximum number of resources to return (0 for unlimited)
}

// ResourceOperation is the interface for any resource operation.
type ResourceOperation[T client.Object] interface {
	List(limit int) ([]ResourceWrapper[T], error)
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

// List lists objects matching namespace/labels with optional server-side pagination.
// When limit > 0: Auto-pages until the total item cap is reached
// When limit = 0: Auto-pages through all results using continue tokens
// Returns an error if limit is negative
func (b *BaseResource[T, L]) List(limit int) ([]ResourceWrapper[T], error) {
	var zero []ResourceWrapper[T]

	// Validate limit parameter
	if limit < 0 {
		return zero, fmt.Errorf("limit must be non-negative, got %d", limit)
	}

	// Set up base list options
	baseListOpts := []client.ListOption{
		client.InNamespace(b.namespace),
		client.MatchingLabels(b.labels),
	}

	// Auto-paging mode: fetch pages using continue tokens
	var allResults []ResourceWrapper[T]
	continueToken := ""
	pageSize := int64(pkgconstants.DefaultPageLimit)
	if pageSize > int64(pkgconstants.MaxPageLimit) {
		pageSize = int64(pkgconstants.MaxPageLimit)
	}

	// Respect the caller-provided cap by reducing page size when close to the limit
	if limit > 0 && limit < int(pageSize) {
		pageSize = int64(limit)
		if pageSize > int64(pkgconstants.MaxPageLimit) {
			pageSize = int64(pkgconstants.MaxPageLimit)
		}
	}

	for {
		if limit > 0 {
			remaining := limit - len(allResults)
			if remaining <= 0 {
				break
			}
			if int64(remaining) < pageSize {
				pageSize = int64(remaining)
			}
		}

		results, nextToken, err := b.listPageWithToken(baseListOpts, pageSize, continueToken)
		if err != nil {
			return zero, err
		}

		allResults = append(allResults, results...)

		// Stop if we've reached the requested cap
		if limit > 0 && len(allResults) >= limit {
			if len(allResults) > limit {
				allResults = allResults[:limit]
			}
			break
		}

		// Check if there are more pages
		if nextToken == "" {
			break
		}
		continueToken = nextToken
	}

	return allResults, nil
}

// listPageWithToken fetches a page of results and returns the continue token for the next page
func (b *BaseResource[T, L]) listPageWithToken(baseOpts []client.ListOption, limit int64, continueToken string) ([]ResourceWrapper[T], string, error) {
	var zero []ResourceWrapper[T]

	list := newPtrTypeOf[L]()

	listOpts := make([]client.ListOption, len(baseOpts))
	copy(listOpts, baseOpts)

	if limit > 0 {
		listOpts = append(listOpts, client.Limit(limit))
	}
	if continueToken != "" {
		listOpts = append(listOpts, client.Continue(continueToken))
	}

	if err := b.client.List(context.Background(), list, listOpts...); err != nil {
		return zero, "", fmt.Errorf("failed to list resources: %w", err)
	}

	listVal := reflect.ValueOf(list).Elem()
	listInterface, ok := any(list).(metav1.ListInterface)
	if !ok {
		return zero, "", fmt.Errorf("invalid list type: does not implement metav1.ListInterface")
	}
	nextToken := listInterface.GetContinue()

	itemsVal := listVal.FieldByName("Items")
	if !itemsVal.IsValid() {
		return zero, "", fmt.Errorf("invalid list type: Items field not found")
	}

	results := make([]ResourceWrapper[T], 0, itemsVal.Len())
	for i := 0; i < itemsVal.Len(); i++ {
		elem := itemsVal.Index(i)

		var rawItem interface{}
		// Check if slice elements are already pointers
		if elem.Kind() == reflect.Pointer {
			// Elements are pointers, use directly
			rawItem = elem.Interface()
		} else {
			// Elements are values, take address
			rawItem = elem.Addr().Interface()
		}

		item, ok := rawItem.(T)
		if !ok {
			return zero, "", fmt.Errorf("item is not of type T")
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

	return results, nextToken, nil
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
	// Prefer server-side label lookup (logical name) to avoid listing the whole namespace.
	items, err := b.listByLogicalName(name, 0)
	if err != nil {
		return fmt.Errorf("failed to list before delete: %w", err)
	}

	// Fallback: page through results and stop once we find a match.
	if len(items) == 0 {
		items, err = b.searchByNamePaged(name, 1)
		if err != nil {
			return fmt.Errorf("failed to search before delete: %w", err)
		}
	}

	for _, item := range items {
		if err := b.client.Delete(context.Background(), item.Resource); err != nil {
			return fmt.Errorf("failed to delete resource: %w", err)
		}
	}
	return nil
}

// GetNames returns sorted names of resources.
func (b *BaseResource[T, L]) GetNames() ([]string, error) {
	// GetNames still needs to return the full set, but page through results to
	// avoid holding every full object in memory at once.
	baseListOpts := []client.ListOption{
		client.InNamespace(b.namespace),
		client.MatchingLabels(b.labels),
	}

	continueToken := ""
	pageSize := int64(pkgconstants.DefaultPageLimit)
	var names []string
	for {
		pageResults, nextToken, err := b.listPageWithToken(baseListOpts, pageSize, continueToken)
		if err != nil {
			return nil, err
		}
		for i := range pageResults {
			names = append(names, pageResults[i].GetName())
		}
		if nextToken == "" {
			break
		}
		continueToken = nextToken
	}
	sort.Strings(names)
	return names, nil
}

// Exists returns true if a resource with the given name exists.
func (b *BaseResource[T, L]) Exists(name string) (bool, error) {
	// Prefer server-side label lookup (logical name).
	labelResults, err := b.listByLogicalName(name, 1)
	if err != nil {
		return false, err
	}
	if len(labelResults) > 0 {
		return true, nil
	}

	// Fallback: page through results and stop at first match.
	results, err := b.searchByNamePaged(name, 1)
	if err != nil {
		return false, err
	}
	return len(results) > 0, nil
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
	// Determine limit for server-side pagination
	// If we are filtering by name client-side, we must fetch all items first to ensure we find the match.
	// Otherwise, we can optimize by limiting the fetch.
	fetchLimit := 0
	if filter != nil && filter.Limit > 0 && (filter.Name == "") {
		fetchLimit = filter.Limit
	}

	items, err := b.List(fetchLimit)
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

	// If filtering by name, we try to avoid fetching all items by doing a targeted
	// server-side lookup first (by the logical name label). If that returns no
	// results we fall back to a paged scan that *stops early* once matching
	// items are found. This prevents accidentally loading large datasets into
	// memory when the cluster contains many resources.
	if filter != nil && filter.Name != "" {
		// Try server-side label lookup first
		labelResults, err := b.listByLogicalName(filter.Name, filter.Limit)
		if err != nil {
			return err
		}
		if len(labelResults) > 0 {
			items = labelResults
			return b.PrintItems(items, format)
		}

		// Fallback: page through results and collect matches until we reach
		// the requested limit (or end). This avoids fetching the entire set.
		pagedResults, err := b.searchByNamePaged(filter.Name, filter.Limit)
		if err != nil {
			return err
		}
		items = pagedResults
		return b.PrintItems(items, format)
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

// FindByName returns resources matching either the logical name label
// (`constants.LabelName`) or the Kubernetes object name. This method avoids
// loading the entire namespace by doing an indexed label lookup first and then
// falling back to a paged scan that can stop early.
func (b *BaseResource[T, L]) FindByName(name string, limit int) ([]ResourceWrapper[T], error) {
	if name == "" {
		return nil, fmt.Errorf("name must not be empty")
	}

	labelResults, err := b.listByLogicalName(name, limit)
	if err != nil {
		return nil, err
	}
	if len(labelResults) > 0 {
		return labelResults, nil
	}

	return b.searchByNamePaged(name, limit)
}

// listByLogicalName attempts a server-side lookup by the logical name label to
// efficiently find resources which set `constants.LabelName`. It pages if
// necessary and honors the caller-provided limit.
func (b *BaseResource[T, L]) listByLogicalName(name string, limit int) ([]ResourceWrapper[T], error) {
	baseListOpts := []client.ListOption{
		client.InNamespace(b.namespace),
		client.MatchingLabels(b.labels),
		client.MatchingLabels(map[string]string{constants.LabelName: name}),
	}

	var results []ResourceWrapper[T]
	continueToken := ""
	pageSize := int64(pkgconstants.DefaultPageLimit)
	if limit > 0 {
		pageSize = int64(limit)
	}
	if pageSize > int64(pkgconstants.MaxPageLimit) {
		pageSize = int64(pkgconstants.MaxPageLimit)
	}

	for {
		if limit > 0 {
			remaining := limit - len(results)
			if remaining <= 0 {
				break
			}
			if int64(remaining) < pageSize {
				pageSize = int64(remaining)
			}
		}

		pageResults, nextToken, err := b.listPageWithToken(baseListOpts, pageSize, continueToken)
		if err != nil {
			return nil, err
		}

		results = append(results, pageResults...)

		if limit > 0 && len(results) >= limit {
			if len(results) > limit {
				results = results[:limit]
			}
			break
		}

		if nextToken == "" {
			break
		}
		continueToken = nextToken
	}
	return results, nil
}

// searchByNamePaged pages through resources, collecting items whose either
// Kubernetes name or logical name matches `name`. It stops once the requested
// limit of matches is found (or end of pages).
func (b *BaseResource[T, L]) searchByNamePaged(name string, limit int) ([]ResourceWrapper[T], error) {
	baseListOpts := []client.ListOption{
		client.InNamespace(b.namespace),
		client.MatchingLabels(b.labels),
	}

	var results []ResourceWrapper[T]
	continueToken := ""
	pageSize := int64(pkgconstants.DefaultPageLimit)

	for {
		pageResults, nextToken, err := b.listPageWithToken(baseListOpts, pageSize, continueToken)
		if err != nil {
			return nil, err
		}

		for _, r := range pageResults {
			if r.KubernetesName == name || r.LogicalName == name {
				results = append(results, r)
				if limit > 0 && len(results) >= limit {
					if len(results) > limit {
						results = results[:limit]
					}
					return results, nil
				}
			}
		}

		if nextToken == "" {
			break
		}
		continueToken = nextToken
	}

	return results, nil
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

// newPtrTypeOf returns a fresh pointer for lists (e.g. &openchoreov1alpha1.BuildList{})
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
