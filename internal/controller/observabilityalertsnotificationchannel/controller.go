// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package observabilityalertsnotificationchannel

import (
	"context"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	openchoreodevv1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	kubernetesClient "github.com/openchoreo/openchoreo/internal/clients/kubernetes"
	"github.com/openchoreo/openchoreo/internal/labels"
)

const (
	// NotificationChannelCleanupFinalizer is the finalizer that is used to clean up notification channel resources.
	NotificationChannelCleanupFinalizer = "openchoreo.dev/notification-channel-cleanup"
)

// Reconciler reconciles a ObservabilityAlertsNotificationChannel object
type Reconciler struct {
	client.Client
	K8sClientMgr *kubernetesClient.KubeMultiClientManager
	Scheme       *runtime.Scheme
	GatewayURL   string
}

// +kubebuilder:rbac:groups=openchoreo.dev,resources=observabilityalertsnotificationchannels,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=openchoreo.dev,resources=observabilityalertsnotificationchannels/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=openchoreo.dev,resources=observabilityalertsnotificationchannels/finalizers,verbs=update
// +kubebuilder:rbac:groups=openchoreo.dev,resources=environments,verbs=get;list;watch
// +kubebuilder:rbac:groups=openchoreo.dev,resources=dataplanes,verbs=get;list;watch
// +kubebuilder:rbac:groups=openchoreo.dev,resources=observabilityplanes,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// It creates a ConfigMap and Secret with the same name as ObservabilityAlertsNotificationChannel
// and applies them to the observability plane cluster using cluster-gateway and cluster-agent architecture.
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

	// Get ObservabilityPlane client
	opClient, err := r.getObservabilityPlaneClient(ctx, channel)
	if err != nil {
		logger.Error(err, "Failed to get observability plane client")
		return ctrl.Result{}, err
	}

	// Create ConfigMap with the same name as the channel
	configMap := r.createConfigMap(channel)
	if err := r.applyConfigMap(ctx, opClient, configMap); err != nil {
		logger.Error(err, "Failed to apply ConfigMap to observability plane")
		return ctrl.Result{}, err
	}

	// Create Secret with the same name as the channel
	secret, err := r.createSecret(ctx, channel)
	if err != nil {
		logger.Error(err, "Failed to create Secret for notification channel")
		return ctrl.Result{}, err
	}
	if err := r.applySecret(ctx, opClient, secret); err != nil {
		logger.Error(err, "Failed to apply Secret to observability plane")
		return ctrl.Result{}, err
	}

	logger.Info("Successfully applied ConfigMap and Secret to observability plane",
		"name", channel.Name, "namespace", channel.Namespace)
	return ctrl.Result{}, nil
}

// getObservabilityPlaneClient gets the observability plane client by deriving it from the environment.
// It follows the chain: Channel.Environment -> Environment.DataPlaneRef -> DataPlane.ObservabilityPlaneRef -> ObservabilityPlane
func (r *Reconciler) getObservabilityPlaneClient(ctx context.Context, channel *openchoreodevv1alpha1.ObservabilityAlertsNotificationChannel) (client.Client, error) {
	// Get the Environment
	env := &openchoreodevv1alpha1.Environment{}
	if err := r.Get(ctx, client.ObjectKey{Name: channel.Spec.Environment, Namespace: channel.Namespace}, env); err != nil {
		return nil, fmt.Errorf("failed to get environment %s: %w", channel.Spec.Environment, err)
	}

	// Check if DataPlaneRef is configured
	if env.Spec.DataPlaneRef == "" {
		return nil, fmt.Errorf("environment %s has no DataPlaneRef configured", env.Name)
	}

	// Get the DataPlane
	dataPlane := &openchoreodevv1alpha1.DataPlane{}
	if err := r.Get(ctx, client.ObjectKey{Name: env.Spec.DataPlaneRef, Namespace: channel.Namespace}, dataPlane); err != nil {
		return nil, fmt.Errorf("failed to get dataplane %s: %w", env.Spec.DataPlaneRef, err)
	}

	// Check if ObservabilityPlaneRef is configured
	if dataPlane.Spec.ObservabilityPlaneRef == "" {
		return nil, fmt.Errorf("dataplane %s has no ObservabilityPlaneRef configured", dataPlane.Name)
	}

	// Get the ObservabilityPlane
	observabilityPlane := &openchoreodevv1alpha1.ObservabilityPlane{}
	if err := r.Get(ctx, client.ObjectKey{
		Name:      dataPlane.Spec.ObservabilityPlaneRef,
		Namespace: channel.Namespace,
	}, observabilityPlane); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, fmt.Errorf("observability plane %s not found", dataPlane.Spec.ObservabilityPlaneRef)
		}
		return nil, fmt.Errorf("failed to get observability plane %s: %w", dataPlane.Spec.ObservabilityPlaneRef, err)
	}

	// Get Kubernetes client - supports agent mode (via HTTP proxy) through cluster gateway
	opClient, err := kubernetesClient.GetK8sClientFromObservabilityPlane(r.K8sClientMgr, observabilityPlane, r.GatewayURL)
	if err != nil {
		return nil, fmt.Errorf("failed to create observability plane client for %s: %w", observabilityPlane.Name, err)
	}

	return opClient, nil
}

// createConfigMap creates a ConfigMap with the same name as the channel
func (r *Reconciler) createConfigMap(channel *openchoreodevv1alpha1.ObservabilityAlertsNotificationChannel) *corev1.ConfigMap {
	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      channel.Name,
			Namespace: channel.Namespace,
			Labels: map[string]string{
				"app.kubernetes.io/managed-by":         "observabilityalertsnotificationchannel-controller",
				"app.kubernetes.io/name":               channel.Name,
				labels.LabelKeyNotificationChannelName: channel.Name,
			},
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
			Labels: map[string]string{
				"app.kubernetes.io/managed-by":         "observabilityalertsnotificationchannel-controller",
				"app.kubernetes.io/name":               channel.Name,
				labels.LabelKeyNotificationChannelName: channel.Name,
			},
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
	if err := opClient.Patch(ctx, configMap, client.Apply, client.ForceOwnership, client.FieldOwner("observabilityalertsnotificationchannel-controller")); err != nil {
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
	if err := opClient.Patch(ctx, secret, client.Apply, client.ForceOwnership, client.FieldOwner("observabilityalertsnotificationchannel-controller")); err != nil {
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
	opClient, err := r.getObservabilityPlaneClient(ctx, channel)
	if err != nil {
		logger.Error(err, "Failed to get observability plane client during finalization")
		return ctrl.Result{}, err
	}

	// Delete ConfigMap from observability plane
	if err := r.deleteConfigMap(ctx, opClient, channel.Name, channel.Namespace); err != nil {
		logger.Error(err, "Failed to delete ConfigMap from observability plane")
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
	if r.K8sClientMgr == nil {
		r.K8sClientMgr = kubernetesClient.NewManager()
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&openchoreodevv1alpha1.ObservabilityAlertsNotificationChannel{}).
		Named("observabilityalertsnotificationchannel").
		Complete(r)
}
