// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package legacyservices

import (
	"context"
	"encoding/json"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/controller"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/legacyservices/git"
)

func TestParseWorkflowParameterAnnotation(t *testing.T) {
	tests := []struct {
		name       string
		annotation string
		want       map[string]string
	}{
		{
			name:       "empty string",
			annotation: "",
			want:       map[string]string{},
		},
		{
			name:       "single key-value pair",
			annotation: "repoUrl: parameters.repository.url\n",
			want:       map[string]string{"repoUrl": "parameters.repository.url"},
		},
		{
			name:       "multiple key-value pairs",
			annotation: "repoUrl: parameters.repository.url\nbranch: parameters.repository.revision.branch\nappPath: parameters.appPath\n",
			want: map[string]string{
				"repoUrl": "parameters.repository.url",
				"branch":  "parameters.repository.revision.branch",
				"appPath": "parameters.appPath",
			},
		},
		{
			name:       "full annotation with all keys",
			annotation: "repoUrl: parameters.repository.url\nbranch: parameters.repository.revision.branch\ncommit: parameters.repository.revision.commit\nappPath: parameters.repository.appPath\nsecretRef: parameters.repository.secretRef\nprojectName: parameters.scope.projectName\ncomponentName: parameters.scope.componentName\n",
			want: map[string]string{
				"repoUrl":       "parameters.repository.url",
				"branch":        "parameters.repository.revision.branch",
				"commit":        "parameters.repository.revision.commit",
				"appPath":       "parameters.repository.appPath",
				"secretRef":     "parameters.repository.secretRef",
				"projectName":   "parameters.scope.projectName",
				"componentName": "parameters.scope.componentName",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := controller.ParseWorkflowParameterAnnotation(tt.annotation)
			if len(got) != len(tt.want) {
				t.Fatalf("got %d entries, want %d: %v", len(got), len(tt.want), got)
			}
			for k, v := range tt.want {
				if got[k] != v {
					t.Errorf("key %q: got %q, want %q", k, got[k], v)
				}
			}
		})
	}
}

func TestGetNestedStringFromRawExtension(t *testing.T) {
	makeRaw := func(v interface{}) *runtime.RawExtension {
		b, _ := json.Marshal(v)
		return &runtime.RawExtension{Raw: b}
	}

	tests := []struct {
		name       string
		raw        *runtime.RawExtension
		dottedPath string
		want       string
		wantErr    bool
	}{
		{
			name:       "nil RawExtension",
			raw:        nil,
			dottedPath: "repository.url",
			wantErr:    true,
		},
		{
			name:       "nil Raw bytes",
			raw:        &runtime.RawExtension{},
			dottedPath: "repository.url",
			wantErr:    true,
		},
		{
			name: "simple top-level key",
			raw: makeRaw(map[string]interface{}{
				"url": "https://github.com/example/repo",
			}),
			dottedPath: "url",
			want:       "https://github.com/example/repo",
		},
		{
			name: "nested path",
			raw: makeRaw(map[string]interface{}{
				"repository": map[string]interface{}{
					"url": "https://github.com/example/repo",
				},
			}),
			dottedPath: "repository.url",
			want:       "https://github.com/example/repo",
		},
		{
			name: "strips parameters prefix",
			raw: makeRaw(map[string]interface{}{
				"repository": map[string]interface{}{
					"url": "https://github.com/example/repo",
				},
			}),
			dottedPath: "parameters.repository.url",
			want:       "https://github.com/example/repo",
		},
		{
			name: "deeply nested path",
			raw: makeRaw(map[string]interface{}{
				"repository": map[string]interface{}{
					"revision": map[string]interface{}{
						"branch": "main",
					},
				},
			}),
			dottedPath: "parameters.repository.revision.branch",
			want:       "main",
		},
		{
			name: "key not found",
			raw: makeRaw(map[string]interface{}{
				"repository": map[string]interface{}{
					"url": "https://github.com/example/repo",
				},
			}),
			dottedPath: "repository.branch",
			wantErr:    true,
		},
		{
			name: "value is not a string",
			raw: makeRaw(map[string]interface{}{
				"repository": map[string]interface{}{
					"port": 8080,
				},
			}),
			dottedPath: "repository.port",
			wantErr:    true,
		},
		{
			name: "intermediate path is not an object",
			raw: makeRaw(map[string]interface{}{
				"repository": "not-an-object",
			}),
			dottedPath: "repository.url",
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := getNestedStringFromRawExtension(tt.raw, tt.dottedPath)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestExtractRepoInfoFromComponent(t *testing.T) {
	makeRaw := func(v interface{}) *runtime.RawExtension {
		b, _ := json.Marshal(v)
		return &runtime.RawExtension{Raw: b}
	}

	scheme := newTestScheme(t)

	tests := []struct {
		name        string
		component   *v1alpha1.Component
		workflow    *v1alpha1.Workflow
		wantRepo    string
		wantAppPath string
		wantBranch  string
		wantErr     bool
	}{
		{
			name: "no workflow config on component",
			component: &v1alpha1.Component{
				ObjectMeta: metav1.ObjectMeta{Name: "comp1", Namespace: "ns1"},
				Spec:       v1alpha1.ComponentSpec{},
			},
			wantErr: true,
		},
		{
			name: "empty workflow name",
			component: &v1alpha1.Component{
				ObjectMeta: metav1.ObjectMeta{Name: "comp1", Namespace: "ns1"},
				Spec: v1alpha1.ComponentSpec{
					Workflow: &v1alpha1.WorkflowRunConfig{Name: ""},
				},
			},
			wantErr: true,
		},
		{
			name: "workflow missing repoUrl in annotation",
			component: &v1alpha1.Component{
				ObjectMeta: metav1.ObjectMeta{Name: "comp1", Namespace: "ns1"},
				Spec: v1alpha1.ComponentSpec{
					Workflow: &v1alpha1.WorkflowRunConfig{
						Name: "wf1",
						Parameters: makeRaw(map[string]interface{}{
							"repository": map[string]interface{}{"url": "https://github.com/example/repo"},
						}),
					},
				},
			},
			workflow: &v1alpha1.Workflow{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "wf1",
					Namespace:   "ns1",
					Annotations: map[string]string{controller.AnnotationKeyComponentWorkflowParameters: "branch: parameters.branch\n"},
				},
			},
			wantErr: true,
		},
		{
			name: "extracts repoUrl successfully",
			component: &v1alpha1.Component{
				ObjectMeta: metav1.ObjectMeta{Name: "comp1", Namespace: "ns1"},
				Spec: v1alpha1.ComponentSpec{
					Workflow: &v1alpha1.WorkflowRunConfig{
						Name: "wf1",
						Parameters: makeRaw(map[string]interface{}{
							"repository": map[string]interface{}{
								"url": "https://github.com/example/repo",
							},
						}),
					},
				},
			},
			workflow: &v1alpha1.Workflow{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "wf1",
					Namespace: "ns1",
					Annotations: map[string]string{
						controller.AnnotationKeyComponentWorkflowParameters: "repoUrl: parameters.repository.url\n",
					},
				},
			},
			wantRepo: "https://github.com/example/repo",
		},
		{
			name: "extracts repoUrl and appPath",
			component: &v1alpha1.Component{
				ObjectMeta: metav1.ObjectMeta{Name: "comp1", Namespace: "ns1"},
				Spec: v1alpha1.ComponentSpec{
					Workflow: &v1alpha1.WorkflowRunConfig{
						Name: "wf1",
						Parameters: makeRaw(map[string]interface{}{
							"repository": map[string]interface{}{
								"url": "https://github.com/example/repo",
							},
							"appPath": "/src/app",
						}),
					},
				},
			},
			workflow: &v1alpha1.Workflow{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "wf1",
					Namespace: "ns1",
					Annotations: map[string]string{
						controller.AnnotationKeyComponentWorkflowParameters: "repoUrl: parameters.repository.url\nappPath: parameters.appPath\n",
					},
				},
			},
			wantRepo:    "https://github.com/example/repo",
			wantAppPath: "/src/app",
		},
		{
			name: "empty repoUrl value in parameters",
			component: &v1alpha1.Component{
				ObjectMeta: metav1.ObjectMeta{Name: "comp1", Namespace: "ns1"},
				Spec: v1alpha1.ComponentSpec{
					Workflow: &v1alpha1.WorkflowRunConfig{
						Name: "wf1",
						Parameters: makeRaw(map[string]interface{}{
							"repository": map[string]interface{}{
								"url": "",
							},
						}),
					},
				},
			},
			workflow: &v1alpha1.Workflow{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "wf1",
					Namespace: "ns1",
					Annotations: map[string]string{
						controller.AnnotationKeyComponentWorkflowParameters: "repoUrl: parameters.repository.url\n",
					},
				},
			},
			wantErr: true,
		},
		{
			name: "nil parameters RawExtension",
			component: &v1alpha1.Component{
				ObjectMeta: metav1.ObjectMeta{Name: "comp1", Namespace: "ns1"},
				Spec: v1alpha1.ComponentSpec{
					Workflow: &v1alpha1.WorkflowRunConfig{
						Name:       "wf1",
						Parameters: nil,
					},
				},
			},
			workflow: &v1alpha1.Workflow{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "wf1",
					Namespace: "ns1",
					Annotations: map[string]string{
						controller.AnnotationKeyComponentWorkflowParameters: "repoUrl: parameters.repository.url\n",
					},
				},
			},
			wantErr: true,
		},
		{
			name: "appPath missing from parameters is not an error",
			component: &v1alpha1.Component{
				ObjectMeta: metav1.ObjectMeta{Name: "comp1", Namespace: "ns1"},
				Spec: v1alpha1.ComponentSpec{
					Workflow: &v1alpha1.WorkflowRunConfig{
						Name: "wf1",
						Parameters: makeRaw(map[string]interface{}{
							"repository": map[string]interface{}{
								"url": "https://github.com/example/repo",
							},
						}),
					},
				},
			},
			workflow: &v1alpha1.Workflow{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "wf1",
					Namespace: "ns1",
					Annotations: map[string]string{
						controller.AnnotationKeyComponentWorkflowParameters: "repoUrl: parameters.repository.url\nappPath: parameters.appPath\n",
					},
				},
			},
			wantRepo:    "https://github.com/example/repo",
			wantAppPath: "",
		},
		{
			name: "branch path configured but missing from parameters is an error",
			component: &v1alpha1.Component{
				ObjectMeta: metav1.ObjectMeta{Name: "comp1", Namespace: "ns1"},
				Spec: v1alpha1.ComponentSpec{
					Workflow: &v1alpha1.WorkflowRunConfig{
						Name: "wf1",
						Parameters: makeRaw(map[string]interface{}{
							"repository": map[string]interface{}{
								"url": "https://github.com/example/repo",
							},
						}),
					},
				},
			},
			workflow: &v1alpha1.Workflow{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "wf1",
					Namespace: "ns1",
					Annotations: map[string]string{
						controller.AnnotationKeyComponentWorkflowParameters: "repoUrl: parameters.repository.url\nbranch: parameters.repository.revision.branch\n",
					},
				},
			},
			wantErr: true,
		},
		{
			name: "extracts repoUrl, appPath, and branch",
			component: &v1alpha1.Component{
				ObjectMeta: metav1.ObjectMeta{Name: "comp1", Namespace: "ns1"},
				Spec: v1alpha1.ComponentSpec{
					Workflow: &v1alpha1.WorkflowRunConfig{
						Name: "wf1",
						Parameters: makeRaw(map[string]interface{}{
							"repository": map[string]interface{}{
								"url": "https://github.com/example/repo",
								"revision": map[string]interface{}{
									"branch": "main",
								},
							},
							"appPath": "/src/app",
						}),
					},
				},
			},
			workflow: &v1alpha1.Workflow{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "wf1",
					Namespace: "ns1",
					Annotations: map[string]string{
						controller.AnnotationKeyComponentWorkflowParameters: "repoUrl: parameters.repository.url\nappPath: parameters.appPath\nbranch: parameters.repository.revision.branch\n",
					},
				},
			},
			wantRepo:    "https://github.com/example/repo",
			wantAppPath: "/src/app",
			wantBranch:  "main",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			builder := fake.NewClientBuilder().WithScheme(scheme)
			if tt.workflow != nil {
				builder = builder.WithObjects(tt.workflow)
			}
			k8sClient := builder.Build()

			svc := &WebhookService{k8sClient: k8sClient}

			gotRepo, gotAppPath, gotBranch, err := svc.extractRepoInfoFromComponent(context.Background(), tt.component)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if gotRepo != tt.wantRepo {
				t.Errorf("repoURL: got %q, want %q", gotRepo, tt.wantRepo)
			}
			if gotAppPath != tt.wantAppPath {
				t.Errorf("appPath: got %q, want %q", gotAppPath, tt.wantAppPath)
			}
			if gotBranch != tt.wantBranch {
				t.Errorf("branch: got %q, want %q", gotBranch, tt.wantBranch)
			}
		})
	}
}

// makeAutoBuildComponent returns a Component with autoBuild enabled, pointing at the given
// Workflow CR and carrying repository parameters including an optional branch.
func makeAutoBuildComponent(name, ns, workflowName, repoURL, branch string, makeRaw func(interface{}) *runtime.RawExtension) *v1alpha1.Component {
	autoBuild := true
	params := map[string]interface{}{
		"repository": map[string]interface{}{
			"url": repoURL,
			"revision": map[string]interface{}{
				"branch": branch,
			},
		},
	}
	return &v1alpha1.Component{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
		Spec: v1alpha1.ComponentSpec{
			AutoBuild: &autoBuild,
			Workflow: &v1alpha1.WorkflowRunConfig{
				Name:       workflowName,
				Parameters: makeRaw(params),
			},
		},
	}
}

// makeWorkflowWithBranch returns a Workflow CR whose annotation maps repoUrl and branch.
func makeWorkflowWithBranch(name, ns string) *v1alpha1.Workflow {
	return &v1alpha1.Workflow{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
			Annotations: map[string]string{
				controller.AnnotationKeyComponentWorkflowParameters: "repoUrl: parameters.repository.url\nbranch: parameters.repository.revision.branch\n",
			},
		},
	}
}

// makeWorkflowNoBranch returns a Workflow CR whose annotation does NOT map branch.
func makeWorkflowNoBranch(name, ns string) *v1alpha1.Workflow {
	return &v1alpha1.Workflow{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
			Annotations: map[string]string{
				controller.AnnotationKeyComponentWorkflowParameters: "repoUrl: parameters.repository.url\n",
			},
		},
	}
}

func TestWebhookBranchFilter_Match(t *testing.T) {
	makeRaw := func(v interface{}) *runtime.RawExtension {
		b, _ := json.Marshal(v)
		return &runtime.RawExtension{Raw: b}
	}

	scheme := newTestScheme(t)
	comp := makeAutoBuildComponent("svc", "ns1", "wf1", "https://github.com/example/repo", "main", makeRaw)
	workflow := makeWorkflowWithBranch("wf1", "ns1")

	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(comp, workflow).Build()
	svc := &WebhookService{k8sClient: k8sClient}

	event := &git.WebhookEvent{
		RepositoryURL: "https://github.com/example/repo",
		Branch:        "main",
	}

	affected, err := svc.findAffectedComponents(context.Background(), event)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(affected) != 1 {
		t.Fatalf("expected 1 affected component, got %d", len(affected))
	}
	if affected[0].Name != "svc" {
		t.Errorf("expected component %q, got %q", "svc", affected[0].Name)
	}
}

func TestWebhookBranchFilter_Mismatch(t *testing.T) {
	makeRaw := func(v interface{}) *runtime.RawExtension {
		b, _ := json.Marshal(v)
		return &runtime.RawExtension{Raw: b}
	}

	scheme := newTestScheme(t)
	comp := makeAutoBuildComponent("svc", "ns1", "wf1", "https://github.com/example/repo", "main", makeRaw)
	workflow := makeWorkflowWithBranch("wf1", "ns1")

	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(comp, workflow).Build()
	svc := &WebhookService{k8sClient: k8sClient}

	event := &git.WebhookEvent{
		RepositoryURL: "https://github.com/example/repo",
		Branch:        "feature/new-api",
	}

	affected, err := svc.findAffectedComponents(context.Background(), event)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(affected) != 0 {
		t.Fatalf("expected 0 affected components for branch mismatch, got %d", len(affected))
	}
}

func TestWebhookBranchFilter_NoConfiguredBranch(t *testing.T) {
	makeRaw := func(v interface{}) *runtime.RawExtension {
		b, _ := json.Marshal(v)
		return &runtime.RawExtension{Raw: b}
	}

	scheme := newTestScheme(t)
	// Component has a branch value in parameters but the Workflow annotation does NOT map "branch",
	// so no branch filtering should occur and any push branch triggers a build.
	autoBuild := true
	comp := &v1alpha1.Component{
		ObjectMeta: metav1.ObjectMeta{Name: "svc", Namespace: "ns1"},
		Spec: v1alpha1.ComponentSpec{
			AutoBuild: &autoBuild,
			Workflow: &v1alpha1.WorkflowRunConfig{
				Name: "wf1",
				Parameters: makeRaw(map[string]interface{}{
					"repository": map[string]interface{}{
						"url": "https://github.com/example/repo",
					},
				}),
			},
		},
	}
	workflow := makeWorkflowNoBranch("wf1", "ns1")

	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(comp, workflow).Build()
	svc := &WebhookService{k8sClient: k8sClient}

	for _, pushBranch := range []string{"main", "feature/foo", "release/v1"} {
		t.Run(pushBranch, func(t *testing.T) {
			event := &git.WebhookEvent{
				RepositoryURL: "https://github.com/example/repo",
				Branch:        pushBranch,
			}
			affected, err := svc.findAffectedComponents(context.Background(), event)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(affected) != 1 {
				t.Fatalf("expected 1 affected component for branch %q (no branch filter), got %d", pushBranch, len(affected))
			}
		})
	}
}
