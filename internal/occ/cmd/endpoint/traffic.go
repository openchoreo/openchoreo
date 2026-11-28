// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package endpoint

import (
	"context"
	"fmt"
	"os"
	"slices"
	"strings"
	"text/tabwriter"

	"k8s.io/utils/ptr"
	"sigs.k8s.io/yaml"

	"github.com/openchoreo/openchoreo/internal/occ/cmd/pagination"
	"github.com/openchoreo/openchoreo/internal/occ/cmdutil"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
	"github.com/openchoreo/openchoreo/pkg/endpointtraffic"
)

// TrafficGet prints the Endpoint's default split and what each environment
// serves, marking environments that have their own split.
func (r *Endpoint) TrafficGet(params TrafficGetParams) error {
	if err := cmdutil.RequireFields("get traffic", "endpoint", map[string]string{
		"namespace": params.Namespace,
		"name":      params.EndpointName,
	}); err != nil {
		return err
	}
	ctx := context.Background()
	endpoint, err := r.client.GetEndpoint(ctx, params.Namespace, params.EndpointName)
	if err != nil {
		return err
	}
	if endpoint.Spec == nil {
		return fmt.Errorf("endpoint %q has no spec", params.EndpointName)
	}
	bindings, err := r.listBindings(ctx, params.Namespace, params.EndpointName)
	if err != nil {
		return err
	}

	defaults := describeConditions(endpoint.Spec.BackendRefs)
	defaultsKey := conditionsKey(endpoint.Spec.BackendRefs)
	var envConditions strings.Builder
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "ENVIRONMENT\tSPLIT\tSOURCE")
	fmt.Fprintf(w, "(default)\t%s\tEndpoint\n", describeSplit(toBackends(endpoint.Spec.BackendRefs)))
	for i := range bindings {
		b := bindings[i]
		if b.Spec == nil {
			continue
		}
		if b.Spec.EndpointRelease == nil || *b.Spec.EndpointRelease == "" {
			fmt.Fprintf(w, "%s\t-\tnot deployed\n", b.Spec.Environment)
			continue
		}
		refs, err := r.releaseRefs(ctx, params.Namespace, *b.Spec.EndpointRelease)
		if err != nil {
			return fmt.Errorf("environment %s: %w", b.Spec.Environment, err)
		}
		release := toBackends(refs)
		heeded := endpointtraffic.Heeded(toOverrides(b.Spec.BackendOverrides), release)
		matches := heededMatches(b.Spec.BackendOverrides, refs)
		source := "default split"
		if len(heeded) > 0 || len(matches) > 0 {
			source = "own split"
		}
		// An environment's conditions are its pinned release's with its own
		// in place; listed when they differ from the Endpoint's.
		if own := withConditions(refs, matches); conditionsKey(own) != defaultsKey {
			fmt.Fprintf(&envConditions, "\n%s's conditions:\n%s", b.Spec.Environment, describeConditions(own))
		}
		served := endpointtraffic.Effective(release, heeded)
		split := describeSplit(served)
		if !routesSomewhere(served) {
			// The controller refuses an own split with no share anywhere and
			// keeps the route it rendered before, which this can't show.
			split = "-"
			source = "own split not applied: no backend has a share, so the previous route stays"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\n", b.Spec.Environment, split, source)
	}
	if err := w.Flush(); err != nil {
		return err
	}
	if defaults != "" {
		fmt.Printf("\nDefault conditions:\n%s", defaults)
	}
	fmt.Print(envConditions.String())
	return nil
}

// TrafficSet saves a split: the Endpoint's default when no environment is
// given, which cuts a release, or one environment's own split, which applies
// there right away.
func (r *Endpoint) TrafficSet(params TrafficSetParams) error {
	if err := cmdutil.RequireFields("set traffic", "endpoint", map[string]string{
		"namespace": params.Namespace,
		"name":      params.EndpointName,
	}); err != nil {
		return err
	}
	if len(params.Split) == 0 {
		return fmt.Errorf("give at least one backend=percent")
	}
	if params.Environment == "" {
		return r.setDefaultSplit(params)
	}
	return r.setEnvironmentSplit(params)
}

// TrafficReset drops an environment's own split, so it serves the release's.
func (r *Endpoint) TrafficReset(params TrafficResetParams) error {
	if err := cmdutil.RequireFields("reset traffic", "endpoint", map[string]string{
		"namespace": params.Namespace,
		"name":      params.EndpointName,
		"env":       params.Environment,
	}); err != nil {
		return err
	}
	ctx := context.Background()
	binding, err := r.findBindingForEnv(ctx, params.Namespace, params.EndpointName, params.Environment)
	if err != nil {
		return err
	}
	binding.Spec.BackendOverrides = nil
	return r.saveBinding(ctx, params.Namespace, binding, params.DryRun,
		fmt.Sprintf("%s: follows the default split\n", params.Environment))
}

func (r *Endpoint) setDefaultSplit(params TrafficSetParams) error {
	ctx := context.Background()
	endpoint, err := r.client.GetEndpoint(ctx, params.Namespace, params.EndpointName)
	if err != nil {
		return err
	}
	if endpoint.Spec == nil {
		return fmt.Errorf("endpoint %q has no spec", params.EndpointName)
	}
	refs := endpoint.Spec.BackendRefs
	if err := endpointtraffic.Validate(toBackends(refs), params.Split); err != nil {
		return err
	}
	for i := range refs {
		v, ok := params.Split[refs[i].Name]
		switch {
		case !ok:
		case refs[i].Mirror != nil:
			refs[i].Mirror.Percent = v
		default:
			refs[i].Weight = ptr.To(v)
		}
	}
	if params.DryRun {
		return printManifest("Endpoint", endpoint.Metadata, endpoint.Spec)
	}
	if _, err := r.client.UpdateEndpoint(ctx, params.Namespace, params.EndpointName, *endpoint); err != nil {
		return err
	}
	fmt.Println("Default split saved. A new release is cut; promote it to each environment.")
	return nil
}

func (r *Endpoint) setEnvironmentSplit(params TrafficSetParams) error {
	ctx := context.Background()
	binding, err := r.findBindingForEnv(ctx, params.Namespace, params.EndpointName, params.Environment)
	if err != nil {
		return err
	}
	if binding.Spec.EndpointRelease == nil || *binding.Spec.EndpointRelease == "" {
		return fmt.Errorf("endpoint %q is not deployed in %s yet", params.EndpointName, params.Environment)
	}
	refs, err := r.releaseRefs(ctx, params.Namespace, *binding.Spec.EndpointRelease)
	if err != nil {
		return err
	}
	release := toBackends(refs)
	if err := endpointtraffic.Validate(release, params.Split); err != nil {
		return err
	}
	// Values not given keep what the environment serves now. Validate has
	// checked that shares, when given, cover every backend that takes one.
	split := map[string]int32{}
	for _, b := range endpointtraffic.Effective(release, endpointtraffic.Heeded(toOverrides(binding.Spec.BackendOverrides), release)) {
		if b.Mirror() {
			split[b.Name] = *b.Copy
		} else {
			split[b.Name] = b.Weight
		}
	}
	for name, v := range params.Split {
		split[name] = v
	}
	overrides := withMatches(fromOverrides(endpointtraffic.Overrides(release, split)),
		ownMatches(binding.Spec.BackendOverrides))
	if len(overrides) == 0 {
		binding.Spec.BackendOverrides = nil
	} else {
		binding.Spec.BackendOverrides = &overrides
	}
	message := fmt.Sprintf("%s: own split saved\n", params.Environment)
	if len(overrides) == 0 {
		message = fmt.Sprintf("%s: same as the default split, so nothing is stored\n", params.Environment)
	}
	return r.saveBinding(ctx, params.Namespace, binding, params.DryRun, message)
}

func (r *Endpoint) saveBinding(ctx context.Context, namespace string, binding *gen.EndpointReleaseBinding, dryRun bool, message string) error {
	if dryRun {
		return printManifest("EndpointReleaseBinding", binding.Metadata, binding.Spec)
	}
	if _, err := r.client.UpdateEndpointReleaseBinding(ctx, namespace, binding.Metadata.Name, *binding); err != nil {
		return fmt.Errorf("update endpoint release binding %q in namespace %q: %w", binding.Metadata.Name, namespace, err)
	}
	fmt.Print(message)
	return nil
}

// releaseRefs reads the backends a release froze.
func (r *Endpoint) releaseRefs(ctx context.Context, namespace, releaseName string) ([]gen.EndpointBackendRef, error) {
	release, err := r.client.GetEndpointRelease(ctx, namespace, releaseName)
	if err != nil {
		return nil, fmt.Errorf("get endpoint release %q in namespace %q: %w", releaseName, namespace, err)
	}
	if release.Spec == nil {
		return nil, fmt.Errorf("endpoint release %q has no spec", releaseName)
	}
	return release.Spec.BackendRefs, nil
}

// heededMatches returns the environment's own conditions, as backend refs:
// the match overrides that differ from the release's, for backends that have
// a match in it. The controller ignores the rest, and one equal to the
// release's changes nothing.
func heededMatches(overrides *[]gen.EndpointBackendOverride, refs []gen.EndpointBackendRef) []gen.EndpointBackendRef {
	var out []gen.EndpointBackendRef
	for _, o := range ownMatches(overrides) {
		for _, ref := range refs {
			if ref.Name == o.Name && ref.Match != nil && matchKey(ref.Match) != matchKey(o.Match) {
				out = append(out, o)
			}
		}
	}
	return out
}

// withConditions returns the release's backends with the environment's own
// conditions in place of theirs.
func withConditions(refs, matches []gen.EndpointBackendRef) []gen.EndpointBackendRef {
	out := slices.Clone(refs)
	for i := range out {
		for _, m := range matches {
			if m.Name == out[i].Name {
				out[i].Match = m.Match
			}
		}
	}
	return out
}

// ownMatches returns every match override as a backend ref, including ones
// the pinned release ignores: they apply again once a release gives the
// backend a match.
func ownMatches(overrides *[]gen.EndpointBackendOverride) []gen.EndpointBackendRef {
	if overrides == nil {
		return nil
	}
	var out []gen.EndpointBackendRef
	for _, o := range *overrides {
		if o.Match != nil {
			out = append(out, gen.EndpointBackendRef{Name: o.Name, Match: o.Match})
		}
	}
	return out
}

// matchKey identifies a match by the requests it selects: defaults filled
// in, header names lowercased, a prefix without its trailing slash, and the
// conditions in any order.
func matchKey(m *gen.EndpointMatch) string {
	var parts []string
	if m.Path != nil {
		t := string(gen.EndpointPathMatchTypePathPrefix)
		if m.Path.Type != nil {
			t = string(*m.Path.Type)
		}
		v := m.Path.Value
		if t == string(gen.EndpointPathMatchTypePathPrefix) && len(v) > 1 {
			v = strings.TrimSuffix(v, "/")
		}
		parts = append(parts, "path\x00"+t+"\x00"+v)
	}
	if m.Headers != nil {
		for _, h := range *m.Headers {
			t := string(gen.EndpointHeaderMatchTypeExact)
			if h.Type != nil {
				t = string(*h.Type)
			}
			parts = append(parts, "header\x00"+strings.ToLower(h.Name)+"\x00"+t+"\x00"+h.Value)
		}
	}
	if m.Cookies != nil {
		for _, c := range *m.Cookies {
			parts = append(parts, "cookie\x00"+c.Name+"\x00"+c.Value)
		}
	}
	slices.Sort(parts)
	return strings.Join(parts, "\n")
}

// conditionsKey identifies the backends' conditions by the requests they
// select, so two that read differently but select the same requests are equal.
func conditionsKey(refs []gen.EndpointBackendRef) string {
	var parts []string
	for _, ref := range refs {
		if ref.Match != nil {
			parts = append(parts, ref.Name+"\x00"+matchKey(ref.Match))
		}
	}
	return strings.Join(parts, "\n\n")
}

// withMatches keeps the environment's own conditions on the overrides a
// split saves, which only carry weights and copies.
func withMatches(overrides []gen.EndpointBackendOverride, matches []gen.EndpointBackendRef) []gen.EndpointBackendOverride {
	for _, m := range matches {
		found := false
		for i := range overrides {
			if overrides[i].Name == m.Name {
				overrides[i].Match = m.Match
				found = true
			}
		}
		if !found {
			overrides = append(overrides, gen.EndpointBackendOverride{Name: m.Name, Match: m.Match})
		}
	}
	return overrides
}

func (r *Endpoint) listBindings(ctx context.Context, namespace, endpointName string) ([]gen.EndpointReleaseBinding, error) {
	return pagination.FetchAll(func(limit int, cursor string) ([]gen.EndpointReleaseBinding, string, error) {
		p := &gen.ListEndpointReleaseBindingsParams{Endpoint: &endpointName}
		p.Limit = &limit
		if cursor != "" {
			p.Cursor = &cursor
		}
		result, err := r.client.ListEndpointReleaseBindings(ctx, namespace, p)
		if err != nil {
			return nil, "", err
		}
		next := ""
		if result.Pagination.NextCursor != nil {
			next = *result.Pagination.NextCursor
		}
		return result.Items, next, nil
	})
}

func toBackends(refs []gen.EndpointBackendRef) []endpointtraffic.Backend {
	out := make([]endpointtraffic.Backend, 0, len(refs))
	for _, ref := range refs {
		b := endpointtraffic.Backend{Name: ref.Name, Weight: 1}
		if ref.Weight != nil {
			b.Weight = *ref.Weight
		}
		if ref.Mirror != nil {
			b.Weight = 0
			b.Copy = ptr.To(ref.Mirror.Percent)
		}
		out = append(out, b)
	}
	return out
}

func toOverrides(overrides *[]gen.EndpointBackendOverride) []endpointtraffic.Override {
	if overrides == nil {
		return nil
	}
	out := make([]endpointtraffic.Override, 0, len(*overrides))
	for _, o := range *overrides {
		// A match-only entry sets no share or copy; heededMatches covers it.
		if o.Weight == nil && o.Mirror == nil {
			continue
		}
		e := endpointtraffic.Override{Name: o.Name, Weight: o.Weight}
		if o.Mirror != nil {
			e.Copy = ptr.To(o.Mirror.Percent)
		}
		out = append(out, e)
	}
	return out
}

func fromOverrides(overrides []endpointtraffic.Override) []gen.EndpointBackendOverride {
	out := make([]gen.EndpointBackendOverride, 0, len(overrides))
	for _, o := range overrides {
		e := gen.EndpointBackendOverride{Name: o.Name, Weight: o.Weight}
		if o.Copy != nil {
			e.Mirror = &gen.EndpointMirror{Percent: *o.Copy}
		}
		out = append(out, e)
	}
	return out
}

// routesSomewhere reports whether any backend gets a share of the requests.
func routesSomewhere(backends []endpointtraffic.Backend) bool {
	for _, b := range backends {
		if !b.Mirror() && b.Weight > 0 {
			return true
		}
	}
	return false
}

// describeSplit reads "a 45% · b 55% · copy 10% → c".
func describeSplit(backends []endpointtraffic.Backend) string {
	var names []string
	var weights []int32
	var copies []string
	for _, b := range backends {
		if b.Mirror() {
			copies = append(copies, fmt.Sprintf("copy %d%% → %s", *b.Copy, b.Name))
			continue
		}
		names = append(names, b.Name)
		weights = append(weights, b.Weight)
	}
	parts := make([]string, 0, len(names)+len(copies))
	for i, p := range endpointtraffic.Percents(weights) {
		parts = append(parts, fmt.Sprintf("%s %d%%", names[i], p))
	}
	return strings.Join(append(parts, copies...), " · ")
}

// describeConditions lists the requests each backend with a match gets.
func describeConditions(refs []gen.EndpointBackendRef) string {
	var out strings.Builder
	for _, ref := range refs {
		if ref.Match == nil {
			continue
		}
		var conds []string
		if p := ref.Match.Path; p != nil {
			if p.Type != nil && *p.Type == gen.EndpointPathMatchTypeExact {
				conds = append(conds, "exact path "+p.Value)
			} else {
				conds = append(conds, "path "+p.Value)
			}
		}
		if ref.Match.Headers != nil {
			for _, h := range *ref.Match.Headers {
				if h.Type != nil && *h.Type == gen.EndpointHeaderMatchTypeRegularExpression {
					conds = append(conds, fmt.Sprintf("header %s matching %s", h.Name, h.Value))
				} else {
					conds = append(conds, fmt.Sprintf("header %s: %s", h.Name, h.Value))
				}
			}
		}
		if ref.Match.Cookies != nil {
			for _, c := range *ref.Match.Cookies {
				conds = append(conds, fmt.Sprintf("cookie %s=%s", c.Name, c.Value))
			}
		}
		fmt.Fprintf(&out, "  %s gets requests with %s\n", ref.Name, strings.Join(conds, " and "))
	}
	return out.String()
}

func printManifest(kind string, meta gen.ObjectMeta, spec any) error {
	data, err := yaml.Marshal(map[string]any{
		"apiVersion": "openchoreo.dev/v1alpha1",
		"kind":       kind,
		"metadata":   meta,
		"spec":       spec,
	})
	if err != nil {
		return err
	}
	fmt.Print(string(data))
	return nil
}
