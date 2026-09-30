// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package gate

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

// hookDigest is the canonical form of one binding plus its hook spec.
type hookDigest struct {
	Phase    Phase                                `json:"phase"`
	Name     string                               `json:"name"`
	Binding  any                                  `json:"binding"`
	Hook     any                                  `json:"hook,omitempty"`
	NotFound bool                                 `json:"notFound,omitempty"`
	HookRef  openchoreov1alpha1.HookRef           `json:"hookRef"`
	Mode     openchoreov1alpha1.HookMode          `json:"mode"`
	Failure  openchoreov1alpha1.HookFailurePolicy `json:"onFailure"`
}

type keyInput struct {
	Release     string `json:"release"`
	HookSetHash string `json:"hookSetHash"`
	// Sequence is omitted when zero so keys computed before it existed are unchanged.
	Sequence int64 `json:"sequence,omitempty"`
}

// Key computes the gate key for one deployment attempt and the hash of the
// effective hook set. The key changes when the release, the attempt sequence,
// or any bound binding or hook spec, changes. Both values are deterministic for
// equal inputs: the hook set is serialized in name order with map keys sorted.
func Key(releaseName string, sequence int64, set EffectiveHookSet) (key, hookSetHash string, err error) {
	digests := make([]hookDigest, 0, len(set.PreDeploy)+len(set.PostDeploy))
	for _, list := range [][]ResolvedHook{set.PreDeploy, set.PostDeploy} {
		for _, h := range list {
			d := hookDigest{
				Phase:    h.Phase,
				Name:     h.Binding.Name,
				NotFound: h.NotFound,
				HookRef:  h.Binding.HookRef,
				Mode:     Mode(h.Binding),
				Failure:  OnFailure(h.Binding, h.Phase),
			}
			if d.Binding, err = canonical(h.Binding); err != nil {
				return "", "", fmt.Errorf("binding %q: %w", h.Binding.Name, err)
			}
			if h.Hook != nil {
				if d.Hook, err = canonical(h.Hook); err != nil {
					return "", "", fmt.Errorf("hook of binding %q: %w", h.Binding.Name, err)
				}
			}
			digests = append(digests, d)
		}
	}
	hookSetHash, err = hashJSON(digests)
	if err != nil {
		return "", "", err
	}
	key, err = hashJSON(keyInput{Release: releaseName, HookSetHash: hookSetHash, Sequence: sequence})
	if err != nil {
		return "", "", err
	}
	return key, hookSetHash, nil
}

// canonical round-trips v through JSON so that RawExtension payloads and struct
// fields alike end up as maps with sorted keys.
func canonical(v any) (any, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	var out any
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func hashJSON(v any) (string, error) {
	c, err := canonical(v)
	if err != nil {
		return "", err
	}
	b, err := json.Marshal(c)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:]), nil
}
