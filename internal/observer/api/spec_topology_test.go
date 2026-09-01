// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package api_test

import (
	"sort"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"

	"github.com/openchoreo/openchoreo/internal/observer/api/gen"
	"github.com/openchoreo/openchoreo/internal/observer/api/internalgen"
)

// operationIDs returns every operationId in a loaded spec, sorted.
func operationIDs(spec *openapi3.T) []string {
	ids := make([]string, 0, spec.Paths.Len())
	for _, item := range spec.Paths.Map() {
		for _, op := range item.Operations() {
			ids = append(ids, op.OperationID)
		}
	}
	sort.Strings(ids)
	return ids
}

// TestSpecTopology pins the public/internal split. Each spec must embed and
// resolve only its own operations, and together they must still cover every
// operation the observer serves.
//
// This also proves the internal spec's external $refs into observer-api.yaml
// resolve at runtime -- internalgen.GetSwagger() composes gen.PathToRawSpec to
// do it, so a broken mapping fails here rather than at first request.
func TestSpecTopology(t *testing.T) {
	pub, err := gen.GetSwagger()
	if err != nil {
		t.Fatalf("public GetSwagger: %v", err)
	}
	internal, err := internalgen.GetSwagger()
	if err != nil {
		t.Fatalf("internal GetSwagger: %v", err)
	}

	pubOps := operationIDs(pub)
	internalOps := operationIDs(internal)

	t.Logf("public ops (%d): %v", len(pubOps), pubOps)

	// oapi-codegen normalizes operationIds to their Go names in the embedded
	// spec, so these are PascalCase even though the YAML says createAlertRule.
	wantInternal := []string{
		"CreateAlertRule",
		"DeleteAlertRule",
		"GetAlertRule",
		"HandleAlertWebhook",
		"UpdateAlertRule",
	}

	if len(internalOps) != len(wantInternal) {
		t.Fatalf("internal spec: got %d operations %v, want %d %v",
			len(internalOps), internalOps, len(wantInternal), wantInternal)
	}
	for i, want := range wantInternal {
		if internalOps[i] != want {
			t.Errorf("internal spec op[%d] = %q, want %q", i, internalOps[i], want)
		}
	}

	// The internal operations must have left the public spec entirely.
	internalSet := make(map[string]bool, len(wantInternal))
	for _, id := range wantInternal {
		internalSet[id] = true
	}
	for _, id := range pubOps {
		if internalSet[id] {
			t.Errorf("operation %q is in BOTH specs; it must live only in the internal spec", id)
		}
	}

	if got, want := len(pubOps)+len(internalOps), 18; got != want {
		t.Errorf("total operations across both specs = %d, want %d (public=%d internal=%d)",
			got, want, len(pubOps), len(internalOps))
	}
}

// TestPublicSpecHealthIsTheOnlyUnauthenticatedOperation guards the one line that
// keeps Kubernetes liveness probes working.
//
// Before the migration, /health was registered outside the JWT-protected route
// group by hand (cmd/observer/main.go). After it, /health is inside the generated
// router and its publicness comes entirely from `security: []` on that operation
// in observer-api.yaml: auth.OpenAPIAuth treats an operation as public only when
// the generated wrapper set no scopes context key, which happens only for that
// override.
//
// Delete that one YAML line and every probe starts getting a 401 -- from a spec
// edit, with no code change and nothing else to notice it. Conversely, adding
// `security: []` to any other public operation silently un-authenticates it.
// Both directions fail here.
func TestPublicSpecHealthIsTheOnlyUnauthenticatedOperation(t *testing.T) {
	pub, err := gen.GetSwagger()
	if err != nil {
		t.Fatalf("public GetSwagger: %v", err)
	}

	if len(pub.Security) == 0 {
		t.Error("public spec has no global security requirement; every operation " +
			"without an explicit override would become unauthenticated")
	}

	var unauthenticated []string
	for path, item := range pub.Paths.Map() {
		for method, op := range item.Operations() {
			// A non-nil but empty Security is the `security: []` override that
			// makes an operation public. Nil means "inherit the global one".
			if op.Security != nil && len(*op.Security) == 0 {
				unauthenticated = append(unauthenticated, method+" "+path+" ("+op.OperationID+")")
			}
		}
	}
	sort.Strings(unauthenticated)

	want := []string{"GET /health (Health)"}
	if len(unauthenticated) != len(want) || unauthenticated[0] != want[0] {
		t.Errorf("unauthenticated operations in the public spec = %v, want %v",
			unauthenticated, want)
	}
}

// TestInternalSpecDeclaresNoSecurity is the guard that keeps this migration
// auth-neutral, and the reason it is safe.
//
// The observer's internal port has no JWT layer, and the ObservabilityAlertRule
// controller sends no Authorization header. Because the internal spec declares
// no security scheme, the generated wrapper sets no scopes context key, so an
// auth middleware wrapped in auth.OpenAPIAuth would treat every internal
// operation as public.
//
// Adding a security scheme here without also giving that controller a token
// source would break alert rule reconciliation. This test makes that a
// deliberate, visible decision rather than a silent one.
func TestInternalSpecDeclaresNoSecurity(t *testing.T) {
	internal, err := internalgen.GetSwagger()
	if err != nil {
		t.Fatalf("internal GetSwagger: %v", err)
	}

	if len(internal.Security) != 0 {
		t.Errorf("internal spec declares a global security requirement (%v); "+
			"enforcing auth on the internal port requires a token source in "+
			"internal/controller/observabilityalertrule first", internal.Security)
	}

	for path, item := range internal.Paths.Map() {
		for method, op := range item.Operations() {
			if op.Security != nil && len(*op.Security) != 0 {
				t.Errorf("%s %s declares a security requirement; see this test's doc comment",
					method, path)
			}
		}
	}
}
