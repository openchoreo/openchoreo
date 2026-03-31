// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package notifications

import (
	"context"
	"fmt"
	"net/smtp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
)

// --- parseRecipientsList ---

func TestParseRecipientsList(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{
			name:  "empty string returns nil",
			input: "",
			want:  nil,
		},
		{
			name:  "single email without brackets",
			input: "alice@example.com",
			want:  []string{"alice@example.com"},
		},
		{
			name:  "single email with brackets",
			input: "[alice@example.com]",
			want:  []string{"alice@example.com"},
		},
		{
			name:  "multiple emails with brackets and spaces",
			input: "[alice@example.com bob@example.com carol@example.com]",
			want:  []string{"alice@example.com", "bob@example.com", "carol@example.com"},
		},
		{
			name:  "whitespace-only string returns nil",
			input: "   ",
			want:  nil,
		},
		{
			name:  "brackets with only spaces returns nil",
			input: "[   ]",
			want:  nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := parseRecipientsList(tc.input)
			assert.Equal(t, tc.want, got)
		})
	}
}

// --- PrepareEmailNotificationConfig ---

func TestPrepareEmailNotificationConfig(t *testing.T) {
	logger := discardLogger()

	tests := []struct {
		name          string
		configMapData map[string]string
		secretData    map[string][]byte
		secret        *corev1.Secret
		check         func(t *testing.T, cfg EmailConfig)
		wantErr       bool
	}{
		{
			name: "all fields present in ConfigMap and Secret",
			configMapData: map[string]string{
				"smtp.host":        "mail.example.com",
				"smtp.port":        "465",
				"from":             "noreply@example.com",
				"to":               "[alice@example.com bob@example.com]",
				"template.subject": "Alert: {{alertName}}",
				"template.body":    "Body: {{alertName}}",
			},
			secretData: map[string][]byte{
				"smtp.auth.username": []byte("smtpuser"),
				"smtp.auth.password": []byte("smtppass"),
			},
			check: func(t *testing.T, cfg EmailConfig) {
				assert.Equal(t, "mail.example.com", cfg.SMTP.Host)
				assert.Equal(t, 465, cfg.SMTP.Port)
				assert.Equal(t, "noreply@example.com", cfg.SMTP.From)
				assert.Equal(t, []string{"alice@example.com", "bob@example.com"}, cfg.To)
				assert.Equal(t, "Alert: {{alertName}}", cfg.SubjectTemplate)
				assert.Equal(t, "Body: {{alertName}}", cfg.BodyTemplate)
				assert.Equal(t, "smtpuser", cfg.SMTP.Username)
				assert.Equal(t, "smtppass", cfg.SMTP.Password)
			},
		},
		{
			name:          "missing smtp.port defaults to 587",
			configMapData: map[string]string{"smtp.host": "mail.example.com"},
			check: func(t *testing.T, cfg EmailConfig) {
				assert.Equal(t, 587, cfg.SMTP.Port)
			},
		},
		{
			name:          "invalid smtp.port string defaults to 587",
			configMapData: map[string]string{"smtp.host": "mail.example.com", "smtp.port": "notanumber"},
			check: func(t *testing.T, cfg EmailConfig) {
				assert.Equal(t, 587, cfg.SMTP.Port)
			},
		},
		{
			name:          "nil secret does not panic",
			configMapData: map[string]string{"smtp.host": "mail.example.com"},
			secret:        nil, // explicitly nil
			check: func(t *testing.T, cfg EmailConfig) {
				assert.Empty(t, cfg.SMTP.Username)
				assert.Empty(t, cfg.SMTP.Password)
			},
		},
		{
			name:          "secret with empty Data map does not panic",
			configMapData: map[string]string{"smtp.host": "mail.example.com"},
			secret:        &corev1.Secret{Data: nil},
			check: func(t *testing.T, cfg EmailConfig) {
				assert.Empty(t, cfg.SMTP.Username)
				assert.Empty(t, cfg.SMTP.Password)
			},
		},
		{
			name:          "missing credential keys warns but no error",
			configMapData: map[string]string{"smtp.host": "mail.example.com"},
			secretData:    map[string][]byte{}, // has Data but no credential keys
			check: func(t *testing.T, cfg EmailConfig) {
				assert.Empty(t, cfg.SMTP.Username)
				assert.Empty(t, cfg.SMTP.Password)
			},
		},
		{
			name:          "recipients parsed from bracket format",
			configMapData: map[string]string{"to": "[a@b.com c@d.com]"},
			check: func(t *testing.T, cfg EmailConfig) {
				assert.Equal(t, []string{"a@b.com", "c@d.com"}, cfg.To)
			},
		},
		{
			name:          "subject and body templates populated",
			configMapData: map[string]string{"template.subject": "subj", "template.body": "body"},
			check: func(t *testing.T, cfg EmailConfig) {
				assert.Equal(t, "subj", cfg.SubjectTemplate)
				assert.Equal(t, "body", cfg.BodyTemplate)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cm := &corev1.ConfigMap{Data: tc.configMapData}

			// Determine which secret to pass
			var secret *corev1.Secret
			if tc.secret != nil {
				secret = tc.secret
			} else if tc.secretData != nil {
				secret = &corev1.Secret{Data: tc.secretData}
			}

			cfg, err := PrepareEmailNotificationConfig(cm, secret, logger)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			if tc.check != nil {
				tc.check(t, cfg)
			}
		})
	}
}

// --- SendEmailWithConfig ---

func TestSendEmailWithConfig_NoRecipients(t *testing.T) {
	config := &NotificationChannelConfig{
		Email: EmailConfig{To: nil},
	}
	err := SendEmailWithConfig(context.Background(), config, "subj", "body")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no recipients specified")
}

func TestSendEmailWithConfig_SkipEmptyHost(t *testing.T) {
	called := false
	orig := smtpSendMail
	t.Cleanup(func() { smtpSendMail = orig })
	smtpSendMail = func(addr string, a smtp.Auth, from string, to []string, msg []byte) error {
		called = true
		return nil
	}

	config := &NotificationChannelConfig{
		Email: EmailConfig{
			To:   []string{"a@b.com"},
			SMTP: SMTPConfig{Host: ""},
		},
	}
	err := SendEmailWithConfig(context.Background(), config, "subj", "body")
	assert.NoError(t, err)
	assert.False(t, called, "smtpSendMail should not be called when host is empty")
}

func TestSendEmailWithConfig_SkipExampleHost(t *testing.T) {
	called := false
	orig := smtpSendMail
	t.Cleanup(func() { smtpSendMail = orig })
	smtpSendMail = func(addr string, a smtp.Auth, from string, to []string, msg []byte) error {
		called = true
		return nil
	}

	config := &NotificationChannelConfig{
		Email: EmailConfig{
			To:   []string{"a@b.com"},
			SMTP: SMTPConfig{Host: "smtp.example.com"},
		},
	}
	err := SendEmailWithConfig(context.Background(), config, "subj", "body")
	assert.NoError(t, err)
	assert.False(t, called, "smtpSendMail should not be called for smtp.example.com")
}

func TestSendEmailWithConfig_SendsWithCredentials(t *testing.T) {
	type capturedArgs struct {
		addr string
		auth smtp.Auth
		from string
		to   []string
		msg  []byte
	}
	var captured capturedArgs

	orig := smtpSendMail
	t.Cleanup(func() { smtpSendMail = orig })
	smtpSendMail = func(addr string, a smtp.Auth, from string, to []string, msg []byte) error {
		captured = capturedArgs{addr: addr, auth: a, from: from, to: to, msg: msg}
		return nil
	}

	config := &NotificationChannelConfig{
		Email: EmailConfig{
			To: []string{"alice@example.com", "bob@example.com"},
			SMTP: SMTPConfig{
				Host:     "mail.example.com",
				Port:     587,
				From:     "noreply@example.com",
				Username: "user",
				Password: "pass",
			},
		},
	}
	err := SendEmailWithConfig(context.Background(), config, "Test Subject", "Test Body")
	require.NoError(t, err)

	assert.Equal(t, "mail.example.com:587", captured.addr)
	assert.NotNil(t, captured.auth, "auth should be set when credentials are provided")
	assert.Equal(t, "noreply@example.com", captured.from)
	assert.Equal(t, []string{"alice@example.com", "bob@example.com"}, captured.to)
	msgStr := string(captured.msg)
	assert.Contains(t, msgStr, "From: noreply@example.com")
	assert.Contains(t, msgStr, "Subject: Test Subject")
	assert.Contains(t, msgStr, "Test Body")
}

func TestSendEmailWithConfig_SendsWithoutCredentials(t *testing.T) {
	var capturedAuth smtp.Auth

	orig := smtpSendMail
	t.Cleanup(func() { smtpSendMail = orig })
	smtpSendMail = func(addr string, a smtp.Auth, from string, to []string, msg []byte) error {
		capturedAuth = a
		return nil
	}

	config := &NotificationChannelConfig{
		Email: EmailConfig{
			To: []string{"alice@example.com"},
			SMTP: SMTPConfig{
				Host: "mail.example.com",
				Port: 587,
				From: "noreply@example.com",
				// No Username or Password
			},
		},
	}
	err := SendEmailWithConfig(context.Background(), config, "subj", "body")
	require.NoError(t, err)
	assert.Nil(t, capturedAuth, "auth should be nil when no credentials provided")
}

func TestSendEmailWithConfig_SMTPError(t *testing.T) {
	orig := smtpSendMail
	t.Cleanup(func() { smtpSendMail = orig })
	smtpSendMail = func(addr string, a smtp.Auth, from string, to []string, msg []byte) error {
		return fmt.Errorf("connection refused")
	}

	config := &NotificationChannelConfig{
		Email: EmailConfig{
			To: []string{"alice@example.com"},
			SMTP: SMTPConfig{
				Host: "mail.example.com",
				Port: 587,
				From: "noreply@example.com",
			},
		},
	}
	err := SendEmailWithConfig(context.Background(), config, "subj", "body")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "connection refused")
}

func TestSendEmailWithConfig_MessageFormat(t *testing.T) {
	var capturedMsg []byte

	orig := smtpSendMail
	t.Cleanup(func() { smtpSendMail = orig })
	smtpSendMail = func(addr string, a smtp.Auth, from string, to []string, msg []byte) error {
		capturedMsg = msg
		return nil
	}

	config := &NotificationChannelConfig{
		Email: EmailConfig{
			To: []string{"alice@example.com"},
			SMTP: SMTPConfig{
				Host: "mail.example.com",
				Port: 587,
				From: "sender@example.com",
			},
		},
	}
	err := SendEmailWithConfig(context.Background(), config, "My Subject", "My Body")
	require.NoError(t, err)

	msg := string(capturedMsg)
	assert.True(t, strings.Contains(msg, "From: sender@example.com\r\n"))
	assert.True(t, strings.Contains(msg, "To: alice@example.com\r\n"))
	assert.True(t, strings.Contains(msg, "Subject: My Subject\r\n"))
	assert.True(t, strings.Contains(msg, "My Body"))
}
