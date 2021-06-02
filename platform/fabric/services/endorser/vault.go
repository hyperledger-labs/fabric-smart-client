/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package endorser

import (
	"hash/fnv"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric"
)

type commitInVault struct {
	transaction *Transaction
}

func (c commitInVault) Call(context view.Context) (interface{}, error) {
	vault := fabric.GetChannelDefaultNetwork(context, c.transaction.Channel()).Vault()

	h := fnv.New64()
	h.Write([]byte(c.transaction.ID()))
	block := h.Sum64()

	// TODO: Change this
	if err := vault.CommitTX(c.transaction.ID(), block, 0); err != nil {
		return nil, err
	}
	return nil, nil
}

func NewCommitInVault(transaction *Transaction) *commitInVault {
	return &commitInVault{transaction: transaction}
}
