// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/openchoreo/openchoreo/internal/cmdutil"
	depagent "github.com/openchoreo/openchoreo/internal/dep-agent"
)

func main() {
	var (
		listenAddr        string
		tlsCertPath       string
		tlsKeyPath        string
		authorizeURL      string
		authorizeCA       string
		authorizeInsecure bool
		heartbeatURL      string
		heartbeatInterval time.Duration
		namespace         string
		handshakeTimeout  time.Duration
		streamOpenTimeout time.Duration
		authorizeTimeout  time.Duration
		dialTimeout       time.Duration
		maxStreams        int
		logLevel          string
	)

	flag.StringVar(&listenAddr, "listen", cmdutil.GetEnv("LISTEN_ADDR", ":8443"),
		"TLS tunnel listen address")
	flag.StringVar(&tlsCertPath, "tls-cert", cmdutil.GetEnv("TLS_CERT_PATH", "/certs/tls.crt"),
		"Path to the server TLS certificate")
	flag.StringVar(&tlsKeyPath, "tls-key", cmdutil.GetEnv("TLS_KEY_PATH", "/certs/tls.key"),
		"Path to the server TLS private key")
	flag.StringVar(&authorizeURL, "authorize-url", cmdutil.GetEnv("AUTHORIZE_URL", ""),
		"Control-plane authorize endpoint URL (POST) called to authorize each stream")
	flag.StringVar(&authorizeCA, "authorize-ca", cmdutil.GetEnv("AUTHORIZE_CA_PATH", ""),
		"Path to the PEM CA bundle pinned when calling the control plane (empty = system roots)")
	flag.BoolVar(&authorizeInsecure, "authorize-insecure",
		cmdutil.GetEnvBool("AUTHORIZE_INSECURE", false),
		"Skip TLS verification of the control plane (development only)")
	flag.StringVar(&heartbeatURL, "heartbeat-url", cmdutil.GetEnv("HEARTBEAT_URL", ""),
		"Control-plane heartbeat endpoint URL (POST), called periodically to keep this "+
			"agent alive while it has live sessions (empty disables)")
	flag.DurationVar(&heartbeatInterval, "heartbeat-interval", depagent.DefaultHeartbeatInterval,
		"How often to refresh liveness while the agent has live sessions")
	flag.StringVar(&namespace, "namespace", cmdutil.GetEnv("POD_NAMESPACE", ""),
		"The agent's own data-plane namespace, sent in heartbeats (defaults to the POD_NAMESPACE downward-API env)")
	flag.DurationVar(&handshakeTimeout, "handshake-timeout", depagent.DefaultHandshakeTimeout,
		"Timeout for the Hello handshake")
	flag.DurationVar(&streamOpenTimeout, "stream-open-timeout", depagent.DefaultStreamOpenTimeout,
		"Timeout for a client to send StreamOpen after opening a stream")
	flag.DurationVar(&authorizeTimeout, "authorize-timeout", depagent.DefaultAuthorizeTimeout,
		"Timeout for a single authorize call to the control plane")
	flag.DurationVar(&dialTimeout, "dial-timeout", depagent.DefaultDialTimeout,
		"Timeout for dialing an upstream dependency target")
	flag.IntVar(&maxStreams, "max-streams-per-session",
		cmdutil.GetEnvInt("MAX_STREAMS_PER_SESSION", 256),
		"Maximum concurrent streams per tunnel connection (0 = unlimited)")
	flag.StringVar(&logLevel, "log-level", cmdutil.GetEnv("LOG_LEVEL", "info"),
		"Log level (debug, info, warn, error)")
	flag.Parse()

	if authorizeURL == "" {
		fmt.Println("Error: --authorize-url is required")
		flag.Usage()
		os.Exit(1)
	}

	logger := cmdutil.SetupLogger(logLevel)

	cfg := depagent.Config{
		ListenAddr:                  listenAddr,
		TLSCertPath:                 tlsCertPath,
		TLSKeyPath:                  tlsKeyPath,
		AuthorizeURL:                authorizeURL,
		AuthorizeCABundlePath:       authorizeCA,
		AuthorizeInsecureSkipVerify: authorizeInsecure,
		HeartbeatURL:                heartbeatURL,
		HeartbeatInterval:           heartbeatInterval,
		Namespace:                   namespace,
		HandshakeTimeout:            handshakeTimeout,
		StreamOpenTimeout:           streamOpenTimeout,
		AuthorizeTimeout:            authorizeTimeout,
		DialTimeout:                 dialTimeout,
		MaxStreamsPerSession:        maxStreams,
	}

	logger.Info("starting OpenChoreo dev-tunnel agent",
		"listen", listenAddr, "authorizeURL", authorizeURL, "tlsCert", tlsCertPath)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	srv, err := depagent.New(cfg, logger)
	if err != nil {
		logger.Error("failed to initialize dep-agent", "error", err)
		os.Exit(1)
	}
	if err := srv.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
		logger.Error("dep-agent failed", "error", err)
		os.Exit(1)
	}
	logger.Info("dep-agent shutdown completed")
}
