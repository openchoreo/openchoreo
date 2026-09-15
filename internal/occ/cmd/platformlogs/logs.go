// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package platformlogs

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"slices"
	"strings"
	"syscall"
	"time"

	obsgen "github.com/openchoreo/openchoreo/internal/observer/api/gen"
	"github.com/openchoreo/openchoreo/internal/occ/auth"
	"github.com/openchoreo/openchoreo/internal/occ/cmd/config"
	"github.com/openchoreo/openchoreo/internal/occ/resources/client"
)

const (
	defaultSince = "1h"
	defaultLimit = 100

	// maxWindow mirrors the observer's own cap, so an over-long --since is refused
	// here with a clear message rather than as a 400 from the server.
	maxWindow = 30 * 24 * time.Hour

	// pollLimit is the page size of a --follow poll. Combined with ascending order it
	// means a burst larger than one page is delivered oldest-first and the next poll
	// resumes where this one stopped, instead of skipping past the gap.
	pollLimit    = 1000
	pollInterval = 2 * time.Second

	outputText = "text"
	outputJSON = "json"

	// viewAction is the permission the observer checks, named here so the error a
	// rejected user sees tells them what to ask for.
	viewAction = "platformlogs:view"
)

// validLevels are the severities the observer accepts, in the order it documents them.
var validLevels = []string{"DEBUG", "INFO", "WARN", "ERROR"}

// observerAPI is the slice of the generated observer client this package uses.
type observerAPI interface {
	GetPlatformLogsWithResponse(ctx context.Context, params *obsgen.GetPlatformLogsParams,
		reqEditors ...obsgen.RequestEditorFn) (*obsgen.GetPlatformLogsResp, error)
}

// PlatformLogs queries platform logs through an observability plane resolved from the
// control plane.
type PlatformLogs struct {
	client client.Interface
}

// New creates a PlatformLogs backed by the given control plane client.
func New(c client.Interface) *PlatformLogs {
	return &PlatformLogs{client: c}
}

// Logs resolves the named observability plane, queries its platform logs and prints them.
func (p *PlatformLogs) Logs(params LogsParams) error {
	ctx := context.Background()

	if err := validateOutput(params.Output); err != nil {
		return err
	}
	levels, err := normalizeLevels(params.Levels)
	if err != nil {
		return err
	}
	window, err := parseSince(params.Since)
	if err != nil {
		return err
	}

	observerURL, err := p.resolveObserverURL(ctx, params)
	if err != nil {
		return err
	}

	credential, err := config.GetCurrentCredential()
	if err != nil {
		return fmt.Errorf("failed to get credentials: %w", err)
	}
	if credential == nil {
		return fmt.Errorf("no current credential available")
	}

	api, err := newObserverAPIClient(observerURL, credential.Token)
	if err != nil {
		return fmt.Errorf("failed to create observer client: %w", err)
	}

	endTime := time.Now()
	startTime := endTime.Add(-window)

	if params.Follow {
		return p.followLogs(ctx, api, params, levels, startTime, endTime)
	}
	return p.fetchAndPrintLogs(ctx, api, params, levels, startTime, endTime)
}

// fetchAndPrintLogs prints the most recent entries in the window. The query runs
// newest-first so that --tail keeps the newest N, then the page is flipped for
// chronological display.
func (p *PlatformLogs) fetchAndPrintLogs(ctx context.Context, api observerAPI, params LogsParams,
	levels []string, startTime, endTime time.Time,
) error {
	logs, err := p.fetchLogs(ctx, api, params, levels, startTime, endTime,
		tailLimit(params.Tail), obsgen.GetPlatformLogsParamsSortOrderDesc)
	if err != nil {
		return err
	}
	slices.Reverse(logs)

	if err := printLogs(logs, params.Output); err != nil {
		return err
	}
	// Reported on stderr so that an empty result does not disturb a JSON pipeline.
	if len(logs) == 0 {
		fmt.Fprintln(os.Stderr, "No platform logs matched the query.")
	}
	return nil
}

// followLogs prints the initial page and then polls for newer entries until interrupted.
func (p *PlatformLogs) followLogs(ctx context.Context, api observerAPI, params LogsParams,
	levels []string, startTime, endTime time.Time,
) error {
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	logs, err := p.fetchLogs(ctx, api, params, levels, startTime, endTime,
		tailLimit(params.Tail), obsgen.GetPlatformLogsParamsSortOrderDesc)
	if err != nil {
		return err
	}
	slices.Reverse(logs)
	if err := printLogs(logs, params.Output); err != nil {
		return err
	}
	startTime = nextStart(logs, endTime)

	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			fmt.Fprintln(os.Stderr, "\nStopping log streaming...")
			return nil
		case <-ticker.C:
			endTime = time.Now()
			if !endTime.After(startTime) {
				continue
			}

			// Polls read ascending so entries print in order and the window can advance
			// past the newest one seen; --tail applies to the initial page only.
			logs, err := p.fetchLogs(ctx, api, params, levels, startTime, endTime,
				pollLimit, obsgen.GetPlatformLogsParamsSortOrderAsc)
			if err != nil {
				if ctx.Err() != nil {
					return nil
				}
				// A failed poll is transient; report it and keep the window where it was
				// so the next poll picks up whatever was missed.
				fmt.Fprintf(os.Stderr, "Error fetching logs: %v\n", err)
				continue
			}
			if err := printLogs(logs, params.Output); err != nil {
				return err
			}
			startTime = nextStart(logs, endTime)
		}
	}
}

// fetchLogs runs one platform logs query against the observer.
func (p *PlatformLogs) fetchLogs(ctx context.Context, api observerAPI, params LogsParams,
	levels []string, startTime, endTime time.Time, limit int,
	sortOrder obsgen.GetPlatformLogsParamsSortOrder,
) ([]obsgen.PlatformLog, error) {
	query := &obsgen.GetPlatformLogsParams{
		StartTime: startTime.UTC(),
		EndTime:   endTime.UTC(),
		Limit:     &limit,
		SortOrder: &sortOrder,
	}
	if len(params.Clusters) > 0 {
		query.ClusterInstance = &params.Clusters
	}
	if len(params.PodNamespaces) > 0 {
		query.Namespace = &params.PodNamespaces
	}
	if len(params.Pods) > 0 {
		query.PodName = &params.Pods
	}
	if len(params.Containers) > 0 {
		query.ContainerName = &params.Containers
	}
	if params.Selector != "" {
		query.Labels = &params.Selector
	}
	if len(levels) > 0 {
		query.LogLevels = &levels
	}
	if params.Search != "" {
		query.SearchPhrase = &params.Search
	}

	resp, err := api.GetPlatformLogsWithResponse(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("observer query failed for observability plane %s: %w", params.PlaneName, err)
	}
	if resp.JSON200 == nil {
		return nil, observerError(resp, params.PlaneName)
	}
	return resp.JSON200.Logs, nil
}

// resolveObserverURL reads the observer URL off the named observability plane. Platform
// logs have no environment to traverse from, so the plane is addressed directly.
func (p *PlatformLogs) resolveObserverURL(ctx context.Context, params LogsParams) (string, error) {
	if params.PlaneKind == NamespacedPlane {
		op, err := p.client.GetObservabilityPlane(ctx, params.Namespace, params.PlaneName)
		if err != nil {
			return "", fmt.Errorf("failed to get observability plane %s: %w", params.PlaneName, err)
		}
		if op.Spec == nil || op.Spec.ObserverURL == nil || *op.Spec.ObserverURL == "" {
			return "", fmt.Errorf("observer URL not configured in observability plane %s", params.PlaneName)
		}
		return *op.Spec.ObserverURL, nil
	}

	cop, err := p.client.GetClusterObservabilityPlane(ctx, params.PlaneName)
	if err != nil {
		return "", fmt.Errorf("failed to get cluster observability plane %s: %w", params.PlaneName, err)
	}
	if cop.Spec == nil || cop.Spec.ObserverURL == nil || *cop.Spec.ObserverURL == "" {
		return "", fmt.Errorf("observer URL not configured in cluster observability plane %s", params.PlaneName)
	}
	return *cop.Spec.ObserverURL, nil
}

// newObserverAPIClient builds an observer client that carries the current credential and
// refreshes it when it has expired, matching how the control plane client is built.
func newObserverAPIClient(observerURL, token string) (*obsgen.ClientWithResponses, error) {
	httpClient := &http.Client{Timeout: 30 * time.Second}

	return obsgen.NewClientWithResponses(
		observerURL,
		obsgen.WithHTTPClient(httpClient),
		obsgen.WithRequestEditorFn(func(ctx context.Context, req *http.Request) error {
			currentToken := token
			if currentToken != "" && auth.IsTokenExpired(currentToken) {
				newToken, err := auth.RefreshToken()
				if err != nil {
					return fmt.Errorf("failed to refresh token: %w", err)
				}
				currentToken = newToken
				token = newToken
			}
			if currentToken != "" {
				req.Header.Set("Authorization", "Bearer "+currentToken)
			}
			return nil
		}),
	)
}

// observerError turns a non-200 platform logs response into a message that says what to
// do about it. A 501 and a 403 are ordinary outcomes here rather than faults: an adapter
// need not implement platform logs, and the permission is deliberately narrow.
func observerError(resp *obsgen.GetPlatformLogsResp, planeName string) error {
	status := 0
	if resp.HTTPResponse != nil {
		status = resp.HTTPResponse.StatusCode
	}

	switch status {
	case http.StatusNotImplemented:
		return fmt.Errorf("observability plane %s does not serve platform logs: its logs adapter has not implemented them", planeName)
	case http.StatusForbidden:
		return fmt.Errorf("not authorized to read platform logs from observability plane %s: this needs the cluster-scoped %s permission, held by the platform-engineer and admin roles",
			planeName, viewAction)
	}

	if msg := observerMessage(resp.Body); msg != "" {
		return fmt.Errorf("observer query failed (HTTP %d): %s", status, msg)
	}
	if len(resp.Body) > 0 {
		return fmt.Errorf("observer query failed (HTTP %d): %s", status, string(resp.Body))
	}
	return fmt.Errorf("observer query failed (HTTP %d)", status)
}

// observerMessage extracts the human-readable message from an observer error body.
func observerMessage(body []byte) string {
	var errResp obsgen.ErrorResponse
	if err := json.Unmarshal(body, &errResp); err == nil && errResp.Message != nil {
		return *errResp.Message
	}
	return ""
}

// printLogs writes every entry in the chosen format.
func printLogs(logs []obsgen.PlatformLog, output string) error {
	for _, log := range logs {
		if err := printLog(log, output); err != nil {
			return err
		}
	}
	return nil
}

// printLog writes one entry. JSON output is one object per line so that --follow streams
// the same shape a one-shot query prints, and so jq reads it without buffering.
func printLog(log obsgen.PlatformLog, output string) error {
	if output == outputJSON {
		encoded, err := json.Marshal(log)
		if err != nil {
			return fmt.Errorf("failed to encode log entry: %w", err)
		}
		fmt.Println(string(encoded))
		return nil
	}
	fmt.Println(formatLogLine(log))
	return nil
}

// formatLogLine renders an entry as "<timestamp> <LEVEL> [<pod>/<container>] <message>",
// dropping whatever the record does not carry.
func formatLogLine(log obsgen.PlatformLog) string {
	var b strings.Builder
	b.WriteString(log.Timestamp.UTC().Format(time.RFC3339))
	if level := deref(log.Level); level != "" {
		b.WriteString(" " + level)
	}
	if source := logSource(log); source != "" {
		b.WriteString(" [" + source + "]")
	}
	b.WriteString(" " + log.Log)
	return b.String()
}

// logSource names the pod and container that produced an entry, as far as it is known.
func logSource(log obsgen.PlatformLog) string {
	pod, container := deref(log.PodName), deref(log.ContainerName)
	switch {
	case pod != "" && container != "":
		return pod + "/" + container
	case pod != "":
		return pod
	default:
		return container
	}
}

// nextStart advances the window past the newest entry already printed so that the next
// poll cannot repeat it. Entries are chronological by this point, so the last is newest.
func nextStart(logs []obsgen.PlatformLog, fallback time.Time) time.Time {
	if len(logs) == 0 {
		return fallback
	}
	return logs[len(logs)-1].Timestamp.Add(time.Millisecond)
}

// tailLimit resolves --tail into a page size, an unset flag meaning the default page.
func tailLimit(tail int) int {
	if tail <= 0 {
		return defaultLimit
	}
	return tail
}

// validateOutput rejects an unknown --output rather than silently printing text.
func validateOutput(output string) error {
	if output != outputText && output != outputJSON {
		return fmt.Errorf("invalid --output %q: must be %q or %q", output, outputText, outputJSON)
	}
	return nil
}

// normalizeLevels upper-cases --level values and rejects anything outside the enum.
func normalizeLevels(levels []string) ([]string, error) {
	if len(levels) == 0 {
		return nil, nil
	}
	normalized := make([]string, 0, len(levels))
	for _, level := range levels {
		upper := strings.ToUpper(strings.TrimSpace(level))
		if !slices.Contains(validLevels, upper) {
			return nil, fmt.Errorf("invalid --level %q: must be one of %s", level, strings.Join(validLevels, ", "))
		}
		normalized = append(normalized, upper)
	}
	return normalized, nil
}

// parseSince converts the relative --since into a window width, defaulting it when unset.
func parseSince(since string) (time.Duration, error) {
	if since == "" {
		since = defaultSince
	}
	duration, err := time.ParseDuration(since)
	if err != nil {
		return 0, fmt.Errorf("invalid --since value %q: %w", since, err)
	}
	if duration <= 0 {
		return 0, fmt.Errorf("invalid --since value %q: duration must be positive", since)
	}
	if duration > maxWindow {
		return 0, fmt.Errorf("invalid --since value %q: the observer accepts a window of at most 30 days", since)
	}
	return duration, nil
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
