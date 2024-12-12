/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package postgres

import (
	"os"
	"testing"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver"
	common2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver/sql/common"
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

	common2.TestCases(t, func(name string) (driver.TransactionalVersionedPersistence, error) {
		return initPersistence(NewVersioned, pgConnStr, name, 50, 2, time.Minute)
	}, func(name string) (driver.UnversionedPersistence, error) {
		return initPersistence(NewUnversioned, pgConnStr, name, 0, 2, time.Minute)
	}, func(name string) (driver.UnversionedNotifier, error) {
		return initPersistence(NewUnversionedNotifier, pgConnStr, name, 0, 2, time.Minute)
	}, func(name string) (driver.VersionedNotifier, error) {
		return initPersistence(NewVersionedNotifier, pgConnStr, name, 50, 2, time.Minute)
	}, func(p driver.UnversionedPersistence) *common2.BasePersistence[driver.UnversionedValue, driver.UnversionedRead] {
		return p.(*UnversionedPersistence).BasePersistence.(*BasePersistence[driver.UnversionedValue, driver.UnversionedRead]).BasePersistence
	})
}
