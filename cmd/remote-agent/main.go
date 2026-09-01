// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"errors"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/pflag"

	coreconfig "github.com/openchoreo/openchoreo/internal/config"
	"github.com/openchoreo/openchoreo/internal/logging"
	remoteagent "github.com/openchoreo/openchoreo/internal/remote-agent"
	"github.com/openchoreo/openchoreo/internal/remote-agent/config"
	"github.com/openchoreo/openchoreo/internal/version"
)

// cliFlags holds flags consumed directly rather than through the config loader.
type cliFlags struct {
	configPath string
	dumpConfig bool
}

func main() {
	flags, cli := setupFlags()
	_ = flags.Parse(os.Args[1:]) // ExitOnError mode handles parse errors

	// Bootstrap logger for pre-configuration errors
	bootLogger := logging.Bootstrap(version.Get().Name)

	// Load unified configuration
	loader, err := config.NewLoader(cli.configPath, flags)
	if err != nil {
		bootLogger.Error("Failed to load configuration", "error", err)
		os.Exit(1)
	}

	// Print merged config and exit
	if cli.dumpConfig {
		if err := loader.DumpYAML(os.Stdout); err != nil {
			bootLogger.Error("Failed to dump configuration", "error", err)
			os.Exit(1)
		}
		os.Exit(0)
	}

	// Unmarshal and validate configuration
	var cfg config.Config
	if err := loader.Unmarshal("", &cfg); err != nil {
		bootLogger.Error("Failed to unmarshal configuration", "error", err)
		os.Exit(1)
	}
	if err := cfg.Validate(); err != nil {
		var validationErrs coreconfig.ValidationErrors
		if errors.As(err, &validationErrs) {
			for _, e := range validationErrs {
				bootLogger.Error("Invalid configuration", "field", e.Field, "message", e.Message)
			}
		} else {
			bootLogger.Error("Invalid configuration", "error", err)
		}
		os.Exit(1)
	}

	// Set up runtime logger from configuration
	logger := logging.NewWithComponent(cfg.Logging.ToLoggingConfig(), version.Get().Name)

	// Log startup with version info
	logger.Info("Starting", append(version.GetLogKeyValues(),
		"listen", cfg.Agent.ListenAddr,
		"authorizeURL", cfg.Authorize.URL,
		"tlsCert", cfg.Agent.TLSCertPath)...)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	srv, err := remoteagent.New(cfg.ToAgentConfig(), logger)
	if err != nil {
		logger.Error("Failed to initialize remote-agent", "error", err)
		os.Exit(1)
	}
	if err := srv.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
		logger.Error("remote-agent failed", "error", err)
		os.Exit(1)
	}
	logger.Info("remote-agent shutdown completed")
}

func setupFlags() (*pflag.FlagSet, *cliFlags) {
	flags := pflag.NewFlagSet("remote-agent", pflag.ExitOnError)
	cli := &cliFlags{}

	agentDefaults := config.AgentDefaults()
	authorizeDefaults := config.AuthorizeDefaults()
	heartbeatDefaults := config.HeartbeatDefaults()
	loggingDefaults := config.LoggingDefaults()

	// Config flags - values loaded for configurations
	flags.String("listen", agentDefaults.ListenAddr, "TLS tunnel listen address")
	flags.String("tls-cert", agentDefaults.TLSCertPath, "Path to the server TLS certificate")
	flags.String("tls-key", agentDefaults.TLSKeyPath, "Path to the server TLS private key")
	flags.String("namespace", agentDefaults.Namespace,
		"The agent's own data-plane namespace, sent in heartbeats (defaults to the POD_NAMESPACE downward-API env)")
	flags.Int("max-streams-per-session", agentDefaults.MaxStreamsPerSession,
		"Maximum concurrent streams per tunnel connection (0 = unlimited)")
	flags.Duration("handshake-timeout", agentDefaults.HandshakeTimeout, "Timeout for the Hello handshake")
	flags.Duration("stream-open-timeout", agentDefaults.StreamOpenTimeout,
		"Timeout for a client to send StreamOpen after opening a stream")
	flags.Duration("dial-timeout", agentDefaults.DialTimeout, "Timeout for dialing an upstream dependency target")
	flags.String("authorize-url", authorizeDefaults.URL,
		"Control-plane authorize endpoint URL (POST) called to authorize each stream")
	flags.String("authorize-ca", authorizeDefaults.CABundlePath,
		"Path to the PEM CA bundle pinned when calling the control plane (empty = system roots)")
	flags.Bool("authorize-insecure", authorizeDefaults.InsecureSkipVerify,
		"Skip TLS verification of the control plane (development only)")
	flags.Duration("authorize-timeout", authorizeDefaults.Timeout,
		"Timeout for a single authorize call to the control plane")
	flags.String("heartbeat-url", heartbeatDefaults.URL,
		"Control-plane heartbeat endpoint URL (POST), called periodically to keep this "+
			"agent alive while it has live sessions (empty disables)")
	flags.Duration("heartbeat-interval", heartbeatDefaults.Interval,
		"How often to refresh liveness while the agent has live sessions")
	flags.String("log-level", loggingDefaults.Level, "Log level (debug, info, warn, error)")
	flags.String("log-format", loggingDefaults.Format, "Log output format (json, text)")

	// Direct flags - bound to variables for immediate use
	flags.StringVar(&cli.configPath, "config", "", "Path to config file")
	flags.BoolVar(&cli.dumpConfig, "dump-config", false, "Print loaded configuration and exit")

	return flags, cli
}
