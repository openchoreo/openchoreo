// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clusteragent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/client-go/dynamic"

	"github.com/openchoreo/openchoreo/internal/cluster-agent/messaging"
	"github.com/openchoreo/openchoreo/internal/resourcetree/protocol"
)

// defaultMaxResponseBytes caps the total marshaled match payload of one
// response. The tunnel reads agent responses unbounded, so this endpoint caps
// itself: a walk over a large cluster reports Truncated rather than pushing an
// unbounded frame through the WebSocket.
const defaultMaxResponseBytes = 8 << 20 // 8MB

// substitutionTokenOpen introduces a ${...} token. Anywhere a token can never
// be substituted — a matchLabels key, a namespace entry — its presence is
// rejected rather than taken literally, so a rule that would silently match
// nothing fails loudly instead.
const substitutionTokenOpen = "${"

// maxContinuationPagesPerQuery bounds how many CONTINUATION pages one query may
// follow across all of its namespace walks. A namespace's first list does not
// spend this budget — a query legitimately fans out over up to
// MaxParentsPerQuery namespaces and most hold a single page — so only paging
// deeper than that spends it. First pages are not unbounded either; they are
// charged against maxListCallsPerQuery below.
//
// This bounds objects scanned rather than matches kept, which the match limit
// already covers. At ListPageSize that is 25,600 objects in one query, roughly
// 50x the largest answer a query can return, so it can only trip on a
// collection far bigger than any query over it could meaningfully summarize.
// The wall clock is the real reason it exists: Handle runs a batch's queries
// sequentially under one 25s context (agent.go), so an unbounded walk over a
// huge namespace does not merely degrade its own query — it burns the shared
// deadline and leaves every later query in the batch reporting Internal. One
// query starving up to MaxQueriesPerRequest-1 siblings would contradict the
// independence the rest of this handler maintains.
//
// The cap is flat rather than derived from q.Limit on purpose: Limit is clamped
// to at most DefaultMatchLimit, so ceil(Limit/ListPageSize) never exceeds three
// pages, and any derived figure would sit far below the floor a sparse walk
// needs — the floor would decide every case anyway. Deriving would also shrink
// the cap exactly where it hurts, since a small limit over sparsely matching
// objects is the case that must page furthest.
const maxContinuationPagesPerQuery = 128

// defaultMaxListCallsPerQuery bounds how many LIST calls one query may issue in
// total, first pages included. maxContinuationPagesPerQuery alone does not bound
// a query's work: a labelSelector query pushes a different selector per parent
// and searches criteria.namespaces for each of them, so its first pages alone
// multiply out to MaxParentsPerQuery * MaxSelectorNamespaces — 2,048 sequential
// lists that the continuation budget never sees, spent inside the single 25s
// context Handle shares across a whole batch (agent.go). One such query would
// leave every later query in the batch reporting Internal, which is exactly the
// sibling starvation the continuation cap was added to prevent.
//
// The value covers the fanout the control plane actually generates — one query
// per edge over up to MaxParentsPerQuery parents, each searching one namespace —
// with room to spare, so no legitimate walk truncates on it. Only the
// multiplicative cross-namespace case can reach it, and that case is bounded
// rather than failed: the query returns the matches it collected with Truncated
// set, and its siblings keep their own untouched budgets.
const defaultMaxListCallsPerQuery = 512

// errTruncated stops a query's traversal once it has filled its match limit,
// exhausted the response byte budget, or run out of pages. It is not a failure:
// the matches already collected are returned alongside Truncated and a
// LimitExceeded error. Wrap it with the reason so the caller learns which
// ceiling it hit.
var errTruncated = errors.New("traversal stopped early")

// resourceTreeHandler answers resource tree match queries against the data
// plane's API server using the agent's own ServiceAccount.
type resourceTreeHandler struct {
	dyn              dynamic.Interface
	logger           *slog.Logger
	maxResponseBytes int
	maxListCalls     int
}

func newResourceTreeHandler(dyn dynamic.Interface, logger *slog.Logger) *resourceTreeHandler {
	return &resourceTreeHandler{
		dyn:              dyn,
		logger:           logger.With("component", "resource-tree"),
		maxResponseBytes: defaultMaxResponseBytes,
		maxListCalls:     defaultMaxListCallsPerQuery,
	}
}

// Handle answers a batch of match queries. Queries are independent: a query
// that fails validation or listing carries its own MatchError and never fails
// its siblings. Only an undecodable body and an oversized batch produce a
// non-200 tunnel response.
//
// req.Path and req.Method are deliberately ignored, and this handler must never
// answer 404. The control plane infers "this agent predates resource tree
// support" from the router's unknown-target 404 (router.go), so a 404 raised
// here for an unrecognized path would be read as version skew and silently
// downgrade the caller to its legacy traversal. If a second path is ever added,
// route on it and reject the unknown case with 400.
func (h *resourceTreeHandler) Handle(ctx context.Context, req *messaging.HTTPTunnelRequest) *messaging.HTTPTunnelResponse {
	var matchReq protocol.MatchRequest
	if err := json.Unmarshal(req.Body, &matchReq); err != nil {
		return messaging.NewHTTPTunnelErrorResponse(req, http.StatusBadRequest,
			fmt.Sprintf("invalid match request: %v", err))
	}

	if len(matchReq.Queries) > protocol.MaxQueriesPerRequest {
		return messaging.NewHTTPTunnelErrorResponse(req, http.StatusBadRequest,
			fmt.Sprintf("too many queries: %d exceeds the limit of %d",
				len(matchReq.Queries), protocol.MaxQueriesPerRequest))
	}

	resp := protocol.MatchResponse{
		Version: protocol.Version,
		Results: make([]protocol.MatchResult, 0, len(matchReq.Queries)),
	}

	if matchReq.Version != protocol.Version {
		message := fmt.Sprintf("unsupported request version %q; this agent speaks %q",
			matchReq.Version, protocol.Version)
		for _, q := range matchReq.Queries {
			resp.Results = append(resp.Results, errorResult(q.ID, protocol.CodeUnsupportedVersion, message))
		}
		return h.encodeResponse(req, resp)
	}

	duplicates := duplicateQueryIDs(matchReq.Queries)
	budget := &responseBudget{remaining: h.maxResponseBytes}

	for i := range matchReq.Queries {
		resp.Results = append(resp.Results, h.runQuery(ctx, &matchReq.Queries[i], duplicates, budget))
	}

	return h.encodeResponse(req, resp)
}

func (h *resourceTreeHandler) encodeResponse(req *messaging.HTTPTunnelRequest, resp protocol.MatchResponse) *messaging.HTTPTunnelResponse {
	body, err := json.Marshal(resp)
	if err != nil {
		h.logger.Error("failed to marshal match response", "requestID", req.RequestID, "error", err)
		return messaging.NewHTTPTunnelErrorResponse(req, http.StatusInternalServerError,
			fmt.Sprintf("failed to marshal match response: %v", err))
	}
	return messaging.NewHTTPTunnelSuccessResponse(req, http.StatusOK,
		map[string][]string{"Content-Type": {"application/json"}}, body)
}

func (h *resourceTreeHandler) runQuery(ctx context.Context, q *protocol.MatchQuery, duplicates map[string]bool, budget *responseBudget) protocol.MatchResult {
	// The id is checked first: the caller correlates results back to queries by
	// it, so a missing or colliding id makes the result unattributable no matter
	// what else the query says.
	if q.ID == "" {
		return errorResult(q.ID, protocol.CodeInvalidQuery, "query id is required")
	}
	if duplicates[q.ID] {
		return errorResult(q.ID, protocol.CodeInvalidQuery,
			fmt.Sprintf("query id %q appears more than once in the request", q.ID))
	}

	// The matcher is checked before anything else so an unrecognized one can
	// never be mistaken for the default: it fails closed with its own code and
	// reaches no API server.
	if q.Matcher != protocol.MatcherOwnerRef && q.Matcher != protocol.MatcherLabelSelector {
		return errorResult(q.ID, protocol.CodeUnsupportedMatcher,
			fmt.Sprintf("unsupported matcher %q", q.Matcher))
	}

	criteria, err := validateQuery(q)
	if err != nil {
		return errorResult(q.ID, protocol.CodeInvalidQuery, err.Error())
	}

	gvr := schema.GroupVersionResource{
		Group:    q.Child.Group,
		Version:  q.Child.Version,
		Resource: q.Child.Resource,
	}
	acc := newMatchAccumulator(q, budget)
	// Both budgets are per query, so one query truncating never shortens what a
	// sibling in the same batch may spend.
	walk := &walkBudget{
		continuations: maxContinuationPagesPerQuery,
		lists:         h.maxListCalls,
	}

	// A batch shares one wall-clock deadline across its queries. If it is already
	// spent, run no doomed LIST and report this query as truncated rather than
	// letting it fail as an internal error — the same shape a mid-walk deadline
	// produces below, so siblings after a wide query degrade uniformly.
	if ctxErr := ctx.Err(); ctxErr != nil {
		return deadlineTruncatedResult(q.ID, acc.matches, ctxErr)
	}

	switch q.Matcher {
	case protocol.MatcherOwnerRef:
		err = h.matchOwnerRef(ctx, q, gvr, acc, walk)
	case protocol.MatcherLabelSelector:
		err = h.matchLabelSelector(ctx, q, gvr, criteria, acc, walk)
	}

	switch {
	case errors.Is(err, errTruncated):
		return protocol.MatchResult{
			ID:        q.ID,
			Matches:   acc.matches,
			Truncated: true,
			Error: &protocol.MatchError{
				Code:    protocol.CodeLimitExceeded,
				Message: fmt.Sprintf("returned %d matches before the result was cut short: %v", len(acc.matches), err),
			},
		}
	case errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded):
		// Wall-clock exhaustion is a truncation, not an internal failure: keep the
		// matches already collected and mark the result incomplete instead of
		// discarding useful work and reporting Internal.
		return deadlineTruncatedResult(q.ID, acc.matches, err)
	case err != nil:
		var qErr *queryError
		if errors.As(err, &qErr) {
			return errorResult(q.ID, qErr.code, qErr.message)
		}
		code, message := classifyListError(err)
		h.logger.Warn("resource tree query failed",
			"queryID", q.ID,
			"matcher", q.Matcher,
			"resource", gvr.String(),
			"code", code,
			"error", err,
		)
		return errorResult(q.ID, code, message)
	}

	return protocol.MatchResult{ID: q.ID, Matches: acc.matches}
}

// matchOwnerRef lists the child kind once per distinct parent namespace — all
// an ownerRef can ever match — and keeps items whose ownerReferences name a
// parent UID. Like the traversal it replaces, it ignores `controller:`.
func (h *resourceTreeHandler) matchOwnerRef(ctx context.Context, q *protocol.MatchQuery, gvr schema.GroupVersionResource, acc *matchAccumulator, walk *walkBudget) error {
	parentUIDs := make([]string, 0, len(q.Parents))
	seenUID := make(map[string]bool, len(q.Parents))
	for _, p := range q.Parents {
		if !seenUID[p.UID] {
			seenUID[p.UID] = true
			parentUIDs = append(parentUIDs, p.UID)
		}
	}

	for _, ns := range parentNamespaces(q.Parents) {
		err := h.eachItem(ctx, gvr, ns, "", walk, func(item *unstructured.Unstructured) error {
			owners := ownerUIDSet(item)
			if len(owners) == 0 {
				return nil
			}
			// Walk the parents in request order so ParentUIDs is deterministic.
			matched := make([]string, 0, 1)
			for _, uid := range parentUIDs {
				if owners[uid] {
					matched = append(matched, uid)
				}
			}
			if len(matched) == 0 {
				return nil
			}
			return acc.add(item, matched)
		})
		if err != nil {
			return err
		}
	}
	return nil
}

// matchLabelSelector substitutes the parent tokens into the criteria per
// parent and pushes the resulting selector to the API server. A parent whose
// substituted value cannot be a legal label value is skipped, not failed: no
// object could carry that label, so "no matches" is the true answer for that
// parent and its siblings still match. A parent whose referenced field is empty
// is skipped for the same reason: the substituted selector keeps nothing of that
// parent, so it would match objects that are not its children.
func (h *resourceTreeHandler) matchLabelSelector(ctx context.Context, q *protocol.MatchQuery, gvr schema.GroupVersionResource, criteria *protocol.LabelSelectorCriteria, acc *matchAccumulator, walk *walkBudget) error {
	usesName, usesNamespace := criteria.ParentFieldUse()
	for _, p := range q.Parents {
		if len(criteria.Namespaces) == 0 && p.Namespace == "" {
			// Unreachable: validateQuery rejects the query outright in this
			// shape. Kept because the alternative is not a wrong answer but a
			// cluster-wide list — Namespace("") lists every namespace — so this
			// one skip is what makes "namespaces are never derived from an empty
			// parent namespace" true of the code rather than of its caller.
			h.logger.Debug("skipping cluster-scoped parent with no criteria.namespaces to search",
				"queryID", q.ID,
				"parent", p.Name,
			)
			continue
		}

		if (usesName && p.Name == "") || (usesNamespace && p.Namespace == "") {
			// With the referenced field empty, substitution leaves nothing of
			// this parent in the selector: a bare token yields "", a LEGAL empty
			// label value matching objects labeled empty, and an embedded one
			// (gw-${parent.metadata.name}-proxy) yields the constant "gw--proxy"
			// that every such parent shares. Mis-attribution either way, so skip
			// the parent; siblings still match.
			h.logger.Debug("skipping parent with an empty field the selector derives from",
				"queryID", q.ID,
				"parent", p.Name,
				"namespace", p.Namespace,
			)
			continue
		}
		set := make(labels.Set, len(criteria.MatchLabels))
		for key, value := range criteria.MatchLabels {
			substituted, err := protocol.SubstituteParentTokens(value, p.Name, p.Namespace)
			if err != nil {
				// Unreachable: the criteria's tokens were validated per query.
				// Kept as a defense so a substitution failure can never fall
				// through as a literal selector value.
				return &queryError{
					code:    protocol.CodeInvalidQuery,
					message: fmt.Sprintf("criteria.matchLabels[%q]: %v", key, err),
				}
			}
			set[key] = substituted
		}

		selector, err := labels.ValidatedSelectorFromSet(set)
		if err != nil {
			h.logger.Debug("skipping parent whose substituted selector cannot be a label selector",
				"queryID", q.ID,
				"parent", p.Name,
				"namespace", p.Namespace,
				"error", err,
			)
			continue
		}

		namespaces := criteria.Namespaces
		if len(namespaces) == 0 {
			namespaces = []string{p.Namespace}
		}

		for _, ns := range namespaces {
			err := h.eachItem(ctx, gvr, ns, selector.String(), walk, func(item *unstructured.Unstructured) error {
				return acc.add(item, []string{p.UID})
			})
			if err != nil {
				return err
			}
		}
	}
	return nil
}

// eachItem pages through one namespace's listing, invoking fn per item. A
// non-nil fn error aborts the traversal and is returned unchanged so errTruncated
// and queryError survive the walk. Every list spends the query's list budget and
// following a continue token spends its continuation budget too, so no walk can
// page on — or fan out — indefinitely.
func (h *resourceTreeHandler) eachItem(ctx context.Context, gvr schema.GroupVersionResource, namespace, selector string, walk *walkBudget, fn func(*unstructured.Unstructured) error) error {
	cont := ""
	for {
		if !walk.takeList() {
			return fmt.Errorf("%w: reached the %d list call limit while listing namespace %q",
				errTruncated, h.maxListCalls, namespace)
		}

		list, err := h.dyn.Resource(gvr).Namespace(namespace).List(ctx, metav1.ListOptions{
			LabelSelector: selector,
			Limit:         protocol.ListPageSize,
			Continue:      cont,
		})
		if err != nil {
			return err
		}

		for i := range list.Items {
			if err := fn(&list.Items[i]); err != nil {
				return err
			}
		}

		cont = list.GetContinue()
		if cont == "" {
			return nil
		}

		if !walk.takeContinuation() {
			return fmt.Errorf("%w: reached the %d continuation page limit while listing namespace %q",
				errTruncated, maxContinuationPagesPerQuery, namespace)
		}
	}
}

// queryError carries a protocol error code raised while executing a query.
type queryError struct {
	code    string
	message string
}

func (e *queryError) Error() string { return e.message }

// walkBudget is one query's allowance of API calls, shared by all of its
// namespace walks so neither a single runaway namespace nor a wide fanout can
// list unboundedly.
//
// The two counters answer different failure modes and are deliberately not one:
// continuations bounds how deep any one namespace may page, and lists bounds how
// many namespace walks a query may start at all. A single counter would have to
// take the larger value and would stop bounding depth.
type walkBudget struct {
	continuations int
	lists         int
}

func (b *walkBudget) takeContinuation() bool {
	if b.continuations <= 0 {
		return false
	}
	b.continuations--
	return true
}

func (b *walkBudget) takeList() bool {
	if b.lists <= 0 {
		return false
	}
	b.lists--
	return true
}

// responseBudget is the byte allowance shared by every query in one response.
type responseBudget struct {
	remaining int
}

func (b *responseBudget) take(n int) bool {
	if n > b.remaining {
		return false
	}
	b.remaining -= n
	return true
}

// matchAccumulator collects one query's matches, deduping by child UID so two
// parents that share a child produce one object with both parent UIDs.
type matchAccumulator struct {
	child        protocol.ChildKind
	metadataOnly bool
	limit        int
	budget       *responseBudget

	matches    []protocol.MatchedObject
	index      map[string]int
	parentSeen map[string]map[string]bool
}

func newMatchAccumulator(q *protocol.MatchQuery, budget *responseBudget) *matchAccumulator {
	return &matchAccumulator{
		child:        q.Child,
		metadataOnly: q.MetadataOnly,
		limit:        clampLimit(q.Limit),
		budget:       budget,
		index:        map[string]int{},
		parentSeen:   map[string]map[string]bool{},
	}
}

func (a *matchAccumulator) add(item *unstructured.Unstructured, parentUIDs []string) error {
	key := childKey(item)

	if idx, ok := a.index[key]; ok {
		for _, uid := range parentUIDs {
			if a.parentSeen[key][uid] {
				continue
			}
			// A duplicate child grows only by its new parent UIDs. Charge each one:
			// with up to MaxParentsPerQuery parents per match, the UID arrays are a
			// large share of the response, and leaving the merge path free let a
			// high-fan-in batch exceed the gateway's hard limit while the byte
			// budget looked untouched.
			if !a.budget.take(parentUIDCost(uid)) {
				return fmt.Errorf("%w: exhausted the response byte budget", errTruncated)
			}
			a.parentSeen[key][uid] = true
			a.matches[idx].ParentUIDs = append(a.matches[idx].ParentUIDs, uid)
		}
		return nil
	}

	if len(a.matches) >= a.limit {
		return fmt.Errorf("%w: reached the match limit of %d", errTruncated, a.limit)
	}

	raw, err := json.Marshal(shapeObject(item, a.child, a.metadataOnly))
	if err != nil {
		return fmt.Errorf("failed to marshal matched object: %w", err)
	}

	seen := make(map[string]bool, len(parentUIDs))
	uids := make([]string, 0, len(parentUIDs))
	cost := len(raw)
	for _, uid := range parentUIDs {
		if !seen[uid] {
			seen[uid] = true
			uids = append(uids, uid)
			cost += parentUIDCost(uid)
		}
	}
	// Charge the object and its initial parent UIDs together so the budget bounds
	// the whole marshaled match, UID array included, not just the object body.
	if !a.budget.take(cost) {
		return fmt.Errorf("%w: exhausted the response byte budget", errTruncated)
	}

	a.index[key] = len(a.matches)
	a.parentSeen[key] = seen
	a.matches = append(a.matches, protocol.MatchedObject{ParentUIDs: uids, Object: raw})
	return nil
}

// parentUIDCost is the byte budget one parent UID consumes in the marshaled
// response. It marshals the UID rather than assuming len(uid): the agent accepts
// any non-empty UID, and a value carrying quotes, backslashes or control bytes
// expands under json.Marshal (a control byte becomes the six bytes \uXXXX). Using
// the escaped length keeps the estimate from ever undercounting the real payload,
// so the response budget stays safely under the gateway's hard response limit.
// Marshaling a string never fails, so the error is discarded.
func parentUIDCost(uid string) int {
	encoded, _ := json.Marshal(uid)
	return len(encoded) + 1 // the quoted, escaped UID plus one separator byte
}

// validateQuery checks the fields every matcher needs and, for labelSelector,
// decodes and validates the criteria. It returns the normalized criteria for
// labelSelector and nil for ownerRef.
func validateQuery(q *protocol.MatchQuery) (*protocol.LabelSelectorCriteria, error) {
	if q.Child.Version == "" {
		return nil, fmt.Errorf("child.version is required")
	}
	if q.Child.Resource == "" {
		return nil, fmt.Errorf("child.resource is required")
	}
	// Kind is not part of the GVR but is required all the same: it backfills
	// apiVersion/kind on items served without them, and a match the caller
	// cannot identify would otherwise arrive as a success it cannot render.
	if q.Child.Kind == "" {
		return nil, fmt.Errorf("child.kind is required")
	}
	// The GVR is built from these fields and used to build a REST path, so its
	// syntax is validated before any list. A resource string with path-cleaning
	// segments (../) would otherwise escape the namespaced path and reach
	// cluster-wide core resources, and would also dodge the core-Secret strip
	// which keys off the literal resource name.
	if err := protocol.ValidateGVR(q.Child.Group, q.Child.Version, q.Child.Resource); err != nil {
		return nil, fmt.Errorf("child.%w", err)
	}
	if len(q.Parents) == 0 {
		return nil, fmt.Errorf("at least one parent is required")
	}
	if len(q.Parents) > protocol.MaxParentsPerQuery {
		return nil, fmt.Errorf("too many parents: %d exceeds the limit of %d",
			len(q.Parents), protocol.MaxParentsPerQuery)
	}
	for i, p := range q.Parents {
		if p.UID == "" {
			return nil, fmt.Errorf("parents[%d].uid is required", i)
		}
	}

	if q.Matcher == protocol.MatcherOwnerRef {
		if hasCriteria(q.Criteria) {
			return nil, fmt.Errorf("matcher %q does not accept criteria", protocol.MatcherOwnerRef)
		}
		// Same reasoning as the labelSelector guard below, and unconditional here
		// because ownerRef has no criteria.namespaces to ask for a wider search
		// deliberately: an empty namespace reaches Namespace("") and lists the
		// whole cluster. The agent is reachable through the gateway independently
		// of the control plane, so "the control plane never sends one" is not the
		// boundary this runs on.
		for i, p := range q.Parents {
			if p.Namespace == "" {
				return nil, fmt.Errorf("parents[%d].namespace is required for matcher %q; "+
					"an empty namespace would search every namespace", i, protocol.MatcherOwnerRef)
			}
		}
		return nil, nil
	}

	criteria, err := validateLabelSelectorCriteria(q.Criteria)
	if err != nil {
		return nil, err
	}

	// Without criteria.namespaces the search is scoped to each parent's own
	// namespace, and an empty one means "every namespace" to the client. Every
	// other guard here narrows the search when input is nonsense; this one would
	// widen it to the whole cluster, bounded only by the selector and the
	// agent's RBAC. A parent that genuinely needs a cluster-wide search says so
	// through criteria.namespaces, which is distinguishable from a caller bug.
	if len(criteria.Namespaces) == 0 {
		for i, p := range q.Parents {
			if p.Namespace == "" {
				return nil, fmt.Errorf("parents[%d].namespace is required for matcher %q unless criteria.namespaces is set; "+
					"an empty namespace would search every namespace", i, protocol.MatcherLabelSelector)
			}
		}
	}

	return criteria, nil
}

// validateLabelSelectorCriteria validates the criteria block agent-side rather
// than trusting the control plane's config validation: criteria arrive over the
// wire and a token where nothing substitutes — a namespace entry, a matchLabels
// key — would otherwise be taken literally and silently match nothing.
func validateLabelSelectorCriteria(raw json.RawMessage) (*protocol.LabelSelectorCriteria, error) {
	if !hasCriteria(raw) {
		return nil, fmt.Errorf("matcher %q requires criteria", protocol.MatcherLabelSelector)
	}

	var criteria protocol.LabelSelectorCriteria
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&criteria); err != nil {
		return nil, fmt.Errorf("invalid labelSelector criteria: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return nil, fmt.Errorf("invalid labelSelector criteria: multiple JSON values")
		}
		return nil, fmt.Errorf("invalid labelSelector criteria: %w", err)
	}

	if len(criteria.MatchLabels) == 0 {
		return nil, fmt.Errorf("criteria.matchLabels must not be empty")
	}

	for key, value := range criteria.MatchLabels {
		if strings.Contains(key, substitutionTokenOpen) {
			return nil, fmt.Errorf("criteria.matchLabels key %q contains a substitution token; only values are substituted", key)
		}
		if errs := validation.IsQualifiedName(key); len(errs) > 0 {
			return nil, fmt.Errorf("criteria.matchLabels key %q is not a valid label key: %s", key, strings.Join(errs, "; "))
		}
		// The probe arguments are irrelevant: substitution only fails on a
		// token the shared implementation does not recognize.
		if _, err := protocol.SubstituteParentTokens(value, "", ""); err != nil {
			return nil, fmt.Errorf("criteria.matchLabels[%q]: %w", key, err)
		}
		// A value with no token is already final, so an illegal one can never
		// match for any parent and is a caller bug worth reporting. A value
		// with a token is only knowable per parent, and is checked there —
		// where an unmatchable result skips that parent instead of failing the
		// query.
		if !strings.Contains(value, substitutionTokenOpen) {
			if errs := validation.IsValidLabelValue(value); len(errs) > 0 {
				return nil, fmt.Errorf("criteria.matchLabels[%q] value %q is not a valid label value: %s",
					key, value, strings.Join(errs, "; "))
			}
		}
	}

	// Config validation requires at least one parent-derived value so a selector
	// cannot attribute the same objects to every parent; criteria arrive over the
	// wire, so the agent enforces the same invariant rather than trusting it.
	if usesName, usesNamespace := criteria.ParentFieldUse(); !usesName && !usesNamespace {
		return nil, fmt.Errorf("criteria.matchLabels must derive at least one value from the parent via %s or %s; "+
			"a selector with no substitution token would attribute the same objects to every parent",
			protocol.TokenParentName, protocol.TokenParentNamespace)
	}

	if len(criteria.Namespaces) > protocol.MaxSelectorNamespaces {
		return nil, fmt.Errorf("too many criteria.namespaces: %d exceeds the limit of %d",
			len(criteria.Namespaces), protocol.MaxSelectorNamespaces)
	}

	normalized := make([]string, 0, len(criteria.Namespaces))
	seen := make(map[string]bool, len(criteria.Namespaces))
	for i, ns := range criteria.Namespaces {
		switch {
		case ns == "":
			return nil, fmt.Errorf("criteria.namespaces[%d] must not be empty", i)
		case strings.Contains(ns, substitutionTokenOpen):
			return nil, fmt.Errorf("criteria.namespaces[%d] %q contains a substitution token; namespace entries are never substituted", i, ns)
		case strings.Contains(ns, "*"):
			return nil, fmt.Errorf("criteria.namespaces[%d] %q must be a literal namespace name, not a wildcard", i, ns)
		}
		if errs := validation.IsDNS1123Label(ns); len(errs) > 0 {
			return nil, fmt.Errorf("criteria.namespaces[%d] %q is not a valid namespace name: %s", i, ns, strings.Join(errs, "; "))
		}
		if !seen[ns] {
			seen[ns] = true
			normalized = append(normalized, ns)
		}
	}
	criteria.Namespaces = normalized

	return &criteria, nil
}

func hasCriteria(raw json.RawMessage) bool {
	trimmed := strings.TrimSpace(string(raw))
	return trimmed != "" && trimmed != "null"
}

// duplicateQueryIDs reports the ids that appear more than once. Every query
// sharing a duplicated id fails: the caller correlates results by id, so a
// collision makes all of them unattributable.
func duplicateQueryIDs(queries []protocol.MatchQuery) map[string]bool {
	counts := make(map[string]int, len(queries))
	for _, q := range queries {
		counts[q.ID]++
	}
	duplicates := map[string]bool{}
	for id, count := range counts {
		if count > 1 {
			duplicates[id] = true
		}
	}
	return duplicates
}

// parentNamespaces returns the distinct namespaces of the parents, in request
// order. The namespace set is derived here and never read off the wire.
func parentNamespaces(parents []protocol.ParentRef) []string {
	namespaces := make([]string, 0, len(parents))
	seen := make(map[string]bool, len(parents))
	for _, p := range parents {
		if !seen[p.Namespace] {
			seen[p.Namespace] = true
			namespaces = append(namespaces, p.Namespace)
		}
	}
	return namespaces
}

func clampLimit(limit int) int {
	if limit <= 0 || limit > protocol.DefaultMatchLimit {
		return protocol.DefaultMatchLimit
	}
	return limit
}

func ownerUIDSet(item *unstructured.Unstructured) map[string]bool {
	refs := item.GetOwnerReferences()
	if len(refs) == 0 {
		return nil
	}
	uids := make(map[string]bool, len(refs))
	for _, ref := range refs {
		uids[string(ref.UID)] = true
	}
	return uids
}

// childKey identifies a matched object for deduping. UID is the real identity;
// namespace/name is a fallback for objects served without one.
func childKey(item *unstructured.Unstructured) string {
	if uid := string(item.GetUID()); uid != "" {
		return uid
	}
	return item.GetNamespace() + "/" + item.GetName()
}

// shapeObject trims a matched object for the wire. managedFields and kubectl's
// last-applied annotation are always dropped; metadataOnly reduces the object to
// its identity and metadata.
//
// The annotation is dropped unconditionally, like managedFields, because it is
// bookkeeping rather than anything a tree renders — and because keeping it would
// undo metadataOnly entirely on a Secret that was ever `kubectl apply`-ed: the
// trim below rebuilds the object around this same metadata map, so the whole
// serialized resource, data block included, would ride along inside it.
func shapeObject(item *unstructured.Unstructured, child protocol.ChildKind, metadataOnly bool) map[string]any {
	obj := item.DeepCopy().Object

	apiVersion, _ := obj["apiVersion"].(string)
	if apiVersion == "" {
		if apiVersion = childAPIVersion(child); apiVersion != "" {
			obj["apiVersion"] = apiVersion
		}
	}
	kind, _ := obj["kind"].(string)
	if kind == "" && child.Kind != "" {
		kind = child.Kind
		obj["kind"] = kind
	}

	metadata, _ := obj["metadata"].(map[string]any)
	if metadata != nil {
		delete(metadata, "managedFields")
		// obj is a deep copy, so the annotations map is this handler's to edit.
		if annotations, ok := metadata["annotations"].(map[string]any); ok {
			delete(annotations, protocol.LastAppliedConfigAnnotation)
		}
	}

	// Secret payloads must never cross the tunnel, even when the caller asks
	// for full objects. Key the defense off the GVR that was listed rather than
	// the configured or returned kind string: a non-canonical kind must not turn
	// off stripping, while a CRD named Secret may legitimately define unrelated
	// data fields.
	if child.Group == "" && child.Version == "v1" && child.Resource == "secrets" {
		delete(obj, "data")
		delete(obj, "stringData")
	}

	if !metadataOnly {
		return obj
	}

	if metadata == nil {
		metadata = map[string]any{}
	}
	trimmed := map[string]any{"metadata": metadata}
	if apiVersion != "" {
		trimmed["apiVersion"] = apiVersion
	}
	if kind != "" {
		trimmed["kind"] = kind
	}
	return trimmed
}

func childAPIVersion(child protocol.ChildKind) string {
	if child.Group == "" {
		return child.Version
	}
	return child.Group + "/" + child.Version
}

// classifyListError maps an API server error onto a protocol error code. An
// authorization denial is reported as Forbidden so the caller can tell missing
// RBAC apart from a genuine failure.
//
// 401 is folded into Forbidden deliberately — both are authorization denials
// and the protocol has no separate code — but the two have different causes: a
// 403 means the agent's ServiceAccount lacks RBAC on that resource, while a 401
// usually means its token was rejected or has expired. A UI that renders
// Forbidden as "grant the agent RBAC" would be wrong for the 401 case; the
// message carries the underlying API error either way.
func classifyListError(err error) (string, string) {
	if apierrors.IsForbidden(err) || apierrors.IsUnauthorized(err) {
		return protocol.CodeForbidden, err.Error()
	}
	return protocol.CodeInternal, err.Error()
}

func errorResult(id, code, message string) protocol.MatchResult {
	return protocol.MatchResult{
		ID:    id,
		Error: &protocol.MatchError{Code: code, Message: message},
	}
}

// deadlineTruncatedResult reports a query cut short by the shared request
// deadline: the matches collected so far, marked truncated, rather than an
// Internal error that would discard them. It mirrors the byte/list-budget
// truncation shape so the caller treats deadline exhaustion the same way.
func deadlineTruncatedResult(id string, matches []protocol.MatchedObject, err error) protocol.MatchResult {
	return protocol.MatchResult{
		ID:        id,
		Matches:   matches,
		Truncated: true,
		Error: &protocol.MatchError{
			Code:    protocol.CodeLimitExceeded,
			Message: fmt.Sprintf("returned %d matches before the request deadline: %v", len(matches), err),
		},
	}
}
