// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package celvalidator

import (
	"sync"
	"sync/atomic"
	"testing"

	"github.com/google/cel-go/cel"
	apiextschema "k8s.io/apiextensions-apiserver/pkg/apiserver/schema"
)

func TestHashStructuralDeterministic(t *testing.T) {
	a := objectStructural(map[string]apiextschema.Structural{
		"b": stringProperty(),
		"a": integerProperty(),
	})
	b := objectStructural(map[string]apiextschema.Structural{
		"a": integerProperty(),
		"b": stringProperty(),
	})
	if hashStructural(a) != hashStructural(b) {
		t.Fatal("structurally equal schemas must hash equal")
	}
	nilHash := hashStructural(nil)
	if nilHash != hashStructural(nil) {
		t.Fatal("nil schema hash must be stable")
	}
}

func TestEnvForCachesBuilds(t *testing.T) {
	pair := SchemaPair{Parameters: objectStructural(nil)}
	e1, err := envFor(KindComponentType, pair)
	if err != nil {
		t.Fatalf("envFor: %v", err)
	}
	e2, err := envFor(KindComponentType, pair)
	if err != nil {
		t.Fatalf("envFor: %v", err)
	}
	if e1.env != e2.env {
		t.Fatal("repeated envFor must return the same *cel.Env")
	}
	if e1.provider == nil {
		t.Fatal("component env must carry a DeclTypeProvider for forEach")
	}
}

func TestEnvForKeyedBySchema(t *testing.T) {
	a := SchemaPair{Parameters: objectStructural(nil)}
	b := SchemaPair{Parameters: objectStructural(map[string]apiextschema.Structural{
		"x": stringProperty(),
	})}
	e1, err := envFor(KindComponentType, a)
	if err != nil {
		t.Fatalf("envFor: %v", err)
	}
	e2, err := envFor(KindComponentType, b)
	if err != nil {
		t.Fatalf("envFor: %v", err)
	}
	if e1.env == e2.env {
		t.Fatal("different parameter schemas must build different envs")
	}
}

func TestEnvForKeyedByKind(t *testing.T) {
	pair := SchemaPair{Parameters: objectStructural(nil)}
	comp, err := envFor(KindComponentType, pair)
	if err != nil {
		t.Fatalf("envFor component: %v", err)
	}
	trait, err := envFor(KindTrait, pair)
	if err != nil {
		t.Fatalf("envFor trait: %v", err)
	}
	if comp.env == trait.env {
		t.Fatal("different kinds must build different envs")
	}
}

func TestEnvForResourceAndWorkflowProviderNil(t *testing.T) {
	pair := SchemaPair{Parameters: objectStructural(nil)}
	res, err := envFor(KindResourceType, pair)
	if err != nil {
		t.Fatalf("envFor resource: %v", err)
	}
	if res.provider != nil {
		t.Fatal("resource env must not carry a DeclTypeProvider")
	}
	wf, err := envFor(KindWorkflow, pair)
	if err != nil {
		t.Fatalf("envFor workflow: %v", err)
	}
	if wf.provider != nil {
		t.Fatal("workflow env must not carry a DeclTypeProvider")
	}
}

func TestEnvForUnknownKind(t *testing.T) {
	if _, err := envFor("Foo", SchemaPair{}); err == nil {
		t.Fatal("unknown kind must error")
	}
}

// TestEnvCacheMemoizes verifies the plain fast path: a second get returns the
// mutated pointer with no extra build.
func TestEnvCacheMemoizes(t *testing.T) {
	c := newEnvCache()
	key := cacheKey{kind: "c", paramsHash: hashStructural(nil), envConfigsHash: hashStructural(nil)}
	var builds atomic.Int32

	build := func() (cachedEnv, error) {
		builds.Add(1)
		env, err := cel.NewEnv()
		return cachedEnv{env: env}, err
	}

	first, err := c.get(key, build)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	second, err := c.get(key, build)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if first.env != second.env {
		t.Fatal("second get must return the memoized *cel.Env")
	}
	if got := builds.Load(); got != 1 {
		t.Fatalf("expected exactly 1 build, got %d", got)
	}
}

// TestEnvCacheConcurrent verifies concurrent misses on the same key collapse
// into a single retained env that every caller shares, and no build happens
// once the cache is warm. Run with -race.
func TestEnvCacheConcurrent(t *testing.T) {
	c := newEnvCache()
	key := cacheKey{kind: "c", paramsHash: hashStructural(nil), envConfigsHash: hashStructural(nil)}
	var builds atomic.Int32

	build := func() (cachedEnv, error) {
		builds.Add(1)
		env, err := cel.NewEnv()
		return cachedEnv{env: env}, err
	}

	const n = 16
	envs := make([]cachedEnv, n)
	var wg sync.WaitGroup
	for i := range n {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			e, err := c.get(key, build)
			if err != nil {
				t.Errorf("get: %v", err)
				return
			}
			envs[i] = e
		}(i)
	}
	wg.Wait()

	if got := builds.Load(); got < 1 {
		t.Fatalf("expected at least 1 build, got %d", got)
	}
	canonical := envs[0]
	for i := 1; i < len(envs); i++ {
		if envs[i].env != canonical.env {
			t.Fatal("all concurrent callers must share one *cel.Env")
		}
	}

	warmBuilds := builds.Load()
	warm, err := c.get(key, build)
	if err != nil {
		t.Fatalf("warm get: %v", err)
	}
	if warm.env != canonical.env {
		t.Fatal("warm get must return the memoized *cel.Env")
	}
	if got := builds.Load(); got != warmBuilds {
		t.Fatalf("warm get must not rebuild (builds went %d -> %d)", warmBuilds, got)
	}
}

// TestEnvCacheErrorsNotCached verifies a failed build is forgotten and retried
// on the next call rather than poisoned.
func TestEnvCacheErrorsNotCached(t *testing.T) {
	c := newEnvCache()
	key := cacheKey{kind: "c", paramsHash: hashStructural(nil), envConfigsHash: hashStructural(nil)}
	var calls atomic.Int32

	build := func() (cachedEnv, error) {
		calls.Add(1)
		return cachedEnv{}, &fatalError{"boom"}
	}

	if _, err := c.get(key, build); err == nil {
		t.Fatal("first build must fail")
	}
	if _, err := c.get(key, build); err == nil {
		t.Fatal("second build must be retried and fail again")
	}
	if got := calls.Load(); got != 2 {
		t.Fatalf("expected 2 build attempts, got %d", got)
	}
}

type fatalError struct{ msg string }

func (e *fatalError) Error() string { return e.msg }

// objectStructural / stringProperty / integerProperty build the smallest JSON
// structurals the hasher cares about. Type is promoted from the embedded
// Generic, so it must be set through that key in a literal.
func objectStructural(props map[string]apiextschema.Structural) *apiextschema.Structural {
	return &apiextschema.Structural{
		Generic:    apiextschema.Generic{Type: "object"},
		Properties: props,
	}
}

func stringProperty() apiextschema.Structural {
	return apiextschema.Structural{Generic: apiextschema.Generic{Type: "string"}}
}

func integerProperty() apiextschema.Structural {
	return apiextschema.Structural{Generic: apiextschema.Generic{Type: "integer"}}
}
