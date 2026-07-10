// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

// Command seeder populates a delivery insights database with deterministic dummy data so
// the DORA read API and the Backstage Insights UI can be developed against realistic
// shapes before the aggregator exists. It also seeds matching alert/incident entries
// (the CFR/MTTR join sources) into the same database. Development tool only — not
// shipped in any image.
//
// Scope UIDs are deliberately set to the scope NAMES so the observer can serve this
// data with INSIGHTS_UID_RESOLUTION=passthrough (no control plane needed to resolve
// names to UIDs).
//
// Usage:
//
//	go run ./internal/observer/store/deliveryinsights/seeder \
//	  -dsn "file:./insights-dev.db?_journal=WAL" -days 120
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"math/rand"
	"os"
	"strings"
	"time"

	"github.com/openchoreo/openchoreo/internal/observer/store/alertentry"
	"github.com/openchoreo/openchoreo/internal/observer/store/deliveryinsights"
	"github.com/openchoreo/openchoreo/internal/observer/store/incidententry"
)

type componentProfile struct {
	projectName    string
	componentName  string
	deploysPerWeek float64       // successful prod cadence; dev/staging scale up from this
	leadTimeMedian time.Duration // typical commit→deploy latency
	failureRate    float64       // fraction of deployments that fail
	provenanceRate float64       // fraction of deployments carrying commit provenance
}

// Scope UIDs equal scope names (see package comment), so profiles carry names only.
var profiles = []componentProfile{
	{
		projectName: "checkout", componentName: "checkout-api",
		deploysPerWeek: 9, leadTimeMedian: 4 * time.Hour, failureRate: 0.05, provenanceRate: 0.95,
	},
	{
		projectName: "checkout", componentName: "checkout-worker",
		deploysPerWeek: 2, leadTimeMedian: 26 * time.Hour, failureRate: 0.10, provenanceRate: 0.85,
	},
	{
		projectName: "payments", componentName: "payments-api",
		deploysPerWeek: 5, leadTimeMedian: 9 * time.Hour, failureRate: 0.08, provenanceRate: 0.90,
	},
	{
		projectName: "payments", componentName: "fraud-detector",
		deploysPerWeek: 1, leadTimeMedian: 3 * 24 * time.Hour, failureRate: 0.18, provenanceRate: 0.60,
	},
}

type environment struct {
	name       string
	rateFactor float64 // deployment cadence relative to the production-like env
}

// parseEnvironments builds the environment list from a comma-separated flag value,
// ordered dev-like → production-like. The first environment deploys at 3x the base
// cadence, the last at 1x, and any in between at 1.5x.
func parseEnvironments(value string) ([]environment, error) {
	names := strings.Split(value, ",")
	envs := make([]environment, 0, len(names))
	for _, raw := range names {
		name := strings.TrimSpace(raw)
		if name == "" {
			continue
		}
		envs = append(envs, environment{name: name, rateFactor: 1.5})
	}
	if len(envs) == 0 {
		return nil, fmt.Errorf("at least one environment name is required")
	}
	envs[0].rateFactor = 3.0
	envs[len(envs)-1].rateFactor = 1.0
	if len(envs) == 1 {
		envs[0].rateFactor = 1.0
	}
	return envs, nil
}

var rolloutFailureReasons = []string{
	"CrashLoopBackOff", "ProgressDeadlineExceeded", "ImagePullBackOff", "ApplyFailed",
}

var alertRuleNames = []string{"high-error-rate", "latency-spike", "pod-crashloop"}

var severities = []string{"critical", "major", "minor"}

func main() {
	dsn := flag.String("dsn", "file:./insights-dev.db?_journal=WAL",
		"SQL DSN for the delivery insights store")
	backend := flag.String("backend", deliveryinsights.BackendSQLite,
		"store backend: sqlite or postgresql")
	days := flag.Int("days", 120, "number of days of history to generate")
	seed := flag.Int64("seed", 1, "random seed (fixed seed => reproducible data)")
	namespace := flag.String("namespace", "default", "org namespace to seed")
	seedIncidents := flag.Bool("seed-incidents", true,
		"also seed matching alert/incident entries into the same database")
	environmentNames := flag.String("environments", "dev,staging,prod",
		"comma-separated environment names, ordered dev-like to production-like")
	flag.Parse()

	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	environments, err := parseEnvironments(*environmentNames)
	if err != nil {
		logger.Error("Invalid -environments flag", "error", err)
		os.Exit(1)
	}
	if err := run(
		logger, *dsn, *backend, *namespace, environments, *days, *seed, *seedIncidents,
	); err != nil {
		logger.Error("Seeding failed", "error", err)
		os.Exit(1)
	}
}

func run(
	logger *slog.Logger,
	dsn, backend, namespace string,
	environments []environment,
	days int,
	seed int64,
	seedIncidents bool,
) error {
	store, err := deliveryinsights.New(backend, dsn, logger)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := store.Close(); closeErr != nil {
			logger.Error("Failed to close store", "error", closeErr)
		}
	}()

	ctx := context.Background()
	if err := store.Initialize(ctx); err != nil {
		return err
	}

	rng := rand.New(rand.NewSource(seed)) //nolint:gosec // deterministic dummy data, not crypto
	now := time.Now().UTC()
	start := now.AddDate(0, 0, -days)

	var facts []deliveryinsights.DeploymentFact
	var recoveries []deliveryinsights.RecoveryFact
	for _, p := range profiles {
		for _, env := range environments {
			f, r := generateComponentEnv(rng, namespace, p, env, start, now)
			facts = append(facts, f...)
			recoveries = append(recoveries, r...)
		}
	}

	incidentCount := 0
	if seedIncidents {
		incidentCount, err = seedIncidentEntries(ctx, logger, backend, dsn, namespace, facts, recoveries)
		if err != nil {
			return err
		}
	}

	if err := store.UpsertDeploymentFacts(ctx, facts); err != nil {
		return err
	}
	if err := store.UpsertRecoveryFacts(ctx, recoveries); err != nil {
		return err
	}

	rollups := deliveryinsights.BuildRollups(facts, recoveries, now.UnixMilli())
	if err := store.UpsertRollups(ctx, rollups); err != nil {
		return err
	}

	for _, source := range []string{"events", "incidents"} {
		if err := store.SetWatermark(ctx, source, now.UnixMilli()); err != nil {
			return err
		}
	}

	logger.Info("Seeded delivery insights store",
		"dsn", dsn,
		"days", days,
		"deploymentFacts", len(facts),
		"recoveryFacts", len(recoveries),
		"rollups", len(rollups),
		"incidents", incidentCount,
	)
	return nil
}

// generateComponentEnv produces the deployment history of one component in one
// environment: successful deploys at the profile's cadence and failures at its failure
// rate — half rollout failures (with a health-sourced recovery episode), half
// incident-attributed (recovery + incident/alert rows added later by
// seedIncidentEntries). Commit provenance is present at the profile's provenance rate.
func generateComponentEnv(
	rng *rand.Rand,
	namespace string,
	p componentProfile,
	env environment,
	start, now time.Time,
) ([]deliveryinsights.DeploymentFact, []deliveryinsights.RecoveryFact) {
	var facts []deliveryinsights.DeploymentFact
	var recoveries []deliveryinsights.RecoveryFact

	perDay := p.deploysPerWeek / 7 * env.rateFactor
	meanGap := time.Duration(float64(24*time.Hour) / perDay)

	release := 0
	for t := randomOffset(rng, start, meanGap); t.Before(now); t = t.Add(jitteredGap(rng, meanGap)) {
		release++
		releaseUID := fmt.Sprintf("uid-rr-%s-%s-%04d", p.componentName, env.name, release)
		ready := t.UnixMilli()
		started := t.Add(-time.Duration(20+rng.Intn(100)) * time.Second).UnixMilli()

		fact := deliveryinsights.DeploymentFact{
			ReleaseUID:       releaseUID,
			OrgNamespace:     namespace,
			ProjectUID:       p.projectName,
			ComponentUID:     p.componentName,
			EnvironmentUID:   env.name,
			ProjectName:      p.projectName,
			ComponentName:    p.componentName,
			EnvironmentName:  env.name,
			ComponentRelease: fmt.Sprintf("%s-%d", p.componentName, release),
			StartedMs:        &started,
			ReadyMs:          &ready,
			Outcome:          deliveryinsights.OutcomeSuccess,
			UpdatedAtMs:      ready,
		}

		if rng.Float64() < p.provenanceRate {
			lead := jitteredDuration(rng, p.leadTimeMedian)
			authored := t.Add(-lead).UnixMilli()
			leadMs := ready - authored
			fact.CommitSHA = randomCommitSHA(rng)
			fact.CommitAuthoredMs = &authored
			fact.LeadTimeMs = &leadMs
		}

		if rng.Float64() < p.failureRate {
			fact.Outcome = deliveryinsights.OutcomeFailed
			if rng.Float64() < 0.5 {
				fact.FailedBy = deliveryinsights.FailedByRollout
				fact.FailureReason = rolloutFailureReasons[rng.Intn(len(rolloutFailureReasons))]
				recoveries = append(recoveries,
					newRecovery(rng, namespace, p, env, releaseUID, deliveryinsights.RecoverySourceHealth, t))
			} else {
				fact.FailedBy = deliveryinsights.FailedByIncident
				// IncidentID is linked after the incident entry is written.
				recoveries = append(recoveries,
					newRecovery(rng, namespace, p, env, releaseUID, deliveryinsights.RecoverySourceIncident, t))
			}
		}

		facts = append(facts, fact)
	}

	return facts, recoveries
}

func newRecovery(
	rng *rand.Rand,
	namespace string,
	p componentProfile,
	env environment,
	releaseUID, source string,
	deployedAt time.Time,
) deliveryinsights.RecoveryFact {
	failureStart := deployedAt.Add(time.Duration(2+rng.Intn(90)) * time.Minute)
	duration := time.Duration(10+rng.Intn(230)) * time.Minute
	recovered := failureStart.Add(duration).UnixMilli()
	durationMs := duration.Milliseconds()

	severity := ""
	if source == deliveryinsights.RecoverySourceIncident {
		severity = severities[rng.Intn(len(severities))]
	}

	return deliveryinsights.RecoveryFact{
		ID:               "recovery-" + releaseUID,
		OrgNamespace:     namespace,
		ProjectUID:       p.projectName,
		ComponentUID:     p.componentName,
		EnvironmentUID:   env.name,
		ReleaseUID:       releaseUID,
		Severity:         severity,
		Source:           source,
		FailureStartedMs: failureStart.UnixMilli(),
		RecoveredMs:      &recovered,
		DurationMs:       &durationMs,
	}
}

// seedIncidentEntries writes one alert entry + one incident entry per incident-sourced
// recovery into the same database (the observer's incident/alert stores create their own
// tables there), links the generated incident IDs back into the facts and recovery
// facts, and leaves the most recent incident open (status active, recovery unresolved)
// so the demo shows an in-flight failure.
func seedIncidentEntries(
	ctx context.Context,
	logger *slog.Logger,
	backend, dsn, namespace string,
	facts []deliveryinsights.DeploymentFact,
	recoveries []deliveryinsights.RecoveryFact,
) (int, error) {
	alertStore, err := alertentry.New(backend, dsn, logger.With("component", "alert-seeder"))
	if err != nil {
		return 0, err
	}
	defer closeQuietly(logger, alertStore.Close)
	if err := alertStore.Initialize(ctx); err != nil {
		return 0, err
	}

	incidentStore, err := incidententry.New(backend, dsn, logger.With("component", "incident-seeder"))
	if err != nil {
		return 0, err
	}
	defer closeQuietly(logger, incidentStore.Close)
	if err := incidentStore.Initialize(ctx); err != nil {
		return 0, err
	}

	factByRelease := make(map[string]*deliveryinsights.DeploymentFact, len(facts))
	for i := range facts {
		factByRelease[facts[i].ReleaseUID] = &facts[i]
	}

	// The most recent incident-sourced recovery stays open.
	openIdx := -1
	for i := range recoveries {
		if recoveries[i].Source != deliveryinsights.RecoverySourceIncident {
			continue
		}
		if openIdx == -1 || recoveries[i].FailureStartedMs > recoveries[openIdx].FailureStartedMs {
			openIdx = i
		}
	}

	// Deterministic ID generation lives in the stores (UUIDs), which is fine: facts are
	// re-linked to whatever IDs come back, so re-seeding stays self-consistent.
	count := 0
	for i := range recoveries {
		r := &recoveries[i]
		if r.Source != deliveryinsights.RecoverySourceIncident {
			continue
		}

		triggeredAt := time.UnixMilli(r.FailureStartedMs).UTC().Format(time.RFC3339Nano)
		ruleName := alertRuleNames[count%len(alertRuleNames)]
		alertID, err := alertStore.WriteAlertEntry(ctx, &alertentry.AlertEntry{
			Timestamp:            triggeredAt,
			AlertRuleName:        ruleName,
			AlertRuleCRName:      ruleName,
			AlertRuleCRNamespace: namespace,
			AlertValue:           "0.12",
			NamespaceName:        namespace,
			ComponentName:        r.ComponentUID,
			EnvironmentName:      r.EnvironmentUID,
			ProjectName:          r.ProjectUID,
			ComponentID:          r.ComponentUID,
			EnvironmentID:        r.EnvironmentUID,
			ProjectID:            r.ProjectUID,
			IncidentEnabled:      true,
			Severity:             r.Severity,
			Description:          fmt.Sprintf("%s breached on %s (%s)", ruleName, r.ComponentUID, r.EnvironmentUID),
			SourceType:           "metric",
			SourceMetric:         "error_rate",
			ConditionOperator:    ">",
			ConditionThreshold:   0.05,
			ConditionWindow:      "5m",
			ConditionInterval:    "1m",
		})
		if err != nil {
			return count, fmt.Errorf("failed to seed alert entry: %w", err)
		}

		entry := &incidententry.IncidentEntry{
			AlertID:         alertID,
			Timestamp:       triggeredAt,
			Status:          incidententry.StatusResolved,
			TriggeredAt:     triggeredAt,
			Description:     fmt.Sprintf("Incident on %s (%s): %s", r.ComponentUID, r.EnvironmentUID, ruleName),
			NamespaceName:   namespace,
			ComponentName:   r.ComponentUID,
			EnvironmentName: r.EnvironmentUID,
			ProjectName:     r.ProjectUID,
			ComponentID:     r.ComponentUID,
			EnvironmentID:   r.EnvironmentUID,
			ProjectID:       r.ProjectUID,
		}
		if i == openIdx {
			entry.Status = incidententry.StatusActive
			r.RecoveredMs = nil
			r.DurationMs = nil
		} else if r.RecoveredMs != nil {
			entry.ResolvedAt = time.UnixMilli(*r.RecoveredMs).UTC().Format(time.RFC3339Nano)
		}

		incidentID, err := incidentStore.WriteIncidentEntry(ctx, entry)
		if err != nil {
			return count, fmt.Errorf("failed to seed incident entry: %w", err)
		}

		r.IncidentID = incidentID
		if fact := factByRelease[r.ReleaseUID]; fact != nil {
			fact.IncidentID = incidentID
		}
		count++
	}

	return count, nil
}

func closeQuietly(logger *slog.Logger, closeFn func() error) {
	if err := closeFn(); err != nil {
		logger.Error("Failed to close store", "error", err)
	}
}

func randomOffset(rng *rand.Rand, start time.Time, meanGap time.Duration) time.Time {
	return start.Add(time.Duration(rng.Int63n(int64(meanGap) + 1)))
}

// jitteredGap spreads deployments unevenly (0.3x–2.5x the mean gap) so daily buckets vary.
func jitteredGap(rng *rand.Rand, mean time.Duration) time.Duration {
	factor := 0.3 + rng.Float64()*2.2
	return time.Duration(float64(mean) * factor)
}

// jitteredDuration returns a value around the median (0.4x–3x) skewed toward the tail,
// giving lead-time distributions a realistic p95 well above p50.
func jitteredDuration(rng *rand.Rand, median time.Duration) time.Duration {
	factor := 0.4 + rng.Float64()*rng.Float64()*2.6
	return time.Duration(float64(median) * factor)
}

func randomCommitSHA(rng *rand.Rand) string {
	const hexChars = "0123456789abcdef"
	sha := make([]byte, 40)
	for i := range sha {
		sha[i] = hexChars[rng.Intn(len(hexChars))]
	}
	return string(sha)
}
