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
	To              []string
	SubjectTemplate string
	BodyTemplate    string
}

// NotificationChannelConfig combines SMTP and email configuration
type NotificationChannelConfig struct {
	SMTP  SMTPConfig
	Email EmailConfig
}

// SendEmailWithConfig sends an alert email using the provided configuration.
func SendEmailWithConfig(_ context.Context, config *NotificationChannelConfig, subject, body string) error {
	to := config.Email.To
	if len(to) == 0 {
		return fmt.Errorf("no recipients specified")
	}

	// Skip sending if no SMTP host is configured
	if config.SMTP.Host == "" || config.SMTP.Host == "smtp.example.com" {
		return nil
	}

	addr := fmt.Sprintf("%s:%d", config.SMTP.Host, config.SMTP.Port)
	var auth smtp.Auth
	if config.SMTP.Username != "" && config.SMTP.Password != "" {
		auth = smtp.PlainAuth("", config.SMTP.Username, config.SMTP.Password, config.SMTP.Host)
	}

	message := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\n\r\n%s",
		config.SMTP.From,
		strings.Join(to, ","),
		subject,
		body,
	)

	return smtp.SendMail(addr, auth, config.SMTP.From, to, []byte(message))
}
