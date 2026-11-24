// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openchoreo/openchoreo/internal/observer/config"
)

func storeConfig(dsn string) *config.Config {
	cfg := &config.Config{}
	cfg.DeliveryInsights.Enabled = true
	cfg.DeliveryInsights.StoreBackend = "sqlite"
	cfg.DeliveryInsights.StoreDSN = dsn
	return cfg
}

func capturingLogger() (*slog.Logger, *bytes.Buffer) {
	var buf bytes.Buffer
	return slog.New(slog.NewTextHandler(&buf, nil)), &buf
}

// TestAStoreThatCannotBeSetUpDisablesOnlyDeliveryInsights pins that a failed
// store turns the feature off, where it used to stop the Observer.
func TestAStoreThatCannotBeSetUpDisablesOnlyDeliveryInsights(t *testing.T) {
	// A file in a directory that does not exist cannot be opened for writing.
	dsn := "file:" + filepath.Join(t.TempDir(), "missing", "dir", "alerts.db")
	cfg := storeConfig(dsn)
	logger, logs := capturingLogger()

	store, closeStore := newDeliveryInsightsStore(cfg, logger)
	assert.Nil(t, store)
	require.NotNil(t, closeStore)
	closeStore()

	assert.False(t, cfg.DeliveryInsights.Enabled,
		"off, so the aggregator is not started and the API reports nothing collected")
	assert.Contains(t, logs.String(), "Delivery Insights is disabled until the Observer restarts")
	assert.Contains(t, logs.String(), "level=ERROR")
}

func TestInvalidSettingsAreReportedAtStartup(t *testing.T) {
	cfg := storeConfig("")
	cfg.DeliveryInsights.Enabled = false
	cfg.DeliveryInsights.DisabledReason = errors.New("deliveryinsights.aggregation.interval must be positive")
	logger, logs := capturingLogger()

	store, _ := newDeliveryInsightsStore(cfg, logger)
	assert.Nil(t, store)
	assert.Contains(t, logs.String(), "Its configuration is invalid")
	assert.Contains(t, logs.String(), "aggregation.interval must be positive")
}

func TestAWorkingStoreIsOpened(t *testing.T) {
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "-"))
	cfg := storeConfig(dsn)
	logger, logs := capturingLogger()

	store, closeStore := newDeliveryInsightsStore(cfg, logger)
	require.NotNil(t, store)
	closeStore()
	assert.True(t, cfg.DeliveryInsights.Enabled)
	assert.NotContains(t, logs.String(), "disabled")
}

func TestTurnedOffNothingIsOpenedOrLogged(t *testing.T) {
	cfg := storeConfig("file:/nonexistent/alerts.db")
	cfg.DeliveryInsights.Enabled = false
	logger, logs := capturingLogger()

	store, _ := newDeliveryInsightsStore(cfg, logger)
	assert.Nil(t, store)
	assert.Empty(t, logs.String())
}
