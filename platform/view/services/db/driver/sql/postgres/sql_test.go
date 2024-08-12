/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package postgres

import (
	"os"
	"testing"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver"
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
	terminate, pgConnStr, err := StartPostgres(t, false)
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
