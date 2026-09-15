// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package platformlogs

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	obsgen "github.com/openchoreo/openchoreo/internal/observer/api/gen"
	"github.com/openchoreo/openchoreo/internal/occ/cmd/config"
	"github.com/openchoreo/openchoreo/internal/occ/resources/client/mocks"
	"github.com/openchoreo/openchoreo/internal/occ/testutil"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
)

const observerTestURL = "http://observer.test"

func setupLogsConfig(t *testing.T) {
	t.Helper()
	home := testutil.SetupTestHome(t)
	testutil.WriteOCConfig(t, home, config.StoredConfig{
		CurrentContext: "test",
		ControlPlanes:  []config.ControlPlane{{Name: "cp", URL: "http://mock-api"}},
		Credentials:    []config.Credential{{Name: "cred", Token: testutil.NonExpiredJWT}},
		Contexts:       []config.Context{{Name: "test", ControlPlane: "cp", Credentials: "cred"}},
	})
}

// mockClusterPlane wires the ClusterObservabilityPlane lookup that resolves the observer URL.
func mockClusterPlane(t *testing.T) *mocks.MockInterface {
	t.Helper()
	observerURL := observerTestURL
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetClusterObservabilityPlane(mock.Anything, "default").Return(
		&gen.ClusterObservabilityPlane{
			Spec: &gen.ClusterObservabilityPlaneSpec{ObserverURL: &observerURL},
		}, nil)
	return mc
}

func platformLogsResp(logs ...obsgen.PlatformLog) *http.Response {
	return testutil.JSONResp(http.StatusOK, obsgen.PlatformLogsResponse{
		Logs:  logs,
		Total: int64(len(logs)),
	})
}

func strPtr(s string) *string { return &s }

func mustTime(t *testing.T, value string) time.Time {
	t.Helper()
	parsed, err := time.Parse(time.RFC3339, value)
	require.NoError(t, err)
	return parsed
}

// --- query construction ---

func TestLogs_SendsFiltersAndPrintsEntries(t *testing.T) {
	setupLogsConfig(t)
	mc := mockClusterPlane(t)

	var got url.Values
	testutil.SetTransport(t, testutil.RoundTripFunc(func(r *http.Request) (*http.Response, error) {
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, "/api/v1alpha1/platform-logs", r.URL.Path)
		assert.Equal(t, "Bearer "+testutil.NonExpiredJWT, r.Header.Get("Authorization"))
		got = r.URL.Query()
		return platformLogsResp(
			obsgen.PlatformLog{
				Timestamp:     mustTime(t, "2026-01-01T00:01:00Z"),
				Log:           "reconcile failed",
				Level:         strPtr("ERROR"),
				PodName:       strPtr("controller-manager-abc"),
				ContainerName: strPtr("manager"),
			},
			obsgen.PlatformLog{
				Timestamp: mustTime(t, "2026-01-01T00:00:00Z"),
				Log:       "starting manager",
			},
		), nil
	}))

	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, New(mc).Logs(LogsParams{
			PlaneKind:     ClusterPlane,
			PlaneName:     "default",
			Clusters:      []string{"clusterX", "clusterY"},
			PodNamespaces: []string{"openchoreo-control-plane"},
			Pods:          []string{"controller-manager-abc"},
			Containers:    []string{"manager"},
			Selector:      "openchoreo.dev/plane=controlplane",
			Levels:        []string{"error", "warn"},
			Search:        "reconcile",
			Since:         "10m",
			Tail:          25,
			Output:        outputText,
		}))
	})

	assert.Equal(t, "clusterX,clusterY", got.Get("clusterInstance"))
	assert.Equal(t, "openchoreo-control-plane", got.Get("namespace"))
	assert.Equal(t, "controller-manager-abc", got.Get("podName"))
	assert.Equal(t, "manager", got.Get("containerName"))
	assert.Equal(t, "openchoreo.dev/plane=controlplane", got.Get("labels"))
	assert.Equal(t, "ERROR,WARN", got.Get("logLevels"))
	assert.Equal(t, "reconcile", got.Get("searchPhrase"))
	assert.Equal(t, "25", got.Get("limit"))
	// A one-shot query reads newest-first so that --tail keeps the newest entries.
	assert.Equal(t, "desc", got.Get("sortOrder"))

	// The page is flipped back into chronological order for display.
	assert.Regexp(t, `(?s)starting manager.*reconcile failed`, out)
	assert.Contains(t, out, "2026-01-01T00:01:00Z ERROR [controller-manager-abc/manager] reconcile failed")
}

func TestLogs_OmitsUnsetFiltersAndDefaultsTheWindow(t *testing.T) {
	setupLogsConfig(t)
	mc := mockClusterPlane(t)

	var got url.Values
	testutil.SetTransport(t, testutil.RoundTripFunc(func(r *http.Request) (*http.Response, error) {
		got = r.URL.Query()
		return platformLogsResp(), nil
	}))

	_ = testutil.CaptureStdout(t, func() {
		require.NoError(t, New(mc).Logs(LogsParams{
			PlaneKind: ClusterPlane, PlaneName: "default", Output: outputText,
		}))
	})

	for _, absent := range []string{"clusterInstance", "namespace", "podName", "containerName", "labels", "logLevels", "searchPhrase"} {
		assert.False(t, got.Has(absent), "expected %s to be omitted", absent)
	}
	assert.Equal(t, "100", got.Get("limit"))

	start := mustTime(t, got.Get("startTime"))
	end := mustTime(t, got.Get("endTime"))
	assert.InDelta(t, time.Hour.Seconds(), end.Sub(start).Seconds(), 5)
}

// TestLogs_MultiKeySelectorIsSentIntact guards the distinction between --selector and the
// coordinate filters: commas separate values there but mean AND here, so the selector must
// travel as one opaque string rather than being split.
func TestLogs_MultiKeySelectorIsSentIntact(t *testing.T) {
	setupLogsConfig(t)
	mc := mockClusterPlane(t)

	const selector = "openchoreo.dev/plane=dataplane,openchoreo.dev/plane-id=eu-1"

	var got url.Values
	testutil.SetTransport(t, testutil.RoundTripFunc(func(r *http.Request) (*http.Response, error) {
		got = r.URL.Query()
		return platformLogsResp(), nil
	}))

	_ = testutil.CaptureStdout(t, func() {
		require.NoError(t, New(mc).Logs(LogsParams{
			PlaneKind: ClusterPlane, PlaneName: "default", Selector: selector, Output: outputText,
		}))
	})

	assert.Equal(t, []string{selector}, got["labels"])
}

// --- output ---

func TestLogs_JSONOutputEmitsOneRecordPerLine(t *testing.T) {
	setupLogsConfig(t)
	mc := mockClusterPlane(t)

	testutil.SetTransport(t, testutil.RoundTripFunc(func(r *http.Request) (*http.Response, error) {
		return platformLogsResp(obsgen.PlatformLog{
			Timestamp:       mustTime(t, "2026-01-01T00:00:00Z"),
			Log:             "starting manager",
			ClusterInstance: strPtr("clusterX"),
			NodeName:        strPtr("node-1"),
			Labels:          &map[string]string{"openchoreo.dev/plane": "controlplane"},
		}), nil
	}))

	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, New(mc).Logs(LogsParams{
			PlaneKind: ClusterPlane, PlaneName: "default", Output: outputJSON,
		}))
	})

	assert.Contains(t, out, `"clusterInstance":"clusterX"`)
	assert.Contains(t, out, `"nodeName":"node-1"`)
	assert.Contains(t, out, `"openchoreo.dev/plane":"controlplane"`)
	assert.Equal(t, 1, len(splitNonEmptyLines(out)))
}

func TestFormatLogLine(t *testing.T) {
	tests := []struct {
		name string
		log  obsgen.PlatformLog
		want string
	}{
		{
			name: "full",
			log: obsgen.PlatformLog{
				Timestamp: mustTime(t, "2026-01-01T00:00:00Z"), Log: "hello",
				Level: strPtr("INFO"), PodName: strPtr("pod-1"), ContainerName: strPtr("manager"),
			},
			want: "2026-01-01T00:00:00Z INFO [pod-1/manager] hello",
		},
		{
			name: "no level",
			log: obsgen.PlatformLog{
				Timestamp: mustTime(t, "2026-01-01T00:00:00Z"), Log: "hello", PodName: strPtr("pod-1"),
			},
			want: "2026-01-01T00:00:00Z [pod-1] hello",
		},
		{
			name: "container only",
			log: obsgen.PlatformLog{
				Timestamp: mustTime(t, "2026-01-01T00:00:00Z"), Log: "hello", ContainerName: strPtr("manager"),
			},
			want: "2026-01-01T00:00:00Z [manager] hello",
		},
		{
			name: "bare",
			log:  obsgen.PlatformLog{Timestamp: mustTime(t, "2026-01-01T00:00:00Z"), Log: "hello"},
			want: "2026-01-01T00:00:00Z hello",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, formatLogLine(tt.log))
		})
	}
}

// --- validation ---

func TestParseSince(t *testing.T) {
	tests := []struct {
		name    string
		since   string
		want    time.Duration
		wantErr string
	}{
		{name: "default when unset", since: "", want: time.Hour},
		{name: "relative", since: "10m", want: 10 * time.Minute},
		{name: "unparseable", since: "10 minutes", wantErr: `invalid --since value "10 minutes"`},
		{name: "zero", since: "0s", wantErr: "duration must be positive"},
		{name: "negative", since: "-5m", wantErr: "duration must be positive"},
		{name: "beyond the observer window", since: "745h", wantErr: "at most 30 days"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseSince(tt.since)
			if tt.wantErr != "" {
				assert.ErrorContains(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestNormalizeLevels(t *testing.T) {
	got, err := normalizeLevels([]string{"error", " warn ", "INFO"})
	require.NoError(t, err)
	assert.Equal(t, []string{"ERROR", "WARN", "INFO"}, got)

	_, err = normalizeLevels([]string{"TRACE"})
	assert.ErrorContains(t, err, `invalid --level "TRACE"`)

	got, err = normalizeLevels(nil)
	require.NoError(t, err)
	assert.Nil(t, got)
}

func TestValidateOutput(t *testing.T) {
	require.NoError(t, validateOutput(outputText))
	require.NoError(t, validateOutput(outputJSON))
	assert.ErrorContains(t, validateOutput("yaml"), `invalid --output "yaml"`)
}

func TestLogs_RejectsBadFlagsBeforeCallingTheAPI(t *testing.T) {
	// No mock expectations and no transport: a rejected flag must not reach the network.
	tests := []struct {
		name    string
		params  LogsParams
		wantErr string
	}{
		{name: "output", params: LogsParams{Output: "yaml"}, wantErr: "invalid --output"},
		{name: "level", params: LogsParams{Output: outputText, Levels: []string{"TRACE"}}, wantErr: "invalid --level"},
		{name: "since", params: LogsParams{Output: outputText, Since: "nope"}, wantErr: "invalid --since"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := New(mocks.NewMockInterface(t)).Logs(tt.params)
			assert.ErrorContains(t, err, tt.wantErr)
		})
	}
}

// --- observer URL resolution ---

func TestLogs_NamespacedPlaneResolvesItsObserverURL(t *testing.T) {
	setupLogsConfig(t)
	observerURL := observerTestURL
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetObservabilityPlane(mock.Anything, "acme-corp", "primary").Return(
		&gen.ObservabilityPlane{Spec: &gen.ObservabilityPlaneSpec{ObserverURL: &observerURL}}, nil)

	testutil.SetTransport(t, testutil.RoundTripFunc(func(r *http.Request) (*http.Response, error) {
		assert.Equal(t, "observer.test", r.URL.Host)
		return platformLogsResp(), nil
	}))

	_ = testutil.CaptureStdout(t, func() {
		require.NoError(t, New(mc).Logs(LogsParams{
			PlaneKind: NamespacedPlane, PlaneName: "primary", Namespace: "acme-corp", Output: outputText,
		}))
	})
}

func TestLogs_MissingObserverURL(t *testing.T) {
	setupLogsConfig(t)
	empty := ""
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetClusterObservabilityPlane(mock.Anything, "default").Return(
		&gen.ClusterObservabilityPlane{
			Spec: &gen.ClusterObservabilityPlaneSpec{ObserverURL: &empty},
		}, nil)

	err := New(mc).Logs(LogsParams{PlaneKind: ClusterPlane, PlaneName: "default", Output: outputText})
	assert.ErrorContains(t, err, "observer URL not configured in cluster observability plane default")
}

// --- error mapping ---

func TestLogs_ObserverErrors(t *testing.T) {
	tests := []struct {
		name    string
		status  int
		body    any
		wantErr string
	}{
		{
			name:   "adapter cannot serve platform logs",
			status: http.StatusNotImplemented,
			body: obsgen.ErrorResponse{
				Message: strPtr("platform logs are not supported by this adapter"),
			},
			wantErr: "observability plane default does not serve platform logs",
		},
		{
			name:    "permission is cluster scoped",
			status:  http.StatusForbidden,
			body:    obsgen.ErrorResponse{Message: strPtr("forbidden")},
			wantErr: "cluster-scoped platformlogs:view permission",
		},
		{
			name:    "bad selector is reported verbatim",
			status:  http.StatusBadRequest,
			body:    obsgen.ErrorResponse{Message: strPtr("set-based selectors are not supported")},
			wantErr: "observer query failed (HTTP 400): set-based selectors are not supported",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setupLogsConfig(t)
			mc := mockClusterPlane(t)
			testutil.SetTransport(t, testutil.RoundTripFunc(func(r *http.Request) (*http.Response, error) {
				return testutil.JSONResp(tt.status, tt.body), nil
			}))

			err := New(mc).Logs(LogsParams{
				PlaneKind: ClusterPlane, PlaneName: "default", Output: outputText,
			})
			assert.ErrorContains(t, err, tt.wantErr)
		})
	}
}

// --- follow bookkeeping ---

// rec builds a record whose identity is its timestamp plus message.
func rec(t *testing.T, ts, log string) obsgen.PlatformLog {
	t.Helper()
	return obsgen.PlatformLog{
		Timestamp: mustTime(t, ts),
		Log:       log,
		PodName:   strPtr("pod-1"),
	}
}

func TestBoundary_HoldsTheInstantRatherThanSteppingOverIt(t *testing.T) {
	start := mustTime(t, "2026-01-01T00:00:00Z")
	b := newBoundary(start)

	// Microsecond resolution: advancing by a fixed millisecond would blind the cursor to
	// everything between .423511 and .424511.
	batch := []obsgen.PlatformLog{
		rec(t, "2026-01-01T00:00:01.423511Z", "first"),
	}
	b.advance(batch)

	now := mustTime(t, "2026-01-01T00:00:05Z")
	assert.Equal(t, mustTime(t, "2026-01-01T00:00:01.423511Z"), b.start(now),
		"the next poll must re-query the boundary instant, not skip past it")
}

func TestBoundary_FiltersOnlyWhatWasPrintedAtTheBoundary(t *testing.T) {
	b := newBoundary(mustTime(t, "2026-01-01T00:00:00Z"))
	b.advance([]obsgen.PlatformLog{
		rec(t, "2026-01-01T00:00:01.100000Z", "old"),
		rec(t, "2026-01-01T00:00:01.500000Z", "boundary-a"),
		rec(t, "2026-01-01T00:00:01.500000Z", "boundary-b"),
	})

	// The re-query returns the boundary instant again, plus records that fall inside the
	// sub-millisecond gap a fixed increment would have skipped, plus something newer.
	got := b.filterNew([]obsgen.PlatformLog{
		rec(t, "2026-01-01T00:00:01.500000Z", "boundary-a"),
		rec(t, "2026-01-01T00:00:01.500000Z", "boundary-b"),
		rec(t, "2026-01-01T00:00:01.500001Z", "inside-the-gap"),
		rec(t, "2026-01-01T00:00:02.000000Z", "newer"),
	})

	msgs := make([]string, 0, len(got))
	for _, l := range got {
		msgs = append(msgs, l.Log)
	}
	assert.Equal(t, []string{"inside-the-gap", "newer"}, msgs)
}

func TestBoundary_RecordsSharingTheBoundaryInstantAreNotLost(t *testing.T) {
	b := newBoundary(mustTime(t, "2026-01-01T00:00:00Z"))
	// A page that ends mid-instant: only one of the two records at .500000 fitted.
	b.advance([]obsgen.PlatformLog{rec(t, "2026-01-01T00:00:01.500000Z", "a")})

	got := b.filterNew([]obsgen.PlatformLog{
		rec(t, "2026-01-01T00:00:01.500000Z", "a"),
		rec(t, "2026-01-01T00:00:01.500000Z", "b"),
	})
	require.Len(t, got, 1)
	assert.Equal(t, "b", got[0].Log, "the tie that did not fit in the page must still arrive")
}

func TestBoundary_EmptyPollHoldsThePosition(t *testing.T) {
	b := newBoundary(mustTime(t, "2026-01-01T00:00:00Z"))
	b.advance([]obsgen.PlatformLog{rec(t, "2026-01-01T00:00:01Z", "first")})
	now := mustTime(t, "2026-01-01T00:00:30Z")
	before := b.start(now)

	// Records are indexed after the event they describe, so an empty poll must not move
	// the cursor to "now" - that would skip whatever is still arriving behind it.
	b.advance(nil)
	assert.Equal(t, before, b.start(now))
}

func TestBoundary_OutOfOrderBatchDoesNotRewind(t *testing.T) {
	b := newBoundary(mustTime(t, "2026-01-01T00:00:00Z"))
	b.advance([]obsgen.PlatformLog{rec(t, "2026-01-01T00:00:05Z", "newest")})
	b.advance([]obsgen.PlatformLog{rec(t, "2026-01-01T00:00:02Z", "older")})

	assert.Equal(t, mustTime(t, "2026-01-01T00:00:05Z"), b.start(mustTime(t, "2026-01-01T00:01:00Z")))
}

func TestBoundary_ClampsAnIdleSessionToTheObserverWindow(t *testing.T) {
	now := time.Now()
	b := newBoundary(now.Add(-100 * 24 * time.Hour))

	// Left following for longer than the observer accepts, the lower bound would otherwise
	// drift into a query the server rejects.
	assert.WithinDuration(t, now.Add(-maxWindow), b.start(now), time.Second)
}

func TestTailLimit(t *testing.T) {
	assert.Equal(t, defaultLimit, tailLimit(0))
	assert.Equal(t, defaultLimit, tailLimit(-1))
	assert.Equal(t, 25, tailLimit(25))
}

func splitNonEmptyLines(s string) []string {
	var lines []string
	for _, line := range strings.Split(s, "\n") {
		if strings.TrimSpace(line) != "" {
			lines = append(lines, line)
		}
	}
	return lines
}

// --- follow loop behavior ---

// stubAPI drives followLogs one poll at a time.
type stubAPI struct {
	calls  int
	params []*obsgen.GetPlatformLogsParams
	fn     func(call int) (*obsgen.GetPlatformLogsResp, error)
}

func (s *stubAPI) GetPlatformLogsWithResponse(_ context.Context, params *obsgen.GetPlatformLogsParams,
	_ ...obsgen.RequestEditorFn,
) (*obsgen.GetPlatformLogsResp, error) {
	s.calls++
	s.params = append(s.params, params)
	return s.fn(s.calls)
}

func okResp(logs ...obsgen.PlatformLog) *obsgen.GetPlatformLogsResp {
	return &obsgen.GetPlatformLogsResp{
		HTTPResponse: &http.Response{StatusCode: http.StatusOK},
		JSON200:      &obsgen.PlatformLogsResponse{Logs: logs, Total: int64(len(logs))},
	}
}

func statusResp(status int, message string) *obsgen.GetPlatformLogsResp {
	body, _ := json.Marshal(obsgen.ErrorResponse{Message: &message})
	return &obsgen.GetPlatformLogsResp{
		HTTPResponse: &http.Response{StatusCode: status},
		Body:         body,
	}
}

// fastPolls shortens the follow interval so a test drives the loop in milliseconds.
func fastPolls(t *testing.T) {
	t.Helper()
	original := pollInterval
	pollInterval = 2 * time.Millisecond
	t.Cleanup(func() { pollInterval = original })
}

func runFollow(t *testing.T, api observerAPI, params LogsParams) (string, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var err error
	now := time.Now()
	out := testutil.CaptureStdout(t, func() {
		err = New(nil).followLogs(ctx, api, params, nil, now.Add(-time.Hour), now)
	})
	return out, err
}

func TestFollowLogs_RetriesRecoverableErrors(t *testing.T) {
	fastPolls(t)
	now := time.Now().UTC()
	api := &stubAPI{fn: func(call int) (*obsgen.GetPlatformLogsResp, error) {
		switch call {
		case 1:
			return okResp(obsgen.PlatformLog{Timestamp: now.Add(-time.Minute), Log: "first"}), nil
		case 2:
			return statusResp(http.StatusServiceUnavailable, "try later"), nil
		case 3:
			return okResp(obsgen.PlatformLog{Timestamp: now.Add(-30 * time.Second), Log: "after recovery"}), nil
		default:
			// Keep failing so the session ends by exhausting its retry budget.
			return statusResp(http.StatusServiceUnavailable, "try later"), nil
		}
	}}

	out, err := runFollow(t, api, LogsParams{PlaneName: "default", Output: outputText})

	assert.Contains(t, out, "after recovery", "a single 503 must not end the session")
	assert.ErrorContains(t, err, "giving up after")
}

func TestFollowLogs_GivesUpAfterRepeatedRecoverableFailures(t *testing.T) {
	fastPolls(t)
	api := &stubAPI{fn: func(call int) (*obsgen.GetPlatformLogsResp, error) {
		if call == 1 {
			return okResp(), nil
		}
		// A refresh token that has stopped working looks recoverable forever.
		return nil, errors.New("failed to refresh token: invalid_grant")
	}}

	_, err := runFollow(t, api, LogsParams{PlaneName: "default", Output: outputText})

	assert.ErrorContains(t, err, "giving up after")
	assert.ErrorContains(t, err, "invalid_grant")
	assert.Equal(t, maxPollFailures+1, api.calls, "retries must be bounded")
}

func TestFollowLogs_ReQueriesTheBoundaryInsteadOfSteppingOverIt(t *testing.T) {
	fastPolls(t)

	// Realistic follow timestamps: within the window, microseconds apart. A fixed
	// millisecond advance would put the next lower bound past inGap and lose it.
	boundary := time.Now().Add(-time.Minute).UTC().Truncate(time.Microsecond)
	inGap := boundary.Add(389 * time.Microsecond)

	at := func(ts time.Time, msg string) obsgen.PlatformLog {
		return obsgen.PlatformLog{Timestamp: ts, Log: msg, PodName: strPtr("pod-1")}
	}

	api := &stubAPI{fn: func(call int) (*obsgen.GetPlatformLogsResp, error) {
		switch call {
		case 1:
			return okResp(at(boundary, "first")), nil
		case 2:
			// The re-query returns the boundary record again plus one inside the gap.
			return okResp(at(boundary, "first"), at(inGap, "would-have-been-skipped")), nil
		default:
			return statusResp(http.StatusForbidden, "stop"), nil
		}
	}}

	out, _ := runFollow(t, api, LogsParams{PlaneName: "default", Output: outputText})

	require.GreaterOrEqual(t, len(api.params), 2)
	assert.True(t, api.params[1].StartTime.Equal(boundary),
		"the second poll must start at the boundary instant itself, got %s want %s",
		api.params[1].StartTime, boundary)
	assert.Contains(t, out, "would-have-been-skipped")
	assert.Equal(t, 1, strings.Count(out, "first"), "the boundary record must not print twice")
}
