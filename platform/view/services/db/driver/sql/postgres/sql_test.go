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

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver"
	common2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/sql/common"
	_ "github.com/lib/pq"
	_ "modernc.org/sqlite"
)

type dbObject interface {
	CreateSchema() error
}

type persistenceConstructor[V dbObject] func(common2.Opts, string) (V, error)

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

	common2.TestCases(t, func(name string) (*common2.VersionedPersistence, error) {
		return initPersistence(NewPersistence, pgConnStr, name, 50)
	}, func(name string) (*common2.UnversionedPersistence, error) {
		return initPersistence(NewUnversioned, pgConnStr, name, 0)
	}, func(name string) (driver.UnversionedNotifier, error) {
		return initPersistence(NewUnversionedNotifier, pgConnStr, name, 0)
	}, func(name string) (driver.VersionedNotifier, error) {
		return initPersistence(NewPersistenceNotifier, pgConnStr, name, 50)
	})
}

func initPersistence[V dbObject](constructor persistenceConstructor[V], pgConnStr, name string, maxOpenConns int) (V, error) {
	p, err := constructor(common2.Opts{DataSource: pgConnStr, MaxOpenConns: maxOpenConns}, name)
	if err != nil {
		return utils.Zero[V](), err
	}
	if err := p.CreateSchema(); err != nil {
		return utils.Zero[V](), err
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
