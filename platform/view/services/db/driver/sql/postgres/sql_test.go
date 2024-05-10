/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package postgres

import (
	"fmt"
	"os"
	"strconv"
	"testing"

	common2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/sql/common"
	_ "github.com/lib/pq"
	_ "modernc.org/sqlite"
)

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

	common2.TestCases(t, func(name string) (*common2.Persistence, error) {
		return initPostgresVersioned(pgConnStr, name)
	}, func(name string) (*common2.Unversioned, error) {
		return initPostgresUnversioned(pgConnStr, name)
	})
}

func initPostgresVersioned(pgConnStr, name string) (*common2.Persistence, error) {
	p, err := NewPersistence(common2.Opts{
		Driver:       "postgres",
		DataSource:   pgConnStr,
		MaxOpenConns: 50,
		SkipPragmas:  false,
	}, name)
	if err != nil {
		return nil, err
	}
	if err := p.CreateSchema(); err != nil {
		return nil, err
	}
	return p, nil
}
func initPostgresUnversioned(pgConnStr, name string) (*common2.Unversioned, error) {
	p, err := NewUnversioned(common2.Opts{
		Driver:       "postgres",
		DataSource:   pgConnStr,
		MaxOpenConns: 0,
		SkipPragmas:  false,
	}, name)
	if err != nil {
		return nil, err
	}
	if err := p.CreateSchema(); err != nil {
		return nil, err
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
