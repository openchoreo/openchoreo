// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package observabilityalertsnotificationchannel

import (
	"context"
	"fmt"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	openchoreodevv1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	kubernetesClient "github.com/openchoreo/openchoreo/internal/clients/kubernetes"
	"github.com/openchoreo/openchoreo/internal/controller"
	"github.com/openchoreo/openchoreo/internal/labels"
)

const (
	// NotificationChannelCleanupFinalizer is the finalizer that is used to clean up notification channel resources.
	NotificationChannelCleanupFinalizer = "openchoreo.dev/notification-channel-cleanup"

	// externalSecretPendingPollInterval is how often a channel is reconciled while its ExternalSecret
	// has not reported a sync result yet, which it usually does within seconds. The ExternalSecret
	// lives on the observability plane, so its status can't be watched.
	externalSecretPendingPollInterval = 5 * time.Second

	// externalSecretPollInterval is how often a channel is reconciled while its ExternalSecret fails
	// to sync.
	externalSecretPollInterval = 30 * time.Second

	// planeConfigPollInterval is how often a channel is reconciled while its secret references fail
	// on the observability plane's configuration or contents (no secretStoreRef, a SecretReference that
	// targets another plane, or an ExternalSecret this controller didn't create in the way). Neither
	// planes nor the resources on them are watched, so a fix is only noticed by polling.
	planeConfigPollInterval = 2 * time.Minute

	// externalSecretRecheckInterval is how often a channel whose ExternalSecret has synced is reconciled
	// again, so a later sync failure (a store outage, a deleted key) shows on the channel. ExternalSecrets
	// live on the observability plane and are not watched.
	externalSecretRecheckInterval = 10 * time.Minute

	// secretReplacementPollInterval is how often a channel is reconciled while the Secret its
	// ExternalSecret produced is being deleted, so a copied Secret can take its name.
	secretReplacementPollInterval = 5 * time.Second
)

// Reconciler reconciles a ObservabilityAlertsNotificationChannel object
type Reconciler struct {
	client.Client
	PlaneClientProvider kubernetesClient.ObservabilityPlaneClientProvider
	Scheme              *runtime.Scheme
	Recorder            record.EventRecorder

	// RemoteKeys is how the secret API lays out keys in the external secret store. A channel can
	// only read SecretReferences whose remote keys belong to the channel's namespace in this layout.
	RemoteKeys RemoteKeyFormat
}

// +kubebuilder:rbac:groups=openchoreo.dev,resources=observabilityalertsnotificationchannels,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=openchoreo.dev,resources=observabilityalertsnotificationchannels/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=openchoreo.dev,resources=observabilityalertsnotificationchannels/finalizers,verbs=update
// +kubebuilder:rbac:groups=openchoreo.dev,resources=environments,verbs=get;list;watch
// +kubebuilder:rbac:groups=openchoreo.dev,resources=dataplanes,verbs=get;list;watch
// +kubebuilder:rbac:groups=openchoreo.dev,resources=clusterdataplanes,verbs=get;list;watch
// +kubebuilder:rbac:groups=openchoreo.dev,resources=observabilityplanes,verbs=get;list;watch
// +kubebuilder:rbac:groups=openchoreo.dev,resources=clusterobservabilityplanes,verbs=get;list;watch
// +kubebuilder:rbac:groups=openchoreo.dev,resources=secretreferences,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// It creates a ConfigMap and Secret with the same name as ObservabilityAlertsNotificationChannel
// and applies them to the observability plane cluster using cluster-gateway and cluster-agent architecture.
// When the channel's secret references name SecretReferences, the Secret is produced by an ExternalSecret
// that syncs the values from the observability plane's secret store. When they name Kubernetes Secrets
// (deprecated), the values are copied into the Secret.
// Owner references are not set on the ConfigMap and Secret as they are applied to a separate cluster.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.19.4/pkg/reconcile
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Fetch the ObservabilityAlertsNotificationChannel instance
	channel := &openchoreodevv1alpha1.ObservabilityAlertsNotificationChannel{}
	if err := r.Get(ctx, req.NamespacedName, channel); err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("ObservabilityAlertsNotificationChannel resource not found. Ignoring since it must be deleted.")
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Failed to get ObservabilityAlertsNotificationChannel")
		return ctrl.Result{}, err
	}

	// Handle deletion and finalizers
	if !channel.DeletionTimestamp.IsZero() {
		return r.finalize(ctx, channel)
	}

	// Ensure the first channel for an environment becomes the default automatically
	// Do this BEFORE ensuring finalizer to avoid validation issues when adding finalizer
	if changed, err := r.ensureDefaultChannel(ctx, channel); err != nil {
		logger.Error(err, "Failed to ensure environment default notification channel")
		return ctrl.Result{}, err
	} else if changed {
		// Refresh the channel so subsequent steps use the updated spec.
		if err := r.Get(ctx, req.NamespacedName, channel); err != nil {
			logger.Error(err, "Failed to refresh channel after defaulting")
			return ctrl.Result{}, err
		}
	}

	// Refresh channel before ensuring finalizer to ensure we have the latest version
	// This avoids validation errors if the channel was updated externally
	if err := r.Get(ctx, req.NamespacedName, channel); err != nil {
		logger.Error(err, "Failed to refresh channel before ensuring finalizer")
		return ctrl.Result{}, err
	}

	// Ensure finalizer is added (after ensuring default to avoid validation conflicts)
	if _, err := r.ensureFinalizer(ctx, channel); err != nil {
		logger.Error(err, "Failed to ensure finalizer")
		return ctrl.Result{}, err
	}

	old := channel.DeepCopy()
	result, err := r.reconcileChannel(ctx, channel)
	channel.Status.ObservedGeneration = channel.Generation
	if statusErr := r.updateStatus(ctx, old, channel); statusErr != nil {
		logger.Error(statusErr, "Failed to update notification channel status")
		if err == nil {
			return ctrl.Result{}, statusErr
		}
	}
	return result, err
}

// reconcileChannel applies the channel's ConfigMap and Secret (or the ExternalSecret that produces
// the Secret) to the observability plane, and records the outcome in the channel's conditions.
func (r *Reconciler) reconcileChannel(ctx context.Context, channel *openchoreodevv1alpha1.ObservabilityAlertsNotificationChannel) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Get ObservabilityPlane client
	obsPlane, opClient, err := r.getObservabilityPlaneClient(ctx, channel)
	if err != nil {
		logger.Error(err, "Failed to get observability plane client")
		controller.MarkFalseCondition(channel, ConditionReady, ReasonObservabilityPlaneUnavailable, err.Error())
		return ctrl.Result{}, err
	}

	// Create ConfigMap with the same name as the channel
	configMap := r.createConfigMap(channel)
	if err := r.applyConfigMap(ctx, opClient, configMap); err != nil {
		logger.Error(err, "Failed to apply ConfigMap to observability plane")
		controller.MarkFalseCondition(channel, ConditionReady, ReasonApplyFailed, err.Error())
		return ctrl.Result{}, err
	}

	// Provide the referenced secret values in a Secret with the same name as the channel
	var requeueAfter time.Duration
	resolved, err := r.resolveSecrets(ctx, channel)
	if err == nil {
		if resolved.source == secretSourceSecretReference {
			err = r.reconcileExternalSecret(ctx, channel, obsPlane, opClient, resolved)
		} else {
			requeueAfter, err = r.reconcileCopiedSecret(ctx, channel, opClient, resolved)
		}
	}
	if invalid, ok := isInvalidSecretsError(err); ok {
		logger.Info("Notification channel secret references cannot be resolved", "reason", invalid.reason, "message", invalid.message)
		controller.MarkFalseCondition(channel, ConditionSecretsReady, invalid.reason, invalid.message)
		controller.MarkFalseCondition(channel, ConditionReady, invalid.reason, invalid.message)
		if invalid.reason == ReasonSecretStoreNotConfigured || invalid.reason == ReasonSecretStoreUnreachable ||
			invalid.reason == ReasonExternalSecretConflict {
			// These depend on the observability plane, which is not watched, so poll for a fix.
			return ctrl.Result{RequeueAfter: planeConfigPollInterval}, nil
		}
		// Retrying won't help until the channel or what it references changes, and both are watched.
		return ctrl.Result{}, nil
	}
	if err != nil {
		logger.Error(err, "Failed to provide secret values to observability plane")
		controller.MarkFalseCondition(channel, ConditionReady, ReasonApplyFailed, err.Error())
		return ctrl.Result{}, err
	}

	secretsReady := meta.FindStatusCondition(channel.Status.Conditions, string(ConditionSecretsReady))
	if secretsReady == nil || secretsReady.Status != metav1.ConditionTrue {
		reason, message := ReasonExternalSecretPending, "waiting for the secret values to reach the observability plane"
		if secretsReady != nil {
			reason, message = controller.ConditionReason(secretsReady.Reason), secretsReady.Message
		}
		controller.MarkFalseCondition(channel, ConditionReady, reason, message)
		if requeueAfter == 0 {
			requeueAfter = externalSecretPollInterval
			if reason == ReasonExternalSecretPending {
				requeueAfter = externalSecretPendingPollInterval
			}
		}
		return ctrl.Result{RequeueAfter: requeueAfter}, nil
	}

	controller.MarkTrueCondition(channel, ConditionReady, ReasonReconciled,
		"ConfigMap and Secret are applied to the observability plane")
	logger.Info("Successfully applied notification channel to observability plane",
		"name", channel.Name, "namespace", channel.Namespace, "secretSource", resolved.source)
	if resolved.source == secretSourceSecretReference {
		return ctrl.Result{RequeueAfter: externalSecretRecheckInterval}, nil
	}
	return ctrl.Result{}, nil
}

// reconcileCopiedSecret copies the referenced values from Kubernetes Secrets into the channel's Secret on
// the observability plane. It also handles channels that reference no secret values, which still need an
// (empty) Secret for the observer to read. It returns how long to wait before reconciling again, if needed.
func (r *Reconciler) reconcileCopiedSecret(
	ctx context.Context,
	channel *openchoreodevv1alpha1.ObservabilityAlertsNotificationChannel,
	opClient client.Client,
	resolved *resolvedSecrets,
) (time.Duration, error) {
	// If the channel used SecretReferences before, the Secret belongs to its ExternalSecret. Remove the
	// ExternalSecret and wait for the Secret to go before applying the copied one under the same name.
	deleted, foreign, err := r.deleteExternalSecret(ctx, opClient, channel.Name, channel.Namespace)
	if err != nil {
		return 0, err
	}
	if foreign {
		return 0, externalSecretConflictError(channel)
	}
	owned, err := isOwnedByExternalSecret(ctx, opClient, channel.Name, channel.Namespace)
	if err != nil {
		return 0, err
	}
	if deleted || owned {
		controller.MarkUnknownCondition(channel, ConditionSecretsReady, ReasonReplacingSecret,
			"waiting for the Secret synced by the previous ExternalSecret to be deleted")
		return secretReplacementPollInterval, nil
	}

	secret, err := r.createSecret(ctx, channel)
	if err != nil {
		return 0, err
	}
	if err := r.applySecret(ctx, opClient, secret); err != nil {
		return 0, err
	}

	if resolved.source != secretSourceKubernetesSecret {
		controller.MarkTrueCondition(channel, ConditionSecretsReady, ReasonNoSecretsReferenced,
			"the channel references no secret values")
		return 0, nil
	}

	message := fmt.Sprintf("secret values are copied from Kubernetes Secrets %s. Referencing Kubernetes Secrets is deprecated; "+
		"create SecretReferences with the same names instead", strings.Join(resolved.secretNames, ", "))
	controller.MarkTrueCondition(channel, ConditionSecretsReady, ReasonDeprecatedKubernetesSecret, message)
	if r.Recorder != nil {
		r.Recorder.Event(channel, corev1.EventTypeWarning, string(ReasonDeprecatedKubernetesSecret), message)
	}
	return 0, nil
}

// updateStatus patches the channel's status if it changed during reconciliation.
func (r *Reconciler) updateStatus(ctx context.Context, old, channel *openchoreodevv1alpha1.ObservabilityAlertsNotificationChannel) error {
	if equality.Semantic.DeepEqual(old.Status, channel.Status) {
		return nil
	}
	return r.Status().Patch(ctx, channel, client.MergeFrom(old))
}

// getObservabilityPlaneClient gets the observability plane and a client for it by deriving it from the environment.
// It follows the chain: Channel.Environment -> Environment.DataPlaneRef -> DataPlane.ObservabilityPlaneRef -> ObservabilityPlane
func (r *Reconciler) getObservabilityPlaneClient(
	ctx context.Context,
	channel *openchoreodevv1alpha1.ObservabilityAlertsNotificationChannel,
) (*controller.ObservabilityPlaneResult, client.Client, error) {
	// Get the Environment
	env := &openchoreodevv1alpha1.Environment{}
	if err := r.Get(ctx, client.ObjectKey{Name: channel.Spec.Environment, Namespace: channel.Namespace}, env); err != nil {
		return nil, nil, fmt.Errorf("failed to get environment %s: %w", channel.Spec.Environment, err)
	}

	dataPlaneResult, err := controller.GetDataPlaneFromRef(ctx, r.Client, env.Namespace, env.Spec.DataPlaneRef)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to resolve dataplane for environment %s: %w", env.Name, err)
	}

	obsResult, err := dataPlaneResult.GetObservabilityPlane(ctx, r.Client)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to resolve observability plane for dataplane %s: %w", dataPlaneResult.GetName(), err)
	}

	opClient, err := obsResult.GetK8sClient(r.PlaneClientProvider)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create observability plane client for %s: %w", obsResult.GetName(), err)
	}

	return obsResult, opClient, nil
}

// channelResourceLabels returns the labels of the resources applied to the observability plane for the channel.
// The observer finds the channel's ConfigMap and Secret by the notification channel name label.
func channelResourceLabels(channel *openchoreodevv1alpha1.ObservabilityAlertsNotificationChannel) map[string]string {
	return map[string]string{
		"app.kubernetes.io/managed-by":         managedByLabelValue,
		"app.kubernetes.io/name":               channel.Name,
		labels.LabelKeyNotificationChannelName: channel.Name,
	}
}

// createConfigMap creates a ConfigMap with the same name as the channel
func (r *Reconciler) createConfigMap(channel *openchoreodevv1alpha1.ObservabilityAlertsNotificationChannel) *corev1.ConfigMap {
	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      channel.Name,
			Namespace: channel.Namespace,
			Labels:    channelResourceLabels(channel),
		},
		Data: map[string]string{
			"type":         string(channel.Spec.Type),
			"isEnvDefault": fmt.Sprintf("%t", channel.Spec.IsEnvDefault),
		},
	}

	// Add email-specific config if type is email
	if channel.Spec.Type == openchoreodevv1alpha1.NotificationChannelTypeEmail && channel.Spec.EmailConfig != nil {
		configMap.Data["from"] = channel.Spec.EmailConfig.From
		configMap.Data["to"] = fmt.Sprintf("%v", channel.Spec.EmailConfig.To)
		configMap.Data["smtp.host"] = channel.Spec.EmailConfig.SMTP.Host
		configMap.Data["smtp.port"] = fmt.Sprintf("%d", channel.Spec.EmailConfig.SMTP.Port)

		// Add template data if present
		if channel.Spec.EmailConfig.Template != nil {
			configMap.Data["template.subject"] = channel.Spec.EmailConfig.Template.Subject
			configMap.Data["template.body"] = channel.Spec.EmailConfig.Template.Body
		}

		// Add TLS config if present
		if channel.Spec.EmailConfig.SMTP.TLS != nil {
			configMap.Data["smtp.tls.insecureSkipVerify"] = fmt.Sprintf("%t", channel.Spec.EmailConfig.SMTP.TLS.InsecureSkipVerify)
		}
	}

	// Add webhook-specific config if type is webhook
	if channel.Spec.Type == openchoreodevv1alpha1.NotificationChannelTypeWebhook && channel.Spec.WebhookConfig != nil {
		configMap.Data["webhook.url"] = channel.Spec.WebhookConfig.URL
		// Store payload template if provided
		if channel.Spec.WebhookConfig.PayloadTemplate != "" {
			configMap.Data["webhook.payloadTemplate"] = channel.Spec.WebhookConfig.PayloadTemplate
		}
		// Store inline header values in ConfigMap, header names for secret-referenced values
		if len(channel.Spec.WebhookConfig.Headers) > 0 {
			headerKeys := make([]string, 0, len(channel.Spec.WebhookConfig.Headers))
			for key, headerValue := range channel.Spec.WebhookConfig.Headers {
				headerKeys = append(headerKeys, key)
				// Store inline values directly in ConfigMap
				if headerValue.Value != nil {
					configMap.Data[fmt.Sprintf("webhook.header.%s", key)] = *headerValue.Value
				}
			}
			configMap.Data["webhook.headers"] = strings.Join(headerKeys, ",")
		}
	}

	return configMap
}

// createSecret creates a Secret with the same name as the channel
// It resolves secret references from the control plane and copies the actual values
func (r *Reconciler) createSecret(ctx context.Context, channel *openchoreodevv1alpha1.ObservabilityAlertsNotificationChannel) (*corev1.Secret, error) {
	logger := log.FromContext(ctx)

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      channel.Name,
			Namespace: channel.Namespace,
			Labels:    channelResourceLabels(channel),
		},
		Type: corev1.SecretTypeOpaque,
		Data: make(map[string][]byte),
	}

	// Add SMTP auth credentials if present - resolve secret references and copy actual values
	if channel.Spec.Type == openchoreodevv1alpha1.NotificationChannelTypeEmail && channel.Spec.EmailConfig != nil && channel.Spec.EmailConfig.SMTP.Auth != nil {
		logger.Info("Resolving SMTP auth credentials",
			"hasUsernameRef", channel.Spec.EmailConfig.SMTP.Auth.Username != nil && channel.Spec.EmailConfig.SMTP.Auth.Username.SecretKeyRef != nil,
			"hasPasswordRef", channel.Spec.EmailConfig.SMTP.Auth.Password != nil && channel.Spec.EmailConfig.SMTP.Auth.Password.SecretKeyRef != nil)

		if channel.Spec.EmailConfig.SMTP.Auth.Username != nil && channel.Spec.EmailConfig.SMTP.Auth.Username.SecretKeyRef != nil {
			ref := channel.Spec.EmailConfig.SMTP.Auth.Username.SecretKeyRef
			logger.Info("Resolving SMTP username from secret",
				"secretName", ref.Name,
				"secretKey", ref.Key,
				"namespace", channel.Namespace)

			username, err := r.resolveSecretKeyRef(ctx, channel.Namespace, ref)
			if err != nil {
				logger.Error(err, "Failed to resolve SMTP username secret reference")
				return nil, fmt.Errorf("failed to resolve SMTP username secret: %w", err)
			}
			secret.Data["smtp.auth.username"] = []byte(username)
			logger.Info("SMTP username resolved successfully")
		}
		if channel.Spec.EmailConfig.SMTP.Auth.Password != nil && channel.Spec.EmailConfig.SMTP.Auth.Password.SecretKeyRef != nil {
			ref := channel.Spec.EmailConfig.SMTP.Auth.Password.SecretKeyRef
			logger.Info("Resolving SMTP password from secret",
				"secretName", ref.Name,
				"secretKey", ref.Key,
				"namespace", channel.Namespace)

			password, err := r.resolveSecretKeyRef(ctx, channel.Namespace, ref)
			if err != nil {
				logger.Error(err, "Failed to resolve SMTP password secret reference")
				return nil, fmt.Errorf("failed to resolve SMTP password secret: %w", err)
			}
			secret.Data["smtp.auth.password"] = []byte(password)
			logger.Info("SMTP password resolved successfully")
		}
	}

	// Add webhook header values if present - resolve secret references and copy actual values
	if channel.Spec.Type == openchoreodevv1alpha1.NotificationChannelTypeWebhook && channel.Spec.WebhookConfig != nil && len(channel.Spec.WebhookConfig.Headers) > 0 {
		logger.Info("Resolving webhook header values from secrets",
			"headerCount", len(channel.Spec.WebhookConfig.Headers))

		for headerName, headerValue := range channel.Spec.WebhookConfig.Headers {
			if headerValue.ValueFrom != nil && headerValue.ValueFrom.SecretKeyRef != nil {
				ref := headerValue.ValueFrom.SecretKeyRef
				logger.Info("Resolving webhook header from secret",
					"headerName", headerName,
					"secretName", ref.Name,
					"secretKey", ref.Key,
					"namespace", channel.Namespace)

				headerVal, err := r.resolveSecretKeyRef(ctx, channel.Namespace, ref)
				if err != nil {
					logger.Error(err, "Failed to resolve webhook header secret reference",
						"headerName", headerName)
					return nil, fmt.Errorf("failed to resolve webhook header %s secret: %w", headerName, err)
				}
				secret.Data[fmt.Sprintf("webhook.header.%s", headerName)] = []byte(headerVal)
				logger.Info("Webhook header resolved successfully", "headerName", headerName)
			}
		}
	}

	return secret, nil
}

// resolveSecretKeyRef resolves a SecretKeyRef to its actual value
func (r *Reconciler) resolveSecretKeyRef(ctx context.Context, namespace string, ref *openchoreodevv1alpha1.SecretKeyRef) (string, error) {
	secret := &corev1.Secret{}
	if err := r.Get(ctx, client.ObjectKey{Name: ref.Name, Namespace: namespace}, secret); err != nil {
		return "", fmt.Errorf("failed to get secret %s/%s: %w", namespace, ref.Name, err)
	}

	value, ok := secret.Data[ref.Key]
	if !ok {
		return "", fmt.Errorf("key %s not found in secret %s/%s", ref.Key, namespace, ref.Name)
	}

	// Trim whitespace/newlines that may have been accidentally included in the secret
	return strings.TrimSpace(string(value)), nil
}

// applyConfigMap applies the ConfigMap to the observability plane cluster
func (r *Reconciler) applyConfigMap(ctx context.Context, opClient client.Client, configMap *corev1.ConfigMap) error {
	// Set GroupVersionKind explicitly for server-side apply
	configMap.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "",
		Version: "v1",
		Kind:    "ConfigMap",
	})

	// Use server-side apply to create or update the ConfigMap
	if err := opClient.Patch(ctx, configMap, client.Apply, client.ForceOwnership, client.FieldOwner(fieldOwner)); err != nil {
		return fmt.Errorf("failed to apply ConfigMap %s: %w", configMap.Name, err)
	}
	return nil
}

// applySecret applies the Secret to the observability plane cluster
func (r *Reconciler) applySecret(ctx context.Context, opClient client.Client, secret *corev1.Secret) error {
	// Set GroupVersionKind explicitly for server-side apply
	secret.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "",
		Version: "v1",
		Kind:    "Secret",
	})

	// Use server-side apply to create or update the Secret
	if err := opClient.Patch(ctx, secret, client.Apply, client.ForceOwnership, client.FieldOwner(fieldOwner)); err != nil {
		return fmt.Errorf("failed to apply Secret %s: %w", secret.Name, err)
	}
	return nil
}

// ensureFinalizer ensures that the finalizer is added to the notification channel.
// The first return value indicates whether the finalizer was added to the channel.
func (r *Reconciler) ensureFinalizer(ctx context.Context, channel *openchoreodevv1alpha1.ObservabilityAlertsNotificationChannel) (bool, error) {
	// If the channel is being deleted, no need to add the finalizer
	if !channel.DeletionTimestamp.IsZero() {
		return false, nil
	}

	if controllerutil.AddFinalizer(channel, NotificationChannelCleanupFinalizer) {
		return true, r.Update(ctx, channel)
	}

	return false, nil
}

// finalize cleans up the resources associated with the notification channel.
func (r *Reconciler) finalize(ctx context.Context, channel *openchoreodevv1alpha1.ObservabilityAlertsNotificationChannel) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("channel", channel.Name, "namespace", channel.Namespace)

	if !controllerutil.ContainsFinalizer(channel, NotificationChannelCleanupFinalizer) {
		// Nothing to do if the finalizer is not present
		return ctrl.Result{}, nil
	}

	// Get ObservabilityPlane client
	_, opClient, err := r.getObservabilityPlaneClient(ctx, channel)
	if err != nil {
		logger.Error(err, "Failed to get observability plane client during finalization")
		return ctrl.Result{}, err
	}

	// Delete ConfigMap from observability plane
	if err := r.deleteConfigMap(ctx, opClient, channel.Name, channel.Namespace); err != nil {
		logger.Error(err, "Failed to delete ConfigMap from observability plane")
		return ctrl.Result{}, err
	}

	// Delete ExternalSecret from observability plane, if the channel used SecretReferences
	if _, _, err := r.deleteExternalSecret(ctx, opClient, channel.Name, channel.Namespace); err != nil {
		logger.Error(err, "Failed to delete ExternalSecret from observability plane")
		return ctrl.Result{}, err
	}

	// Delete Secret from observability plane
	if err := r.deleteSecret(ctx, opClient, channel.Name, channel.Namespace); err != nil {
		logger.Error(err, "Failed to delete Secret from observability plane")
		return ctrl.Result{}, err
	}

	// Remove the finalizer once cleanup is done
	if controllerutil.RemoveFinalizer(channel, NotificationChannelCleanupFinalizer) {
		if err := r.Update(ctx, channel); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to remove finalizer: %w", err)
		}
	}

	logger.Info("Successfully finalized notification channel")
	return ctrl.Result{}, nil
}

// deleteConfigMap deletes the ConfigMap from the observability plane cluster
func (r *Reconciler) deleteConfigMap(ctx context.Context, opClient client.Client, name, namespace string) error {
	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
	}

	if err := opClient.Delete(ctx, configMap); err != nil {
		if apierrors.IsNotFound(err) {
			// ConfigMap already deleted, which is fine
			return nil
		}
		return fmt.Errorf("failed to delete ConfigMap %s/%s: %w", namespace, name, err)
	}

	return nil
}

// deleteSecret deletes the Secret from the observability plane cluster
func (r *Reconciler) deleteSecret(ctx context.Context, opClient client.Client, name, namespace string) error {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
	}

	if err := opClient.Delete(ctx, secret); err != nil {
		if apierrors.IsNotFound(err) {
			// Secret already deleted, which is fine
			return nil
		}
		return fmt.Errorf("failed to delete Secret %s/%s: %w", namespace, name, err)
	}

	return nil
}

// ensureDefaultChannel sets IsEnvDefault=true when this is the first channel for the environment.
// It only mutates the resource if no other default exists for the same environment.
func (r *Reconciler) ensureDefaultChannel(ctx context.Context, channel *openchoreodevv1alpha1.ObservabilityAlertsNotificationChannel) (bool, error) {
	// If already marked default, nothing to do.
	if channel.Spec.IsEnvDefault {
		return false, nil
	}

	// Check if any other channel in the same namespace+environment is already default.
	var channels openchoreodevv1alpha1.ObservabilityAlertsNotificationChannelList
	if err := r.List(ctx, &channels, client.InNamespace(channel.Namespace)); err != nil {
		return false, fmt.Errorf("failed to list notification channels: %w", err)
	}

	envChannelCount := 0
	for _, existing := range channels.Items {
		if existing.Spec.Environment != channel.Spec.Environment {
			continue
		}
		if !existing.DeletionTimestamp.IsZero() {
			// Skip channels that are on their way out.
			continue
		}
		envChannelCount++
		if existing.Name == channel.Name {
			continue
		}
		if existing.Spec.IsEnvDefault {
			// A default already exists for this environment; leave this one unchanged.
			return false, nil
		}
	}

	// If this is the only active channel for the environment, make it default.
	if envChannelCount == 1 {
		log.FromContext(ctx).
			WithValues("channel", channel.Name, "namespace", channel.Namespace, "environment", channel.Spec.Environment).
			Info("Setting notification channel as default for environment (first channel)")
		original := channel.DeepCopy()
		channel.Spec.IsEnvDefault = true
		if err := r.Patch(ctx, channel, client.MergeFrom(original)); err != nil {
			return false, fmt.Errorf("failed to set channel as environment default: %w", err)
		}
		return true, nil
	}

	// No default exists; mark this channel as the default.
	logger := log.FromContext(ctx).WithValues("channel", channel.Name, "namespace", channel.Namespace, "environment", channel.Spec.Environment)
	logger.Info("Setting notification channel as default for environment")
	original := channel.DeepCopy()
	channel.Spec.IsEnvDefault = true
	if err := r.Patch(ctx, channel, client.MergeFrom(original)); err != nil {
		return false, fmt.Errorf("failed to set channel as environment default: %w", err)
	}

	return true, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	if r.Recorder == nil {
		r.Recorder = mgr.GetEventRecorderFor("observabilityalertsnotificationchannel-controller")
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&openchoreodevv1alpha1.ObservabilityAlertsNotificationChannel{}).
		// A change to a referenced SecretReference or Secret changes what the channel's Secret holds,
		// and creating or deleting one can change whether a reference resolves to a SecretReference.
		Watches(&openchoreodevv1alpha1.SecretReference{}, handler.EnqueueRequestsFromMapFunc(r.findChannelsReferencing)).
		Watches(&corev1.Secret{}, handler.EnqueueRequestsFromMapFunc(r.findChannelsReferencing)).
		Named("observabilityalertsnotificationchannel").
		Complete(r)
}

// findChannelsReferencing returns reconcile requests for the notification channels in the object's
// namespace whose secret references name the object.
func (r *Reconciler) findChannelsReferencing(ctx context.Context, obj client.Object) []reconcile.Request {
	var channels openchoreodevv1alpha1.ObservabilityAlertsNotificationChannelList
	if err := r.List(ctx, &channels, client.InNamespace(obj.GetNamespace())); err != nil {
		log.FromContext(ctx).Error(err, "Failed to list notification channels for secret reference change",
			"namespace", obj.GetNamespace(), "name", obj.GetName())
		return nil
	}

	var requests []reconcile.Request
	for i := range channels.Items {
		channel := &channels.Items[i]
		for _, ref := range channelSecretRefs(channel) {
			if ref.ref.Name == obj.GetName() {
				requests = append(requests, reconcile.Request{
					NamespacedName: client.ObjectKeyFromObject(channel),
				})
				break
			}
		}
	}
	return requests
}
