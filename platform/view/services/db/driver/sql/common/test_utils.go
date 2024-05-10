/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package common

import (
	"testing"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/dbtest"
	_ "github.com/lib/pq"
	_ "modernc.org/sqlite"
)

func TestCases(t *testing.T, versionedProvider func(name string) (*Persistence, error), unversionedProvider func(name string) (*Unversioned, error)) {
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
}
