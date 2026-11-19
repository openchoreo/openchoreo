// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package destination

import (
	"context"
	"errors"
	"net"
	"net/netip"
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
	none := map[netip.Addr]bool{}

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
		protected := map[netip.Addr]bool{mustAddr("10.43.0.9"): true, mustAddr("8.8.4.4"): true}
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

	got := s.snapshot(context.Background())
	assert.True(t, got[mustAddr("10.43.0.9")], "a protected host's address")
	assert.True(t, got[mustAddr("10.43.0.20")], "a protected IP address")

	// A failed lookup keeps the addresses the host had.
	delete(answers, "logs-adapter")
	s.refreshed = time.Time{}
	assert.True(t, s.snapshot(context.Background())[mustAddr("10.43.0.9")])

	// A new answer replaces them once the refresh interval has passed.
	answers["logs-adapter"] = []netip.Addr{netip.MustParseAddr("10.43.0.10")}
	assert.True(t, s.snapshot(context.Background())[mustAddr("10.43.0.9")], "not looked up again before the interval")
	s.refreshed = time.Time{}
	got = s.snapshot(context.Background())
	assert.True(t, got[mustAddr("10.43.0.10")])
	assert.False(t, got[mustAddr("10.43.0.9")])
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
