// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/openchoreo/openchoreo/internal/observer/api/gen"
	"github.com/openchoreo/openchoreo/internal/observer/store/deliveryinsights"
)

// ScopeUIDResolver resolves scope names to the UIDs the delivery insights store is keyed
// by. Satisfied by *ResourceUIDResolver (production) and the passthrough resolver (dev).
type ScopeUIDResolver interface {
	GetProjectUID(ctx context.Context, namespace, project string) (string, error)
	GetComponentUID(ctx context.Context, namespace, project, component string) (string, error)
	GetEnvironmentUID(ctx context.Context, namespace, environment string) (string, error)
}

// passthroughUIDResolver treats scope names as UIDs directly. Development affordance for
// running against seeded dummy data without a control plane to resolve names against
// (enabled via DELIVERY_INSIGHTS_UID_RESOLUTION=passthrough).
type passthroughUIDResolver struct{}

// NewPassthroughUIDResolver returns a resolver that echoes names back as UIDs.
func NewPassthroughUIDResolver() ScopeUIDResolver {
	return passthroughUIDResolver{}
}

func (passthroughUIDResolver) GetProjectUID(_ context.Context, _, project string) (string, error) {
	return project, nil
}

func (passthroughUIDResolver) GetComponentUID(_ context.Context, _, _, component string) (string, error) {
	return component, nil
}

func (passthroughUIDResolver) GetEnvironmentUID(_ context.Context, _, environment string) (string, error) {
	return environment, nil
}

// DeliveryInsightsService serves the Delivery Insights (DORA metrics) read API.
type DeliveryInsightsService interface {
	QueryDoraMetrics(ctx context.Context, req gen.DoraMetricsQueryRequest) (*gen.DoraMetricsQueryResponse, error)
	QueryDoraDeployments(
		ctx context.Context, req gen.DoraDeploymentsQueryRequest) (*gen.DoraDeploymentsQueryResponse, error)
}

// DoraMetricsService computes DORA metrics from the delivery insights store:
// count metrics (deployment frequency, change failure rate) from pre-computed rollups,
// distribution metrics (lead time, MTTR) from fact rows because percentiles do not
// re-aggregate across buckets.
type DoraMetricsService struct {
	store    deliveryinsights.Store
	resolver ScopeUIDResolver
	logger   *slog.Logger
	// collecting is whether this observer derives delivery facts at all
	// (observer.featurePreview.deliveryInsights.enabled). False means nothing is
	// being written,
	// so every metric stays empty however much is deployed. Reads are served
	// either way, which is why it has to be reported: without it an empty result
	// is ambiguous, a scope that deployed nothing being indistinguishable from an
	// observer that was never asked to collect.
	collecting bool
	// eventsAvailable reports whether the deployed adapter can actually serve the
	// event sweep. Read per request rather than captured, because the aggregator
	// only discovers it from the adapter's first 501, which is after this service
	// is built. Nil when nothing is collecting.
	eventsAvailable func() bool
}

var _ DeliveryInsightsService = (*DoraMetricsService)(nil)

// NewDeliveryInsightsService creates the delivery insights query service.
func NewDeliveryInsightsService(
	store deliveryinsights.Store, resolver ScopeUIDResolver, logger *slog.Logger,
	collecting bool, eventsAvailable func() bool,
) *DoraMetricsService {
	return &DoraMetricsService{
		store:           store,
		resolver:        resolver,
		logger:          logger,
		collecting:      collecting,
		eventsAvailable: eventsAvailable,
	}
}

// resolvedDeliveryInsightsScope is a search scope translated to store keys.
type resolvedDeliveryInsightsScope struct {
	scopeType      string
	scopeUID       string
	namespace      string
	projectUID     string
	componentUID   string
	environmentUID string
}

func (s *DoraMetricsService) resolveScope(
	ctx context.Context, scope gen.ComponentSearchScope,
) (resolvedDeliveryInsightsScope, error) {
	namespace := strings.TrimSpace(scope.Namespace)
	project := strings.TrimSpace(stringPtrValue(scope.Project))
	component := strings.TrimSpace(stringPtrValue(scope.Component))
	environment := strings.TrimSpace(stringPtrValue(scope.Environment))

	rs := resolvedDeliveryInsightsScope{
		scopeType: deliveryinsights.ScopeTypeNamespace,
		scopeUID:  namespace,
		namespace: namespace,
	}

	if project != "" {
		uid, err := s.resolver.GetProjectUID(ctx, namespace, project)
		if err != nil {
			return rs, wrapScopeError(err, "project", project)
		}
		rs.projectUID = uid
		rs.scopeType = deliveryinsights.ScopeTypeProject
		rs.scopeUID = uid
	}
	if component != "" {
		uid, err := s.resolver.GetComponentUID(ctx, namespace, project, component)
		if err != nil {
			return rs, wrapScopeError(err, "component", component)
		}
		rs.componentUID = uid
		rs.scopeType = deliveryinsights.ScopeTypeComponent
		rs.scopeUID = uid
	}
	if environment != "" {
		uid, err := s.resolver.GetEnvironmentUID(ctx, namespace, environment)
		if err != nil {
			return rs, wrapScopeError(err, "environment", environment)
		}
		rs.environmentUID = uid
	}
	return rs, nil
}

func (rs resolvedDeliveryInsightsScope) factQuery(startMs, endMs int64) deliveryinsights.FactQuery {
	return deliveryinsights.FactQuery{
		Namespace:      rs.namespace,
		ProjectUID:     rs.projectUID,
		ComponentUID:   rs.componentUID,
		EnvironmentUID: rs.environmentUID,
		StartMs:        startMs,
		EndMs:          endMs,
		// The metrics are statistics over the window, so every matching row has to be
		// read. Any cap here keeps one end of an ordered distribution and drops the
		// other, so percentiles come out biased while CountDeployments stays exact --
		// the two would disagree, and raising the cap would only move the point at
		// which they start to.
		AllRows: true,
	}
}

// Payload structs mirror the OpenAPI response schema; the built payload is JSON
// round-tripped into the generated type (same pattern as the alerts service) to avoid
// hand-assembling oapi-codegen's nested anonymous structs.
type doraWindowPayload struct {
	StartTime   string `json:"startTime"`
	EndTime     string `json:"endTime"`
	GeneratedAt string `json:"generatedAt"`
}

type doraFrequencySummaryPayload struct {
	Total          int      `json:"total"`
	PerDay         float64  `json:"perDay"`
	Classification string   `json:"classification"`
	DeltaPct       *float64 `json:"deltaPct"`
}

type doraLeadTimeSummaryPayload struct {
	P50Ms          *int64   `json:"p50Ms"`
	P95Ms          *int64   `json:"p95Ms"`
	Coverage       float64  `json:"coverage"`
	Classification string   `json:"classification"`
	DeltaPct       *float64 `json:"deltaPct"`
}

type doraCFRSummaryPayload struct {
	Rate           float64  `json:"rate"`
	Failed         int      `json:"failed"`
	Total          int      `json:"total"`
	Classification string   `json:"classification"`
	DeltaPct       *float64 `json:"deltaPct"`
}

type doraMTTRSummaryPayload struct {
	MeanMs         *int64   `json:"meanMs"`
	P50Ms          *int64   `json:"p50Ms"`
	Recoveries     int      `json:"recoveries"`
	Classification string   `json:"classification"`
	DeltaPct       *float64 `json:"deltaPct"`
}

type doraSummaryPayload struct {
	DeploymentFrequency *doraFrequencySummaryPayload `json:"deploymentFrequency,omitempty"`
	LeadTime            *doraLeadTimeSummaryPayload  `json:"leadTime,omitempty"`
	ChangeFailureRate   *doraCFRSummaryPayload       `json:"changeFailureRate,omitempty"`
	MTTR                *doraMTTRSummaryPayload      `json:"mttr,omitempty"`
}

type doraFrequencyPointPayload struct {
	BucketStart string `json:"bucketStart"`
	Count       int    `json:"count"`
}

type doraLeadTimePointPayload struct {
	BucketStart string `json:"bucketStart"`
	P50Ms       int64  `json:"p50Ms"`
	P75Ms       int64  `json:"p75Ms"`
	P95Ms       int64  `json:"p95Ms"`
}

type doraCFRPointPayload struct {
	BucketStart string  `json:"bucketStart"`
	Rate        float64 `json:"rate"`
	Failed      int     `json:"failed"`
	Total       int     `json:"total"`
}

type doraMTTRPointPayload struct {
	BucketStart string `json:"bucketStart"`
	MeanMs      int64  `json:"meanMs"`
	P50Ms       int64  `json:"p50Ms"`
	Count       int    `json:"count"`
}

type doraSeriesPayload struct {
	DeploymentFrequency *[]doraFrequencyPointPayload `json:"deploymentFrequency,omitempty"`
	LeadTime            *[]doraLeadTimePointPayload  `json:"leadTime,omitempty"`
	ChangeFailureRate   *[]doraCFRPointPayload       `json:"changeFailureRate,omitempty"`
	MTTR                *[]doraMTTRPointPayload      `json:"mttr,omitempty"`
}

// doraDataAvailabilityPayload answers why the metrics beside it might be empty.
// A scope that has deployed nothing and an observer that is not collecting both
// return a success with empty series, so without this a client cannot say which
// it is showing. Neither field depends on the query, so both travel with every
// response rather than costing a second call.
type doraDataAvailabilityPayload struct {
	// Whether this observer derives delivery facts at all.
	Collecting bool `json:"collecting"`
	// Whether the deployed adapter can serve the delivery event sweep. False
	// leaves deployment frequency, lead time and change failure rate without
	// input; mean time to recovery comes from incidents and is unaffected.
	DeliveryEvents bool `json:"deliveryEvents"`
}

type doraMetricsResponsePayload struct {
	DataAvailability doraDataAvailabilityPayload `json:"dataAvailability"`
	Scope            gen.ComponentSearchScope    `json:"scope"`
	Granularity      string                      `json:"granularity"`
	Window           doraWindowPayload           `json:"window"`
	Summary          doraSummaryPayload          `json:"summary"`
	Series           doraSeriesPayload           `json:"series"`
}

// dataAvailability reports what this observer can currently produce.
func (s *DoraMetricsService) dataAvailability() doraDataAvailabilityPayload {
	return doraDataAvailabilityPayload{
		Collecting:     s.collecting,
		DeliveryEvents: s.eventsAvailable != nil && s.eventsAvailable(),
	}
}

// emptyMetricsResponse answers a metrics query on an observer that is not
// collecting: the window and scope as asked for, no series, and the availability
// that explains why.
func (s *DoraMetricsService) emptyMetricsResponse(
	req gen.DoraMetricsQueryRequest,
) (*gen.DoraMetricsQueryResponse, error) {
	granularity := deliveryinsights.GranularityDaily
	if req.Granularity != nil && *req.Granularity != "" {
		granularity = string(*req.Granularity)
	}
	payload := doraMetricsResponsePayload{
		DataAvailability: s.dataAvailability(),
		Scope:            req.SearchScope,
		Granularity:      granularity,
		Window: doraWindowPayload{
			StartTime:   req.StartTime.UTC().Format(time.RFC3339),
			EndTime:     req.EndTime.UTC().Format(time.RFC3339),
			GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		},
	}
	var response gen.DoraMetricsQueryResponse
	if err := roundTripJSON(payload, &response); err != nil {
		return nil, err
	}
	return &response, nil
}

// emptyDeploymentsResponse is emptyMetricsResponse for the drill-down list: no
// rows, and a count that says there were none to page through.
func (s *DoraMetricsService) emptyDeploymentsResponse(
	_ gen.DoraDeploymentsQueryRequest,
) (*gen.DoraDeploymentsQueryResponse, error) {
	total := 0
	return &gen.DoraDeploymentsQueryResponse{TotalCount: &total}, nil
}

// QueryDoraMetrics computes the requested DORA metrics for a scope and window.
func (s *DoraMetricsService) QueryDoraMetrics(
	ctx context.Context, req gen.DoraMetricsQueryRequest,
) (*gen.DoraMetricsQueryResponse, error) {
	// No store means the feature is switched off, and its tables were never
	// created. That is a configuration answer, not a failure: the response says
	// nothing is being collected and carries empty series, which is what the
	// client needs to tell "switched off" from "nothing deployed". Failing here
	// would leave it unable to tell either from a broken observer.
	if s.store == nil {
		return s.emptyMetricsResponse(req)
	}

	rs, err := s.resolveScope(ctx, req.SearchScope)
	if err != nil {
		return nil, err
	}

	granularity := deliveryinsights.GranularityDaily
	if req.Granularity != nil && *req.Granularity != "" {
		granularity = string(*req.Granularity)
	}
	wanted := wantedMetrics(req.Metrics)

	startMs := req.StartTime.UTC().UnixMilli()
	endMs := req.EndTime.UTC().UnixMilli()
	windowMs := endMs - startMs

	// Summary totals are counted over the exact window from the facts, not summed
	// from rollup buckets. A bucket that straddles the window start would otherwise
	// be added in full, which both inflated totals by however much of that bucket
	// preceded the window (so the same window reported different totals per
	// granularity) and double-counted that bucket into the preceding window's
	// comparison. Lead time and MTTR already read the facts directly.
	curTotals, err := s.countDeployments(ctx, rs, startMs, endMs)
	if err != nil {
		return nil, err
	}
	prevTotals, err := s.countDeployments(ctx, rs, startMs-windowMs, startMs)
	if err != nil {
		return nil, err
	}

	// The series stays bucket-based: it is a presentation of the requested
	// granularity, so the bucket containing the window start is shown whole.
	bucketStart := deliveryinsights.BucketStartMs(granularity, startMs)
	current, err := s.queryRollupsByBucket(ctx, rs, granularity, bucketStart, endMs)
	if err != nil {
		return nil, err
	}

	payload := doraMetricsResponsePayload{
		DataAvailability: s.dataAvailability(),
		Scope:            req.SearchScope,
		Granularity:      granularity,
		Window: doraWindowPayload{
			StartTime:   req.StartTime.UTC().Format(time.RFC3339),
			EndTime:     req.EndTime.UTC().Format(time.RFC3339),
			GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		},
	}

	if wanted[string(gen.DoraMetricsQueryRequestMetricsDeploymentFrequency)] {
		payload.Summary.DeploymentFrequency = buildFrequencySummary(curTotals, prevTotals, windowMs)
		points := buildFrequencySeries(current, granularity, bucketStart, endMs)
		payload.Series.DeploymentFrequency = &points
	}
	if wanted[string(gen.DoraMetricsQueryRequestMetricsChangeFailureRate)] {
		payload.Summary.ChangeFailureRate = buildCFRSummary(curTotals, prevTotals)
		points := buildCFRSeries(current, granularity, bucketStart, endMs)
		payload.Series.ChangeFailureRate = &points
	}
	if wanted[string(gen.DoraMetricsQueryRequestMetricsLeadTime)] {
		summary, ltErr := s.buildLeadTimeSummary(ctx, rs, startMs, endMs, windowMs, curTotals)
		if ltErr != nil {
			return nil, ltErr
		}
		payload.Summary.LeadTime = summary
		points := buildLeadTimeSeries(current)
		payload.Series.LeadTime = &points
	}
	if wanted[string(gen.DoraMetricsQueryRequestMetricsMttr)] {
		summary, mttrErr := s.buildMTTRSummary(ctx, rs, startMs, endMs, windowMs)
		if mttrErr != nil {
			return nil, mttrErr
		}
		payload.Summary.MTTR = summary
		points := buildMTTRSeries(current)
		payload.Series.MTTR = &points
	}

	var response gen.DoraMetricsQueryResponse
	if err := roundTripJSON(payload, &response); err != nil {
		return nil, err
	}
	return &response, nil
}

// QueryDoraDeployments returns the individual deployments behind the metrics.
func (s *DoraMetricsService) QueryDoraDeployments(
	ctx context.Context, req gen.DoraDeploymentsQueryRequest,
) (*gen.DoraDeploymentsQueryResponse, error) {
	// See QueryDoraMetrics: switched off is answered, not failed.
	if s.store == nil {
		return s.emptyDeploymentsResponse(req)
	}

	rs, err := s.resolveScope(ctx, req.SearchScope)
	if err != nil {
		return nil, err
	}

	start := time.Now()
	query := rs.factQuery(req.StartTime.UTC().UnixMilli(), req.EndTime.UTC().UnixMilli())
	// factQuery reads every row because the metrics above are statistics over the
	// window. This is a page of rows for a list, so the cap is the point: leaving
	// AllRows set would ignore Limit outright and answer every request with the
	// whole window.
	query.AllRows = false
	query.Limit = intPtrValue(req.Limit, defaultQueryLimit)
	if req.SortOrder != nil {
		query.SortOrder = string(*req.SortOrder)
	}

	facts, total, err := s.store.QueryDeploymentFacts(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("query deployment facts: %w", err)
	}

	type deploymentPayload struct {
		DeployedAt       string `json:"deployedAt"`
		ProjectName      string `json:"projectName"`
		ComponentName    string `json:"componentName"`
		EnvironmentName  string `json:"environmentName"`
		ComponentRelease string `json:"componentRelease"`
		Commit           string `json:"commit"`
		Outcome          string `json:"outcome"`
		FailedBy         string `json:"failedBy"`
		FailureReason    string `json:"failureReason"`
		IncidentID       string `json:"incidentId"`
		LeadTimeMs       *int64 `json:"leadTimeMs"`
	}
	deployments := make([]deploymentPayload, 0, len(facts))
	for i := range facts {
		f := &facts[i]
		deployments = append(deployments, deploymentPayload{
			DeployedAt:       time.UnixMilli(f.OccurredMs()).UTC().Format(time.RFC3339),
			ProjectName:      f.ProjectName,
			ComponentName:    f.ComponentName,
			EnvironmentName:  f.EnvironmentName,
			ComponentRelease: f.ComponentRelease,
			Commit:           f.CommitSHA,
			Outcome:          f.Outcome,
			FailedBy:         f.FailedBy,
			FailureReason:    f.FailureReason,
			IncidentID:       f.IncidentID,
			LeadTimeMs:       f.LeadTimeMs,
		})
	}

	responsePayload := struct {
		Deployments []deploymentPayload `json:"deployments"`
		TotalCount  int                 `json:"totalCount"`
		TookMs      int                 `json:"tookMs"`
	}{
		Deployments: deployments,
		TotalCount:  total,
		TookMs:      int(time.Since(start).Milliseconds()),
	}

	var response gen.DoraDeploymentsQueryResponse
	if err := roundTripJSON(responsePayload, &response); err != nil {
		return nil, err
	}
	return &response, nil
}

// queryRollupsByBucket returns the scope's rollups keyed by bucket start.
func (s *DoraMetricsService) queryRollupsByBucket(
	ctx context.Context, rs resolvedDeliveryInsightsScope, granularity string, startMs, endMs int64,
) (map[int64]deliveryinsights.MetricRollup, error) {
	rollups, err := s.store.QueryRollups(ctx, deliveryinsights.RollupQuery{
		ScopeType:      rs.scopeType,
		ScopeUID:       rs.scopeUID,
		EnvironmentUID: rs.environmentUID,
		Granularity:    granularity,
		StartMs:        startMs,
		EndMs:          endMs,
		// Placed the same way the fact reads are, so a scope whose project and
		// component do not belong together returns nothing here too rather than
		// only from the fact-backed halves of the response.
		Namespace:  rs.namespace,
		ProjectUID: rs.projectUID,
	})
	if err != nil {
		return nil, fmt.Errorf("query delivery metric rollups: %w", err)
	}
	byBucket := make(map[int64]deliveryinsights.MetricRollup, len(rollups))
	for _, r := range rollups {
		byBucket[r.BucketStartMs] = r
	}
	return byBucket, nil
}

type rollupTotals struct {
	total   int
	success int
	failed  int
}

// countDeployments tallies deployments over exactly [startMs, endMs) for the scope.
func (s *DoraMetricsService) countDeployments(
	ctx context.Context, rs resolvedDeliveryInsightsScope, startMs, endMs int64,
) (rollupTotals, error) {
	counts, err := s.store.CountDeployments(ctx, rs.factQuery(startMs, endMs))
	if err != nil {
		return rollupTotals{}, fmt.Errorf("count deployments: %w", err)
	}
	return rollupTotals{
		total:   counts.Total,
		success: counts.Success,
		failed:  counts.Failed,
	}, nil
}

func buildFrequencySummary(cur, prev rollupTotals, windowMs int64) *doraFrequencySummaryPayload {
	perDay := 0.0
	if windowMs > 0 {
		perDay = round2(float64(cur.success) / (float64(windowMs) / float64(24*time.Hour.Milliseconds())))
	}
	return &doraFrequencySummaryPayload{
		Total:          cur.success,
		PerDay:         perDay,
		Classification: classifyDeploymentFrequency(cur.success, perDay),
		DeltaPct:       deltaPct(float64(cur.success), float64(prev.success)),
	}
}

func buildCFRSummary(cur, prev rollupTotals) *doraCFRSummaryPayload {
	rate := 0.0
	if cur.total > 0 {
		rate = round3(float64(cur.failed) / float64(cur.total))
	}
	var prevRate float64
	if prev.total > 0 {
		prevRate = float64(prev.failed) / float64(prev.total)
	}
	return &doraCFRSummaryPayload{
		Rate:           rate,
		Failed:         cur.failed,
		Total:          cur.total,
		Classification: classifyChangeFailureRate(cur.total, rate),
		DeltaPct:       deltaPct(rate, prevRate),
	}
}

func (s *DoraMetricsService) buildLeadTimeSummary(
	ctx context.Context, rs resolvedDeliveryInsightsScope, startMs, endMs, windowMs int64, cur rollupTotals,
) (*doraLeadTimeSummaryPayload, error) {
	leadTimes, err := s.store.QueryLeadTimes(ctx, rs.factQuery(startMs, endMs))
	if err != nil {
		return nil, fmt.Errorf("query lead times: %w", err)
	}
	prevLeadTimes, err := s.store.QueryLeadTimes(ctx, rs.factQuery(startMs-windowMs, startMs))
	if err != nil {
		return nil, fmt.Errorf("query previous-window lead times: %w", err)
	}

	p50 := deliveryinsights.Percentile(leadTimes, 0.50)
	prevP50 := deliveryinsights.Percentile(prevLeadTimes, 0.50)
	coverage := 0.0
	if cur.success > 0 {
		coverage = round3(math.Min(1, float64(len(leadTimes))/float64(cur.success)))
	}

	summary := &doraLeadTimeSummaryPayload{
		P50Ms:          p50,
		P95Ms:          deliveryinsights.Percentile(leadTimes, 0.95),
		Coverage:       coverage,
		Classification: classifyLeadTime(p50),
	}
	if p50 != nil && prevP50 != nil {
		summary.DeltaPct = deltaPct(float64(*p50), float64(*prevP50))
	}
	return summary, nil
}

func (s *DoraMetricsService) buildMTTRSummary(
	ctx context.Context, rs resolvedDeliveryInsightsScope, startMs, endMs, windowMs int64,
) (*doraMTTRSummaryPayload, error) {
	durations, err := s.store.QueryRecoveryDurations(ctx, rs.factQuery(startMs, endMs))
	if err != nil {
		return nil, fmt.Errorf("query recovery durations: %w", err)
	}
	prevDurations, err := s.store.QueryRecoveryDurations(ctx, rs.factQuery(startMs-windowMs, startMs))
	if err != nil {
		return nil, fmt.Errorf("query previous-window recovery durations: %w", err)
	}

	mean := deliveryinsights.Mean(durations)
	prevMean := deliveryinsights.Mean(prevDurations)
	summary := &doraMTTRSummaryPayload{
		MeanMs:         mean,
		P50Ms:          deliveryinsights.Percentile(durations, 0.50),
		Recoveries:     len(durations),
		Classification: classifyMTTR(mean),
	}
	if mean != nil && prevMean != nil {
		summary.DeltaPct = deltaPct(float64(*mean), float64(*prevMean))
	}
	return summary, nil
}

func buildFrequencySeries(
	byBucket map[int64]deliveryinsights.MetricRollup, granularity string, startMs, endMs int64,
) []doraFrequencyPointPayload {
	points := make([]doraFrequencyPointPayload, 0)
	for b := startMs; b < endMs; b = deliveryinsights.NextBucketStartMs(granularity, b) {
		points = append(points, doraFrequencyPointPayload{
			BucketStart: formatMs(b),
			Count:       byBucket[b].DeploySuccess,
		})
	}
	return points
}

func buildCFRSeries(
	byBucket map[int64]deliveryinsights.MetricRollup, granularity string, startMs, endMs int64,
) []doraCFRPointPayload {
	points := make([]doraCFRPointPayload, 0)
	for b := startMs; b < endMs; b = deliveryinsights.NextBucketStartMs(granularity, b) {
		r := byBucket[b]
		rate := 0.0
		if r.DeployTotal > 0 {
			rate = round3(float64(r.DeployFailed) / float64(r.DeployTotal))
		}
		points = append(points, doraCFRPointPayload{
			BucketStart: formatMs(b),
			Rate:        rate,
			Failed:      r.DeployFailed,
			Total:       r.DeployTotal,
		})
	}
	return points
}

func buildLeadTimeSeries(byBucket map[int64]deliveryinsights.MetricRollup) []doraLeadTimePointPayload {
	points := make([]doraLeadTimePointPayload, 0)
	for _, b := range sortedBuckets(byBucket) {
		r := byBucket[b]
		if r.LeadTimeP50Ms == nil {
			continue
		}
		points = append(points, doraLeadTimePointPayload{
			BucketStart: formatMs(b),
			P50Ms:       *r.LeadTimeP50Ms,
			P75Ms:       int64PtrValue(r.LeadTimeP75Ms),
			P95Ms:       int64PtrValue(r.LeadTimeP95Ms),
		})
	}
	return points
}

func buildMTTRSeries(byBucket map[int64]deliveryinsights.MetricRollup) []doraMTTRPointPayload {
	points := make([]doraMTTRPointPayload, 0)
	for _, b := range sortedBuckets(byBucket) {
		r := byBucket[b]
		if r.MTTRMeanMs == nil {
			continue
		}
		points = append(points, doraMTTRPointPayload{
			BucketStart: formatMs(b),
			MeanMs:      *r.MTTRMeanMs,
			P50Ms:       int64PtrValue(r.MTTRP50Ms),
			Count:       r.RecoveryCount,
		})
	}
	return points
}

// DORA performance classifications, per the DORA research program's benchmark tiers.
func classifyDeploymentFrequency(total int, perDay float64) string {
	switch {
	case total == 0:
		return string(gen.DoraClassificationUnknown)
	case perDay >= 1:
		return string(gen.DoraClassificationElite)
	case perDay >= 1.0/7:
		return string(gen.DoraClassificationHigh)
	case perDay >= 1.0/30:
		return string(gen.DoraClassificationMedium)
	default:
		return string(gen.DoraClassificationLow)
	}
}

func classifyLeadTime(p50Ms *int64) string {
	if p50Ms == nil {
		return string(gen.DoraClassificationUnknown)
	}
	switch {
	case *p50Ms < 24*time.Hour.Milliseconds():
		return string(gen.DoraClassificationElite)
	case *p50Ms < 7*24*time.Hour.Milliseconds():
		return string(gen.DoraClassificationHigh)
	case *p50Ms < 30*24*time.Hour.Milliseconds():
		return string(gen.DoraClassificationMedium)
	default:
		return string(gen.DoraClassificationLow)
	}
}

func classifyChangeFailureRate(total int, rate float64) string {
	switch {
	case total == 0:
		return string(gen.DoraClassificationUnknown)
	case rate <= 0.05:
		return string(gen.DoraClassificationElite)
	case rate <= 0.10:
		return string(gen.DoraClassificationHigh)
	case rate <= 0.15:
		return string(gen.DoraClassificationMedium)
	default:
		return string(gen.DoraClassificationLow)
	}
}

func classifyMTTR(meanMs *int64) string {
	if meanMs == nil {
		return string(gen.DoraClassificationUnknown)
	}
	switch {
	case *meanMs < time.Hour.Milliseconds():
		return string(gen.DoraClassificationElite)
	case *meanMs < 24*time.Hour.Milliseconds():
		return string(gen.DoraClassificationHigh)
	case *meanMs < 7*24*time.Hour.Milliseconds():
		return string(gen.DoraClassificationMedium)
	default:
		return string(gen.DoraClassificationLow)
	}
}

func wantedMetrics(metrics *[]gen.DoraMetricsQueryRequestMetrics) map[string]bool {
	wanted := make(map[string]bool, 4)
	if metrics == nil || len(*metrics) == 0 {
		for _, m := range []gen.DoraMetricsQueryRequestMetrics{
			gen.DoraMetricsQueryRequestMetricsDeploymentFrequency, gen.DoraMetricsQueryRequestMetricsLeadTime, gen.DoraMetricsQueryRequestMetricsChangeFailureRate, gen.DoraMetricsQueryRequestMetricsMttr,
		} {
			wanted[string(m)] = true
		}
		return wanted
	}
	for _, m := range *metrics {
		wanted[string(m)] = true
	}
	return wanted
}

// deltaPct returns the percentage change from prev to cur, or nil when there is no
// meaningful baseline (prev == 0).
func deltaPct(cur, prev float64) *float64 {
	if prev == 0 {
		return nil
	}
	d := round1((cur - prev) / prev * 100)
	return &d
}

func sortedBuckets(byBucket map[int64]deliveryinsights.MetricRollup) []int64 {
	buckets := make([]int64, 0, len(byBucket))
	for b := range byBucket {
		buckets = append(buckets, b)
	}
	sort.Slice(buckets, func(i, j int) bool { return buckets[i] < buckets[j] })
	return buckets
}

func roundTripJSON(payload any, out any) error {
	raw, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal delivery insights response payload: %w", err)
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("failed to unmarshal delivery insights response payload: %w", err)
	}
	return nil
}

func formatMs(ms int64) string {
	return time.UnixMilli(ms).UTC().Format(time.RFC3339)
}

func int64PtrValue(v *int64) int64 {
	if v == nil {
		return 0
	}
	return *v
}

func round1(v float64) float64 { return math.Round(v*10) / 10 }

func round2(v float64) float64 { return math.Round(v*100) / 100 }

func round3(v float64) float64 { return math.Round(v*1000) / 1000 }
