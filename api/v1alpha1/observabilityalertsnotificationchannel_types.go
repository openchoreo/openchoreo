// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// NotificationChannelType defines the type of notification channel
// Currently only "email" is supported. Other channel types will be added in the future.
// +kubebuilder:validation:Enum=email
type NotificationChannelType string

const (
	// NotificationChannelTypeEmail represents an email notification channel
	NotificationChannelTypeEmail NotificationChannelType = "email"
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
	// +kubebuilder:validation:Required
	From string `json:"from"`

	// To is the list of recipient email addresses
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinItems=1
	To []string `json:"to"`

	// SMTP configuration for sending emails
	// +kubebuilder:validation:Required
	SMTP SMTPConfig `json:"smtp"`

	// Template defines the email template using CEL expressions
	// +optional
	Template *EmailTemplate `json:"template,omitempty"`
}

// SMTPConfig defines SMTP server configuration
type SMTPConfig struct {
	// Host is the SMTP server hostname
	// +kubebuilder:validation:Required
	Host string `json:"host"`

	// Port is the SMTP server port
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	Port int32 `json:"port"`

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

// NotificationChannelConfig defines the configuration for notification channels
// Currently only email configuration is supported. The structure will be extended
// to support other channel types (e.g., Slack, Webhook, PagerDuty) in the future.
// When type is "email", this struct directly contains the email configuration fields.
type NotificationChannelConfig struct {
	// EmailConfig is embedded to allow direct access to email fields at the config level
	// This matches the YAML structure where config directly contains from, to, smtp, template fields
	EmailConfig `json:",inline"`
}

// ObservabilityAlertsNotificationChannelSpec defines the desired state of ObservabilityAlertsNotificationChannel.
// +kubebuilder:validation:XValidation:rule="self.type == 'email' ? has(self.config.from) && has(self.config.to) && has(self.config.smtp) : true",message="email config fields (from, to, smtp) are required when type is email"
type ObservabilityAlertsNotificationChannelSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Environment is the name of the openchoreo environment this notification channel belongs to
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="spec.environment is immutable"
	Environment string `json:"environment"`

	// Type specifies the type of notification channel
	// Currently only "email" is supported
	// +kubebuilder:validation:Required
	Type NotificationChannelType `json:"type"`

	// Config contains the channel-specific configuration
	// Currently only email configuration is supported
	// +kubebuilder:validation:Required
	Config NotificationChannelConfig `json:"config"`
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
// It defines a channel for sending alert notifications. Currently only email notifications are supported.
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
