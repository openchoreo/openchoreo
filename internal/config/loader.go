// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

// Package config provides a unified configuration loader for OpenChoreo components.
package config

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	koanfyaml "github.com/knadh/koanf/parsers/yaml"
	"github.com/knadh/koanf/providers/env"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/providers/structs"
	"github.com/knadh/koanf/v2"
	"github.com/spf13/pflag"
	"gopkg.in/yaml.v3"
)

// Loader handles configuration loading from multiple sources.
type Loader struct {
	k         *koanf.Koanf
	envPrefix string
}

// Validator can be implemented by config structs to enable validation.
type Validator interface {
	Validate() error
}

// NewLoader creates a new configuration loader.
// envPrefix should be like "OC_API" (without trailing delimiter).
// Environment variables use double underscore (__) for nesting:
// OC_API__SERVER__PORT -> server.port
func NewLoader(envPrefix string) *Loader {
	return &Loader{
		k:         koanf.New("."),
		envPrefix: envPrefix + "__",
	}
}

// LoadWithDefaults loads configuration with the following priority (highest to lowest):
//  1. Environment variables (OC_API__SERVER__PORT -> server.port)
//  2. Config file (YAML)
//  3. Struct defaults
//
// If configPath is specified but the file does not exist, an error is returned.
// If configPath is empty, only defaults and environment variables are used.
func (l *Loader) LoadWithDefaults(defaults any, configPath string) error {
	if defaults != nil {
		if err := l.k.Load(structs.Provider(defaults, "koanf"), nil); err != nil {
			return fmt.Errorf("failed to load defaults: %w", err)
		}
	}

	if configPath != "" {
		if _, err := os.Stat(configPath); err != nil {
			return fmt.Errorf("config file not found: %s", configPath)
		}
		if err := l.k.Load(file.Provider(configPath), koanfyaml.Parser()); err != nil {
			return fmt.Errorf("failed to load config file: %w", err)
		}
	}

	// Double underscore (__) for nesting: OC_API__SERVER__PORT -> server.port
	envProvider := env.Provider(l.envPrefix, ".", func(s string) string {
		key := strings.ToLower(strings.TrimPrefix(s, l.envPrefix))
		key = strings.ReplaceAll(key, "__", ".")
		return key
	})
	if err := l.k.Load(envProvider, nil); err != nil {
		return fmt.Errorf("failed to load environment variables: %w", err)
	}

	return nil
}

// LoadFlags applies CLI flag overrides using explicit mappings.
// Only flags that were explicitly set by the user are applied.
// Call this after LoadWithDefaults for highest priority overrides.
func (l *Loader) LoadFlags(flags *pflag.FlagSet, mappings map[string]string) error {
	var errs []error
	flags.Visit(func(f *pflag.Flag) {
		if key, ok := mappings[f.Name]; ok {
			if err := l.k.Set(key, f.Value.String()); err != nil {
				errs = append(errs, fmt.Errorf("flag %s: %w", f.Name, err))
			}
		}
	})
	return errors.Join(errs...)
}

// Unmarshal unmarshals the loaded configuration into the provided struct.
func (l *Loader) Unmarshal(path string, out any) error {
	return l.k.Unmarshal(path, out)
}

// UnmarshalAndValidate unmarshals the configuration and validates it.
// If out implements Validator, Validate() is called after unmarshaling.
func (l *Loader) UnmarshalAndValidate(path string, out any) error {
	if err := l.k.Unmarshal(path, out); err != nil {
		return err
	}
	if v, ok := out.(Validator); ok {
		return v.Validate()
	}
	return nil
}

// Set manually sets a configuration value.
func (l *Loader) Set(key string, value any) error {
	return l.k.Set(key, value)
}

// Raw returns all loaded configuration as a nested map.
func (l *Loader) Raw() map[string]any {
	return l.k.Raw()
}

// DumpYAML writes the loaded configuration as YAML to the provided writer.
func (l *Loader) DumpYAML(w io.Writer) error {
	return yaml.NewEncoder(w).Encode(l.k.Raw())
}
