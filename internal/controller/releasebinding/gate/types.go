// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

// Package gate resolves the deployment hooks a DeploymentPipeline binds to an
// environment, computes the gate key that identifies one deployment attempt,
// and maps hook parameters to values. It is pure with respect to the
// ReleaseBinding controller: nothing here creates or observes WorkflowRuns.
package gate

import (
	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

// Phase names a hook phase as it appears in status, annotations and labels.
type Phase string

const (
	// PhasePreDeploy hooks run before the RenderedRelease is created or updated.
	PhasePreDeploy Phase = "preDeploy"
	// PhasePostDeploy hooks run after the release reports ResourcesReady.
	PhasePostDeploy Phase = "postDeploy"
)

const (
	// SkipReasonNotApplicable marks a binding whose appliesTo matched nothing.
	SkipReasonNotApplicable = "NotApplicable"
	// SkipReasonNotEnabled marks a binding whose hook's enabledTo excludes this component type.
	SkipReasonNotEnabled = "NotEnabled"
	// ReasonHookNotFound marks a binding whose hookRef does not resolve.
	ReasonHookNotFound = "HookNotFound"
)

// ResolvedHook is one binding of the effective set with its hook spec attached.
type ResolvedHook struct {
	Binding openchoreov1alpha1.HookBinding
	Phase   Phase
	// Hook is the referenced hook's spec; nil when NotFound.
	Hook *openchoreov1alpha1.HookSpec
	// NotFound is set when the hookRef does not resolve. A Sync pre-deploy
	// binding in this state blocks the gate.
	NotFound bool
	// SkipReason is non-empty when the binding does not apply to this
	// deployment (appliesTo miss). The binding still contributes to the key.
	SkipReason string
}

// Applicable reports whether the binding runs for this deployment.
func (h ResolvedHook) Applicable() bool { return h.SkipReason == "" }

// EffectiveHookSet is the union of every binding on every promotion path into
// one environment, de-duplicated by binding name and sorted for determinism.
type EffectiveHookSet struct {
	PreDeploy  []ResolvedHook
	PostDeploy []ResolvedHook
}

// Empty reports whether no binding targets the environment at all.
func (s EffectiveHookSet) Empty() bool {
	return len(s.PreDeploy) == 0 && len(s.PostDeploy) == 0
}

// Subject is what appliesTo selectors are evaluated against: the release's
// frozen component type.
type Subject struct {
	ComponentTypeKind openchoreov1alpha1.ComponentTypeRefKind
	// ComponentTypeName is the frozen reference name, in "{workloadType}/{name}"
	// form on a ComponentRelease. Selectors may name either form.
	ComponentTypeName string
}

// Mode returns the binding's effective mode (Sync when unset).
func Mode(b openchoreov1alpha1.HookBinding) openchoreov1alpha1.HookMode {
	if b.Mode == "" {
		return openchoreov1alpha1.HookModeSync
	}
	return b.Mode
}

// OnFailure returns the binding's effective failure policy: Block for
// pre-deploy and Ignore for post-deploy when unset.
func OnFailure(b openchoreov1alpha1.HookBinding, phase Phase) openchoreov1alpha1.HookFailurePolicy {
	if b.OnFailure != "" {
		return b.OnFailure
	}
	if phase == PhasePreDeploy {
		return openchoreov1alpha1.HookFailurePolicyBlock
	}
	return openchoreov1alpha1.HookFailurePolicyIgnore
}
