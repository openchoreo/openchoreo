// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package endpoint

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	"k8s.io/utils/ptr"
	"sigs.k8s.io/yaml"

	"github.com/openchoreo/openchoreo/internal/occ/cmdutil"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
)

// RolloutStart writes an endpoint's rollout plan, from a file holding the
// start request or from flags.
func (r *Endpoint) RolloutStart(params RolloutStartParams) error {
	if params.File == "" && params.Plan.Strategy == "" {
		return fmt.Errorf("give the plan with --file, or with --strategy, --stable and --candidate")
	}
	if err := requireRollout("start", params.RolloutParams, nil); err != nil {
		return err
	}
	var req gen.EndpointRolloutStartRequest
	if params.File != "" {
		data, err := os.ReadFile(params.File)
		if err != nil {
			return fmt.Errorf("failed to read %s: %w", params.File, err)
		}
		if err := yaml.UnmarshalStrict(data, &req); err != nil {
			return fmt.Errorf("failed to parse %s: %w", params.File, err)
		}
	} else {
		built, err := r.planFromFlags(params)
		if err != nil {
			return err
		}
		req = built
	}
	req.DryRun = &params.DryRun
	status, err := r.client.StartEndpointRollout(context.Background(), params.Namespace, params.EndpointName, req)
	if err != nil {
		return err
	}
	return r.report(params.RolloutParams, status)
}

// planFromFlags builds a start request. The candidate is a backend the
// Endpoint has, or NAME=COMPONENT[@RELEASELINE][/ENDPOINT] to add it in the
// same call. It gets what its strategy needs: weight 0, a mirror for a
// mirror rollout, the cookie for an A/B test, and the testers' header.
func (r *Endpoint) planFromFlags(params RolloutStartParams) (gen.EndpointRolloutStartRequest, error) {
	f := params.Plan
	if f.Stable == "" || len(f.Candidates) == 0 {
		return gen.EndpointRolloutStartRequest{}, fmt.Errorf("--stable and --candidate are required with --strategy")
	}
	strategy := gen.EndpointRolloutStrategy(f.Strategy)
	if len(f.Candidates) > 1 {
		return gen.EndpointRolloutStartRequest{}, fmt.Errorf("a rollout has one candidate; give one --candidate")
	}
	match, err := candidateMatch(strategy, f)
	if err != nil {
		return gen.EndpointRolloutStartRequest{}, err
	}
	if strategy == gen.EndpointRolloutStrategyMirror && match != nil {
		return gen.EndpointRolloutStartRequest{}, fmt.Errorf("a mirror rollout copies requests and routes none by match; drop --header and --cookie")
	}
	ep, err := r.client.GetEndpoint(context.Background(), params.Namespace, params.EndpointName)
	if err != nil {
		return gen.EndpointRolloutStartRequest{}, err
	}
	var refs []gen.EndpointBackendRef
	if ep.Spec != nil {
		refs = ep.Spec.BackendRefs
	}
	stableRef := findRef(refs, f.Stable)
	if stableRef == nil {
		return gen.EndpointRolloutStartRequest{}, fmt.Errorf("%s has no backend %q to keep as stable", params.EndpointName, f.Stable)
	}

	ro := gen.EndpointRollout{Strategy: strategy, Stable: f.Stable}
	backends := make([]gen.EndpointBackendRef, 0, len(f.Candidates))
	for _, spec := range f.Candidates {
		ref, err := candidateRef(spec, refs, stableRef)
		if err != nil {
			return gen.EndpointRolloutStartRequest{}, err
		}
		ref.Weight, ref.Mirror = ptr.To[int32](0), nil
		if match != nil {
			ref.Match = match
		}
		if strategy == gen.EndpointRolloutStrategyMirror {
			ref.Match, ref.Mirror = nil, &gen.EndpointMirror{Percent: 0}
		}
		backends = append(backends, ref)
		ro.Candidates = append(ro.Candidates, struct {
			Name string `json:"name"`
		}{Name: ref.Name})
	}
	if strategy == gen.EndpointRolloutStrategyCanary || strategy == gen.EndpointRolloutStrategyMirror {
		percents := f.Steps
		if len(percents) == 0 {
			percents = []int32{5, 25, 50, 100}
		}
		steps := gen.EndpointRolloutSteps{}
		for _, p := range percents {
			step := gen.EndpointRolloutStep{}
			for _, c := range ro.Candidates {
				step.Backends = append(step.Backends, struct {
					Name    string `json:"name"`
					Percent int32  `json:"percent"`
				}{Name: c.Name, Percent: p})
			}
			steps.Steps = append(steps.Steps, step)
		}
		if strategy == gen.EndpointRolloutStrategyCanary {
			ro.Canary = &steps
		} else {
			ro.Mirror = &steps
		}
	}
	return gen.EndpointRolloutStartRequest{Rollout: ro, Backends: &backends}, nil
}

// candidateMatch is the match the candidate is reached by: the testers'
// header, and for an abTest the cookie of the group that gets it.
func candidateMatch(strategy gen.EndpointRolloutStrategy, f RolloutPlanFlags) (*gen.EndpointMatch, error) {
	match := &gen.EndpointMatch{}
	if f.Header != "" {
		name, value, ok := strings.Cut(f.Header, "=")
		if !ok || name == "" || value == "" {
			return nil, fmt.Errorf("--header must be name=value")
		}
		match.Headers = &[]gen.EndpointHeaderMatch{{Name: name, Value: value}}
	}
	if f.Cookie != "" {
		name, value, ok := strings.Cut(f.Cookie, "=")
		if !ok || name == "" || value == "" {
			return nil, fmt.Errorf("--cookie must be name=value")
		}
		match.Cookies = &[]gen.EndpointCookieMatch{{Name: name, Value: value}}
	}
	if strategy == gen.EndpointRolloutStrategyAbTest && match.Cookies == nil {
		return nil, fmt.Errorf("an abTest needs --cookie name=value: the group that gets the candidate")
	}
	if match.Headers == nil && match.Cookies == nil {
		return nil, nil
	}
	return match, nil
}

func findRef(refs []gen.EndpointBackendRef, name string) *gen.EndpointBackendRef {
	for i := range refs {
		if refs[i].Name == name {
			return &refs[i]
		}
	}
	return nil
}

// candidateRef resolves --candidate: an existing backend, or a new one on
// COMPONENT[@RELEASELINE][/ENDPOINT], whose endpoint defaults to stable's.
func candidateRef(spec string, refs []gen.EndpointBackendRef, stable *gen.EndpointBackendRef) (gen.EndpointBackendRef, error) {
	name, target, adds := strings.Cut(spec, "=")
	existing := findRef(refs, name)
	if !adds {
		if existing == nil {
			return gen.EndpointBackendRef{}, fmt.Errorf(
				"the endpoint has no backend %q; add it with --candidate %s=COMPONENT[@RELEASELINE][/ENDPOINT]", name, name)
		}
		return *existing, nil
	}
	if existing != nil {
		return gen.EndpointBackendRef{}, fmt.Errorf("the endpoint already has a backend %q; give just --candidate %s", name, name)
	}
	target, endpoint, _ := strings.Cut(target, "/")
	component, line, _ := strings.Cut(target, "@")
	if endpoint == "" && stable.Component != nil {
		endpoint = stable.Component.Endpoint
	}
	if component == "" || endpoint == "" {
		return gen.EndpointBackendRef{}, fmt.Errorf("--candidate %s needs a component and an endpoint", spec)
	}
	backend := map[string]any{"name": component, "endpoint": endpoint}
	if line != "" {
		backend["releaseLine"] = line
	}
	raw, err := json.Marshal(map[string]any{"name": name, "kind": "Component", "component": backend})
	if err != nil {
		return gen.EndpointBackendRef{}, err
	}
	var ref gen.EndpointBackendRef
	if err := json.Unmarshal(raw, &ref); err != nil {
		return gen.EndpointBackendRef{}, err
	}
	if line != "" {
		if out, _ := json.Marshal(ref.Component); !strings.Contains(string(out), "releaseLine") {
			return gen.EndpointBackendRef{}, fmt.Errorf("this OpenChoreo API does not support release line backends")
		}
	}
	return ref, nil
}

// RolloutStep moves an environment to its next step. --to picks a step,
// --back goes one step back and --last to the final one; --expect-step
// refuses the change unless the environment is still at that step.
func (r *Endpoint) RolloutStep(params RolloutStepParams) error {
	if err := requireRollout("step", params.RolloutParams, map[string]string{"env": params.Environment}); err != nil {
		return err
	}
	picked := 0
	for _, set := range []bool{params.To != nil, params.Back, params.Last} {
		if set {
			picked++
		}
	}
	if picked > 1 {
		return fmt.Errorf("give only one of --to, --back and --last")
	}
	ctx := context.Background()
	status, err := r.client.GetEndpointRollout(ctx, params.Namespace, params.EndpointName)
	if err != nil {
		return err
	}
	if strategyOf(status) == gen.EndpointRolloutStrategyAbTest {
		return fmt.Errorf("an A/B test has no steps: its group gets the candidate until it ends; "+
			"finish it with: occ endpoint rollout finish %s --keep <winner>", params.EndpointName)
	}
	switch {
	case params.Last:
		params.To = ptr.To(lastStep(status))
	case params.Back:
		env := environmentOf(status, params.Environment)
		if env == nil {
			return fmt.Errorf("endpoint %q is not deployed in %s", params.EndpointName, params.Environment)
		}
		if env.Step == nil || *env.Step <= 0 {
			return fmt.Errorf("%s is %s; there is no step to go back to", params.Environment, describeStep(env))
		}
		params.To = ptr.To(*env.Step - 1)
		if params.ExpectedStep == nil {
			params.ExpectedStep = env.Step
		}
	}
	req := gen.EndpointRolloutStepRequest{
		Environment:  params.Environment,
		To:           params.To,
		ExpectedStep: params.ExpectedStep,
		DryRun:       &params.DryRun,
	}
	result, err := r.client.StepEndpointRollout(ctx, params.Namespace, params.EndpointName, req)
	if err != nil {
		return err
	}
	return r.report(params.RolloutParams, result)
}

// RolloutAbort moves an environment back to step 0. Requests a candidate's
// match selects still reach it.
func (r *Endpoint) RolloutAbort(params RolloutStepParams) error {
	if err := requireRollout("abort", params.RolloutParams, map[string]string{"env": params.Environment}); err != nil {
		return err
	}
	req := gen.EndpointRolloutAbortRequest{Environment: params.Environment, DryRun: &params.DryRun}
	status, err := r.client.AbortEndpointRollout(context.Background(), params.Namespace, params.EndpointName, req)
	if err != nil {
		return err
	}
	return r.report(params.RolloutParams, status)
}

// RolloutFinish ends the rollout keeping one backend.
func (r *Endpoint) RolloutFinish(params RolloutFinishParams) error {
	if err := requireRollout("finish", params.RolloutParams, map[string]string{"keep": params.Keep}); err != nil {
		return err
	}
	req := gen.EndpointRolloutFinishRequest{Keep: params.Keep, DryRun: &params.DryRun}
	status, err := r.client.FinishEndpointRollout(context.Background(), params.Namespace, params.EndpointName, req)
	if err != nil {
		return err
	}
	return r.report(params.RolloutParams, status)
}

// RolloutStatus waits until an environment serves what its binding sets, and
// fails when it doesn't within the timeout, so a pipeline can gate on it.
func (r *Endpoint) RolloutStatus(params RolloutWaitParams) error {
	if err := requireRollout("status", params.RolloutParams, map[string]string{"env": params.Environment}); err != nil {
		return err
	}
	deadline := time.Now().Add(params.Timeout)
	ctx := context.Background()
	if params.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithDeadline(ctx, deadline)
		defer cancel()
	}
	for first := true; ; first = false {
		// An answer to a request sent after the deadline is too late, even
		// when it says ready; the first one always counts.
		inTime := first || !time.Now().After(deadline)
		status, err := r.client.GetEndpointRollout(ctx, params.Namespace, params.EndpointName)
		if err != nil {
			if ctx.Err() != nil {
				return fmt.Errorf("%s is not serving its latest change after %s", params.Environment, params.Timeout)
			}
			return err
		}
		env := environmentOf(status, params.Environment)
		if env == nil {
			return fmt.Errorf("endpoint %q is not deployed in %s", params.EndpointName, params.Environment)
		}
		if env.Ready && inTime {
			fmt.Printf("%s: %s, %s\n", params.Environment, describeStep(env), health(*env))
			return nil
		}
		left := time.Until(deadline)
		if left <= 0 {
			return fmt.Errorf("%s is not serving its latest change after %s: %s", params.Environment, params.Timeout, health(*env))
		}
		time.Sleep(min(params.Interval, left))
	}
}

// RolloutSet sets hand-picked weights in an environment. The rollout then
// shows the environment as set by hand until a step moves it back on plan.
func (r *Endpoint) RolloutSet(params RolloutSetParams) error {
	if err := requireRollout("set", params.RolloutParams, map[string]string{"env": params.Environment}); err != nil {
		return err
	}
	if len(params.Weights) == 0 {
		return fmt.Errorf("give at least one backend=weight")
	}
	ctx := context.Background()
	status, err := r.client.GetEndpointRollout(ctx, params.Namespace, params.EndpointName)
	if err != nil {
		return err
	}
	env := environmentOf(status, params.Environment)
	if env == nil {
		return fmt.Errorf("endpoint %q is not deployed in %s", params.EndpointName, params.Environment)
	}
	binding, err := r.client.GetEndpointReleaseBinding(ctx, params.Namespace, env.BindingName)
	if err != nil {
		return err
	}
	// Kept from the binding being written, so a change made after the status
	// read is not lost.
	var current []gen.EndpointBackendOverride
	if binding.Spec.BackendOverrides != nil {
		current = *binding.Spec.BackendOverrides
	}
	overrides := make([]gen.EndpointBackendOverride, 0, len(current)+len(params.Weights))
	for _, o := range current {
		if _, replaced := params.Weights[o.Name]; !replaced {
			overrides = append(overrides, o)
		}
	}
	names := make([]string, 0, len(params.Weights))
	for name := range params.Weights {
		names = append(names, name)
	}
	slices.Sort(names)
	for _, name := range names {
		overrides = append(overrides, gen.EndpointBackendOverride{Name: name, Weight: ptr.To(params.Weights[name])})
	}
	binding.Spec.BackendOverrides = &overrides
	if params.DryRun {
		return printBinding(binding)
	}
	updated, err := r.client.UpdateEndpointReleaseBinding(ctx, params.Namespace, binding.Metadata.Name, *binding)
	if err != nil {
		return err
	}
	if params.Output == "yaml" {
		return printBinding(updated)
	}
	fmt.Printf("%s: weights set by hand\n", params.Environment)
	return nil
}

func printBinding(binding *gen.EndpointReleaseBinding) error {
	data, err := yaml.Marshal(manifest("EndpointReleaseBinding", binding.Metadata, binding.Spec))
	if err != nil {
		return err
	}
	fmt.Print(string(data))
	return nil
}

func environmentOf(status *gen.EndpointRolloutStatus, name string) *gen.EndpointRolloutEnvironment {
	for i := range status.Environments {
		if status.Environments[i].Environment == name {
			return &status.Environments[i]
		}
	}
	return nil
}

func describeStep(env *gen.EndpointRolloutEnvironment) string {
	switch {
	case env.Step == nil:
		return "not on the plan"
	case *env.Step < 0:
		return setByHand
	}
	return fmt.Sprintf("at step %d", *env.Step)
}

func requireRollout(action string, params RolloutParams, extra map[string]string) error {
	fields := map[string]string{"namespace": params.Namespace, "name": params.EndpointName}
	for k, v := range extra {
		fields[k] = v
	}
	return cmdutil.RequireFields("rollout "+action, "endpoint", fields)
}

// report prints the rollout after a call. With -o yaml it prints the
// resource the call wrote, or would write on a dry run, so it can be
// committed to a GitOps repository instead.
func (r *Endpoint) report(params RolloutParams, status *gen.EndpointRolloutStatus) error {
	if params.Output != "yaml" {
		if params.DryRun {
			fmt.Println("Dry run: nothing was written.")
		}
		return printRollout(status)
	}
	if status.Change == nil {
		return fmt.Errorf("the call returned no change to print")
	}
	ctx := context.Background()
	var obj any
	switch {
	case status.Change.Endpoint != nil:
		ep, err := r.client.GetEndpoint(ctx, params.Namespace, params.EndpointName)
		if err != nil {
			return err
		}
		ep.Spec, ep.Status = status.Change.Endpoint, nil
		obj = manifest("Endpoint", ep.Metadata, ep.Spec)
	default:
		name := ""
		for _, env := range status.Environments {
			if status.Change.Environment != nil && env.Environment == *status.Change.Environment {
				name = env.BindingName
			}
		}
		b, err := r.client.GetEndpointReleaseBinding(ctx, params.Namespace, name)
		if err != nil {
			return err
		}
		b.Spec.BackendOverrides = status.Change.After
		obj = manifest("EndpointReleaseBinding", b.Metadata, b.Spec)
	}
	data, err := yaml.Marshal(obj)
	if err != nil {
		return fmt.Errorf("failed to marshal to YAML: %w", err)
	}
	fmt.Print(string(data))
	return nil
}

func manifest(kind string, meta gen.ObjectMeta, spec any) map[string]any {
	m := map[string]any{"name": meta.Name}
	if meta.Namespace != nil {
		m["namespace"] = *meta.Namespace
	}
	return map[string]any{
		"apiVersion": "openchoreo.dev/v1alpha1",
		"kind":       kind,
		"metadata":   m,
		"spec":       spec,
	}
}

func printRollout(status *gen.EndpointRolloutStatus) error {
	if status.Rollout == nil {
		fmt.Printf("Endpoint '%s' has no rollout plan\n", status.Endpoint)
	} else {
		ro := status.Rollout
		candidates := make([]string, 0, len(ro.Candidates))
		for _, c := range ro.Candidates {
			candidates = append(candidates, c.Name)
		}
		stages := 0
		if status.Stages != nil {
			stages = len(*status.Stages) - 1
		}
		fmt.Printf("Endpoint '%s': %s from %s to %s, %d steps\n",
			status.Endpoint, ro.Strategy, ro.Stable, strings.Join(candidates, ", "), stages)
	}
	if len(status.Environments) == 0 {
		fmt.Println("Not deployed to any environment")
		return nil
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "ENVIRONMENT\tRELEASE\tSTEP\tSHARE\tREADY")
	for _, env := range status.Environments {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", env.Environment, deref(env.EndpointRelease),
			stepOf(env), shareOf(env), readyOf(env))
	}
	return w.Flush()
}

func stepOf(env gen.EndpointRolloutEnvironment) string {
	switch {
	case env.Missing != nil && len(*env.Missing) > 0:
		return "needs " + strings.Join(*env.Missing, ", ")
	case env.Step == nil:
		return "-"
	case *env.Step < 0:
		return setByHand
	}
	return fmt.Sprint(*env.Step)
}

func shareOf(env gen.EndpointRolloutEnvironment) string {
	if env.Percents == nil {
		return "-"
	}
	names := make([]string, 0, len(*env.Percents))
	for name := range *env.Percents {
		names = append(names, name)
	}
	sort.Strings(names)
	parts := make([]string, 0, len(names))
	for _, name := range names {
		parts = append(parts, fmt.Sprintf("%s %d%%", name, (*env.Percents)[name]))
	}
	return strings.Join(parts, ", ")
}

func readyOf(env gen.EndpointRolloutEnvironment) string {
	if env.Ready {
		return "yes"
	}
	if env.Message != nil && *env.Message != "" {
		return "no: " + *env.Message
	}
	return "no"
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
