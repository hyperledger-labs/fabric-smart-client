/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package common

import (
	"testing"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/dbtest"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver"
	_ "modernc.org/sqlite"
)

type provider[V any] func(name string) (V, error)

func TestCases(t *testing.T,
	unversionedProvider provider[driver.UnversionedPersistence],
	unversionedNotifierProvider provider[driver.UnversionedNotifier],
	baseUnpacker func(p driver.UnversionedPersistence) *UnversionedPersistence) {
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
		b := baseUnpacker(un)
		t.Run(c.Name, func(xt *testing.T) {
			defer un.Close()
			c.Fn(xt, b.readDB, b.writeDB, b.errorWrapper, b.table)
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
}
