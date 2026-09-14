// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package remote

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/openchoreo/openchoreo/internal/remoteconnect"
)

// Fallbacks for a control plane that supplies no ResolveResponse.RenewAfterSeconds, and
// bounds on what a misconfigured one can ask for.
const (
	minRenewInterval = 10 * time.Second
	// Share of a capability's remaining life to wait when the server supplied no cadence.
	fallbackRenewFraction = 2
	maxRenewRetryInterval = 60 * time.Second
)

// remoteUnit is one resolved workload within a session: the request that resolved it,
// the capability that authorizes its streams, the tunnels its targets fan out to, and
// which agent serves each target key. A resolve is per workload, so renewal is too.
type remoteUnit struct {
	req  remoteconnect.ResolveRequest
	out  io.Writer
	dial func(ctx context.Context, agent remoteconnect.AgentEndpoint, capability func() string) (tunnel, error)

	mu         sync.RWMutex
	capability string
	expiry     time.Time
	renewAfter time.Duration
	// tunnels and agents are keyed by agent ID (the provider's data-plane namespace).
	tunnels map[string]tunnel
	agents  map[string]remoteconnect.AgentEndpoint
	// bound is the target key -> agent ID map fixed at session start, one entry per key
	// that got a local listener. A listener cannot be added to a running session, so it
	// never changes.
	bound map[string]string
	// live is the target key -> agent ID map from the current capability. A key in bound
	// but not in live has lost its authorization or its resolution.
	live map[string]string
	// warnedUnbound keeps a newly appeared key from being reported on every renewal.
	warnedUnbound map[string]bool
	// retired holds tunnels a renewal replaced. They stay open for the streams already
	// running on them and are closed with the session.
	retired []tunnel

	// minInterval floors and maxRetry caps every computed wait; overridable in tests.
	minInterval time.Duration
	maxRetry    time.Duration
}

// newRemoteUnit captures the result of a workload's first resolve. Built before the
// tunnels are dialed, because each tunnel needs this unit's capability provider;
// installTunnel then registers them.
func newRemoteUnit(req remoteconnect.ResolveRequest, resp *remoteconnect.ResolveResponse,
	out io.Writer,
	dial func(context.Context, remoteconnect.AgentEndpoint, func() string) (tunnel, error)) *remoteUnit {
	u := &remoteUnit{
		req:           req,
		out:           out,
		dial:          dial,
		minInterval:   minRenewInterval,
		maxRetry:      maxRenewRetryInterval,
		capability:    resp.Capability,
		tunnels:       map[string]tunnel{},
		agents:        map[string]remoteconnect.AgentEndpoint{},
		bound:         map[string]string{},
		live:          map[string]string{},
		warnedUnbound: map[string]bool{},
	}
	for id, a := range resp.Agents {
		u.agents[id] = a
	}
	for _, t := range resp.Targets {
		u.bound[t.Key] = t.AgentID
		u.live[t.Key] = t.AgentID
	}
	u.absorbTiming(resp)
	return u
}

// installTunnel registers a freshly dialed tunnel for one agent ID.
func (u *remoteUnit) installTunnel(agentID string, tn tunnel) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.tunnels[agentID] = tn
}

// absorbTiming records the expiry and cadence of the capability in resp. The caller must
// hold the write lock, except during construction.
func (u *remoteUnit) absorbTiming(resp *remoteconnect.ResolveResponse) {
	if exp, ok := remoteconnect.CapabilityExpiry(resp.Capability); ok {
		u.expiry = exp
	}
	if resp.RenewAfterSeconds > 0 {
		u.renewAfter = time.Duration(resp.RenewAfterSeconds) * time.Second
	} else {
		u.renewAfter = 0 // fall back to a fraction of the remaining life
	}
}

// cap is the capability provider handed to every tunnel this unit owns.
func (u *remoteUnit) cap() string {
	u.mu.RLock()
	defer u.mu.RUnlock()
	return u.capability
}

// expiresAt reports when the current capability stops authorizing new connections.
func (u *remoteUnit) expiresAt() time.Time {
	u.mu.RLock()
	defer u.mu.RUnlock()
	return u.expiry
}

// openStream opens a stream for one target key on whichever tunnel currently serves it.
// The lookup is per call so that a renewal moving a dependency to a different agent, or
// dropping it, takes effect on the next connection.
func (u *remoteUnit) openStream(key string) (net.Conn, error) {
	u.mu.RLock()
	agentID, live := u.live[key]
	tn := u.tunnels[agentID]
	// Read with the tunnel it belongs to, so a renewal cannot repoint the stream.
	capability := u.capability
	u.mu.RUnlock()

	if !live {
		return nil, errRevoked
	}
	if tn == nil {
		return nil, fmt.Errorf("no remote-agent tunnel for %s", key)
	}
	return tn.OpenStreamWith(key, capability)
}

// errRevoked is returned for a key the current capability no longer authorizes.
var errRevoked = errors.New("no longer authorized in this environment")

// adopt reconciles a renewal's result into the unit, swaps in the new capability, and
// reports how many agents it could not reach.
//
// Tunnels are dialed before the capability is swapped: the new capability's targets name
// the new agent namespaces, and an agent refuses a target belonging to another
// namespace, so swapping first would break every connection opened in between.
func (u *remoteUnit) adopt(ctx context.Context, resp *remoteconnect.ResolveResponse) int {
	newLive := make(map[string]string, len(resp.Targets))
	for _, t := range resp.Targets {
		newLive[t.Key] = t.AgentID
	}

	// Agents the new target set needs that are not already served by an identical tunnel.
	u.mu.RLock()
	needed := map[string]remoteconnect.AgentEndpoint{}
	for _, agentID := range newLive {
		endpoint, ok := resp.Agents[agentID]
		if !ok {
			continue // resolve named an agent it did not describe; leave the old tunnel
		}
		if existing, have := u.tunnels[agentID]; have && existing != nil && u.agents[agentID] == endpoint {
			continue
		}
		needed[agentID] = endpoint
	}
	u.mu.RUnlock()

	dialed := map[string]tunnel{}
	unreached := 0
	for agentID, endpoint := range needed {
		tn, err := u.dial(ctx, endpoint, u.cap)
		if err != nil {
			unreached++
			// Leave the old tunnel in place: a failed re-dial must not take down a
			// session that is otherwise working.
			fmt.Fprintf(u.out, "  ! could not connect the remote-agent serving %s after renewal: %v\n", agentID, err)
			continue
		}
		dialed[agentID] = tn
	}

	u.mu.Lock()
	defer u.mu.Unlock()

	for agentID, tn := range dialed {
		if old, ok := u.tunnels[agentID]; ok && old != nil && old != tn {
			u.retired = append(u.retired, old)
		}
		u.tunnels[agentID] = tn
	}
	// An endpoint is recorded only once a tunnel serves it, so a failed re-dial still reads
	// as changed and is retried on the next renewal.
	for agentID, endpoint := range resp.Agents {
		if _, wanted := needed[agentID]; wanted {
			if _, ok := dialed[agentID]; !ok {
				continue
			}
		}
		u.agents[agentID] = endpoint
	}

	// A bound key that dropped out of the capability: report it once, on the transition.
	for key := range u.bound {
		_, wasLive := u.live[key]
		_, isLive := newLive[key]
		switch {
		case wasLive && !isLive:
			fmt.Fprintf(u.out, "  ! %s: %v; new connections will be refused\n", key, errRevoked)
		case !wasLive && isLive:
			fmt.Fprintf(u.out, "  %s: authorized again\n", key)
		}
	}
	// A key the capability now carries that no listener stands for.
	for key := range newLive {
		if _, isBound := u.bound[key]; !isBound && !u.warnedUnbound[key] {
			u.warnedUnbound[key] = true
			fmt.Fprintf(u.out, "  ! %s is a new dependency; restart `occ remote` to open a local port for it\n", key)
		}
	}

	u.capability = resp.Capability
	u.live = newLive
	u.absorbTiming(resp)
	return unreached
}

// needsRenewal reports whether this unit owns a listener. A fetch-only workload read its
// values at startup and will never open another stream, so its capability is not renewed.
func (u *remoteUnit) needsRenewal() bool {
	u.mu.RLock()
	defer u.mu.RUnlock()
	return len(u.bound) > 0
}

// retiredTunnels returns the tunnels superseded by renewals, for the session to close.
func (u *remoteUnit) retiredTunnels() []tunnel {
	u.mu.RLock()
	defer u.mu.RUnlock()
	return append([]tunnel(nil), u.retired...)
}

// nextInterval is how long to wait before the next renewal attempt.
func (u *remoteUnit) nextInterval() time.Duration {
	u.mu.RLock()
	after, expiry := u.renewAfter, u.expiry
	u.mu.RUnlock()

	remaining := time.Until(expiry)
	if after <= 0 {
		// The server said nothing: wait out a fraction of the remaining life.
		if remaining <= 0 {
			return u.minInterval
		}
		after = remaining / fallbackRenewFraction
	}
	if after < u.minInterval {
		after = u.minInterval
	}
	// A renewal at or after the expiry cannot refresh anything, so the floor and any
	// server cadence both give way to the remaining life.
	if remaining > 0 && after >= remaining {
		after = remaining / fallbackRenewFraction
	}
	return after
}

// renew keeps this unit's capability current until ctx is done or the control plane
// refuses in a way that retrying cannot fix. Renewal is a resolve, so it re-runs every
// authorization check the original connect ran: the capability's expiry is the session's
// revocation window rather than its length.
func (u *remoteUnit) renew(ctx context.Context, resolver Resolver) {
	req := u.req
	req.Purpose = remoteconnect.PurposeRenew
	// Values were fetched once at session start and are already in the subshell's
	// environment, so renewals ask for no grants (see ResolveRequest.SkipSecrets).
	req.SkipSecrets = true

	wait := u.nextInterval()
	failures := 0
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		}

		req.PreviousCapability = u.cap()
		resp, err := resolver.Resolve(ctx, req)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			var statusErr *resolveStatusError
			if errors.As(err, &statusErr) && statusErr.terminal() {
				fmt.Fprintf(u.out, "  ! session ended for %s/%s: %s\n",
					u.req.Project, u.req.Component, terminalReason(statusErr))
				return
			}
			failures++
			wait = u.retryInterval(failures)
			fmt.Fprintf(u.out, "  ! could not renew the session for %s/%s: %v\n%s\n",
				u.req.Project, u.req.Component, err, u.renewalDeadlineNote(wait))
			continue
		}

		if resp.Capability == "" {
			// Adopting it would leave every subsequent stream presenting nothing, which
			// the agent refuses. Keep the capability we have and retry.
			failures++
			wait = u.retryInterval(failures)
			fmt.Fprintf(u.out, "  ! renewal for %s/%s returned no capability\n%s\n",
				u.req.Project, u.req.Component, u.renewalDeadlineNote(wait))
			continue
		}

		// An unreached agent cannot serve its keys, so retry on the error cadence instead
		// of waiting out a full renewal interval.
		if unreached := u.adopt(ctx, resp); unreached > 0 {
			failures++
			wait = u.retryInterval(failures)
			continue
		}
		failures = 0
		wait = u.nextInterval()
	}
}

// retryInterval backs off after a failed renewal, bounded so a recovered control plane
// is noticed promptly.
func (u *remoteUnit) retryInterval(failures int) time.Duration {
	wait := time.Duration(failures) * 10 * time.Second
	if wait > u.maxRetry {
		wait = u.maxRetry
	}
	if wait < u.minInterval {
		wait = u.minInterval
	}
	// A retry has to land before new connections start failing.
	if remaining := time.Until(u.expiresAt()); remaining > 0 && wait >= remaining {
		wait = remaining / fallbackRenewFraction
	}
	return wait
}

// renewalDeadlineNote says what the developer loses, and when, if the retries keep
// failing.
func (u *remoteUnit) renewalDeadlineNote(retryIn time.Duration) string {
	expiry := u.expiresAt()
	if expiry.IsZero() {
		return fmt.Sprintf("    retrying in %s", retryIn.Truncate(time.Second))
	}
	if remaining := time.Until(expiry); remaining > 0 {
		return fmt.Sprintf("    retrying in %s; new connections stop working at %s (in %s)",
			retryIn.Truncate(time.Second), expiry.Local().Format(time.Kitchen), remaining.Truncate(time.Second))
	}
	return fmt.Sprintf("    retrying in %s; new connections are already being refused, "+
		"established ones are unaffected", retryIn.Truncate(time.Second))
}

// terminalReason turns a refusal into the developer's next step.
func terminalReason(err *resolveStatusError) string {
	switch err.status {
	case http.StatusUnauthorized:
		return "your login is no longer valid — run `occ login`, then restart `occ remote`"
	case http.StatusForbidden:
		body := err.body
		if body == "" {
			body = "the control plane refused to renew it"
		}
		return body + " — restart `occ remote` to start a new session"
	default:
		return err.Error()
	}
}
