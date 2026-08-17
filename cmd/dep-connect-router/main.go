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
	depconnectrouter "github.com/openchoreo/openchoreo/internal/dep-connect-router"
	"github.com/openchoreo/openchoreo/internal/dep-connect-router/config"
	"github.com/openchoreo/openchoreo/internal/logging"
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
	logger.Info("Starting", append(version.GetLogKeyValues(), "listen", cfg.Router.ListenAddr)...)

	routerService, err := depconnectrouter.New(cfg.ToRouterConfig(), logger)
	if err != nil {
		logger.Error("Failed to initialize dep-connect SNI router", "error", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := routerService.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
		logger.Error("dep-connect SNI router failed", "error", err)
		os.Exit(1)
	}
	logger.Info("dep-connect SNI router shutdown completed")
}

func setupFlags() (*pflag.FlagSet, *cliFlags) {
	flags := pflag.NewFlagSet("dep-connect-router", pflag.ExitOnError)
	cli := &cliFlags{}

	routerDefaults := config.RouterDefaults()
	loggingDefaults := config.LoggingDefaults()

	// Config flags - values loaded for configurations
	flags.String("listen", routerDefaults.ListenAddr, "L4 listen address for the SNI router")
	flags.String("label-selector", routerDefaults.LabelSelector,
		"Label selector identifying dep-agent Services to route to")
	flags.String("sni-annotation", routerDefaults.SNIAnnotationKey,
		"Service annotation holding each dep-agent's SNI host")
	flags.Duration("refresh-interval", routerDefaults.RefreshInterval,
		"How often to refresh the dep-agent backend map")
	flags.Duration("dial-timeout", routerDefaults.DialTimeout, "Timeout for dialing a dep-agent backend")
	flags.String("log-level", loggingDefaults.Level, "Log level (debug, info, warn, error)")
	flags.String("log-format", loggingDefaults.Format, "Log output format (json, text)")

	// Direct flags - bound to variables for immediate use
	flags.StringVar(&cli.configPath, "config", "", "Path to config file")
	flags.BoolVar(&cli.dumpConfig, "dump-config", false, "Print loaded configuration and exit")

	return flags, cli
}
