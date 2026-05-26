// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package framework

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// WebhookReceiverApp is the label/app value of the in-cluster webhook
// receiver pod. Exposed so suites can target it with `kubectl exec`.
const WebhookReceiverApp = "webhook-receiver"

// DeployWebhookReceiver applies the receiver manifest into the given
// namespace and waits for the pod to be Ready. The receiver listens on
// `:8080` inside its pod; the in-cluster URL is
// `http://webhook-receiver.<namespace>.svc.cluster.local:8080`.
func DeployWebhookReceiver(kubeContext, namespace string) error {
	root, err := RepoRoot()
	if err != nil {
		return fmt.Errorf("locate repo root: %w", err)
	}
	manifestPath := filepath.Join(root,
		"test/e2e/fixtures/alerts/webhook-receiver/receiver.yaml")
	raw, err := os.ReadFile(manifestPath)
	if err != nil {
		return fmt.Errorf("read receiver manifest %s: %w", manifestPath, err)
	}
	// The manifest doesn't carry a Namespace document — create it first so
	// the apply works even on a freshly-installed cluster.
	nsYAML := fmt.Sprintf("apiVersion: v1\nkind: Namespace\nmetadata:\n  name: %s\n", namespace)
	if _, err := KubectlApplyLiteral(kubeContext, nsYAML); err != nil {
		return fmt.Errorf("create receiver namespace %q: %w", namespace, err)
	}
	rendered := strings.ReplaceAll(string(raw), "__NAMESPACE__", namespace)
	if _, err := KubectlApplyLiteral(kubeContext, rendered); err != nil {
		return fmt.Errorf("apply receiver manifest: %w", err)
	}
	if _, err := Kubectl(kubeContext,
		"-n", namespace,
		"rollout", "status", "deployment/"+WebhookReceiverApp, "--timeout=3m",
	); err != nil {
		return fmt.Errorf("receiver did not become ready: %w", err)
	}
	return nil
}

// WebhookReceiverURL returns the in-cluster URL alert notification channels
// should target.
func WebhookReceiverURL(namespace string) string {
	return fmt.Sprintf("http://%s.%s.svc.cluster.local:8080/notify",
		WebhookReceiverApp, namespace)
}

// ReceivedNotifications returns all webhook request bodies the receiver has
// observed since startup, oldest first. mendhak/http-https-echo emits one
// JSON line per request to stdout; this helper greps for those lines and
// extracts the `body` field. The framework returns the body as a string so
// callers can either treat it as opaque or unmarshal it themselves.
func ReceivedNotifications(kubeContext, namespace string) ([]string, error) {
	out, err := Kubectl(kubeContext,
		"logs", "-n", namespace, "-l", "app="+WebhookReceiverApp,
		"--tail=500", "--prefix=false",
	)
	if err != nil {
		return nil, fmt.Errorf("read receiver logs: %w (%s)", err, out)
	}
	var bodies []string
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || !strings.HasPrefix(line, "{") {
			continue
		}
		var rec struct {
			Method string `json:"method"`
			Path   string `json:"path"`
			Body   any    `json:"body"`
		}
		if jerr := json.Unmarshal([]byte(line), &rec); jerr != nil {
			// Skip non-JSON output (image banners, etc.). Don't fail —
			// downstream callers are interested in the JSON records only.
			continue
		}
		// Only count POST'd webhooks; readiness probes use GET /health and
		// are explicitly excluded by LOG_IGNORE_PATH in the manifest, but
		// be defensive.
		if rec.Method != "POST" {
			continue
		}
		switch b := rec.Body.(type) {
		case string:
			bodies = append(bodies, b)
		case nil:
			// no-op — the receiver may emit `body: null` for empty POSTs.
		default:
			// Body was decoded as a JSON object/array — re-marshal so the
			// caller sees the literal JSON payload that was POSTed.
			marshaled, jerr := json.Marshal(b)
			if jerr != nil {
				continue
			}
			bodies = append(bodies, string(marshaled))
		}
	}
	return bodies, nil
}
