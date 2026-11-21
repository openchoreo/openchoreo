// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package destination

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	specialAddrs = []string{
		"127.0.0.1", "127.1.2.3", "0.0.0.0", "0.1.2.3", "255.255.255.255",
		"169.254.169.254", "224.0.0.1", "240.0.0.1",
		"192.0.0.8", "192.0.2.1", "198.18.0.1", "198.51.100.1", "203.0.113.1",
		"::", "::1", "::a00:1", "fe80::1", "fe80::1%eth0", "fec0::1", "ff02::1",
		"::ffff:127.0.0.1", "::ffff:169.254.169.254",
		"64:ff9b::a00:1", "64:ff9b:1::1", "2002:a00:1::", "2001::1", "2001:db8::1", "100::1", "3fff::1",
	}
	privateAddrs = []string{
		"10.0.0.1", "172.16.5.4", "172.31.255.255", "192.168.1.1", "100.64.0.1", "100.127.255.255",
		"fc00::1", "fd12:3456::1", "::ffff:10.0.0.1",
	}
	publicAddrs = []string{"8.8.8.8", "1.1.1.1", "172.32.0.1", "100.128.0.1", "2606:4700:4700::1111", "::ffff:8.8.8.8"}
)

func mustAddr(s string) netip.Addr { return normalize(netip.MustParseAddr(s)) }

func TestClassification(t *testing.T) {
	for _, s := range specialAddrs {
		assert.True(t, isSpecialPurpose(mustAddr(s)), "%s must be special-purpose", s)
	}
	for _, s := range privateAddrs {
		assert.False(t, isSpecialPurpose(mustAddr(s)), "%s must not be special-purpose", s)
		assert.True(t, isPrivate(mustAddr(s)), "%s must be private", s)
	}
	for _, s := range publicAddrs {
		assert.False(t, isSpecialPurpose(mustAddr(s)), "%s must not be special-purpose", s)
		assert.False(t, isPrivate(mustAddr(s)), "%s must not be private", s)
	}
}

func TestPolicyCheck(t *testing.T) {
	none := protection{}

	t.Run("default allows only public addresses", func(t *testing.T) {
		p := NewPolicy(Options{})
		for _, s := range publicAddrs {
			assert.NoError(t, p.check("receiver.example.com", mustAddr(s), none), s)
		}
		for _, s := range append(append([]string{}, privateAddrs...), specialAddrs...) {
			assert.ErrorIs(t, p.check("receiver.example.com", mustAddr(s), none), ErrNotAllowed, s)
		}
	})

	t.Run("listing every private range never allows special-purpose addresses", func(t *testing.T) {
		p := NewPolicy(Options{Private: Entries{Prefixes: privatePrefixes}})
		for _, s := range privateAddrs {
			assert.NoError(t, p.check("receiver", mustAddr(s), none), s)
		}
		for _, s := range specialAddrs {
			assert.ErrorIs(t, p.check("receiver", mustAddr(s), none), ErrNotAllowed, s)
		}
	})

	t.Run("only listed private destinations are allowed", func(t *testing.T) {
		p := NewPolicy(Options{Private: Entries{
			Prefixes: []netip.Prefix{netip.MustParsePrefix("10.1.2.0/24")},
			Hosts:    []string{"relay.corp.internal"},
		}})
		assert.NoError(t, p.check("anything", mustAddr("10.1.2.3"), none), "an address in a listed range")
		assert.NoError(t, p.check("anything", mustAddr("::ffff:10.1.2.3"), none), "an IPv4-mapped address in a listed range")
		assert.NoError(t, p.check("relay.corp.internal", mustAddr("10.9.9.9"), none), "a listed host")
		assert.ErrorIs(t, p.check("other.corp.internal", mustAddr("10.9.9.9"), none), ErrNotAllowed, "an unlisted host")
		assert.ErrorIs(t, p.check("anything", mustAddr("10.1.3.1"), none), ErrNotAllowed, "an address outside the listed range")
		assert.ErrorIs(t, p.check("relay.corp.internal", mustAddr("127.0.0.1"), none), ErrNotAllowed,
			"a listed host that resolves to loopback")
		assert.NoError(t, p.check("anything", mustAddr("8.8.8.8"), none), "public addresses stay allowed")
	})

	t.Run("protected addresses are refused whatever else allows them", func(t *testing.T) {
		protected := protection{addrs: map[netip.Addr]bool{mustAddr("10.43.0.9"): true, mustAddr("8.8.4.4"): true}}
		p := NewPolicy(Options{Private: Entries{
			Prefixes: []netip.Prefix{netip.MustParsePrefix("10.43.0.0/16")},
			Hosts:    []string{"relay.corp.internal"},
		}})
		err := p.check("relay.corp.internal", mustAddr("10.43.0.9"), protected)
		assert.ErrorIs(t, err, ErrNotAllowed)
		assert.Contains(t, err.Error(), "the observer itself uses")
		assert.ErrorIs(t, p.check("receiver", mustAddr("8.8.4.4"), protected), ErrNotAllowed)
		assert.NoError(t, p.check("receiver", mustAddr("10.43.0.10"), protected))
	})

	t.Run("loopback for tests", func(t *testing.T) {
		p := NewPolicy(Options{AllowLoopback: true})
		assert.NoError(t, p.check("localhost", mustAddr("127.0.0.1"), none))
		assert.NoError(t, p.check("localhost", mustAddr("::1"), none))
		assert.ErrorIs(t, p.check("receiver", mustAddr("169.254.169.254"), none), ErrNotAllowed)
		assert.ErrorIs(t, p.check("receiver", mustAddr("10.0.0.1"), none), ErrNotAllowed)
	})
}

func TestParseEntries(t *testing.T) {
	entries, err := ParseEntries([]string{
		" 10.43.0.0/16", "10.0.12.5", "", "fd00::/64", "::ffff:192.168.1.10", "::ffff:10.0.0.0/104",
		"100.64.1.0/24", "Relay.Corp.Internal.", "webhook-bridge.monitoring.svc.cluster.local",
	})
	require.NoError(t, err)
	assert.Equal(t, []netip.Prefix{
		netip.MustParsePrefix("10.43.0.0/16"),
		netip.MustParsePrefix("10.0.12.5/32"),
		netip.MustParsePrefix("fd00::/64"),
		netip.MustParsePrefix("192.168.1.10/32"),
		netip.MustParsePrefix("10.0.0.0/8"),
		netip.MustParsePrefix("100.64.1.0/24"),
	}, entries.Prefixes)
	assert.Equal(t, []string{"relay.corp.internal", "webhook-bridge.monitoring.svc.cluster.local"}, entries.Hosts)

	empty, err := ParseEntries(nil)
	require.NoError(t, err)
	assert.True(t, empty.IsEmpty())

	for _, value := range []string{
		"127.0.0.1", "169.254.169.254", "0.0.0.0/0", "10.0.0.0/7", "8.8.8.8", "fe80::/10",
		"localhost", "metrics.localhost", "bad host", "-bad.example.com", "a..b", "::ffff:0:0/80",
	} {
		_, err := ParseEntries([]string{value})
		assert.Error(t, err, value)
	}
}

func TestProtectedAddrsSnapshot(t *testing.T) {
	answers := map[string][]netip.Addr{"logs-adapter": {netip.MustParseAddr("10.43.0.9")}}
	s := newProtectedAddrs([]string{"Logs-Adapter.", "10.43.0.20", ""})
	s.lookup = func(_ context.Context, host string) ([]netip.Addr, error) {
		if addrs, ok := answers[host]; ok {
			return addrs, nil
		}
		return nil, errors.New("no such host")
	}

	got := mustSnapshot(t, s, context.Background())
	assert.True(t, got.addrs[mustAddr("10.43.0.9")], "a protected host's address")
	assert.True(t, got.addrs[mustAddr("10.43.0.20")], "a protected IP address")
	assert.True(t, got.hosts["logs-adapter"], "a protected host's name")
	assert.False(t, got.incomplete)

	// A failed lookup keeps the addresses the host had.
	delete(answers, "logs-adapter")
	s.refreshed = time.Time{}
	got = mustSnapshot(t, s, context.Background())
	assert.True(t, got.addrs[mustAddr("10.43.0.9")])
	assert.False(t, got.incomplete, "a host that resolved before keeps its addresses")

	// A new answer replaces them once the refresh interval has passed.
	answers["logs-adapter"] = []netip.Addr{netip.MustParseAddr("10.43.0.10")}
	assert.True(t, mustSnapshot(t, s, context.Background()).addrs[mustAddr("10.43.0.9")], "not looked up again before the interval")
	s.refreshed = time.Time{}
	got = mustSnapshot(t, s, context.Background())
	assert.True(t, got.addrs[mustAddr("10.43.0.10")])
	assert.False(t, got.addrs[mustAddr("10.43.0.9")])
}

func TestDialContext(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			_ = conn.Close()
		}
	}()
	_, port, err := net.SplitHostPort(ln.Addr().String())
	require.NoError(t, err)
	ctx := context.Background()

	_, err = NewPolicy(Options{Private: Entries{Prefixes: privatePrefixes}}).DialContext(ctx, "tcp", ln.Addr().String())
	assert.ErrorIs(t, err, ErrNotAllowed, "loopback is refused")

	conn, err := NewPolicy(Options{AllowLoopback: true}).DialContext(ctx, "tcp", ln.Addr().String())
	require.NoError(t, err)
	_ = conn.Close()

	// A protected host is refused by its address too, not only by its name.
	p := NewPolicy(Options{AllowLoopback: true, ProtectedHosts: []string{"localhost"}})
	_, err = p.DialContext(ctx, "tcp", net.JoinHostPort("127.0.0.1", port))
	assert.ErrorIs(t, err, ErrNotAllowed)
}

func TestProtectedNamesAreRefusedWhileTheirAddressesAreStale(t *testing.T) {
	ctx := context.Background()
	p := NewPolicy(Options{
		Private:        Entries{Prefixes: []netip.Prefix{netip.MustParsePrefix("10.43.0.0/16")}},
		ProtectedHosts: []string{"logs-adapter"},
	})
	answer := netip.MustParseAddr("10.43.0.9")
	p.protected.lookup = func(context.Context, string) ([]netip.Addr, error) { return []netip.Addr{answer}, nil }
	require.True(t, mustSnapshot(t, p.protected, ctx).addrs[mustAddr("10.43.0.9")])

	// The service moves to an address that the cached snapshot does not have yet.
	answer = netip.MustParseAddr("10.43.0.10")
	require.NoError(t, p.check("anything", mustAddr("10.43.0.10"), mustSnapshot(t, p.protected, ctx)), "the new address is not cached")

	_, err := p.DialContext(ctx, "tcp", "Logs-Adapter.:9098")
	assert.ErrorIs(t, err, ErrNotAllowed, "the protected name is refused before it is resolved")
}

func TestConnectionsAreCheckedAgainstProtectionRefreshedDuringTheirLookup(t *testing.T) {
	ctx := context.Background()
	p := NewPolicy(Options{
		Private:        Entries{Prefixes: []netip.Prefix{netip.MustParsePrefix("10.43.0.0/16")}},
		ProtectedHosts: []string{"logs-adapter"},
	})
	answer := netip.MustParseAddr("10.43.0.9")
	p.protected.lookup = func(context.Context, string) ([]netip.Addr, error) { return []netip.Addr{answer}, nil }
	mustSnapshot(t, p.protected, ctx)

	// DialContext builds the check before it looks the receiver up.
	control := p.control("receiver")
	require.NoError(t, control("tcp", "10.43.0.10:8080", nil), "not protected yet")

	// The protected service moves to 10.43.0.10 while the receiver is being looked up.
	answer = netip.MustParseAddr("10.43.0.10")
	p.protected.refreshed = time.Time{}
	mustSnapshot(t, p.protected, ctx)

	assert.ErrorIs(t, control("tcp", "10.43.0.10:8080", nil), ErrNotAllowed)
}

func TestUnresolvedProtectedHostRefusesListedPrivateDestinations(t *testing.T) {
	ctx := context.Background()
	p := NewPolicy(Options{
		Private:        Entries{Prefixes: []netip.Prefix{netip.MustParsePrefix("10.43.0.0/16")}},
		ProtectedHosts: []string{"logs-adapter", "sre-agent"},
	})
	var lookupErr error = &net.DNSError{Err: "i/o timeout", Name: "logs-adapter", IsTimeout: true}
	p.protected.lookup = func(_ context.Context, host string) ([]netip.Addr, error) {
		if host == "sre-agent" {
			return nil, &net.DNSError{Err: "no such host", Name: host, IsNotFound: true}
		}
		if lookupErr != nil {
			return nil, lookupErr
		}
		return []netip.Addr{netip.MustParseAddr("10.43.0.9")}, nil
	}

	prot := mustSnapshot(t, p.protected, ctx)
	require.True(t, prot.incomplete, "a failed first lookup leaves the protection incomplete")
	err := p.check("receiver", mustAddr("10.43.0.9"), prot)
	assert.ErrorIs(t, err, ErrProtectionUnavailable)
	assert.NotErrorIs(t, err, ErrNotAllowed, "the refusal is temporary")
	assert.NoError(t, p.check("receiver", mustAddr("8.8.8.8"), prot), "public addresses stay allowed")
	assert.ErrorIs(t, p.check("receiver", mustAddr("10.99.0.1"), prot), ErrNotAllowed, "unlisted private addresses are refused as before")

	// The lookup is retried after protectedRetryInterval, not protectedRefreshInterval.
	lookupErr = nil
	p.protected.refreshed = time.Now().Add(-protectedRetryInterval)
	prot = mustSnapshot(t, p.protected, ctx)
	require.False(t, prot.incomplete, "a name that does not exist has nothing to protect")
	assert.ErrorIs(t, p.check("receiver", mustAddr("10.43.0.9"), prot), ErrNotAllowed)
	assert.NoError(t, p.check("receiver", mustAddr("10.43.0.10"), prot))
}

// mustSnapshot returns s's protection, failing the test on an error.
func mustSnapshot(t *testing.T, s *protectedAddrs, ctx context.Context) protection {
	t.Helper()
	prot, err := s.snapshot(ctx)
	require.NoError(t, err)
	return prot
}

func TestProtectedAddrsRefreshIsSharedAndOutlivesItsCaller(t *testing.T) {
	release := make(chan struct{})
	var lookups atomic.Int32
	s := newProtectedAddrs([]string{"logs-adapter"})
	s.lookup = func(ctx context.Context, _ string) ([]netip.Addr, error) {
		lookups.Add(1)
		select {
		case <-release:
			return []netip.Addr{netip.MustParseAddr("10.43.0.9")}, nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	// The caller that starts the lookup stops waiting at its own deadline.
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err := s.snapshot(ctx)
	assert.ErrorIs(t, err, context.DeadlineExceeded)
	assert.Less(t, time.Since(start), protectedLookupTimeout, "the caller does not wait for the lookup")

	// Other callers wait for the same lookup instead of starting their own.
	results := make(chan protection, 3)
	for range 3 {
		go func() {
			prot, err := s.snapshot(context.Background())
			assert.NoError(t, err)
			results <- prot
		}()
	}
	close(release)
	for range 3 {
		prot := <-results
		assert.True(t, prot.addrs[mustAddr("10.43.0.9")], "the first caller's deadline does not end the shared lookup")
		assert.False(t, prot.incomplete)
	}
	assert.Equal(t, int32(1), lookups.Load(), "one lookup serves every caller")
}

func TestDialContextGivesUpWhenItsDeadlineEndsDuringARefresh(t *testing.T) {
	release := make(chan struct{})
	defer close(release)
	p := NewPolicy(Options{ProtectedHosts: []string{"logs-adapter"}})
	p.protected.lookup = func(ctx context.Context, _ string) ([]netip.Addr, error) {
		select {
		case <-release:
		case <-ctx.Done():
		}
		return nil, ctx.Err()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, err := p.DialContext(ctx, "tcp", "receiver.example.com:443")
	assert.ErrorIs(t, err, context.DeadlineExceeded, "a deadline is a temporary failure")
	assert.NotErrorIs(t, err, ErrNotAllowed)
}

type callerKey struct{}

func TestProtectedAddrsRefreshDoesNotSeeTheCallersValues(t *testing.T) {
	s := newProtectedAddrs([]string{"logs-adapter"})
	var sawCallerValue atomic.Bool
	s.lookup = func(ctx context.Context, _ string) ([]netip.Addr, error) {
		if ctx.Value(callerKey{}) != nil {
			sawCallerValue.Store(true)
		}
		return []netip.Addr{netip.MustParseAddr("10.43.0.9")}, nil
	}
	mustSnapshot(t, s, context.WithValue(context.Background(), callerKey{}, "webhook request trace"))
	assert.False(t, sawCallerValue.Load(), "the shared lookup must not carry the values of the caller that started it")
}
