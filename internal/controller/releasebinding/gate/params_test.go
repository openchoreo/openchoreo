// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package gate

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/runtime"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/template"
)

func bindingWithParams(raw string) openchoreov1alpha1.HookBinding {
	b := openchoreov1alpha1.HookBinding{Name: "b"}
	if raw != "" {
		b.Parameters = &runtime.RawExtension{Raw: []byte(raw)}
	}
	return b
}

// Workflow parameters are strings, so every binding value must reach the
// workflow as the text the pipeline author wrote: numbers without an exponent
// or trailing zeros, booleans as true/false, null as empty, and structured
// values as JSON a workflow step can parse back.
func TestBindingParametersStringifies(t *testing.T) {
	got, err := BindingParameters(bindingWithParams(
		`{"s":"x","b":false,"n":1.5,"i":3,"big":10000000000,"z":null,"o":{"k":"v"},"l":[1,"a"]}`))
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"s": "x", "b": "false", "n": "1.5", "i": "3", "big": "10000000000",
		"z": "", "o": `{"k":"v"}`, "l": `[1,"a"]`,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

// A binding with no parameters is the common case and must not be an error.
func TestBindingParametersEmpty(t *testing.T) {
	for name, b := range map[string]openchoreov1alpha1.HookBinding{
		"nil parameters": {Name: "b"},
		"empty raw":      {Name: "b", Parameters: &runtime.RawExtension{}},
	} {
		t.Run(name, func(t *testing.T) {
			got, err := BindingParameters(b)
			if err != nil {
				t.Fatal(err)
			}
			if got == nil || len(got) != 0 {
				t.Fatalf("got %#v, want an empty non-nil map", got)
			}
		})
	}
}

// Values that are not produced by JSON decoding (CEL results) take the typed
// branches; a value that cannot be rendered at all must fail loudly rather than
// be sent to the workflow as something arbitrary.
func TestStringify(t *testing.T) {
	for name, tc := range map[string]struct {
		in   any
		want string
	}{
		"int":    {in: 42, want: "42"},
		"int64":  {in: int64(-7), want: "-7"},
		"uint64": {in: uint64(18446744073709551615), want: "18446744073709551615"},
		"bool":   {in: true, want: "true"},
		"nil":    {in: nil, want: ""},
		"list":   {in: []any{"a", int64(1)}, want: `["a",1]`},
	} {
		t.Run(name, func(t *testing.T) {
			got, err := stringify(tc.in)
			if err != nil {
				t.Fatal(err)
			}
			if got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
	if _, err := stringify(make(chan int)); err == nil || !strings.Contains(err.Error(), "chan int") {
		t.Fatalf("an unrenderable value must error naming its type, got %v", err)
	}
}

// From expressions are CEL over the deployment context. Non-string results
// must be rendered the same way binding values are, so a hook author can use
// e.g. deployment.environment.isProduction directly; an expression that does
// not evaluate must stop the gate with an error naming the parameter instead
// of handing the workflow an empty value.
func TestResolveParametersFromExpressions(t *testing.T) {
	engine := template.NewEngine()
	inputs := BuildContext(ContextInput{
		Release: &openchoreov1alpha1.ComponentRelease{},
		Environment: &openchoreov1alpha1.Environment{
			Spec: openchoreov1alpha1.EnvironmentSpec{IsProduction: true},
		},
	})
	inputs["deployment"].(map[string]any)["environment"].(map[string]any)["name"] = "prod"

	hook := &openchoreov1alpha1.HookSpec{Parameters: []openchoreov1alpha1.HookParameter{
		{Name: "prod", From: "${deployment.environment.isProduction}"},
		{Name: "count", From: "${1 + 2}"},
		{Name: "list", From: "${[deployment.environment.name, 'x']}"},
		{Name: "map", From: "${{'env': deployment.environment.name}}"},
		{Name: "mixed", From: "env-${deployment.environment.name}"},
	}}
	got, err := ResolveParameters(context.Background(), engine, hook, bindingWithParams(""), inputs)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"prod": "true", "count": "3", "list": `["prod","x"]`, "map": `{"env":"prod"}`, "mixed": "env-prod",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}

	bad := &openchoreov1alpha1.HookSpec{Parameters: []openchoreov1alpha1.HookParameter{
		{Name: "missing", From: "${deployment.nosuch.field}"},
	}}
	_, err = ResolveParameters(context.Background(), engine, bad, bindingWithParams(""), inputs)
	if err == nil || !strings.Contains(err.Error(), `parameter "missing"`) {
		t.Fatalf("a failing From expression must error naming the parameter, got %v", err)
	}
}

// Each rejection names the rule that was broken, so the status condition tells
// the binding author what to fix. Asserting the message also proves the right
// branch rejected the binding, not an unrelated one.
func TestResolveParametersRejections(t *testing.T) {
	engine := template.NewEngine()
	inputs := map[string]any{"deployment": map[string]any{"workload": map[string]any{"image": "img"}}}
	hook := &openchoreov1alpha1.HookSpec{Parameters: []openchoreov1alpha1.HookParameter{
		{Name: "fixed", Value: strp("CRITICAL")},
		{Name: "image", From: "${deployment.workload.image}"},
		{Name: "ticket", Required: true},
	}}
	for name, tc := range map[string]struct {
		raw  string
		want string
	}{
		"fixed value":          {raw: `{"fixed":"LOW","ticket":"x"}`, want: `parameter "fixed" is fixed by the hook and cannot be set by binding "b"`},
		"non-overridable from": {raw: `{"image":"evil","ticket":"x"}`, want: `parameter "image" is computed by the hook and is not overridable`},
		"required missing":     {raw: `{}`, want: `parameter "ticket" is required but binding "b" does not supply it`},
		// Sorted, so the message is identical on every reconcile and the
		// condition is not rewritten (which would re-trigger the reconcile).
		"undeclared keys sorted": {raw: `{"ticket":"x","zeta":1,"alpha":2}`, want: `binding "b" sets parameters the hook does not declare: [alpha zeta]`},
		"not an object":          {raw: `"str"`, want: `binding "b": parameters must be a JSON object`},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := ResolveParameters(context.Background(), engine, hook, bindingWithParams(tc.raw), inputs)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("got %v, want an error containing %q", err, tc.want)
			}
		})
	}

	// The same hook resolves once the binding follows the rules, so the
	// rejections above are caused by the offending keys alone.
	got, err := ResolveParameters(context.Background(), engine, hook, bindingWithParams(`{"ticket":"x"}`), inputs)
	if err != nil {
		t.Fatal(err)
	}
	if want := map[string]string{"fixed": "CRITICAL", "image": "img", "ticket": "x"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}
