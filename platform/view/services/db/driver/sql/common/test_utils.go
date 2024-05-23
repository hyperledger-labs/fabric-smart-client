/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package common

import (
	"testing"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/dbtest"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver"
	_ "github.com/lib/pq"
	_ "modernc.org/sqlite"
)

type provider[V any] func(name string) (V, error)

func TestCases(t *testing.T, versionedProvider provider[*Persistence], unversionedProvider provider[*Unversioned], unversionedNotifierProvider provider[driver.UnversionedNotifier], versionedNotifierProvider provider[driver.VersionedNotifier]) {
	for _, c := range dbtest.Cases {
		db, err := versionedProvider(c.Name)
		if err != nil {
			t.Fatal(err)
		}
		t.Run(c.Name, func(xt *testing.T) {
			defer db.Close()
			c.Fn(xt, db)
		})
	}
	for _, c := range dbtest.UnversionedCases {
		un, err := unversionedProvider(c.Name)
		if err != nil {
			t.Fatal(err)
		}
		t.Run(c.Name, func(xt *testing.T) {
			defer un.Close()
			c.Fn(xt, un)
		})
	}
	for _, c := range dbtest.ErrorCases {
		un, err := unversionedProvider(c.Name)
		if err != nil {
			t.Fatal(err)
		}
		t.Run(c.Name, func(xt *testing.T) {
			defer un.Close()
			c.Fn(xt, un.readDB, un.writeDB, un.errorWrapper, un.table)
		})
	}
	for _, c := range dbtest.UnversionedNotifierCases {
		un, err := unversionedNotifierProvider(c.Name)
		if err != nil {
			t.Fatal(err)
		}
		t.Run(c.Name, func(xt *testing.T) {
			defer un.Close()
			c.Fn(xt, un)
		})
	}
	for _, c := range dbtest.VersionedNotifierCases {
		un, err := versionedNotifierProvider(c.Name)
		if err != nil {
			t.Fatal(err)
		}
		t.Run(c.Name, func(xt *testing.T) {
			defer un.Close()
			c.Fn(xt, un)
		})
	}
}
