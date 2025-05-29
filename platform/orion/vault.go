/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package orion

import (
	"context"

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
	StoreEnvelope(ctx context.Context, txID driver2.TxID, env interface{}) error
	StoreTransaction(ctx context.Context, txID driver2.TxID, raw []byte) error
	StoreTransient(ctx context.Context, txID driver2.TxID, tm driver.TransientMap) error
	Status(ctx context.Context, txID driver2.TxID) (driver.ValidationCode, string, error)
	DiscardTx(ctx context.Context, txID driver2.TxID, message string) error
	GetLastTxID(context.Context) (driver2.TxID, error)
	NewRWSet(ctx context.Context, txID driver2.TxID) (*RWSet, error)
	NewRWSetFromBytes(ctx context.Context, txID driver2.TxID, results []byte) (*RWSet, error)
	CommitTX(ctx context.Context, txID driver2.TxID, block driver.BlockNum, indexInBloc driver.TxNum) error
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

func (v *vault) Status(ctx context.Context, txID driver2.TxID) (driver.ValidationCode, string, error) {
	return v.Vault.Status(ctx, txID)
}

func (v *vault) NewRWSet(ctx context.Context, txid driver2.TxID) (*RWSet, error) {
	rws, err := v.Vault.NewRWSet(ctx, txid)
	if err != nil {
		return nil, err
	}

	return &RWSet{RWSet: rws}, nil

}

func (v *vault) NewRWSetFromBytes(ctx context.Context, id driver2.TxID, results []byte) (*RWSet, error) {
	rws, err := v.Vault.NewRWSetFromBytes(ctx, id, results)
	if err != nil {
		return nil, err
	}

	return &RWSet{RWSet: rws}, nil
}
