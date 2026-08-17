// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

// Package depconnect defines the wire contract shared by the `occ local` client, the
// control plane (openchoreo-api), and the per-project+env dep-agent: the resolve
// request/response (resolve.go), the CP-signed capability that scopes a session
// (capability.go), the occ<->agent tunnel protocol and client (protocol.go,
// client.go), and the agent->CP authorize callback (authorize.go).
//
// Transport model:
//  1. occ resolves a workload's dependencies against the control plane, which
//     resolves each target, provisions a dedicated dep-agent (Deployment + L4
//     Service) into the data plane's project+env namespace, and returns the targets,
//     a signed capability, and the dep-agent's L4 endpoint.
//  2. occ dials the dep-agent endpoint directly over TLS, presents the capability in
//     a Hello handshake, and multiplexes one yamux stream per accepted local
//     connection. The byte path does not traverse the control plane.
//  3. For each stream the dep-agent calls the control plane's authorize endpoint with
//     the capability and target key; the control plane verifies the capability and
//     returns the concrete host:port, which the agent dials and pipes.
package depconnect
