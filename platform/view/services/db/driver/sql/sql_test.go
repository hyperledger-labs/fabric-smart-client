/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package sql

import (
	"context"
	"fmt"
	"path"
	"testing"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/dbtest"
	_ "github.com/lib/pq"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
	_ "modernc.org/sqlite"
)

func TestSqlite(t *testing.T) {
	tempDir := t.TempDir()

	for _, c := range dbtest.Cases {
		db := initSqliteVersioned(t, tempDir, c.Name)
		t.Run(c.Name, func(xt *testing.T) {
			defer db.Close()
			c.Fn(xt, db)
		})
	}
	for _, c := range dbtest.UnversionedCases {
		un := initSqliteUnversioned(t, tempDir, c.Name)
		t.Run(c.Name, func(xt *testing.T) {
			defer un.Close()
			c.Fn(xt, un)
		})
	}
}

func TestPostgres(t *testing.T) {
	terminate, pgConnStr := startPostgresContainer(t)
	defer terminate()

	for _, c := range dbtest.Cases {
		db := initPostgresVersioned(t, pgConnStr, c.Name)
		t.Run(c.Name, func(xt *testing.T) {
			defer db.Close()
			c.Fn(xt, db)
		})
	}
	for _, c := range dbtest.UnversionedCases {
		un := initPostgresUnversioned(t, pgConnStr, c.Name)
		t.Run(c.Name, func(xt *testing.T) {
			defer un.Close()
			c.Fn(xt, un)
		})
	}
}

func initSqliteUnversioned(t *testing.T, tempDir, key string) *Unversioned {
	db, err := openDB("sqlite", fmt.Sprintf("file:%s?_pragma=busy_timeout(5000)", path.Join(tempDir, key)), 2)
	if err != nil {
		t.Fatal(err)
	}
	p := NewUnversioned(db, "test")
	if err := p.CreateSchema(); err != nil {
		t.Fatal(err)
	}
	return p
}

func initSqliteVersioned(t *testing.T, tempDir, key string) *Persistence {
	db, err := openDB("sqlite", fmt.Sprintf("file:%s?_pragma=busy_timeout(5000)", path.Join(tempDir, key)), 2)
	if err != nil {
		t.Fatal(err)
	}
	p := NewPersistence(db, "test")
	if err := p.CreateSchema(); err != nil {
		t.Fatal(err)
	}
	return p
}

func initPostgresVersioned(t *testing.T, pgConnStr, key string) *Persistence {
	db, err := openDB("postgres", pgConnStr, 10)
	if err != nil {
		t.Fatal(err)
	}
	if db == nil {
		t.Fatal("database is nil")
	}
	p := NewPersistence(db, key)
	if err := p.CreateSchema(); err != nil {
		t.Fatal(err)
	}
	return p
}

func initPostgresUnversioned(t *testing.T, pgConnStr, key string) *Unversioned {
	db, err := openDB("postgres", pgConnStr, 10)
	if err != nil {
		t.Fatal(err)
	}
	if db == nil {
		t.Fatal("database is nil")
	}
	p := NewUnversioned(db, key)
	if err := p.CreateSchema(); err != nil {
		t.Fatal(err)
	}
	return p
}

// https://testcontainers.com/guides/getting-started-with-testcontainers-for-go/
// Note: Before running tests: docker pull postgres:16.0-alpine
// Test may time out if image is not present on machine.
func startPostgresContainer(t *testing.T) (func(), string) {
	// if os.Getenv("TESTCONTAINERS") != "true" {
	// 	t.Skip("set environment variable TESTCONTAINERS to true to include postgres test")
	// }
	if testing.Short() {
		t.Skip("skipping postgres test in short mode")
	}

	ctx := context.Background()
	pg, err := postgres.RunContainer(ctx,
		testcontainers.WithImage("postgres:16.0-alpine"),
		testcontainers.WithWaitStrategy(
			wait.ForExposedPort().WithStartupTimeout(30*time.Second)),
		postgres.WithDatabase("testdb"),
		postgres.WithUsername("postgres"),
		postgres.WithPassword("example"),
	)
	if err != nil {
		t.Fatal(err)
	}
	pgConnStr, err := pg.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatal(err)
	}

	return func() { pg.Terminate(ctx) }, pgConnStr
}
