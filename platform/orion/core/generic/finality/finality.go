/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package finality

import (
	"context"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/orion/core/generic/committer"
	"github.com/hyperledger-labs/fabric-smart-client/platform/orion/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
	errors2 "github.com/pkg/errors"
)

type Committer interface {
	// IsFinal takes in input a transaction id and waits for its confirmation.
	IsFinal(ctx context.Context, txID string) error
}

type Network interface {
	SessionManager() driver.SessionManager
	IdentityManager() driver.IdentityManager
}

type finality struct {
	committer Committer
	network   Network
}

func NewService(committer Committer, network Network) (*finality, error) {
	return &finality{
		committer: committer,
		network:   network,
	}, nil
}

func (f *finality) IsFinal(ctx context.Context, txID string) error {
	if ctx == nil {
		ctx = context.Background()
	}

	// ask the committer first
	err := f.committer.IsFinal(ctx, txID)
	if err == nil {
		return nil
	}

	// if the transaction is unknown, then check the custodian
	var err2 error
	if errors.HasCause(err, committer.ErrUnknownTX) {
		// ask the ledger
		s, err2 := f.network.SessionManager().NewSession(f.network.IdentityManager().Me())
		if err2 == nil {
			l, err2 := s.Ledger()
			if err2 == nil {
				flag, err2 := l.GetTransactionReceipt(txID)
				if err2 == nil {
					if flag == driver.VALID {
						return nil
					}
					return errors2.Errorf("invalid transaction [%s]", txID)
				}
			}
		}
	}
	return errors2.Errorf("failed retrieveing transaction finality for [%s]: [%s][%s]", txID, err, err2)
}

func (f *finality) IsFinalForParties(txID string, parties ...view.Identity) error {
	panic("implement me")
}
