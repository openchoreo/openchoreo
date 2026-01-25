// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package notifications

import (
	"context"
	"fmt"
	"log/slog"
	"net/smtp"
	"strconv"
	"strings"

	corev1 "k8s.io/api/core/v1"
)

// SMTPConfig holds SMTP configuration for sending emails
type SMTPConfig struct {
	Host     string
	Port     int
	Username string
	Password string
	From     string
}

// EmailConfig holds email-specific configuration
type EmailConfig struct {
	SMTP            SMTPConfig
	To              []string
	SubjectTemplate string
	BodyTemplate    string
}

// NotificationChannelConfig combines email and webhook configuration
type NotificationChannelConfig struct {
	Type    string // "email" or "webhook"
	Email   EmailConfig
	Webhook WebhookConfig
}

// SendEmailWithConfig sends an alert email using the provided configuration.
func SendEmailWithConfig(_ context.Context, config *NotificationChannelConfig, subject, body string) error {
	to := config.Email.To
	if len(to) == 0 {
		return fmt.Errorf("no recipients specified")
	}

	// Skip sending if no SMTP host is configured
	if config.Email.SMTP.Host == "" || config.Email.SMTP.Host == "smtp.example.com" {
		return nil
	}

	addr := fmt.Sprintf("%s:%d", config.Email.SMTP.Host, config.Email.SMTP.Port)
	var auth smtp.Auth
	if config.Email.SMTP.Username != "" && config.Email.SMTP.Password != "" {
		auth = smtp.PlainAuth("", config.Email.SMTP.Username, config.Email.SMTP.Password, config.Email.SMTP.Host)
	}

	message := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\n\r\n%s",
		config.Email.SMTP.From,
		strings.Join(to, ","),
		subject,
		body,
	)

	return smtp.SendMail(addr, auth, config.Email.SMTP.From, to, []byte(message))
}

// PrepareEmailNotificationConfig prepares email notification configuration from ConfigMap and Secret
func PrepareEmailNotificationConfig(configMap *corev1.ConfigMap, secret *corev1.Secret, logger *slog.Logger) (EmailConfig, error) {
	// Parse SMTP port from ConfigMap
	smtpPort := 587 // default SMTP port
	if portStr, ok := configMap.Data["smtp.port"]; ok {
		if port, err := strconv.Atoi(portStr); err == nil {
			smtpPort = port
		}
	}

	// Parse recipients from ConfigMap (stored as string representation of array)
	var recipients []string
	if toStr, ok := configMap.Data["to"]; ok {
		// The 'to' field is stored as a string like "[email1@example.com email2@example.com]"
		// Parse it back to a slice
		recipients = parseRecipientsList(toStr)
	}

	emailConfig := EmailConfig{
		SMTP: SMTPConfig{
			Host: configMap.Data["smtp.host"],
			Port: smtpPort,
			From: configMap.Data["from"],
		},
		To:              recipients,
		SubjectTemplate: configMap.Data["template.subject"],
		BodyTemplate:    configMap.Data["template.body"],
	}

	// Read SMTP credentials directly from the secret
	if secret != nil && secret.Data != nil {
		if logger != nil {
			logger.Debug("Reading SMTP credentials from secret",
				"secretName", secret.Name,
				"secretNamespace", secret.Namespace)
		}

		if username, ok := secret.Data["smtp.auth.username"]; ok {
			emailConfig.SMTP.Username = string(username)
			if logger != nil {
				logger.Debug("SMTP username loaded")
			}
		} else if logger != nil {
			logger.Warn("SMTP username key not found in secret")
		}
		if password, ok := secret.Data["smtp.auth.password"]; ok {
			emailConfig.SMTP.Password = string(password)
			if logger != nil {
				logger.Debug("SMTP password loaded")
			}
		} else if logger != nil {
			logger.Warn("SMTP password key not found in secret")
		}
	} else if logger != nil {
		logger.Warn("Secret is nil or has no data",
			"secretNil", secret == nil)
	}

	if logger != nil {
		logger.Debug("Final SMTP config",
			"host", emailConfig.SMTP.Host,
			"port", emailConfig.SMTP.Port,
			"from", emailConfig.SMTP.From,
			"hasUsername", emailConfig.SMTP.Username != "",
			"hasPassword", emailConfig.SMTP.Password != "")
	}

	return emailConfig, nil
}

// parseRecipientsList parses a string representation of recipients list
// The format is "[email1@example.com email2@example.com]" as stored by the controller
func parseRecipientsList(s string) []string {
	// Remove brackets if present
	s = strings.TrimPrefix(s, "[")
	s = strings.TrimSuffix(s, "]")
	s = strings.TrimSpace(s)

	if s == "" {
		return nil
	}

	// Split by whitespace
	parts := strings.Fields(s)
	return parts
}
