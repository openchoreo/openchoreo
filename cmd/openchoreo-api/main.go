// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/openchoreo/openchoreo/internal/authz"
	"github.com/openchoreo/openchoreo/internal/authz/usertype"
	kubernetesClient "github.com/openchoreo/openchoreo/internal/clients/kubernetes"
	k8s "github.com/openchoreo/openchoreo/internal/openchoreo-api/clients"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/handlers"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
)

var (
	port = flag.Int("port", 8080, "port http server runs on")
)

// OpenChoreoAPIConfig represents the top-level configuration structure
type OpenChoreoAPIConfig struct {
	Authz AuthzAPIConfig `yaml:"authz"`
}

// AuthzAPIConfig represents the authorization configuration section
type AuthzAPIConfig struct {
	UserTypes []usertype.UserTypeConfig `yaml:"user_types"`
}

func main() {
	flag.Parse()

	slogHandler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})
	baseLogger := slog.New(slogHandler)
	slog.SetDefault(baseLogger)

	// Create shutdown context
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	k8sClient, err := k8s.NewK8sClient()
	if err != nil {
		baseLogger.Error("Failed to initialize Kubernetes client", slog.Any("error", err))
		os.Exit(1)
	}

	// Load configuration
	configPath := os.Getenv("OPENCHOREO_API_CONFIG_PATH")
	if configPath == "" {
		configPath = "/etc/openchoreo/config.yaml"
	}

	config, err := loadConfig(configPath)
	if err != nil {
		baseLogger.Error("Failed to load configuration file",
			slog.String("config_path", configPath),
			slog.Any("error", err))
		os.Exit(1)
	}

	baseLogger.Info("Loaded configuration from file",
		slog.String("config_path", configPath),
		slog.Int("user_types_count", len(config.Authz.UserTypes)))

	// Initialize authorization
	authzConfig := authz.AuthZConfig{
		Enabled:              os.Getenv("AUTHZ_ENABLED") == "true",
		DatabasePath:         os.Getenv("AUTHZ_DATABASE_PATH"),
		DefaultRolesFilePath: os.Getenv("AUTHZ_DEFAULT_ROLES_FILE_PATH"),
		UserTypeConfigs:      config.Authz.UserTypes,
		EnableCache:          false,
	}
	pap, pdp, err := authz.Initialize(authzConfig, baseLogger.With("component", "authz"))
	if err != nil {
		baseLogger.Error("Failed to initialize authorization", slog.Any("error", err))
		os.Exit(1)
	}

	// Initialize services with PAP and PDP
	services := services.NewServices(k8sClient, kubernetesClient.NewManager(), pap, pdp, baseLogger)

	// Initialize HTTP handlers
	handler := handlers.New(services, baseLogger.With("component", "handlers"))

	srv := &http.Server{
		Addr:         ":" + strconv.Itoa(*port),
		Handler:      handler.Routes(),
		ReadTimeout:  15 * time.Second, // TODO: Make these configurable
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Start server
	go func() {
		baseLogger.Info("OpenChoreo API server listening on", slog.String("address", srv.Addr))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			baseLogger.Error("Server error", slog.Any("error", err))
			os.Exit(1)
		}
	}()

	// Wait for shutdown signal
	<-ctx.Done()

	// Graceful shutdown
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		baseLogger.Error("Server shutdown error", slog.Any("error", err))
	}

	// Close authorization database connection
	if casbinEnforcer, ok := pap.(interface{ Close() error }); ok {
		if err := casbinEnforcer.Close(); err != nil {
			baseLogger.Error("Failed to close authorization database", slog.Any("error", err))
		}
	}

	baseLogger.Info("Server stopped gracefully")
}

// loadConfig loads and validates the configuration from the specified file path
func loadConfig(filePath string) (*OpenChoreoAPIConfig, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var config OpenChoreoAPIConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	if err := usertype.ValidateConfig(config.Authz.UserTypes); err != nil {
		return nil, fmt.Errorf("invalid user type config: %w", err)
	}

	usertype.SortByPriority(config.Authz.UserTypes)
	return &config, nil
}
