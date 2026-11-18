// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

// Package endpointrollout carries out an Endpoint's rollout plan: it writes
// the plan on the Endpoint and each step as backendOverrides on an
// environment's EndpointReleaseBinding. It goes through the Endpoint, release
// and binding services, so their authorization applies.
package endpointrollout

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"

	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/util/retry"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
	clusterendpointtypesvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/clusterendpointtype"
	endpointsvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/endpoint"
	endpointreleasesvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/endpointrelease"
	endpointreleasebindingsvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/endpointreleasebinding"
	endpointtypesvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/endpointtype"
	endpointpipeline "github.com/openchoreo/openchoreo/internal/pipeline/endpoint"
	"github.com/openchoreo/openchoreo/pkg/endpointrollout"
)

var (
	ErrNoRollout           = errors.New("the endpoint has no rollout plan")
	ErrEnvironmentNotBound = errors.New("the endpoint is not deployed in this environment")
)

// ConflictError is a change refused because of where the rollout stands.
type ConflictError struct{ Msg string }

func (e *ConflictError) Error() string { return e.Msg }

func conflict(format string, args ...any) error {
	return &ConflictError{Msg: fmt.Sprintf(format, args...)}
}

// Service runs an Endpoint's rollout.
type Service interface {
	Get(ctx context.Context, namespaceName, endpointName string) (*Status, error)
	Start(ctx context.Context, namespaceName, endpointName string, req StartRequest) (*Status, error)
	Step(ctx context.Context, namespaceName, endpointName string, req StepRequest) (*Status, error)
	Abort(ctx context.Context, namespaceName, endpointName string, req AbortRequest) (*Status, error)
	Finish(ctx context.Context, namespaceName, endpointName string, req FinishRequest) (*Status, error)
}

// StartRequest writes a rollout plan, adding or replacing the backends it
// names in the same update.
type StartRequest struct {
	Rollout  openchoreov1alpha1.EndpointRollout      `json:"rollout"`
	Backends []openchoreov1alpha1.EndpointBackendRef `json:"backends,omitempty"`
	DryRun   bool                                    `json:"dryRun,omitempty"`
}

// StepRequest moves an environment to a stage: the next one, or To.
// ExpectedStep refuses the change unless the environment is at that stage,
// so a retried call can't skip a step.
type StepRequest struct {
	Environment  string `json:"environment"`
	To           *int   `json:"to,omitempty"`
	ExpectedStep *int   `json:"expectedStep,omitempty"`
	DryRun       bool   `json:"dryRun,omitempty"`
}

// AbortRequest moves an environment back to step 0. Weighted traffic goes
// to stable; requests a candidate's match selects still reach it.
type AbortRequest struct {
	Environment string `json:"environment"`
	DryRun      bool   `json:"dryRun,omitempty"`
}

// FinishRequest ends the rollout keeping one backend.
type FinishRequest struct {
	Keep   string `json:"keep"`
	DryRun bool   `json:"dryRun,omitempty"`
}

// Status is an Endpoint's rollout and where each environment stands.
type Status struct {
	Endpoint      string                              `json:"endpoint"`
	LatestRelease string                              `json:"latestRelease,omitempty"`
	Rollout       *openchoreov1alpha1.EndpointRollout `json:"rollout,omitempty"`
	Stages        []endpointrollout.Stage             `json:"stages,omitempty"`
	Environments  []EnvironmentStatus                 `json:"environments"`
	Change        *Change                             `json:"change,omitempty"`
}

// EnvironmentStatus is what one environment sets and serves.
type EnvironmentStatus struct {
	Environment     string                                       `json:"environment"`
	BindingName     string                                       `json:"bindingName"`
	EndpointRelease string                                       `json:"endpointRelease,omitempty"`
	Step            *int                                         `json:"step,omitempty"`
	Percents        map[string]int32                             `json:"percents,omitempty"`
	Missing         []string                                     `json:"missing,omitempty"`
	Overrides       []openchoreov1alpha1.EndpointBackendOverride `json:"overrides"`
	Backends        []openchoreov1alpha1.ResolvedEndpointBackend `json:"backends"`
	Ready           bool                                         `json:"ready"`
	Message         string                                       `json:"message,omitempty"`
}

// Change is what a call writes; on a dry run it is returned instead.
type Change struct {
	Environment string                                       `json:"environment,omitempty"`
	Before      []openchoreov1alpha1.EndpointBackendOverride `json:"before,omitempty"`
	After       []openchoreov1alpha1.EndpointBackendOverride `json:"after,omitempty"`
	Endpoint    *openchoreov1alpha1.EndpointSpec             `json:"endpoint,omitempty"`
}

type service struct {
	endpoints    endpointsvc.Service
	releases     endpointreleasesvc.Service
	bindings     endpointreleasebindingsvc.Service
	allBindings  endpointreleasebindingsvc.Service
	types        endpointtypesvc.Service
	clusterTypes clusterendpointtypesvc.Service
	logger       *slog.Logger
}

var _ Service = (*service)(nil)

// NewService creates the rollout service. Plan changes need endpoint update
// and steps need binding update in the environment. The type services only
// read what a type can render, and allBindings only counts the Endpoint's
// environments a caller can't view, so they are passed without
// authorization.
func NewService(
	endpoints endpointsvc.Service,
	releases endpointreleasesvc.Service,
	bindings endpointreleasebindingsvc.Service,
	allBindings endpointreleasebindingsvc.Service,
	types endpointtypesvc.Service,
	clusterTypes clusterendpointtypesvc.Service,
	logger *slog.Logger,
) Service {
	return &service{
		endpoints: endpoints, releases: releases, bindings: bindings, allBindings: allBindings,
		types: types, clusterTypes: clusterTypes, logger: logger,
	}
}

// environment is the binding a step changes, read with the Endpoint's other
// bindings and their releases so the step's status needs no second read.
type environment struct {
	binding  *openchoreov1alpha1.EndpointReleaseBinding
	refs     []openchoreov1alpha1.EndpointBackendRef
	bindings []openchoreov1alpha1.EndpointReleaseBinding
	releases releases
}

// releases holds each release a call has read, by name, so environments on
// the same release read it once; nil for one that is gone.
type releases map[string]*openchoreov1alpha1.EndpointRelease

func (s *service) Get(ctx context.Context, namespaceName, endpointName string) (*Status, error) {
	ep, err := s.endpoints.GetEndpoint(ctx, namespaceName, endpointName)
	if err != nil {
		return nil, err
	}
	return s.status(ctx, namespaceName, ep)
}

func (s *service) Start(ctx context.Context, namespaceName, endpointName string, req StartRequest) (*Status, error) {
	return onFresh(func() (*Status, error) { return s.start(ctx, namespaceName, endpointName, req) })
}

// dropStaleOverrides removes overrides naming a new candidate from bindings
// whose release doesn't have it, such as a weight left by an earlier rollout
// of a backend with the same name. The controller ignores them until a
// release with the candidate is pinned, and would then apply them instead of
// the candidate's 0, so they go before the Endpoint is saved and the release
// is cut. Overrides that are stale are stale either way, so a failed save or
// a retry loses nothing. Best effort: one that can't be cleared is logged.
func (s *service) dropStaleOverrides(ctx context.Context, namespaceName, endpointName string, candidates []string) {
	read := releases{}
	s.editOverrides(ctx, namespaceName, endpointName, "clear a stale override of the new candidate",
		func(b *openchoreov1alpha1.EndpointReleaseBinding) ([]openchoreov1alpha1.EndpointBackendOverride, bool, error) {
			refs, err := s.releaseRefs(ctx, namespaceName, b, read)
			if err != nil {
				return nil, false, err
			}
			kept := slices.DeleteFunc(slices.Clone(b.Spec.BackendOverrides), func(o openchoreov1alpha1.EndpointBackendOverride) bool {
				inRelease := slices.ContainsFunc(refs, func(ref openchoreov1alpha1.EndpointBackendRef) bool { return ref.Name == o.Name })
				return slices.Contains(candidates, o.Name) && !inRelease
			})
			return kept, len(kept) != len(b.Spec.BackendOverrides), nil
		})
}

func (s *service) start(ctx context.Context, namespaceName, endpointName string, req StartRequest) (*Status, error) {
	ep, err := s.endpoints.GetEndpoint(ctx, namespaceName, endpointName)
	if err != nil {
		return nil, err
	}
	if ep.Spec.Rollout != nil {
		return nil, conflict("a %s rollout from %s is already planned; finish it first", ep.Spec.Rollout.Strategy, ep.Spec.Rollout.Stable)
	}
	if err := s.seesEveryEnvironment(ctx, namespaceName, endpointName); err != nil {
		return nil, err
	}
	for _, b := range req.Backends {
		if i := slices.IndexFunc(ep.Spec.BackendRefs, func(r openchoreov1alpha1.EndpointBackendRef) bool { return r.Name == b.Name }); i >= 0 {
			// A backend given by name only keeps what it routes to.
			if b.Component == nil {
				b.Kind, b.Component = ep.Spec.BackendRefs[i].Kind, ep.Spec.BackendRefs[i].Component
			}
			ep.Spec.BackendRefs[i] = b
		} else {
			ep.Spec.BackendRefs = append(ep.Spec.BackendRefs, b)
		}
	}
	ep.Spec.Rollout = req.Rollout.DeepCopy()
	if err := endpointrollout.Validate(ep.Spec.Rollout, ep.Spec.BackendRefs); err != nil {
		return nil, &services.ValidationError{Msg: err.Error()}
	}
	if problems := endpointpipeline.MatchProblems(ep.Spec.BackendRefs); len(problems) > 0 {
		return nil, &services.ValidationError{Msg: strings.Join(problems, "; ")}
	}
	if err := s.renderable(ctx, namespaceName, ep); err != nil {
		return nil, err
	}
	if busy, err := s.candidateTraffic(ctx, namespaceName, ep); err != nil || len(busy) > 0 {
		if err != nil {
			return nil, err
		}
		return nil, conflict("%s; move it to 0 there first, as a rollout starts with no traffic on its candidates",
			strings.Join(busy, "; "))
	}
	if !req.DryRun {
		s.dropStaleOverrides(ctx, namespaceName, endpointName, candidateNames(ep.Spec.Rollout))
	}
	return s.writeEndpoint(ctx, namespaceName, ep, req.DryRun)
}

// candidateTraffic describes the environments where a new plan's candidate
// already gets traffic, by its weight or mirror share on the release there,
// or an override of it. A candidate's match alone doesn't count.
func (s *service) candidateTraffic(ctx context.Context, namespaceName string, ep *openchoreov1alpha1.Endpoint) ([]string, error) {
	bindings, err := s.listBindings(ctx, namespaceName, ep.Name)
	if err != nil {
		return nil, err
	}
	read := releases{}
	var out []string
	for i := range bindings {
		b := &bindings[i]
		refs, err := s.releaseRefs(ctx, namespaceName, b, read)
		if err != nil {
			return nil, err
		}
		weights, mirrors := endpointrollout.EffectiveWeights(refs, b.Spec.BackendOverrides)
		for _, c := range ep.Spec.Rollout.Candidates {
			if weights[c.Name] > 0 || mirrors[c.Name] > 0 {
				out = append(out, fmt.Sprintf("%s already gets traffic in %s", c.Name, b.Spec.Environment))
			}
		}
	}
	return out, nil
}

// renderable refuses a rollout that uses a routing feature the Endpoint's
// type does not declare, which the controller would refuse to release. A type
// that can't be read is left for the controller to report.
func (s *service) renderable(ctx context.Context, namespaceName string, ep *openchoreov1alpha1.Endpoint) error {
	var declared []openchoreov1alpha1.EndpointCapability
	switch ref := ep.Spec.Type; ref.Kind {
	case openchoreov1alpha1.EndpointTypeRefKindClusterEndpointType:
		t, err := s.clusterTypes.GetClusterEndpointType(ctx, ref.Name)
		if errors.Is(err, clusterendpointtypesvc.ErrClusterEndpointTypeNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		declared = t.Spec.Capabilities
	default:
		t, err := s.types.GetEndpointType(ctx, namespaceName, ref.Name)
		if errors.Is(err, endpointtypesvc.ErrEndpointTypeNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		declared = t.Spec.Capabilities
	}
	problems := endpointpipeline.CapabilityProblems(declared, ep.Spec.BackendRefs)
	if slices.Contains(declared, openchoreov1alpha1.EndpointCapabilityMirror) &&
		!slices.Contains(declared, openchoreov1alpha1.EndpointCapabilityMirrorPercent) &&
		len(problems) == 0 && mirrorsAShare(ep.Spec.Rollout) {
		problems = append(problems, fmt.Sprintf("a mirror step below 100%% needs the %s capability",
			openchoreov1alpha1.EndpointCapabilityMirrorPercent))
	}
	if len(problems) == 0 {
		return nil
	}
	return &services.ValidationError{Msg: fmt.Sprintf("%s %q can't render this rollout: %s",
		ep.Spec.Type.Kind, ep.Spec.Type.Name, strings.Join(problems, "; "))}
}

// mirrorsAShare reports whether a mirror plan copies part of the requests at
// some step.
func mirrorsAShare(r *openchoreov1alpha1.EndpointRollout) bool {
	if r == nil || r.Strategy != openchoreov1alpha1.EndpointRolloutMirror || r.Mirror == nil {
		return false
	}
	for _, st := range r.Mirror.Steps {
		for _, b := range st.Backends {
			if b.Percent > 0 && b.Percent < 100 {
				return true
			}
		}
	}
	return false
}

func (s *service) Step(ctx context.Context, namespaceName, endpointName string, req StepRequest) (*Status, error) {
	return onFresh(func() (*Status, error) { return s.step(ctx, namespaceName, endpointName, req) })
}

func (s *service) step(ctx context.Context, namespaceName, endpointName string, req StepRequest) (*Status, error) {
	ep, err := s.requireRollout(ctx, namespaceName, endpointName)
	if err != nil {
		return nil, err
	}
	env, err := s.environment(ctx, namespaceName, ep, req.Environment)
	if err != nil {
		return nil, err
	}
	r := ep.Spec.Rollout
	stages := endpointrollout.Stages(r)
	at := endpointrollout.Position(r, env.refs, env.binding.Spec.BackendOverrides)
	if req.ExpectedStep != nil && *req.ExpectedStep != at {
		return nil, conflict("%s is at %s, not step %d; check its status and retry", req.Environment, describe(at), *req.ExpectedStep)
	}
	if req.To == nil && r.Strategy == openchoreov1alpha1.EndpointRolloutABTest {
		return nil, conflict("an A/B test has no steps; finish it keeping the version you want")
	}
	to := endpointrollout.Next(r, env.refs, env.binding.Spec.BackendOverrides)
	if req.To != nil {
		to = *req.To
	}
	if to < 0 || to >= len(stages) {
		if req.To == nil {
			return nil, conflict("%s is at the last step; finish the rollout", req.Environment)
		}
		return nil, &services.ValidationError{Msg: fmt.Sprintf("step must be from 0 to %d", len(stages)-1)}
	}
	return s.setStage(ctx, namespaceName, ep, env, stages[to], req.DryRun)
}

func (s *service) Abort(ctx context.Context, namespaceName, endpointName string, req AbortRequest) (*Status, error) {
	return s.Step(ctx, namespaceName, endpointName, StepRequest{Environment: req.Environment, To: ptr(0), DryRun: req.DryRun})
}

func (s *service) Finish(ctx context.Context, namespaceName, endpointName string, req FinishRequest) (*Status, error) {
	var leaving []string
	var strategy openchoreov1alpha1.EndpointRolloutStrategy
	status, err := onFresh(func() (*Status, error) {
		var status *Status
		var err error
		status, strategy, leaving, err = s.finish(ctx, namespaceName, endpointName, req)
		return status, err
	})
	if err != nil || req.DryRun || strategy != openchoreov1alpha1.EndpointRolloutMirror {
		return status, err
	}
	s.dropMirrorOverrides(ctx, namespaceName, endpointName, leaving)
	return status, nil
}

func (s *service) finish(
	ctx context.Context, namespaceName, endpointName string, req FinishRequest,
) (*Status, openchoreov1alpha1.EndpointRolloutStrategy, []string, error) {
	ep, err := s.requireRollout(ctx, namespaceName, endpointName)
	if err != nil {
		return nil, "", nil, err
	}
	r := ep.Spec.Rollout
	refs, leaving, err := endpointrollout.Finish(r, ep.Spec.BackendRefs, req.Keep)
	if err != nil {
		return nil, "", nil, &services.ValidationError{Msg: err.Error()}
	}
	if err := s.seesEveryEnvironment(ctx, namespaceName, ep.Name); err != nil {
		return nil, "", nil, err
	}
	bindings, err := s.listBindings(ctx, namespaceName, ep.Name)
	if err != nil {
		return nil, "", nil, err
	}
	if err := s.finishChecks(ctx, namespaceName, r, bindings, req.Keep, leaving); err != nil {
		return nil, "", nil, err
	}
	// A step saved since the bindings were read could change what they
	// route. Kubernetes can't save the Endpoint on condition of the bindings,
	// so read them again right before it and start over, as a step does,
	// when any moved.
	if !req.DryRun {
		if err := s.bindingsUnchanged(ctx, namespaceName, ep.Name, bindings); err != nil {
			return nil, "", nil, err
		}
		// An environment hidden from the caller and created since the first
		// check is in no list the checks read.
		if err := s.seesEveryEnvironment(ctx, namespaceName, ep.Name); err != nil {
			return nil, "", nil, err
		}
	}
	if !req.DryRun {
		s.clearFinishedOverrides(ctx, namespaceName, ep.Name, req.Keep, leaving)
	}
	ep.Spec.BackendRefs = refs
	ep.Spec.Rollout = nil
	status, err := s.writeEndpoint(ctx, namespaceName, ep, req.DryRun)
	return status, r.Strategy, leaving, err
}

// finishChecks refuses a finish that would move traffic in an environment.
// It only reads, sharing one read of each release.
func (s *service) finishChecks(
	ctx context.Context, namespaceName string, r *openchoreov1alpha1.EndpointRollout,
	bindings []openchoreov1alpha1.EndpointReleaseBinding, keep string, leaving []string,
) error {
	if blocked := notRouting(bindings); len(blocked) > 0 {
		return conflict("%s; fix that first, as what it routes now can't be read", strings.Join(blocked, "; "))
	}
	read := releases{}
	if r.Strategy == openchoreov1alpha1.EndpointRolloutABTest {
		split, err := s.splitByHand(ctx, namespaceName, bindings, r, read)
		if err != nil {
			return err
		}
		if len(split) > 0 {
			return conflict("%s; abort there, or ship the candidate there, first", strings.Join(split, "; "))
		}
	}
	switch {
	case r.Strategy != openchoreov1alpha1.EndpointRolloutABTest:
		serving, err := s.serving(ctx, namespaceName, bindings, leaving, read)
		if err != nil {
			return err
		}
		if len(serving) > 0 {
			return conflict("%s still gets traffic; move it to 0%% there first", strings.Join(serving, ", "))
		}
	case keep == r.Stable:
		// An A/B candidate keeps its match's requests until the end, so it
		// may leave with traffic, but stable must have some to take them.
		shipped, err := s.shippedAway(ctx, namespaceName, bindings, r, read)
		if err != nil {
			return err
		}
		if len(shipped) > 0 {
			return conflict("%s; abort there first, or finish keeping the candidate", strings.Join(shipped, "; "))
		}
	default:
		held, err := s.heldByOverride(ctx, namespaceName, bindings, keep, read)
		if err != nil {
			return err
		}
		if len(held) > 0 {
			return conflict("%s; deploy the latest release there first, as clearing %s's 0 would send it traffic before then",
				strings.Join(held, "; "), keep)
		}
	}
	if keep != r.Stable {
		if gone := unresolved(bindings, keep); len(gone) > 0 {
			return conflict("%s isn't deployed in %s; deploy it there first, as the finished release sends it %s's traffic",
				keep, strings.Join(gone, ", "), r.Stable)
		}
	}
	moved, err := s.shareMoves(ctx, namespaceName, bindings, r, keep, read)
	if err != nil {
		return err
	}
	if len(moved) > 0 {
		return conflict("%s; move all of %s's weight to %s there first, so its share carries over",
			strings.Join(moved, "; "), r.Stable, keep)
	}
	return nil
}

// shareMoves describes the environments where the kept backend would not get
// the share stable and the candidates have now once a finished release is
// deployed there. Overrides an Abort scaled stay, while the release gives an
// A/B candidate kept from step 0 stable's own weight, which can be far
// smaller than the scaled ones next to it.
func (s *service) shareMoves(
	ctx context.Context, namespaceName string, bindings []openchoreov1alpha1.EndpointReleaseBinding,
	r *openchoreov1alpha1.EndpointRollout, keep string, read releases,
) ([]string, error) {
	inRollout := append([]string{r.Stable}, candidateNames(r)...)
	var out []string
	for i := range bindings {
		b := &bindings[i]
		refs, err := s.releaseRefs(ctx, namespaceName, b, read)
		if err != nil {
			return nil, err
		}
		if len(refs) == 0 || len(endpointrollout.Missing(r, refs)) > 0 {
			continue
		}
		finished, leaving, err := endpointrollout.Finish(r, refs, keep)
		if err != nil {
			return nil, err
		}
		now, _ := endpointrollout.EffectiveWeights(refs, b.Spec.BackendOverrides)
		then, _ := endpointrollout.EffectiveWeights(finished, finishedOverrides(refs, b.Spec.BackendOverrides, keep, leaving))
		var pair, total, totalThen int64
		for _, ref := range refs {
			total += int64(now[ref.Name])
			if slices.Contains(inRollout, ref.Name) {
				pair += int64(now[ref.Name])
			}
		}
		for _, ref := range finished {
			totalThen += int64(then[ref.Name])
		}
		if total == 0 || totalThen == 0 || int64(then[keep])*total == pair*totalThen {
			continue
		}
		out = append(out, fmt.Sprintf("in %s, %s would get %.1f%% of the traffic once the finished release is deployed there, not the %.1f%% the rollout's backends get now",
			b.Spec.Environment, keep, float64(then[keep])*100/float64(totalThen), float64(pair)*100/float64(total)))
	}
	return out, nil
}

// notRouting describes the bindings that can't render their latest change:
// their backends didn't resolve, or rendering failed after they did. The
// controller then keeps the route it had but reports the weights it was asked
// for, so their status doesn't say which backends still get traffic. A
// binding with no release yet routes nothing, so it isn't one of them.
func notRouting(bindings []openchoreov1alpha1.EndpointReleaseBinding) []string {
	var out []string
	for i := range bindings {
		conditions := bindings[i].Status.Conditions
		c := meta.FindStatusCondition(conditions, "BackendsResolved")
		if c == nil || c.Status != metav1.ConditionFalse {
			c = meta.FindStatusCondition(conditions, "Synced")
		}
		if c != nil && c.Status == metav1.ConditionFalse && c.Reason != "EndpointReleaseNotSet" {
			out = append(out, fmt.Sprintf("%s isn't routing its latest change (%s)", bindings[i].Spec.Environment, c.Message))
		}
	}
	return out
}

// unresolved lists the environments that report name as a backend they
// can't resolve.
func unresolved(bindings []openchoreov1alpha1.EndpointReleaseBinding, name string) []string {
	var out []string
	for i := range bindings {
		if slices.ContainsFunc(bindings[i].Status.Backends, func(b openchoreov1alpha1.ResolvedEndpointBackend) bool {
			return b.Name == name && !b.Resolved
		}) {
			out = append(out, bindings[i].Spec.Environment)
		}
	}
	return out
}

// bindingsUnchanged reports a conflict when the Endpoint's bindings are no
// longer the ones read: one was saved, added or removed since.
func (s *service) bindingsUnchanged(
	ctx context.Context, namespaceName, endpointName string, read []openchoreov1alpha1.EndpointReleaseBinding,
) error {
	now, err := s.listBindings(ctx, namespaceName, endpointName)
	if err != nil {
		return err
	}
	if len(now) != len(read) {
		return apierrors.NewConflict(schema.GroupResource{Group: openchoreov1alpha1.GroupVersion.Group, Resource: "endpointreleasebindings"},
			endpointName, errors.New("an environment was added or removed since they were read"))
	}
	for i := range read {
		j := slices.IndexFunc(now, func(b openchoreov1alpha1.EndpointReleaseBinding) bool { return b.Name == read[i].Name })
		if j < 0 {
			return apierrors.NewConflict(schema.GroupResource{Group: openchoreov1alpha1.GroupVersion.Group, Resource: "endpointreleasebindings"},
				read[i].Name, errors.New("it was removed since it was read"))
		}
		if err := unchanged(&read[i], &now[j], "endpointreleasebindings"); err != nil {
			return err
		}
	}
	return nil
}

// serving lists the leaving backends that get traffic in an environment, by
// what the binding asks for or what it still routes, since a step just
// written may not be applied yet.
func (s *service) serving(
	ctx context.Context, namespaceName string, bindings []openchoreov1alpha1.EndpointReleaseBinding, leaving []string,
	read releases,
) ([]string, error) {
	var out []string
	for i := range bindings {
		b := &bindings[i]
		refs, err := s.releaseRefs(ctx, namespaceName, b, read)
		if err != nil {
			return nil, err
		}
		wanted, _ := endpointrollout.EffectiveWeights(refs, b.Spec.BackendOverrides)
		for _, name := range leaving {
			routed := slices.ContainsFunc(b.Status.Backends, func(rb openchoreov1alpha1.ResolvedEndpointBackend) bool {
				return rb.Name == name && rb.Weight > 0
			})
			if wanted[name] > 0 || routed {
				out = append(out, fmt.Sprintf("%s in %s", name, b.Spec.Environment))
			}
		}
	}
	return out, nil
}

// shippedAway describes the environments where an A/B candidate was given
// everyone's traffic and stable none. Finishing with stable there would leave
// its 0 override as the only weight, and dropping that override would split
// traffic with the candidate wherever the old release is still pinned.
func (s *service) shippedAway(
	ctx context.Context, namespaceName string, bindings []openchoreov1alpha1.EndpointReleaseBinding,
	r *openchoreov1alpha1.EndpointRollout, read releases,
) ([]string, error) {
	var out []string
	for i := range bindings {
		b := &bindings[i]
		refs, err := s.releaseRefs(ctx, namespaceName, b, read)
		if err != nil {
			return nil, err
		}
		if len(endpointrollout.Missing(r, refs)) > 0 {
			continue
		}
		weights, _ := endpointrollout.EffectiveWeights(refs, b.Spec.BackendOverrides)
		for _, c := range r.Candidates {
			if weights[r.Stable] == 0 && weights[c.Name] > 0 {
				out = append(out, fmt.Sprintf("%s serves everyone in %s, where %s gets no traffic",
					c.Name, b.Spec.Environment, r.Stable))
			}
		}
	}
	return out, nil
}

// splitByHand describes the environments where stable and an A/B candidate
// both have weight, which only weights set by hand give them. Either finish
// would shift traffic there: the kept backend's override keeps its own part of
// the pair's share when the finished release gives it the whole share.
func (s *service) splitByHand(
	ctx context.Context, namespaceName string, bindings []openchoreov1alpha1.EndpointReleaseBinding,
	r *openchoreov1alpha1.EndpointRollout, read releases,
) ([]string, error) {
	var out []string
	for i := range bindings {
		b := &bindings[i]
		refs, err := s.releaseRefs(ctx, namespaceName, b, read)
		if err != nil {
			return nil, err
		}
		if len(endpointrollout.Missing(r, refs)) > 0 {
			continue
		}
		weights, _ := endpointrollout.EffectiveWeights(refs, b.Spec.BackendOverrides)
		for _, c := range r.Candidates {
			if weights[r.Stable] > 0 && weights[c.Name] > 0 {
				out = append(out, fmt.Sprintf("%s and %s both have weight in %s, set by hand", r.Stable, c.Name, b.Spec.Environment))
			}
		}
	}
	return out, nil
}

// heldByOverride describes the environments whose pinned release gives name a
// weight above 0 that only a 0 override holds down. A finish keeping name
// clears those overrides, so there it would send name traffic before the new
// release is deployed.
func (s *service) heldByOverride(
	ctx context.Context, namespaceName string, bindings []openchoreov1alpha1.EndpointReleaseBinding, name string,
	read releases,
) ([]string, error) {
	var out []string
	for i := range bindings {
		b := &bindings[i]
		if !holdsAtZero(b.Spec.BackendOverrides, name) {
			continue
		}
		refs, err := s.releaseRefs(ctx, namespaceName, b, read)
		if err != nil {
			return nil, err
		}
		if own, _ := endpointrollout.EffectiveWeights(refs, nil); own[name] > 0 {
			out = append(out, fmt.Sprintf("%s runs release %s, where %s has weight %d",
				b.Spec.Environment, b.Spec.EndpointRelease, name, own[name]))
		}
	}
	return out, nil
}

// holdsAtZero reports whether the overrides set name's weight to 0.
func holdsAtZero(overrides []openchoreov1alpha1.EndpointBackendOverride, name string) bool {
	return slices.ContainsFunc(overrides, func(o openchoreov1alpha1.EndpointBackendOverride) bool {
		return o.Name == name && o.Mirror == nil && o.Weight != nil && *o.Weight == 0
	})
}

// onFresh runs a call again from a fresh read when what it read changed
// before it saved, so it never saves over someone else's change. A conflict
// that outlasts the retries is refused as one.
func onFresh(call func() (*Status, error)) (*Status, error) {
	var status *Status
	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		var err error
		status, err = call()
		return err
	})
	if apierrors.IsConflict(err) {
		return nil, conflict("the Endpoint changed while saving; check its status and retry")
	}
	return status, err
}

// unchanged reports a conflict when the stored object moved past the one a
// call read, since the update services save the whole spec they are given.
func unchanged(read, stored metav1.Object, resource string) error {
	if read.GetResourceVersion() == stored.GetResourceVersion() {
		return nil
	}
	return apierrors.NewConflict(schema.GroupResource{Group: openchoreov1alpha1.GroupVersion.Group, Resource: resource},
		read.GetName(), errors.New("it changed since it was read"))
}

// planUnchanged reports a conflict when the Endpoint's spec moved past the
// one a step was computed from, so the step starts over from the new plan. A
// status update alone doesn't count.
func (s *service) planUnchanged(ctx context.Context, namespaceName string, read *openchoreov1alpha1.Endpoint) error {
	stored, err := s.endpoints.GetEndpoint(ctx, namespaceName, read.Name)
	if err != nil {
		return err
	}
	if equality.Semantic.DeepEqual(read.Spec, stored.Spec) {
		return nil
	}
	return unchanged(read, stored, "endpoints")
}

// dropMirrorOverrides removes the mirror shares the rollout set on the
// backends leaving the Endpoint, so a backend added again later doesn't
// inherit them. Weights are clearFinishedOverrides' to clear. A share on a
// binding whose release still has the mirror stays: it holds the release's
// own share down there, and once a release without it is pinned the
// controller ignores it. The Endpoint is already finished, so a binding that
// can't be cleaned is logged rather than failing the call.
func (s *service) dropMirrorOverrides(ctx context.Context, namespaceName, endpointName string, leaving []string) {
	read := releases{}
	s.editOverrides(ctx, namespaceName, endpointName, "clear the finished mirror's share",
		func(b *openchoreov1alpha1.EndpointReleaseBinding) ([]openchoreov1alpha1.EndpointBackendOverride, bool, error) {
			refs, err := s.releaseRefs(ctx, namespaceName, b, read)
			if err != nil {
				return nil, false, err
			}
			gone := slices.DeleteFunc(slices.Clone(leaving), func(name string) bool {
				return slices.ContainsFunc(refs, func(ref openchoreov1alpha1.EndpointBackendRef) bool {
					return ref.Name == name && ref.Mirror != nil
				})
			})
			kept, changed := withoutMirrors(b.Spec.BackendOverrides, gone)
			return kept, changed, nil
		})
}

// clearFinishedOverrides removes the overrides a finished rollout leaves on
// each binding, wherever removing one changes nothing on the release the
// binding runs, so routing stays as it is even when the Endpoint save then
// fails. A leaving backend's 0 goes where that release gives it 0 too. The
// kept backend's goes where it is then the only backend with weight, or where
// it holds a 0 the release also gives it, as an A/B Abort leaves it. Overrides
// that still route the pinned release stay until a release without the
// leaving backends is deployed there; mirror shares are dropMirrorOverrides'
// to clear. They go before the
// Endpoint is saved, so autoDeploy never pins the new release with them.
func (s *service) clearFinishedOverrides(ctx context.Context, namespaceName, endpointName, keep string, leaving []string) {
	read := releases{}
	s.editOverrides(ctx, namespaceName, endpointName, "clear the finished rollout's overrides",
		func(b *openchoreov1alpha1.EndpointReleaseBinding) ([]openchoreov1alpha1.EndpointBackendOverride, bool, error) {
			if len(b.Spec.BackendOverrides) == 0 {
				return nil, false, nil
			}
			refs, err := s.releaseRefs(ctx, namespaceName, b, read)
			if err != nil {
				return nil, false, err
			}
			kept := finishedOverrides(refs, b.Spec.BackendOverrides, keep, leaving)
			return kept, len(kept) != len(b.Spec.BackendOverrides), nil
		})
}

// finishedOverrides is what clearFinishedOverrides keeps of a binding's
// overrides on the release with refs.
func finishedOverrides(
	refs []openchoreov1alpha1.EndpointBackendRef, overrides []openchoreov1alpha1.EndpointBackendOverride,
	keep string, leaving []string,
) []openchoreov1alpha1.EndpointBackendOverride {
	own, _ := endpointrollout.EffectiveWeights(refs, nil)
	kept := slices.DeleteFunc(slices.Clone(overrides), func(o openchoreov1alpha1.EndpointBackendOverride) bool {
		return holdsAtZero([]openchoreov1alpha1.EndpointBackendOverride{o}, o.Name) &&
			slices.Contains(leaving, o.Name) && own[o.Name] == 0
	})
	after, _ := endpointrollout.EffectiveWeights(refs, kept)
	alone := !slices.ContainsFunc(refs, func(ref openchoreov1alpha1.EndpointBackendRef) bool {
		return ref.Name != keep && after[ref.Name] > 0
	})
	return slices.DeleteFunc(kept, func(o openchoreov1alpha1.EndpointBackendOverride) bool {
		if o.Name != keep || o.Mirror != nil || o.Weight == nil {
			return false
		}
		return (alone && *o.Weight > 0 && own[keep] > 0) || (*o.Weight == 0 && own[keep] == 0)
	})
}

// editOverrides saves the overrides edit gives each of the Endpoint's
// bindings it changes. Only those are read again and saved, retrying on a
// conflict since autoDeploy may be moving them to a new release. Best effort:
// a binding that can't be saved is logged as what couldn't be done.
func (s *service) editOverrides(
	ctx context.Context, namespaceName, endpointName, what string,
	edit func(*openchoreov1alpha1.EndpointReleaseBinding) ([]openchoreov1alpha1.EndpointBackendOverride, bool, error),
) {
	bindings, err := s.listBindings(ctx, namespaceName, endpointName)
	if err != nil {
		s.logger.Warn("Could not "+what, "endpoint", endpointName, "error", err)
		return
	}
	for i := range bindings {
		_, changed, err := edit(&bindings[i])
		if err == nil && changed {
			name := bindings[i].Name
			err = retry.RetryOnConflict(retry.DefaultRetry, func() error {
				b, err := s.bindings.GetEndpointReleaseBinding(ctx, namespaceName, name)
				if err != nil {
					return err
				}
				kept, changed, err := edit(b)
				if err != nil || !changed {
					return err
				}
				b.Spec.BackendOverrides = kept
				_, err = s.bindings.UpdateEndpointReleaseBinding(ctx, namespaceName, b)
				return err
			})
		}
		if err != nil {
			s.logger.Warn("Could not "+what, "endpoint", endpointName,
				"environment", bindings[i].Spec.Environment, "error", err)
		}
	}
}

func withoutMirrors(
	overrides []openchoreov1alpha1.EndpointBackendOverride, leaving []string,
) ([]openchoreov1alpha1.EndpointBackendOverride, bool) {
	kept := make([]openchoreov1alpha1.EndpointBackendOverride, 0, len(overrides))
	changed := false
	for _, o := range overrides {
		if o.Mirror != nil && slices.Contains(leaving, o.Name) {
			changed = true
			if o.Weight == nil {
				continue
			}
			o.Mirror = nil
		}
		kept = append(kept, o)
	}
	return kept, changed
}

// undeployed describes the rollout backends a step leaves with traffic,
// including weight the release gives them, that the environment reports it
// can't resolve, or doesn't report yet. A step forward also needs the report
// to be for the binding's current spec, so it isn't one from before a new
// release was pinned; back to step 0 doesn't wait for one. The controller
// keeps the last route while a backend with traffic is unresolved, so such a
// step would not apply.
func undeployed(
	r *openchoreov1alpha1.EndpointRollout,
	binding *openchoreov1alpha1.EndpointReleaseBinding,
	refs []openchoreov1alpha1.EndpointBackendRef,
	stage endpointrollout.Stage,
	overrides []openchoreov1alpha1.EndpointBackendOverride,
) []string {
	weights, mirrors := endpointrollout.EffectiveWeights(refs, overrides)
	c := meta.FindStatusCondition(binding.Status.Conditions, "BackendsResolved")
	current := stage.Index == 0 || (c != nil && c.ObservedGeneration == binding.Generation)
	var gone []string
	for _, name := range append([]string{r.Stable}, candidateNames(r)...) {
		if weights[name] == 0 && mirrors[name] == 0 {
			continue
		}
		i := slices.IndexFunc(binding.Status.Backends, func(b openchoreov1alpha1.ResolvedEndpointBackend) bool {
			return b.Name == name
		})
		if i < 0 || !current {
			gone = append(gone, fmt.Sprintf("%s, which %s doesn't report yet; wait until it serves its latest change",
				name, binding.Spec.Environment))
			continue
		}
		if binding.Status.Backends[i].Resolved {
			continue
		}
		reason := binding.Status.Backends[i].Message
		if reason == "" {
			reason = binding.Status.Backends[i].Reason
		}
		gone = append(gone, fmt.Sprintf("%s, which isn't deployed in %s (%s)", name, binding.Spec.Environment, reason))
	}
	return gone
}

func candidateNames(r *openchoreov1alpha1.EndpointRollout) []string {
	names := make([]string, 0, len(r.Candidates))
	for _, c := range r.Candidates {
		names = append(names, c.Name)
	}
	return names
}

func (s *service) setStage(
	ctx context.Context,
	namespaceName string,
	ep *openchoreov1alpha1.Endpoint,
	env *environment,
	stage endpointrollout.Stage,
	dryRun bool,
) (*Status, error) {
	if env.binding.Spec.EndpointRelease == "" {
		return nil, conflict("%s has no release yet; deploy the latest release there first", env.binding.Spec.Environment)
	}
	if missing := endpointrollout.Missing(ep.Spec.Rollout, env.refs); len(missing) > 0 {
		return nil, conflict("%s runs release %s, which has no backend %s; deploy the latest release there first",
			env.binding.Spec.Environment, env.binding.Spec.EndpointRelease, strings.Join(missing, ", "))
	}
	// Back to step 0 only takes traffic off the candidates, which is safe on
	// any release; a step forward needs one that routes as the Endpoint does.
	if stage.Index > 0 {
		if changed := endpointrollout.Mismatched(ep.Spec.Rollout, ep.Spec.BackendRefs, env.refs); len(changed) > 0 {
			return nil, conflict("%s runs release %s, where %s; deploy the latest release there first",
				env.binding.Spec.Environment, env.binding.Spec.EndpointRelease, strings.Join(changed, "; "))
		}
	}
	before := env.binding.Spec.BackendOverrides
	after, err := endpointrollout.Overrides(ep.Spec.Rollout, env.refs, before, stage)
	if errors.Is(err, endpointrollout.ErrNoRolloutTraffic) || errors.Is(err, endpointrollout.ErrStageTooFine) {
		return nil, conflict("in %s, %v", env.binding.Spec.Environment, err)
	}
	if err != nil {
		return nil, err
	}
	if err := s.typeRenders(ctx, namespaceName, env, after); err != nil {
		return nil, err
	}
	if gone := undeployed(ep.Spec.Rollout, env.binding, env.refs, stage, after); len(gone) > 0 {
		return nil, conflict("this step sends traffic to %s; deploy it there first, or move traffic away from it",
			strings.Join(gone, " and "))
	}
	change := &Change{Environment: env.binding.Spec.Environment, Before: before, After: after}
	// The status is read before the write, so every release it needs is
	// cached and the one after it reads nothing: a step that was saved never
	// reports a failure that would invite a second step.
	if _, err := s.statusOf(ctx, namespaceName, ep, env.bindings, env.releases); err != nil {
		return nil, err
	}
	if !dryRun {
		stored, err := s.bindings.GetEndpointReleaseBinding(ctx, namespaceName, env.binding.Name)
		if err != nil {
			return nil, err
		}
		if err := unchanged(env.binding, stored, "endpointreleasebindings"); err != nil {
			return nil, err
		}
		if err := s.planUnchanged(ctx, namespaceName, ep); err != nil {
			return nil, err
		}
		env.binding.Spec.BackendOverrides = after
		updated, err := s.bindings.UpdateEndpointReleaseBinding(ctx, namespaceName, env.binding)
		if err != nil {
			return nil, err
		}
		*env.binding = *updated
		s.logger.Info("Endpoint rollout stepped", "namespace", namespaceName, "endpoint", ep.Name,
			"environment", env.binding.Spec.Environment, "step", stage.Index)
	}
	status, err := s.statusOf(ctx, namespaceName, ep, env.bindings, env.releases)
	if err != nil {
		return nil, err
	}
	status.Change = change
	return status, nil
}

// typeRenders refuses overrides the pinned release's EndpointType snapshot
// can't render, such as a mirror share below 100 where it declares Mirror
// alone. The controller would refuse them and keep the environment's old
// route, so the step would report a change that never applies.
func (s *service) typeRenders(
	ctx context.Context, namespaceName string, env *environment, overrides []openchoreov1alpha1.EndpointBackendOverride,
) error {
	release, err := s.release(ctx, namespaceName, env.binding, env.releases)
	if release == nil || err != nil {
		return err
	}
	_, mirrors := endpointrollout.EffectiveWeights(env.refs, overrides)
	refs := make([]openchoreov1alpha1.EndpointBackendRef, 0, len(env.refs))
	for _, ref := range env.refs {
		if ref.Mirror != nil {
			ref.Mirror = &openchoreov1alpha1.EndpointMirror{Percent: mirrors[ref.Name]}
		}
		refs = append(refs, ref)
	}
	problems := endpointpipeline.CapabilityProblems(release.Spec.EndpointType.Spec.Capabilities, refs)
	if len(problems) == 0 {
		return nil
	}
	return conflict("in %s, release %s's EndpointType can't render this step: %s; deploy a release whose type declares it",
		env.binding.Spec.Environment, release.Name, strings.Join(problems, "; "))
}

func (s *service) writeEndpoint(ctx context.Context, namespaceName string, ep *openchoreov1alpha1.Endpoint, dryRun bool) (*Status, error) {
	if dryRun {
		status, err := s.status(ctx, namespaceName, ep)
		if err != nil {
			return nil, err
		}
		preview := ep.DeepCopy()
		endpointsvc.WidenOwner(preview)
		status.Change = &Change{Endpoint: &preview.Spec}
		return status, nil
	}
	stored, err := s.endpoints.GetEndpoint(ctx, namespaceName, ep.Name)
	if err != nil {
		return nil, err
	}
	if err := unchanged(ep, stored, "endpoints"); err != nil {
		return nil, err
	}
	updated, err := s.endpoints.UpdateEndpoint(ctx, namespaceName, ep)
	if err != nil {
		return nil, err
	}
	s.logger.Info("Endpoint rollout plan updated", "namespace", namespaceName, "endpoint", updated.Name)
	status, err := s.status(ctx, namespaceName, updated)
	if err != nil {
		// The Endpoint is saved, so the call succeeded; a retry would be refused.
		s.logger.Warn("Could not read the rollout status after saving", "endpoint", updated.Name, "error", err)
		status = &Status{Endpoint: updated.Name, Rollout: updated.Spec.Rollout, Environments: []EnvironmentStatus{}}
	}
	status.Change = &Change{Endpoint: &updated.Spec}
	return status, nil
}

func (s *service) requireRollout(ctx context.Context, namespaceName, endpointName string) (*openchoreov1alpha1.Endpoint, error) {
	ep, err := s.endpoints.GetEndpoint(ctx, namespaceName, endpointName)
	if err != nil {
		return nil, err
	}
	if ep.Spec.Rollout == nil {
		return nil, ErrNoRollout
	}
	return ep, nil
}

func (s *service) environment(ctx context.Context, namespaceName string, ep *openchoreov1alpha1.Endpoint, name string) (*environment, error) {
	bindings, err := s.listBindings(ctx, namespaceName, ep.Name)
	if err != nil {
		return nil, err
	}
	var names []string
	for i := range bindings {
		if bindings[i].Spec.Environment == name {
			names = append(names, bindings[i].Name)
		}
	}
	if len(names) > 1 {
		return nil, conflict("%s has %d bindings in %s (%s); remove all but one", ep.Name, len(names), name, strings.Join(names, ", "))
	}
	read := releases{}
	for i := range bindings {
		if bindings[i].Spec.Environment != name {
			continue
		}
		refs, err := s.releaseRefs(ctx, namespaceName, &bindings[i], read)
		if err != nil {
			return nil, err
		}
		return &environment{binding: &bindings[i], refs: refs, bindings: bindings, releases: read}, nil
	}
	return nil, fmt.Errorf("%w: %s is not deployed in %s", ErrEnvironmentNotBound, ep.Name, name)
}

// releaseRefs is the backends of the release an environment runs, which a
// step's weights are computed against.
func (s *service) releaseRefs(
	ctx context.Context, namespaceName string, b *openchoreov1alpha1.EndpointReleaseBinding, read releases,
) ([]openchoreov1alpha1.EndpointBackendRef, error) {
	release, err := s.release(ctx, namespaceName, b, read)
	if release == nil || err != nil {
		return nil, err
	}
	return release.Spec.BackendRefs, nil
}

// release is the release an environment runs, or nil when it has none or it
// is gone.
func (s *service) release(
	ctx context.Context, namespaceName string, b *openchoreov1alpha1.EndpointReleaseBinding, read releases,
) (*openchoreov1alpha1.EndpointRelease, error) {
	name := b.Spec.EndpointRelease
	if name == "" {
		return nil, nil
	}
	if release, ok := read[name]; ok {
		return release, nil
	}
	release, err := s.releases.GetEndpointRelease(ctx, namespaceName, name)
	switch {
	case errors.Is(err, endpointreleasesvc.ErrEndpointReleaseNotFound):
		release = nil
	case err != nil:
		return nil, err
	}
	read[name] = release
	return release, nil
}

// listBindings is the Endpoint's bindings the caller may view.
func (s *service) listBindings(ctx context.Context, namespaceName, endpointName string) ([]openchoreov1alpha1.EndpointReleaseBinding, error) {
	return pagedBindings(ctx, s.bindings, namespaceName, endpointName)
}

func pagedBindings(
	ctx context.Context, bindings endpointreleasebindingsvc.Service, namespaceName, endpointName string,
) ([]openchoreov1alpha1.EndpointReleaseBinding, error) {
	var out []openchoreov1alpha1.EndpointReleaseBinding
	opts := services.ListOptions{Limit: 100}
	for {
		page, err := bindings.ListEndpointReleaseBindings(ctx, namespaceName, endpointName, opts)
		if err != nil {
			return nil, err
		}
		out = append(out, page.Items...)
		if page.NextCursor == "" {
			return out, nil
		}
		opts.Cursor = page.NextCursor
	}
}

// seesEveryEnvironment refuses a change to the whole rollout while some of the
// Endpoint's environments are hidden from the caller: the checks that keep
// traffic safe, and the overrides it clears, would skip them. The refusal
// counts them without naming any.
func (s *service) seesEveryEnvironment(ctx context.Context, namespaceName, endpointName string) error {
	visible, err := s.listBindings(ctx, namespaceName, endpointName)
	if err != nil {
		return err
	}
	all, err := pagedBindings(ctx, s.allBindings, namespaceName, endpointName)
	if err != nil {
		return err
	}
	hidden := 0
	for i := range all {
		if !slices.ContainsFunc(visible, func(b openchoreov1alpha1.EndpointReleaseBinding) bool { return b.Name == all[i].Name }) {
			hidden++
		}
	}
	if hidden > 0 {
		return conflict("%d of this Endpoint's environments aren't visible to you, and this change affects every environment; "+
			"ask someone who can view them all", hidden)
	}
	return nil
}

func (s *service) status(ctx context.Context, namespaceName string, ep *openchoreov1alpha1.Endpoint) (*Status, error) {
	bindings, err := s.listBindings(ctx, namespaceName, ep.Name)
	if err != nil {
		return nil, err
	}
	return s.statusOf(ctx, namespaceName, ep, bindings, releases{})
}

// statusOf is where each of the given bindings stands in the Endpoint's
// rollout.
func (s *service) statusOf(
	ctx context.Context,
	namespaceName string,
	ep *openchoreov1alpha1.Endpoint,
	bindings []openchoreov1alpha1.EndpointReleaseBinding,
	read releases,
) (*Status, error) {
	status := &Status{Endpoint: ep.Name, Rollout: ep.Spec.Rollout, Environments: []EnvironmentStatus{}}
	if ep.Status.LatestRelease != nil {
		status.LatestRelease = ep.Status.LatestRelease.Name
	}
	r := ep.Spec.Rollout
	if r != nil {
		status.Stages = endpointrollout.Stages(r)
	}
	for i := range bindings {
		b := &bindings[i]
		env := EnvironmentStatus{
			Environment:     b.Spec.Environment,
			BindingName:     b.Name,
			EndpointRelease: b.Spec.EndpointRelease,
			Overrides:       b.Spec.BackendOverrides,
			Backends:        b.Status.Backends,
		}
		if env.Overrides == nil {
			env.Overrides = []openchoreov1alpha1.EndpointBackendOverride{}
		}
		if env.Backends == nil {
			env.Backends = []openchoreov1alpha1.ResolvedEndpointBackend{}
		}
		env.Ready, env.Message = ready(b)
		if r != nil {
			refs, err := s.releaseRefs(ctx, namespaceName, b, read)
			if err != nil {
				return nil, err
			}
			if env.Missing = endpointrollout.Missing(r, refs); len(env.Missing) == 0 {
				env.Step = ptr(endpointrollout.Position(r, refs, b.Spec.BackendOverrides))
				env.Percents = endpointrollout.Percents(r, refs, b.Spec.BackendOverrides)
			}
		}
		status.Environments = append(status.Environments, env)
	}
	return status, nil
}

// ready says whether the binding serves its latest spec.
func ready(b *openchoreov1alpha1.EndpointReleaseBinding) (bool, string) {
	c := meta.FindStatusCondition(b.Status.Conditions, "Ready")
	switch {
	case c == nil || c.ObservedGeneration < b.Generation:
		return false, "the latest change is still being applied"
	case c.Status != metav1.ConditionTrue:
		return false, c.Message
	}
	return true, ""
}

func describe(step int) string {
	if step < 0 {
		return "weights set by hand"
	}
	return fmt.Sprintf("step %d", step)
}

func ptr[T any](v T) *T { return &v }
