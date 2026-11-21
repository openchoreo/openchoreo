// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package notifications

import (
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"
	"net"
	"net/mail"
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
	// FromName is an optional sender display name used only in the From header.
	FromName string
}

// EmailConfig holds email-specific configuration
type EmailConfig struct {
	SMTP            SMTPConfig
	To              []string
	SubjectTemplate string
	BodyTemplate    string
	ContentType     string // text/plain or text/html
}

// NotificationChannelConfig combines email and webhook configuration
type NotificationChannelConfig struct {
	Type    string // "email" or "webhook"
	Email   EmailConfig
	Webhook WebhookConfig
}

var smtpSendMail = sendMailContext

// SendEmailWithConfig sends an alert email using the provided configuration. The send is bounded
// by ctx: it dials with ctx and gives the connection ctx's deadline.
func SendEmailWithConfig(ctx context.Context, config *NotificationChannelConfig, subject, body string) error {
	to := config.Email.To
	if len(to) == 0 {
		return newDeliveryError(ReasonChannelConfig, "no recipients", fmt.Errorf("no recipients specified"))
	}

	// A channel without a real SMTP host cannot deliver; report it rather than claim success.
	if config.Email.SMTP.Host == "" || config.Email.SMTP.Host == "smtp.example.com" {
		return newDeliveryError(ReasonChannelConfig, "SMTP host is not configured", fmt.Errorf("SMTP host is not configured"))
	}

	addr := fmt.Sprintf("%s:%d", config.Email.SMTP.Host, config.Email.SMTP.Port)
	var auth smtp.Auth
	if config.Email.SMTP.Username != "" && config.Email.SMTP.Password != "" {
		auth = smtp.PlainAuth("", config.Email.SMTP.Username, config.Email.SMTP.Password, config.Email.SMTP.Host)
	}

	contentType := config.Email.ContentType
	if contentType != "text/html" {
		contentType = "text/plain" // default to text/plain if not specified
	}

	// The From header carries the display name (if any); the envelope sender (MAIL FROM)
	// must remain the bare address. mail.Address quotes the name and RFC 2047-encodes
	// non-ASCII or control characters, so the name cannot inject extra headers.
	// A whitespace-only name is treated as unset, so the header never shows a blank name.
	fromHeader := config.Email.SMTP.From
	if fromName := strings.TrimSpace(config.Email.SMTP.FromName); fromName != "" {
		fromHeader = (&mail.Address{Name: fromName, Address: config.Email.SMTP.From}).String()
	}

	message := fmt.Sprintf(
		"From: %s\r\n"+
			"To: %s\r\n"+
			"Subject: %s\r\n"+
			"MIME-Version: 1.0\r\n"+
			"Content-Type: %s; charset=\"UTF-8\"\r\n"+
			"Content-Transfer-Encoding: 8bit\r\n"+
			"\r\n"+
			"%s",
		fromHeader,
		strings.Join(to, ","),
		subject,
		contentType,
		body,
	)

	return smtpSendMail(ctx, addr, auth, config.Email.SMTP.From, to, []byte(message))
}

// sendMailContext sends like smtp.SendMail (STARTTLS when offered, then AUTH, MAIL, RCPT and
// DATA), but bounded by ctx. smtp.SendMail has no deadline, so a server that accepts the
// connection and stays silent would hold the send forever.
func sendMailContext(ctx context.Context, addr string, auth smtp.Auth, from string, to []string, msg []byte) error {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return newDeliveryError(ReasonChannelConfig, "invalid SMTP address", err)
	}

	conn, err := smtpDial(ctx, "tcp", addr)
	if err != nil {
		return err
	}
	defer conn.Close()
	if deadline, ok := ctx.Deadline(); ok {
		if err := conn.SetDeadline(deadline); err != nil {
			return err
		}
	}
	// Canceling ctx unblocks a pending read or write.
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stop()

	c, err := smtp.NewClient(conn, host)
	if err != nil {
		return contextErr(ctx, err)
	}
	defer c.Close()

	// Extension hides a failed greeting, so greet first to report why it failed.
	if err := c.Hello("localhost"); err != nil {
		return contextErr(ctx, err)
	}
	if ok, _ := c.Extension("STARTTLS"); ok {
		if err := c.StartTLS(&tls.Config{ServerName: host, MinVersion: tls.VersionTLS12}); err != nil {
			return contextErr(ctx, err)
		}
	}
	if auth != nil {
		if ok, _ := c.Extension("AUTH"); !ok {
			return newDeliveryError(ReasonChannelConfig, "SMTP server does not support AUTH",
				fmt.Errorf("smtp: server doesn't support AUTH"))
		}
		if err := c.Auth(auth); err != nil {
			return contextErr(ctx, err)
		}
	}
	if err := c.Mail(from); err != nil {
		return contextErr(ctx, err)
	}
	for _, rcpt := range to {
		if err := c.Rcpt(rcpt); err != nil {
			return contextErr(ctx, err)
		}
	}
	w, err := c.Data()
	if err != nil {
		return contextErr(ctx, err)
	}
	if _, err := w.Write(msg); err != nil {
		return contextErr(ctx, err)
	}
	if err := w.Close(); err != nil {
		return contextErr(ctx, err)
	}
	// The server has accepted the message, so a failed QUIT must not cause a resend.
	_ = c.Quit()
	return nil
}

// contextErr reports an error caused by ctx ending as ctx's error, since closing the connection
// on cancellation surfaces as a plain network error.
func contextErr(ctx context.Context, err error) error {
	if err != nil && ctx.Err() != nil {
		return fmt.Errorf("%w: %w", ctx.Err(), err)
	}
	return err
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

	contentType := configMap.Data["template.contentType"]
	if contentType == "" {
		contentType = "text/plain" // default to text/plain if not specified
	}

	emailConfig := EmailConfig{
		SMTP: SMTPConfig{
			Host:     configMap.Data["smtp.host"],
			Port:     smtpPort,
			From:     configMap.Data["from"],
			FromName: configMap.Data["fromName"],
		},
		To:              recipients,
		SubjectTemplate: configMap.Data["template.subject"],
		BodyTemplate:    configMap.Data["template.body"],
		ContentType:     contentType,
	}

	// Read SMTP credentials directly from the secret
	if secret != nil && secret.Data != nil {
		logger.Debug("Reading SMTP credentials from secret",
			"secretName", secret.Name,
			"secretNamespace", secret.Namespace)

		if username, ok := secret.Data["smtp.auth.username"]; ok {
			emailConfig.SMTP.Username = string(username)
			logger.Debug("SMTP username loaded")
		} else {
			logger.Warn("SMTP username key not found in secret")
		}
		if password, ok := secret.Data["smtp.auth.password"]; ok {
			emailConfig.SMTP.Password = string(password)
			logger.Debug("SMTP password loaded")
		} else {
			logger.Warn("SMTP password key not found in secret")
		}
	} else {
		logger.Warn("Secret is nil or has no data",
			"secretNil", secret == nil)
	}

	logger.Debug("Final SMTP config",
		"host", emailConfig.SMTP.Host,
		"port", emailConfig.SMTP.Port,
		"from", emailConfig.SMTP.From,
		"fromName", emailConfig.SMTP.FromName,
		"hasUsername", emailConfig.SMTP.Username != "",
		"hasPassword", emailConfig.SMTP.Password != "")

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
