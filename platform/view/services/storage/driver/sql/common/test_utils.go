/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package common

import (
	"testing"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver"
	_ "modernc.org/sqlite"
)

type provider[V any] func(name string) (V, error)

func TestCases(t *testing.T,
	unversionedProvider provider[driver.KeyValueStore],
	unversionedNotifierProvider provider[driver.UnversionedNotifier],
	baseUnpacker func(p driver.KeyValueStore) *KeyValueStore,
) {
	for _, c := range Cases {
		un, err := unversionedProvider(c.Name)
		if err != nil {
			t.Fatal(err)
		}
		t.Run(c.Name, func(xt *testing.T) {
			defer utils.IgnoreErrorFunc(un.Close)
			c.Fn(xt, un)
		})
	}
	for _, c := range UnversionedCases {
		un, err := unversionedProvider(c.Name)
		if err != nil {
			t.Fatal(err)
		}
		t.Run(c.Name, func(xt *testing.T) {
			defer utils.IgnoreErrorFunc(un.Close)
			c.Fn(xt, un)
		})
	}
	for _, c := range ErrorCases {
		un, err := unversionedProvider(c.Name)
		if err != nil {
			t.Fatal(err)
		}
		b := baseUnpacker(un)
		t.Run(c.Name, func(xt *testing.T) {
			defer utils.IgnoreErrorFunc(un.Close)
			c.Fn(xt, b.readDB, b.writeDB, b.errorWrapper, b.table)
		})
	}
	for _, c := range UnversionedNotifierCases {
		un, err := unversionedNotifierProvider(c.Name)
		if err != nil {
			t.Fatal(err)
		}
		t.Run(c.Name, func(xt *testing.T) {
			defer utils.IgnoreErrorFunc(un.Close)
			c.Fn(xt, un)
		})
	}
}
