/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package sql

import (
	"fmt"
	"os"
	"path"
	"strconv"
	"testing"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/dbtest"
	_ "github.com/lib/pq"
	_ "modernc.org/sqlite"
)

func TestSqlite(t *testing.T) {
	tempDir := t.TempDir()
	for _, c := range dbtest.Cases {
		db, err := initSqliteVersioned(tempDir, c.Name)
		if err != nil {
			t.Fatal(err)
		}
		t.Run(c.Name, func(xt *testing.T) {
			defer db.Close()
			c.Fn(xt, db)
		})
	}
	for _, c := range dbtest.UnversionedCases {
		un, err := initSqliteUnversioned(tempDir, c.Name)
		if err != nil {
			t.Fatal(err)
		}
		t.Run(c.Name, func(xt *testing.T) {
			defer un.Close()
			c.Fn(xt, un)
		})
	}
}

func TestPostgres(t *testing.T) {
	if os.Getenv("TEST_POSTGRES") != "true" {
		t.Skip("set environment variable TEST_POSTGRES to true to include postgres test")
	}
	if testing.Short() {
		t.Skip("skipping postgres test in short mode")
	}
	t.Log("starting postgres")
	terminate, pgConnStr, err := startPostgres(t, false)
	if err != nil {
		t.Fatal(err)
	}
	defer terminate()
	t.Log("postgres ready")

	for _, c := range dbtest.Cases {
		db, err := initPostgresVersioned(pgConnStr, c.Name)
		if err != nil {
			t.Fatal(err)
		}
		t.Run(c.Name, func(xt *testing.T) {
			defer db.Close()
			c.Fn(xt, db)
		})
	}
	for _, c := range dbtest.UnversionedCases {
		un, err := initPostgresUnversioned(pgConnStr, c.Name)
		if err != nil {
			t.Fatal(err)
		}
		t.Run(c.Name, func(xt *testing.T) {
			defer un.Close()
			c.Fn(xt, un)
		})
	}
}

func initSqliteUnversioned(tempDir, key string) (*Unversioned, error) {
	readDB, writeDB, err := openDB("sqlite", fmt.Sprintf("file:%s.sqlite?_pragma=busy_timeout(1000)", path.Join(tempDir, key)), 0, false)
	if err != nil {
		return nil, err
	}
	p := NewUnversioned(readDB, writeDB, "test")
	if err := p.CreateSchema(); err != nil {
		return nil, err
	}
	return p, nil
}

func initSqliteVersioned(tempDir, key string) (*Persistence, error) {
	readDB, writeDB, err := openDB("sqlite", fmt.Sprintf("%s.sqlite", path.Join(tempDir, key)), 0, false)
	if err != nil || readDB == nil || writeDB == nil {
		return nil, fmt.Errorf("database not open: %w", err)
	}
	p := NewPersistence(readDB, writeDB, "test")
	if err := p.CreateSchema(); err != nil {
		return nil, err
	}
	return p, nil
}

func initPostgresVersioned(pgConnStr, key string) (*Persistence, error) {
	readDB, writeDB, err := openDB("postgres", pgConnStr, 50, false)
	if err != nil || readDB == nil || writeDB == nil {
		return nil, fmt.Errorf("database not open: %w", err)
	}
	p := NewPersistence(readDB, writeDB, key)
	if err := p.CreateSchema(); err != nil {
		return nil, err
	}
	return p, nil
}

func initPostgresUnversioned(pgConnStr, key string) (*Unversioned, error) {
	readDB, writeDB, err := openDB("postgres", pgConnStr, 0, false)
	if err != nil || readDB == nil || writeDB == nil {
		return nil, fmt.Errorf("database not open: %w", err)
	}
	p := NewUnversioned(readDB, writeDB, key)
	if err := p.CreateSchema(); err != nil {
		return nil, fmt.Errorf("can't create schema: %w", err)
	}
	return p, nil
}

func startPostgres(t Logger, printLogs bool) (func(), string, error) {
	port := getEnv("POSTGRES_PORT", "5432")
	p, err := strconv.Atoi(port)
	if err != nil {
		return nil, "", fmt.Errorf("port must be a number: %s", port)
	}

	c := PostgresConfig{
		Image:     getEnv("POSTGRES_IMAGE", "postgres:latest"),
		Container: getEnv("POSTGRES_CONTAINER", "fsc-postgres"),
		DBName:    getEnv("POSTGRES_DB", "testdb"),
		User:      getEnv("POSTGRES_USER", "postgres"),
		Pass:      getEnv("POSTGRES_PASSWORD", "example"),
		Host:      getEnv("POSTGRES_HOST", "localhost"),
		Port:      p,
	}
	closeFunc, err := startPostgresWithLogger(c, t, printLogs)
	if err != nil {
		return nil, "", err
	}
	return closeFunc, c.DataSource(), err
}

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}
