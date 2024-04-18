/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package orion

import (
	driver2 "github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/orion/driver"
	"github.com/pkg/errors"
)

type RWSet struct {
	driver.RWSet
}

func NewRWSet(rws driver.RWSet) *RWSet {
	return &RWSet{RWSet: rws}
}

func (r *RWSet) Equals(rws interface{}, nss ...string) error {
	r, ok := rws.(*RWSet)
	if !ok {
		return errors.Errorf("expected instance of *RWSet, got [%t]", rws)
	}
	return r.RWSet.Equals(r.RWSet, nss...)
}

type Vault interface {
	StoreEnvelope(id string, env interface{}) error
	StoreTransaction(id string, raw []byte) error
	StoreTransient(id string, tm driver.TransientMap) error
	Status(txID string) (driver.ValidationCode, string, error)
	DiscardTx(txID string, message string) error
	GetLastTxID() (string, error)
	NewQueryExecutor() (driver2.QueryExecutor, error)
	NewRWSet(txid string) (*RWSet, error)
	GetRWSet(id string, results []byte) (*RWSet, error)
	CommitTX(txid string, block uint64, indexInBloc int) error
	AddStatusReporter(sr driver.StatusReporter) error
}

// vault models a key-value store that can be updated by committing rwsets
type vault struct {
	driver.Vault
	driver.EnvelopeService
	driver.TransactionService
	driver.MetadataService
}

func newVault(ons driver.OrionNetworkService) Vault {
	return &vault{
		Vault:              ons.Vault(),
		EnvelopeService:    ons.EnvelopeService(),
		TransactionService: ons.TransactionService(),
		MetadataService:    ons.MetadataService(),
	}
}

func (v *vault) Status(txID string) (driver.ValidationCode, string, error) {
	return v.Vault.Status(txID)
}

func (v *vault) NewRWSet(txid string) (*RWSet, error) {
	rws, err := v.Vault.NewRWSet(txid)
	if err != nil {
		return nil, err
	}

	return &RWSet{RWSet: rws}, nil

}

func (v *vault) GetRWSet(id string, results []byte) (*RWSet, error) {
	rws, err := v.Vault.GetRWSet(id, results)
	if err != nil {
		return nil, err
	}

	return &RWSet{RWSet: rws}, nil
}
