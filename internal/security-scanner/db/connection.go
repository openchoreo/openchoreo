// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package db

import (
	"database/sql"
	"embed"
	"fmt"

	_ "github.com/lib/pq"
	_ "github.com/mattn/go-sqlite3"
	"github.com/pressly/goose/v3"

	"github.com/openchoreo/openchoreo/internal/security-scanner/db/backend"
	"github.com/openchoreo/openchoreo/internal/security-scanner/db/backend/postgres"
	"github.com/openchoreo/openchoreo/internal/security-scanner/db/backend/sqlite"
)

//go:embed migrations/sqlite/*.sql
var sqliteMigrations embed.FS

//go:embed migrations/postgres/*.sql
var postgresMigrations embed.FS

type Config struct {
	Backend      DBBackend
	DSN          string
	MaxOpenConns int
	MaxIdleConns int
}

func InitDB(cfg Config) (*DBConnection, error) {
	var db *sql.DB
	var querier backend.Querier
	var err error

	switch cfg.Backend {
	case SQLite:
		db, err = initSQLite(cfg.DSN)
		if err != nil {
			return nil, err
		}
		querier = backend.NewGenericAdapter(sqlite.New(db))

	case Postgres:
		db, err = initPostgres(cfg.DSN)
		if err != nil {
			return nil, err
		}
		querier = backend.NewGenericAdapter(postgres.New(db))

	default:
		return nil, fmt.Errorf("unsupported database backend: %s", cfg.Backend)
	}

	if cfg.MaxOpenConns > 0 {
		db.SetMaxOpenConns(cfg.MaxOpenConns)
	}
	if cfg.MaxIdleConns > 0 {
		db.SetMaxIdleConns(cfg.MaxIdleConns)
	}

	return &DBConnection{
		db:      db,
		backend: cfg.Backend,
		querier: querier,
	}, nil
}

func initSQLite(dsn string) (*sql.DB, error) {
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open SQLite: %w", err)
	}

	goose.SetBaseFS(sqliteMigrations)
	if err := goose.SetDialect("sqlite3"); err != nil {
		return nil, fmt.Errorf("failed to set SQLite dialect: %w", err)
	}

	if err := goose.Up(db, "migrations/sqlite"); err != nil {
		return nil, fmt.Errorf("failed to run SQLite migrations: %w", err)
	}

	return db, nil
}

func initPostgres(dsn string) (*sql.DB, error) {
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open Postgres: %w", err)
	}

	goose.SetBaseFS(postgresMigrations)
	if err := goose.SetDialect("postgres"); err != nil {
		return nil, fmt.Errorf("failed to set Postgres dialect: %w", err)
	}

	if err := goose.Up(db, "migrations/postgres"); err != nil {
		return nil, fmt.Errorf("failed to run Postgres migrations: %w", err)
	}

	return db, nil
}
