// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// HookRefKind is the kind of a hook reference.
// +kubebuilder:validation:Enum=Hook;ClusterHook
type HookRefKind string

const (
	// HookRefKindHook references a namespace-scoped Hook.
	HookRefKindHook HookRefKind = "Hook"
	// HookRefKindClusterHook references a cluster-scoped ClusterHook.
	HookRefKindClusterHook HookRefKind = "ClusterHook"
)

// HookRef references a Hook or ClusterHook.
type HookRef struct {
	// Kind is the kind of hook (Hook or ClusterHook).
	// +optional
	// +kubebuilder:default=Hook
	Kind HookRefKind `json:"kind,omitempty"`

	// Name is the name of the hook resource.
	// +required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=253
	Name string `json:"name"`
}

// HookMode decides whether the deployment waits for the hook.
// +kubebuilder:validation:Enum=Sync;Async
type HookMode string

const (
	// HookModeSync waits for the hook and applies onFailure.
	HookModeSync HookMode = "Sync"
	// HookModeAsync dispatches the hook and never waits for it.
	HookModeAsync HookMode = "Async"
)

// HookFailurePolicy decides what a failed Sync hook does to the deployment.
// +kubebuilder:validation:Enum=Block;Ignore;Alert
type HookFailurePolicy string

const (
	// HookFailurePolicyBlock stops the deployment (pre-deploy only).
	HookFailurePolicyBlock HookFailurePolicy = "Block"
	// HookFailurePolicyIgnore records the failure and continues.
	HookFailurePolicyIgnore HookFailurePolicy = "Ignore"
	// HookFailurePolicyAlert degrades Ready until acknowledged (post-deploy only).
	HookFailurePolicyAlert HookFailurePolicy = "Alert"
)

// DeploymentTrigger names the event that started a hook. It is reported to the
// hook through the deployment context; bindings do not filter on it.
// +kubebuilder:validation:Enum=ReleaseChange;BindingCreate
type DeploymentTrigger string

const (
	// DeploymentTriggerReleaseChange fires when the pinned ComponentRelease changes.
	DeploymentTriggerReleaseChange DeploymentTrigger = "ReleaseChange"
	// DeploymentTriggerBindingCreate fires on the first deployment of a binding.
	DeploymentTriggerBindingCreate DeploymentTrigger = "BindingCreate"
)

// HookSubjectSelectorKind is the kind of type a hook binding is scoped to.
// +kubebuilder:validation:Enum=ComponentType;ClusterComponentType
type HookSubjectSelectorKind string

const (
	HookSubjectSelectorKindComponentType        HookSubjectSelectorKind = "ComponentType"
	HookSubjectSelectorKindClusterComponentType HookSubjectSelectorKind = "ClusterComponentType"
)

// HookSubjectSelector scopes a binding to components of a component type.
type HookSubjectSelector struct {
	// Kind is the type kind to match.
	// +required
	Kind HookSubjectSelectorKind `json:"kind"`

	// Name is the name of the type resource.
	// +required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=253
	Name string `json:"name"`
}

// HookBinding attaches a hook to an environment.
type HookBinding struct {
	// Name identifies the binding. It must be unique across both phases of an environment.
	// +required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=40
	// +kubebuilder:validation:Pattern=`^[a-z]([-a-z0-9]*[a-z0-9])?$`
	Name string `json:"name"`

	// HookRef is the Hook or ClusterHook to run.
	// +required
	HookRef HookRef `json:"hookRef"`

	// Mode selects whether the deployment waits for the hook.
	// +optional
	// +kubebuilder:default=Sync
	Mode HookMode `json:"mode,omitempty"`

	// Parameters supplies values for the hook's open parameters
	// (default, required, or from+overridable). Keys are parameter names,
	// values are strings.
	// +optional
	// +kubebuilder:pruning:PreserveUnknownFields
	Parameters *runtime.RawExtension `json:"parameters,omitempty"`

	// AppliesTo restricts the binding to components of the listed component
	// types. Empty applies to every component.
	// +optional
	AppliesTo []HookSubjectSelector `json:"appliesTo,omitempty"`

	// OnFailure decides what a failed Sync hook does to the deployment.
	// Defaults to Block for pre-deploy and Ignore for post-deploy.
	// +optional
	OnFailure HookFailurePolicy `json:"onFailure,omitempty"`

	// Timeout bounds a Sync hook's run time, as a Go duration (e.g. 30m, 2h).
	// Defaults to 30m.
	// +optional
	// +kubebuilder:validation:Pattern=`^(\d+h)?(\d+m)?(\d+s)?$`
	Timeout string `json:"timeout,omitempty"`

	// Retries is the number of automatic re-runs after a Sync failure.
	// +optional
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=5
	Retries int32 `json:"retries,omitempty"`
}

// HookSet holds the pre-deploy and post-deploy bindings of an environment.
type HookSet struct {
	// PreDeploy hooks run before the RenderedRelease is created.
	// +optional
	// +listType=map
	// +listMapKey=name
	// +kubebuilder:validation:MaxItems=10
	PreDeploy []HookBinding `json:"preDeploy,omitempty"`

	// PostDeploy hooks run after the release reports ResourcesReady.
	// +optional
	// +listType=map
	// +listMapKey=name
	// +kubebuilder:validation:MaxItems=10
	PostDeploy []HookBinding `json:"postDeploy,omitempty"`
}

// HookPhase is the observed phase of one hook run.
// +kubebuilder:validation:Enum=Pending;Running;Succeeded;Failed;TimedOut;Skipped;Dispatched;DispatchFailed;PlaneUnavailable
type HookPhase string

const (
	HookPhasePending          HookPhase = "Pending"
	HookPhaseRunning          HookPhase = "Running"
	HookPhaseSucceeded        HookPhase = "Succeeded"
	HookPhaseFailed           HookPhase = "Failed"
	HookPhaseTimedOut         HookPhase = "TimedOut"
	HookPhaseSkipped          HookPhase = "Skipped"
	HookPhaseDispatched       HookPhase = "Dispatched"
	HookPhaseDispatchFailed   HookPhase = "DispatchFailed"
	HookPhasePlaneUnavailable HookPhase = "PlaneUnavailable"
)

// DeploymentHookStatus is the observed state of one hook binding for the current gate key.
type DeploymentHookStatus struct {
	// Name is the binding name.
	// +required
	Name string `json:"name"`

	// HookRef is the hook the binding resolved to.
	// +optional
	HookRef HookRef `json:"hookRef,omitempty"`

	// Mode is the effective mode of the binding.
	// +optional
	Mode HookMode `json:"mode,omitempty"`

	// Phase is the observed phase of the run.
	// +optional
	Phase HookPhase `json:"phase,omitempty"`

	// Reason is a machine-readable reason for the phase.
	// +optional
	Reason string `json:"reason,omitempty"`

	// Message is a human-readable detail, copied from the failing task when available.
	// +optional
	Message string `json:"message,omitempty"`

	// WorkflowRunRef is the name of the WorkflowRun that executed the hook.
	// +optional
	WorkflowRunRef string `json:"workflowRunRef,omitempty"`

	// Attempt is the 1-based attempt number of the current run.
	// +optional
	Attempt int32 `json:"attempt,omitempty"`

	// StartedAt is when the current attempt started.
	// +optional
	StartedAt *metav1.Time `json:"startedAt,omitempty"`

	// FinishedAt is when the current attempt reached a terminal phase.
	// +optional
	FinishedAt *metav1.Time `json:"finishedAt,omitempty"`
}

// GatePassRecord records one gate pass.
type GatePassRecord struct {
	// Key is the gate key that passed.
	// +required
	Key string `json:"key"`

	// Release is the ComponentRelease name the key was computed for.
	// +optional
	Release string `json:"release,omitempty"`

	// HookSetHash is the hash of the effective hook set at the time of the pass.
	// +optional
	HookSetHash string `json:"hookSetHash,omitempty"`

	// PassedAt is when the gate passed.
	// +optional
	PassedAt *metav1.Time `json:"passedAt,omitempty"`
}

// DeploymentGateStatus is the observed state of the hook gate on a ReleaseBinding.
type DeploymentGateStatus struct {
	// Key identifies the current deployment attempt: a hash of the release,
	// the effective hook set and the deployment sequence.
	// +optional
	Key string `json:"key,omitempty"`

	// PassedKey is the last key whose pre-deploy hooks all passed.
	// +optional
	PassedKey string `json:"passedKey,omitempty"`

	// Sequence counts deployment attempts on this binding. It is part of the key,
	// so a release that returns after another one was pinned runs its hooks again
	// under a fresh key (and fresh WorkflowRun names).
	// +optional
	Sequence int64 `json:"sequence,omitempty"`

	// HookSetHash is the hash of the effective hook set for the current key.
	// +optional
	HookSetHash string `json:"hookSetHash,omitempty"`

	// LastPassedRelease is the ComponentRelease name of the last pass.
	// +optional
	LastPassedRelease string `json:"lastPassedRelease,omitempty"`

	// PostDeployKey is the key whose post-deploy hooks have been started.
	// +optional
	PostDeployKey string `json:"postDeployKey,omitempty"`

	// History lists the most recent gate passes, newest first.
	// +optional
	// +kubebuilder:validation:MaxItems=10
	History []GatePassRecord `json:"history,omitempty"`

	// PreDeploy is the status of each pre-deploy binding for the current key.
	// +optional
	// +listType=map
	// +listMapKey=name
	PreDeploy []DeploymentHookStatus `json:"preDeploy,omitempty"`

	// PostDeploy is the status of each post-deploy binding for the current key.
	// +optional
	// +listType=map
	// +listMapKey=name
	PostDeploy []DeploymentHookStatus `json:"postDeploy,omitempty"`
}
