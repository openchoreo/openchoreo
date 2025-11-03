// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package checkov

import (
	"context"
	"strings"
)

type MockScanner struct {
	Findings []Finding
	Err      error
}

func (m *MockScanner) Scan(ctx context.Context, manifest []byte) ([]Finding, error) {
	if m.Err != nil {
		return nil, m.Err
	}
	if m.Findings != nil {
		return m.Findings, nil
	}
	return generateMockFindings(manifest), nil
}

func generateMockFindings(manifest []byte) []Finding {
	manifestStr := string(manifest)
	findings := []Finding{}

	if strings.Contains(manifestStr, "privileged: true") {
		findings = append(findings, Finding{
			CheckID:     "CKV_K8S_16",
			CheckName:   "Container should not be privileged",
			Severity:    SeverityHigh,
			Category:    "security",
			Description: "Privileged containers have access to all Linux Kernel capabilities and devices.",
			Remediation: "Remove privileged: true or set to false",
		})
	}

	if strings.Contains(manifestStr, "runAsUser: 0") || strings.Contains(manifestStr, "runAsNonRoot: false") {
		findings = append(findings, Finding{
			CheckID:     "CKV_K8S_40",
			CheckName:   "Containers should run as a high UID",
			Severity:    SeverityMedium,
			Category:    "security",
			Description: "Force the container to run with user ID > 10000 to avoid conflicts with the host.",
			Remediation: "Set runAsUser to a value > 10000",
		})
	}

	if !strings.Contains(manifestStr, "resources:") || strings.Contains(manifestStr, "resources: {}") {
		findings = append(findings, Finding{
			CheckID:     "CKV_K8S_11",
			CheckName:   "CPU limits should be set",
			Severity:    SeverityLow,
			Category:    "resource-limits",
			Description: "Enforcing CPU limits prevents DOS via resource exhaustion.",
			Remediation: "Set resources.limits.cpu",
		})
		findings = append(findings, Finding{
			CheckID:     "CKV_K8S_12",
			CheckName:   "Memory limits should be set",
			Severity:    SeverityLow,
			Category:    "resource-limits",
			Description: "Enforcing memory limits prevents DOS via resource exhaustion.",
			Remediation: "Set resources.limits.memory",
		})
	}

	if !strings.Contains(manifestStr, "readOnlyRootFilesystem: true") {
		findings = append(findings, Finding{
			CheckID:     "CKV_K8S_22",
			CheckName:   "Use read-only filesystem for containers",
			Severity:    SeverityLow,
			Category:    "security",
			Description: "An immutable root filesystem can prevent malicious binaries from writing to the host system.",
			Remediation: "Set securityContext.readOnlyRootFilesystem to true",
		})
	}

	if !strings.Contains(manifestStr, "allowPrivilegeEscalation: false") {
		findings = append(findings, Finding{
			CheckID:     "CKV_K8S_20",
			CheckName:   "Containers should not allow privilege escalation",
			Severity:    SeverityMedium,
			Category:    "security",
			Description: "Do not allow privilege escalation to prevent gaining more privileges than the parent process.",
			Remediation: "Set securityContext.allowPrivilegeEscalation to false",
		})
	}

	if !strings.Contains(manifestStr, "livenessProbe:") {
		findings = append(findings, Finding{
			CheckID:     "CKV_K8S_8",
			CheckName:   "Liveness Probe Should be Configured",
			Severity:    SeverityLow,
			Category:    "reliability",
			Description: "Liveness probes allow Kubernetes to determine if a container is healthy.",
			Remediation: "Add a livenessProbe configuration",
		})
	}

	if !strings.Contains(manifestStr, "readinessProbe:") {
		findings = append(findings, Finding{
			CheckID:     "CKV_K8S_9",
			CheckName:   "Readiness Probe Should be Configured",
			Severity:    SeverityLow,
			Category:    "reliability",
			Description: "Readiness probes allow Kubernetes to determine if a pod is ready to serve requests.",
			Remediation: "Add a readinessProbe configuration",
		})
	}

	if strings.Contains(manifestStr, "image:") && strings.Contains(manifestStr, ":latest") {
		findings = append(findings, Finding{
			CheckID:     "CKV_K8S_14",
			CheckName:   "Image Tag should be fixed - not latest or blank",
			Severity:    SeverityMedium,
			Category:    "security",
			Description: "Using latest tag can lead to unpredictable behavior.",
			Remediation: "Use specific image tags instead of 'latest'",
		})
	}

	return findings
}

func SetMockScanner(scanner Scanner) {
	defaultScanner = scanner
}

func ResetScanner() {
	defaultScanner = &checkovScanner{}
}
