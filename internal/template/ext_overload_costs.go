// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package template

import (
	"math"

	"github.com/google/cel-go/common"
	"github.com/google/cel-go/common/types"
	"github.com/google/cel-go/common/types/ref"
	"github.com/google/cel-go/common/types/traits"
)

// This file carries the runtime cost trackers for the cel-go extension overloads that the
// pinned cel-go release does not meter itself.
//
// Newer cel-go installs its own interpreter.OverloadCostTracker functions for these
// extensions, so `lists.range(n)` is charged by the list it allocates and `s.lowerAscii()` by
// the bytes it walks. They arrive one extension at a time: lists in v0.25, strings in v0.28,
// encoders in v0.30. On v0.22.1, which this branch pins, ext.listsLib.ProgramOptions,
// ext.stringLib.ProgramOptions and ext.encoderLib.ProgramOptions all return an empty
// slice. Every one of those overloads therefore falls through to cel-go's
// flat one-unit default, and the per-expression cost limit and the reconcile budget see
// nothing. Two shapes made that concrete before these trackers existed:
//
//   - `size(lists.range(10000).distinct())` measured 3 units and ran for ~160ms, so a template
//     could repeat it a few hundred times inside a 2,000,000-unit budget.
//   - `size([lists.range(10000), ...x4500])` measured 4,511 units and allocated ~1GB.
//
// The formulas below mirror the ones newer cel-go ships, so a template costs the same order of
// magnitude here as it will after an upgrade. THEY MUST BE DELETED ON THE FIRST BUMP PAST
// v0.24, and the ones still live re-checked at every bump after it: the library's own trackers
// take precedence over an ActualCostEstimator (see CostTracker.costCall, which consults
// overloadTrackers first and returns), so a duplicate is dead code at best, and any formula
// that diverged would silently become the losing one. A partial bump is the worst case,
// because a version that meters lists but not strings leaves half this file live.
//
// ext.Sets() is deliberately absent. It installs its own trackers as far back as v0.22.1, and
// those already win over anything this estimator returns.
//
// Sizes differ from cel-go's own ext.actualSize in one respect, noted on runtimeSize: strings
// are measured in bytes rather than runes.

// extOverloadCost prices one extension-overload call, or reports nil to leave the call on
// cel-go's own accounting.
//
// Dispatch is by overload ID wherever cel-go resolves one. It does not always: a function
// whose overloads share an arity cannot be resolved from a DynType argument, and every
// template input is declared DynType, so `payload.sort()` and `payload.reverse()` arrive with
// an empty ID. Those are the calls a tenant actually controls, so the three affected functions
// - sort, @sortByAssociatedKeys and reverse - are keyed by name instead, which covers the
// resolved and unresolved forms in one case. reverse is the only name shared by a list and a
// string overload, and the receiver's own type tells them apart.
func extOverloadCost(function, overloadID string, args []ref.Val, result ref.Val) *uint64 {
	var cost uint64

	switch overloadID {
	case "lists_range", "list_reverse", "list_slice":
		cost = allocatingListCost(1, runtimeSize(result))

	case "list_distinct":
		cost = listSelfCompareCost(argAt(args, 0))

	case "list_flatten", "list_flatten_int":
		cost = listFlattenCost(args, result)

	case "list_join", "list_join_string":
		cost = listJoinCost(argAt(args, 0), result)

	case "string_split_string", "string_split_string_int":
		cost = stringSplitCost(argAt(args, 0), result)

	case "string_replace_string_string", "string_replace_string_string_int":
		cost = stringReplaceCost(args, result)

	case "string_lower_ascii", "string_upper_ascii", "string_trim", "string_reverse",
		"string_substring_int", "string_substring_int_int":
		cost = stringTransformCost(argAt(args, 0), result)

	case "string_index_of_string", "string_index_of_string_int",
		"string_last_index_of_string", "string_last_index_of_string_int":
		cost = stringSearchCost(argAt(args, 0), argAt(args, 1))

	case "string_char_at_int":
		cost = stringCharAtCost(argAt(args, 0))

	case "base64_encode_bytes", "base64_decode_string":
		cost = byteScanCost(argAt(args, 0))

	default:
		switch function {
		case "sort":
			cost = listSelfCompareCost(argAt(args, 0))
		case "@sortByAssociatedKeys":
			// The sort is driven by the key list, which is what newer cel-go charges for.
			cost = listSelfCompareCost(argAt(args, 1))
		case "reverse":
			if _, isList := argAt(args, 0).(traits.Lister); isList {
				cost = allocatingListCost(1, runtimeSize(result))
			} else {
				cost = stringTransformCost(argAt(args, 0), result)
			}
		default:
			return nil
		}
	}

	return &cost
}

// allocatingListCost charges a call that builds a list: the elements it produced, scaled by
// how many times each is touched, plus the dispatch and the allocation itself.
func allocatingListCost(costFactor float64, size uint64) uint64 {
	if costFactor < 0 {
		costFactor = 1.0
	}
	return saturatingAdd(saturatingAdd(scaledCost(size, costFactor), callDispatchCost), common.ListCreateBaseCost)
}

// listSelfCompareCost charges a worst-case O(n^2) pass in which every element is compared
// against every other: distinct, sort and sortBy all do this. Comparing strings or bytes walks
// their contents as well, which is the extra factor.
func listSelfCompareCost(val ref.Val) uint64 {
	size := runtimeSize(val)
	costFactor := 2.0
	lister, ok := val.(traits.Lister)
	if !ok || size == 0 {
		return allocatingListCost(costFactor, 0)
	}
	switch lister.Get(types.IntZero).Type() {
	case types.StringType, types.BytesType:
		costFactor += common.StringTraversalCostFactor
	}
	return allocatingListCost(costFactor, saturatingMul(size, size))
}

// listFlattenCost charges a flatten by whichever is larger: the list it produced, or the depth
// it was asked to descend times the list it was handed. Neither bound dominates the other -
// a deep flatten visits more elements than it returns, a shallow one over nested lists returns
// more than it was given - and cel-go has used each of the two formulas in turn.
func listFlattenCost(args []ref.Val, result ref.Val) uint64 {
	depth := uint64(1)
	if d, ok := argAt(args, 1).(types.Int); ok && int64(d) > 1 {
		depth = uint64(d) //nolint:gosec // guarded above: d is greater than 1, so the conversion is in range
	}
	traversed := saturatingMul(depth, runtimeSize(argAt(args, 0)))
	return allocatingListCost(1, max(traversed, runtimeSize(result)))
}

// listJoinCost charges a join by the elements walked plus the string built.
func listJoinCost(input, result ref.Val) uint64 {
	return saturatingAdd(
		saturatingAdd(callDispatchCost, traversalCost(saturatingAdd(runtimeSize(input), 1))),
		runtimeSize(result))
}

// stringSplitCost charges a split by the string walked, the allocation of the result list,
// and the number of pieces it holds - the piece count, not their bytes, which is what
// newer cel-go charges here too.
func stringSplitCost(input, result ref.Val) uint64 {
	scan := saturatingAdd(callDispatchCost, traversalCost(saturatingAdd(runtimeSize(input), 1)))
	return saturatingAdd(saturatingAdd(scan, runtimeSize(result)), common.ListCreateBaseCost)
}

// stringTransformCost charges an O(n) rewrite of a string by what it read and what it wrote.
func stringTransformCost(input, result ref.Val) uint64 {
	return saturatingAdd(
		saturatingAdd(callDispatchCost, traversalCost(runtimeSize(input))),
		runtimeSize(result))
}

// stringSearchCost charges an O(n*m) scan of a haystack for a needle.
func stringSearchCost(haystack, needle ref.Val) uint64 {
	return saturatingAdd(callDispatchCost,
		traversalCost(saturatingMul(runtimeSize(haystack), runtimeSize(needle))))
}

// stringReplaceCost charges the search for every occurrence plus the string that was built.
// An empty target or needle still costs a pass, so neither is allowed to zero the product.
func stringReplaceCost(args []ref.Val, result ref.Val) uint64 {
	targetSize := max(runtimeSize(argAt(args, 0)), 1)
	needleSize := max(runtimeSize(argAt(args, 1)), 1)
	search := traversalCost(saturatingMul(targetSize, needleSize))
	return saturatingAdd(saturatingAdd(callDispatchCost, search), runtimeSize(result))
}

// stringCharAtCost charges a single character read: cel-go converts the whole string to runes
// to index it, so the cost is proportional to the string, not to the one character returned.
func stringCharAtCost(input ref.Val) uint64 {
	return saturatingAdd(saturatingAdd(callDispatchCost, traversalCost(runtimeSize(input))), 1)
}

// byteScanCost charges a call that reads its argument end to end and writes a proportional
// result, which is what the base64 codecs do.
func byteScanCost(input ref.Val) uint64 {
	return saturatingAdd(callDispatchCost, traversalCost(runtimeSize(input)))
}

// callDispatchCost is what cel-go charges for reaching an overload at all, before anything it
// reads or writes is counted.
const callDispatchCost uint64 = 1

// runtimeSize reports the size cel-go's own trackers would use for a value: the entry count of
// a collection, the length of a string or byte sequence, and one for a scalar that reports no
// size at all.
//
// It differs from cel-go's ext.actualSize in measuring a string in bytes rather than runes.
// These overloads walk bytes, and a rune count under-charges multi-byte input by up to a
// factor of four - or sixteen where two sizes are multiplied, as in stringSearchCost and
// stringReplaceCost. Over-charging is the direction that matters, because the cost is a
// guard rather than a bill.
func runtimeSize(val ref.Val) uint64 {
	if val == nil {
		return 0
	}
	switch v := val.Value().(type) {
	case string:
		return uint64(len(v))
	case []byte:
		return uint64(len(v))
	}
	size := collectionLen(val)
	if size < 0 {
		return 1
	}
	return uint64(size)
}

// argAt reads one argument without assuming the arity cel-go dispatched with. A tracker runs
// inside the cost observer, where an index out of range would panic the reconcile rather than
// mismeter it.
func argAt(args []ref.Val, i int) ref.Val {
	if i < 0 || i >= len(args) {
		return nil
	}
	return args[i]
}

// scaledCost multiplies a size by a cost factor and rounds up, saturating rather than wrapping
// or producing an implementation-defined conversion at the top of the uint64 range.
func scaledCost(size uint64, factor float64) uint64 {
	scaled := math.Ceil(float64(size) * factor)
	if scaled <= 0 {
		return 0
	}
	if scaled >= math.MaxUint64 {
		return math.MaxUint64
	}
	return uint64(scaled)
}

// saturatingMul multiplies two costs without wrapping, for the same reason saturatingAdd does:
// a wrapped product of an n^2 term would read as a tiny cost.
func saturatingMul(a, b uint64) uint64 {
	if a == 0 || b == 0 {
		return 0
	}
	if a > math.MaxUint64/b {
		return math.MaxUint64
	}
	return a * b
}
