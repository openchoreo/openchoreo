// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package lsp

import (
	"github.com/openchoreo/openchoreo/internal/occ/auth"
	"github.com/openchoreo/openchoreo/internal/occ/cmd/config"
	server "github.com/openchoreo/openchoreo/tools/lsp"
	"github.com/spf13/cobra"
	"log/slog"
	"os"
)

// NewLSPCmd runs the language server in the CLI process.
func NewLSPCmd() *cobra.Command {
	var level, file string
	var stdio bool
	var clientPID int
	cmd := &cobra.Command{
		Use: "lsp", Short: "Run the language server over stdio", Args: cobra.NoArgs,
		Annotations: map[string]string{config.SkipContextBootstrapAnnotation: ""},
		PreRunE:     auth.RequireLogin(),
		RunE: func(cmd *cobra.Command, args []string) error {
			logger, closeFn, err := buildLogger(level, file)
			if err != nil {
				return err
			}
			if closeFn != nil {
				defer closeFn()
			}
			s := server.NewServer()
			s.SetLogger(logger)
			return s.Start()
		},
	}
	cmd.Flags().StringVar(&level, "log-level", "info", "log level: debug, info, warn, error")
	cmd.Flags().StringVar(&file, "log-file", "", "optional log file (default: stderr)")
	cmd.Flags().BoolVar(&stdio, "stdio", false, "accepted for language client compatibility")
	cmd.Flags().IntVar(&clientPID, "clientProcessId", 0, "accepted for language client compatibility")
	return cmd
}

func buildLogger(level, filePath string) (*slog.Logger, func(), error) {
	var levelVar slog.Level
	switch level {
	case "debug":
		levelVar = slog.LevelDebug
	case "warn", "warning":
		levelVar = slog.LevelWarn
	case "error":
		levelVar = slog.LevelError
	default:
		levelVar = slog.LevelInfo
	}

	out := os.Stderr
	var closeFn func()
	if filePath != "" {
		f, err := os.OpenFile(filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
		if err != nil {
			return nil, nil, err
		}
		out = f
		closeFn = func() { _ = f.Close() }
	}

	handler := slog.NewTextHandler(out, &slog.HandlerOptions{Level: levelVar})
	return slog.New(handler), closeFn, nil
}
