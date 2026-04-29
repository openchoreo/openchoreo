// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package secret

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"

	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
)

// supportedSecretTypes lists the K8s Secret types accepted by the secret API.
var supportedSecretTypes = map[corev1.SecretType]struct{}{
	corev1.SecretTypeOpaque:           {},
	corev1.SecretTypeBasicAuth:        {},
	corev1.SecretTypeSSHAuth:          {},
	corev1.SecretTypeDockerConfigJson: {},
	corev1.SecretTypeTLS:              {},
}

// requiredKeys defines the keys that must be present (and non-empty) in the
// data map for each typed Secret. Opaque has no required keys but must contain
// at least one entry (enforced separately).
var requiredKeys = map[corev1.SecretType][]string{
	corev1.SecretTypeBasicAuth:        {"username", "password"},
	corev1.SecretTypeSSHAuth:          {"ssh-privatekey"},
	corev1.SecretTypeDockerConfigJson: {".dockerconfigjson"},
	corev1.SecretTypeTLS:              {"tls.crt", "tls.key"},
}

// supportedPlaneKinds lists the target plane kinds accepted by the secret API.
var supportedPlaneKinds = map[string]struct{}{
	planeKindWorkflowPlane:        {},
	planeKindClusterWorkflowPlane: {},
	planeKindDataPlane:            {},
	planeKindClusterDataPlane:     {},
}

func validateSecretName(name string) error {
	if name == "" {
		return &services.ValidationError{Msg: "secretName is required"}
	}
	return nil
}

func validatePlaneKind(kind string) error {
	if kind == "" {
		return &services.ValidationError{Msg: "targetPlane.kind is required"}
	}
	if _, ok := supportedPlaneKinds[kind]; !ok {
		return &services.ValidationError{Msg: fmt.Sprintf("unsupported targetPlane.kind: %s", kind)}
	}
	return nil
}

func validateSecretData(secretType corev1.SecretType, data map[string]string) error {
	if _, ok := supportedSecretTypes[secretType]; !ok {
		return &services.ValidationError{Msg: fmt.Sprintf("unsupported secretType: %s", secretType)}
	}
	if len(data) == 0 {
		return &services.ValidationError{Msg: "data must contain at least one entry"}
	}
	for k, v := range data {
		if k == "" {
			return &services.ValidationError{Msg: "data keys must not be empty"}
		}
		if v == "" {
			return &services.ValidationError{Msg: fmt.Sprintf("data[%q] must not be empty", k)}
		}
	}
	for _, required := range requiredKeys[secretType] {
		if _, ok := data[required]; !ok {
			return &services.ValidationError{Msg: fmt.Sprintf("data[%q] is required for secretType %s", required, secretType)}
		}
	}
	return nil
}
