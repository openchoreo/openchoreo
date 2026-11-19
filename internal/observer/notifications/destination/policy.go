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

var (
	// ErrNotAllowed is returned when a notification would connect to an address that the policy
	// does not allow.
	ErrNotAllowed = errors.New("notification destination is not allowed")
	// ErrProtectionUnavailable is returned for a private destination while the addresses of a
	// service the observer uses could not be looked up. A later attempt can succeed.
	ErrProtectionUnavailable = errors.New("the addresses of a service the observer uses could not be looked up")
)

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
	prot, err := p.protected.snapshot(ctx)
	if err != nil {
		return nil, err
	}
	if err := prot.checkName(host); err != nil {
		return nil, err
	}
	dialer := &net.Dialer{
		Timeout:   30 * time.Second,
		KeepAlive: 30 * time.Second,
		Control:   p.control(host),
	}
	return dialer.DialContext(ctx, network, address)
}

// control returns the check the dialer runs on each address resolved for host, just before it
// connects. The check uses the latest protection rather than the one DialContext waited for, since
// the protected addresses can be refreshed while host is being looked up.
func (p *Policy) control(host string) func(network, address string, c syscall.RawConn) error {
	return func(_, address string, _ syscall.RawConn) error {
		ip, _, err := net.SplitHostPort(address)
		if err != nil {
			return fmt.Errorf("%w: %w", ErrNotAllowed, err)
		}
		addr, err := netip.ParseAddr(ip)
		if err != nil {
			return fmt.Errorf("%w: %q is not an IP address", ErrNotAllowed, ip)
		}
		return p.check(host, normalize(addr), p.protected.latest())
	}
}

// check decides whether a connection to addr, made for host, is allowed.
func (p *Policy) check(host string, addr netip.Addr, prot protection) error {
	switch {
	case prot.addrs[addr]:
		return fmt.Errorf("%w: %s belongs to a service the observer itself uses", ErrNotAllowed, addr)
	case p.allowLoopback && addr.IsLoopback():
		return nil
	case isSpecialPurpose(addr):
		return fmt.Errorf("%w: %s is a loopback, link-local or other special-purpose address", ErrNotAllowed, addr)
	case !isPrivate(addr):
		return nil
	case !p.listed(host, addr):
		return fmt.Errorf("%w: %s is a private address that is not among the listed private destinations", ErrNotAllowed, addr)
	case prot.incomplete:
		// The address could belong to a protected service whose addresses are not known yet.
		return fmt.Errorf("%w, so the private address %s is refused for now", ErrProtectionUnavailable, addr)
	}
	return nil
}

// listed reports whether a private address, reached for host, is among the private destinations.
func (p *Policy) listed(host string, addr netip.Addr) bool {
	if p.privateHosts[host] {
		return true
	}
	for _, prefix := range p.private.Prefixes {
		if prefix.Contains(addr) {
			return true
		}
	}
	return false
}

const (
	// protectedRefreshInterval is how often the addresses of the protected hosts are looked up again.
	protectedRefreshInterval = 30 * time.Second
	// protectedRetryInterval is how soon the lookups are retried while a protected host has no
	// known addresses because its lookup failed.
	protectedRetryInterval = time.Second
	// protectedLookupTimeout bounds one round of lookups of the protected hosts.
	protectedLookupTimeout = 2 * time.Second
)

// protection is what notifications never reach, as known at one point in time.
type protection struct {
	hosts map[string]bool
	addrs map[netip.Addr]bool
	// incomplete is set while a protected host has no known addresses because its lookup failed
	// for a reason other than the name not existing.
	incomplete bool
}

// checkName refuses a host that names a service the observer itself uses, whatever its name
// resolves to at the moment.
func (prot protection) checkName(host string) error {
	if prot.hosts[host] {
		return fmt.Errorf("%w: %s is a service the observer itself uses", ErrNotAllowed, host)
	}
	return nil
}

// protectedAddrs keeps the names and addresses of the services the observer uses and of the
// observer itself.
type protectedAddrs struct {
	hosts  []string
	names  map[string]bool
	static map[netip.Addr]bool
	lookup func(ctx context.Context, host string) ([]netip.Addr, error)

	mu         sync.Mutex
	resolved   map[string][]netip.Addr
	unresolved map[string]bool
	current    protection
	refreshed  time.Time
	// refreshing is closed when the running lookup of the hosts finishes; nil when none runs.
	refreshing chan struct{}
}

func newProtectedAddrs(hosts []string) *protectedAddrs {
	s := &protectedAddrs{
		names:      map[string]bool{},
		static:     ownAddrs(),
		resolved:   map[string][]netip.Addr{},
		unresolved: map[string]bool{},
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
		if !s.names[host] {
			s.names[host] = true
			s.hosts = append(s.hosts, host)
		}
	}
	s.current = s.build()
	return s
}

// snapshot returns what is protected. When the last lookup of the hosts is older than
// protectedRefreshInterval, or than protectedRetryInterval while a host is unresolved, it starts a
// lookup that every caller shares, and waits for it or for ctx to end, whichever comes first.
func (s *protectedAddrs) snapshot(ctx context.Context) (protection, error) {
	s.mu.Lock()
	if !s.dueLocked() {
		current := s.current
		s.mu.Unlock()
		return current, nil
	}
	done := s.refreshing
	if done == nil {
		done = make(chan struct{})
		s.refreshing = done
		go s.refresh(done)
	}
	s.mu.Unlock()

	select {
	case <-done:
		s.mu.Lock()
		defer s.mu.Unlock()
		return s.current, nil
	case <-ctx.Done():
		return protection{}, fmt.Errorf("waiting for the addresses of the services the observer uses: %w", ctx.Err())
	}
}

// latest returns what is protected as of the last finished lookup, without starting or waiting for
// one.
func (s *protectedAddrs) latest() protection {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.current
}

// dueLocked reports whether the hosts should be looked up again. s.mu must be held.
func (s *protectedAddrs) dueLocked() bool {
	if len(s.hosts) == 0 {
		return false
	}
	interval := protectedRefreshInterval
	if s.current.incomplete {
		interval = protectedRetryInterval
	}
	return time.Since(s.refreshed) >= interval
}

// refresh looks the hosts up and closes done. Its lookups get a context of their own rather than
// one derived from the caller that started them: that caller's deadline must not end them for the
// others, and its values, such as the httptrace hooks of a webhook request, must not observe them.
// A host whose lookup fails keeps the addresses it had; one that never resolved makes the
// protection incomplete, unless its name does not exist.
func (s *protectedAddrs) refresh(done chan struct{}) {
	defer close(done)
	ctx, cancel := context.WithTimeout(context.Background(), protectedLookupTimeout)
	defer cancel()
	type result struct {
		addrs []netip.Addr
		err   error
	}
	results := make([]result, len(s.hosts))
	var wg sync.WaitGroup
	for i, host := range s.hosts {
		wg.Add(1)
		go func() {
			defer wg.Done()
			addrs, err := s.lookup(ctx, host)
			results[i] = result{addrs: addrs, err: err}
		}()
	}
	wg.Wait()

	s.mu.Lock()
	defer s.mu.Unlock()
	for i, host := range s.hosts {
		switch r := results[i]; {
		case r.err == nil && len(r.addrs) > 0:
			s.resolved[host] = r.addrs
			delete(s.unresolved, host)
		case r.err == nil || isNotFound(r.err):
			// A service that does not exist, such as an optional agent that is not deployed, has
			// no addresses to protect.
			delete(s.unresolved, host)
		case len(s.resolved[host]) == 0:
			s.unresolved[host] = true
		}
	}
	s.refreshed = time.Now()
	s.current = s.build()
	s.refreshing = nil
}

func (s *protectedAddrs) build() protection {
	addrs := make(map[netip.Addr]bool, len(s.static))
	for addr := range s.static {
		addrs[addr] = true
	}
	for _, resolved := range s.resolved {
		for _, addr := range resolved {
			addrs[normalize(addr)] = true
		}
	}
	return protection{hosts: s.names, addrs: addrs, incomplete: len(s.unresolved) > 0}
}

func isNotFound(err error) bool {
	var dnsErr *net.DNSError
	return errors.As(err, &dnsErr) && dnsErr.IsNotFound
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
