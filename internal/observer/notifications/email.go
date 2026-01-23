// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package notifications

import (
	"context"
	"fmt"
	"net/smtp"
	"strings"
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
