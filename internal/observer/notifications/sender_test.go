// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package notifications

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/smtp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openchoreo/openchoreo/internal/observer/types"
)

func newTestAlertDetails() *types.AlertDetails {
	return &types.AlertDetails{
		AlertName:        "HighCPU",
		AlertSeverity:    "critical",
		AlertDescription: "CPU usage exceeded threshold",
		AlertThreshold:   "90",
		AlertValue:       "95",
		AlertType:        "metric",
		Component:        "checkout",
		Project:          "ecommerce",
		Environment:      "production",
	}
}

// --- RenderPlaintextTemplate ---

func TestRenderPlaintextTemplate(t *testing.T) {
	logger := discardLogger()

	tests := []struct {
		name     string
		template string
		inputs   map[string]interface{}
		want     string
	}{
		{
			name:     "empty template returns empty string",
			template: "",
			inputs:   map[string]interface{}{},
			want:     "",
		},
		{
			name:     "template with no CEL expressions returns original",
			template: "No expressions here",
			inputs:   map[string]interface{}{},
			want:     "No expressions here",
		},
		{
			name:     "single CEL expression interpolated",
			template: "Alert: ${alertName}",
			inputs:   map[string]interface{}{"alertName": "HighCPU"},
			want:     "Alert: HighCPU",
		},
		{
			name:     "multiple CEL expressions in one template",
			template: "${alertName} severity=${alertSeverity}",
			inputs:   map[string]interface{}{"alertName": "HighCPU", "alertSeverity": "critical"},
			want:     "HighCPU severity=critical",
		},
		{
			name:     "invalid CEL expression returns original template",
			template: "${nonexistent_var_xyz}",
			inputs:   map[string]interface{}{},
			want:     "${nonexistent_var_xyz}",
		},
		{
			name:     "CEL expression with AlertDetails inputs",
			template: "Alert ${alertName} in ${environment}",
			inputs:   newTestAlertDetails().ToMap(),
			want:     "Alert HighCPU in production",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := RenderPlaintextTemplate(tc.template, tc.inputs, logger)
			assert.Equal(t, tc.want, got)
		})
	}
}

// --- RenderJSONTemplate ---

func TestRenderJSONTemplate(t *testing.T) {
	logger := discardLogger()

	tests := []struct {
		name        string
		templateMap map[string]interface{}
		inputs      map[string]interface{}
		wantErr     bool
		check       func(t *testing.T, got map[string]interface{})
	}{
		{
			name:        "empty template map returns empty map",
			templateMap: map[string]interface{}{},
			inputs:      map[string]interface{}{},
			check: func(t *testing.T, got map[string]interface{}) {
				assert.Empty(t, got)
			},
		},
		{
			name:        "template with static value returns unchanged",
			templateMap: map[string]interface{}{"key": "staticValue"},
			inputs:      map[string]interface{}{},
			check: func(t *testing.T, got map[string]interface{}) {
				assert.Equal(t, "staticValue", got["key"])
			},
		},
		{
			name:        "CEL expression in value is evaluated",
			templateMap: map[string]interface{}{"alert": "${alertName}"},
			inputs:      map[string]interface{}{"alertName": "HighCPU"},
			check: func(t *testing.T, got map[string]interface{}) {
				assert.Equal(t, "HighCPU", got["alert"])
			},
		},
		{
			name:        "multiple CEL expressions evaluated",
			templateMap: map[string]interface{}{"name": "${alertName}", "severity": "${alertSeverity}"},
			inputs:      map[string]interface{}{"alertName": "HighCPU", "alertSeverity": "critical"},
			check: func(t *testing.T, got map[string]interface{}) {
				assert.Equal(t, "HighCPU", got["name"])
				assert.Equal(t, "critical", got["severity"])
			},
		},
		{
			name:        "invalid CEL expression returns error",
			templateMap: map[string]interface{}{"alert": "${undefined_var_that_does_not_exist}"},
			inputs:      map[string]interface{}{},
			wantErr:     true,
		},
		{
			name:        "CEL expression with AlertDetails inputs",
			templateMap: map[string]interface{}{"component": "${component}", "project": "${project}"},
			inputs:      newTestAlertDetails().ToMap(),
			check: func(t *testing.T, got map[string]interface{}) {
				assert.Equal(t, "checkout", got["component"])
				assert.Equal(t, "ecommerce", got["project"])
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := RenderJSONTemplate(tc.templateMap, tc.inputs, logger)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			if tc.check != nil {
				tc.check(t, got)
			}
		})
	}
}

// --- prepareWebhookPayload ---

func TestPrepareWebhookPayload(t *testing.T) {
	logger := discardLogger()
	alert := newTestAlertDetails()

	tests := []struct {
		name        string
		templateStr string
		wantErr     bool
		check       func(t *testing.T, got map[string]interface{})
	}{
		{
			name:        "empty template returns AlertDetails map directly",
			templateStr: "",
			check: func(t *testing.T, got map[string]interface{}) {
				assert.Equal(t, "HighCPU", got["alertName"])
				assert.Equal(t, "critical", got["alertSeverity"])
			},
		},
		{
			name:        "valid JSON template with CEL expression is rendered",
			templateStr: `{"alertTitle": "${alertName}", "env": "${environment}"}`,
			check: func(t *testing.T, got map[string]interface{}) {
				assert.Equal(t, "HighCPU", got["alertTitle"])
				assert.Equal(t, "production", got["env"])
			},
		},
		{
			name:        "invalid JSON template string returns error",
			templateStr: `{not valid json`,
			wantErr:     true,
		},
		{
			name:        "valid JSON template with unresolvable CEL falls back to unrendered template",
			templateStr: `{"key": "${undefined_variable_xyz}"}`,
			check: func(t *testing.T, got map[string]interface{}) {
				// Fallback: returns the unrendered template map
				assert.Equal(t, "${undefined_variable_xyz}", got["key"])
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := prepareWebhookPayload(tc.templateStr, alert, logger)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			if tc.check != nil {
				tc.check(t, got)
			}
		})
	}
}

// --- prepareEmailContent ---

func TestPrepareEmailContent(t *testing.T) {
	logger := discardLogger()
	alert := newTestAlertDetails()

	tests := []struct {
		name        string
		emailConfig EmailConfig
		check       func(t *testing.T, subject, body string)
	}{
		{
			name:        "default subject when no subject template",
			emailConfig: EmailConfig{},
			check: func(t *testing.T, subject, body string) {
				assert.Equal(t, "OpenChoreo alert triggered: HighCPU", subject)
			},
		},
		{
			name:        "custom subject rendered from CEL template",
			emailConfig: EmailConfig{SubjectTemplate: "ALERT: ${alertName} [${alertSeverity}]"},
			check: func(t *testing.T, subject, body string) {
				assert.Equal(t, "ALERT: HighCPU [critical]", subject)
			},
		},
		{
			name:        "default body contains timestamp and payload",
			emailConfig: EmailConfig{},
			check: func(t *testing.T, subject, body string) {
				assert.Contains(t, body, "An alert was triggered at")
				assert.Contains(t, body, "Payload:")
				assert.Contains(t, body, "HighCPU") // alert name in JSON payload
			},
		},
		{
			name:        "custom body rendered from CEL template",
			emailConfig: EmailConfig{BodyTemplate: "Alert ${alertName} in ${environment} is ${alertSeverity}"},
			check: func(t *testing.T, subject, body string) {
				assert.Equal(t, "Alert HighCPU in production is critical", body)
			},
		},
		{
			name: "both subject and body use templates",
			emailConfig: EmailConfig{
				SubjectTemplate: "${alertName}",
				BodyTemplate:    "${environment}",
			},
			check: func(t *testing.T, subject, body string) {
				assert.Equal(t, "HighCPU", subject)
				assert.Equal(t, "production", body)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			subject, body, err := prepareEmailContent(tc.emailConfig, alert, logger)
			require.NoError(t, err)
			if tc.check != nil {
				tc.check(t, subject, body)
			}
		})
	}
}

// --- SendAlertNotification ---

func TestSendAlertNotification_UnsupportedType(t *testing.T) {
	config := &NotificationChannelConfig{Type: "slack"}
	alert := newTestAlertDetails()
	err := SendAlertNotification(context.Background(), config, alert, discardLogger())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported notification channel type: slack")
}

func TestSendAlertNotification_WebhookRoute(t *testing.T) {
	t.Run("sends AlertDetails as JSON payload when no template", func(t *testing.T) {
		var capturedBody []byte
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var err error
			capturedBody, err = io.ReadAll(r.Body)
			require.NoError(t, err)
			w.WriteHeader(http.StatusOK)
		}))
		defer ts.Close()

		origClient := httpClient
		t.Cleanup(func() { httpClient = origClient })
		httpClient = ts.Client()

		config := &NotificationChannelConfig{
			Type:    "webhook",
			Webhook: WebhookConfig{URL: ts.URL},
		}
		alert := newTestAlertDetails()
		err := SendAlertNotification(context.Background(), config, alert, discardLogger())
		require.NoError(t, err)

		var got map[string]interface{}
		require.NoError(t, json.Unmarshal(capturedBody, &got))
		assert.Equal(t, "HighCPU", got["alertName"])
	})

	t.Run("uses PayloadTemplate when provided", func(t *testing.T) {
		var capturedBody []byte
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var err error
			capturedBody, err = io.ReadAll(r.Body)
			require.NoError(t, err)
			w.WriteHeader(http.StatusOK)
		}))
		defer ts.Close()

		origClient := httpClient
		t.Cleanup(func() { httpClient = origClient })
		httpClient = ts.Client()

		config := &NotificationChannelConfig{
			Type: "webhook",
			Webhook: WebhookConfig{
				URL:             ts.URL,
				PayloadTemplate: `{"title": "${alertName}", "level": "${alertSeverity}"}`,
			},
		}
		alert := newTestAlertDetails()
		err := SendAlertNotification(context.Background(), config, alert, discardLogger())
		require.NoError(t, err)

		var got map[string]interface{}
		require.NoError(t, json.Unmarshal(capturedBody, &got))
		assert.Equal(t, "HighCPU", got["title"])
		assert.Equal(t, "critical", got["level"])
	})

	t.Run("propagates webhook error", func(t *testing.T) {
		origClient := httpClient
		t.Cleanup(func() { httpClient = origClient })
		// Use an unreachable URL
		config := &NotificationChannelConfig{
			Type:    "webhook",
			Webhook: WebhookConfig{URL: "http://127.0.0.1:1/hook"},
		}
		alert := newTestAlertDetails()
		err := SendAlertNotification(context.Background(), config, alert, discardLogger())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to send alert notification webhook")
	})
}

func TestSendAlertNotification_EmailRoute(t *testing.T) {
	t.Run("sends email with default subject and body", func(t *testing.T) {
		var capturedSubject, capturedBody string

		orig := smtpSendMail
		t.Cleanup(func() { smtpSendMail = orig })
		smtpSendMail = func(addr string, a smtp.Auth, from string, to []string, msg []byte) error {
			msgStr := string(msg)
			for _, line := range strings.Split(msgStr, "\r\n") {
				if strings.HasPrefix(line, "Subject: ") {
					capturedSubject = strings.TrimPrefix(line, "Subject: ")
				}
			}
			// Body is after the blank line
			parts := strings.SplitN(msgStr, "\r\n\r\n", 2)
			if len(parts) == 2 {
				capturedBody = parts[1]
			}
			return nil
		}

		config := &NotificationChannelConfig{
			Type: "email",
			Email: EmailConfig{
				To:   []string{"alice@example.com"},
				SMTP: SMTPConfig{Host: "mail.example.com", Port: 587},
			},
		}
		alert := newTestAlertDetails()
		err := SendAlertNotification(context.Background(), config, alert, discardLogger())
		require.NoError(t, err)
		assert.Equal(t, "OpenChoreo alert triggered: HighCPU", capturedSubject)
		assert.Contains(t, capturedBody, "An alert was triggered at")
	})

	t.Run("uses subject and body templates when provided", func(t *testing.T) {
		var capturedSubject, capturedBody string

		orig := smtpSendMail
		t.Cleanup(func() { smtpSendMail = orig })
		smtpSendMail = func(addr string, a smtp.Auth, from string, to []string, msg []byte) error {
			msgStr := string(msg)
			for _, line := range strings.Split(msgStr, "\r\n") {
				if strings.HasPrefix(line, "Subject: ") {
					capturedSubject = strings.TrimPrefix(line, "Subject: ")
				}
			}
			parts := strings.SplitN(msgStr, "\r\n\r\n", 2)
			if len(parts) == 2 {
				capturedBody = parts[1]
			}
			return nil
		}

		config := &NotificationChannelConfig{
			Type: "email",
			Email: EmailConfig{
				To:              []string{"alice@example.com"},
				SMTP:            SMTPConfig{Host: "mail.example.com", Port: 587},
				SubjectTemplate: "Alert: ${alertName}",
				BodyTemplate:    "Severity: ${alertSeverity}",
			},
		}
		alert := newTestAlertDetails()
		err := SendAlertNotification(context.Background(), config, alert, discardLogger())
		require.NoError(t, err)
		assert.Equal(t, "Alert: HighCPU", capturedSubject)
		assert.Equal(t, "Severity: critical", capturedBody)
	})

	t.Run("propagates email error", func(t *testing.T) {
		orig := smtpSendMail
		t.Cleanup(func() { smtpSendMail = orig })
		smtpSendMail = func(addr string, a smtp.Auth, from string, to []string, msg []byte) error {
			return assert.AnError
		}

		config := &NotificationChannelConfig{
			Type: "email",
			Email: EmailConfig{
				To:   []string{"alice@example.com"},
				SMTP: SMTPConfig{Host: "mail.example.com", Port: 587},
			},
		}
		alert := newTestAlertDetails()
		err := SendAlertNotification(context.Background(), config, alert, discardLogger())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to send alert notification email")
	})
}
