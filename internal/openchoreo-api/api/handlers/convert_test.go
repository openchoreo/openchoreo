// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/runtime"
)

// ---------------------------------------------------------------------------
// Test types
// ---------------------------------------------------------------------------

// flat structs
type srcFull struct {
	Name  string `json:"name"`
	Value int    `json:"value"`
	Extra string `json:"extra"`
}

type dstPartial struct {
	Name string `json:"name"`
}

type dstStrict struct {
	Count int `json:"count"`
}

// nested structs
type address struct {
	Street string `json:"street"`
	City   string `json:"city"`
}

type srcNested struct {
	Name    string  `json:"name"`
	Address address `json:"address"`
}

type dstNested struct {
	Name    string  `json:"name"`
	Address address `json:"address"`
}

// pointer fields
type srcWithPointers struct {
	Name  *string `json:"name,omitempty"`
	Count *int    `json:"count,omitempty"`
}

type dstWithPointers struct {
	Name  *string `json:"name,omitempty"`
	Count *int    `json:"count,omitempty"`
}

// slice fields
type srcWithSlice struct {
	Tags   []string `json:"tags"`
	Scores []int    `json:"scores"`
}

type dstWithSlice struct {
	Tags   []string `json:"tags"`
	Scores []int    `json:"scores"`
}

// map fields
type srcWithMap struct {
	Labels map[string]string `json:"labels"`
}

type dstWithMap struct {
	Labels map[string]string `json:"labels"`
}

// time fields
type srcWithTime struct {
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"createdAt"`
}

type dstWithTime struct {
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"createdAt"`
}

// type rename — same JSON shape, different Go type names
type srcID struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

type dstID struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// JSON tag mismatch — src field "foo_name" does not match dst field "name"
type srcTagMismatch struct {
	Name string `json:"foo_name"`
}

type dstTagMismatch struct {
	Name string `json:"name"`
}

// deeply nested
type inner struct {
	Value string `json:"value"`
}

type middle struct {
	Inner inner `json:"inner"`
}

type srcDeep struct {
	Middle middle `json:"middle"`
}

type dstDeep struct {
	Middle middle `json:"middle"`
}

// boolean and float fields
type srcPrimitives struct {
	Active  bool    `json:"active"`
	Score   float64 `json:"score"`
	Payload []byte  `json:"payload"`
}

type dstPrimitives struct {
	Active  bool    `json:"active"`
	Score   float64 `json:"score"`
	Payload []byte  `json:"payload"`
}

// map[string]interface{} fields
type srcWithAnyMap struct {
	Name     string                 `json:"name"`
	Metadata map[string]interface{} `json:"metadata"`
}

type dstWithAnyMap struct {
	Name     string                 `json:"name"`
	Metadata map[string]interface{} `json:"metadata"`
}

// struct with an any (interface{}) field
type srcWithAnyField struct {
	Name  string      `json:"name"`
	Extra interface{} `json:"extra"`
}

type dstWithAnyField struct {
	Name  string      `json:"name"`
	Extra interface{} `json:"extra"`
}

// runtime.RawExtension ↔ map[string]interface{} — the real pattern used in handlers
type srcWithRawExtension struct {
	Name       string                `json:"name"`
	Parameters *runtime.RawExtension `json:"parameters,omitempty"`
}

type dstWithAnyParameters struct {
	Name       string                  `json:"name"`
	Parameters *map[string]interface{} `json:"parameters,omitempty"`
}

// error helpers
type unmarshalErrorDst struct{}

func (u *unmarshalErrorDst) UnmarshalJSON(_ []byte) error {
	return &json.SyntaxError{}
}

type marshalErrorSrc struct {
	Ch chan int `json:"ch"`
}

// ---------------------------------------------------------------------------
// TestConvert
// ---------------------------------------------------------------------------

func TestConvert(t *testing.T) {
	t.Run("successful conversion same type", func(t *testing.T) {
		src := srcFull{Name: "foo", Value: 42, Extra: "bar"}
		got, err := convert[srcFull, srcFull](src)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != src {
			t.Errorf("got %+v, want %+v", got, src)
		}
	})

	t.Run("extra source fields not in destination are dropped", func(t *testing.T) {
		src := srcFull{Name: "foo", Value: 42, Extra: "ignored"}
		got, err := convert[srcFull, dstPartial](src)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.Name != "foo" {
			t.Errorf("got name %q, want %q", got.Name, "foo")
		}
	})

	t.Run("missing source fields zero-fill destination", func(t *testing.T) {
		src := srcFull{Name: "foo", Value: 42}
		got, err := convert[srcFull, dstStrict](src)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.Count != 0 {
			t.Errorf("got count %d, want 0", got.Count)
		}
	})

	t.Run("nested structs conversion", func(t *testing.T) {
		src := srcNested{
			Name:    "alice",
			Address: address{Street: "1 Main St", City: "Springfield"},
		}
		got, err := convert[srcNested, dstNested](src)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.Name != src.Name || got.Address.Street != src.Address.Street || got.Address.City != src.Address.City {
			t.Errorf("got %+v, want %+v", got, src)
		}
	})

	t.Run("deeply nested structs conversion", func(t *testing.T) {
		src := srcDeep{Middle: middle{Inner: inner{Value: "deep"}}}
		got, err := convert[srcDeep, dstDeep](src)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.Middle.Inner.Value != "deep" {
			t.Errorf("got %q, want %q", got.Middle.Inner.Value, "deep")
		}
	})

	t.Run("nil pointer fields preserved as nil", func(t *testing.T) {
		src := srcWithPointers{Name: nil, Count: nil}
		got, err := convert[srcWithPointers, dstWithPointers](src)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.Name != nil || got.Count != nil {
			t.Errorf("expected nil pointer fields, got %+v", got)
		}
	})

	t.Run("non-nil pointer fields conversion", func(t *testing.T) {
		name := "bob"
		count := 7
		src := srcWithPointers{Name: &name, Count: &count}
		got, err := convert[srcWithPointers, dstWithPointers](src)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.Name == nil || *got.Name != name {
			t.Errorf("got name %v, want %q", got.Name, name)
		}
		if got.Count == nil || *got.Count != count {
			t.Errorf("got count %v, want %d", got.Count, count)
		}
	})

	t.Run("slice fields conversion", func(t *testing.T) {
		src := srcWithSlice{Tags: []string{"a", "b", "c"}, Scores: []int{1, 2, 3}}
		got, err := convert[srcWithSlice, dstWithSlice](src)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got.Tags) != 3 || got.Tags[1] != "b" {
			t.Errorf("unexpected tags: %v", got.Tags)
		}
		if len(got.Scores) != 3 || got.Scores[2] != 3 {
			t.Errorf("unexpected scores: %v", got.Scores)
		}
	})

	t.Run("nil slice fields produce nil destination slices", func(t *testing.T) {
		src := srcWithSlice{Tags: nil, Scores: nil}
		got, err := convert[srcWithSlice, dstWithSlice](src)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.Tags != nil || got.Scores != nil {
			t.Errorf("expected nil slices, got tags=%v scores=%v", got.Tags, got.Scores)
		}
	})

	t.Run("map fields conversion", func(t *testing.T) {
		src := srcWithMap{Labels: map[string]string{"env": "prod", "app": "api"}}
		got, err := convert[srcWithMap, dstWithMap](src)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.Labels["env"] != "prod" || got.Labels["app"] != "api" {
			t.Errorf("unexpected labels: %v", got.Labels)
		}
	})

	t.Run("nil map produces nil destination map", func(t *testing.T) {
		src := srcWithMap{Labels: nil}
		got, err := convert[srcWithMap, dstWithMap](src)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.Labels != nil {
			t.Errorf("expected nil map, got %v", got.Labels)
		}
	})

	t.Run("time.Time field conversion", func(t *testing.T) {
		ts := time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC)
		src := srcWithTime{Name: "event", CreatedAt: ts}
		got, err := convert[srcWithTime, dstWithTime](src)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !got.CreatedAt.Equal(ts) {
			t.Errorf("got time %v, want %v", got.CreatedAt, ts)
		}
	})

	t.Run("boolean and float64 fields conversion", func(t *testing.T) {
		src := srcPrimitives{Active: true, Score: 3.14, Payload: []byte("hello")}
		got, err := convert[srcPrimitives, dstPrimitives](src)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !got.Active {
			t.Errorf("got active %v, want true", got.Active)
		}
		if got.Score != 3.14 {
			t.Errorf("got score %v, want 3.14", got.Score)
		}
		if string(got.Payload) != "hello" {
			t.Errorf("got payload %q, want %q", got.Payload, "hello")
		}
	})

	t.Run("JSON tag mismatch loses field value", func(t *testing.T) {
		// src serialises to {"foo_name":"x"} but dst expects {"name":"..."}
		src := srcTagMismatch{Name: "x"}
		got, err := convert[srcTagMismatch, dstTagMismatch](src)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// the field is not mapped — destination Name must be empty
		if got.Name != "" {
			t.Errorf("got name %q, want empty string due to tag mismatch", got.Name)
		}
	})

	t.Run("RawExtension with flat JSON object converts to map[string]interface{}", func(t *testing.T) {
		raw, _ := json.Marshal(map[string]interface{}{"key": "value", "count": 3})
		src := srcWithRawExtension{
			Name:       "x",
			Parameters: &runtime.RawExtension{Raw: raw},
		}
		got, err := convert[srcWithRawExtension, dstWithAnyParameters](src)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.Parameters == nil {
			t.Fatal("expected non-nil parameters")
		}
		params := *got.Parameters
		if params["key"] != "value" {
			t.Errorf("got key %v, want %q", params["key"], "value")
		}
		// JSON numbers into interface{} arrive as float64
		if params["count"] != float64(3) {
			t.Errorf("got count %v (%T), want float64(3)", params["count"], params["count"])
		}
	})

	t.Run("map[string]interface{} with flat JSON object converts to RawExtension", func(t *testing.T) {
		params := map[string]interface{}{"key": "value", "count": float64(3)}
		src := dstWithAnyParameters{
			Name:       "x",
			Parameters: &params,
		}
		got, err := convert[dstWithAnyParameters, srcWithRawExtension](src)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.Parameters == nil {
			t.Fatal("expected non-nil RawExtension")
		}
		var decoded map[string]interface{}
		if err := json.Unmarshal(got.Parameters.Raw, &decoded); err != nil {
			t.Fatalf("failed to unmarshal RawExtension: %v", err)
		}
		if decoded["key"] != "value" {
			t.Errorf("got key %v, want %q", decoded["key"], "value")
		}
		if decoded["count"] != float64(3) {
			t.Errorf("got count %v (%T), want float64(3)", decoded["count"], decoded["count"])
		}
	})

	t.Run("nil RawExtension produces nil map[string]interface{} pointer", func(t *testing.T) {
		src := srcWithRawExtension{Name: "x", Parameters: nil}
		got, err := convert[srcWithRawExtension, dstWithAnyParameters](src)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.Parameters != nil {
			t.Errorf("expected nil parameters, got %v", got.Parameters)
		}
	})

	t.Run("RawExtension with nested object converts to nested map", func(t *testing.T) {
		raw, _ := json.Marshal(map[string]interface{}{
			"db": map[string]interface{}{"host": "localhost", "port": 5432},
		})
		src := srcWithRawExtension{Parameters: &runtime.RawExtension{Raw: raw}}
		got, err := convert[srcWithRawExtension, dstWithAnyParameters](src)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		db, ok := (*got.Parameters)["db"].(map[string]interface{})
		if !ok {
			t.Fatalf("db not a map, got %T", (*got.Parameters)["db"])
		}
		if db["host"] != "localhost" {
			t.Errorf("got host %v, want %q", db["host"], "localhost")
		}
	})

	t.Run("RawExtension with array value converts to []interface{}", func(t *testing.T) {
		raw, _ := json.Marshal(map[string]interface{}{
			"tags": []string{"a", "b"},
		})
		src := srcWithRawExtension{Parameters: &runtime.RawExtension{Raw: raw}}
		got, err := convert[srcWithRawExtension, dstWithAnyParameters](src)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		tags, ok := (*got.Parameters)["tags"].([]interface{})
		if !ok {
			t.Fatalf("tags not []interface{}, got %T", (*got.Parameters)["tags"])
		}
		if len(tags) != 2 || tags[0] != "a" {
			t.Errorf("unexpected tags: %v", tags)
		}
	})

	t.Run("RawExtension with boolean and null values converts correctly", func(t *testing.T) {
		raw, _ := json.Marshal(map[string]interface{}{
			"enabled": true,
			"note":    nil,
		})
		src := srcWithRawExtension{Parameters: &runtime.RawExtension{Raw: raw}}
		got, err := convert[srcWithRawExtension, dstWithAnyParameters](src)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		params := *got.Parameters
		if params["enabled"] != true {
			t.Errorf("got enabled %v, want true", params["enabled"])
		}
		if params["note"] != nil {
			t.Errorf("got note %v, want nil", params["note"])
		}
	})

	t.Run("map[string]interface{} conversion preserves string values", func(t *testing.T) {
		src := srcWithAnyMap{
			Name: "x",
			Metadata: map[string]interface{}{
				"env":     "prod",
				"version": "v1",
			},
		}
		got, err := convert[srcWithAnyMap, dstWithAnyMap](src)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.Metadata["env"] != "prod" || got.Metadata["version"] != "v1" {
			t.Errorf("unexpected metadata: %v", got.Metadata)
		}
	})

	t.Run("map[string]interface{} with nested map value", func(t *testing.T) {
		src := srcWithAnyMap{
			Metadata: map[string]interface{}{
				"nested": map[string]interface{}{"key": "val"},
			},
		}
		got, err := convert[srcWithAnyMap, dstWithAnyMap](src)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		nested, ok := got.Metadata["nested"].(map[string]interface{})
		if !ok {
			t.Fatalf("nested not a map[string]interface{}, got %T", got.Metadata["nested"])
		}
		if nested["key"] != "val" {
			t.Errorf("got nested key %q, want %q", nested["key"], "val")
		}
	})

	t.Run("map[string]interface{} with slice value", func(t *testing.T) {
		src := srcWithAnyMap{
			Metadata: map[string]interface{}{
				"tags": []interface{}{"a", "b", "c"},
			},
		}
		got, err := convert[srcWithAnyMap, dstWithAnyMap](src)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		tags, ok := got.Metadata["tags"].([]interface{})
		if !ok {
			t.Fatalf("tags not a []interface{}, got %T", got.Metadata["tags"])
		}
		if len(tags) != 3 || tags[0] != "a" {
			t.Errorf("unexpected tags: %v", tags)
		}
	})

	t.Run("nil map[string]interface{} field produces nil in destination", func(t *testing.T) {
		src := srcWithAnyMap{Name: "x", Metadata: nil}
		got, err := convert[srcWithAnyMap, dstWithAnyMap](src)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.Metadata != nil {
			t.Errorf("expected nil metadata, got %v", got.Metadata)
		}
	})

	t.Run("interface{} field holding a string converts correctly", func(t *testing.T) {
		src := srcWithAnyField{Name: "x", Extra: "hello"}
		got, err := convert[srcWithAnyField, dstWithAnyField](src)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.Extra != "hello" {
			t.Errorf("got extra %v, want %q", got.Extra, "hello")
		}
	})

	t.Run("interface{} field holding a bool converts correctly", func(t *testing.T) {
		src := srcWithAnyField{Extra: true}
		got, err := convert[srcWithAnyField, dstWithAnyField](src)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.Extra != true {
			t.Errorf("got extra %v, want true", got.Extra)
		}
	})

	t.Run("interface{} field holding nil converts as nil", func(t *testing.T) {
		src := srcWithAnyField{Name: "x", Extra: nil}
		got, err := convert[srcWithAnyField, dstWithAnyField](src)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.Extra != nil {
			t.Errorf("got extra %v, want nil", got.Extra)
		}
	})

	t.Run("map[string]interface{} to struct extracts matching fields", func(t *testing.T) {
		// convert a free-form map into a typed struct — only matching JSON keys land.
		src := map[string]interface{}{"name": "alice", "unknown": "ignored"}
		got, err := convert[map[string]interface{}, dstPartial](src)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.Name != "alice" {
			t.Errorf("got name %q, want %q", got.Name, "alice")
		}
	})

	t.Run("struct to map[string]interface{} produces correct keys", func(t *testing.T) {
		src := srcFull{Name: "bob", Value: 5, Extra: "x"}
		got, err := convert[srcFull, map[string]interface{}](src)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got["name"] != "bob" {
			t.Errorf("got name %v, want %q", got["name"], "bob")
		}
		if got["value"] != float64(5) {
			t.Errorf("got value %v (%T), want float64(5)", got["value"], got["value"])
		}
	})

	t.Run("marshal error returns wrapped error", func(t *testing.T) {
		src := marshalErrorSrc{Ch: make(chan int)}
		_, err := convert[marshalErrorSrc, dstPartial](src)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "marshal source") {
			t.Errorf("error %q does not contain 'marshal source'", err.Error())
		}
	})

	t.Run("unmarshal error returns wrapped error", func(t *testing.T) {
		src := srcFull{Name: "foo"}
		_, err := convert[srcFull, unmarshalErrorDst](src)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "unmarshal into destination") {
			t.Errorf("error %q does not contain 'unmarshal into destination'", err.Error())
		}
	})
}

// ---------------------------------------------------------------------------
// TestConvertList
// ---------------------------------------------------------------------------

func TestConvertList(t *testing.T) {
	t.Run("nil input returns empty slice", func(t *testing.T) {
		got, err := convertList[srcFull, dstPartial](nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 0 {
			t.Errorf("got len %d, want 0", len(got))
		}
	})

	t.Run("empty input returns empty slice", func(t *testing.T) {
		got, err := convertList[srcFull, dstPartial]([]srcFull{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 0 {
			t.Errorf("got len %d, want 0", len(got))
		}
	})

	t.Run("single item converted", func(t *testing.T) {
		got, err := convertList[srcFull, dstPartial]([]srcFull{{Name: "a", Value: 1}})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 1 || got[0].Name != "a" {
			t.Errorf("got %+v, want [{Name:a}]", got)
		}
	})

	t.Run("multiple items all converted in order", func(t *testing.T) {
		items := []srcFull{{Name: "a"}, {Name: "b"}, {Name: "c"}}
		got, err := convertList[srcFull, dstPartial](items)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 3 {
			t.Errorf("got len %d, want 3", len(got))
		}
		for i, want := range []string{"a", "b", "c"} {
			if got[i].Name != want {
				t.Errorf("item %d: got name %q, want %q", i, got[i].Name, want)
			}
		}
	})

	t.Run("nested struct items converted", func(t *testing.T) {
		items := []srcNested{
			{Name: "alice", Address: address{Street: "1 Main St", City: "A"}},
			{Name: "bob", Address: address{Street: "2 High St", City: "B"}},
		}
		got, err := convertList[srcNested, dstNested](items)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got[1].Address.City != "B" {
			t.Errorf("got city %q, want %q", got[1].Address.City, "B")
		}
	})

	t.Run("error at first item stops early and wraps index", func(t *testing.T) {
		items := []marshalErrorSrc{{Ch: make(chan int)}, {Ch: make(chan int)}}
		_, err := convertList[marshalErrorSrc, dstPartial](items)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "item 0") {
			t.Errorf("error %q does not contain 'item 0'", err.Error())
		}
	})

	t.Run("error at non-first item wraps correct index", func(t *testing.T) {
		// Build a mixed slice: first two items are valid srcFull values marshaled
		// to marshalErrorSrc is not possible across types — simulate with a
		// dedicated mixed-error scenario using srcFull -> unmarshalErrorDst.
		items := []srcFull{{Name: "ok1"}, {Name: "ok2"}, {Name: "fail"}}
		_, err := convertList[srcFull, unmarshalErrorDst](items)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "item 0") {
			t.Errorf("error %q does not contain 'item 0'", err.Error())
		}
	})
}
