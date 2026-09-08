// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

// Package localdevaddresses parses the openchoreo.dev/local-dev-addresses annotation,
// which declares the addresses of a (Cluster)ResourceType that occ remote can tunnel.
package localdevaddresses

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// AnnotationKey is set on a (Cluster)ResourceType and stamped onto its ResourceRelease.
const AnnotationKey = "openchoreo.dev/local-dev-addresses"

// MaxAddresses bounds the addresses one resource type may declare; each costs a tunnel.
const MaxAddresses = 10

const outputPrefix = "outputs."

// entryForm is quoted in parse errors.
const entryForm = "name=outputs.<host>:outputs.<port>"

const maxNameLength = 63

// namePattern excludes "/", which separates target-key segments, and this syntax's own
// separators.
var namePattern = regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9._-]*[a-zA-Z0-9])?$`)

// Declaration is one address: the outputs carrying its two halves.
// Naming outputs is what lets a consumer redirect their env bindings.
type Declaration struct {
	Name       string
	HostOutput string
	PortOutput string
}

// FromAnnotations parses the addresses declared on an object's annotations. An absent
// annotation means nothing to dial.
func FromAnnotations(annotations map[string]string) ([]Declaration, error) {
	return Parse(annotations[AnnotationKey])
}

// Parse reads comma-separated "name=outputs.<host>:outputs.<port>" entries, as in
// "database=outputs.host:outputs.port,adminEndpoint=outputs.host:outputs.adminPort".
// Surrounding whitespace is ignored; anything else malformed is an error rather than a
// dropped address.
func Parse(value string) ([]Declaration, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil
	}

	entries := strings.Split(value, ",")
	if len(entries) > MaxAddresses {
		return nil, fmt.Errorf("declares %d addresses, at most %d are allowed", len(entries), MaxAddresses)
	}

	decls := make([]Declaration, 0, len(entries))
	seen := make(map[string]bool, len(entries))
	// One env var cannot carry a redirected address for two of them.
	portOwner := make(map[string]string, len(entries))

	for _, entry := range entries {
		d, err := parseEntry(entry)
		if err != nil {
			return nil, err
		}
		if seen[d.Name] {
			return nil, fmt.Errorf("address %q is declared more than once", d.Name)
		}
		if owner, taken := portOwner[d.PortOutput]; taken {
			return nil, fmt.Errorf("address %q: port output %q is already used by address %q",
				d.Name, d.PortOutput, owner)
		}
		seen[d.Name] = true
		portOwner[d.PortOutput] = d.Name
		decls = append(decls, d)
	}
	return decls, nil
}

func parseEntry(entry string) (Declaration, error) {
	entry = strings.TrimSpace(entry)
	if entry == "" {
		return Declaration{}, fmt.Errorf("empty address entry: expected %q", entryForm)
	}

	name, address, ok := strings.Cut(entry, "=")
	if !ok {
		return Declaration{}, fmt.Errorf("address %q: expected %q", entry, entryForm)
	}
	name = strings.TrimSpace(name)
	if len(name) > maxNameLength {
		return Declaration{}, fmt.Errorf("address name %q is longer than %d characters", name, maxNameLength)
	}
	if !namePattern.MatchString(name) {
		return Declaration{}, fmt.Errorf("address name %q may contain only alphanumerics, '-', '_' and '.'", name)
	}

	host, port, ok := strings.Cut(address, ":")
	if !ok {
		return Declaration{}, fmt.Errorf("address %q: %q must name both halves as %q",
			name, strings.TrimSpace(address), entryForm)
	}
	hostOutput, err := outputRef(name, host)
	if err != nil {
		return Declaration{}, err
	}
	portOutput, err := outputRef(name, port)
	if err != nil {
		return Declaration{}, err
	}
	return Declaration{Name: name, HostOutput: hostOutput, PortOutput: portOutput}, nil
}

// outputRef resolves an "outputs.<name>" reference to the output name.
func outputRef(address, ref string) (string, error) {
	ref = strings.TrimSpace(ref)
	output, ok := strings.CutPrefix(ref, outputPrefix)
	if !ok || strings.TrimSpace(output) == "" {
		return "", fmt.Errorf("address %q: %q must reference an output as %q",
			address, ref, outputPrefix+"<name>")
	}
	return strings.TrimSpace(output), nil
}

// Output is one already-resolved ResourceType output: its plain value, or a marker that
// the value stays on the data plane behind a Secret/ConfigMap reference.
type Output struct {
	Value     string
	Reference bool
}

// Address is one resolved address. Host and Port are zero when Reason says why there is
// nothing to dial.
type Address struct {
	Name       string
	HostOutput string
	PortOutput string
	Host       string
	Port       int32
	Reason     string
}

// Resolve turns declarations into addresses using the outputs they name. A declaration
// that cannot produce a dialable address comes back with Reason set rather than being
// dropped, so a caller can report it against the address it belongs to.
func Resolve(decls []Declaration, outputs map[string]Output) []Address {
	addrs := make([]Address, 0, len(decls))
	for i := range decls {
		d := &decls[i]
		a := Address{Name: d.Name, HostOutput: d.HostOutput, PortOutput: d.PortOutput}

		host, hostOK := outputs[d.HostOutput]
		port, portOK := outputs[d.PortOutput]
		switch {
		case !hostOK:
			a.Reason = fmt.Sprintf("host output %q is not declared by the resource type", d.HostOutput)
		case !portOK:
			a.Reason = fmt.Sprintf("port output %q is not declared by the resource type", d.PortOutput)
		case host.Reference || port.Reference:
			a.Reason = "address is published only through a Secret or ConfigMap reference, which the control plane does not read"
		case host.Value == "":
			a.Reason = fmt.Sprintf("output %q has no value yet; retry once the resource reports it", d.HostOutput)
		case port.Value == "":
			a.Reason = fmt.Sprintf("output %q has no value yet; retry once the resource reports it", d.PortOutput)
		default:
			n, err := parsePort(port.Value)
			if err != nil {
				a.Reason = fmt.Sprintf("output %q: %v", d.PortOutput, err)
			} else {
				a.Host, a.Port = host.Value, n
			}
		}
		addrs = append(addrs, a)
	}
	return addrs
}

func parsePort(s string) (int32, error) {
	n, err := strconv.ParseInt(s, 10, 32)
	if err != nil {
		return 0, fmt.Errorf("port %q is not numeric", s)
	}
	if n < 1 || n > 65535 {
		return 0, fmt.Errorf("port %d is out of range", n)
	}
	return int32(n), nil
}
