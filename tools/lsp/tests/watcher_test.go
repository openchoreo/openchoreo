// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package lsp_test

import (
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sourcegraph/jsonrpc2"
	protocol "github.com/tliron/glsp/protocol_3_16"

	"github.com/openchoreo/openchoreo/tools/lsp"
)

// Filesystem watcher tests. After the initial scan the server watches the
// workspace tree, so on-disk changes to non-open YAML files must re-index and
// republish diagnostics for every open document. The observable used here is
// the FileNameRefDiagnostics "unknown-resource-file" code: an open document
// whose `name:` reference is a near-miss of an indexed file name gains the
// diagnostic when the file appears on disk and loses it when the file goes.
//
// These tests run against a real filesystem (t.TempDir), so they poll the wire
// with generous deadlines instead of asserting on exact frame timing.

const watchOpenComponent = "apiVersion: openchoreo.dev/v1alpha1\n" +
	"kind: Component\n" +
	"metadata:\n" +
	"  name: hello-srv\n" +
	"spec:\n" +
	"  componentType: service\n" +
	"  source:\n" +
	"    repository:\n" +
	"      name: docker-gitops-releasd\n" +
	"      branch: main\n"

// watchPipeline defines a file whose name is one edit away from the
// misspelling in watchOpenComponent: creating it must surface the reference
// diagnostic, deleting it must clear it.
const watchPipeline = "apiVersion: openchoreo.dev/v1alpha1\n" +
	"kind: DeploymentPipeline\n" +
	"metadata:\n" +
	"  name: docker-gitops-release\n" +
	"spec:\n" +
	"  componentType: service\n"

const unknownResourceFileCode = "unknown-resource-file"

// watchUnrelated is a valid resource whose name is not a near-miss of any
// reference in watchOpenComponent, so it never changes the reference diag.
const watchUnrelated = "apiVersion: openchoreo.dev/v1alpha1\n" +
	"kind: Component\n" +
	"metadata:\n" +
	"  name: unrelated-svc\n" +
	"spec:\n" +
	"  componentType: service\n"

// watchPipelineFileName is a DeploymentPipeline whose metadata.name differs
// from its file name. References must then resolve through the file name, so
// renaming the file on disk actually changes what FileNamesRefDiagnostics
// sees. (watchOpenComponent stays a near-miss of this file's basename.)
const watchPipelineFileName = "apiVersion: openchoreo.dev/v1alpha1\n" +
	"kind: DeploymentPipeline\n" +
	"metadata:\n" +
	"  name: pipeline-spec-name\n" +
	"spec:\n" +
	"  componentType: service\n"

// codeProbe extracts diagnostic codes from raw params, bypassing glsp's
// IntegerOrString unmarshal quirk for diagnostic.Code.
type codeProbe struct {
	URI         string `json:"uri"`
	Diagnostics []struct {
		Code json.RawMessage `json:"code"`
	} `json:"diagnostics"`
}

// watchServer starts a server over a pipe, initializes it against a
// filesystem root, waits for the initial scan (after which the watcher runs),
// and cleans the watcher up at the end of the test.
func watchServer(t *testing.T, root string) (*lsp.Server, net.Conn) {
	t.Helper()
	s, client := newIndexingTestServer(t)
	t.Cleanup(s.StopWatcher)

	if _, err := s.Initialize(nil, initializeIndexingParams(t, root, false)); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	waitIndexed(t, s)
	return s, client
}

// makeWatchRoot creates a workspace with an empty "resources" directory so the
// watcher has a pre-existing subdirectory to observe.
func makeWatchRoot(t *testing.T) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "workspace")
	if err := os.MkdirAll(filepath.Join(root, "resources"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	return root
}

// readWatchPublish reads frames until the next publishDiagnostics and returns
// its raw params for code probing.
func readWatchPublish(t *testing.T, client net.Conn, within time.Duration) json.RawMessage {
	t.Helper()
	if err := client.SetReadDeadline(time.Now().Add(within)); err != nil {
		t.Fatal(err)
	}
	stream := jsonrpc2.NewBufferedStream(client, jsonrpc2.VSCodeObjectCodec{})
	defer func() {
		if err := client.SetReadDeadline(time.Time{}); err != nil {
			t.Error(err)
		}
	}()

	for {
		var frame rxFrame
		if err := stream.ReadObject(&frame); err != nil {
			t.Fatalf("readWatchPublish within %v: %v", within, err)
		}
		if frame.Method == protocol.ServerTextDocumentPublishDiagnostics {
			var probe codeProbe
			if err := json.Unmarshal(frame.Params, &probe); err != nil {
				t.Fatal(err)
			}
			// Closed workspace files now publish too. These tests observe the
			// virtual open consumer, not the resource file that changed.
			if strings.HasPrefix(probe.URI, "file:///virtual/") {
				return frame.Params
			}
		}
	}
}

func probeCodes(raw json.RawMessage) []string {
	var probe codeProbe
	if err := json.Unmarshal(raw, &probe); err != nil {
		return nil
	}
	var codes []string
	for _, d := range probe.Diagnostics {
		var s string
		if err := json.Unmarshal(d.Code, &s); err == nil {
			codes = append(codes, s)
		}
	}
	return codes
}

func hasDiagCode(raw json.RawMessage) bool {
	code := unknownResourceFileCode

	for _, c := range probeCodes(raw) {
		if c == code {
			return true
		}
	}
	return false
}

// drainWatchFrames consumes any frames the server still has in flight. Several
// watch events (a kqueue flush plus per-path debounces) can each republish
// every open document, so after making its assertion a test must drain the
// surplus; otherwise server-side SendAsync calls stay blocked on the unbuffered
// net.Pipe and the connection cannot close cleanly.
func drainWatchFrames(t *testing.T, client net.Conn, within time.Duration) {
	t.Helper()
	deadline := time.Now().Add(within)
	for {
		if err := client.SetReadDeadline(time.Now().Add(200 * time.Millisecond)); err != nil {
			t.Fatal(err)
		}
		stream := jsonrpc2.NewBufferedStream(client, jsonrpc2.VSCodeObjectCodec{})
		var frame rxFrame
		if err := stream.ReadObject(&frame); err != nil {
			if err := client.SetReadDeadline(time.Time{}); err != nil {
				t.Fatal(err)
			}
			return
		}
		if time.Now().After(deadline) {
			if err := client.SetReadDeadline(time.Time{}); err != nil {
				t.Fatal(err)
			}
			return
		}
	}
}

// readWatchPublishForURI reads frames until a publishDiagnostics whose URI
// matches wantURI, skipping publishes for other documents (e.g. both files of
// a split-window pair are open and republished together).
func readWatchPublishForURI(t *testing.T, client net.Conn, wantURI string, within time.Duration) json.RawMessage {
	t.Helper()
	if err := client.SetReadDeadline(time.Now().Add(within)); err != nil {
		t.Fatal(err)
	}
	stream := jsonrpc2.NewBufferedStream(client, jsonrpc2.VSCodeObjectCodec{})
	defer func() {
		if err := client.SetReadDeadline(time.Time{}); err != nil {
			t.Error(err)
		}
	}()

	for {
		var frame rxFrame
		if err := stream.ReadObject(&frame); err != nil {
			t.Fatalf("readWatchPublishForURI(%q) within %v: %v", wantURI, within, err)
		}
		if frame.Method != protocol.ServerTextDocumentPublishDiagnostics {
			continue
		}
		var probe codeProbe
		if err := json.Unmarshal(frame.Params, &probe); err != nil {
			continue
		}
		if probe.URI == wantURI {
			return frame.Params
		}
	}
}

func TestWatchCreateIndexesFileAndPublishesDiagnostics(t *testing.T) {
	root := makeWatchRoot(t)
	s, client := watchServer(t, root)

	openURI := testValueFileVirtualOpenYaml
	openDoc(t, s, openURI, watchOpenComponent, 1)
	expectNoFrame(t, client, 300*time.Millisecond)

	// A resource file that is one edit away from the open document's
	// misspelled reference appears on disk.
	created := filepath.Join(root, "resources", "docker-gitops-release.yaml")
	if err := os.WriteFile(created, []byte(watchPipeline), 0o600); err != nil {
		t.Fatalf("write resource file: %v", err)
	}

	raw := readWatchPublish(t, client, 5*time.Second)
	if !hasDiagCode(raw) {
		t.Errorf("resource appeared on disk but the reference diagnostic stayed clear; codes=%v", probeCodes(raw))
	}
}

func TestWatchDeleteFileClearsDiagnostics(t *testing.T) {
	root := makeWatchRoot(t)
	s, client := watchServer(t, root)

	// The reference target exists from the very start.
	created := filepath.Join(root, "resources", "docker-gitops-release.yaml")
	if err := os.WriteFile(created, []byte(watchPipeline), 0o600); err != nil {
		t.Fatalf("write resource file: %v", err)
	}

	openURI := testValueFileVirtualOpenYaml
	openDoc(t, s, openURI, watchOpenComponent, 1)

	// The initial index already contains the file, so the diagnostic is
	// present on the open document right away.
	raw := readWatchPublish(t, client, 5*time.Second)
	if !hasDiagCode(raw) {
		t.Fatalf("expected the reference diagnostic before deletion; codes=%v", probeCodes(raw))
	}

	// Deleting the file must remove its index entry and clear the diagnostic.
	if err := os.Remove(created); err != nil {
		t.Fatalf("remove resource file: %v", err)
	}
	raw = readWatchPublish(t, client, 5*time.Second)
	if hasDiagCode(raw) {
		t.Errorf("resource vanished on disk but the reference diagnostic stayed; codes=%v", probeCodes(raw))
	}
}

func TestWatchRegistersNewSubdirectories(t *testing.T) {
	root := makeWatchRoot(t)
	s, client := watchServer(t, root)

	openURI := testValueFileVirtualOpenYaml
	openDoc(t, s, openURI, watchOpenComponent, 1)
	expectNoFrame(t, client, 300*time.Millisecond)

	// A brand-new directory is created after the watcher is running; files
	// written inside it later must still be indexed.
	sub := filepath.Join(root, "created-later")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(sub, "docker-gitops-release.yaml"), []byte(watchPipeline), 0o600); err != nil {
		t.Fatalf("write resource file: %v", err)
	}

	// Directory creation and file creation can legitimately publish separate
	// snapshots. Wait for the final indexed state, not the first notification.
	deadline := time.Now().Add(5 * time.Second)
	for {
		raw := readWatchPublish(t, client, time.Until(deadline))
		if hasDiagCode(raw) {
			break
		}
		if !time.Now().Before(deadline) {
			t.Fatalf("file in newly created subdirectory was not indexed; codes=%v", probeCodes(raw))
		}
	}
}

// TestWatchRenameReferencedFileWhileOpenUpdatesDiagnostics reproduces a
// split-window editing workflow: the file being renamed and the document that
// references it are both open.
func TestWatchRenameReferencedFileWhileOpenUpdatesDiagnostics(t *testing.T) {
	root := makeWatchRoot(t)

	// The reference target exists on disk from the very start. Its metadata.name
	// differs from its file name, so the open document's reference resolves
	// through the file name specifically.
	created := filepath.Join(root, "resources", "docker-gitops-release.yaml")
	if err := os.WriteFile(created, []byte(watchPipelineFileName), 0o600); err != nil {
		t.Fatalf("write resource file: %v", err)
	}

	s, client := watchServer(t, root)

	// Both files are open (split window): the target by its real path, the
	// referencing document under a virtual URI.
	refURI := "file://" + created
	openDoc(t, s, refURI, watchPipelineFileName, 1)

	openURI := testValueFileVirtualOpenYaml
	openDoc(t, s, openURI, watchOpenComponent, 1)

	// Baseline: an unrelated disk change forces a revalidation of the open
	// document, which is still a near-miss of the target's name. (didOpen with
	// a nil context below sends no frame, so the watcher supplies the publish.)
	extra := filepath.Join(root, "resources", "unrelated.yaml")
	if err := os.WriteFile(extra, []byte(watchUnrelated), 0o600); err != nil {
		t.Fatalf("write unrelated file: %v", err)
	}
	first := readWatchPublishForURI(t, client, openURI, 5*time.Second)
	if !hasDiagCode(first) {
		t.Fatalf("expected the reference diagnostic before rename; codes=%v", probeCodes(first))
	}

	// Rename the referenced file on disk while its document is still open.
	renamed := filepath.Join(root, "resources", "renamed-pipeline.yaml")
	if err := os.Rename(created, renamed); err != nil {
		t.Fatalf("rename resource file: %v", err)
	}

	// The old file name must leave the index even though the document is open,
	// so the misspelled reference no longer matches anything. The rename's
	// events race each other, so a stale republish may arrive first; read until
	// the diagnostic clears.
	cleared := false
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		after := readWatchPublishForURI(t, client, openURI, time.Until(deadline))
		if !hasDiagCode(after) {
			cleared = true
			break
		}
	}
	if !cleared {
		t.Error("stale name still resolves the reference after rename")
	}

	// Let the remainder of the revalidation burst drain so SendAsync completes.
	drainWatchFrames(t, client, 2*time.Second)
}

// watchTraitsRef is a ComponentType that references a Trait by its declared
// resource name, mirroring the sample-gitops webapp/observability-alert-rule
// pair in real workspaces.
const watchTraitsRef = "apiVersion: openchoreo.dev/v1alpha1\n" +
	"kind: ComponentType\n" +
	"metadata:\n" +
	"  name: web-application\n" +
	"spec:\n" +
	"  allowedTraits:\n" +
	"    - name: observability-alert-rule\n"

// watchTraitsTarget declares the referenced Trait resource.
const watchTraitsTarget = "apiVersion: openchoreo.dev/v1alpha1\n" +
	"kind: Trait\n" +
	"metadata:\n" +
	"  name: observability-alert-rule\n" +
	"spec: {}\n"

// watchTraitsTargetRenamed is the same Trait after its metadata.name changed.
const watchTraitsTargetRenamed = "apiVersion: openchoreo.dev/v1alpha1\n" +
	"kind: Trait\n" +
	"metadata:\n" +
	"  name: observability-alert-rules\n" +
	"spec: {}\n"

// TestDidChangeResourceNameRevalidatesReferencingDocuments reproduces the
// reported live bug: renaming a resource's metadata.name while its file is
// open must raise the reference diagnostic in an open document that points at
// the old name, without the user touching that document. A resource name is
// what `name:` references resolve against, so a mere file basename cannot keep
// the reference valid after the name is gone.
func TestDidChangeResourceNameRevalidatesReferencingDocuments(t *testing.T) {
	root := makeWatchRoot(t)

	target := filepath.Join(root, "resources", "observability-alert-rule.yaml")
	if err := os.WriteFile(target, []byte(watchTraitsTarget), 0o600); err != nil {
		t.Fatalf("write trait file: %v", err)
	}

	s, client := watchServer(t, root)

	// Split window: the trait file and the document referencing it are both
	// open.
	traitURI := "file://" + target
	openDoc(t, s, traitURI, watchTraitsTarget, 1)

	openURI := "file:///virtual/webapp.yaml"
	openDoc(t, s, openURI, watchTraitsRef, 1)

	// Baseline: an unrelated disk change forces a revalidation; the reference
	// resolves exactly against the trait's metadata.name, so no diagnostic.
	extra := filepath.Join(root, "resources", "unrelated.yaml")
	if err := os.WriteFile(extra, []byte(watchUnrelated), 0o600); err != nil {
		t.Fatalf("write unrelated file: %v", err)
	}
	first := readWatchPublishForURI(t, client, openURI, 5*time.Second)
	if hasDiagCode(first) {
		t.Fatalf("unexpected reference diagnostic before the rename; codes=%v", probeCodes(first))
	}

	// Rename the resource inside the open trait file (its metadata.name value),
	// as the user did. The old name leaves the index; the referencing document
	// must be revalidated automatically and report the now-unresolved name.
	changeDoc(t, s, traitURI, 2, protocol.TextDocumentContentChangeEventWhole{Text: watchTraitsTargetRenamed})

	got := false
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		after := readWatchPublishForURI(t, client, openURI, time.Until(deadline))
		if hasDiagCode(after) {
			got = true
			break
		}
	}
	if !got {
		t.Error("reference diagnostic never surfaced after the resource was renamed")
	}

	drainWatchFrames(t, client, 2*time.Second)
}

func TestWatchIgnoresChangesToOpenDocuments(t *testing.T) {
	root := makeWatchRoot(t)
	s, client := watchServer(t, root)

	// The open document's in-memory text is authoritative: on-disk writes to
	// its file must not trigger a republish (the editor owns those edits).
	path := filepath.Join(root, "app.yaml")
	openURI := "file://" + path
	openDoc(t, s, openURI, watchOpenComponent, 1)
	expectNoFrame(t, client, 300*time.Millisecond)

	if err := os.WriteFile(path, []byte(watchPipeline), 0o600); err != nil {
		t.Fatalf("write open document on disk: %v", err)
	}

	expectNoFrame(t, client, 800*time.Millisecond)
}

func TestWatchIgnoresNonYAMLAndHiddenFiles(t *testing.T) {
	root := makeWatchRoot(t)
	s, client := watchServer(t, root)

	openURI := testValueFileVirtualOpenYaml
	openDoc(t, s, openURI, watchOpenComponent, 1)
	expectNoFrame(t, client, 300*time.Millisecond)

	// Non-YAML and dot-named files must not re-index anything.
	if err := os.WriteFile(filepath.Join(root, "notes.txt"), []byte("hello\n"), 0o600); err != nil {
		t.Fatalf("write txt: %v", err)
	}
	// Even a YAML file whose content would match the reference is ignored
	// because it is hidden.
	if err := os.WriteFile(filepath.Join(root, ".docker-gitops-release.yaml"), []byte(watchPipeline), 0o600); err != nil {
		t.Fatalf("write hidden yaml: %v", err)
	}

	expectNoFrame(t, client, 800*time.Millisecond)
}
