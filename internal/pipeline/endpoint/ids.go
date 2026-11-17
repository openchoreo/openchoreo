// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package endpointpipeline

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

// NetworkPolicyIDPrefix prefixes the RenderedRelease ID of the network policy
// an EndpointReleaseBinding renders for each backend. EndpointType authors
// choose their own resource IDs, so the platform reserves this prefix: a type
// resource with such an ID would collide with a generated policy.
const NetworkPolicyIDPrefix = "openchoreo-networkpolicy-"

// maxResourceIDLength is the longest RenderedRelease resource ID. The ID is
// copied into the openchoreo.dev/rendered-release-resource-id label on the
// applied object, so it must fit a label value.
const maxResourceIDLength = 63

// resourceIDHashLength is the number of hex characters of the backend name's
// hash kept in a shortened ID.
const resourceIDHashLength = 8

// NetworkPolicyID returns the RenderedRelease ID of the network policy for the
// named backend. It is NetworkPolicyIDPrefix followed by the backend name while
// that fits a label value; a longer name is truncated and followed by a hash of
// the whole name, so IDs stay stable and distinct per backend.
func NetworkPolicyID(backend string) string {
	id := NetworkPolicyIDPrefix + backend
	if len(id) <= maxResourceIDLength {
		return id
	}
	sum := sha256.Sum256([]byte(backend))
	keep := maxResourceIDLength - len(NetworkPolicyIDPrefix) - 1 - resourceIDHashLength
	return NetworkPolicyIDPrefix + strings.TrimRight(backend[:keep], "-") + "-" +
		hex.EncodeToString(sum[:])[:resourceIDHashLength]
}
