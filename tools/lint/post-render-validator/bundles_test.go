// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package postrendervalidator

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

func workloadWithEndpointYAML(project, component, image, endpointType string) string {
	return fmt.Sprintf(`apiVersion: openchoreo.dev/v1alpha1
kind: Workload
metadata:
  name: %s-workload
  namespace: default
spec:
  owner:
    projectName: %s
    componentName: %s
  container:
    image: %s
  endpoints:
    app:
      type: %s
      port: 8080
`, component, project, component, image, endpointType)
}

func componentTypeWithValidation(kind, name, rule, message string) string {
	base := componentTypeYAML(kind, name)
	return strings.Replace(base,
		"spec:\n  workloadType: deployment\n",
		fmt.Sprintf("spec:\n  workloadType: deployment\n  validations:\n    - rule: %q\n      message: %q\n", rule, message),
		1,
	)
}

func TestValidateFiles_GroupsComponentsIndependently(t *testing.T) {
	validation := `${workload.endpoints.exists(name, endpoint, endpoint.type == 'HTTP')}`
	_, files := writeProject(t, map[string]string{
		"webapp-ct.yaml":   componentTypeWithValidation("ComponentType", "web-application", validation, "webapp needs HTTP"),
		"database-ct.yaml": componentTypeYAML("ComponentType", "database"),
		"frontend.yaml":    componentYAML("frontend", "default", "ComponentType", "deployment/web-application"),
		"postgres.yaml":    componentYAML("postgres", "default", "ComponentType", "deployment/database"),
		"frontend-wl.yaml": workloadWithEndpointYAML("default", "frontend", "nginx:1.25", "HTTP"),
		"postgres-wl.yaml": workloadWithEndpointYAML("default", "postgres", "postgres:16", "TCP"),
	})

	findings := ValidateFiles(context.Background(), files)
	if len(findings) != 0 {
		t.Fatalf("components should be validated with their own ComponentTypes, got %+v", findings)
	}

}

func TestValidateFiles_AttributesDeclaredFailureToOwningComponentType(t *testing.T) {
	validation := `${workload.endpoints.exists(name, endpoint, endpoint.type == 'HTTP')}`
	_, files := writeProject(t, map[string]string{
		"webapp-ct.yaml":   componentTypeWithValidation("ComponentType", "web-application", validation, "webapp needs HTTP"),
		"database-ct.yaml": componentTypeYAML("ComponentType", "database"),
		"frontend.yaml":    componentYAML("frontend", "default", "ComponentType", "deployment/web-application"),
		"postgres.yaml":    componentYAML("postgres", "default", "ComponentType", "deployment/database"),
		"frontend-wl.yaml": workloadWithEndpointYAML("default", "frontend", "nginx:1.25", "TCP"),
		"postgres-wl.yaml": workloadWithEndpointYAML("default", "postgres", "postgres:16", "TCP"),
	})

	findings := ValidateFiles(context.Background(), files)
	var failure *Finding
	for i := range findings {
		if findings[i].Code == "post-render-error" && strings.Contains(findings[i].Message, "webapp needs HTTP") {
			failure = &findings[i]
			break
		}
	}
	if failure == nil {
		t.Fatalf("expected webapp validation failure, got %+v", findings)
	}
	if !strings.Contains(failure.Path, "webapp-ct.yaml") {
		t.Fatalf("failure attributed to %s, want webapp-ct.yaml", failure.Path)
	}
	if strings.Contains(failure.Path, "database-ct.yaml") {
		t.Fatalf("database ComponentType received the webapp failure: %+v", failure)
	}
}

func TestValidateFiles_ClusterDataPlaneBundle(t *testing.T) {
	_, files := writeProject(t, map[string]string{
		"frontend-ct.yaml": componentTypeYAML("ComponentType", "frontend-ct"),
		"frontend.yaml":    componentYAML("frontend", "default", "ComponentType", "deployment/frontend-ct"),
		"frontend-wl.yaml": workloadWithEndpointYAML("default", "frontend", "nginx:1.25", "HTTP"),
		"frontend-rb.yaml": releaseBindingYAML("frontend-dev", "default", "frontend", "dev"),
		"dev.yaml":         environmentYAML("dev", "ClusterDataPlane", "shared"),
		"cluster-dp.yaml":  dataPlaneYAML("ClusterDataPlane", "shared"),
	})

	findings := ValidateFiles(context.Background(), files)
	if hasFinding(findings, "post-render-error") {
		t.Fatalf("ClusterDataPlane should be converted for rendering, got %+v", findings)
	}
}

func TestValidateFiles_ReportsMissingDataPlaneOnce(t *testing.T) {
	_, files := writeProject(t, map[string]string{
		"frontend-ct.yaml": componentTypeYAML("ComponentType", "frontend-ct"),
		"backend-ct.yaml":  componentTypeYAML("ComponentType", "backend-ct"),
		"frontend.yaml":    componentYAML("frontend", "default", "ComponentType", "deployment/frontend-ct"),
		"backend.yaml":     componentYAML("backend", "default", "ComponentType", "deployment/backend-ct"),
		"frontend-wl.yaml": workloadYAML("default", "frontend", "nginx:1.25"),
		"backend-wl.yaml":  workloadYAML("default", "backend", "nginx:1.25"),
		"frontend-rb.yaml": releaseBindingYAML("frontend-dev", "default", "frontend", "dev"),
		"backend-rb.yaml":  releaseBindingYAML("backend-dev", "default", "backend", "dev"),
		"dev.yaml":         environmentYAML("dev", "ClusterDataPlane", "shared"),
	})

	findings := ValidateFiles(context.Background(), files)
	var errors []Finding
	for _, finding := range findings {
		if finding.Code == "post-render-error" {
			errors = append(errors, finding)
		}
	}
	if len(errors) != 1 {
		t.Fatalf("expected one deduplicated missing DataPlane error, got %+v", findings)
	}
	if !strings.Contains(errors[0].Path, "dev.yaml") {
		t.Fatalf("missing DataPlane attributed to %s, want dev.yaml", errors[0].Path)
	}
	if !strings.Contains(errors[0].Message, `Environment dev references ClusterDataPlane "shared", but it was not found`) {
		t.Fatalf("unexpected message: %s", errors[0].Message)
	}
	if strings.Contains(errors[0].Message, "CEL") || strings.Contains(errors[0].Message, "no such key") {
		t.Fatalf("missing DataPlane should not surface CEL errors: %s", errors[0].Message)
	}
	if !strings.Contains(errors[0].Fix, "ClusterDataPlane") {
		t.Fatalf("missing DataPlane should include fix guidance, got %q", errors[0].Fix)
	}
	if errors[0].Line == 0 || errors[0].FixLine != errors[0].Line || !strings.Contains(errors[0].FixPath, "dev.yaml") {
		t.Fatalf("fix should point at the Environment dataPlaneRef, got %+v", errors[0])
	}
}

func TestValidateFiles_ImageLatestNamesComponent(t *testing.T) {
	_, files := writeProject(t, map[string]string{
		"collab-ct.yaml": componentTypeYAML("ComponentType", "collab-ct"),
		"collab.yaml":    componentYAML("collab-svc", "default", "ComponentType", "deployment/collab-ct"),
		"collab-wl.yaml": workloadYAML("default", "collab-svc", "temp-image-placeholder"),
	})

	findings := ValidateFiles(context.Background(), files)
	finding := findFinding(findings, "image-latest")
	if finding == nil {
		t.Fatalf("expected image-latest, got %+v", findings)
	}
	if !strings.Contains(finding.Message, "Deployment/collab-svc") {
		t.Fatalf("image-latest message should name the component, got %q", finding.Message)
	}
	if !strings.Contains(finding.Fix, "specify an image tag") || finding.FixLine == 0 {
		t.Fatalf("image-latest should include fix guidance and a location, got %+v", finding)
	}
}
