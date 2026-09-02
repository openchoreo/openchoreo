// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package remote

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/openchoreo/openchoreo/internal/remoteconnect"
)

// secretFileMode is the mode of a materialized file binding, and secretDirMode of the
// directory holding them: owner-only, since the content is the same credential the
// cluster keeps in a Secret.
const (
	secretFileMode = 0o600
	secretDirMode  = 0o700
)

// fileStore materializes fetched file bindings under one session-scoped directory. The
// directory lives outside the project working tree so a credential can never be
// committed by accident, and is removed when the session ends.
type fileStore struct {
	root string
	// paths maps the in-cluster mount path to the local file standing in for it, so the
	// report and any address substitution can name both.
	paths map[string]string
}

// newFileStore creates the session directory lazily — a session with no file bindings
// should leave nothing behind on disk.
func newFileStore() *fileStore { return &fileStore{paths: map[string]string{}} }

// write stores one fetched value for mountPath and returns the local path. The local name
// is derived from the whole mount path, so two bindings differing only in directory keep
// distinct files.
func (f *fileStore) write(mountPath string, value []byte) (string, error) {
	if f.root == "" {
		dir, err := os.MkdirTemp("", "occ-local-")
		if err != nil {
			return "", fmt.Errorf("create session directory: %w", err)
		}
		if err := os.Chmod(dir, secretDirMode); err != nil {
			return "", fmt.Errorf("restrict session directory: %w", err)
		}
		f.root = dir
	}
	local := filepath.Join(f.root, localFileName(mountPath))
	if err := os.WriteFile(local, value, secretFileMode); err != nil {
		return "", fmt.Errorf("write %s: %w", local, err)
	}
	f.paths[mountPath] = local
	return local, nil
}

// cleanup removes the session directory and everything in it. Called on every exit path,
// including signal-driven ones, so fetched credentials do not outlive the session.
func (f *fileStore) cleanup() {
	if f.root == "" {
		return
	}
	_ = os.RemoveAll(f.root)
	f.root = ""
}

// localFileName flattens a container mount path into a single filename. Separators
// become underscores rather than nested directories: the app is pointed at the new path
// explicitly, so the layout carries no meaning, and a flat directory cannot be escaped
// by a path that starts with "..".
//
// A digest of the mount path is appended because flattening is not injective:
// "/etc/tls_certs/ca.pem" and "/etc/tls/certs/ca.pem" flatten alike.
func localFileName(mountPath string) string {
	cleaned := strings.TrimLeft(filepath.ToSlash(mountPath), "/")
	name := strings.ReplaceAll(cleaned, "/", "_")
	// A mount path is a Kubernetes path and cannot be empty, but a caller-supplied one
	// could be; never return "" and write to the directory itself.
	if name == "" || name == "." || name == ".." {
		name = "binding"
	}
	sum := sha256.Sum256([]byte(mountPath))
	return name + "-" + hex.EncodeToString(sum[:4])
}

// fetchBindings resolves every fetch key a resolve produced into the local process's
// environment and, for file bindings, into files under store.
//
// Values are handled as bytes and placed only in env or a session file. Nothing here
// prints, logs, or errors with a value: occ's own output goes to the developer's
// terminal and scrollback, which is not a place a credential belongs.
//
// A failure is per binding. One denied or missing value leaves that variable unset and
// reported, and the rest of the session proceeds — a session that dies because one
// optional credential could not be read would be worse than one that names the gap.
// sensitive collects the env var names whose values came from a fetch, so --print-env
// can redact them. Without it, printing the resolved environment would defeat the point
// of never routing these values through the control plane.
// localAddrs carries, per resource, the in-cluster address each endpoint resolved to and
// the local listener now standing in for it. Fetched values need the same substitution
// StaticEnv gets: a resource may publish its connection URL through a Secret, and that
// URL embeds the cluster address. Before this feature such a binding was omitted with a
// warning; resolving it without re-pointing would instead hand the app an address the
// developer's machine cannot reach — silently, which is the worse failure.
func fetchBindings(
	overrides map[string]string,
	sensitive map[string]bool,
	out io.Writer,
	resp *remoteconnect.ResolveResponse,
	agentTunnels map[string]tunnel,
	store *fileStore,
	localAddrs map[string][]addrSwap,
	noSecrets bool,
) {
	for _, rb := range resp.Resources {
		refKey := remoteconnect.ResourceRefKey(rb.Ref)

		if noSecrets {
			for _, envVar := range slices.Sorted(maps.Keys(rb.FetchEnv)) {
				fmt.Fprintf(out, "  ! %s: %s not resolved (--no-secrets)\n", refKey, envVar)
			}
			for _, mountPath := range slices.Sorted(maps.Keys(rb.FetchFile)) {
				fmt.Fprintf(out, "  ! %s: %s not provisioned (--no-secrets)\n", refKey, mountPath)
			}
			continue
		}

		// Collected first, then re-pointed as a batch, so rewriteAddrs sees the same
		// shape it does for StaticEnv. Sorted so the report is stable across runs.
		fetched := make(map[string]string, len(rb.FetchEnv))
		for _, envVar := range slices.Sorted(maps.Keys(rb.FetchEnv)) {
			value, err := fetchOne(agentTunnels, rb.FetchAgentID, rb.FetchEnv[envVar])
			if err != nil {
				fmt.Fprintf(out, "  ! %s: %s not resolved (%v)\n", refKey, envVar, err)
				continue
			}
			// An env var carries text. A value that is not valid UTF-8 (a keystore, a DER
			// certificate) cannot round-trip through the environment, and silently
			// passing mangled bytes would surface as an obscure failure inside the app.
			if !isEnvSafe(value) {
				fmt.Fprintf(out, "  ! %s: %s not resolved (value is binary; bind it as a file instead)\n",
					refKey, envVar)
				continue
			}
			fetched[envVar] = string(value)
			fmt.Fprintf(out, "  %-28s %s resolved (value hidden)\n", refKey, envVar)
		}

		// rewriteAddrs reports by key name only and never prints a value, so it is safe
		// to run over fetched material.
		for envVar, value := range rewriteAddrs(fetched, localAddrs[rb.Ref], refKey, out) {
			// Not mergeOverrides: that reports a clash by printing both values, which
			// would put a credential in the terminal. Report the clash by name only.
			if existing, clash := overrides[envVar]; clash && existing != value {
				fmt.Fprintf(out, "  ! warning: %s set by multiple workloads; using the fetched value\n", envVar)
			}
			overrides[envVar] = value
			sensitive[envVar] = true
		}

		// File contents are written through byte-for-byte, with no address substitution.
		// A mounted value is as likely to be a certificate, a keystore, or a binary blob
		// as a config file, and rewriting bytes inside one would corrupt it. If a file
		// embeds a cluster address, that is the developer's to handle.
		for _, mountPath := range slices.Sorted(maps.Keys(rb.FetchFile)) {
			value, err := fetchOne(agentTunnels, rb.FetchAgentID, rb.FetchFile[mountPath])
			if err != nil {
				fmt.Fprintf(out, "  ! %s: %s not provisioned (%v)\n", refKey, mountPath, err)
				continue
			}
			local, werr := store.write(mountPath, value)
			if werr != nil {
				fmt.Fprintf(out, "  ! %s: %s not provisioned (%v)\n", refKey, mountPath, werr)
				continue
			}
			fmt.Fprintf(out, "  %-28s %s -> %s\n", refKey, mountPath, local)
		}
	}
}

// fetchOne routes a fetch key to the agent the resolve designated for it. The key's
// agent is found through the response's targets/agents rather than assumed, so a
// multi-agent session fetches each value from the namespace that owns it.
func fetchOne(agentTunnels map[string]tunnel, agentID, key string) ([]byte, error) {
	if key == "" {
		return nil, fmt.Errorf("no fetch key")
	}
	tn, err := tunnelForAgent(agentTunnels, agentID)
	if err != nil {
		return nil, err
	}
	return tn.Fetch(key)
}

// tunnelForAgent returns the tunnel to the agent holding a resource's fetch values.
func tunnelForAgent(agentTunnels map[string]tunnel, agentID string) (tunnel, error) {
	if agentID == "" {
		return nil, fmt.Errorf("resolve named no remote-agent for this resource's values")
	}
	tn, ok := agentTunnels[agentID]
	if !ok {
		return nil, fmt.Errorf("no tunnel open to remote-agent %q", agentID)
	}
	return tn, nil
}

// repointFilePaths redirects env vars that name a materialized mount path at the local
// file standing in for it, so an app told where to read a certificate through its
// environment finds one.
//
// Only an exact whole-value match is rewritten, never a substring. A mount path is a
// short, common-looking string ("/etc/certs/ca.pem"), and rewriting it inside a larger
// value — a command line, a config blob, a comma-separated list — risks corrupting
// something that merely mentioned the path. Anything the exact rule cannot reach is
// reported instead, so the developer points the app at the file themselves rather than
// silently getting the cluster's path.
func repointFilePaths(overrides map[string]string, out io.Writer, store *fileStore) {
	if len(store.paths) == 0 {
		return
	}
	repointed := make(map[string]bool, len(store.paths))
	for _, envVar := range slices.Sorted(maps.Keys(overrides)) {
		local, ok := store.paths[overrides[envVar]]
		if !ok {
			continue
		}
		fmt.Fprintf(out, "  %-28s -> %s\n", envVar, local)
		repointed[overrides[envVar]] = true
		overrides[envVar] = local
	}
	for _, mountPath := range slices.Sorted(maps.Keys(store.paths)) {
		if repointed[mountPath] {
			continue
		}
		fmt.Fprintf(out, "  ! no env var names %s; point the app at %s yourself\n",
			mountPath, store.paths[mountPath])
	}
}

// isEnvSafe reports whether value can be carried in an environment variable: valid UTF-8
// with no NUL, which terminates a C string and would truncate the value.
func isEnvSafe(value []byte) bool {
	if !utf8.Valid(value) {
		return false
	}
	for _, b := range value {
		if b == 0 {
			return false
		}
	}
	return true
}
