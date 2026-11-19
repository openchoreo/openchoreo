// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

// Package destination decides which addresses alert notifications may be sent to.
//
// Public addresses are always allowed. Loopback, link-local (including cloud metadata),
// multicast and other special-purpose addresses are never allowed, nor are the addresses of
// the services the observer itself uses. Private addresses are allowed only when the operator
// lists them, by address, CIDR or host name.
package destination

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"strings"
	"sync"
	"syscall"
	"time"
)

// ErrNotAllowed is returned when a notification would connect to an address that the policy does
// not allow.
var ErrNotAllowed = errors.New("notification destination is not allowed")

// Options configure a Policy.
type Options struct {
	// Private lists the private addresses (RFC 1918, CGNAT 100.64.0.0/10 and unique local IPv6
	// fc00::/7) that are allowed. No other private address is.
	Private Entries
	// ProtectedHosts are the hosts of the services the observer itself uses. Their addresses are
	// never connected to.
	ProtectedHosts []string
	// AllowLoopback allows loopback addresses. Tests set it to reach their servers, which listen
	// on loopback; the observer never does.
	AllowLoopback bool
}

// Entries are private destinations, given as IP addresses, CIDRs or host names.
type Entries struct {
	Prefixes []netip.Prefix
	Hosts    []string
}

// IsEmpty reports whether e lists nothing.
func (e Entries) IsEmpty() bool {
	return len(e.Prefixes) == 0 && len(e.Hosts) == 0
}

var (
	// cgnatPrefix is the shared address space (RFC 6598), which is private like RFC 1918.
	cgnatPrefix = netip.MustParsePrefix("100.64.0.0/10")

	// privatePrefixes are the ranges isPrivate reports.
	privatePrefixes = []netip.Prefix{
		netip.MustParsePrefix("10.0.0.0/8"),
		netip.MustParsePrefix("172.16.0.0/12"),
		netip.MustParsePrefix("192.168.0.0/16"),
		cgnatPrefix,
		netip.MustParsePrefix("fc00::/7"),
	}

	// specialPurposePrefixes are the special-purpose ranges, besides those that netip reports as
	// loopback, link-local, multicast or unspecified, that are never connected to. They include
	// the IPv6 ranges that embed or translate to an IPv4 address.
	specialPurposePrefixes = []netip.Prefix{
		netip.MustParsePrefix("0.0.0.0/8"),
		netip.MustParsePrefix("192.0.0.0/24"),
		netip.MustParsePrefix("192.0.2.0/24"),
		netip.MustParsePrefix("192.88.99.0/24"),
		netip.MustParsePrefix("198.18.0.0/15"),
		netip.MustParsePrefix("198.51.100.0/24"),
		netip.MustParsePrefix("203.0.113.0/24"),
		netip.MustParsePrefix("240.0.0.0/4"),
		netip.MustParsePrefix("::/96"),
		netip.MustParsePrefix("64:ff9b::/96"),
		netip.MustParsePrefix("64:ff9b:1::/48"),
		netip.MustParsePrefix("100::/64"),
		netip.MustParsePrefix("2001::/23"),
		netip.MustParsePrefix("2001:db8::/32"),
		netip.MustParsePrefix("2002::/16"),
		netip.MustParsePrefix("3fff::/20"),
		netip.MustParsePrefix("fec0::/10"),
	}
)

// normalize returns an address in the form it is compared in.
func normalize(addr netip.Addr) netip.Addr {
	return addr.Unmap().WithZone("")
}

// isSpecialPurpose reports whether addr is a loopback, link-local, multicast, broadcast,
// unspecified or other special-purpose address.
func isSpecialPurpose(addr netip.Addr) bool {
	addr = normalize(addr)
	if !addr.IsGlobalUnicast() {
		return true
	}
	for _, prefix := range specialPurposePrefixes {
		if prefix.Contains(addr) {
			return true
		}
	}
	return false
}

// isPrivate reports whether addr is a private address: RFC 1918, CGNAT or unique local IPv6.
func isPrivate(addr netip.Addr) bool {
	addr = normalize(addr)
	for _, prefix := range privatePrefixes {
		if prefix.Contains(addr) {
			return true
		}
	}
	return false
}

// isPrivatePrefix reports whether every address in prefix is private.
func isPrivatePrefix(prefix netip.Prefix) bool {
	if !prefix.IsValid() {
		return false
	}
	prefix = prefix.Masked()
	first := prefix.Addr()
	last := lastAddr(prefix)
	for _, private := range privatePrefixes {
		if private.Contains(first) && private.Contains(last) {
			return true
		}
	}
	return false
}

func lastAddr(prefix netip.Prefix) netip.Addr {
	b := prefix.Addr().AsSlice()
	for i := prefix.Bits(); i < len(b)*8; i++ {
		b[i/8] |= 1 << (7 - i%8)
	}
	addr, _ := netip.AddrFromSlice(b)
	return addr
}

// NormalizeHost returns a host name in the form it is compared in.
func NormalizeHost(host string) string {
	return strings.TrimSuffix(strings.ToLower(strings.TrimSpace(host)), ".")
}

// ParseEntries parses private destinations: IP addresses, CIDRs and host names. An IP address or
// CIDR must be private, since public addresses are always allowed and other addresses never are.
func ParseEntries(values []string) (Entries, error) {
	var entries Entries
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		prefix, err := netip.ParsePrefix(value)
		if err != nil {
			if addr, addrErr := netip.ParseAddr(value); addrErr == nil {
				addr = normalize(addr)
				prefix, err = netip.PrefixFrom(addr, addr.BitLen()), nil
			}
		}
		if err == nil {
			if prefix.Addr().Is4In6() {
				prefix = netip.PrefixFrom(prefix.Addr().Unmap(), prefix.Bits()-96)
			}
			if !isPrivatePrefix(prefix) {
				return Entries{}, fmt.Errorf("%q is not a private address range; public addresses are always "+
					"allowed, and loopback, link-local and other special-purpose addresses never are", value)
			}
			entries.Prefixes = append(entries.Prefixes, prefix.Masked())
			continue
		}
		host := NormalizeHost(value)
		if !isHostName(host) {
			return Entries{}, fmt.Errorf("%q is neither an IP address, a CIDR nor a host name", value)
		}
		if host == "localhost" || strings.HasSuffix(host, ".localhost") {
			return Entries{}, fmt.Errorf("%q names a loopback address, which is never allowed", value)
		}
		entries.Hosts = append(entries.Hosts, host)
	}
	return entries, nil
}

// isHostName reports whether s is a syntactically valid DNS host name.
func isHostName(s string) bool {
	if s == "" || len(s) > 253 {
		return false
	}
	for _, label := range strings.Split(s, ".") {
		if label == "" || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, r := range label {
			if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '-' && r != '_' {
				return false
			}
		}
	}
	return true
}

// Policy decides which addresses notifications may connect to. Its DialContext applies it to
// every connection.
type Policy struct {
	allowLoopback bool
	private       Entries
	privateHosts  map[string]bool
	protected     *protectedAddrs
}

// NewPolicy returns a Policy for the given options.
func NewPolicy(opts Options) *Policy {
	hosts := make(map[string]bool, len(opts.Private.Hosts))
	for _, host := range opts.Private.Hosts {
		hosts[NormalizeHost(host)] = true
	}
	return &Policy{
		allowLoopback: opts.AllowLoopback,
		private:       opts.Private,
		privateHosts:  hosts,
		protected:     newProtectedAddrs(opts.ProtectedHosts),
	}
}

// DialContext connects like net.Dialer.DialContext, refusing every address the policy does not
// allow. The check runs on each address the name resolves to, so a name that resolves to a
// refused address, or one whose answer changes between checks, is refused too.
func (p *Policy) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return nil, err
	}
	host = NormalizeHost(host)
	protected := p.protected.snapshot(ctx)
	dialer := &net.Dialer{
		Timeout:   30 * time.Second,
		KeepAlive: 30 * time.Second,
		Control: func(_, address string, _ syscall.RawConn) error {
			ip, _, err := net.SplitHostPort(address)
			if err != nil {
				return fmt.Errorf("%w: %w", ErrNotAllowed, err)
			}
			addr, err := netip.ParseAddr(ip)
			if err != nil {
				return fmt.Errorf("%w: %q is not an IP address", ErrNotAllowed, ip)
			}
			return p.check(host, normalize(addr), protected)
		},
	}
	return dialer.DialContext(ctx, network, address)
}

// check decides whether a connection to addr, made for host, is allowed.
func (p *Policy) check(host string, addr netip.Addr, protected map[netip.Addr]bool) error {
	switch {
	case protected[addr]:
		return fmt.Errorf("%w: %s belongs to a service the observer itself uses", ErrNotAllowed, addr)
	case p.allowLoopback && addr.IsLoopback():
		return nil
	case isSpecialPurpose(addr):
		return fmt.Errorf("%w: %s is a loopback, link-local or other special-purpose address", ErrNotAllowed, addr)
	case !isPrivate(addr), p.privateHosts[host]:
		return nil
	}
	for _, prefix := range p.private.Prefixes {
		if prefix.Contains(addr) {
			return nil
		}
	}
	return fmt.Errorf("%w: %s is a private address that is not among the listed private destinations", ErrNotAllowed, addr)
}

const (
	// protectedRefreshInterval is how often the addresses of the protected hosts are looked up again.
	protectedRefreshInterval = 30 * time.Second
	// protectedLookupTimeout bounds one round of lookups of the protected hosts.
	protectedLookupTimeout = 2 * time.Second
)

// protectedAddrs are the addresses of the services the observer uses and of the observer itself.
type protectedAddrs struct {
	hosts  []string
	static map[netip.Addr]bool

	mu        sync.Mutex
	resolved  map[string][]netip.Addr
	current   map[netip.Addr]bool
	refreshed time.Time
	lookup    func(ctx context.Context, host string) ([]netip.Addr, error)
}

func newProtectedAddrs(hosts []string) *protectedAddrs {
	s := &protectedAddrs{
		static:   ownAddrs(),
		resolved: map[string][]netip.Addr{},
		lookup: func(ctx context.Context, host string) ([]netip.Addr, error) {
			return net.DefaultResolver.LookupNetIP(ctx, "ip", host)
		},
	}
	for _, host := range hosts {
		host = NormalizeHost(host)
		if host == "" {
			continue
		}
		if addr, err := netip.ParseAddr(host); err == nil {
			s.static[normalize(addr)] = true
			continue
		}
		s.hosts = append(s.hosts, host)
	}
	s.current = s.merge()
	return s
}

// snapshot returns the protected addresses, looking the hosts up again when the last lookup is
// older than protectedRefreshInterval. A host whose lookup fails keeps the addresses it had.
func (s *protectedAddrs) snapshot(ctx context.Context) map[netip.Addr]bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.hosts) == 0 || time.Since(s.refreshed) < protectedRefreshInterval {
		return s.current
	}
	lookupCtx, cancel := context.WithTimeout(ctx, protectedLookupTimeout)
	defer cancel()
	results := make([][]netip.Addr, len(s.hosts))
	var wg sync.WaitGroup
	for i, host := range s.hosts {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if addrs, err := s.lookup(lookupCtx, host); err == nil {
				results[i] = addrs
			}
		}()
	}
	wg.Wait()
	for i, host := range s.hosts {
		if len(results[i]) > 0 {
			s.resolved[host] = results[i]
		}
	}
	s.refreshed = time.Now()
	s.current = s.merge()
	return s.current
}

func (s *protectedAddrs) merge() map[netip.Addr]bool {
	merged := make(map[netip.Addr]bool, len(s.static))
	for addr := range s.static {
		merged[addr] = true
	}
	for _, addrs := range s.resolved {
		for _, addr := range addrs {
			merged[normalize(addr)] = true
		}
	}
	return merged
}

// ownAddrs returns the addresses of this host's network interfaces, so that notifications cannot
// reach the observer through its own pod address.
func ownAddrs() map[netip.Addr]bool {
	own := map[netip.Addr]bool{}
	ifaceAddrs, err := net.InterfaceAddrs()
	if err != nil {
		return own
	}
	for _, ifaceAddr := range ifaceAddrs {
		ipNet, ok := ifaceAddr.(*net.IPNet)
		if !ok {
			continue
		}
		if addr, ok := netip.AddrFromSlice(ipNet.IP); ok && !addr.IsLoopback() {
			own[normalize(addr)] = true
		}
	}
	return own
}
