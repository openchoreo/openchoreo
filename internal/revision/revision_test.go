// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package revision

import (
	"fmt"
	"math"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

// test helpers

func raw(s string) *runtime.RawExtension {
	return &runtime.RawExtension{Raw: []byte(s)}
}

func mustHash(t *testing.T, v any) string {
	t.Helper()
	h, err := CanonicalHash(v)
	require.NoError(t, err)
	return h
}

// baseSpec returns a ReleaseBindingSpec with every field populated.
func baseSpec() openchoreov1alpha1.ReleaseBindingSpec {
	return openchoreov1alpha1.ReleaseBindingSpec{
		Owner:                           openchoreov1alpha1.ReleaseBindingOwner{ProjectName: "proj", ComponentName: "comp"},
		Environment:                     "dev",
		ReleaseName:                     "comp-abc123",
		ComponentTypeEnvironmentConfigs: raw(`{"replicas":2,"resources":{"cpu":"500m","memory":"1Gi"}}`),
		TraitEnvironmentConfigs: map[string]runtime.RawExtension{
			"ingress": *raw(`{"host":"a.example.com","tls":true}`),
		},
		WorkloadOverrides: &openchoreov1alpha1.WorkloadOverrideTemplateSpec{
			Container: &openchoreov1alpha1.ContainerOverride{
				Env: []openchoreov1alpha1.EnvVar{{Key: "LOG_LEVEL", Value: "info"}},
			},
		},
		State: openchoreov1alpha1.ReleaseStateActive,
	}
}

// ─────────────────────────────────────────────────────────────
// CanonicalHash
// ─────────────────────────────────────────────────────────────

func TestCanonicalHash_KeyOrderDoesNotChangeHash(t *testing.T) {
	tests := []struct {
		name string
		a    any
		b    any
	}{
		{
			name: "top-level raw JSON keys in different order",
			a:    raw(`{"a":1,"b":2,"c":3}`),
			b:    raw(`{"c":3,"a":1,"b":2}`),
		},
		{
			name: "nested raw JSON keys in different order",
			a:    raw(`{"outer":{"x":"1","y":{"p":true,"q":false}},"z":[{"k1":1,"k2":2}]}`),
			b:    raw(`{"z":[{"k2":2,"k1":1}],"outer":{"y":{"q":false,"p":true},"x":"1"}}`),
		},
		{
			name: "componentTypeEnvironmentConfigs keys reordered inside a typed spec",
			a:    baseSpec(),
			b: func() openchoreov1alpha1.ReleaseBindingSpec {
				s := baseSpec()
				s.ComponentTypeEnvironmentConfigs = raw(`{"resources":{"memory":"1Gi","cpu":"500m"},"replicas":2}`)
				return s
			}(),
		},
		{
			name: "traitEnvironmentConfigs value keys reordered inside a typed spec",
			a:    baseSpec(),
			b: func() openchoreov1alpha1.ReleaseBindingSpec {
				s := baseSpec()
				s.TraitEnvironmentConfigs = map[string]runtime.RawExtension{
					"ingress": *raw(`{"tls":true,"host":"a.example.com"}`),
				}
				return s
			}(),
		},
		{
			name: "Go maps built in different insertion order",
			a:    map[string]any{"one": 1, "two": 2, "three": map[string]any{"x": 1, "y": 2}},
			b:    map[string]any{"three": map[string]any{"y": 2, "x": 1}, "two": 2, "one": 1},
		},
		{
			name: "insignificant whitespace in raw JSON",
			a:    raw(`{"a":1,"b":[1,2]}`),
			b:    raw("{ \"b\" : [ 1 , 2 ] ,\n\t\"a\" : 1 }"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, mustHash(t, tt.a), mustHash(t, tt.b))
		})
	}
}

func TestCanonicalHash_ChangedFieldChangesHash(t *testing.T) {
	base := mustHash(t, baseSpec())

	tests := []struct {
		name   string
		mutate func(s *openchoreov1alpha1.ReleaseBindingSpec)
	}{
		{name: "releaseName", mutate: func(s *openchoreov1alpha1.ReleaseBindingSpec) { s.ReleaseName = "comp-def456" }},
		{name: "environment", mutate: func(s *openchoreov1alpha1.ReleaseBindingSpec) { s.Environment = "staging" }},
		{name: "owner.componentName", mutate: func(s *openchoreov1alpha1.ReleaseBindingSpec) { s.Owner.ComponentName = otherComponent }},
		{name: "state", mutate: func(s *openchoreov1alpha1.ReleaseBindingSpec) { s.State = openchoreov1alpha1.ReleaseStateUndeploy }},
		{
			name: "nested raw value",
			mutate: func(s *openchoreov1alpha1.ReleaseBindingSpec) {
				s.ComponentTypeEnvironmentConfigs = raw(`{"replicas":2,"resources":{"cpu":"500m","memory":"2Gi"}}`)
			},
		},
		{
			name: "raw number value",
			mutate: func(s *openchoreov1alpha1.ReleaseBindingSpec) {
				s.ComponentTypeEnvironmentConfigs = raw(`{"replicas":3,"resources":{"cpu":"500m","memory":"1Gi"}}`)
			},
		},
		{
			name: "raw value type string vs number",
			mutate: func(s *openchoreov1alpha1.ReleaseBindingSpec) {
				s.ComponentTypeEnvironmentConfigs = raw(`{"replicas":"2","resources":{"cpu":"500m","memory":"1Gi"}}`)
			},
		},
		{
			name: "trait config key",
			mutate: func(s *openchoreov1alpha1.ReleaseBindingSpec) {
				s.TraitEnvironmentConfigs = map[string]runtime.RawExtension{"ingress-2": *raw(`{"host":"a.example.com","tls":true}`)}
			},
		},
		{
			name: "env var value",
			mutate: func(s *openchoreov1alpha1.ReleaseBindingSpec) {
				s.WorkloadOverrides.Container.Env[0].Value = "debug"
			},
		},
		{
			name: "env var order",
			mutate: func(s *openchoreov1alpha1.ReleaseBindingSpec) {
				s.WorkloadOverrides.Container.Env = []openchoreov1alpha1.EnvVar{
					{Key: "B", Value: "2"}, {Key: "LOG_LEVEL", Value: "info"},
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := baseSpec()
			tt.mutate(&s)
			assert.NotEqual(t, base, mustHash(t, s))
		})
	}
}

func TestCanonicalHash_NilVsEmpty(t *testing.T) {
	tests := []struct {
		name     string
		a        any
		b        any
		wantSame bool
	}{
		// Treated as "not set": these must hash the same.
		{
			name:     "nil vs empty traitEnvironmentConfigs map",
			a:        openchoreov1alpha1.ReleaseBindingSpec{Environment: "dev"},
			b:        openchoreov1alpha1.ReleaseBindingSpec{Environment: "dev", TraitEnvironmentConfigs: map[string]runtime.RawExtension{}},
			wantSame: true,
		},
		{
			name:     "nil vs empty workloadOverrides",
			a:        openchoreov1alpha1.ReleaseBindingSpec{Environment: "dev"},
			b:        openchoreov1alpha1.ReleaseBindingSpec{Environment: "dev", WorkloadOverrides: &openchoreov1alpha1.WorkloadOverrideTemplateSpec{}},
			wantSame: true,
		},
		{
			name: "nil vs empty env slice",
			a: openchoreov1alpha1.ReleaseBindingSpec{Environment: "dev", WorkloadOverrides: &openchoreov1alpha1.WorkloadOverrideTemplateSpec{
				Container: &openchoreov1alpha1.ContainerOverride{Env: nil},
			}},
			b: openchoreov1alpha1.ReleaseBindingSpec{Environment: "dev", WorkloadOverrides: &openchoreov1alpha1.WorkloadOverrideTemplateSpec{
				Container: &openchoreov1alpha1.ContainerOverride{Env: []openchoreov1alpha1.EnvVar{}},
			}},
			wantSame: true,
		},
		{
			name:     "nil raw extension vs empty raw object",
			a:        openchoreov1alpha1.ReleaseBindingSpec{Environment: "dev"},
			b:        openchoreov1alpha1.ReleaseBindingSpec{Environment: "dev", ComponentTypeEnvironmentConfigs: raw(`{}`)},
			wantSame: true,
		},
		{
			name:     "trait entry with empty config vs no entry",
			a:        openchoreov1alpha1.ReleaseBindingSpec{Environment: "dev"},
			b:        openchoreov1alpha1.ReleaseBindingSpec{Environment: "dev", TraitEnvironmentConfigs: map[string]runtime.RawExtension{"ingress": *raw(`{}`)}},
			wantSame: true,
		},
		{name: "raw null member vs absent", a: raw(`{"a":1,"b":null}`), b: raw(`{"a":1}`), wantSame: true},
		{name: "raw empty array member vs absent", a: raw(`{"a":1,"b":[]}`), b: raw(`{"a":1}`), wantSame: true},
		{name: "raw object that empties out vs absent", a: raw(`{"a":1,"b":{"c":null,"d":[]}}`), b: raw(`{"a":1}`), wantSame: true},
		{name: "nil vs empty map at top level", a: nil, b: map[string]any{}, wantSame: true},
		{name: "nil vs empty slice at top level", a: []string(nil), b: []string{}, wantSame: true},
		{name: "integer vs float spelling of the same number", a: raw(`{"n":1}`), b: raw(`{"n":1.0}`), wantSame: true},
		{name: "integer vs exponent spelling of the same number", a: raw(`{"n":100}`), b: raw(`{"n":1e2}`), wantSame: true},

		// Values, not "not set": these must hash differently.
		{name: "raw empty string vs absent", a: raw(`{"a":1,"b":""}`), b: raw(`{"a":1}`), wantSame: false},
		{name: "raw false vs absent", a: raw(`{"a":1,"b":false}`), b: raw(`{"a":1}`), wantSame: false},
		{name: "raw zero vs absent", a: raw(`{"a":1,"b":0}`), b: raw(`{"a":1}`), wantSame: false},
		{name: "empty array element keeps its position", a: raw(`{"a":[{},1]}`), b: raw(`{"a":[1]}`), wantSame: false},
		{name: "non-integer floats stay distinct", a: raw(`{"n":1.5}`), b: raw(`{"n":1.25}`), wantSame: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ha, hb := mustHash(t, tt.a), mustHash(t, tt.b)
			if tt.wantSame {
				assert.Equal(t, ha, hb)
			} else {
				assert.NotEqual(t, ha, hb)
			}
		})
	}
}

func TestCanonicalHash_Format(t *testing.T) {
	h1 := mustHash(t, baseSpec())
	h2 := mustHash(t, baseSpec())

	assert.Equal(t, h1, h2, "same spec must always hash the same")
	assert.Len(t, h1, 64)
	assert.Equal(t, strings.ToLower(h1), h1)
}

func TestCanonicalHash_Errors(t *testing.T) {
	tests := []struct {
		name string
		v    any
	}{
		{name: "unmarshalable channel", v: make(chan int)},
		{name: "invalid raw JSON", v: raw(`{"a":`)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := CanonicalHash(tt.v)
			assert.Error(t, err)
		})
	}
}

// ─────────────────────────────────────────────────────────────
// Name
// ─────────────────────────────────────────────────────────────

func TestName(t *testing.T) {
	exactFit := strings.Repeat("a", maxNameLength-len("-r1"))

	tests := []struct {
		name          string
		binding       string
		n             int64
		want          string
		wantShortened bool
	}{
		{name: "short name", binding: "my-binding", n: 1, want: "my-binding-r1"},
		{name: "multi-digit revision", binding: "my-binding", n: 42, want: "my-binding-r42"},
		{name: "binding containing -r", binding: "api-rest-dev", n: 7, want: "api-rest-dev-r7"},
		{name: "exactly 253 characters is not shortened", binding: exactFit, n: 1, want: exactFit + "-r1"},
		{name: "254 characters is shortened", binding: exactFit + "a", n: 1, wantShortened: true},
		{name: "253-character binding is shortened", binding: strings.Repeat("b", 253), n: 3, wantShortened: true},
		{name: "max int64 revision", binding: strings.Repeat("c", 240), n: math.MaxInt64, wantShortened: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Name(tt.binding, tt.n)

			assert.LessOrEqual(t, len(got), maxNameLength)
			assert.Empty(t, validation.IsDNS1123Subdomain(got))
			if tt.want != "" {
				assert.Equal(t, tt.want, got)
			}
			if tt.wantShortened {
				assert.NotEqual(t, tt.binding+"-r"+fmt.Sprint(tt.n), got)
				assert.True(t, strings.HasSuffix(got, fmt.Sprintf("-r%d", tt.n)), "got %q", got)
			}

			n, err := ParseNumber(got)
			require.NoError(t, err)
			assert.Equal(t, tt.n, n)
		})
	}
}

func TestName_LongBindingNamesStayUniqueAndValid(t *testing.T) {
	// Every binding shares the same 245-character prefix, so truncation alone
	// would collide; the hash suffix must keep them apart.
	prefix := strings.Repeat("x", 245)

	tests := []struct {
		name     string
		bindings []string
	}{
		{
			name:     "same prefix, different tails",
			bindings: []string{prefix + "-alpha", prefix + "-beta", prefix + "-gamma", prefix + "1", prefix + "2"},
		},
		{
			name: "dotted subdomain names differing only at the end",
			bindings: []string{
				strings.Repeat("seg.", 62) + "aaaa",
				strings.Repeat("seg.", 62) + "aaab",
			},
		},
		{
			// For n=1 the kept prefix is 253 - len("-r1") - len("-") - 12 = 237
			// characters, so a separator at index 236 is the last kept character.
			name: "truncation lands on '-' or '.'",
			bindings: []string{
				strings.Repeat("a", 236) + "-" + strings.Repeat("b", 16),
				strings.Repeat("a", 236) + "." + strings.Repeat("b", 16),
				strings.Repeat("a", 235) + "--" + strings.Repeat("b", 16),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			seen := map[string]string{}
			for _, b := range tt.bindings {
				require.Empty(t, validation.IsDNS1123Subdomain(b), "test input %q must be a valid name", b)

				for _, n := range []int64{1, 9, 10, 12345, math.MaxInt64} {
					got := Name(b, n)

					assert.LessOrEqual(t, len(got), maxNameLength, "binding %q n=%d", b, n)
					assert.Empty(t, validation.IsDNS1123Subdomain(got), "binding %q n=%d -> %q", b, n, got)

					if prev, ok := seen[got]; ok {
						t.Fatalf("name %q produced by both %q and %q", got, prev, b)
					}
					seen[got] = b

					parsed, err := ParseNumber(got)
					require.NoError(t, err)
					assert.Equal(t, n, parsed)
				}
			}
		})
	}
}

func TestName_ShortenedNamesShareStablePrefix(t *testing.T) {
	binding := strings.Repeat("p", 252)

	r1 := Name(binding, 1)
	r2 := Name(binding, 2)

	assert.Equal(t, r1, Name(binding, 1), "Name must be deterministic")
	assert.Equal(t, strings.TrimSuffix(r1, "-r1"), strings.TrimSuffix(r2, "-r2"),
		"revisions of one binding must share the shortened prefix")
}

// ─────────────────────────────────────────────────────────────
// ParseNumber
// ─────────────────────────────────────────────────────────────

func TestParseNumber(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    int64
		wantErr string
	}{
		{name: "simple", input: "my-binding-r1", want: 1},
		{name: "multi-digit", input: "my-binding-r1024", want: 1024},
		{name: "binding containing -r uses last separator", input: "api-rest-r9-r12", want: 12},
		{name: "max int64", input: fmt.Sprintf("b-r%d", int64(math.MaxInt64)), want: math.MaxInt64},
		{name: "no separator", input: "my-binding", wantErr: "has no"},
		{name: "separator at start", input: "-r1", wantErr: "has no"},
		{name: "empty number", input: "my-binding-r", wantErr: "has no revision number"},
		{name: "non-numeric", input: "my-binding-rabc", wantErr: "non-numeric"},
		{name: "trailing text", input: "my-binding-r1x", wantErr: "non-numeric"},
		{name: "plus sign", input: "my-binding-r+1", wantErr: "non-numeric"},
		{name: "negative", input: "my-binding-r-1", wantErr: "non-numeric"},
		{name: "zero", input: "my-binding-r0", wantErr: "leading zero"},
		{name: "leading zero", input: "my-binding-r01", wantErr: "leading zero"},
		{name: "overflow", input: "my-binding-r9223372036854775808", wantErr: "out of range"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseNumber(tt.input)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}
