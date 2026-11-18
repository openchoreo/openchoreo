// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0
package lsp

import (
	"context"
	"encoding/json"
	"github.com/sourcegraph/jsonrpc2"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	completion "github.com/openchoreo/openchoreo/tools/lsp/code-completion"
	"github.com/tliron/glsp"
)

func TestClusterNotificationSnapshots(t *testing.T) {
	s := NewServer()
	h := clusterHandler{feed: &s.cluster, report: func(error) {}}
	send := func(method, data string) {
		t.Helper()
		_, valid, params, err := h.Handle(&glsp.Context{Method: method, Params: json.RawMessage(data)})
		if !valid || !params || err != nil {
			t.Fatal(valid, params, err)
		}
	}
	send("openchoreo/updateResources", `{"ComponentType":["remote","remote"],"ComponentRelease:app":["r1"]}`)
	idx := s.clusterCompletionIndex("file:///test.yaml", "")
	if len(idx.NamesByKind()["ComponentType"]) != 1 || len(idx.ClusterNames("ComponentRelease:app")) != 1 {
		t.Fatal(idx.names)
	}
	if _, ok := idx.names["ComponentRelease:app"]; ok {
		t.Fatal("scoped releases leaked")
	}
	for _, bad := range []string{`null`, `[]`, `{"ComponentType":null}`, `{"ComponentType":[1]}`, `{`, ` `} {
		send("openchoreo/updateResources", bad)
	}
	if len(s.cluster.names["ComponentType"]) != 2 {
		t.Fatal("invalid notification replaced snapshot")
	}
	send("openchoreo/updateResources", `{"Trait":["t"]}`)
	if len(s.cluster.names["ComponentType"]) != 0 {
		t.Fatal("snapshot merged instead of replaced")
	}
	send("openchoreo/updateResources", `{}`)
	if len(s.cluster.names) != 0 {
		t.Fatal("clear failed")
	}
}

func TestClusterSchemaCompletion(t *testing.T) {
	s := NewServer()
	data := `{"ComponentType":{"service":{"parameters":{"type":"object","properties":{"greeting":{"type":"string","description":"Greeting text","enum":["hello","hi"]},"nested":{"type":"object","properties":{"count":{"type":"integer"}}}},"required":["greeting"]}}}}`
	if err := s.cluster.update("openchoreo/updateResourceSchemas", []byte(data)); err != nil {
		t.Fatal(err)
	}
	text := "apiVersion: openchoreo.dev/v1alpha1\nkind: Component\nmetadata:\n  name: app\nspec:\n  componentType:\n    kind: ComponentType\n    name: service\n  parameters:\n    "
	idx := s.clusterCompletionIndex("file:///test.yaml", text)
	items, err := completion.HandleWithIndex(text, 10, 5, idx, "file:///test.yaml")
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(items)
	var result any
	_ = json.Unmarshal(raw, &result)
	if !containsLabel(raw, "greeting") {
		t.Fatalf("missing cluster property: %s", raw)
	}
	for _, tc := range []struct {
		suffix string
		label  string
		absent string
	}{
		{"nested:\n      ", "count", "greeting"},
		{"greeting: hello\n    ", "nested", "greeting"},
	} {
		nested := text + tc.suffix
		lines := strings.Split(nested, "\n")
		result, err := completion.HandleWithIndex(nested, len(lines), len(lines[len(lines)-1])+1, s.clusterCompletionIndex("file:///test.yaml", nested), "file:///test.yaml")
		if err != nil {
			t.Fatal(err)
		}
		raw, _ := json.Marshal(result)
		if !containsLabel(raw, tc.label) || containsLabel(raw, tc.absent) {
			t.Fatalf("unexpected nested/filter completion: %s", raw)
		}
	}
	for _, bad := range []string{`null`, `{"ComponentType":null}`, `{"ComponentType":{"service":{"parameters":[]}}}`} {
		if s.cluster.update("openchoreo/updateResourceSchemas", []byte(bad)) == nil {
			t.Fatalf("accepted %s", bad)
		}
	}
	if len(s.cluster.schemas) != 1 {
		t.Fatal("invalid schemas replaced cache")
	}
	if err := s.cluster.update("openchoreo/updateResourceSchemas", []byte(`{}`)); err != nil {
		t.Fatal(err)
	}
	if len(s.cluster.schemas) != 0 {
		t.Fatal("schema clear failed")
	}
}
func containsLabel(raw []byte, label string) bool {
	var list struct {
		Items []map[string]any `json:"items"`
	}
	if json.Unmarshal(raw, &list) == nil {
		items := list.Items
		for _, i := range items {
			if i["label"] == label {
				return true
			}
		}
	}
	return false
}
func TestClusterConcurrentUpdates(t *testing.T) {
	s := NewServer()
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 40; j++ {
				if err := s.cluster.update("openchoreo/updateResources", []byte(`{"Trait":["t"]}`)); err != nil {
					t.Error(err)
				}
				idx := s.clusterCompletionIndex("file:///test.yaml", "")
				_ = idx.NamesByKind()
				_ = idx.Snapshot("")
			}
		}()
	}
	wg.Wait()
}

func TestClusterWireNotifications(t *testing.T) {
	s := NewServer()
	left, right := net.Pipe()
	defer left.Close()
	defer right.Close()
	server := jsonrpc2.NewConn(context.Background(), jsonrpc2.NewBufferedStream(left, jsonrpc2.VSCodeObjectCodec{}), jsonrpc2.HandlerWithError(func(ctx context.Context, c *jsonrpc2.Conn, r *jsonrpc2.Request) (any, error) {
		_, _, _, err := s.server.Handler.Handle(&glsp.Context{Method: r.Method, Params: *r.Params})
		return nil, err
	}))
	defer server.Close()
	client := jsonrpc2.NewConn(context.Background(), jsonrpc2.NewBufferedStream(right, jsonrpc2.VSCodeObjectCodec{}), jsonrpc2.HandlerWithError(func(context.Context, *jsonrpc2.Conn, *jsonrpc2.Request) (any, error) { return nil, nil }))
	defer client.Close()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()
	for _, test := range []struct {
		method  string
		payload any
	}{
		{"openchoreo/updateResources", map[string]any{"Trait": []string{"live"}}},
		{"openchoreo/updateResourceSchemas", map[string]any{"Trait": map[string]any{"live": map[string]any{"parameters": map[string]any{"type": "object"}}}}},
	} {
		if err := client.Notify(ctx, test.method, test.payload); err != nil {
			t.Fatal(err)
		}
		if err := client.Call(ctx, test.method, test.payload, nil); err != nil {
			t.Fatal(err)
		}
	}
	// Calls use the same JSON-RPC method dispatch and provide a processing barrier.
	if len(s.clusterCompletionIndex("", "").names["Trait"]) != 1 {
		t.Fatal("wire name snapshot missing")
	}
	if err := client.Call(ctx, "openchoreo/updateResources", map[string]any{}, nil); err != nil {
		t.Fatal(err)
	}
	if len(s.clusterCompletionIndex("", "").names["Trait"]) != 0 {
		t.Fatal("wire clear failed")
	}
}

func TestClusterLocalDefinitionWins(t *testing.T) {
	s := NewServer()
	text := "apiVersion: openchoreo.dev/v1alpha1\nkind: ComponentType\nmetadata: {name: service}\nspec: {}\n"
	if err := s.indexer.UpdateFile("file:///tmp/local-type.yaml", []byte(text)); err != nil {
		t.Fatal(err)
	}
	if err := s.cluster.update("openchoreo/updateResourceSchemas", []byte(`{"ComponentType":{"service":{"parameters":{"type":"object","properties":{"remote":{"type":"string"}}}}}}`)); err != nil {
		t.Fatal(err)
	}
	idx := s.clusterCompletionIndex("file:///tmp/app.yaml", text)
	count := 0
	for _, d := range idx.docs {
		if d.Document.Kind == "ComponentType" && d.Document.Metadata.Name == "service" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("local definition was shadowed: %d", count)
	}
}

func TestClusterScopedReleases(t *testing.T) {
	s := NewServer()
	if err := s.cluster.update("openchoreo/updateResources", []byte(`{"ComponentRelease:app":["r1"],"ComponentRelease:other":["r2"]}`)); err != nil {
		t.Fatal(err)
	}
	text := "apiVersion: openchoreo.dev/v1alpha1\nkind: ReleaseBinding\nmetadata: {name: b}\nspec:\n  owner: {componentName: app}\n  releaseName: "
	items, err := completion.HandleWithIndex(text, 6, 16, s.clusterCompletionIndex("", text), "")
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(items)
	if !containsLabel(raw, "r1") || containsLabel(raw, "r2") {
		t.Fatalf("release scope: %s", raw)
	}
}
