// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package observabilityalertsnotificationchannel

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openchoreodevv1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/controller"
	esv1 "github.com/openchoreo/openchoreo/internal/dataplane/kubernetes/types/externalsecrets/v1"
	"github.com/openchoreo/openchoreo/internal/labels"
)

const (
	// fieldOwner is the field manager used for server-side apply on the observability plane.
	fieldOwner = "observabilityalertsnotificationchannel-controller"

	// managedByLabelValue marks the ConfigMap and the copied Secret applied by this controller.
	// The Secret that the ExternalSecret produces does not carry it, which tells the two apart.
	managedByLabelValue = "observabilityalertsnotificationchannel-controller"

	// Keys of the secret values in the Secret the observer reads.
	smtpAuthUsernameKey = "smtp.auth.username"
	smtpAuthPasswordKey = "smtp.auth.password"
	webhookHeaderKeyFmt = "webhook.header.%s"
)

// secretSource is where a channel's referenced secret values come from.
type secretSource string

const (
	// secretSourceNone means the channel references no secret values.
	secretSourceNone secretSource = ""
	// secretSourceSecretReference means the values are synced from the observability plane's
	// secret store by an ExternalSecret built from SecretReferences.
	secretSourceSecretReference secretSource = "SecretReference"
	// secretSourceKubernetesSecret means the values are copied from Kubernetes Secrets on the
	// control plane. This is deprecated.
	secretSourceKubernetesSecret secretSource = "Secret"
)

// channelSecretRef is a secret value a channel references, with the key it gets in the
// Secret the observer reads.
type channelSecretRef struct {
	secretKey string
	ref       *openchoreodevv1alpha1.SecretKeyRef
}

// resolvedSecretRef is a channelSecretRef resolved to an entry of a SecretReference.
type resolvedSecretRef struct {
	channelSecretRef
	secretReference *openchoreodevv1alpha1.SecretReference
	remoteRef       openchoreodevv1alpha1.RemoteReference
}

// resolvedSecrets describes where every secret value a channel references comes from.
type resolvedSecrets struct {
	source secretSource
	// refs holds the resolved references when source is secretSourceSecretReference.
	refs []resolvedSecretRef
	// secretNames holds the referenced Kubernetes Secrets when source is secretSourceKubernetesSecret.
	secretNames []string
}

// invalidSecretsError is a problem with the channel's secret references that only a change to
// the channel or to what it references can fix, so it is reported on the channel instead of retried.
type invalidSecretsError struct {
	reason  controller.ConditionReason
	message string
}

func (e *invalidSecretsError) Error() string {
	return e.message
}

func newInvalidSecretsError(reason controller.ConditionReason, format string, args ...any) error {
	return &invalidSecretsError{reason: reason, message: fmt.Sprintf(format, args...)}
}

// channelSecretRefs returns the secret values the channel references, sorted by the key they
// get in the Secret the observer reads.
func channelSecretRefs(channel *openchoreodevv1alpha1.ObservabilityAlertsNotificationChannel) []channelSecretRef {
	var refs []channelSecretRef
	add := func(secretKey string, from *openchoreodevv1alpha1.SecretValueFrom) {
		if from != nil && from.SecretKeyRef != nil {
			refs = append(refs, channelSecretRef{secretKey: secretKey, ref: from.SecretKeyRef})
		}
	}

	switch channel.Spec.Type {
	case openchoreodevv1alpha1.NotificationChannelTypeEmail:
		if channel.Spec.EmailConfig != nil && channel.Spec.EmailConfig.SMTP.Auth != nil {
			add(smtpAuthUsernameKey, channel.Spec.EmailConfig.SMTP.Auth.Username)
			add(smtpAuthPasswordKey, channel.Spec.EmailConfig.SMTP.Auth.Password)
		}
	case openchoreodevv1alpha1.NotificationChannelTypeWebhook:
		if channel.Spec.WebhookConfig != nil {
			for name, header := range channel.Spec.WebhookConfig.Headers {
				add(fmt.Sprintf(webhookHeaderKeyFmt, name), header.ValueFrom)
			}
		}
	}

	sort.Slice(refs, func(i, j int) bool { return refs[i].secretKey < refs[j].secretKey })
	return refs
}

// resolveSecrets works out where each referenced secret value comes from. A name resolves to the
// SecretReference with that name in the channel's namespace, or, if there is none, to the
// Kubernetes Secret with that name. All references of a channel must resolve the same way, since
// both ways produce the one Secret the observer reads for the channel.
func (r *Reconciler) resolveSecrets(ctx context.Context, channel *openchoreodevv1alpha1.ObservabilityAlertsNotificationChannel) (*resolvedSecrets, error) {
	resolved := &resolvedSecrets{source: secretSourceNone}
	secretNames := map[string]struct{}{}

	for _, ref := range channelSecretRefs(channel) {
		secretRef := &openchoreodevv1alpha1.SecretReference{}
		err := r.Get(ctx, client.ObjectKey{Name: ref.ref.Name, Namespace: channel.Namespace}, secretRef)
		switch {
		case err == nil:
			remoteRef, ok := findRemoteRef(secretRef, ref.ref.Key)
			if !ok {
				return nil, newInvalidSecretsError(ReasonSecretKeyNotFound,
					"SecretReference %q has no data entry with secretKey %q", ref.ref.Name, ref.ref.Key)
			}
			resolved.refs = append(resolved.refs, resolvedSecretRef{
				channelSecretRef: ref,
				secretReference:  secretRef,
				remoteRef:        remoteRef,
			})
		case apierrors.IsNotFound(err):
			secret := &corev1.Secret{}
			if err := r.Get(ctx, client.ObjectKey{Name: ref.ref.Name, Namespace: channel.Namespace}, secret); err != nil {
				if apierrors.IsNotFound(err) {
					return nil, newInvalidSecretsError(ReasonSecretNotFound,
						"neither a SecretReference nor a Secret named %q exists in namespace %q", ref.ref.Name, channel.Namespace)
				}
				return nil, fmt.Errorf("failed to get secret %s/%s: %w", channel.Namespace, ref.ref.Name, err)
			}
			if _, ok := secret.Data[ref.ref.Key]; !ok {
				return nil, newInvalidSecretsError(ReasonSecretKeyNotFound,
					"Secret %q has no key %q", ref.ref.Name, ref.ref.Key)
			}
			secretNames[ref.ref.Name] = struct{}{}
		default:
			return nil, fmt.Errorf("failed to get secret reference %s/%s: %w", channel.Namespace, ref.ref.Name, err)
		}
	}

	for name := range secretNames {
		resolved.secretNames = append(resolved.secretNames, name)
	}
	sort.Strings(resolved.secretNames)

	switch {
	case len(resolved.refs) > 0 && len(resolved.secretNames) > 0:
		return nil, newInvalidSecretsError(ReasonMixedSecretSources,
			"secret references must either all name SecretReferences or all name Kubernetes Secrets, "+
				"but these resolve to Kubernetes Secrets while the others resolve to SecretReferences: %s",
			strings.Join(resolved.secretNames, ", "))
	case len(resolved.refs) > 0:
		resolved.source = secretSourceSecretReference
	case len(resolved.secretNames) > 0:
		resolved.source = secretSourceKubernetesSecret
	}
	return resolved, nil
}

// findRemoteRef returns the remote reference of the SecretReference entry with the given secretKey.
func findRemoteRef(secretRef *openchoreodevv1alpha1.SecretReference, key string) (openchoreodevv1alpha1.RemoteReference, bool) {
	for _, data := range secretRef.Spec.Data {
		if data.SecretKey == key {
			return data.RemoteRef, true
		}
	}
	return openchoreodevv1alpha1.RemoteReference{}, false
}

// validateSecretReferences checks that every SecretReference can be synced into the observability
// plane for the channel: the plane has a secret store, the value was pushed to that plane's store,
// and the remote key is the channel's namespace's own. Webhook channels send header values to a
// URL of the user's choosing, so a channel may only read the one key that the secret API writes for
// the SecretReference: "<prefix><sep><namespace><sep><type><sep><name>", built from the channel's
// namespace and the SecretReference's type and name. Both names are validated Kubernetes names, so
// this rules out keys of other namespaces or of the platform, and path segments such as "..".
func (r *Reconciler) validateSecretReferences(
	channel *openchoreodevv1alpha1.ObservabilityAlertsNotificationChannel,
	obsPlane *controller.ObservabilityPlaneResult,
	resolved *resolvedSecrets,
) error {
	if obsPlane.GetSecretStoreName() == "" {
		return newInvalidSecretsError(ReasonSecretStoreNotConfigured,
			"observability plane %q has no secretStoreRef, so SecretReferences cannot be synced to it", obsPlane.GetName())
	}

	planeKind := observabilityPlaneKind(obsPlane)
	for _, ref := range resolved.refs {
		if target := ref.secretReference.Spec.TargetPlane; target != nil &&
			(target.Kind != planeKind || target.Name != obsPlane.GetName()) {
			return newInvalidSecretsError(ReasonSecretStoreUnreachable,
				"SecretReference %q targets %s %q, not the channel's observability plane %s %q",
				ref.ref.Name, target.Kind, target.Name, planeKind, obsPlane.GetName())
		}
		want := r.RemoteKeys.secretKey(channel.Namespace, ref.secretReference.Spec.Template.Type, ref.secretReference.Name)
		if ref.remoteRef.Key != want {
			return newInvalidSecretsError(ReasonRemoteKeyNotAllowed,
				"SecretReference %q points at remote key %q, but a notification channel may only read %q, "+
					"the key that the secret API writes for this SecretReference",
				ref.ref.Name, ref.remoteRef.Key, want)
		}
		if other := r.RemoteKeys.sharedWith(channel.Namespace, want); other != "" {
			return newInvalidSecretsError(ReasonRemoteKeyNotAllowed,
				"SecretReference %q points at remote key %q, which could be a key of namespace %q as well as of namespace %q, "+
					"whether or not that namespace exists, so it belongs to neither",
				ref.ref.Name, ref.remoteRef.Key, channel.Namespace, other)
		}
	}
	return nil
}

func observabilityPlaneKind(obsPlane *controller.ObservabilityPlaneResult) string {
	if obsPlane.ClusterObservabilityPlane != nil {
		return "ClusterObservabilityPlane"
	}
	return "ObservabilityPlane"
}

// RemoteKeyFormat describes how the secret API lays out keys in the external secret store, as
// "<prefix><sep><namespace><sep><type><sep><name>". It must match the secret API's
// remote_key_prefix and remote_key_separator settings.
type RemoteKeyFormat struct {
	// Prefix is the "/"-separated first segment of every key. Empty means no prefix segment.
	Prefix string
	// Separator joins the key segments, including those of a nested prefix.
	Separator string
}

// remoteKeyTypeSegments are the segments the secret API puts after the namespace in every remote
// key, one per secret type. They must match remoteKeySegment in internal/openchoreo-api/services/secret.
var remoteKeyTypeSegments = []string{"generic", "basic-auth", "ssh-auth", "registry", "tls"}

// remoteKeyTypeSegment returns the segment the secret API puts after the namespace in the remote key
// of a secret of the given type. It must match remoteKeySegment in internal/openchoreo-api/services/secret.
func remoteKeyTypeSegment(secretType corev1.SecretType) string {
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

// keyPrefix returns the start that every remote key has, before the namespace.
func (f RemoteKeyFormat) keyPrefix() string {
	var segments []string
	for _, segment := range strings.Split(f.Prefix, "/") {
		if segment != "" {
			segments = append(segments, segment)
		}
	}
	if len(segments) == 0 {
		return ""
	}
	return strings.Join(segments, f.Separator) + f.Separator
}

// namespacePrefix returns the start that every remote key of the given namespace has.
func (f RemoteKeyFormat) namespacePrefix(namespace string) string {
	return f.keyPrefix() + namespace + f.Separator
}

// secretKey returns the remote key that the secret API writes for a secret of the namespace with
// the given type and name.
func (f RemoteKeyFormat) secretKey(namespace string, secretType corev1.SecretType, name string) string {
	return f.namespacePrefix(namespace) + remoteKeyTypeSegment(secretType) + f.Separator + name
}

// separatorInNamespaceNames reports whether the separator can occur in a namespace name. Then a
// namespace's key prefix also matches keys of namespaces whose names extend it: with "-", the prefix
// of namespace "team" matches the keys of namespace "team-a".
func (f RemoteKeyFormat) separatorInNamespaceNames() bool {
	return strings.ContainsAny(f.Separator, "abcdefghijklmnopqrstuvwxyz0123456789-")
}

// laysOutKey reports whether key is laid out as a key of the namespace: its prefix, then a type
// segment.
func (f RemoteKeyFormat) laysOutKey(namespace, key string) bool {
	rest, ok := strings.CutPrefix(key, f.namespacePrefix(namespace))
	if !ok {
		return false
	}
	for _, segment := range remoteKeyTypeSegments {
		if strings.HasPrefix(rest, segment+f.Separator) {
			return true
		}
	}
	return false
}

// sharedWith returns another namespace name that a key of the namespace is also laid out for, or ""
// if there is none. That can only happen when the separator can occur in namespace names: with "-",
// "secret-team-generic-generic-x" is both namespace "team"'s generic secret "generic-x" and namespace
// "team-generic"'s generic secret "x". Such a key belongs to neither, whether or not the other
// namespace exists: one created later could otherwise write the key and have its value synced to
// this namespace's channel.
func (f RemoteKeyFormat) sharedWith(namespace, key string) string {
	if !f.separatorInNamespaceNames() {
		return ""
	}
	for _, other := range f.keyNamespaces(key) {
		if other != namespace {
			return other
		}
	}
	return ""
}

// keyNamespaces returns every valid namespace name that the key is laid out for.
func (f RemoteKeyFormat) keyNamespaces(key string) []string {
	rest, ok := strings.CutPrefix(key, f.keyPrefix())
	if !ok {
		return nil
	}
	var namespaces []string
	for i := 1; i < len(rest); i++ {
		if namespace := rest[:i]; len(validation.IsDNS1123Label(namespace)) == 0 && f.laysOutKey(namespace, key) {
			namespaces = append(namespaces, namespace)
		}
	}
	return namespaces
}

// buildExternalSecret builds the ExternalSecret that syncs the channel's referenced values from the
// observability plane's secret store into the Secret the observer reads.
func buildExternalSecret(
	channel *openchoreodevv1alpha1.ObservabilityAlertsNotificationChannel,
	secretStoreName string,
	resolved *resolvedSecrets,
) (*unstructured.Unstructured, error) {
	data := make([]esv1.ExternalSecretData, 0, len(resolved.refs))
	var refreshInterval *metav1.Duration
	for _, ref := range resolved.refs {
		data = append(data, esv1.ExternalSecretData{
			SecretKey: ref.secretKey,
			RemoteRef: esv1.ExternalSecretDataRemoteRef{
				Key:      ref.remoteRef.Key,
				Property: ref.remoteRef.Property,
				Version:  ref.remoteRef.Version,
			},
		})
		// Refresh as often as the most frequently refreshed SecretReference asks for.
		if interval := ref.secretReference.Spec.RefreshInterval; interval != nil &&
			(refreshInterval == nil || interval.Duration < refreshInterval.Duration) {
			refreshInterval = interval
		}
	}

	es := &esv1.ExternalSecret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      channel.Name,
			Namespace: channel.Namespace,
			Labels:    channelResourceLabels(channel),
		},
		Spec: esv1.ExternalSecretSpec{
			SecretStoreRef: esv1.SecretStoreRef{
				Kind: "ClusterSecretStore",
				Name: secretStoreName,
			},
			Target: esv1.ExternalSecretTarget{
				Name:           channel.Name,
				CreationPolicy: esv1.CreatePolicyOwner,
				Template: &esv1.ExternalSecretTemplate{
					Type: corev1.SecretTypeOpaque,
					Metadata: esv1.ExternalSecretTemplateMetadata{
						Labels: map[string]string{
							"app.kubernetes.io/name":               channel.Name,
							labels.LabelKeyNotificationChannelName: channel.Name,
						},
					},
				},
			},
			RefreshInterval: refreshInterval,
			Data:            data,
		},
	}

	obj, err := runtime.DefaultUnstructuredConverter.ToUnstructured(es)
	if err != nil {
		return nil, fmt.Errorf("failed to convert ExternalSecret: %w", err)
	}
	u := &unstructured.Unstructured{Object: obj}
	u.SetGroupVersionKind(esv1.ExtSecretGroupVersionKind)
	// Leave out what the zero-valued typed fields put there; the controller owns neither.
	unstructured.RemoveNestedField(u.Object, "status")
	unstructured.RemoveNestedField(u.Object, "metadata", "creationTimestamp")
	return u, nil
}

// reconcileExternalSecret applies the ExternalSecret for a channel whose secret values come from
// SecretReferences, and reports its sync state on the channel.
func (r *Reconciler) reconcileExternalSecret(
	ctx context.Context,
	channel *openchoreodevv1alpha1.ObservabilityAlertsNotificationChannel,
	obsPlane *controller.ObservabilityPlaneResult,
	opClient client.Client,
	resolved *resolvedSecrets,
) error {
	if err := r.validateSecretReferences(channel, obsPlane, resolved); err != nil {
		return err
	}

	// A Secret this controller copied while the channel referenced Kubernetes Secrets would stop
	// the ExternalSecret from taking over the name, so remove it first.
	if err := r.deleteCopiedSecret(ctx, opClient, channel); err != nil {
		return err
	}

	// Don't take over an ExternalSecret with the channel's name that this controller didn't create.
	existing, err := getExternalSecret(ctx, opClient, channel.Name, channel.Namespace)
	if err != nil {
		return err
	}
	if existing != nil && !isManagedByController(existing) {
		return externalSecretConflictError(channel)
	}

	es, err := buildExternalSecret(channel, obsPlane.GetSecretStoreName(), resolved)
	if err != nil {
		return err
	}
	if err := opClient.Patch(ctx, es, client.Apply, client.ForceOwnership, client.FieldOwner(fieldOwner)); err != nil {
		return fmt.Errorf("failed to apply ExternalSecret %s: %w", es.GetName(), err)
	}

	current := newExternalSecretObject()
	if err := opClient.Get(ctx, client.ObjectKey{Name: channel.Name, Namespace: channel.Namespace}, current); err != nil {
		return fmt.Errorf("failed to get ExternalSecret %s: %w", channel.Name, err)
	}
	status, reason, message, err := externalSecretReadiness(current)
	if err != nil {
		return err
	}
	switch status {
	case metav1.ConditionTrue:
		controller.MarkTrueCondition(channel, ConditionSecretsReady, reason, message)
	case metav1.ConditionFalse:
		controller.MarkFalseCondition(channel, ConditionSecretsReady, reason, message)
	default:
		controller.MarkUnknownCondition(channel, ConditionSecretsReady, reason, message)
	}
	return nil
}

// externalSecretReadiness maps the ExternalSecret's Ready condition to the channel's SecretsReady condition.
// ESO updates the status asynchronously, so right after a spec change the status can still describe the
// previous spec. A Ready condition counts only once ESO has synced the current generation; a failure is
// reported as is.
func externalSecretReadiness(u *unstructured.Unstructured) (metav1.ConditionStatus, controller.ConditionReason, string, error) {
	es := &esv1.ExternalSecret{}
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(u.Object, es); err != nil {
		return "", "", "", fmt.Errorf("failed to convert ExternalSecret %s: %w", u.GetName(), err)
	}
	for _, cond := range es.Status.Conditions {
		if cond.Type != esv1.ExternalSecretReady {
			continue
		}
		switch cond.Status {
		case corev1.ConditionTrue:
			if !syncedGeneration(es) {
				break
			}
			return metav1.ConditionTrue, ReasonExternalSecretSynced, "secret values are synced from the secret store", nil
		case corev1.ConditionFalse:
			return metav1.ConditionFalse, ReasonExternalSecretNotSynced,
				fmt.Sprintf("ExternalSecret %s/%s is not synced: %s", es.Namespace, es.Name, cond.Message), nil
		}
	}
	return metav1.ConditionUnknown, ReasonExternalSecretPending,
		fmt.Sprintf("waiting for ExternalSecret %s/%s to sync", es.Namespace, es.Name), nil
}

// syncedGeneration reports whether ESO's last successful sync was of the ExternalSecret's current
// generation. ESO records the synced version as "<generation>-<hash of labels and annotations>".
func syncedGeneration(es *esv1.ExternalSecret) bool {
	generation, _, ok := strings.Cut(es.Status.SyncedResourceVersion, "-")
	return ok && generation == strconv.FormatInt(es.Generation, 10)
}

// deleteCopiedSecret deletes the channel's Secret on the observability plane if this controller
// copied it, leaving a Secret produced by the ExternalSecret alone.
func (r *Reconciler) deleteCopiedSecret(ctx context.Context, opClient client.Client, channel *openchoreodevv1alpha1.ObservabilityAlertsNotificationChannel) error {
	secret := &corev1.Secret{}
	if err := opClient.Get(ctx, client.ObjectKey{Name: channel.Name, Namespace: channel.Namespace}, secret); err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("failed to get Secret %s/%s: %w", channel.Namespace, channel.Name, err)
	}
	if !isManagedByController(secret) {
		return nil
	}
	if err := opClient.Delete(ctx, secret); err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("failed to delete copied Secret %s/%s: %w", channel.Namespace, channel.Name, err)
	}
	return nil
}

// deleteExternalSecret deletes the channel's ExternalSecret from the observability plane. It reports
// whether this controller's ExternalSecret existed and was deleted, and whether an ExternalSecret with
// that name that this controller didn't create is there instead, which it leaves alone.
func (r *Reconciler) deleteExternalSecret(ctx context.Context, opClient client.Client, name, namespace string) (deleted, foreign bool, err error) {
	// Look the ExternalSecret up first: the cluster gateway proxy client reports deleting an object
	// that does not exist as a success, so the delete alone can't tell whether there was one.
	es, err := getExternalSecret(ctx, opClient, name, namespace)
	if err != nil || es == nil {
		return false, false, err
	}
	if !isManagedByController(es) {
		return false, true, nil
	}
	if err := opClient.Delete(ctx, es); err != nil {
		if apierrors.IsNotFound(err) || meta.IsNoMatchError(err) {
			return false, false, nil
		}
		return false, false, fmt.Errorf("failed to delete ExternalSecret %s/%s: %w", namespace, name, err)
	}
	return true, false, nil
}

// getExternalSecret returns the ExternalSecret with the given name on the observability plane, or nil
// if there is none. An observability plane without the ExternalSecret API has none.
func getExternalSecret(ctx context.Context, opClient client.Client, name, namespace string) (*unstructured.Unstructured, error) {
	es := newExternalSecretObject()
	if err := opClient.Get(ctx, client.ObjectKey{Name: name, Namespace: namespace}, es); err != nil {
		if apierrors.IsNotFound(err) || meta.IsNoMatchError(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get ExternalSecret %s/%s: %w", namespace, name, err)
	}
	return es, nil
}

// isManagedByController reports whether this controller created the object.
func isManagedByController(obj client.Object) bool {
	return obj.GetLabels()["app.kubernetes.io/managed-by"] == managedByLabelValue
}

func externalSecretConflictError(channel *openchoreodevv1alpha1.ObservabilityAlertsNotificationChannel) error {
	return newInvalidSecretsError(ReasonExternalSecretConflict,
		"an ExternalSecret %s/%s that this controller didn't create already exists on the observability plane",
		channel.Namespace, channel.Name)
}

// isOwnedByExternalSecret reports whether the channel's Secret on the observability plane still
// belongs to an ExternalSecret, which deletes it once the ExternalSecret is gone.
func isOwnedByExternalSecret(ctx context.Context, opClient client.Client, name, namespace string) (bool, error) {
	secret := &corev1.Secret{}
	if err := opClient.Get(ctx, client.ObjectKey{Name: name, Namespace: namespace}, secret); err != nil {
		if apierrors.IsNotFound(err) {
			return false, nil
		}
		return false, fmt.Errorf("failed to get Secret %s/%s: %w", namespace, name, err)
	}
	for _, owner := range secret.OwnerReferences {
		if owner.Kind == esv1.ExtSecretKind {
			return true, nil
		}
	}
	return false, nil
}

func newExternalSecretObject() *unstructured.Unstructured {
	u := &unstructured.Unstructured{}
	u.SetGroupVersionKind(esv1.ExtSecretGroupVersionKind)
	return u
}

// isInvalidSecretsError reports whether err is a problem with the channel's secret references.
func isInvalidSecretsError(err error) (*invalidSecretsError, bool) {
	var invalid *invalidSecretsError
	if errors.As(err, &invalid) {
		return invalid, true
	}
	return nil, false
}
