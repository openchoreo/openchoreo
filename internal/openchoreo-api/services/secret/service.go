// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package secret

import (
	"context"
	"fmt"
	"log/slog"
	"maps"
	"sort"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	kubernetesClient "github.com/openchoreo/openchoreo/internal/clients/kubernetes"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
)

const (
	// kvNamespacePrefix is the prefix of the namespace in the target plane
	// where the K8s Secret and PushSecret are provisioned.
	kvNamespacePrefix = "openchoreo-kv-"

	// fieldOwner is the field manager name used for Server-Side Apply.
	fieldOwner = "openchoreo-api"

	// managedByLabel marks resources created by the openchoreo-api secret
	// service. UIs filter SecretReferences on this label to distinguish ones
	// backed by a PushSecret from hand-written SecretReferences.
	managedByLabel         = "openchoreo.dev/managed-by"
	managedByOpenchoreoAPI = "openchoreo-api"

	planeKindWorkflowPlane        = "WorkflowPlane"
	planeKindClusterWorkflowPlane = "ClusterWorkflowPlane"
	planeKindDataPlane            = "DataPlane"
	planeKindClusterDataPlane     = "ClusterDataPlane"

	pushSecretAPIVersion = "external-secrets.io/v1alpha1"
	pushSecretKind       = "PushSecret"

	// syncTriggerAnnotation is stamped on Create so ESO reconciles the
	// PushSecret immediately and pushes the K8s Secret values to the
	// external store, instead of waiting for the next refreshInterval.
	syncTriggerAnnotation = "openchoreo.dev/sync-trigger"
)

// kvNamespace returns the namespace in the target plane where the K8s Secret
// and PushSecret are provisioned.
func kvNamespace(ownerNamespace string) string {
	return kvNamespacePrefix + ownerNamespace
}

// remoteKeyFor maps a (namespace, secretType, name) tuple to the key path used
// in the external secret store.
func remoteKeyFor(namespace string, secretType corev1.SecretType, name string) string {
	segment := remoteKeySegment(secretType)
	return fmt.Sprintf("secret/%s/%s/%s", namespace, segment, name)
}

func remoteKeySegment(secretType corev1.SecretType) string {
	switch secretType {
	case corev1.SecretTypeBasicAuth:
		return "basic-auth"
	case corev1.SecretTypeSSHAuth:
		return "ssh-auth"
	case corev1.SecretTypeDockerConfigJson:
		return "registry"
	case corev1.SecretTypeTLS:
		return "tls"
	default:
		return "generic"
	}
}

// secretService handles secret business logic without authorization checks.
type secretService struct {
	k8sClient           client.Client
	planeClientProvider kubernetesClient.PlaneClientProvider
	logger              *slog.Logger
}

var _ Service = (*secretService)(nil)

// NewService creates a new secret service without authorization.
func NewService(k8sClient client.Client, planeClientProvider kubernetesClient.PlaneClientProvider, logger *slog.Logger) Service {
	return &secretService{
		k8sClient:           k8sClient,
		planeClientProvider: planeClientProvider,
		logger:              logger,
	}
}

// CreateSecret provisions a new secret across the control plane and the target plane.
func (s *secretService) CreateSecret(ctx context.Context, namespaceName string, req *CreateSecretParams) (*corev1.Secret, error) {
	s.logger.Debug("Creating secret",
		"namespace", namespaceName, "secret", req.SecretName, "type", req.SecretType,
		"plane", req.TargetPlane.Kind+"/"+req.TargetPlane.Name)

	if err := validateSecretName(req.SecretName); err != nil {
		return nil, err
	}
	if err := validateSecretData(req.SecretType, req.Data); err != nil {
		return nil, err
	}
	if err := validatePlaneKind(req.TargetPlane.Kind); err != nil {
		return nil, err
	}
	if req.TargetPlane.Name == "" {
		return nil, &services.ValidationError{Msg: "targetPlane.name is required"}
	}
	if err := validateLabels(req.Labels); err != nil {
		return nil, err
	}

	// Conflict check on the SecretReference in the control plane.
	existing := &openchoreov1alpha1.SecretReference{}
	key := client.ObjectKey{Name: req.SecretName, Namespace: namespaceName}
	if err := s.k8sClient.Get(ctx, key, existing); err == nil {
		return nil, ErrSecretAlreadyExists
	} else if client.IgnoreNotFound(err) != nil {
		return nil, fmt.Errorf("failed to check existing secret reference: %w", err)
	}

	planeInfo, err := s.resolvePlane(ctx, namespaceName, req.TargetPlane.Kind, req.TargetPlane.Name)
	if err != nil {
		return nil, err
	}

	targetNs := kvNamespace(namespaceName)
	if err := s.ensureNamespaceExists(ctx, planeInfo.k8sClient, targetNs); err != nil {
		return nil, err
	}

	k8sSecret := buildK8sSecret(req.SecretName, targetNs, req.SecretType, req.Data)
	if err := planeInfo.k8sClient.Patch(ctx, k8sSecret, client.Apply, client.ForceOwnership, client.FieldOwner(fieldOwner)); err != nil {
		return nil, fmt.Errorf("failed to apply k8s secret in target plane: %w", err)
	}

	pushSecret := buildPushSecret(req.SecretName, namespaceName, targetNs, planeInfo.secretStoreName, req.SecretType, sortedKeys(req.Data))
	if err := planeInfo.k8sClient.Patch(ctx, pushSecret, client.Apply, client.ForceOwnership, client.FieldOwner(fieldOwner)); err != nil {
		return nil, fmt.Errorf("failed to apply push secret in target plane: %w", err)
	}

	secretRef := buildSecretReference(namespaceName, req.SecretName, req.SecretType, req.TargetPlane, sortedKeys(req.Data), req.Labels)
	if err := s.k8sClient.Create(ctx, secretRef); err != nil {
		return nil, fmt.Errorf("failed to create secret reference: %w", err)
	}

	s.logger.Info("Created secret", "namespace", namespaceName, "secret", req.SecretName)
	return buildResponseSecret(secretRef, req.SecretType, stringMapToBytes(req.Data)), nil
}

// UpdateSecret replaces a secret's data with the supplied final state.
// Keys present in the existing secret but absent from req.Data are pruned
// from the K8s Secret, PushSecret, and SecretReference.
func (s *secretService) UpdateSecret(ctx context.Context, namespaceName, secretName string, req *UpdateSecretParams) (*corev1.Secret, error) {
	s.logger.Debug("Updating secret", "namespace", namespaceName, "secret", secretName)

	secretRef := &openchoreov1alpha1.SecretReference{}
	key := client.ObjectKey{Name: secretName, Namespace: namespaceName}
	if err := s.k8sClient.Get(ctx, key, secretRef); err != nil {
		if client.IgnoreNotFound(err) == nil {
			return nil, ErrSecretNotFound
		}
		return nil, fmt.Errorf("failed to get secret reference: %w", err)
	}
	if !isManagedSecret(secretRef) {
		return nil, ErrSecretNotFound
	}

	secretType := secretRef.Spec.Template.Type
	if err := validateSecretData(secretType, req.Data); err != nil {
		return nil, err
	}
	if err := validateLabels(req.Labels); err != nil {
		return nil, err
	}

	planeInfo, err := s.resolvePlane(ctx, namespaceName, secretRef.Spec.TargetPlane.Kind, secretRef.Spec.TargetPlane.Name)
	if err != nil {
		return nil, err
	}

	targetNs := kvNamespace(namespaceName)
	newKeys := sortedKeys(req.Data)

	k8sSecret := buildK8sSecret(secretName, targetNs, secretType, req.Data)
	if err := planeInfo.k8sClient.Patch(ctx, k8sSecret, client.Apply, client.ForceOwnership, client.FieldOwner(fieldOwner)); err != nil {
		return nil, fmt.Errorf("failed to apply k8s secret in target plane: %w", err)
	}

	pushSecret := buildPushSecret(secretName, namespaceName, targetNs, planeInfo.secretStoreName, secretType, newKeys)
	if err := planeInfo.k8sClient.Patch(ctx, pushSecret, client.Apply, client.ForceOwnership, client.FieldOwner(fieldOwner)); err != nil {
		return nil, fmt.Errorf("failed to apply push secret in target plane: %w", err)
	}

	existingKeys := dataSourceKeys(secretRef.Spec.Data)
	newLabels := mergeManagedLabels(req.Labels)

	keysChanged := !sameStringSlice(existingKeys, newKeys)
	labelsChanged := !maps.Equal(secretRef.Labels, newLabels)
	if keysChanged || labelsChanged {
		secretRef.Spec.Data = buildSecretDataSources(namespaceName, secretName, secretType, newKeys)
		secretRef.Labels = newLabels
		if err := s.k8sClient.Update(ctx, secretRef); err != nil {
			if apierrors.IsInvalid(err) {
				return nil, &services.ValidationError{Msg: services.ExtractValidationMessage(err)}
			}
			return nil, fmt.Errorf("failed to update secret reference: %w", err)
		}
	}

	s.logger.Info("Updated secret", "namespace", namespaceName, "secret", secretName)
	return buildResponseSecret(secretRef, secretType, stringMapToBytes(req.Data)), nil
}

// GetSecret returns a secret managed by this API, including its data from
// the target plane. Returns ErrSecretNotFound if either the SecretReference
// or the target-plane K8s Secret is missing.
func (s *secretService) GetSecret(ctx context.Context, namespaceName, secretName string) (*corev1.Secret, error) {
	s.logger.Debug("Getting secret", "namespace", namespaceName, "secret", secretName)

	secretRef := &openchoreov1alpha1.SecretReference{}
	key := client.ObjectKey{Name: secretName, Namespace: namespaceName}
	if err := s.k8sClient.Get(ctx, key, secretRef); err != nil {
		if client.IgnoreNotFound(err) == nil {
			return nil, ErrSecretNotFound
		}
		return nil, fmt.Errorf("failed to get secret reference: %w", err)
	}
	if !isManagedSecret(secretRef) {
		return nil, ErrSecretNotFound
	}

	planeInfo, err := s.resolvePlane(ctx, namespaceName, secretRef.Spec.TargetPlane.Kind, secretRef.Spec.TargetPlane.Name)
	if err != nil {
		return nil, err
	}

	planeSecret, err := s.fetchSecret(ctx, planeInfo.k8sClient, kvNamespace(namespaceName), secretName)
	if err != nil {
		return nil, err
	}

	return buildResponseSecret(secretRef, secretRef.Spec.Template.Type, planeSecret.Data), nil
}

// ListSecrets returns a single page of secrets managed by this API.
//
// The returned page mirrors one K8s LIST of SecretReferences: NextCursor is
// the K8s continuation token and a page may return fewer items than the
// requested limit (when some SecretReferences have no synced target-plane
// Secret yet, or when a plane LIST fails). Callers that need full pages
// regardless of drops should wrap this with services.FilteredList — that's
// what the authz wrapper does.
//
// To avoid one GET per item, the page is grouped by (plane kind, name) and
// each distinct plane is hit with a single LIST against openchoreo-kv-{ns}.
// With K planes referenced in a page of N items, that's K plane round-trips
// instead of N.
func (s *secretService) ListSecrets(ctx context.Context, namespaceName string, opts services.ListOptions) (*services.ListResult[corev1.Secret], error) {
	s.logger.Debug("Listing secrets", "namespace", namespaceName, "limit", opts.Limit, "cursor", opts.Cursor)

	commonOpts, err := services.BuildListOptions(opts)
	if err != nil {
		return nil, err
	}
	listOpts := make([]client.ListOption, 0, 2+len(commonOpts))
	listOpts = append(listOpts,
		client.InNamespace(namespaceName),
		client.MatchingLabels{managedByLabel: managedByOpenchoreoAPI},
	)
	listOpts = append(listOpts, commonOpts...)

	var srList openchoreov1alpha1.SecretReferenceList
	if err := s.k8sClient.List(ctx, &srList, listOpts...); err != nil {
		return nil, fmt.Errorf("failed to list secrets: %w", err)
	}

	type planeKey struct {
		kind, name string
	}
	byPlane := make(map[planeKey][]*openchoreov1alpha1.SecretReference, 2)
	for i := range srList.Items {
		sr := &srList.Items[i]
		if !isManagedSecret(sr) {
			continue
		}
		key := planeKey{sr.Spec.TargetPlane.Kind, sr.Spec.TargetPlane.Name}
		byPlane[key] = append(byPlane[key], sr)
	}

	planeSecrets := make(map[planeKey]map[string]*corev1.Secret, len(byPlane))
	for key, refs := range byPlane {
		wanted := make(map[string]struct{}, len(refs))
		for _, sr := range refs {
			wanted[sr.Name] = struct{}{}
		}
		index, err := s.listPlaneSecrets(ctx, namespaceName, key.kind, key.name, wanted)
		if err != nil {
			s.logger.Warn("Skipping secrets from plane: list failed",
				"namespace", namespaceName, "planeKind", key.kind, "planeName", key.name, "error", err)
			continue
		}
		planeSecrets[key] = index
	}

	items := make([]corev1.Secret, 0, len(srList.Items))
	for i := range srList.Items {
		sr := &srList.Items[i]
		if !isManagedSecret(sr) {
			continue
		}
		key := planeKey{sr.Spec.TargetPlane.Kind, sr.Spec.TargetPlane.Name}
		index, ok := planeSecrets[key]
		if !ok {
			continue // plane resolution or list failed; already logged
		}
		planeSecret, ok := index[sr.Name]
		if !ok {
			s.logger.Debug("Skipping secret: target-plane Secret missing",
				"namespace", namespaceName, "secret", sr.Name)
			continue
		}
		items = append(items, *buildResponseSecret(sr, sr.Spec.Template.Type, planeSecret.Data))
	}

	result := &services.ListResult[corev1.Secret]{
		Items:      items,
		NextCursor: srList.Continue,
	}
	if srList.RemainingItemCount != nil {
		remaining := *srList.RemainingItemCount
		result.RemainingCount = &remaining
	}
	return result, nil
}

// planeSecretsPageSize bounds the per-page response size when listing Secrets
// in a target plane. Large enough that small namespaces complete in one
// round-trip, small enough that pathological namespaces don't blow memory.
const planeSecretsPageSize = 100

// listPlaneSecrets resolves the target plane and returns the Secrets in
// openchoreo-kv-{ns} whose names appear in `wanted`, indexed by name. It
// paginates the underlying K8s LIST and short-circuits once every wanted
// name is found.
func (s *secretService) listPlaneSecrets(ctx context.Context, ownerNamespace, planeKind, planeName string, wanted map[string]struct{}) (map[string]*corev1.Secret, error) {
	planeInfo, err := s.resolvePlane(ctx, ownerNamespace, planeKind, planeName)
	if err != nil {
		return nil, err
	}

	index := make(map[string]*corev1.Secret, len(wanted))
	cursor := ""
	for {
		listOpts := []client.ListOption{
			client.InNamespace(kvNamespace(ownerNamespace)),
			client.Limit(planeSecretsPageSize),
		}
		if cursor != "" {
			listOpts = append(listOpts, client.Continue(cursor))
		}

		var list corev1.SecretList
		if err := planeInfo.k8sClient.List(ctx, &list, listOpts...); err != nil {
			return nil, fmt.Errorf("failed to list secrets in target plane: %w", err)
		}

		for i := range list.Items {
			name := list.Items[i].Name
			if _, ok := wanted[name]; !ok {
				continue
			}
			index[name] = &list.Items[i]
			if len(index) == len(wanted) {
				return index, nil
			}
		}

		if list.Continue == "" {
			return index, nil
		}
		cursor = list.Continue
	}
}

// fetchSecret returns the K8s Secret object from the target plane.
// Missing Secret maps to ErrSecretNotFound.
func (s *secretService) fetchSecret(ctx context.Context, k8sClient client.Client, namespace, name string) (*corev1.Secret, error) {
	secret := &corev1.Secret{}
	if err := k8sClient.Get(ctx, client.ObjectKey{Name: name, Namespace: namespace}, secret); err != nil {
		if client.IgnoreNotFound(err) == nil {
			return nil, ErrSecretNotFound
		}
		return nil, fmt.Errorf("failed to get k8s secret in target plane: %w", err)
	}
	return secret, nil
}

// DeleteSecret removes a secret from the control plane and the target plane.
func (s *secretService) DeleteSecret(ctx context.Context, namespaceName, secretName string) error {
	s.logger.Debug("Deleting secret", "namespace", namespaceName, "secret", secretName)

	secretRef := &openchoreov1alpha1.SecretReference{}
	key := client.ObjectKey{Name: secretName, Namespace: namespaceName}
	if err := s.k8sClient.Get(ctx, key, secretRef); err != nil {
		if client.IgnoreNotFound(err) == nil {
			return ErrSecretNotFound
		}
		return fmt.Errorf("failed to get secret reference: %w", err)
	}
	if !isManagedSecret(secretRef) {
		return ErrSecretNotFound
	}

	planeInfo, err := s.resolvePlane(ctx, namespaceName, secretRef.Spec.TargetPlane.Kind, secretRef.Spec.TargetPlane.Name)
	if err != nil {
		return err
	}

	targetNs := kvNamespace(namespaceName)

	pushSecret := &unstructured.Unstructured{}
	pushSecret.SetAPIVersion(pushSecretAPIVersion)
	pushSecret.SetKind(pushSecretKind)
	pushSecret.SetName(secretName)
	pushSecret.SetNamespace(targetNs)
	if err := planeInfo.k8sClient.Delete(ctx, pushSecret); err != nil && client.IgnoreNotFound(err) != nil {
		return fmt.Errorf("failed to delete push secret: %w", err)
	}

	k8sSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: secretName, Namespace: targetNs},
	}
	if err := planeInfo.k8sClient.Delete(ctx, k8sSecret); err != nil && client.IgnoreNotFound(err) != nil {
		return fmt.Errorf("failed to delete k8s secret: %w", err)
	}

	if err := s.k8sClient.Delete(ctx, secretRef); err != nil && client.IgnoreNotFound(err) != nil {
		return fmt.Errorf("failed to delete secret reference: %w", err)
	}

	s.logger.Info("Deleted secret", "namespace", namespaceName, "secret", secretName)
	return nil
}

// planeInfo holds resolved details for a target plane.
type planeInfo struct {
	k8sClient       client.Client
	secretStoreName string
}

// resolvePlane fetches the plane CR by kind and name, validates it, and returns a client.
func (s *secretService) resolvePlane(ctx context.Context, namespaceName, kind, name string) (*planeInfo, error) {
	switch kind {
	case planeKindWorkflowPlane:
		wp := &openchoreov1alpha1.WorkflowPlane{}
		if err := s.k8sClient.Get(ctx, client.ObjectKey{Name: name, Namespace: namespaceName}, wp); err != nil {
			return nil, mapPlaneGetError(err)
		}
		if wp.Spec.SecretStoreRef == nil || wp.Spec.SecretStoreRef.Name == "" {
			return nil, ErrSecretStoreNotConfigured
		}
		c, err := s.planeClientProvider.WorkflowPlaneClient(wp)
		if err != nil {
			return nil, fmt.Errorf("failed to get workflow plane client: %w", err)
		}
		return &planeInfo{k8sClient: c, secretStoreName: wp.Spec.SecretStoreRef.Name}, nil

	case planeKindClusterWorkflowPlane:
		cwp := &openchoreov1alpha1.ClusterWorkflowPlane{}
		if err := s.k8sClient.Get(ctx, client.ObjectKey{Name: name}, cwp); err != nil {
			return nil, mapPlaneGetError(err)
		}
		if cwp.Spec.SecretStoreRef == nil || cwp.Spec.SecretStoreRef.Name == "" {
			return nil, ErrSecretStoreNotConfigured
		}
		c, err := s.planeClientProvider.ClusterWorkflowPlaneClient(cwp)
		if err != nil {
			return nil, fmt.Errorf("failed to get cluster workflow plane client: %w", err)
		}
		return &planeInfo{k8sClient: c, secretStoreName: cwp.Spec.SecretStoreRef.Name}, nil

	case planeKindDataPlane:
		dp := &openchoreov1alpha1.DataPlane{}
		if err := s.k8sClient.Get(ctx, client.ObjectKey{Name: name, Namespace: namespaceName}, dp); err != nil {
			return nil, mapPlaneGetError(err)
		}
		if dp.Spec.SecretStoreRef == nil || dp.Spec.SecretStoreRef.Name == "" {
			return nil, ErrSecretStoreNotConfigured
		}
		c, err := s.planeClientProvider.DataPlaneClient(dp)
		if err != nil {
			return nil, fmt.Errorf("failed to get data plane client: %w", err)
		}
		return &planeInfo{k8sClient: c, secretStoreName: dp.Spec.SecretStoreRef.Name}, nil

	case planeKindClusterDataPlane:
		cdp := &openchoreov1alpha1.ClusterDataPlane{}
		if err := s.k8sClient.Get(ctx, client.ObjectKey{Name: name}, cdp); err != nil {
			return nil, mapPlaneGetError(err)
		}
		if cdp.Spec.SecretStoreRef == nil || cdp.Spec.SecretStoreRef.Name == "" {
			return nil, ErrSecretStoreNotConfigured
		}
		c, err := s.planeClientProvider.ClusterDataPlaneClient(cdp)
		if err != nil {
			return nil, fmt.Errorf("failed to get cluster data plane client: %w", err)
		}
		return &planeInfo{k8sClient: c, secretStoreName: cdp.Spec.SecretStoreRef.Name}, nil

	default:
		return nil, &services.ValidationError{Msg: fmt.Sprintf("unsupported targetPlane.kind: %s", kind)}
	}
}

func mapPlaneGetError(err error) error {
	if client.IgnoreNotFound(err) == nil {
		return ErrPlaneNotFound
	}
	return fmt.Errorf("failed to get target plane: %w", err)
}

// ensureNamespaceExists creates the namespace in the target plane if absent.
func (s *secretService) ensureNamespaceExists(ctx context.Context, k8sClient client.Client, namespaceName string) error {
	ns := &corev1.Namespace{}
	if err := k8sClient.Get(ctx, client.ObjectKey{Name: namespaceName}, ns); err == nil {
		return nil
	} else if client.IgnoreNotFound(err) != nil {
		return fmt.Errorf("failed to check namespace existence: %w", err)
	}

	ns = &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespaceName}}
	if err := k8sClient.Create(ctx, ns); err != nil {
		if apierrors.IsAlreadyExists(err) {
			return nil
		}
		return fmt.Errorf("failed to create namespace %s: %w", namespaceName, err)
	}
	return nil
}

// --- builders ---

func buildK8sSecret(name, targetNamespace string, secretType corev1.SecretType, data map[string]string) *corev1.Secret {
	// Use Data (not StringData) so SSA's field manager owns each key in the
	// persisted map. StringData is a write-only convenience that the apiserver
	// expands into Data; our field manager would never own anything in Data,
	// so SSA could not prune keys dropped on a later Update.
	dataBytes := make(map[string][]byte, len(data))
	for k, v := range data {
		dataBytes[k] = []byte(v)
	}
	return &corev1.Secret{
		TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "Secret"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: targetNamespace,
		},
		Type: secretType,
		Data: dataBytes,
	}
}

func buildPushSecret(name, ownerNamespace, targetNamespace, secretStoreName string, secretType corev1.SecretType, keys []string) *unstructured.Unstructured {
	remoteKey := remoteKeyFor(ownerNamespace, secretType, name)

	dataMatches := make([]map[string]any, 0, len(keys))
	for _, k := range keys {
		dataMatches = append(dataMatches, map[string]any{
			"match": map[string]any{
				"secretKey": k,
				"remoteRef": map[string]any{
					"remoteKey": remoteKey,
					"property":  k,
				},
			},
		})
	}

	ps := &unstructured.Unstructured{}
	ps.SetAPIVersion(pushSecretAPIVersion)
	ps.SetKind(pushSecretKind)
	ps.SetName(name)
	ps.SetNamespace(targetNamespace)
	ps.SetAnnotations(map[string]string{
		syncTriggerAnnotation: time.Now().UTC().Format(time.RFC3339Nano),
	})
	ps.Object["spec"] = map[string]any{
		"updatePolicy":   "Replace",
		"deletionPolicy": "Delete",
		"secretStoreRefs": []map[string]any{
			{"kind": "ClusterSecretStore", "name": secretStoreName},
		},
		"selector": map[string]any{
			"secret": map[string]any{"name": name},
		},
		"data": dataMatches,
	}
	return ps
}

func buildSecretReference(ownerNamespace, name string, secretType corev1.SecretType, target openchoreov1alpha1.TargetPlaneRef, keys []string, userLabels map[string]string) *openchoreov1alpha1.SecretReference {
	return &openchoreov1alpha1.SecretReference{
		TypeMeta: metav1.TypeMeta{
			APIVersion: openchoreov1alpha1.GroupVersion.String(),
			Kind:       "SecretReference",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ownerNamespace,
			Labels:    mergeManagedLabels(userLabels),
		},
		Spec: openchoreov1alpha1.SecretReferenceSpec{
			TargetPlane: &openchoreov1alpha1.TargetPlaneRef{Kind: target.Kind, Name: target.Name},
			Template:    openchoreov1alpha1.SecretTemplate{Type: secretType},
			Data:        buildSecretDataSources(ownerNamespace, name, secretType, keys),
		},
	}
}

// mergeManagedLabels returns a label map that contains the user-supplied
// labels with the reserved managed-by label always set on top, so the
// managed-by marker cannot be dropped or overridden by the caller.
func mergeManagedLabels(userLabels map[string]string) map[string]string {
	labels := make(map[string]string, len(userLabels)+1)
	for k, v := range userLabels {
		labels[k] = v
	}
	labels[managedByLabel] = managedByOpenchoreoAPI
	return labels
}

func buildSecretDataSources(ownerNamespace, name string, secretType corev1.SecretType, keys []string) []openchoreov1alpha1.SecretDataSource {
	remoteKey := remoteKeyFor(ownerNamespace, secretType, name)
	out := make([]openchoreov1alpha1.SecretDataSource, 0, len(keys))
	for _, k := range keys {
		out = append(out, openchoreov1alpha1.SecretDataSource{
			SecretKey: k,
			RemoteRef: openchoreov1alpha1.RemoteReference{
				Key:      remoteKey,
				Property: k,
			},
		})
	}
	return out
}

// --- helpers ---

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// isManagedSecret reports whether a SecretReference was created by this API.
// A managed secret has the managed-by label and a populated targetPlane.
func isManagedSecret(sr *openchoreov1alpha1.SecretReference) bool {
	if sr == nil || sr.Spec.TargetPlane == nil {
		return false
	}
	return sr.Labels[managedByLabel] == managedByOpenchoreoAPI
}

// dataSourceKeys returns the sorted list of secretKey values declared on a
// SecretReference's data sources.
func dataSourceKeys(sources []openchoreov1alpha1.SecretDataSource) []string {
	keys := make([]string, 0, len(sources))
	for _, src := range sources {
		keys = append(keys, src.SecretKey)
	}
	sort.Strings(keys)
	return keys
}

// sameStringSlice reports whether two sorted string slices contain the same
// elements in the same order.
func sameStringSlice(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// stringMapToBytes converts a string→string data map (as supplied by callers)
// into the byte-valued map that matches the Kubernetes Secret shape.
func stringMapToBytes(m map[string]string) map[string][]byte {
	out := make(map[string][]byte, len(m))
	for k, v := range m {
		out[k] = []byte(v)
	}
	return out
}

// buildResponseSecret synthesizes a corev1.Secret response object using the
// SecretReference's identity metadata and the supplied data. Callers must
// treat data as sensitive.
func buildResponseSecret(sr *openchoreov1alpha1.SecretReference, secretType corev1.SecretType, data map[string][]byte) *corev1.Secret {
	return &corev1.Secret{
		TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "Secret"},
		ObjectMeta: metav1.ObjectMeta{
			Name:              sr.Name,
			Namespace:         sr.Namespace,
			Labels:            sr.Labels,
			Annotations:       sr.Annotations,
			CreationTimestamp: sr.CreationTimestamp,
			ResourceVersion:   sr.ResourceVersion,
			UID:               sr.UID,
		},
		Type: secretType,
		Data: data,
	}
}
