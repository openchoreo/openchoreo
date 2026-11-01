// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package celvalidator

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/google/cel-go/cel"
	"golang.org/x/sync/singleflight"
	apiextschema "k8s.io/apiextensions-apiserver/pkg/apiserver/schema"
	apiservercel "k8s.io/apiserver/pkg/cel"
)

// cachedEnv is the memoized product of a schema-aware env build.
//
// provider is only non-nil for kinds whose templates use forEach (ComponentType
// and Trait, where extendEnvWithForEach resolves the loop variable through the
// DeclTypeProvider). Resource and Workflow templates do not need it, matching
// the signature the builders already expose.
type cachedEnv struct {
	env      *cel.Env
	provider *apiservercel.DeclTypeProvider
}

// cacheKey identifies one env build: the kind plus the content hash of the two
// schema sections that type the parameters / environmentConfigs variables.
type cacheKey struct {
	kind           string
	paramsHash     [32]byte
	envConfigsHash [32]byte
}

func (k cacheKey) String() string {
	return fmt.Sprintf("%s/%x/%x", k.kind, k.paramsHash, k.envConfigsHash)
}

// hashStructural returns a content hash of a structural schema, or a fixed hash
// for a nil schema. json.Marshal is deterministic for these structs (its map
// fields sort by key), so equal schemas hash equal regardless of how the YAML
// that produced them was formatted.
func hashStructural(s *apiextschema.Structural) [32]byte {
	if s == nil {
		return sha256.Sum256([]byte("nil"))
	}
	raw, err := json.Marshal(s)
	if err != nil {
		// A structural schema is always json-marshalable; hash its type name as
		// a stable fallback rather than failing a lint run on it.
		return sha256.Sum256([]byte(s.Type))
	}
	return sha256.Sum256(raw)
}

// envCache memoizes env builds keyed by kind + schema content. It is
// process-internal only (no disk, per the bootstrap no-$HOME-writes contract)
// and is safe for concurrent use: singleflight collapses simultaneous misses on
// the same key into a single build, and every caller receives the same *cel.Env.
//
// Only successful builds are retained. A failed build is forgotten so a later
// document (or a later point in a long-lived LSP session) can retry.
type envCache struct {
	mu      sync.Mutex
	entries map[cacheKey]*cachedEnv
	group   singleflight.Group
}

func newEnvCache() *envCache {
	return &envCache{entries: make(map[cacheKey]*cachedEnv)}
}

// get returns the cached env for key, building it with build on a miss. err is
// never cached.
//
// singleflight collapses simultaneous misses on the same key into one build.
// Because singleflight forgets a key as soon as its call completes, a caller
// can straddle the completion boundary and trigger a second build; the closure
// therefore re-checks the map and prefers the entry a previous call already
// stored, so every caller ends up with the same *cel.Env.
func (c *envCache) get(key cacheKey, build func() (cachedEnv, error)) (cachedEnv, error) {
	c.mu.Lock()
	if entry, ok := c.entries[key]; ok {
		c.mu.Unlock()
		return *entry, nil
	}
	c.mu.Unlock()

	val, err, _ := c.group.Do(key.String(), func() (any, error) {
		entry, err := build()
		if err != nil {
			// Do not retain failures. singleflight currently forgets completed
			// keys, but be explicit so a future version that retains results
			// cannot serve cached errors.
			c.group.Forget(key.String())
			return nil, err
		}
		c.mu.Lock()
		if prev, ok := c.entries[key]; ok {
			c.mu.Unlock()
			return *prev, nil
		}
		c.entries[key] = &entry
		c.mu.Unlock()
		return entry, nil
	})
	if err != nil {
		return cachedEnv{}, err
	}
	return val.(cachedEnv), nil
}

// defaultEnvCache is shared across every validator invocation in the process
// (occ lint and the LSP), so all documents with the same kind and schema reuse
// one env.
var defaultEnvCache = newEnvCache()

// envFor returns the schema-aware CEL environment for a CEL-bearing kind,
// memoized in defaultEnvCache. Unsupported kinds return an error that is never
// cached.
func envFor(kind string, pair SchemaPair) (cachedEnv, error) {
	key := cacheKey{
		kind:           kind,
		paramsHash:     hashStructural(pair.Parameters),
		envConfigsHash: hashStructural(pair.EnvironmentConfigs),
	}

	var build func() (cachedEnv, error)
	switch kind {
	case KindProjectType, KindClusterProjectType:
		build = func() (cachedEnv, error) {
			env, provider, err := NewProjectEnv(SchemaOptions{ParametersSchema: pair.Parameters, EnvironmentConfigsSchema: pair.EnvironmentConfigs})
			return cachedEnv{env: env, provider: provider}, err
		}

	case KindComponentType, KindClusterComponentType:
		build = func() (cachedEnv, error) {
			var env *cel.Env
			var provider *apiservercel.DeclTypeProvider
			var err error
			env, provider, err = NewComponentEnv(SchemaOptions{ParametersSchema: pair.Parameters, EnvironmentConfigsSchema: pair.EnvironmentConfigs})
			if err != nil {
				return cachedEnv{}, err
			}
			return cachedEnv{env: env, provider: provider}, nil
		}
	case KindTrait, KindClusterTrait:
		build = func() (cachedEnv, error) {
			var env *cel.Env
			var provider *apiservercel.DeclTypeProvider
			var err error
			env, provider, err = NewTraitEnv(SchemaOptions{ParametersSchema: pair.Parameters, EnvironmentConfigsSchema: pair.EnvironmentConfigs})
			if err != nil {
				return cachedEnv{}, err
			}
			return cachedEnv{env: env, provider: provider}, nil
		}
	case KindResourceType, KindClusterResourceType:
		build = func() (cachedEnv, error) {
			env, err := NewResourceEnv(SchemaOptions{
				ParametersSchema:         pair.Parameters,
				EnvironmentConfigsSchema: pair.EnvironmentConfigs,
			})
			if err != nil {
				return cachedEnv{}, err
			}
			return cachedEnv{env: env}, nil
		}
	case KindWorkflow, KindClusterWorkflow:
		build = func() (cachedEnv, error) {
			env, err := NewWorkflowEnv(SchemaOptions{ParametersSchema: pair.Parameters})
			if err != nil {
				return cachedEnv{}, err
			}
			return cachedEnv{env: env}, nil
		}
	default:
		return cachedEnv{}, fmt.Errorf("no CEL environment for kind %q", kind)
	}

	return defaultEnvCache.get(key, build)
}
