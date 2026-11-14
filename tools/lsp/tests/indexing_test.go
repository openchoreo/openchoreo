// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package lsp_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/sourcegraph/jsonrpc2"
	protocol "github.com/tliron/glsp/protocol_3_16"

	"github.com/openchoreo/openchoreo/tools/lsp"
)

// Background indexing tests: they drive the scan through the public Initialize
// (with rootUri + capabilities, exactly like a client would) and read the
// $/progress / window/workDoneProgress/create frames off the wire, replying to
// the create request like a real client.

const indexingTestYAML = "apiVersion: openchoreo.dev/v1alpha1\nkind: Component\nmetadata:\n  name: svc-"

type indexingFrame struct {
	Method string          `json:"method"`
	Params json.RawMessage `json:"params"`
	ID     json.RawMessage `json:"id"`
}

type progressKind struct {
	Kind string `json:"kind"`
}

// initializeIndexingParams builds InitializeParams via JSON so the anonymous
// Window capability struct can be populated with the WorkDoneProgress flag.
func initializeIndexingParams(t *testing.T, root string, workDoneProgress bool) *protocol.InitializeParams {
	t.Helper()
	rootJSON, err := json.Marshal("file://" + root)
	if err != nil {
		t.Fatalf("marshal root: %v", err)
	}
	body := fmt.Sprintf(`{"rootUri":%s,"capabilities":{"window":{"workDoneProgress":%t}}}`, rootJSON, workDoneProgress)
	var p protocol.InitializeParams
	if err := json.Unmarshal([]byte(body), &p); err != nil {
		t.Fatalf("unmarshal initialize params: %v", err)
	}
	return &p
}

// initializeWorkspaceFoldersParams builds InitializeParams the way a modern
// multi-root client does: workspaceFolders with entries for every root and a
// null rootUri.
func initializeWorkspaceFoldersParams(t *testing.T, roots []string, workDoneProgress bool) *protocol.InitializeParams {
	t.Helper()
	folders := make([]map[string]string, 0, len(roots))
	for i, root := range roots {
		folders = append(folders, map[string]string{
			"uri":         "file://" + root,
			testValueName: fmt.Sprintf("folder-%d", i),
		})
	}
	foldersJSON, err := json.Marshal(folders)
	if err != nil {
		t.Fatalf("marshal folders: %v", err)
	}
	body := fmt.Sprintf(`{"rootUri":null,"workspaceFolders":%s,"capabilities":{"window":{"workDoneProgress":%t}}}`, foldersJSON, workDoneProgress)
	var p protocol.InitializeParams
	if err := json.Unmarshal([]byte(body), &p); err != nil {
		t.Fatalf("unmarshal initialize params: %v", err)
	}
	return &p
}

// newIndexingTestServer wires a fresh server to the client side of a net.Pipe.
// The test reads frames from that side and answers server-to-client requests.
func newIndexingTestServer(t *testing.T) (*lsp.Server, net.Conn) {
	t.Helper()
	serverSide, clientSide := net.Pipe()
	t.Cleanup(func() {
		serverSide.Close()
		clientSide.Close()
	})

	s := lsp.NewServer()
	s.SetLogger(slog.New(slog.NewTextHandler(io.Discard, nil)))
	conn := jsonrpc2.NewConn(
		context.Background(),
		jsonrpc2.NewBufferedStream(serverSide, jsonrpc2.VSCodeObjectCodec{}),
		jsonrpc2.HandlerWithError(func(ctx context.Context, c *jsonrpc2.Conn, req *jsonrpc2.Request) (any, error) {
			return nil, nil
		}),
	)
	s.SetConnection(conn)
	t.Cleanup(func() { serverSide.Close(); clientSide.Close(); conn.Close() })
	return s, clientSide
}

// makeIndexWorkspace creates count YAML files (and one ignored text file).
func makeIndexWorkspace(t *testing.T, count int) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "workspace")
	for i := 0; i < count; i++ {
		path := filepath.Join(root, "config", "component-"+string(rune('a'+i))+".yaml")
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(path, []byte(indexingTestYAML+string(rune('a'+i))+"\n"), 0o600); err != nil {
			t.Fatalf("write yaml: %v", err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "config", "notes.txt"), []byte("not yaml\n"), 0o600); err != nil {
		t.Fatalf("write txt: %v", err)
	}
	return root
}

// waitIndexed blocks until the initial workspace scan completes.
func waitIndexed(t *testing.T, s *lsp.Server) {
	t.Helper()
	select {
	case <-s.Indexed():
	case <-time.After(5 * time.Second):
		t.Fatal("workspace scan did not finish in time")
	}
}

// replyIndexingCreate answers a window/workDoneProgress/create request on the
// client side of the pipe. errCode 0 acknowledges it with a null result; any
// other code rejects it like a client that registered no such handler.
func replyIndexingCreate(t *testing.T, client net.Conn, frame indexingFrame, errCode int64) {
	t.Helper()
	var id jsonrpc2.ID
	if err := json.Unmarshal(frame.ID, &id); err != nil {
		t.Fatalf("unmarshal request id: %v", err)
	}
	resp := jsonrpc2.Response{ID: id}
	if errCode == 0 {
		result := json.RawMessage("null")
		resp.Result = &result
	} else {
		resp.Error = &jsonrpc2.Error{Code: errCode, Message: "Method not found"}
	}
	if err := (jsonrpc2.VSCodeObjectCodec{}).WriteObject(client, resp); err != nil {
		t.Fatalf("write progress create reply: %v", err)
	}
}

// consumeProgress reads frames from the client side until a $/progress "end"
// arrives, replying to the create request with replyErrCode along the way, and
// returns whether create/begin/report were observed before the end.
func consumeProgress(t *testing.T, client net.Conn, replyErrCode int64) (create, begin, report bool) {
	t.Helper()
	if err := client.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := client.SetReadDeadline(time.Time{}); err != nil {
			t.Error(err)
		}
	}()
	stream := jsonrpc2.NewBufferedStream(client, jsonrpc2.VSCodeObjectCodec{})

	for {
		var frame indexingFrame
		if err := stream.ReadObject(&frame); err != nil {
			t.Fatalf("reading progress frames: %v", err)
		}

		switch frame.Method {
		case protocol.ServerWindowWorkDoneProgressCreate:
			create = true
			replyIndexingCreate(t, client, frame, replyErrCode)
		case protocol.MethodProgress:
			var p struct {
				Token json.RawMessage `json:"token"`
				Value progressKind    `json:"value"`
			}
			if err := json.Unmarshal(frame.Params, &p); err != nil {
				t.Fatalf("unmarshal $/progress: %v", err)
			}
			var token string
			if err := json.Unmarshal(p.Token, &token); err != nil {
				t.Fatalf("unmarshal progress token: %v", err)
			}
			if token != "openchoreo-indexing" {
				t.Errorf("progress token = %q, want %q", token, "openchoreo-indexing")
			}
			switch p.Value.Kind {
			case "begin":
				begin = true
			case "report":
				report = true
			case "end":
				return create, begin, report
			}
		}
	}
}

func TestIndexingReportsProgressWhenClientSupportsIt(t *testing.T) {
	s, client := newIndexingTestServer(t)
	root := makeIndexWorkspace(t, 3)

	if _, err := s.Initialize(nil, initializeIndexingParams(t, root, true)); err != nil {
		t.Fatalf("Initialize: %v", err)
	}

	create, begin, report := consumeProgress(t, client, 0)
	if !create {
		t.Error("did not receive window/workDoneProgress/create before begin")
	}
	if !begin {
		t.Error("did not receive $/progress begin")
	}
	if !report {
		t.Error("did not receive any $/progress report")
	}

	// The end notification concludes the sequence: nothing may follow.
	expectNoIndexingFrames(t, client)
}

func TestIndexingSkipsProgressWhenClientRejectsCreate(t *testing.T) {
	s, client := newIndexingTestServer(t)
	root := makeIndexWorkspace(t, 2)

	if _, err := s.Initialize(nil, initializeIndexingParams(t, root, true)); err != nil {
		t.Fatalf("Initialize: %v", err)
	}

	// The client advertises WorkDoneProgress but its request handler answers
	// -32601: it registered no server-to-client request handlers.
	if err := client.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
		t.Fatal(err)
	}
	stream := jsonrpc2.NewBufferedStream(client, jsonrpc2.VSCodeObjectCodec{})
	var create indexingFrame
	for {
		if err := stream.ReadObject(&create); err != nil {
			t.Fatalf("reading frames: %v", err)
		}
		if create.Method == protocol.ServerWindowWorkDoneProgressCreate {
			break
		}
	}
	replyIndexingCreate(t, client, create, -32601)

	// Indexing still completes, and no $/progress frames may follow.
	waitIndexed(t, s)
	expectNoIndexingFrames(t, client)
}

func TestIndexingNoProgressWithoutClientSupport(t *testing.T) {
	s, client := newIndexingTestServer(t)
	root := makeIndexWorkspace(t, 2)

	if _, err := s.Initialize(nil, initializeIndexingParams(t, root, false)); err != nil {
		t.Fatalf("Initialize: %v", err)
	}

	waitIndexed(t, s)

	// No work-done progress frames may be sent for a client that did not
	// declare Window.WorkDoneProgress.
	expectNoIndexingFrames(t, client)
}

func TestIndexingRunsOncePerServer(t *testing.T) {
	s, client := newIndexingTestServer(t)
	root := makeIndexWorkspace(t, 2)

	if _, err := s.Initialize(nil, initializeIndexingParams(t, root, true)); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	// Drain the first scan's progress frames.
	consumeProgress(t, client, 0)

	// Initialize again with the same root: the scan must not be re-run, so
	// nothing new is sent.
	if _, err := s.Initialize(nil, initializeIndexingParams(t, root, true)); err != nil {
		t.Fatalf("second Initialize: %v", err)
	}
	expectNoIndexingFrames(t, client)
}

// TestIndexingMultipleRoots drives the scan through workspaceFolders (the
// multi-root shape a modern client sends) and verifies resources from every
// root land in the single shared index.
func TestIndexingMultipleRoots(t *testing.T) {
	s, client := newIndexingTestServer(t)

	rootA := filepath.Join(t.TempDir(), "proj-a")
	rootB := filepath.Join(t.TempDir(), "proj-b")
	for _, root := range []string{rootA, rootB} {
		if err := os.MkdirAll(root, 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
	}
	if err := os.WriteFile(filepath.Join(rootA, "a.yaml"), []byte(indexingTestYAML+"from-a\n"), 0o600); err != nil {
		t.Fatalf("write a.yaml: %v", err)
	}
	if err := os.WriteFile(filepath.Join(rootB, "b.yaml"), []byte(indexingTestYAML+"from-b\n"), 0o600); err != nil {
		t.Fatalf("write b.yaml: %v", err)
	}

	if _, err := s.Initialize(nil, initializeWorkspaceFoldersParams(t, []string{rootA, rootB}, false)); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	waitIndexed(t, s)

	names := s.ResourceNames()
	if !containsName(names, "svc-from-a") {
		t.Errorf("index missing resource from root A: %v", names)
	}
	if !containsName(names, "svc-from-b") {
		t.Errorf("index missing resource from root B: %v", names)
	}
	// No $/progress frames were declared, so nothing may be sent.
	expectNoIndexingFrames(t, client)
}

// TestInitializeWithoutRootsKeepsIndexEmpty mirrors a client that sends neither
// rootUri nor workspaceFolders: the server must not crash and simply leaves the
// index empty (reference checks then degrade gracefully instead of panicking).
func TestInitializeWithoutRootsKeepsIndexEmpty(t *testing.T) {
	s, client := newIndexingTestServer(t)

	var p protocol.InitializeParams
	if err := json.Unmarshal([]byte(`{"rootUri":null,"capabilities":{}}`), &p); err != nil {
		t.Fatalf("unmarshal initialize params: %v", err)
	}
	if _, err := s.Initialize(nil, &p); err != nil {
		t.Fatalf("Initialize: %v", err)
	}

	// No scan starts, so the index stays empty and ResourceNames is empty.
	if names := s.ResourceNames(); len(names) != 0 {
		t.Fatalf("expected empty index, got %v", names)
	}
	expectNoIndexingFrames(t, client)
}

// Automatic workspace diagnostics may follow indexing; progress must still stop.
func expectNoIndexingFrames(t *testing.T, client net.Conn) {
	within := 300 * time.Millisecond

	t.Helper()
	if err := client.SetReadDeadline(time.Now().Add(within)); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := client.SetReadDeadline(time.Time{}); err != nil {
			t.Error(err)
		}
	}()
	stream := jsonrpc2.NewBufferedStream(client, jsonrpc2.VSCodeObjectCodec{})
	for {
		var frame rxFrame
		if err := stream.ReadObject(&frame); err != nil {
			return
		}
		if frame.Method != protocol.ServerTextDocumentPublishDiagnostics {
			t.Fatalf("unexpected post-indexing frame %q", frame.Method)
		}
	}
}
