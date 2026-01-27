// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package notifications

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/openchoreo/openchoreo/internal/observer/types"
	"github.com/openchoreo/openchoreo/internal/template"
)

// SendAlertNotification sends an alert notification via the configured notification channel.
// It handles template rendering for payloads internally.
func SendAlertNotification(ctx context.Context, config *NotificationChannelConfig, alertDetails *types.AlertDetails, logger *slog.Logger) error {
	switch config.Type {
	case "webhook":
		// Prepare webhook payload
		payload, err := prepareWebhookPayload(config.Webhook.PayloadTemplate, alertDetails, logger)
		if err != nil {
			return fmt.Errorf("failed to prepare webhook payload: %w", err)
		}

		// Send the webhook with prepared payload
		if err := SendWebhookWithConfig(ctx, &config.Webhook, payload); err != nil {
			logger.Error("Failed to send alert notification webhook",
				"error", err,
				"webhookURL", config.Webhook.URL,
				"payload", payload)
			return fmt.Errorf("failed to send alert notification webhook: %w", err)
		}

		logger.Debug("Alert notification sent successfully via webhook",
			"alertName", alertDetails.AlertName,
			"webhookURL", config.Webhook.URL,
			"usedTemplate", config.Webhook.PayloadTemplate != "")
		return nil

	case "email":
		// Prepare email content
		subject, body, err := prepareEmailContent(config.Email, alertDetails, logger)
		if err != nil {
			return fmt.Errorf("failed to prepare email content: %w", err)
		}

		// Send the notification using the fetched config
		if err := SendEmailWithConfig(ctx, config, subject, body); err != nil {
			logger.Error("Failed to send alert notification email",
				"error", err,
				"recipients", config.Email.To)
			return fmt.Errorf("failed to send alert notification email: %w", err)
		}

		logger.Debug("Alert notification sent successfully",
			"alertName", alertDetails.AlertName,
			"recipients count", len(config.Email.To))
		return nil

	default:
		return fmt.Errorf("unsupported notification channel type: %s", config.Type)
	}
}

// prepareWebhookPayload prepares the webhook payload by rendering the template if provided
func prepareWebhookPayload(templateStr string, alertDetails *types.AlertDetails, logger *slog.Logger) (map[string]interface{}, error) {
	// Compute CEL inputs once for this notification flow
	celInputs := alertDetails.ToMap()

	if templateStr == "" {
		// No template provided, return the converted alert details map
		return celInputs, nil
	}

	// Unmarshal the payload template string to JSON
	var payloadTemplate map[string]interface{}
	if err := json.Unmarshal([]byte(templateStr), &payloadTemplate); err != nil {
		logger.Error("Failed to unmarshal webhook payload template to JSON",
			"error", err,
			"payloadTemplate", templateStr)
		return nil, fmt.Errorf("failed to unmarshal webhook payload template to JSON: %w", err)
	}

	// Render the JSON template using CEL expressions with precomputed inputs
	renderedTemplateMap, err := RenderJSONTemplate(payloadTemplate, celInputs, logger)
	if err != nil {
		logger.Warn("Failed to render webhook payload template, using unrendered template",
			"error", err,
			"payloadTemplate", templateStr)
		// Fallback to unrendered template
		return payloadTemplate, nil
	}

	logger.Debug("Webhook payload template rendered",
		"payload", renderedTemplateMap)
	return renderedTemplateMap, nil
}

// prepareEmailContent prepares the email subject and body by rendering templates if provided
func prepareEmailContent(emailConfig EmailConfig, alertDetails *types.AlertDetails, logger *slog.Logger) (string, string, error) {
	// Compute CEL inputs once for this notification flow (reused for both subject and body templates)
	celInputs := alertDetails.ToMap()

	// Render the incoming alert payload for human-friendly notifications
	payload, err := json.MarshalIndent(alertDetails, "", "  ")
	if err != nil {
		return "", "", fmt.Errorf("failed to marshal alert payload: %w", err)
	}

	// Build subject using template if available, otherwise use default
	subject := fmt.Sprintf("OpenChoreo alert triggered: %s", alertDetails.AlertName)
	if emailConfig.SubjectTemplate != "" {
		subject = RenderPlaintextTemplate(emailConfig.SubjectTemplate, celInputs, logger)
	}

	// Build body using template if available, otherwise use default
	emailBody := fmt.Sprintf("An alert was triggered at %s UTC.\n\nPayload:\n%s\n", time.Now().UTC().Format(time.RFC3339), string(payload))
	if emailConfig.BodyTemplate != "" {
		emailBody = RenderPlaintextTemplate(emailConfig.BodyTemplate, celInputs, logger)
	}

	return subject, emailBody, nil
}

// RenderJSONTemplate renders a JSON template with CEL expressions for webhook payloads.
// It expects the template to be a parsed JSON map and returns the rendered map.
// If rendering fails, an error is returned.
// celInputs should be the precomputed map from AlertDetails.ToMap() for better performance.
func RenderJSONTemplate(templateData map[string]interface{}, celInputs map[string]interface{}, logger *slog.Logger) (map[string]interface{}, error) {
	logger.Debug("Rendering JSON template", "alertData", celInputs, "template", templateData)

	// Render template and data using the shared template engine
	engine := getTemplateEngine()
	renderedTemplate, err := engine.Render(templateData, celInputs)
	if err != nil {
		logger.Warn("Failed to render JSON template",
			"error", err,
			"template", templateData)
		return nil, fmt.Errorf("failed to render template: %w", err)
	}

	renderedTemplateMap, ok := renderedTemplate.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("rendered template is not a map, got %T", renderedTemplate)
	}

	return renderedTemplateMap, nil
}

// RenderPlaintextTemplate renders a plaintext template with CEL expressions for email subjects and bodies.
// It treats the template as a plain string and evaluates CEL expressions within it.
// If any CEL expression fails to resolve, a warning is logged and the original expression is preserved in the output.
// celInputs should be the precomputed map from AlertDetails.ToMap() for better performance.
func RenderPlaintextTemplate(templateStr string, celInputs map[string]interface{}, logger *slog.Logger) string {
	logger.Debug("Rendering plaintext template", "alertData", celInputs)

	// Render the plaintext template as a string with CEL expressions using the shared template engine
	engine := getTemplateEngine()
	rendered, err := engine.Render(templateStr, celInputs)
	if err != nil {
		logger.Warn("Failed to render plaintext template, returning original template",
			"error", err,
			"template", templateStr)
		return templateStr
	}

	// Convert rendered result to string
	if renderedStr, ok := rendered.(string); ok {
		return renderedStr
	}
	return fmt.Sprintf("%v", rendered)
}

var (
	templateEngineOnce sync.Once
	templateEngine     *template.Engine
)

// getTemplateEngine returns a shared template engine instance for CEL evaluation.
// Internally caches CEL environments and compiled programs for better performance.
func getTemplateEngine() *template.Engine {
	templateEngineOnce.Do(func() {
		templateEngine = template.NewEngine()
	})
	return templateEngine
}
