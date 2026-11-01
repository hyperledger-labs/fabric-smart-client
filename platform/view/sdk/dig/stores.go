/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package sdk

import (
	"context"

	"go.uber.org/dig"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/node"
	commondig "github.com/hyperledger-labs/fabric-smart-client/platform/common/sdk/dig"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver/sql/common/migrate"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/kvs"
)

// StoreOpenerEntry provides a node.StoreOpener into the dig value group
// "fsc-store-openers", whose entries `node migrate` calls. An SDK provides one
// during Install; SDKs nested in each other share one container, so every entry
// is reached.
type StoreOpenerEntry struct {
	dig.Out
	Opener node.StoreOpener `group:"fsc-store-openers"`
}

// StoreOpenerFunc adapts a function to a node.StoreOpener.
type StoreOpenerFunc func(ctx context.Context) error

// OpenStores calls f.
func (f StoreOpenerFunc) OpenStores(ctx context.Context) error { return f(ctx) }

// storeOpeners is the node.StoreOpener registered in the node's registry. It
// resolves the group when called, after every SDK has installed, and
// calls each entry.
type storeOpeners struct{ c commondig.Container }

func (s storeOpeners) OpenStores(ctx context.Context) error {
	return s.c.Invoke(func(in struct {
		dig.In
		Openers []node.StoreOpener `group:"fsc-store-openers"`
	},
	) error {
		for _, o := range in.Openers {
			if err := o.OpenStores(ctx); err != nil {
				return err
			}
		}
		return nil
	})
}

// migratingGate reports the node not ready with reason "migrating" while a store
// of this process runs schema steps or waits for the migration lock, such as a
// vault converted when its channel opens after the listeners started.
type migratingGate struct{}

func (migratingGate) NotReady() (string, bool) { return "migrating", migrate.InProgress() }

// newViewStoreOpener opens the view stores: the KVS, binding, signer info and
// audit info stores. dig builds each store once, so resolving them is opening
// them.
func newViewStoreOpener(c commondig.Container) StoreOpenerEntry {
	return StoreOpenerEntry{Opener: StoreOpenerFunc(func(context.Context) error {
		return c.Invoke(func(*kvs.KVS, driver.BindingStore, driver.SignerInfoStore, driver.AuditInfoStore) {})
	})}
}
