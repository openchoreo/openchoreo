// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// NotificationChannelType defines the type of notification channel
// Currently "email" and "webhook" are supported. Other channel types will be added in the future.
// +kubebuilder:validation:Enum=email;webhook
type NotificationChannelType string

const (
	// NotificationChannelTypeEmail represents an email notification channel
	NotificationChannelTypeEmail NotificationChannelType = "email"
	// NotificationChannelTypeWebhook represents a webhook notification channel
	NotificationChannelTypeWebhook NotificationChannelType = "webhook"
)

// SecretValueFrom defines how to obtain a secret value
type SecretValueFrom struct {
	// SecretKeyRef references a specific key in a Kubernetes secret
	// +optional
	SecretKeyRef *SecretKeyRef `json:"secretKeyRef,omitempty"`
}

// EmailConfig defines the configuration for email notification channels
type EmailConfig struct {
	// From is the sender email address
	// Required when type is "email"
	// +optional
	From string `json:"from,omitempty"`

	// To is the list of recipient email addresses
	// Required when type is "email"
	// +optional
	// +kubebuilder:validation:MinItems=1
	To []string `json:"to,omitempty"`

	// SMTP configuration for sending emails
	// Required when type is "email"
	// +optional
	SMTP SMTPConfig `json:"smtp,omitempty"`

	// Template defines the email template using CEL expressions
	// +optional
	Template *EmailTemplate `json:"template,omitempty"`
}

// SMTPConfig defines SMTP server configuration
type SMTPConfig struct {
	// Host is the SMTP server hostname
	// Required when type is "email"
	// +optional
	Host string `json:"host,omitempty"`

	// Port is the SMTP server port
	// Required when type is "email"
	// +optional
	Port int32 `json:"port,omitempty"`

	// Auth defines SMTP authentication credentials
	// +optional
	Auth *SMTPAuth `json:"auth,omitempty"`

	// TLS configuration
	// +optional
	TLS *SMTPTLSConfig `json:"tls,omitempty"`
}

// SMTPAuth defines SMTP authentication configuration
type SMTPAuth struct {
	// Username for SMTP authentication
	// Can be provided inline or via secret reference
	// +optional
	Username *SecretValueFrom `json:"username,omitempty"`

	// Password for SMTP authentication
	// Can be provided inline or via secret reference
	// +optional
	Password *SecretValueFrom `json:"password,omitempty"`
}

// SMTPTLSConfig defines TLS configuration for SMTP
type SMTPTLSConfig struct {
	// InsecureSkipVerify skips TLS certificate verification (not recommended for production)
	// +optional
	InsecureSkipVerify bool `json:"insecureSkipVerify,omitempty"`
}

// EmailTemplate defines the email template with CEL expressions
type EmailTemplate struct {
	// Subject is the email subject line template using CEL expressions
	// Example: "[${alert.severity}] - ${alert.name} Triggered"
	// +kubebuilder:validation:Required
	Subject string `json:"subject"`

	// Body is the email body template using CEL expressions
	// Example: "Alert: ${alert.name} triggered at ${alert.startsAt}.\nSummary: ${alert.description}"
	// +kubebuilder:validation:Required
	Body string `json:"body"`
}

// WebhookHeaderValue defines a header value that can be provided inline or via secret reference
// +kubebuilder:validation:XValidation:rule="has(self.value) != has(self.valueFrom)",message="exactly one of value or valueFrom must be set"
type WebhookHeaderValue struct {
	// Value is the inline header value
	// Mutually exclusive with valueFrom
	// +optional
	Value *string `json:"value,omitempty"`

	// ValueFrom references a secret containing the header value
	// Mutually exclusive with value
	// +optional
	ValueFrom *SecretValueFrom `json:"valueFrom,omitempty"`
}

// WebhookConfig defines the configuration for webhook notification channels
type WebhookConfig struct {
	// URL is the webhook endpoint URL where alerts will be sent
	// Required when type is "webhook"
	// +optional
	// +kubebuilder:validation:Format=uri
	URL string `json:"url,omitempty"`

	// Headers are optional HTTP headers to include in the webhook request
	// Headers can be provided inline or via secret references
	// +optional
	Headers map[string]WebhookHeaderValue `json:"headers,omitempty"`

	// PayloadTemplate is an optional JSON payload template using CEL expressions
	// If not provided, the raw alertDetails object will be sent as JSON
	// CEL expressions use ${...} syntax and have access to alert fields:
	// - ${alert.ruleName}, ${alert.description}, ${alert.severity}, ${alert.value}, etc.
	// Example for Slack: {"text": "Alert: ${alert.ruleName}", "blocks": [...]}
	// +optional
	PayloadTemplate string `json:"payloadTemplate,omitempty"`
}

// NotificationChannelConfig is deprecated. Use EmailConfig and WebhookConfig directly in the spec instead.
// This type is kept for backward compatibility but should not be used in new code.
type NotificationChannelConfig struct {
	// EmailConfig is embedded to allow direct access to email fields at the config level
	// +optional
	EmailConfig `json:",inline"`

	// WebhookConfig is embedded to allow direct access to webhook fields at the config level
	// +optional
	WebhookConfig `json:",inline"`
}

// ObservabilityAlertsNotificationChannelSpec defines the desired state of ObservabilityAlertsNotificationChannel.
// +kubebuilder:validation:XValidation:rule="self.type == 'email' ? has(self.emailConfig) : true",message="emailConfig is required when type is email"
// +kubebuilder:validation:XValidation:rule="self.type == 'webhook' ? has(self.webhookConfig) : true",message="webhookConfig is required when type is webhook"
// +kubebuilder:validation:XValidation:rule="self.type == 'email' && has(self.emailConfig) ? (has(self.emailConfig.from) && size(self.emailConfig.from) > 0) && (has(self.emailConfig.to) && size(self.emailConfig.to) > 0) && (has(self.emailConfig.smtp) && has(self.emailConfig.smtp.host) && size(self.emailConfig.smtp.host) > 0 && has(self.emailConfig.smtp.port) && self.emailConfig.smtp.port >= 1 && self.emailConfig.smtp.port <= 65535) : true",message="emailConfig fields (from, to, smtp.host, smtp.port) are required when type is email, and smtp.port must be between 1 and 65535"
// +kubebuilder:validation:XValidation:rule="self.type == 'webhook' && has(self.webhookConfig) ? (has(self.webhookConfig.url) && size(self.webhookConfig.url) > 0) : true",message="webhookConfig.url is required when type is webhook"
type ObservabilityAlertsNotificationChannelSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Environment is the name of the openchoreo environment this notification channel belongs to
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="spec.environment is immutable"
	Environment string `json:"environment"`

	// IsEnvDefault indicates if this is the default notification channel for the environment
	// There can be only one default notification channel for an environment
	// First notification channel created for an environment will be the default unless otherwise specified
	// +kubebuilder:default=false
	IsEnvDefault bool `json:"isEnvDefault,omitempty"`

	// Type specifies the type of notification channel
	// Currently "email" and "webhook" are supported
	// +kubebuilder:validation:Required
	Type NotificationChannelType `json:"type"`

	// EmailConfig contains the email notification channel configuration
	// Required when type is "email"
	// +optional
	EmailConfig *EmailConfig `json:"emailConfig,omitempty"`

	// WebhookConfig contains the webhook notification channel configuration
	// Required when type is "webhook"
	// +optional
	WebhookConfig *WebhookConfig `json:"webhookConfig,omitempty"`
}

// ObservabilityAlertsNotificationChannelStatus defines the observed state of ObservabilityAlertsNotificationChannel.
type ObservabilityAlertsNotificationChannelStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Type",type=string,JSONPath=`.spec.type`
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:printcolumn:name="Notifications",type=integer,JSONPath=`.status.notificationCount`

// ObservabilityAlertsNotificationChannel is the Schema for the observabilityalertsnotificationchannels API.
// It defines a channel for sending alert notifications. Currently email and webhook notifications are supported.
type ObservabilityAlertsNotificationChannel struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ObservabilityAlertsNotificationChannelSpec   `json:"spec,omitempty"`
	Status ObservabilityAlertsNotificationChannelStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ObservabilityAlertsNotificationChannelList contains a list of ObservabilityAlertsNotificationChannel.
type ObservabilityAlertsNotificationChannelList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ObservabilityAlertsNotificationChannel `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ObservabilityAlertsNotificationChannel{}, &ObservabilityAlertsNotificationChannelList{})
}
