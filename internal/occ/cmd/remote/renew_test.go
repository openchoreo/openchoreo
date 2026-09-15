// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package remote

import (
	"context"
	"errors"
	"net"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/openchoreo/openchoreo/internal/remoteconnect"
)

// initialCapability is what unitFixture's unit starts with; renewals replace it.
const initialCapability = "capability-1"

// scriptedResolver answers each Resolve from a queue, so a test can drive a sequence of
// renewals (and failures) deterministically.
type scriptedResolver struct {
	mu    sync.Mutex
	steps []resolveStep
	calls []remoteconnect.ResolveRequest
	done  chan struct{}
}

type resolveStep struct {
	resp *remoteconnect.ResolveResponse
	err  error
}

func (r *scriptedResolver) Resolve(_ context.Context, req remoteconnect.ResolveRequest) (*remoteconnect.ResolveResponse, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, req)
	if len(r.steps) == 0 {
		if r.done != nil {
			select {
			case r.done <- struct{}{}:
			default:
			}
		}
		return nil, errors.New("no more scripted responses")
	}
	step := r.steps[0]
	r.steps = r.steps[1:]
	if len(r.steps) == 0 && r.done != nil {
		select {
		case r.done <- struct{}{}:
		default:
		}
	}
	return step.resp, step.err
}

func (r *scriptedResolver) requests() []remoteconnect.ResolveRequest {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]remoteconnect.ResolveRequest(nil), r.calls...)
}

// unitFixture builds a unit with one target served by one agent, plus a recorder of
// every subsequent dial.
func unitFixture(t *testing.T, out *syncBuf) (*remoteUnit, *[]string) {
	t.Helper()
	echo := startLocalEcho(t)

	var dialedMu sync.Mutex
	dialed := []string{}
	dial := func(_ context.Context, agent remoteconnect.AgentEndpoint, _ func() string) (tunnel, error) {
		dialedMu.Lock()
		dialed = append(dialed, agent.ServerName)
		dialedMu.Unlock()
		return &fakeTunnel{addr: echo}, nil
	}

	resp := &remoteconnect.ResolveResponse{
		Capability: initialCapability,
		Targets: []remoteconnect.ResolvedTarget{
			{Key: "res/postgres/primary", Proto: "tcp", AgentID: "dp-a"},
		},
		Agents:            map[string]remoteconnect.AgentEndpoint{"dp-a": {ServerName: "dp-a.remote-connect"}},
		RenewAfterSeconds: 1200,
	}
	u := newRemoteUnit(remoteconnect.ResolveRequest{Project: "doclet", Component: "doc"}, resp, out, dial)
	u.minInterval = time.Millisecond // the floor is production pacing, not behavior
	u.installTunnel("dp-a", &fakeTunnel{addr: echo})
	return u, &dialed
}

func startLocalEcho(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		for {
			c, aerr := ln.Accept()
			if aerr != nil {
				return
			}
			_ = c.Close()
		}
	}()
	return ln.Addr().String()
}

// The next connection is authorized by the fresh capability while the tunnel underneath
// is untouched.
func TestAdoptSwapsCapabilityForSubsequentStreams(t *testing.T) {
	out := &syncBuf{}
	u, dialed := unitFixture(t, out)

	if got := u.cap(); got != initialCapability {
		t.Fatalf("initial capability = %q", got)
	}
	u.adopt(context.Background(), &remoteconnect.ResolveResponse{
		Capability: "capability-2",
		Targets: []remoteconnect.ResolvedTarget{
			{Key: "res/postgres/primary", Proto: "tcp", AgentID: "dp-a"},
		},
		Agents:            map[string]remoteconnect.AgentEndpoint{"dp-a": {ServerName: "dp-a.remote-connect"}},
		RenewAfterSeconds: 900,
	})

	if got := u.cap(); got != "capability-2" {
		t.Errorf("capability after renewal = %q, want capability-2", got)
	}
	if len(*dialed) != 0 {
		t.Errorf("an unchanged agent must not be re-dialed, got %v", *dialed)
	}
	c, err := u.openStream("res/postgres/primary")
	if err != nil {
		t.Fatalf("open stream after renewal: %v", err)
	}
	_ = c.Close()
	if got, want := u.nextInterval(), 900*time.Second; got != want {
		t.Errorf("nextInterval after renewal = %s, want %s", got, want)
	}
}

// A role revoked mid-session drops the target from the next capability, and new
// connections to it must stop.
func TestAdoptRefusesRevokedTarget(t *testing.T) {
	out := &syncBuf{}
	u, _ := unitFixture(t, out)

	u.adopt(context.Background(), &remoteconnect.ResolveResponse{
		Capability: "capability-2",
		Targets:    nil, // no longer authorized
	})

	if _, err := u.openStream("res/postgres/primary"); !errors.Is(err, errRevoked) {
		t.Fatalf("expected errRevoked for a dropped target, got %v", err)
	}
	if got := out.String(); !strings.Contains(got, "res/postgres/primary") ||
		!strings.Contains(got, "no longer authorized") {
		t.Errorf("revocation should be reported with a reason, got %q", got)
	}

	// And it recovers if the grant comes back.
	u.adopt(context.Background(), &remoteconnect.ResolveResponse{
		Capability: "capability-3",
		Targets: []remoteconnect.ResolvedTarget{
			{Key: "res/postgres/primary", Proto: "tcp", AgentID: "dp-a"},
		},
		Agents: map[string]remoteconnect.AgentEndpoint{"dp-a": {ServerName: "dp-a.remote-connect"}},
	})
	c, err := u.openStream("res/postgres/primary")
	if err != nil {
		t.Fatalf("target should be usable again after being re-authorized: %v", err)
	}
	_ = c.Close()
	if got := out.String(); !strings.Contains(got, "authorized again") {
		t.Errorf("restoration should be reported, got %q", got)
	}
}

// A dependency that moved to another provider namespace needs a different tunnel. The
// old one is retired rather than closed, because streams are still running on it.
func TestAdoptDialsNewAgentForMovedTarget(t *testing.T) {
	out := &syncBuf{}
	u, dialed := unitFixture(t, out)

	u.adopt(context.Background(), &remoteconnect.ResolveResponse{
		Capability: "capability-2",
		Targets: []remoteconnect.ResolvedTarget{
			{Key: "res/postgres/primary", Proto: "tcp", AgentID: "dp-b"},
		},
		Agents: map[string]remoteconnect.AgentEndpoint{"dp-b": {ServerName: "dp-b.remote-connect"}},
	})

	if len(*dialed) != 1 || (*dialed)[0] != "dp-b.remote-connect" {
		t.Fatalf("expected a dial to the new agent, got %v", *dialed)
	}
	c, err := u.openStream("res/postgres/primary")
	if err != nil {
		t.Fatalf("open stream on the moved target: %v", err)
	}
	_ = c.Close()
}

// A re-dial that fails leaves the old tunnel in place, but the agent must still count as
// unreached: the next renewal has to try it again rather than treat the move as done.
func TestAdoptRetriesAgentAfterFailedRedial(t *testing.T) {
	out := &syncBuf{}
	echo := startLocalEcho(t)

	var mu sync.Mutex
	var attempts []string
	refuse := true
	dial := func(_ context.Context, agent remoteconnect.AgentEndpoint, _ func() string) (tunnel, error) {
		mu.Lock()
		attempts = append(attempts, agent.ServerName)
		refusing := refuse
		mu.Unlock()
		if refusing {
			return nil, errors.New("connection refused")
		}
		return &fakeTunnel{addr: echo}, nil
	}

	u := newRemoteUnit(
		remoteconnect.ResolveRequest{Project: "doclet", Component: "doc"},
		&remoteconnect.ResolveResponse{
			Capability: initialCapability,
			Targets: []remoteconnect.ResolvedTarget{
				{Key: "res/postgres/primary", Proto: "tcp", AgentID: "dp-a"},
			},
			Agents:            map[string]remoteconnect.AgentEndpoint{"dp-a": {ServerName: "dp-a.remote-connect"}},
			RenewAfterSeconds: 1200,
		}, out, dial)
	u.minInterval = time.Millisecond
	u.installTunnel("dp-a", &fakeTunnel{addr: echo})

	// The same agent reachable at a new endpoint, as after the agent Deployment rolls.
	moved := &remoteconnect.ResolveResponse{
		Capability: "capability-2",
		Targets: []remoteconnect.ResolvedTarget{
			{Key: "res/postgres/primary", Proto: "tcp", AgentID: "dp-a"},
		},
		Agents: map[string]remoteconnect.AgentEndpoint{"dp-a": {ServerName: "dp-a2.remote-connect"}},
	}

	u.adopt(context.Background(), moved)
	mu.Lock()
	refuse = false
	mu.Unlock()
	u.adopt(context.Background(), moved)

	mu.Lock()
	got := append([]string(nil), attempts...)
	mu.Unlock()
	if len(got) != 2 {
		t.Fatalf("the next renewal should re-dial an agent whose move failed, got attempts %v", got)
	}
	c, err := u.openStream("res/postgres/primary")
	if err != nil {
		t.Fatalf("open stream once the re-dial succeeded: %v", err)
	}
	_ = c.Close()
}

// A dependency added upstream cannot be given a local port mid-session, so it is
// reported once.
func TestAdoptWarnsAboutUnboundTarget(t *testing.T) {
	out := &syncBuf{}
	u, _ := unitFixture(t, out)

	next := &remoteconnect.ResolveResponse{
		Capability: "capability-2",
		Targets: []remoteconnect.ResolvedTarget{
			{Key: "res/postgres/primary", Proto: "tcp", AgentID: "dp-a"},
			{Key: "res/redis/primary", Proto: "tcp", AgentID: "dp-a"},
		},
		Agents: map[string]remoteconnect.AgentEndpoint{"dp-a": {ServerName: "dp-a.remote-connect"}},
	}
	u.adopt(context.Background(), next)
	u.adopt(context.Background(), next)

	got := out.String()
	if !strings.Contains(got, "res/redis/primary") || !strings.Contains(got, "new dependency") {
		t.Errorf("a newly appeared dependency should be reported, got %q", got)
	}
	if n := strings.Count(got, "res/redis/primary"); n != 1 {
		t.Errorf("the warning should appear once, not on every renewal (appeared %d times)", n)
	}
}

// A 401 or 403 will not change on retry, so the renewer stops and names the remedy.
func TestRenewStopsOnTerminalRefusal(t *testing.T) {
	out := &syncBuf{}
	u, _ := unitFixture(t, out)
	u.renewAfter = time.Millisecond // renew promptly; the cadence is not under test here

	r := &scriptedResolver{steps: []resolveStep{
		{err: &resolveStatusError{status: http.StatusUnauthorized, body: "token expired"}},
	}}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	stopped := make(chan struct{})
	go func() { u.renew(ctx, r); close(stopped) }()

	select {
	case <-stopped:
	case <-ctx.Done():
		t.Fatal("renewer did not stop on a terminal refusal")
	}
	if calls := r.requests(); len(calls) != 1 {
		t.Errorf("expected exactly one attempt for a terminal refusal, got %d", len(calls))
	}
	if got := out.String(); !strings.Contains(got, "occ login") {
		t.Errorf("a 401 should point at `occ login`, got %q", got)
	}
}

// Values are fetched once at session start, so a renewal asks for no grants, keeping the
// session on the full dial TTL.
func TestRenewRequestsDropGrants(t *testing.T) {
	out := &syncBuf{}
	u, _ := unitFixture(t, out)
	u.renewAfter = time.Millisecond

	r := &scriptedResolver{steps: []resolveStep{
		{err: &resolveStatusError{status: http.StatusForbidden, body: "session bound"}},
	}}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	u.renew(ctx, r)

	calls := r.requests()
	if len(calls) == 0 {
		t.Fatal("no renewal attempt was made")
	}
	if !calls[0].SkipSecrets {
		t.Error("a renewal must set SkipSecrets")
	}
	if calls[0].Purpose != remoteconnect.PurposeRenew {
		t.Errorf("renewal purpose = %q, want %q", calls[0].Purpose, remoteconnect.PurposeRenew)
	}
	if calls[0].PreviousCapability != initialCapability {
		t.Errorf("renewal should present the capability it is replacing, got %q", calls[0].PreviousCapability)
	}
	if got := out.String(); !strings.Contains(got, "session bound") {
		t.Errorf("a 403 should relay the control plane's reason, got %q", got)
	}
}

// Without a server cadence, occ derives one from the capability's remaining life.
func TestNextIntervalFallsBackWithoutServerCadence(t *testing.T) {
	out := &syncBuf{}
	u, _ := unitFixture(t, out)

	u.mu.Lock()
	u.renewAfter = 0
	u.expiry = time.Now().Add(30 * time.Minute)
	u.mu.Unlock()

	got := u.nextInterval()
	if got < 14*time.Minute || got > 15*time.Minute {
		t.Errorf("fallback interval = %s, want about half of the remaining 30m", got)
	}

	// An already-lapsed capability still retries rather than sleeping forever.
	u.mu.Lock()
	u.expiry = time.Now().Add(-time.Minute)
	u.mu.Unlock()
	if got := u.nextInterval(); got != u.minInterval {
		t.Errorf("interval for a lapsed capability = %s, want the %s floor", got, u.minInterval)
	}
}

// A fetch-only workload owns no listener, so its capability is not renewed.
func TestFetchOnlyUnitIsNotRenewed(t *testing.T) {
	out := &syncBuf{}
	dial := func(context.Context, remoteconnect.AgentEndpoint, func() string) (tunnel, error) {
		t.Fatal("a fetch-only unit should dial nothing here")
		return nil, nil
	}
	fetchOnly := newRemoteUnit(remoteconnect.ResolveRequest{Project: "doclet", Component: "doc"},
		&remoteconnect.ResolveResponse{Capability: initialCapability}, out, dial)
	if fetchOnly.needsRenewal() {
		t.Error("a unit with no dial targets should not be renewed")
	}

	withTargets, _ := unitFixture(t, out)
	if !withTargets.needsRenewal() {
		t.Error("a unit with dial targets must be renewed")
	}
}

// Adopting an empty capability would leave every subsequent stream presenting nothing,
// so the current one is kept and the renewal retried.
func TestRenewRejectsEmptyCapability(t *testing.T) {
	out := &syncBuf{}
	u, _ := unitFixture(t, out)
	u.renewAfter = time.Millisecond

	r := &scriptedResolver{steps: []resolveStep{
		{resp: &remoteconnect.ResolveResponse{Capability: ""}},
		{err: &resolveStatusError{status: http.StatusForbidden, body: "done"}},
	}}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	u.renew(ctx, r)

	if got := u.cap(); got != initialCapability {
		t.Errorf("capability = %q, want the original to be kept", got)
	}
	if got := out.String(); !strings.Contains(got, "no capability") {
		t.Errorf("expected the empty response to be reported, got %q", got)
	}
}

// An agent left unreached is retried on the error cadence. The renewal response asks for
// a 1200s interval, so a test that finishes at all proves the retry did not wait for it.
func TestRenewRetriesUnreachedAgentBeforeRenewAfter(t *testing.T) {
	out := &syncBuf{}
	echo := startLocalEcho(t)

	var mu sync.Mutex
	dials := 0
	dial := func(context.Context, remoteconnect.AgentEndpoint, func() string) (tunnel, error) {
		mu.Lock()
		dials++
		first := dials == 1
		mu.Unlock()
		if first {
			return nil, errors.New("connection refused")
		}
		return &fakeTunnel{addr: echo}, nil
	}

	u := newRemoteUnit(
		remoteconnect.ResolveRequest{Project: "doclet", Component: "doc"},
		&remoteconnect.ResolveResponse{
			Capability: initialCapability,
			Targets: []remoteconnect.ResolvedTarget{
				{Key: "res/postgres/primary", Proto: "tcp", AgentID: "dp-a"},
			},
			Agents: map[string]remoteconnect.AgentEndpoint{"dp-a": {ServerName: "dp-a.remote-connect"}},
		}, out, dial)
	u.minInterval = time.Millisecond
	u.maxRetry = time.Millisecond
	u.installTunnel("dp-a", &fakeTunnel{addr: echo})

	steps := make([]resolveStep, 0, 8)
	for range 8 {
		steps = append(steps, resolveStep{resp: &remoteconnect.ResolveResponse{
			Capability: "capability-2",
			Targets: []remoteconnect.ResolvedTarget{
				{Key: "res/postgres/primary", Proto: "tcp", AgentID: "dp-b"},
			},
			Agents:            map[string]remoteconnect.AgentEndpoint{"dp-b": {ServerName: "dp-b.remote-connect"}},
			RenewAfterSeconds: 1200,
		}})
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go u.renew(ctx, &scriptedResolver{steps: steps})

	deadline := time.Now().Add(5 * time.Second)
	for {
		mu.Lock()
		retried := dials >= 2
		mu.Unlock()
		if retried {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("the unreached agent was never re-dialed; output:\n%s", out.String())
		}
		time.Sleep(time.Millisecond)
	}

	// The retry installed a working tunnel, so the moved key is servable again.
	c, err := u.openStream("res/postgres/primary")
	if err != nil {
		t.Fatalf("open stream after the retry: %v", err)
	}
	_ = c.Close()
}

// A renewal scheduled at or after the expiry cannot refresh anything, so neither the
// server's cadence nor the floor may outlast the capability.
func TestNextIntervalStaysInsideTheCapabilityLife(t *testing.T) {
	tests := []struct {
		name        string
		renewAfter  time.Duration
		minInterval time.Duration
		remaining   time.Duration
	}{
		{
			name:       "server cadence longer than the remaining life",
			renewAfter: 1200 * time.Second, minInterval: minRenewInterval, remaining: 30 * time.Second,
		},
		{
			name:       "floor longer than a one-second capability",
			renewAfter: 0, minInterval: minRenewInterval, remaining: time.Second,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := &syncBuf{}
			u, _ := unitFixture(t, out)
			u.minInterval = tt.minInterval
			u.mu.Lock()
			u.renewAfter = tt.renewAfter
			u.expiry = time.Now().Add(tt.remaining)
			u.mu.Unlock()

			got := u.nextInterval()
			if got <= 0 {
				t.Fatalf("nextInterval() = %s, want a positive wait", got)
			}
			if got >= tt.remaining {
				t.Errorf("nextInterval() = %s, want less than the %s remaining", got, tt.remaining)
			}
		})
	}
}

// A retry scheduled at the expiry itself lands too late to renew anything.
func TestRetryIntervalStartsBeforeExpiry(t *testing.T) {
	out := &syncBuf{}
	u, _ := unitFixture(t, out)
	u.minInterval = minRenewInterval
	u.maxRetry = maxRenewRetryInterval
	const remaining = 5 * time.Second
	u.mu.Lock()
	u.expiry = time.Now().Add(remaining)
	u.mu.Unlock()

	got := u.retryInterval(1)
	if got <= 0 {
		t.Fatalf("retryInterval() = %s, want a positive wait", got)
	}
	// Equal to the remaining life would land exactly at the expiry, so require real margin.
	if got >= remaining*3/4 {
		t.Errorf("retryInterval() = %s, want comfortably less than the %s remaining", got, remaining)
	}
}
