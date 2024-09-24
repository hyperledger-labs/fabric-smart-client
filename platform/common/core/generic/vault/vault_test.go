/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package vault

import (
	"encoding/json"
	"testing"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/core/generic/vault/db"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/core/generic/vault/txidstore"
	driver2 "github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils/collections"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/cache/secondcache"
	db2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/db"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/flogging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics/disabled"
	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/otel/trace/noop"
	"golang.org/x/exp/slices"
)

//go:generate counterfeiter -o mocks/config.go -fake-name Config . config

type testArtifactProvider struct{}

func (p *testArtifactProvider) NewCachedVault(ddb VersionedPersistence) (*Vault[ValidationCode], error) {
	txidStore, err := txidstore.NewSimpleTXIDStore[ValidationCode](db2.Unversioned(ddb), &VCProvider{})
	if err != nil {
		return nil, err
	}
	vaultLogger := flogging.MustGetLogger("vault-logger")
	return New[ValidationCode](
		vaultLogger,
		ddb,
		txidstore.NewCache[ValidationCode](txidStore, secondcache.NewTyped[*txidstore.Entry[ValidationCode]](100), vaultLogger),
		&VCProvider{},
		newInterceptor,
		&populator{},
		&disabled.Provider{},
		&noop.TracerProvider{},
		&BlockTxIndexVersionBuilder{},
	), nil
}

func (p *testArtifactProvider) NewNonCachedVault(ddb VersionedPersistence) (*Vault[ValidationCode], error) {
	txidStore, err := txidstore.NewSimpleTXIDStore[ValidationCode](db2.Unversioned(ddb), &VCProvider{})
	if err != nil {
		return nil, err
	}
	return New[ValidationCode](
		flogging.MustGetLogger("vault"),
		ddb,
		txidstore.NewNoCache[ValidationCode](txidStore),
		&VCProvider{},
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
	qe VersionedQueryExecutor,
	txidStore TXIDStoreReader[ValidationCode],
	txid driver2.TxID,
) TxInterceptor {
	return NewInterceptor[ValidationCode](
		logger,
		qe,
		txidStore,
		txid,
		&VCProvider{},
		&marshaller{},
		&BlockTxIndexVersionComparator{},
	)
}

type populator struct {
	marshaller marshaller
}

func (p *populator) Populate(rws *ReadWriteSet, rwsetBytes []byte, namespaces ...driver2.Namespace) error {
	return p.marshaller.Append(rws, rwsetBytes, namespaces...)
}

type marshaller struct{}

func (m *marshaller) Marshal(rws *ReadWriteSet) ([]byte, error) {
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
	for ns, reads := range source.ReadSet.Reads {
		if len(nss) != 0 && !namespaces.Contains(ns) {
			continue
		}
		for s, position := range reads {
			v, in := destination.ReadSet.Get(ns, s)
			if in && !Equal(position, v) {
				return errors.Errorf("invalid read [%s:%s]: previous value returned at fver [%v], current value at fver [%v]", ns, s, position, v)
			}
			destination.ReadSet.Add(ns, s, position)
		}
	}
	destination.OrderedReads = source.OrderedReads

	// writeset
	for ns, writes := range source.WriteSet.Writes {
		if len(nss) != 0 && !namespaces.Contains(ns) {
			continue
		}
		for s, position := range writes {
			if destination.WriteSet.In(ns, s) {
				return errors.Errorf("duplicate write entry for key %s:%s", ns, s)
			}
			destination.WriteSet.Add(ns, s, position)
		}
	}
	destination.OrderedWrites = source.OrderedWrites

	// meta writes
	for ns, writes := range source.MetaWriteSet.MetaWrites {
		if len(nss) != 0 && !namespaces.Contains(ns) {
			continue
		}
		for s, position := range writes {
			if destination.MetaWriteSet.In(ns, s) {
				return errors.Errorf("duplicate metadata write entry for key %s:%s", ns, s)
			}
			destination.MetaWriteSet.Add(ns, s, position)
		}
	}

	return nil
}

func TestMemory(t *testing.T) {
	RemoveNils = func(items []VersionedRead) []VersionedRead { return items }
	artifactProvider := &testArtifactProvider{}
	for _, c := range SingleDBCases {
		ddb, err := db.OpenMemoryVersioned()
		assert.NoError(t, err)
		t.Run(c.Name, func(xt *testing.T) {
			defer ddb.Close()
			c.Fn(xt, ddb, artifactProvider)
		})
	}

	for _, c := range DoubleDBCases {
		db1, err := db.OpenMemoryVersioned()
		assert.NoError(t, err)
		db2, err := db.OpenMemoryVersioned()
		assert.NoError(t, err)
		t.Run(c.Name, func(xt *testing.T) {
			defer db1.Close()
			defer db2.Close()
			c.Fn(xt, db1, db2, artifactProvider)
		})
	}
}

func TestBadger(t *testing.T) {
	RemoveNils = func(items []VersionedRead) []VersionedRead { return items }
	//for _, c := range SingleDBCases {
	//	ddb, terminate, err := OpenBadgerVersioned(t.TempDir(), "DB-TestVaultBadgerDB1")
	//	assert.NoError(t, err)
	//	t.Run(c.Name, func(xt *testing.T) {
	//		defer ddb.Close()
	//		c.Fn(xt, ddb)
	//	})
	//}
	artifactProvider := &testArtifactProvider{}
	for _, c := range DoubleDBCases {
		db1, err := db.OpenBadgerVersioned(t.TempDir(), "DB-TestVaultBadgerDB1")
		assert.NoError(t, err)
		db2, err := db.OpenBadgerVersioned(t.TempDir(), "DB-TestVaultBadgerDB2")
		assert.NoError(t, err)
		t.Run(c.Name, func(xt *testing.T) {
			defer db1.Close()
			defer db2.Close()
			c.Fn(xt, db1, db2, artifactProvider)
		})
	}
}

func TestSqlite(t *testing.T) {
	RemoveNils = func(items []VersionedRead) []VersionedRead {
		return slices.DeleteFunc(items, func(e VersionedRead) bool { return e.Raw == nil })
	}
	artifactProvider := &testArtifactProvider{}

	for _, c := range SingleDBCases {
		ddb, err := db.OpenSqliteVersioned("node1", t.TempDir())
		assert.NoError(t, err)
		t.Run(c.Name, func(xt *testing.T) {
			defer ddb.Close()
			c.Fn(xt, ddb, artifactProvider)
		})
	}

	for _, c := range DoubleDBCases {
		db1, err := db.OpenSqliteVersioned("node1", t.TempDir())
		assert.NoError(t, err)
		db2, err := db.OpenSqliteVersioned("node2", t.TempDir())
		assert.NoError(t, err)
		t.Run(c.Name, func(xt *testing.T) {
			defer db1.Close()
			defer db2.Close()
			c.Fn(xt, db1, db2, artifactProvider)
		})
	}
}

func TestPostgres(t *testing.T) {
	RemoveNils = func(items []VersionedRead) []VersionedRead {
		return slices.DeleteFunc(items, func(e VersionedRead) bool { return e.Raw == nil })
	}
	artifactProvider := &testArtifactProvider{}

	for _, c := range SingleDBCases {
		ddb, terminate, err := db.OpenPostgresVersioned("common-sdk-node1")
		assert.NoError(t, err)
		t.Run(c.Name, func(xt *testing.T) {
			defer ddb.Close()
			defer terminate()
			c.Fn(xt, ddb, artifactProvider)
		})
	}

	for _, c := range DoubleDBCases {
		db1, terminate1, err := db.OpenPostgresVersioned("common-sdk-node1")
		assert.NoError(t, err)
		db2, terminate2, err := db.OpenPostgresVersioned("common-sdk-node2")
		assert.NoError(t, err)
		t.Run(c.Name, func(xt *testing.T) {
			defer db1.Close()
			defer db2.Close()
			defer terminate1()
			defer terminate2()
			c.Fn(xt, db1, db2, artifactProvider)
		})
	}
}
