// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package template

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"time"
	"unicode/utf8"
)

// ErrCostLimitExceeded reports that a single CEL expression exceeded the
// per-expression cost limit. Reconcilers use errors.Is against it to classify
// the failure as terminal (the template is immutable; retrying cannot succeed).
var ErrCostLimitExceeded = errors.New("cel expression cost limit exceeded")

// ErrCostBudgetExceeded reports that the cumulative render cost budget for a
// reconcile (or a single Render, for standalone callers) was exhausted.
var ErrCostBudgetExceeded = errors.New("template render cost budget exceeded")

// ErrExpansionLimitExceeded reports that a fan-out guard refused to expand a
// collection: a forEach whose item count exceeds the cap, or a trait patch whose
// item-by-target cross product exceeds the cap. The lists.range() size cap is
// deliberately not among them — cel-go raises it as an untyped error and refuses
// before allocating, so it needs neither a message match nor paced retries.
//
// These guards trip on the shape of the inputs, not on how busy the process is,
// so the same object trips the same guard on every reconcile until a human edits
// the template or the data behind it (see IsTerminalRenderError).
var ErrExpansionLimitExceeded = errors.New("template expansion limit exceeded")

// IsTerminalRenderError reports whether a render error is deterministic for
// unchanged inputs: a per-expression cost-limit breach, an exhausted reconcile
// cost budget, or a fan-out guard breach. Deadline expiry is deliberately
// excluded — it is load-dependent and therefore transient.
func IsTerminalRenderError(err error) bool {
	return errors.Is(err, ErrCostLimitExceeded) ||
		errors.Is(err, ErrCostBudgetExceeded) ||
		errors.Is(err, ErrExpansionLimitExceeded)
}

// IsRenderAborted reports whether a render error means no further expression in the same
// unit of work is worth evaluating: a terminal cost breach, or a cancelled or expired
// context. Error aggregators that would otherwise evaluate every rule consult it so an
// exhausted budget is reported once instead of being spent again on every remaining rule.
func IsRenderAborted(err error) bool {
	return IsTerminalRenderError(err) ||
		errors.Is(err, context.Canceled) ||
		errors.Is(err, context.DeadlineExceeded)
}

// maxErrorExprLen bounds how much of a tenant-authored fragment an error may quote.
// Render errors are copied verbatim into status conditions; a tenant is free to author a
// multi-hundred-kilobyte expression, and an object that large is rejected by the API
// server. The status write then fails, and the breach is never recorded on the object.
const maxErrorExprLen = 256

// TruncationMarker is appended to a fragment that TruncateTo had to cut, so a reader
// can tell a bounded message from a complete one.
const TruncationMarker = "…(truncated)"

// TruncateFragment bounds a fragment of tenant-authored input - an expression, a where
// clause, a forEach item value - for inclusion in an error message. Fragments shorter than
// the bound are returned unchanged, so error text that names a short expression reads
// exactly as it always has.
//
// Every layer that names such a fragment in an error must route it through here. The engine
// bounds what it names, but an outer wrap that re-quotes the raw input puts the whole
// fragment back into the message, and it is the outermost message that reaches the status
// condition.
func TruncateFragment(s string) string {
	return TruncateTo(s, maxErrorExprLen)
}

// FormatFragment bounds a tenant-authored *value* - a rendered forEach item, a dynamic map
// key that came back as the wrong type - for inclusion in an error message. It is the
// value-shaped counterpart to TruncateFragment, and exists because the obvious spelling,
// TruncateFragment(fmt.Sprintf("%v", val)), bounds only the result: fmt serializes the
// whole value into its own buffer first, so a rendered item large enough to be worth
// guarding against is paid for in full merely to display 256 bytes of it. This walks the
// value instead and stops as soon as it has enough, so the work is proportional to what
// survives rather than to what was rendered.
//
// The rendering follows fmt's %v for the shapes a rendered template value takes - maps
// print as map[k:v ...] with sorted keys, lists as [a b c] - so bounded previews read the
// same as the unbounded ones they replace.
func FormatFragment(val any) string {
	if s, ok := val.(string); ok {
		return TruncateFragment(s)
	}
	// One byte of headroom over the bound: a value that needed more than the bound then
	// arrives here longer than it, so TruncateTo marks the cut instead of returning an
	// exactly-sized string that reads as complete.
	return TruncateFragment(string(appendFragment(nil, val, maxErrorExprLen+1)))
}

// appendFragment appends a %v-like rendering of val to dst, giving up as soon as dst has
// reached limit. Containers are walked here rather than handed to fmt so that a large one
// is abandoned partway; the scalars at the leaves are small enough to format whole.
func appendFragment(dst []byte, val any, limit int) []byte {
	if len(dst) >= limit {
		return dst
	}
	switch v := val.(type) {
	case nil:
		return append(dst, "<nil>"...)
	case string:
		return appendBounded(dst, v, limit)
	case map[string]any:
		// Sorted keys, as fmt does: an unordered preview would name a different entry on
		// every reconcile, rewriting the status condition that carries it.
		dst = append(dst, "map["...)
		for i, k := range slices.Sorted(maps.Keys(v)) {
			if len(dst) >= limit {
				break
			}
			if i > 0 {
				dst = append(dst, ' ')
			}
			dst = appendBounded(dst, k, limit)
			dst = append(dst, ':')
			dst = appendFragment(dst, v[k], limit)
		}
		return append(dst, ']')
	case []any:
		dst = append(dst, '[')
		for i, item := range v {
			if len(dst) >= limit {
				break
			}
			if i > 0 {
				dst = append(dst, ' ')
			}
			dst = appendFragment(dst, item, limit)
		}
		return append(dst, ']')
	default:
		// Scalars: numbers, booleans, and the occasional type that is not part of a
		// rendered value's shape. Formatting one whole costs no more than reading it.
		return appendBounded(dst, fmt.Sprintf("%v", v), limit)
	}
}

// appendBounded appends as much of s as the room up to limit allows, cutting on a rune
// boundary. A preview assembled from pieces has to stay valid UTF-8 at every cut, not just
// the final one: the API server rejects invalid UTF-8 in a status condition, and a cut in
// the middle of the preview is not one the outer TruncateTo would ever repair.
func appendBounded(dst []byte, s string, limit int) []byte {
	room := limit - len(dst)
	if room <= 0 {
		return dst
	}
	if room >= len(s) {
		return append(dst, s...)
	}
	for room > 0 && !utf8.RuneStart(s[room]) {
		room--
	}
	return append(dst, s[:room]...)
}

// TruncateTo bounds s to maxBytes, appending a truncation marker when it cuts. The marker
// is counted against the bound, so the returned string never exceeds maxBytes - callers
// bound a message precisely because something downstream rejects one that is too large,
// and a return value that overshoots by the marker's length defeats that.
//
// A bound too small to hold the marker leaves no room to say anything about the cut, so
// the text is cut to the bound and the marker is dropped rather than pushing past it.
// A non-positive bound returns an empty string.
//
// The cut lands on a rune boundary: a fragment sliced mid-rune is invalid UTF-8, and the
// API server rejects invalid UTF-8 in a status condition - which would recreate the very
// failure this bound exists to prevent.
func TruncateTo(s string, maxBytes int) string {
	if maxBytes <= 0 {
		return ""
	}
	if len(s) <= maxBytes {
		return s
	}
	marker := TruncationMarker
	cut := maxBytes - len(marker)
	if cut <= 0 {
		marker = ""
		cut = maxBytes
	}
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut] + marker
}

// WithRenderTimeout derives a context bounding one render entry point. It is meant to wrap
// a single contiguous stretch of CEL evaluation, never a stretch that also performs API
// calls: an API stall under a render deadline would surface as DeadlineExceeded and be
// misread as a runaway template.
//
// A non-positive timeout means "no deadline" and yields the parent unchanged, so callers
// can always defer the returned cancel.
func WithRenderTimeout(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	if timeout <= 0 {
		return ctx, func() {}
	}
	return context.WithTimeout(ctx, timeout)
}
