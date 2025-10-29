// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package db

import (
	"database/sql"

	"github.com/openchoreo/openchoreo/internal/security-scanner/db/backend"
)

type DBBackend string

const (
	SQLite   DBBackend = "sqlite"
	Postgres DBBackend = "postgres"
)

type DBConnection struct {
	db      *sql.DB
	backend DBBackend
	querier backend.Querier
}

func (c *DBConnection) Querier() backend.Querier {
	return c.querier
}

func (c *DBConnection) DB() *sql.DB {
	return c.db
}

func (c *DBConnection) Backend() DBBackend {
	return c.backend
}

func (c *DBConnection) Close() error {
	return c.db.Close()
}
