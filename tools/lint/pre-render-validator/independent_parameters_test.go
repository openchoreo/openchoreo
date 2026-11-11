// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0
package schemavalidator

import (
	"context"
	"strings"
	"testing"
)

func TestParameterChecksIgnoreUnrelatedComponentErrors(t *testing.T) {
	const source = `apiVersion: openchoreo.dev/v1alpha1
kind: ComponentType
metadata: {name: web, namespace: default}
spec:
  workloadType: deployment
  parameters:
    openAPIV3Schema:
      type: object
      additionalProperties: false
      required: [serviceName]
      properties:
        serviceName: {type: string}
        tuning:
          type: object
          properties:
            timeoutSeconds: {type: integer}
        allowedOrigins: {type: array, items: {type: string}}
---
apiVersion: openchoreo.dev/v1alpha1
kind: Trait
metadata: {name: storage, namespace: default}
spec:
  parameters:
    openAPIV3Schema:
      type: object
      properties:
        size: {type: integer}
---
apiVersion: openchoreo.dev/v1alpha1
kind: Component
metadata: {name: app, namespace: default}
spec:
  autoDeploy: true
  componentType: {kind: ComponentType, name: deployment/web}
  parameters:
    tuning: {timeoutSeconds: slow}
    allowedOrigins: [42]
    debugFlag: true
  traits:
    - name: storage
      parameters: {size: large}
`
	for _, replacement := range []string{"autoDeploy: true", `autoDeploy: "Yes"`, "autoDeploy: [wrong]", "autoDeploy: true\n  workflow: 42"} {
		t.Run(replacement, func(t *testing.T) {
			findings := ValidateDocuments(context.Background(), parseResources(t, strings.Replace(source, "autoDeploy: true", replacement, 1)))
			messages := ""
			count := 0
			for _, f := range findings {
				if f.Code == "schema-parameters-mismatch" {
					count++
					messages += f.Message + "\n"
					if f.Line < 34 {
						t.Fatalf("incorrect instance location: %+v", f)
					}
				}
			}
			if count != 5 {
				t.Fatalf("expected four Component errors and one Trait value error, got %d: %+v", count, findings)
			}
			for _, field := range []string{"serviceName", "timeoutSeconds", "allowedOrigins", "debugFlag", "size"} {
				if !strings.Contains(messages, field) {
					t.Fatalf("missing independent check for %s: %s", field, messages)
				}
			}
		})
	}
}
