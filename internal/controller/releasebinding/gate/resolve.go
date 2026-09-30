// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package gate

import (
	"context"
	"fmt"
	"sort"
	"strings"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

// Resolve builds the effective hook set bound by an environment.
//
// Bindings are sorted by name (the environment webhook guarantees names are
// unique across both phases). Each binding's Hook or ClusterHook is fetched; a
// missing hook is recorded, not returned as an error, so the gate can surface
// it on the binding. appliesTo is evaluated against subject; a miss marks the
// binding Skipped.
func Resolve(ctx context.Context, c client.Client, env *openchoreov1alpha1.Environment, subject Subject) (EffectiveHookSet, error) {
	var set EffectiveHookSet
	if env == nil || env.Spec.Hooks == nil {
		return set, nil
	}

	pre := byName(env.Spec.Hooks.PreDeploy)
	post := byName(env.Spec.Hooks.PostDeploy)

	var err error
	if set.PreDeploy, err = resolveList(ctx, c, env.Namespace, pre, PhasePreDeploy, subject); err != nil {
		return set, err
	}
	if set.PostDeploy, err = resolveList(ctx, c, env.Namespace, post, PhasePostDeploy, subject); err != nil {
		return set, err
	}
	return set, nil
}

func byName(bindings []openchoreov1alpha1.HookBinding) map[string]openchoreov1alpha1.HookBinding {
	m := make(map[string]openchoreov1alpha1.HookBinding, len(bindings))
	for _, b := range bindings {
		if _, seen := m[b.Name]; !seen {
			m[b.Name] = b
		}
	}
	return m
}

func resolveList(ctx context.Context, c client.Client, namespace string,
	bindings map[string]openchoreov1alpha1.HookBinding, phase Phase, subject Subject) ([]ResolvedHook, error) {
	names := make([]string, 0, len(bindings))
	for n := range bindings {
		names = append(names, n)
	}
	sort.Strings(names)

	out := make([]ResolvedHook, 0, len(names))
	for _, n := range names {
		b := bindings[n]
		rh := ResolvedHook{Binding: b, Phase: phase}
		if !Matches(b.AppliesTo, subject) {
			rh.SkipReason = SkipReasonNotApplicable
		}
		spec, err := fetchHookSpec(ctx, c, namespace, b.HookRef)
		if err != nil {
			if !apierrors.IsNotFound(err) {
				return nil, fmt.Errorf("resolving hook for binding %q: %w", b.Name, err)
			}
			rh.NotFound = true
		}
		rh.Hook = spec
		if rh.SkipReason == "" && spec != nil && !EnabledFor(spec.EnabledTo, subject) {
			rh.SkipReason = SkipReasonNotEnabled
		}
		out = append(out, rh)
	}
	return out, nil
}

func fetchHookSpec(ctx context.Context, c client.Client, namespace string, ref openchoreov1alpha1.HookRef) (*openchoreov1alpha1.HookSpec, error) {
	switch ref.Kind {
	case openchoreov1alpha1.HookRefKindClusterHook:
		ch := &openchoreov1alpha1.ClusterHook{}
		if err := c.Get(ctx, client.ObjectKey{Name: ref.Name}, ch); err != nil {
			return nil, err
		}
		return &ch.Spec, nil
	default:
		h := &openchoreov1alpha1.Hook{}
		if err := c.Get(ctx, client.ObjectKey{Namespace: namespace, Name: ref.Name}, h); err != nil {
			return nil, err
		}
		return &h.Spec, nil
	}
}

// Matches reports whether an appliesTo list selects the subject's component
// type. An empty list applies to everything. A selector may name a component type by its
// bare name or by the "{workloadType}/{name}" reference form.
func Matches(selectors []openchoreov1alpha1.HookSubjectSelector, subject Subject) bool {
	if len(selectors) == 0 {
		return true
	}
	for _, s := range selectors {
		switch s.Kind {
		case openchoreov1alpha1.HookSubjectSelectorKindComponentType:
			if subject.ComponentTypeKind == openchoreov1alpha1.ComponentTypeRefKindComponentType &&
				componentTypeNameMatches(s.Name, subject.ComponentTypeName) {
				return true
			}
		case openchoreov1alpha1.HookSubjectSelectorKindClusterComponentType:
			if subject.ComponentTypeKind == openchoreov1alpha1.ComponentTypeRefKindClusterComponentType &&
				componentTypeNameMatches(s.Name, subject.ComponentTypeName) {
				return true
			}
		}
	}
	return false
}

// EnabledFor reports whether a hook's enabledTo list includes the subject's
// component type. An empty list enables the hook for every component.
func EnabledFor(refs []openchoreov1alpha1.HookSubjectRef, subject Subject) bool {
	if len(refs) == 0 {
		return true
	}
	for _, r := range refs {
		switch r.Kind {
		case openchoreov1alpha1.HookSubjectRefKindComponentType:
			if subject.ComponentTypeKind == openchoreov1alpha1.ComponentTypeRefKindComponentType &&
				componentTypeNameMatches(r.Name, subject.ComponentTypeName) {
				return true
			}
		case openchoreov1alpha1.HookSubjectRefKindClusterComponentType:
			if subject.ComponentTypeKind == openchoreov1alpha1.ComponentTypeRefKindClusterComponentType &&
				componentTypeNameMatches(r.Name, subject.ComponentTypeName) {
				return true
			}
		}
	}
	return false
}

func componentTypeNameMatches(selector, ref string) bool {
	if selector == ref {
		return true
	}
	if i := strings.Index(ref, "/"); i >= 0 {
		return selector == ref[i+1:]
	}
	return false
}
