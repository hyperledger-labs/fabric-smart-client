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

	o := testOpts{dataSource: pgConnStr, maxIdleConns: 2, maxIdleTime: 2 * time.Minute}
	common2.TestCases(t, func(name string) (driver.UnversionedPersistence, error) {
		return common2.NewPersistenceWithOpts[DbOpts](name, o, NewUnversionedPersistence)
	}, func(name string) (driver.UnversionedNotifier, error) {
		return common2.NewPersistenceWithOpts[DbOpts](name, o, NewUnversionedNotifier)
	}, func(p driver.UnversionedPersistence) *common2.UnversionedPersistence {
		return p.(*UnversionedPersistence).UnversionedPersistence
	})
}
