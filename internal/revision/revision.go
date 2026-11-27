// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

// Package revision provides the hashing and naming helpers shared by
// ReleaseBindingRevision and ResourceReleaseBindingRevision.
package revision

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

const (
	// maxNameLength is the Kubernetes object name limit (RFC 1123 DNS subdomain).
	maxNameLength = 253

	// nameHashLength is the number of hex characters of the binding-name hash
	// appended when a binding name has to be shortened.
	nameHashLength = 12

	// numberSeparator separates the binding name from the revision number.
	numberSeparator = "-r"

	// MaxChangeMessageLength is the longest change message a revision keeps, in
	// characters; it matches the CRD's maxLength for spec.message.
	MaxChangeMessageLength = 1024
)

// ChangeMessage returns the change message in a binding's annotations
// (openchoreo.dev/change-message), without surrounding whitespace and cut to
// MaxChangeMessageLength characters so it always fits a revision's spec.message.
// It is empty when none was given.
func ChangeMessage(annotations map[string]string) string {
	msg := strings.TrimSpace(annotations[openchoreov1alpha1.AnnotationKeyChangeMessage])
	if runes := []rune(msg); len(runes) > MaxChangeMessageLength {
		msg = strings.TrimSpace(string(runes[:MaxChangeMessageLength]))
	}
	return msg
}

// CanonicalHash returns the hex-encoded SHA-256 of the canonical JSON form of v.
//
// v is first marshaled with encoding/json, so json tags and omitempty apply,
// then decoded and re-encoded in this canonical form:
//
//   - Object keys are sorted. This also covers free-form JSON such as
//     runtime.RawExtension, whose bytes would otherwise keep their source key order.
//   - Absent, null and empty objects are treated as "not set": an object member
//     whose value is null or {} is dropped. The check runs bottom-up, so an
//     object left empty after its members are dropped is dropped too. As a
//     result, a nil map, an empty one, and an omitted field all hash the same.
//   - Empty arrays are values: [] is kept and differs from "not set". A schema
//     default applies only to an absent field, so {} and {"ports":[]} can render
//     differently ([] stays empty while an absent field takes a default such as
//     [80]). Typed slice fields tagged omitempty are still omitted when empty,
//     by encoding/json.
//   - Array elements are never dropped, because their position is meaningful. An
//     element that is null or {} is kept as null so that [{}, x] and [x] stay
//     different.
//   - Numbers are compared by exact value, not spelling: 1, 1.0, 10e-1 and 1e0
//     hash the same, and numbers that differ in any digit hash differently
//     (9007199254740992.0 and 9007199254740993.0 do, though both round to the
//     same float64).
//   - Strings, booleans and zero numbers are values: "", false and 0 are kept and
//     differ from "not set". (Typed struct fields tagged omitempty are already
//     omitted by encoding/json when zero, before this step runs.)
//
// If v is entirely "not set" (nil, an empty map, or a struct whose fields are all
// omitted), it hashes as JSON null.
func CanonicalHash(v any) (string, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return "", fmt.Errorf("marshal value for hashing: %w", err)
	}

	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var generic any
	if err := dec.Decode(&generic); err != nil {
		return "", fmt.Errorf("decode value for hashing: %w", err)
	}

	normalized, err := normalize(generic)
	if err != nil {
		return "", err
	}

	// encoding/json writes map keys in sorted order, which makes the output canonical.
	canonical, err := json.Marshal(normalized)
	if err != nil {
		return "", fmt.Errorf("marshal canonical form: %w", err)
	}

	sum := sha256.Sum256(canonical)
	return hex.EncodeToString(sum[:]), nil
}

// normalize converts a value decoded with UseNumber into its canonical form.
// It returns nil for values that count as "not set".
func normalize(v any) (any, error) {
	switch t := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, val := range t {
			n, err := normalize(val)
			if err != nil {
				return nil, err
			}
			if n != nil {
				out[k] = n
			}
		}
		if len(out) == 0 {
			return nil, nil
		}
		return out, nil
	case []any:
		// make, not nil: an empty array is a value and is kept.
		out := make([]any, len(t))
		for i, val := range t {
			n, err := normalize(val)
			if err != nil {
				return nil, err
			}
			out[i] = n
		}
		return out, nil
	case json.Number:
		return normalizeNumber(t)
	default:
		// nil, string and bool are already canonical.
		return t, nil
	}
}

// maxPlainDigits is the most digits an integer is written out with in the
// canonical form; larger integers and all fractions use an exponent.
const maxPlainDigits = 21

// normalizeNumber returns n spelled canonically: equal values are spelled the
// same and different values never are. It works on the decimal digits, not
// through float64, so nothing is rounded. An integer of up to maxPlainDigits
// digits is written out in full ("100", as strconv.FormatInt would); any other
// value is written as its significant digits and an exponent ("15e-1", "1e400").
// n comes from a json.Decoder, so it is a valid JSON number.
func normalizeNumber(n json.Number) (json.Number, error) {
	s, neg := strings.CutPrefix(n.String(), "-")
	mantissa, expPart, hasExp := strings.Cut(strings.ToLower(s), "e")
	var exp int64
	if hasExp {
		e, err := strconv.ParseInt(expPart, 10, 32)
		if err != nil {
			return "", fmt.Errorf("normalize number %q: exponent out of range", n.String())
		}
		exp = e
	}
	intPart, frac, _ := strings.Cut(mantissa, ".")
	if intPart == "" || strings.Trim(intPart+frac, "0123456789") != "" {
		return "", fmt.Errorf("normalize number %q: not a JSON number", n.String())
	}

	// value = digits * 10^exp, with no leading or trailing zeros in digits.
	digits := strings.TrimLeft(intPart+frac, "0")
	exp -= int64(len(frac))
	significant := strings.TrimRight(digits, "0")
	exp += int64(len(digits) - len(significant))
	if significant == "" {
		return "0", nil // 0, 0.0, -0 and 0e5 are all zero
	}

	sign := ""
	if neg {
		sign = "-"
	}
	if exp >= 0 && int64(len(significant))+exp <= maxPlainDigits {
		return json.Number(sign + significant + strings.Repeat("0", int(exp))), nil
	}
	return json.Number(sign + significant + "e" + strconv.FormatInt(exp, 10)), nil
}

// Name returns the revision object name "{prefix}-r{n}", where prefix is the
// binding's name: "frontend-development-r3". Numbers count per binding, so the
// name is unique in the namespace. Clients show a revision as
// "{owner}-r{n}" (DisplayName), which is not unique across environments.
//
// If that would exceed the 253-character Kubernetes name limit, the prefix is
// truncated and followed by "-" and the first 12 hex characters of the SHA-256
// of the full prefix: "{truncated}-{hash}-r{n}". The hash depends only on the
// prefix, so two long prefixes that share a truncated start still get
// different names. Trailing '-' and '.' are trimmed from the truncated part so
// the result stays a valid DNS subdomain when prefix is one.
//
// Callers pass n >= 1, matching the CRD's minimum for spec.revision.
func Name(prefix string, n int64) string {
	suffix := numberSeparator + strconv.FormatInt(n, 10)
	if len(prefix)+len(suffix) <= maxNameLength {
		return prefix + suffix
	}

	sum := sha256.Sum256([]byte(prefix))
	hash := hex.EncodeToString(sum[:])[:nameHashLength]

	keep := maxNameLength - len(suffix) - len("-") - nameHashLength
	truncated := strings.TrimRight(prefix[:keep], "-.")
	return truncated + "-" + hash + suffix
}

// DisplayName returns how a revision is shown to people: the name of the
// component or resource that owns its binding and its number, "{owner}-r{n}",
// e.g. "frontend-r1".
func DisplayName(owner string, n int64) string {
	return owner + numberSeparator + strconv.FormatInt(n, 10)
}

// ParseNumber returns the revision number from a name produced by Name.
// It reads the digits after the last "-r" and rejects anything that Name could
// not have produced: a missing separator, non-digits, a sign, a leading zero,
// zero, or a value that overflows int64.
func ParseNumber(name string) (int64, error) {
	idx := strings.LastIndex(name, numberSeparator)
	if idx <= 0 {
		return 0, fmt.Errorf("revision name %q has no %q suffix", name, numberSeparator)
	}

	digits := name[idx+len(numberSeparator):]
	if digits == "" {
		return 0, fmt.Errorf("revision name %q has no revision number", name)
	}
	for _, c := range digits {
		if c < '0' || c > '9' {
			return 0, fmt.Errorf("revision name %q has a non-numeric revision %q", name, digits)
		}
	}
	if digits[0] == '0' {
		return 0, fmt.Errorf("revision name %q has a revision number with a leading zero or zero: %q", name, digits)
	}

	n, err := strconv.ParseInt(digits, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("revision name %q: %w", name, err)
	}
	return n, nil
}
