// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

// Package depconnectrouter implements the OpenChoreo dep-connect SNI router: a single
// per-data-plane L4 entrypoint that terminates nothing. It peeks the TLS ClientHello
// SNI of each incoming connection, looks up the matching per-project+env dep-agent,
// and splices the raw (still-encrypted) bytes through — so occ's TLS session and
// capability handshake terminate at the agent, not here. This removes the need for a
// LoadBalancer/NodePort per agent.
package depconnectrouter

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"time"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

// Config configures the SNI router.
type Config struct {
	ListenAddr       string
	LabelSelector    string
	SNIAnnotationKey string
	RefreshInterval  time.Duration
	DialTimeout      time.Duration
}

// Defaults.
const (
	DefaultLabelSelector    = "app.kubernetes.io/managed-by=openchoreo-api-depconnect"
	DefaultSNIAnnotationKey = "openchoreo.dev/depconnect-sni"
	DefaultRefreshInterval  = 5 * time.Second
	DefaultDialTimeout      = 10 * time.Second
)

// Router is the SNI-passthrough entrypoint.
type Router struct {
	cfg              Config
	reg              *registry
	log              *slog.Logger
	dialer           func(ctx context.Context, network, addr string) (net.Conn, error)
	handshakeTimeout time.Duration
}

// New builds a Router with an in-cluster Kubernetes client for agent discovery.
func New(cfg Config, log *slog.Logger) (*Router, error) {
	restCfg, err := rest.InClusterConfig()
	if err != nil {
		return nil, fmt.Errorf("depconnectrouter: in-cluster config: %w", err)
	}
	clientset, err := kubernetes.NewForConfig(restCfg)
	if err != nil {
		return nil, fmt.Errorf("depconnectrouter: build client: %w", err)
	}
	return newRouter(cfg, newRegistry(clientset, cfg.LabelSelector, cfg.SNIAnnotationKey, log), log), nil
}

func newRouter(cfg Config, reg *registry, log *slog.Logger) *Router {
	if cfg.RefreshInterval == 0 {
		cfg.RefreshInterval = DefaultRefreshInterval
	}
	if cfg.DialTimeout == 0 {
		cfg.DialTimeout = DefaultDialTimeout
	}
	return &Router{
		cfg:              cfg,
		reg:              reg,
		log:              log,
		dialer:           (&net.Dialer{}).DialContext,
		handshakeTimeout: 10 * time.Second,
	}
}

// Run starts the discovery loop, listens on the configured address, and serves until
// ctx is done.
func (r *Router) Run(ctx context.Context) error {
	go r.reg.Start(ctx, r.cfg.RefreshInterval)

	ln, err := net.Listen("tcp", r.cfg.ListenAddr)
	if err != nil {
		return fmt.Errorf("depconnectrouter: listen on %s: %w", r.cfg.ListenAddr, err)
	}
	r.log.Info("dep-connect SNI router listening", "addr", ln.Addr().String())
	return r.Serve(ctx, ln)
}

// Serve accepts connections on ln and routes each by SNI until ctx is done.
func (r *Router) Serve(ctx context.Context, ln net.Listener) error {
	go func() { <-ctx.Done(); _ = ln.Close() }()

	for {
		conn, err := ln.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return fmt.Errorf("depconnectrouter: accept: %w", err)
		}
		go r.handleConn(ctx, conn)
	}
}

// handleConn peeks the SNI, routes to the matching agent, and splices bytes.
func (r *Router) handleConn(ctx context.Context, client net.Conn) {
	defer client.Close()

	_ = client.SetReadDeadline(time.Now().Add(r.handshakeTimeout))
	sni, raw, err := readClientHelloSNI(client)
	if err != nil {
		r.log.Debug("could not read SNI; dropping", "error", err)
		return
	}
	_ = client.SetReadDeadline(time.Time{})

	backend, ok := r.reg.lookup(ctx, sni)
	if !ok {
		r.log.Warn("no dep-agent for SNI; dropping", "sni", sni)
		return
	}

	dialCtx, cancel := context.WithTimeout(ctx, r.cfg.DialTimeout)
	upstream, err := r.dialer(dialCtx, "tcp", backend)
	cancel()
	if err != nil {
		r.log.Warn("dial dep-agent failed", "sni", sni, "backend", backend, "error", err)
		return
	}
	defer upstream.Close()

	// Replay the ClientHello we consumed, then splice the rest untouched.
	if _, err := upstream.Write(raw); err != nil {
		r.log.Debug("replay ClientHello failed", "sni", sni, "error", err)
		return
	}
	r.log.Debug("routing connection", "sni", sni, "backend", backend)
	splice(client, upstream)
}

// splice copies bytes both ways until either side ends, then closes both.
func splice(a, b net.Conn) {
	done := make(chan struct{}, 2)
	cp := func(dst, src net.Conn) {
		_, _ = io.Copy(dst, src)
		_ = dst.Close()
		_ = src.Close()
		done <- struct{}{}
	}
	go cp(a, b)
	go cp(b, a)
	<-done
	<-done
}
