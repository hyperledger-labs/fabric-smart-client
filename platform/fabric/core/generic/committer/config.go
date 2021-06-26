/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package committer

import (
	pb "github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/protoutil"
)

func (c *committer) handleConfig(block *pb.FilteredBlock, transactions []*pb.FilteredTransaction, i int, event *TxEvent) {
	tx := transactions[i]

	logger.Debugf("Committing config transaction [%s]", tx.Txid)

	if len(transactions) != 1 {
		logger.Panicf("Config block should contain only one transaction [%s]", tx.Txid)
	}

	switch tx.TxValidationCode {
	case pb.TxValidationCode_VALID:
		ledger, err := c.network.Ledger(c.channel)
		if err != nil {
			logger.Panicf("Cannot get Committer [%s]", err)
		}

		committer, err := c.network.Committer(c.channel)
		if err != nil {
			logger.Panicf("Cannot get Committer [%s]", err)
		}

		b, err := ledger.GetBlockByNumber(block.Number)
		if err != nil {
			logger.Panicf("Cannot get config transaction [%s]: [%s]", tx.Txid, err)
		}

		env, err := protoutil.UnmarshalEnvelope(b.DataAt(0))
		if err != nil {
			logger.Panicf("Cannot get payload from config transaction [%s]: [%s]", tx.Txid, err)
		}

		payload, err := protoutil.UnmarshalPayload(env.Payload)
		if err != nil {
			logger.Panicf("Cannot get payload from config transaction [%s]: [%s]", tx.Txid, err)
		}

		if err := committer.CommitConfig(block.Number, payload.Data); err != nil {
			logger.Panicf("Cannot commit config envelope [%s]", err)
		}
	}
}
