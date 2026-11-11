// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package lsp_test

import (
	"encoding/json"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/openchoreo/openchoreo/tools/lsp"
	protocol "github.com/tliron/glsp/protocol_3_16"
)

const postRenderTestCommand = "openchoreo.runPostRenderValidation"

const postRenderComponentType = `apiVersion: openchoreo.dev/v1alpha1
kind: ComponentType
metadata:
  name: frontend-ct
  namespace: default
spec:
  workloadType: deployment
  parameters:
    openAPIV3Schema:
      type: object
      properties:
        replicas:
          type: integer
          default: 1
        port:
          type: integer
          default: 8080
  resources:
    - id: deployment
      template:
        apiVersion: apps/v1
        kind: Deployment
        metadata:
          name: ${metadata.name}
          namespace: ${metadata.namespace}
          labels: ${metadata.labels}
        spec:
          replicas: ${parameters.replicas}
          selector:
            matchLabels: ${metadata.podSelectors}
          template:
            metadata:
              labels: ${metadata.podSelectors}
            spec:
              containers:
                - name: app
                  image: ${workload.container.image}
                  ports:
                    - containerPort: ${parameters.port}
                      protocol: TCP
                  resources:
                    limits:
                      cpu: 100m
                      memory: 128Mi
                    requests:
                      cpu: 50m
                      memory: 64Mi
`

const postRenderComponent = `apiVersion: openchoreo.dev/v1alpha1
kind: Component
metadata:
  name: frontend
  namespace: default
spec:
  owner:
    projectName: default
  componentType:
    kind: ComponentType
    name: deployment/frontend-ct
  parameters:
    replicas: 1
    port: 8080
`

const postRenderWorkload = `apiVersion: openchoreo.dev/v1alpha1
kind: Workload
metadata:
  name: frontend-workload
  namespace: default
spec:
  owner:
    projectName: default
    componentName: frontend
  container:
    image: nginx
`

func postRenderProject(t *testing.T) (componentPath, workloadPath string) {
	t.Helper()
	dir := t.TempDir()
	write := func(name, content string) string {
		t.Helper()
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
		return path
	}
	write("ct.yaml", postRenderComponentType)
	componentPath = write("component.yaml", postRenderComponent)
	workloadPath = write("workload.yaml", postRenderWorkload)
	return componentPath, workloadPath
}

func fileURI(path string) string {
	return (&url.URL{Scheme: "file", Path: filepath.ToSlash(path)}).String()
}

type postRenderRun struct {
	summary map[string]any
	err     error
}

// startPostRender runs executeCommand in the background. The command publishes
// diagnostics before returning, and net.Pipe is unbuffered, so the caller must
// read the publish while the command is in flight.
func startPostRender(t *testing.T, s *lsp.Server, uri string) <-chan postRenderRun {
	t.Helper()
	done := make(chan postRenderRun, 1)
	go func() {
		result, err := s.ExecuteCommand(nil, &protocol.ExecuteCommandParams{
			Command:   postRenderTestCommand,
			Arguments: []any{uri},
		})
		if err != nil {
			done <- postRenderRun{err: err}
			return
		}
		raw, err := json.Marshal(result)
		if err != nil {
			done <- postRenderRun{err: err}
			return
		}
		var summary map[string]any
		if err := json.Unmarshal(raw, &summary); err != nil {
			done <- postRenderRun{err: err}
			return
		}
		done <- postRenderRun{summary: summary}
	}()
	return done
}

func waitPostRender(t *testing.T, done <-chan postRenderRun) map[string]any {
	t.Helper()
	run := <-done
	if run.err != nil {
		t.Fatalf("executeCommand error: %v", run.err)
	}
	return run.summary
}

func hasPostRenderDiagnostic(diags []protocol.Diagnostic, message string) bool {
	for _, diag := range diags {
		if diag.Source == nil || *diag.Source != "openchoreo-post-render" {
			continue
		}
		if strings.Contains(diag.Message, message) {
			return true
		}
	}
	return false
}

func TestPostRenderPublishesFindingsForClosedBundleFile(t *testing.T) {
	componentPath, workloadPath := postRenderProject(t)
	s, client := newDebouncedServer(t)
	componentURI := fileURI(componentPath)
	openDoc(t, s, componentURI, postRenderComponent, 1)

	done := startPostRender(t, s, componentURI)
	params := readPostRenderFinding(t, client, 2*time.Second)
	if string(params.URI) != fileURI(workloadPath) {
		t.Fatalf("diagnostics URI = %q, want workload %q", params.URI, fileURI(workloadPath))
	}
	if !hasPostRenderDiagnostic(params.Diagnostics, "specify an image tag") {
		t.Fatalf("expected image-latest on workload, got %+v", params.Diagnostics)
	}
	summary := waitPostRender(t, done)
	if summary["errors"].(float64) == 0 {
		t.Fatalf("expected post-render errors, got %v", summary)
	}
	details, _ := summary["details"].([]any)
	if len(details) == 0 {
		t.Fatalf("expected finding details, got %v", summary)
	}
	detail, _ := details[0].(string)
	if !strings.Contains(detail, "workload.yaml:11:") ||
		!strings.Contains(detail, "specify an image tag") {
		t.Fatalf("detail = %q, want file location and reason", detail)
	}
	if !strings.Contains(detail, "Fix at workload.yaml:11:5: specify an image tag") {
		t.Fatalf("detail = %q, want fix guidance at the finding location", detail)
	}
}

func TestPostRenderMergesWithOpenDocumentStructuralDiagnostics(t *testing.T) {
	componentPath, workloadPath := postRenderProject(t)
	s, client := newDebouncedServer(t)
	componentURI := fileURI(componentPath)
	workloadURI := fileURI(workloadPath)
	openDoc(t, s, workloadURI, postRenderWorkload, 7)

	done := startPostRender(t, s, componentURI)

	params := readPostRenderFinding(t, client, 2*time.Second)
	if string(params.URI) != workloadURI {
		t.Fatalf("diagnostics URI = %q, want %q", params.URI, workloadURI)
	}
	if params.Version == nil || *params.Version != 7 {
		t.Fatalf("version = %v, want 7", params.Version)
	}
	if !hasPostRenderDiagnostic(params.Diagnostics, "specify an image tag") {
		t.Fatalf("expected merged post-render finding, got %+v", params.Diagnostics)
	}
	waitPostRender(t, done)
}

func TestPostRenderSurvivesStructuralRepublish(t *testing.T) {
	componentPath, workloadPath := postRenderProject(t)
	s, client := newDebouncedServer(t)
	componentURI := fileURI(componentPath)
	workloadURI := fileURI(workloadPath)
	openDoc(t, s, workloadURI, postRenderWorkload, 1)

	done := startPostRender(t, s, componentURI)
	if params := readPostRenderFinding(t, client, 2*time.Second); !hasPostRenderDiagnostic(params.Diagnostics, "specify an image tag") {
		t.Fatalf("expected initial post-render finding, got %+v", params.Diagnostics)
	}
	waitPostRender(t, done)

	// Editing an unrelated file triggers a structural revalidation of every
	// other open document. The cached post-render finding must survive it.
	otherURI := fileURI(filepath.Join(t.TempDir(), "other.yaml"))
	openDoc(t, s, otherURI, "apiVersion: v1\nkind: ConfigMap\n", 1)
	changeDoc(t, s, otherURI, 2, protocol.TextDocumentContentChangeEventWhole{Text: "apiVersion: v1\nkind: Secret\n"})

	for i := 0; i < 4; i++ {
		params := readPostRenderFinding(t, client, 2*time.Second)
		if string(params.URI) != workloadURI {
			continue
		}
		if !hasPostRenderDiagnostic(params.Diagnostics, "specify an image tag") {
			t.Fatalf("structural republish erased post-render finding: %+v", params.Diagnostics)
		}
		return
	}
	t.Fatalf("no publish for %s", workloadURI)
}

func TestPostRenderClearsStaleFindingsOnEdit(t *testing.T) {
	componentPath, workloadPath := postRenderProject(t)
	s, client := newDebouncedServer(t)
	componentURI := fileURI(componentPath)
	workloadURI := fileURI(workloadPath)
	openDoc(t, s, componentURI, postRenderComponent, 1)
	openDoc(t, s, workloadURI, postRenderWorkload, 1)

	done := startPostRender(t, s, componentURI)
	// Debounced structural publishes may arrive before the explicit render.
	// Wait for the command's diagnostic rather than assuming frame ordering.
	deadline := time.Now().Add(3 * time.Second)
	for {
		params := readPublish(t, client, time.Until(deadline))
		if hasPostRenderDiagnostic(params.Diagnostics, "specify an image tag") {
			break
		}
	}

	waitPostRender(t, done)

	changeDoc(t, s, workloadURI, 2, protocol.TextDocumentContentChangeEventWhole{Text: postRenderWorkload})
	params := readPublish(t, client, 2*time.Second)
	if hasPostRenderDiagnostic(params.Diagnostics, "specify an image tag") {
		t.Fatalf("stale post-render finding survived edit: %+v", params.Diagnostics)
	}
}

func TestPostRenderReportsSelectionErrorOnActiveDocument(t *testing.T) {
	dir := t.TempDir()
	componentPath := filepath.Join(dir, "component.yaml")
	if err := os.WriteFile(componentPath, []byte(postRenderComponent), 0o644); err != nil {
		t.Fatalf("write component: %v", err)
	}
	s, client := newDebouncedServer(t)
	componentURI := fileURI(componentPath)
	openDoc(t, s, componentURI, postRenderComponent, 1)

	done := startPostRender(t, s, componentURI)
	params := readPostRenderFinding(t, client, 2*time.Second)
	if string(params.URI) != componentURI {
		t.Fatalf("diagnostics URI = %q, want active %q", params.URI, componentURI)
	}
	if !hasPostRenderDiagnostic(params.Diagnostics, "was not found") {
		t.Fatalf("expected post-render-error, got %+v", params.Diagnostics)
	}
	summary := waitPostRender(t, done)
	if summary["errors"].(float64) != 1 {
		t.Fatalf("expected one selection error, got %v", summary)
	}
	message, _ := summary["message"].(string)
	if !strings.Contains(message, "frontend-ct") {
		t.Fatalf("message = %q, want missing ComponentType detail", message)
	}
}

func TestPostRenderCodeLens(t *testing.T) {
	componentPath, _ := postRenderProject(t)
	s, _ := newDebouncedServer(t)
	componentURI := fileURI(componentPath)
	openDoc(t, s, componentURI, postRenderComponent, 1)

	lenses, err := s.CodeLens(nil, &protocol.CodeLensParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: protocol.DocumentUri(componentURI)},
	})
	if err != nil {
		t.Fatalf("codeLens error: %v", err)
	}
	if len(lenses) != 1 || lenses[0].Command == nil {
		t.Fatalf("expected one command lens, got %+v", lenses)
	}
	if lenses[0].Command.Command != postRenderTestCommand {
		t.Fatalf("command = %q, want %q", lenses[0].Command.Command, postRenderTestCommand)
	}
	if lenses[0].Command.Title != "Validate Project" {
		t.Fatalf("title = %q", lenses[0].Command.Title)
	}
	if len(lenses[0].Command.Arguments) != 1 || lenses[0].Command.Arguments[0] != componentURI {
		t.Fatalf("arguments = %+v, want [%s]", lenses[0].Command.Arguments, componentURI)
	}

	plainURI := fileURI(filepath.Join(t.TempDir(), "plain.yaml"))
	openDoc(t, s, plainURI, "apiVersion: v1\nkind: ConfigMap\n", 1)
	lenses, err = s.CodeLens(nil, &protocol.CodeLensParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: protocol.DocumentUri(plainURI)},
	})
	if err != nil {
		t.Fatalf("codeLens error: %v", err)
	}
	if len(lenses) != 0 {
		t.Fatalf("expected no lens for non-OpenChoreo YAML, got %+v", lenses)
	}
}

func TestPostRenderDiscoversGitOpsProjectResources(t *testing.T) {
	root := t.TempDir()
	write := func(relative, content string) string {
		t.Helper()
		path := filepath.Join(root, relative)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
		return path
	}
	write("platform/component-types/frontend-ct.yaml", postRenderComponentType)
	componentPath := write("projects/default/components/frontend/component.yaml", postRenderComponent)
	workloadPath := write("projects/default/components/frontend/workload.yaml", postRenderWorkload)

	s, client := newDebouncedServer(t)
	if _, err := s.Initialize(nil, &protocol.InitializeParams{WorkspaceFolders: []protocol.WorkspaceFolder{{URI: fileURI(root), Name: "project"}}}); err != nil {
		t.Fatal(err)
	}
	componentURI := fileURI(componentPath)
	openDoc(t, s, componentURI, postRenderComponent, 1)

	done := startPostRender(t, s, componentURI)
	params := readPostRenderFinding(t, client, 2*time.Second)
	if string(params.URI) != fileURI(workloadPath) {
		t.Fatalf("diagnostics URI = %q, want workload %q", params.URI, fileURI(workloadPath))
	}
	if !hasPostRenderDiagnostic(params.Diagnostics, "specify an image tag") {
		t.Fatalf("expected image-latest from platform ComponentType, got %+v", params.Diagnostics)
	}
	summary := waitPostRender(t, done)
	if summary["errors"].(float64) == 0 {
		t.Fatalf("expected post-render errors, got %v", summary)
	}
}

// Static workspace notifications may arrive before manual render findings.
func readPostRenderFinding(t *testing.T, client net.Conn, timeout time.Duration) protocol.PublishDiagnosticsParams {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		params := readPublish(t, client, time.Until(deadline))
		for _, diagnostic := range params.Diagnostics {
			if diagnostic.Source != nil && *diagnostic.Source == "openchoreo-post-render" {
				return params
			}
		}
	}
}
