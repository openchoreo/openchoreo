// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package resourcereleasebinding

import (
	"context"
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/controller"
	resourcepipeline "github.com/openchoreo/openchoreo/internal/pipeline/resource"
)

// endpointRenderInput builds the minimum RenderInput the pipeline accepts, carrying the
// given endpoint declarations. Endpoint resolution reads only spec.endpoints and the
// already-resolved outputs, so no manifests or CEL context are needed.
func endpointRenderInput(endpoints []openchoreov1alpha1.ResourceTypeEndpoint) *resourcepipeline.RenderInput {
	return &resourcepipeline.RenderInput{
		ResourceType: &openchoreov1alpha1.ResourceType{
			Spec: openchoreov1alpha1.ResourceTypeSpec{Endpoints: endpoints},
		},
		Resource: &openchoreov1alpha1.Resource{},
	}
}

func plainOutputs(pairs ...string) []resourcepipeline.ResolvedOutput {
	out := make([]resourcepipeline.ResolvedOutput, 0, len(pairs)/2)
	for i := 0; i+1 < len(pairs); i += 2 {
		out = append(out, resourcepipeline.ResolvedOutput{Name: pairs[i], Value: pairs[i+1]})
	}
	return out
}

func endpointReconciler() *Reconciler {
	return &Reconciler{Pipeline: resourcepipeline.NewPipeline()}
}

// Every declared endpoint that resolves lands in status.endpoints, in spec order, with
// its output mapping intact — that mapping is what lets a consumer redirect the right
// env bindings.
func TestEvaluateEndpointsWritesStatus(t *testing.T) {
	binding := &openchoreov1alpha1.ResourceReleaseBinding{}
	in := endpointRenderInput([]openchoreov1alpha1.ResourceTypeEndpoint{
		{Name: "client", HostFrom: "host", PortFrom: "port"},
		{Name: "monitor", HostFrom: "host", PortFrom: "monitorPort"},
	})
	retry := endpointReconciler().evaluateEndpoints(context.Background(), binding, in, nil,
		plainOutputs("host", "nats.ns.svc", "port", "4222", "monitorPort", "8222"), logr.Discard())

	require.Zero(t, retry)
	requireCondition(t, binding, ConditionEndpointsResolved, metav1.ConditionTrue, ReasonEndpointsResolved)
	require.Equal(t, []openchoreov1alpha1.ResolvedResourceEndpoint{
		{Name: "client", Host: "nats.ns.svc", Port: 4222, HostFrom: "host", PortFrom: "port"},
		{Name: "monitor", Host: "nats.ns.svc", Port: 8222, HostFrom: "host", PortFrom: "monitorPort"},
	}, binding.Status.Endpoints)
}

// A per-endpoint failure keeps the resolvable subset, matching the outputs contract: one
// malformed endpoint must not erase a still-valid view of the others.
func TestEvaluateEndpointsPartialFailureKeepsSubset(t *testing.T) {
	binding := &openchoreov1alpha1.ResourceReleaseBinding{}
	in := endpointRenderInput([]openchoreov1alpha1.ResourceTypeEndpoint{
		{Name: "client", HostFrom: "host", PortFrom: "port"},
		{Name: "broken", HostFrom: "host", PortFrom: "notAPort"},
	})
	retry := endpointReconciler().evaluateEndpoints(context.Background(), binding, in, nil,
		plainOutputs("host", "pg.ns.svc", "port", "5432", "notAPort", "quite-wrong"), logr.Discard())

	require.Len(t, binding.Status.Endpoints, 1)
	require.Equal(t, "client", binding.Status.Endpoints[0].Name)
	// A rendered value that is not a port does not fix itself, so no timed retry.
	require.Zero(t, retry)
	requireCondition(t, binding, ConditionEndpointsResolved, metav1.ConditionFalse, ReasonEndpointResolutionFailed)
}

// A top-level failure (nothing resolved at all) leaves the previous endpoints in place
// rather than erasing them, so a transient pipeline error does not make a healthy
// resource look undialable to every consumer.
func TestEvaluateEndpointsTopLevelFailureKeepsPrevious(t *testing.T) {
	previous := []openchoreov1alpha1.ResolvedResourceEndpoint{
		{Name: "client", Host: "pg.ns.svc", Port: 5432, HostFrom: "host", PortFrom: "port"},
	}
	binding := &openchoreov1alpha1.ResourceReleaseBinding{
		Status: openchoreov1alpha1.ResourceReleaseBindingStatus{Endpoints: previous},
	}
	// A nil Resource fails validateInput before any endpoint is looked at.
	in := endpointRenderInput([]openchoreov1alpha1.ResourceTypeEndpoint{
		{Name: "client", HostFrom: "host", PortFrom: "port"},
	})
	in.Resource = nil

	retry := endpointReconciler().evaluateEndpoints(context.Background(), binding, in, nil, plainOutputs("host", "h", "port", "5432"), logr.Discard())

	require.Equal(t, previous, binding.Status.Endpoints)
	requireCondition(t, binding, ConditionEndpointsResolved, metav1.ConditionFalse, ReasonEndpointResolutionFailed)
	require.Zero(t, retry)
}

// An endpoint whose address is only readable on the data plane resolves without one.
// It is recorded rather than dropped, so a consumer is told the endpoint exists but
// cannot be tunneled, instead of being told the endpoint does not exist.
func TestEvaluateEndpointsRecordsUnresolvedAddress(t *testing.T) {
	binding := &openchoreov1alpha1.ResourceReleaseBinding{}
	in := endpointRenderInput([]openchoreov1alpha1.ResourceTypeEndpoint{
		{Name: "client", HostFrom: "host", PortFrom: "port"},
	})
	outputs := []resourcepipeline.ResolvedOutput{
		{Name: "host", SecretKeyRef: &openchoreov1alpha1.SecretKeyRef{Name: "conn", Key: "host"}},
		{Name: "port", Value: "5432"},
	}
	retry := endpointReconciler().evaluateEndpoints(context.Background(), binding, in, nil, outputs, logr.Discard())

	require.Zero(t, retry)
	require.Len(t, binding.Status.Endpoints, 1)
	got := binding.Status.Endpoints[0]
	require.Empty(t, got.Host)
	require.Zero(t, got.Port)
	// The mapping survives, which is the whole point of recording it.
	require.Equal(t, "host", got.HostFrom)
	require.Equal(t, "port", got.PortFrom)
}

// A type declaring no endpoints leaves status.endpoints unset rather than an empty list,
// so a resource with nothing to dial is not confused with one whose endpoints all failed.
func TestEvaluateEndpointsNoneDeclared(t *testing.T) {
	binding := &openchoreov1alpha1.ResourceReleaseBinding{}
	retry := endpointReconciler().evaluateEndpoints(context.Background(), binding, endpointRenderInput(nil), nil,
		plainOutputs("bucket", "assets"), logr.Discard())

	require.Zero(t, retry)
	require.Nil(t, binding.Status.Endpoints)
	// No endpoints declared, so the condition would say nothing.
	require.Nil(t, meta.FindStatusCondition(binding.Status.Conditions, string(ConditionEndpointsResolved)))
}

func requireCondition(t *testing.T, binding *openchoreov1alpha1.ResourceReleaseBinding,
	condType controller.ConditionType, status metav1.ConditionStatus, reason controller.ConditionReason,
) {
	t.Helper()
	cond := meta.FindStatusCondition(binding.Status.Conditions, string(condType))
	require.NotNil(t, cond, "condition %s not set", condType)
	require.Equal(t, status, cond.Status)
	require.Equal(t, string(reason), cond.Reason)
}

// An address awaiting observed state is pending, and retried on a timer.
func TestEvaluateEndpointsPendingAddressRetries(t *testing.T) {
	binding := &openchoreov1alpha1.ResourceReleaseBinding{}
	in := endpointRenderInput([]openchoreov1alpha1.ResourceTypeEndpoint{
		{Name: "client", HostFrom: "host", PortFrom: "port"},
	})
	// The Service has not reported its port yet, so the output renders empty.
	retry := endpointReconciler().evaluateEndpoints(context.Background(), binding, in, nil,
		plainOutputs("host", "pg.ns.svc", "port", ""), logr.Discard())

	require.Equal(t, endpointRetryInterval, retry)
	requireCondition(t, binding, ConditionEndpointsResolved, metav1.ConditionFalse, ReasonEndpointsPending)
}

// A pending endpoint alongside a defective one reports as failed, not pending.
func TestEvaluateEndpointsMixedFailureIsNotPending(t *testing.T) {
	binding := &openchoreov1alpha1.ResourceReleaseBinding{}
	in := endpointRenderInput([]openchoreov1alpha1.ResourceTypeEndpoint{
		{Name: "pending", HostFrom: "host", PortFrom: "emptyPort"},
		{Name: "broken", HostFrom: "host", PortFrom: "notAPort"},
	})
	retry := endpointReconciler().evaluateEndpoints(context.Background(), binding, in, nil,
		plainOutputs("host", "pg.ns.svc", "emptyPort", "", "notAPort", "quite-wrong"), logr.Discard())

	require.Zero(t, retry)
	requireCondition(t, binding, ConditionEndpointsResolved, metav1.ConditionFalse, ReasonEndpointResolutionFailed)
}

// A resource whose endpoints did not resolve is still Ready.
func TestEndpointFailureLeavesReadyTrue(t *testing.T) {
	for _, tc := range []struct {
		name    string
		outputs []resourcepipeline.ResolvedOutput
		reason  controller.ConditionReason
	}{
		{"defective endpoint", plainOutputs("host", "pg.ns.svc", "port", "quite-wrong"), ReasonEndpointResolutionFailed},
		{"pending endpoint", plainOutputs("host", "pg.ns.svc", "port", ""), ReasonEndpointsPending},
	} {
		t.Run(tc.name, func(t *testing.T) {
			binding := &openchoreov1alpha1.ResourceReleaseBinding{}
			r := endpointReconciler()

			// The state a healthy resource reaches before endpoints are looked at.
			controller.MarkTrueCondition(binding, ConditionSynced, ReasonReleaseSynced, "up to date")
			controller.MarkTrueCondition(binding, ConditionResourcesReady, ReasonResourcesReady, "all ready")
			controller.MarkTrueCondition(binding, ConditionOutputsResolved, ReasonOutputsResolved, "Resolved 2 output(s)")

			in := endpointRenderInput([]openchoreov1alpha1.ResourceTypeEndpoint{
				{Name: "client", HostFrom: "host", PortFrom: "port"},
			})
			r.evaluateEndpoints(context.Background(), binding, in, nil, tc.outputs, logr.Discard())
			r.setReadyCondition(binding)

			requireCondition(t, binding, ConditionEndpointsResolved, metav1.ConditionFalse, tc.reason)
			requireCondition(t, binding, ConditionReady, metav1.ConditionTrue, ReasonReady)
			// OutputsResolved must not be collateral damage either.
			requireCondition(t, binding, ConditionOutputsResolved, metav1.ConditionTrue, ReasonOutputsResolved)
		})
	}
}

// Advancing to a release that declares no endpoints clears the previous ones.
func TestEvaluateEndpointsClearsPreviousWhenNoneDeclared(t *testing.T) {
	binding := &openchoreov1alpha1.ResourceReleaseBinding{
		Status: openchoreov1alpha1.ResourceReleaseBindingStatus{
			Endpoints: []openchoreov1alpha1.ResolvedResourceEndpoint{
				{Name: "client", Host: "pg.ns.svc", Port: 5432, HostFrom: "host", PortFrom: "port"},
			},
		},
	}
	controller.MarkTrueCondition(binding, ConditionEndpointsResolved, ReasonEndpointsResolved, "Resolved 1 endpoint(s)")

	retry := endpointReconciler().evaluateEndpoints(context.Background(), binding, endpointRenderInput(nil), nil,
		plainOutputs("bucket", "assets"), logr.Discard())

	require.Zero(t, retry)
	require.Nil(t, binding.Status.Endpoints)
	require.Nil(t, meta.FindStatusCondition(binding.Status.Conditions, string(ConditionEndpointsResolved)))
}
