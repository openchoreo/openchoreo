// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package endpoint

import (
	"context"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/openchoreo/openchoreo/internal/occ/cmd/pagination"
	"github.com/openchoreo/openchoreo/internal/occ/cmdutil"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
)

const (
	clearScreen = "\033[H\033[2J"
	setByHand   = "set by hand"
)

// RolloutGet prints an endpoint's rollout as a tree: the plan, then each
// environment's step, set and served weights, and its progress through the
// steps. With Watch it redraws until interrupted.
func (r *Endpoint) RolloutGet(params RolloutGetParams) error {
	if err := requireRollout("get", params.RolloutParams, nil); err != nil {
		return err
	}
	ctx := context.Background()
	for {
		status, err := r.client.GetEndpointRollout(ctx, params.Namespace, params.EndpointName)
		if err != nil {
			return err
		}
		ep, err := r.client.GetEndpoint(ctx, params.Namespace, params.EndpointName)
		if err != nil {
			return err
		}
		var refs []gen.EndpointBackendRef
		if ep.Spec != nil {
			refs = ep.Spec.BackendRefs
		}
		if params.Watch {
			fmt.Print(clearScreen)
		}
		printRolloutTree(os.Stdout, status, refs, params.Environment)
		if !params.Watch {
			return nil
		}
		time.Sleep(params.Interval)
	}
}

// RolloutList prints every endpoint in a namespace that has a rollout plan
// and the step each environment is on.
func (r *Endpoint) RolloutList(params ListParams) error {
	if err := cmdutil.RequireFields("rollout list", "endpoint", map[string]string{"namespace": params.Namespace}); err != nil {
		return err
	}
	ctx := context.Background()
	items, err := pagination.FetchAll(func(limit int, cursor string) ([]gen.Endpoint, string, error) {
		p := &gen.ListEndpointsParams{Limit: &limit}
		if cursor != "" {
			p.Cursor = &cursor
		}
		if params.Project != "" {
			p.Project = &params.Project
		}
		result, err := r.client.ListEndpoints(ctx, params.Namespace, p)
		if err != nil {
			return nil, "", err
		}
		next := ""
		if result.Pagination.NextCursor != nil {
			next = *result.Pagination.NextCursor
		}
		return result.Items, next, nil
	})
	if err != nil {
		return err
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "NAME\tSTRATEGY\tSTABLE\tCANDIDATES\tENVIRONMENTS")
	found := false
	for _, ep := range items {
		if ep.Spec == nil || ep.Spec.Rollout == nil {
			continue
		}
		found = true
		status, err := r.client.GetEndpointRollout(ctx, params.Namespace, ep.Metadata.Name)
		if err != nil {
			return err
		}
		envs := make([]string, 0, len(status.Environments))
		for _, env := range status.Environments {
			envs = append(envs, fmt.Sprintf("%s %s", env.Environment, stepText(env, status)))
		}
		ro := ep.Spec.Rollout
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", ep.Metadata.Name, ro.Strategy, ro.Stable,
			strings.Join(candidateNames(ro), ","), strings.Join(envs, ", "))
	}
	if !found {
		fmt.Println("No endpoints with a rollout plan found")
		return nil
	}
	return w.Flush()
}

func candidateNames(ro *gen.EndpointRollout) []string {
	out := make([]string, 0, len(ro.Candidates))
	for _, c := range ro.Candidates {
		out = append(out, c.Name)
	}
	return out
}

func lastStep(status *gen.EndpointRolloutStatus) int {
	if status.Stages == nil {
		return 0
	}
	return len(*status.Stages) - 1
}

func strategyOf(status *gen.EndpointRolloutStatus) gen.EndpointRolloutStrategy {
	if status.Rollout == nil {
		return ""
	}
	return status.Rollout.Strategy
}

// stepText is where an environment stands, in the strategy's own words:
// a canary or mirror step, a blue/green preview or switch, or an A/B test
// that runs until it is finished.
func stepText(env gen.EndpointRolloutEnvironment, status *gen.EndpointRolloutStatus) string {
	switch {
	case env.Missing != nil && len(*env.Missing) > 0:
		return "needs " + strings.Join(*env.Missing, ", ")
	case env.Step == nil:
		return "-"
	case *env.Step < 0:
		return setByHand
	}
	step := *env.Step
	switch strategyOf(status) {
	case gen.EndpointRolloutStrategyBlueGreen:
		if step == 0 {
			return "preview"
		}
		return "switched"
	case gen.EndpointRolloutStrategyAbTest:
		return "experiment"
	case gen.EndpointRolloutStrategyMirror:
		if step == 0 {
			return "not copying"
		}
		return fmt.Sprintf("copying %d/%d", step, lastStep(status))
	}
	return fmt.Sprintf("%d/%d", step, lastStep(status))
}

func printRolloutTree(out io.Writer, status *gen.EndpointRolloutStatus, refs []gen.EndpointBackendRef, onlyEnv string) {
	ro := status.Rollout
	if ro == nil {
		fmt.Fprintf(out, "Endpoint '%s' has no rollout plan\n", status.Endpoint)
		return
	}
	strategy := ro.Strategy
	fmt.Fprintf(out, "Name:       %s\n", status.Endpoint)
	fmt.Fprintf(out, "Strategy:   %s   %s → %s\n", strategyName(strategy), ro.Stable, strings.Join(candidateNames(ro), ", "))
	stages := []gen.EndpointRolloutStage{}
	if status.Stages != nil {
		stages = *status.Stages
	}
	switch strategy {
	case gen.EndpointRolloutStrategyCanary:
		if len(stages) > 1 {
			fmt.Fprintf(out, "Steps:      %s\n", strings.Join(stageLabels(stages), " → "))
		}
	case gen.EndpointRolloutStrategyMirror:
		if len(stages) > 1 {
			fmt.Fprintf(out, "Copies:     %s\n", strings.Join(stageLabels(stages), " → "))
		}
	case gen.EndpointRolloutStrategyBlueGreen:
		fmt.Fprintln(out, "Steps:      preview → switched")
	}
	for _, c := range candidateNames(ro) {
		if reach := reachOf(refs, c, strategy); reach != "" {
			fmt.Fprintf(out, "%-12s%s → %s\n", reachLabel(strategy)+":", reach, c)
		}
	}
	if status.LatestRelease != nil {
		fmt.Fprintf(out, "Release:    %s\n", *status.LatestRelease)
	}
	fmt.Fprintln(out)

	envs := make([]gen.EndpointRolloutEnvironment, 0, len(status.Environments))
	for _, env := range status.Environments {
		if onlyEnv == "" || env.Environment == onlyEnv {
			envs = append(envs, env)
		}
	}
	if len(envs) == 0 {
		if onlyEnv != "" {
			fmt.Fprintf(out, "Not deployed to %s\n", onlyEnv)
			return
		}
		fmt.Fprintln(out, "Not deployed to any environment")
		return
	}
	w := tabwriter.NewWriter(out, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "ENVIRONMENT\tRELEASE\tSTEP\tSET\tACTUAL\tSTATUS")
	for _, env := range envs {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n", env.Environment, deref(env.EndpointRelease),
			stepText(env, status), setWeights(env), servedWeights(env), health(env))
	}
	_ = w.Flush()

	fmt.Fprintln(out)
	switch strategy {
	case gen.EndpointRolloutStrategyAbTest:
		fmt.Fprintf(out, "Each group gets its candidate until the test ends: occ endpoint rollout finish %s --keep <winner>\n",
			status.Endpoint)
	case gen.EndpointRolloutStrategyBlueGreen:
		for _, env := range envs {
			fmt.Fprintf(out, "%-12s %s\n", env.Environment, blueGreenProgress(env))
		}
	default:
		if len(stages) > 1 {
			for _, env := range envs {
				fmt.Fprintf(out, "%-12s %s\n", env.Environment, progress(stages, env))
			}
		}
	}
}

func strategyName(s gen.EndpointRolloutStrategy) string {
	switch s {
	case gen.EndpointRolloutStrategyBlueGreen:
		return "Blue/green"
	case gen.EndpointRolloutStrategyAbTest:
		return "A/B test"
	case gen.EndpointRolloutStrategyMirror:
		return "Mirror"
	}
	return "Canary"
}

func reachLabel(s gen.EndpointRolloutStrategy) string {
	switch s {
	case gen.EndpointRolloutStrategyBlueGreen:
		return "Preview"
	case gen.EndpointRolloutStrategyAbTest:
		return "Group"
	case gen.EndpointRolloutStrategyMirror:
		return "Copies"
	}
	return "Testers"
}

// reachOf is how requests reach a candidate before, or besides, its share:
// its match, or for a mirror the share of requests copied to it.
func reachOf(refs []gen.EndpointBackendRef, name string, strategy gen.EndpointRolloutStrategy) string {
	for _, ref := range refs {
		if ref.Name != name {
			continue
		}
		if ref.Mirror != nil && strategy == gen.EndpointRolloutStrategyMirror {
			return fmt.Sprintf("%d%% of requests, by default", ref.Mirror.Percent)
		}
		if ref.Match == nil {
			return ""
		}
		var parts []string
		if ref.Match.Path != nil {
			parts = append(parts, "path "+ref.Match.Path.Value)
		}
		if ref.Match.Cookies != nil {
			for _, c := range *ref.Match.Cookies {
				parts = append(parts, fmt.Sprintf("cookie %s=%s", c.Name, c.Value))
			}
		}
		if ref.Match.Headers != nil {
			for _, h := range *ref.Match.Headers {
				parts = append(parts, fmt.Sprintf("header %s: %s", h.Name, h.Value))
			}
		}
		return strings.Join(parts, " + ")
	}
	return ""
}

func blueGreenProgress(env gen.EndpointRolloutEnvironment) string {
	if env.Step != nil && *env.Step >= 1 {
		return "✔ preview  ✔ switched"
	}
	return "► preview  ○ switched"
}

func stageLabels(stages []gen.EndpointRolloutStage) []string {
	out := make([]string, 0, len(stages)-1)
	for _, st := range stages[1:] {
		out = append(out, stageLabel(st))
	}
	return out
}

func stageLabel(st gen.EndpointRolloutStage) string {
	parts := make([]string, 0, len(st.Backends))
	for _, b := range st.Backends {
		parts = append(parts, fmt.Sprintf("%d%%", b.Percent))
	}
	return strings.Join(parts, "/")
}

// progress marks each step done (✔), current (►) or to come (○).
func progress(stages []gen.EndpointRolloutStage, env gen.EndpointRolloutEnvironment) string {
	at := -1
	if env.Step != nil {
		at = *env.Step
	}
	parts := make([]string, 0, len(stages)-1)
	for _, st := range stages[1:] {
		mark := "○"
		switch {
		case st.Index == at:
			mark = "►"
		case st.Index < at:
			mark = "✔"
		}
		parts = append(parts, mark+" "+stageLabel(st))
	}
	return strings.Join(parts, "  ")
}

func setWeights(env gen.EndpointRolloutEnvironment) string {
	if len(env.Overrides) == 0 {
		return "release weights"
	}
	parts := make([]string, 0, len(env.Overrides))
	for _, o := range env.Overrides {
		switch {
		case o.Weight != nil:
			parts = append(parts, fmt.Sprintf("%s %d", o.Name, *o.Weight))
		case o.Mirror != nil:
			parts = append(parts, fmt.Sprintf("%s copy %d%%", o.Name, o.Mirror.Percent))
		}
	}
	sort.Strings(parts)
	return strings.Join(parts, "  ")
}

func servedWeights(env gen.EndpointRolloutEnvironment) string {
	parts := make([]string, 0, len(env.Backends))
	for _, b := range env.Backends {
		weight := int32(0)
		if b.Weight != nil {
			weight = *b.Weight
		}
		part := fmt.Sprintf("%s %d", b.Name, weight)
		if b.Mirror != nil {
			part = fmt.Sprintf("%s copy %d%%", b.Name, b.Mirror.Percent)
		}
		if !b.Resolved {
			part += " (unresolved)"
		}
		parts = append(parts, part)
	}
	sort.Strings(parts)
	return strings.Join(parts, "  ")
}

func health(env gen.EndpointRolloutEnvironment) string {
	if env.Ready {
		return "✔ Healthy"
	}
	if env.Message != nil && *env.Message != "" {
		return "◌ " + *env.Message
	}
	return "◌ Progressing"
}
