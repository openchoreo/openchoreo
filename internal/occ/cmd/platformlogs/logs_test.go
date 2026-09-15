// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package platformlogs

import (
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

func TestNextStart(t *testing.T) {
	fallback := mustTime(t, "2026-01-01T00:05:00Z")

	assert.Equal(t, fallback, nextStart(nil, fallback),
		"an empty poll must leave the window where it was")

	logs := []obsgen.PlatformLog{
		{Timestamp: mustTime(t, "2026-01-01T00:00:00Z")},
		{Timestamp: mustTime(t, "2026-01-01T00:01:00Z")},
	}
	assert.Equal(t, mustTime(t, "2026-01-01T00:01:00Z").Add(time.Millisecond), nextStart(logs, fallback),
		"the window must advance past the newest entry already printed")
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
