// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package observabilityalertsnotificationchannel

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	openchoreodevv1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	kubernetesClient "github.com/openchoreo/openchoreo/internal/clients/kubernetes"
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
// +kubebuilder:rbac:groups=openchoreo.dev,resources=observabilityplanes,verbs=get;list;watch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// It creates a ConfigMap and Secret with the same name as ObservabilityAlertsNotificationChannel
// and applies them to the observability plane cluster using cluster-gateway and cluster-agent architecture.
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

	// Get ObservabilityPlane client
	opClient, err := r.getObservabilityPlaneClient(ctx, channel.Namespace)
	if err != nil {
		logger.Error(err, "Failed to get observability plane client")
		return ctrl.Result{}, err
	}

	// Create ConfigMap with the same name as the channel
	configMap := r.createConfigMap(channel)
	// Set owner reference using scheme to get correct APIVersion and Kind
	if err := ctrl.SetControllerReference(channel, configMap, r.Scheme); err != nil {
		logger.Error(err, "Failed to set owner reference on ConfigMap")
		return ctrl.Result{}, err
	}
	if err := r.applyConfigMap(ctx, opClient, configMap); err != nil {
		logger.Error(err, "Failed to apply ConfigMap to observability plane")
		return ctrl.Result{}, err
	}

	// Create Secret with the same name as the channel
	secret := r.createSecret(channel)
	// Set owner reference using scheme to get correct APIVersion and Kind
	if err := ctrl.SetControllerReference(channel, secret, r.Scheme); err != nil {
		logger.Error(err, "Failed to set owner reference on Secret")
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

// getObservabilityPlaneClient gets the observability plane client
func (r *Reconciler) getObservabilityPlaneClient(ctx context.Context, namespace string) (client.Client, error) {
	// List ObservabilityPlanes in the namespace
	var observabilityPlanes openchoreodevv1alpha1.ObservabilityPlaneList
	if err := r.List(ctx, &observabilityPlanes, client.InNamespace(namespace)); err != nil {
		return nil, fmt.Errorf("failed to list ObservabilityPlanes in namespace %s: %w", namespace, err)
	}

	if len(observabilityPlanes.Items) == 0 {
		return nil, fmt.Errorf("no ObservabilityPlane found in namespace %s", namespace)
	}

	// Use the first ObservabilityPlane found
	observabilityPlane := &observabilityPlanes.Items[0]

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
				"app.kubernetes.io/managed-by": "observabilityalertsnotificationchannel-controller",
				"app.kubernetes.io/name":       channel.Name,
			},
		},
		Data: map[string]string{
			"type":      string(channel.Spec.Type),
			"from":      channel.Spec.Config.From,
			"to":        fmt.Sprintf("%v", channel.Spec.Config.To),
			"smtp.host": channel.Spec.Config.SMTP.Host,
			"smtp.port": fmt.Sprintf("%d", channel.Spec.Config.SMTP.Port),
		},
	}

	// Add template data if present
	if channel.Spec.Config.Template != nil {
		configMap.Data["template.subject"] = channel.Spec.Config.Template.Subject
		configMap.Data["template.body"] = channel.Spec.Config.Template.Body
	}

	// Add TLS config if present
	if channel.Spec.Config.SMTP.TLS != nil {
		configMap.Data["smtp.tls.insecureSkipVerify"] = fmt.Sprintf("%t", channel.Spec.Config.SMTP.TLS.InsecureSkipVerify)
	}

	return configMap
}

// createSecret creates a Secret with the same name as the channel
func (r *Reconciler) createSecret(channel *openchoreodevv1alpha1.ObservabilityAlertsNotificationChannel) *corev1.Secret {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      channel.Name,
			Namespace: channel.Namespace,
			Labels: map[string]string{
				"app.kubernetes.io/managed-by": "observabilityalertsnotificationchannel-controller",
				"app.kubernetes.io/name":       channel.Name,
			},
		},
		Type: corev1.SecretTypeOpaque,
		Data: make(map[string][]byte),
	}

	// Add SMTP auth credentials if present
	if channel.Spec.Config.SMTP.Auth != nil {
		if channel.Spec.Config.SMTP.Auth.Username != nil && channel.Spec.Config.SMTP.Auth.Username.SecretKeyRef != nil {
			// Note: This is a reference, not the actual value. In a real implementation,
			// you would need to resolve the secret reference and copy the value.
			// For now, we'll store the reference information.
			secret.Data["smtp.auth.username.secret"] = []byte(fmt.Sprintf("%s/%s", channel.Spec.Config.SMTP.Auth.Username.SecretKeyRef.Name, channel.Spec.Config.SMTP.Auth.Username.SecretKeyRef.Key))
		}
		if channel.Spec.Config.SMTP.Auth.Password != nil && channel.Spec.Config.SMTP.Auth.Password.SecretKeyRef != nil {
			secret.Data["smtp.auth.password.secret"] = []byte(fmt.Sprintf("%s/%s", channel.Spec.Config.SMTP.Auth.Password.SecretKeyRef.Name, channel.Spec.Config.SMTP.Auth.Password.SecretKeyRef.Key))
		}
	}

	return secret
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
