/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package vault

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	driver2 "github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/collections"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics/disabled"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/vault"
	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/otel/trace/noop"
	"golang.org/x/exp/slices"
)

//go:generate counterfeiter -o mocks/config.go -fake-name Config . config

type testArtifactProvider struct{}

func (p *testArtifactProvider) NewCachedVault(ddb driver.VaultStore) (*Vault[ValidationCode], error) {
	vaultLogger := logging.MustGetLogger("vault-logger")
	return New[ValidationCode](
		vaultLogger,
		vault.NewCachedVault(ddb, 100),
		VCProvider,
		newInterceptor,
		&populator{},
		&disabled.Provider{},
		&noop.TracerProvider{},
		&BlockTxIndexVersionBuilder{},
	), nil
}

func (p *testArtifactProvider) NewNonCachedVault(ddb driver.VaultStore) (*Vault[ValidationCode], error) {
	return New[ValidationCode](
		logging.MustGetLogger("vault"),
		vault.NewCachedVault(ddb, 0),
		VCProvider,
		newInterceptor,
		&populator{},
		&disabled.Provider{},
		&noop.TracerProvider{},
		&BlockTxIndexVersionBuilder{},
	), nil
}

func (p *testArtifactProvider) NewMarshaller() Marshaller {
	return &marshaller{}
}

func newInterceptor(
	logger Logger,
	ctx context.Context,
	rwSet ReadWriteSet,
	qe VersionedQueryExecutor,
	txidStore TxStatusStore,
	txid driver2.TxID,
) TxInterceptor {
	return NewInterceptor[ValidationCode](
		logger,
		ctx,
		rwSet,
		qe,
		txidStore,
		txid,
		VCProvider,
		&marshaller{},
		&BlockTxIndexVersionComparator{},
	)
}

type populator struct {
	marshaller marshaller
}

func (p *populator) Populate(rwsetBytes []byte, namespaces ...driver2.Namespace) (ReadWriteSet, error) {
	rwSet := EmptyRWSet()
	if err := p.marshaller.Append(&rwSet, rwsetBytes, namespaces...); err != nil {
		return ReadWriteSet{}, err
	}
	return rwSet, nil
}

type marshaller struct{}

func (m *marshaller) Marshal(txID string, rws *ReadWriteSet) ([]byte, error) {
	return json.Marshal(rws)
}

func (m *marshaller) Append(destination *ReadWriteSet, raw []byte, nss ...string) error {
	source := &ReadWriteSet{}
	err := json.Unmarshal(raw, source)
	if err != nil {
		return errors.Wrapf(err, "provided invalid read-write set bytes, unmarshal failed")
	}

	namespaces := collections.NewSet(nss...)

	// readset
	for ns, reads := range source.Reads {
		if len(nss) != 0 && !namespaces.Contains(ns) {
			continue
		}
		for s, position := range reads {
			v, in := destination.ReadSet.Get(ns, s)
			if in && !Equal(position, v) {
				return errors.Errorf("invalid read [%s:%s]: previous value returned at version [%v], current value at version [%v]", ns, s, position, v)
			}
			destination.ReadSet.Add(ns, s, position)
		}
	}
	destination.OrderedReads = source.OrderedReads

	// writeset
	for ns, writes := range source.Writes {
		if len(nss) != 0 && !namespaces.Contains(ns) {
			continue
		}
		for s, position := range writes {
			if destination.WriteSet.In(ns, s) {
				return errors.Errorf("duplicate write entry for key %s:%s", ns, s)
			}
			if err := destination.WriteSet.Add(ns, s, position); err != nil {
				return err
			}
		}
	}
	destination.OrderedWrites = source.OrderedWrites

	// meta writes
	for ns, writes := range source.MetaWrites {
		if len(nss) != 0 && !namespaces.Contains(ns) {
			continue
		}
		for s, position := range writes {
			if destination.MetaWriteSet.In(ns, s) {
				return errors.Errorf("duplicate metadata write entry for key %s:%s", ns, s)
			}
			if err := destination.MetaWriteSet.Add(ns, s, position); err != nil {
				return errors.Wrapf(err, "duplicate metadata write entry for key %s:%s", ns, s)
			}
		}
	}

	return nil
}

func TestMemory(t *testing.T) {
	RemoveNils = func(items []driver2.VaultRead) []driver2.VaultRead { return items }
	artifactProvider := &testArtifactProvider{}
	for _, c := range SingleDBCases {
		ddb, err := vault.OpenMemoryVault(c.Name)
		assert.NoError(t, err)
		assert.NotNil(t, ddb)
		t.Run(c.Name, func(xt *testing.T) {
			defer utils.IgnoreErrorFunc(ddb.Close)
			c.Fn(xt, ddb, artifactProvider)
		})
	}

	for _, c := range DoubleDBCases {
		db1, err := vault.OpenMemoryVault(c.Name)
		assert.NoError(t, err)
		db2, err := vault.OpenMemoryVault(c.Name)
		assert.NoError(t, err)
		t.Run(c.Name, func(xt *testing.T) {
			defer utils.IgnoreErrorFunc(db1.Close)
			defer utils.IgnoreErrorFunc(db2.Close)
			c.Fn(xt, db1, db2, artifactProvider)
		})
	}
}

func TestSqlite(t *testing.T) {
	RemoveNils = func(items []driver2.VaultRead) []driver2.VaultRead {
		return slices.DeleteFunc(items, func(e driver2.VaultRead) bool { return e.Raw == nil })
	}
	artifactProvider := &testArtifactProvider{}

	for _, c := range SingleDBCases {
		ddb, err := vault.OpenSqliteVault("node1", t.TempDir())
		assert.NoError(t, err)
		t.Run(c.Name, func(xt *testing.T) {
			defer utils.IgnoreErrorFunc(ddb.Close)
			c.Fn(xt, ddb, artifactProvider)
		})
	}

	for _, c := range DoubleDBCases {
		db1, err := vault.OpenSqliteVault("node1", t.TempDir())
		assert.NoError(t, err)
		db2, err := vault.OpenSqliteVault("node2", t.TempDir())
		assert.NoError(t, err)
		t.Run(c.Name, func(xt *testing.T) {
			defer utils.IgnoreErrorFunc(db1.Close)
			defer utils.IgnoreErrorFunc(db2.Close)
			c.Fn(xt, db1, db2, artifactProvider)
		})
	}
}

func TestPostgres(t *testing.T) {
	RemoveNils = func(items []driver2.VaultRead) []driver2.VaultRead {
		return slices.DeleteFunc(items, func(e driver2.VaultRead) bool { return e.Raw == nil })
	}
	artifactProvider := &testArtifactProvider{}

	for _, c := range append(SingleDBCases, ReadCommittedDBCases...) {
		ddb, terminate, err := vault.OpenPostgresVault("common-sdk-node1")
		assert.NoError(t, err)
		t.Run(c.Name, func(xt *testing.T) {
			defer utils.IgnoreErrorFunc(ddb.Close)
			defer terminate()
			c.Fn(xt, ddb, artifactProvider)
		})
	}

	for _, c := range DoubleDBCases {
		db1, terminate1, err := vault.OpenPostgresVault("common-sdk-node1")
		assert.NoError(t, err)
		db2, terminate2, err := vault.OpenPostgresVault("common-sdk-node2")
		assert.NoError(t, err)
		t.Run(c.Name, func(xt *testing.T) {
			defer utils.IgnoreErrorFunc(db1.Close)
			defer utils.IgnoreErrorFunc(db2.Close)
			defer terminate1()
			defer terminate2()
			c.Fn(xt, db1, db2, artifactProvider)
		})
	}
}
