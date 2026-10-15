// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package autobuild

import (
	"context"
	"encoding/json"
	"slices"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/models"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/git"
)

const testSchemaURLBranchAndCommit = `{"type":"object","properties":{"repository":{"type":"object","properties":{"url":{"type":"string","x-openchoreo-component-parameter-repository-url":true},"revision":{"type":"object","properties":{"branch":{"type":"string","x-openchoreo-component-parameter-repository-branch":true},"commit":{"type":"string","x-openchoreo-component-parameter-repository-commit":true}}}}}}}`

const testRepoURL = "https://github.com/example/app"

func rawJSON(v any) *runtime.RawExtension {
	b, _ := json.Marshal(v)
	return &runtime.RawExtension{Raw: b}
}

func revision(fields map[string]any) *runtime.RawExtension {
	return rawJSON(map[string]any{"repository": map[string]any{"revision": fields}})
}

func makeReleaseLine(name, component string, workflowParameters *runtime.RawExtension) *v1alpha1.ReleaseLine {
	line := &v1alpha1.ReleaseLine{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "team-a"},
		Spec: v1alpha1.ReleaseLineSpec{
			Owner: v1alpha1.ReleaseLineOwner{ProjectName: "proj", ComponentName: component},
		},
	}
	if workflowParameters != nil {
		line.Spec.Workflow = &v1alpha1.ReleaseLineWorkflow{Parameters: workflowParameters}
	}
	return line
}

// newReleaseLineWebhookClient returns a client with component "api", which has release lines
// enabled and builds branch "stable", and these lines:
//   - main: inherits the component's branch
//   - next: builds branch "next"
//   - pinned: builds branch "next", pinned to a commit
//
// plus a line of another component that also builds branch "next".
func newReleaseLineWebhookClient(t *testing.T) client.Client {
	t.Helper()
	comp := makeAutoBuildComponent("api", "team-a", "build", testRepoURL, "stable", rawJSON)
	comp.Spec.Owner.ProjectName = "proj"
	comp.Spec.ReleaseLinesEnabled = true
	workflow := makeWorkflowWithBranch("build", "team-a")
	workflow.Spec.Parameters.OpenAPIV3Schema.Raw = []byte(testSchemaURLBranchAndCommit)

	return fake.NewClientBuilder().WithScheme(newTestSchemeForWebhook(t)).WithObjects(
		comp, workflow,
		makeReleaseLine("main", "api", nil),
		makeReleaseLine("next", "api", revision(map[string]any{"branch": "next"})),
		makeReleaseLine("pinned", "api", revision(map[string]any{"branch": "next", "commit": "abc123"})),
		makeReleaseLine("other-next", "other", revision(map[string]any{"branch": "next"})),
	).Build()
}

func TestFindBuildTargets_ReleaseLines(t *testing.T) {
	tests := []struct {
		branch string
		want   []string
	}{
		{branch: "stable", want: []string{"team-a/api@main"}},
		{branch: "next", want: []string{"team-a/api@next"}},
		{branch: "other", want: []string{}},
	}
	for _, tt := range tests {
		t.Run(tt.branch, func(t *testing.T) {
			svc := &webhookProcessor{k8sClient: newReleaseLineWebhookClient(t), logger: discardLogger()}
			targets, err := svc.findBuildTargets(context.Background(), &git.WebhookEvent{
				Provider:      string(git.ProviderGitHub),
				RepositoryURL: testRepoURL,
				Branch:        tt.branch,
			})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			got := make([]string, 0, len(targets))
			for _, target := range targets {
				got = append(got, target.String())
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("build targets: got %v, want %v", got, tt.want)
			}
		})
	}
}

type recordingTrigger struct {
	releaseLines []string
}

func (r *recordingTrigger) TriggerWorkflow(_ context.Context, _, _, _, _, releaseLine string) (*models.WorkflowRunTriggerResponse, error) {
	r.releaseLines = append(r.releaseLines, releaseLine)
	return &models.WorkflowRunTriggerResponse{}, nil
}

type fixedEventProvider struct {
	event *git.WebhookEvent
}

func (p fixedEventProvider) ValidateWebhookPayload([]byte, string, string) error { return nil }

func (p fixedEventProvider) ParseWebhookPayload([]byte) (*git.WebhookEvent, error) {
	return p.event, nil
}

func TestProcessWebhook_TriggersReleaseLine(t *testing.T) {
	trigger := &recordingTrigger{}
	processor := NewWebhookProcessor(newReleaseLineWebhookClient(t), trigger, discardLogger())

	triggered, err := processor.ProcessWebhook(context.Background(), fixedEventProvider{event: &git.WebhookEvent{
		Provider:      string(git.ProviderGitHub),
		RepositoryURL: testRepoURL,
		Branch:        "next",
		Commit:        "def456",
	}}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !slices.Equal(trigger.releaseLines, []string{"next"}) {
		t.Errorf("triggered release lines: got %v, want [next]", trigger.releaseLines)
	}
	if !slices.Equal(triggered, []string{"team-a/api@next"}) {
		t.Errorf("triggered: got %v, want [team-a/api@next]", triggered)
	}
}
