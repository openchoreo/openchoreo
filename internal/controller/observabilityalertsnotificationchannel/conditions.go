// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package observabilityalertsnotificationchannel

import (
	"github.com/openchoreo/openchoreo/internal/controller"
)

const (
	// ConditionReady indicates whether the channel's configuration and secrets were applied to the
	// observability plane.
	ConditionReady controller.ConditionType = "Ready"

	// ConditionSecretsReady indicates whether the secret values the channel references are
	// available to the observability plane.
	ConditionSecretsReady controller.ConditionType = "SecretsReady"
)

const (
	// ReasonReconciled means the channel's ConfigMap and Secret were applied to the observability plane.
	ReasonReconciled controller.ConditionReason = "Reconciled"
	// ReasonObservabilityPlaneUnavailable means the observability plane for the channel's environment
	// could not be resolved or reached.
	ReasonObservabilityPlaneUnavailable controller.ConditionReason = "ObservabilityPlaneUnavailable"
	// ReasonApplyFailed means applying the channel's resources to the observability plane failed.
	ReasonApplyFailed controller.ConditionReason = "ApplyFailed"

	// ReasonNoSecretsReferenced means the channel references no secret values.
	ReasonNoSecretsReferenced controller.ConditionReason = "NoSecretsReferenced"
	// ReasonExternalSecretSynced means the ExternalSecret synced the referenced values from the secret store.
	ReasonExternalSecretSynced controller.ConditionReason = "ExternalSecretSynced"
	// ReasonExternalSecretPending means the ExternalSecret has not reported a sync result yet.
	ReasonExternalSecretPending controller.ConditionReason = "ExternalSecretPending"
	// ReasonExternalSecretNotSynced means the ExternalSecret failed to sync the referenced values.
	ReasonExternalSecretNotSynced controller.ConditionReason = "ExternalSecretNotSynced"
	// ReasonReplacingSecret means the Secret an earlier ExternalSecret synced is being deleted, so the
	// copied Secret can take its name.
	ReasonReplacingSecret controller.ConditionReason = "ReplacingSecret"
	// ReasonDeprecatedKubernetesSecret means the referenced values were copied from Kubernetes Secrets,
	// which is deprecated in favor of SecretReferences.
	ReasonDeprecatedKubernetesSecret controller.ConditionReason = "DeprecatedKubernetesSecret"

	// ReasonSecretNotFound means neither a SecretReference nor a Kubernetes Secret has the referenced name.
	ReasonSecretNotFound controller.ConditionReason = "SecretNotFound"
	// ReasonSecretKeyNotFound means the referenced SecretReference or Secret has no entry for the referenced key.
	ReasonSecretKeyNotFound controller.ConditionReason = "SecretKeyNotFound"
	// ReasonMixedSecretSources means some references resolve to SecretReferences and others to Kubernetes Secrets.
	ReasonMixedSecretSources controller.ConditionReason = "MixedSecretSources"
	// ReasonSecretStoreNotConfigured means the observability plane has no secretStoreRef.
	ReasonSecretStoreNotConfigured controller.ConditionReason = "SecretStoreNotConfigured"
	// ReasonExternalSecretConflict means an ExternalSecret with the channel's name that this controller
	// didn't create already exists on the observability plane, so the controller leaves it alone.
	ReasonExternalSecretConflict controller.ConditionReason = "ExternalSecretConflict"
	// ReasonSecretStoreUnreachable means a SecretReference targets a plane other than the channel's
	// observability plane, so its value is not in the store the observability plane reads.
	ReasonSecretStoreUnreachable controller.ConditionReason = "SecretStoreUnreachable"
	// ReasonRemoteKeyNotAllowed means a SecretReference points at a remote key outside the channel
	// namespace's part of the secret store.
	ReasonRemoteKeyNotAllowed controller.ConditionReason = "RemoteKeyNotAllowed"
)
