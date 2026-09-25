// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package gate

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

// MaxHistory bounds status.gate.history (mirrors the CRD MaxItems).
const MaxHistory = 10

// RecordPass marks the key passed: passedKey, lastPassedRelease and a history
// entry (newest first, bounded).
func RecordPass(gate *openchoreov1alpha1.DeploymentGateStatus, key, release, hookSetHash string, now metav1.Time) {
	gate.PassedKey = key
	gate.LastPassedRelease = release
	for _, r := range gate.History {
		if r.Key == key {
			return
		}
	}
	gate.History = append([]openchoreov1alpha1.GatePassRecord{{
		Key:         key,
		Release:     release,
		HookSetHash: hookSetHash,
		PassedAt:    &now,
	}}, gate.History...)
	if len(gate.History) > MaxHistory {
		gate.History = gate.History[:MaxHistory]
	}
}

// CarryForward decides whether a gate that had passed stays passed when the
// key changes only because the bound hook set changed (same release). The new
// set applies from the next release change, so a hook edit never re-opens a
// gate that already passed for the release currently deployed.
func CarryForward(gate *openchoreov1alpha1.DeploymentGateStatus,
	release, newHookSetHash string) bool {
	if gate == nil || gate.Key == "" || gate.PassedKey != gate.Key {
		return false
	}
	if gate.LastPassedRelease != release || gate.HookSetHash == newHookSetHash {
		return false
	}
	return true
}

// CurrentTrigger classifies why the gate is evaluated for key. It compares the
// attempt against the most recent pass of a different key, so the answer is
// stable across every reconcile of the same key: the first deployment is
// BindingCreate, anything later is a ReleaseChange (a new release, or a hook
// set edit that re-opened the gate).
func CurrentTrigger(gate *openchoreov1alpha1.DeploymentGateStatus, key, release, hookSetHash string) openchoreov1alpha1.DeploymentTrigger {
	if gate == nil {
		return openchoreov1alpha1.DeploymentTriggerBindingCreate
	}
	var prev *openchoreov1alpha1.GatePassRecord
	for i := range gate.History {
		if gate.History[i].Key != key {
			prev = &gate.History[i]
			break
		}
	}
	if prev == nil {
		return openchoreov1alpha1.DeploymentTriggerBindingCreate
	}
	return openchoreov1alpha1.DeploymentTriggerReleaseChange
}
