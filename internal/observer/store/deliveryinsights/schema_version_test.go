// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package deliveryinsights

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestASchemaNewerThanTheCodeIsRefused pins what a downgraded Observer does with
// a store a newer one migrated. Skipping the versions it does not know -- what
// the runner did before -- left it aggregating by its own, older rules into
// tables shaped for the newer ones.
func TestASchemaNewerThanTheCodeIsRefused(t *testing.T) {
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "-"))
	ctx := context.Background()

	// A newer Observer migrated the store one version past this one.
	newer, err := New(BackendSQLite, dsn, slog.Default())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, newer.Close()) })
	require.NoError(t, newer.Initialize(ctx))
	_, err = newer.(*sqlStore).db.ExecContext(ctx,
		`INSERT INTO delivery_insights_schema_version (version, applied_at_ms) VALUES (?, ?);`,
		latestSchemaVersion()+1, time.Now().UnixMilli())
	require.NoError(t, err)

	// This Observer starts against it.
	older, err := New(BackendSQLite, dsn, slog.Default())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, older.Close()) })
	err = older.Initialize(ctx)
	require.ErrorIs(t, err, ErrSchemaNewer)
	require.Contains(t, err.Error(), fmt.Sprintf("version %d", latestSchemaVersion()+1))
}

// TestTheCurrentSchemaIsAccepted pins that re-initialising a store already at
// this Observer's version -- every restart -- is not mistaken for a downgrade.
func TestTheCurrentSchemaIsAccepted(t *testing.T) {
	store := newTestStore(t)
	require.NoError(t, store.Initialize(context.Background()))
}
