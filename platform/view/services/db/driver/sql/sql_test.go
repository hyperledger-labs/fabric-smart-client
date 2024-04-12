/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package sql

import (
	"database/sql"
	"fmt"
	"os"
	"path"
	"strconv"
	"testing"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/dbtest"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver"
	_ "github.com/lib/pq"
	_ "modernc.org/sqlite"
)

func TestSqlite(t *testing.T) {
	tempDir := t.TempDir()
	for _, c := range dbtest.Cases {
		db, _, _, _, err := initSqliteVersioned(tempDir, c.Name)
		if err != nil {
			t.Fatal(err)
		}
		t.Run(c.Name, func(xt *testing.T) {
			defer db.Close()
			c.Fn(xt, db)
		})
	}
	for _, c := range dbtest.UnversionedCases {
		un, _, _, _, err := initSqliteUnversioned(tempDir, c.Name)
		if err != nil {
			t.Fatal(err)
		}
		t.Run(c.Name, func(xt *testing.T) {
			defer un.Close()
			c.Fn(xt, un)
		})
	}
	for _, c := range dbtest.ErrorCases {
		un, readDB, writeDB, errorWrapper, err := initSqliteUnversioned(tempDir, c.Name)
		if err != nil {
			t.Fatal(err)
		}
		t.Run(c.Name, func(xt *testing.T) {
			defer un.Close()
			c.Fn(xt, readDB, writeDB, errorWrapper, "test")
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
		db, _, _, _, err := initPostgresVersioned(pgConnStr, c.Name)
		if err != nil {
			t.Fatal(err)
		}
		t.Run(c.Name, func(xt *testing.T) {
			defer db.Close()
			c.Fn(xt, db)
		})
	}
	for _, c := range dbtest.UnversionedCases {
		un, _, _, _, err := initPostgresUnversioned(pgConnStr, c.Name)
		if err != nil {
			t.Fatal(err)
		}
		t.Run(c.Name, func(xt *testing.T) {
			defer un.Close()
			c.Fn(xt, un)
		})
	}
	for _, c := range dbtest.ErrorCases {
		un, readDB, writeDB, errorWrapper, err := initPostgresUnversioned(pgConnStr, c.Name)
		if err != nil {
			t.Fatal(err)
		}
		t.Run(c.Name, func(xt *testing.T) {
			defer un.Close()
			c.Fn(xt, readDB, writeDB, errorWrapper, c.Name)
		})
	}
}

func initSqliteVersioned(tempDir, key string) (*Persistence, *sql.DB, *sql.DB, driver.SQLErrorWrapper, error) {
	return initVersioned("sqlite", fmt.Sprintf("%s.sqlite", path.Join(tempDir, key)), "test", 0)
}

func initPostgresVersioned(pgConnStr, key string) (*Persistence, *sql.DB, *sql.DB, driver.SQLErrorWrapper, error) {
	return initVersioned("postgres", pgConnStr, key, 50)
}

func initVersioned(driverName, dataSource, table string, maxOpenConns int) (*Persistence, *sql.DB, *sql.DB, driver.SQLErrorWrapper, error) {
	readDB, writeDB, err := openDB(driverName, dataSource, maxOpenConns, false)
	if err != nil || readDB == nil || writeDB == nil {
		return nil, nil, nil, nil, fmt.Errorf("database not open: %w", err)
	}
	errorWrapper := sqlErrorWrapper(driverName)
	p := NewPersistence(readDB, writeDB, table, errorWrapper)
	if err := p.CreateSchema(); err != nil {
		return nil, nil, nil, nil, err
	}
	return p, readDB, writeDB, errorWrapper, nil
}

func initPostgresUnversioned(pgConnStr, key string) (*Unversioned, *sql.DB, *sql.DB, driver.SQLErrorWrapper, error) {
	return initUnversioned("postgres", pgConnStr, key)
}

func initSqliteUnversioned(tempDir, key string) (*Unversioned, *sql.DB, *sql.DB, driver.SQLErrorWrapper, error) {
	return initUnversioned("sqlite", fmt.Sprintf("file:%s.sqlite?_pragma=busy_timeout(1000)", path.Join(tempDir, key)), "test")
}

func initUnversioned(driverName, dataSource, table string) (*Unversioned, *sql.DB, *sql.DB, driver.SQLErrorWrapper, error) {
	readDB, writeDB, err := openDB(driverName, dataSource, 0, false)
	if err != nil || readDB == nil || writeDB == nil {
		return nil, nil, nil, nil, fmt.Errorf("database not open: %w", err)
	}
	errorWrapper := sqlErrorWrapper(driverName)
	p := NewUnversioned(readDB, writeDB, table, errorWrapper)
	if err := p.CreateSchema(); err != nil {
		return nil, nil, nil, nil, fmt.Errorf("can't create schema: %w", err)
	}
	return p, readDB, writeDB, errorWrapper, nil
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
