/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package pvt

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/flogging"
	"github.com/pkg/errors"
)

var logger = flogging.MustGetLogger("fabric-sdk.pvt")

type Processor struct {
	network            *fabric.NetworkService
	transactionManager *fabric.TransactionManager
}

func NewProcessor(network *fabric.NetworkService) *Processor {
	return &Processor{network: network, transactionManager: network.TransactionManager()}
}

func (p *Processor) Process(req fabric.Request, tx fabric.ProcessTransaction, rws *fabric.RWSet, ns string) error {
	logger.Debugf("process namespace [%s]", ns)
	k, _, err := rws.GetWriteAt("pvt", 0)
	if err != nil {
		return errors.Wrapf(err, "failed to get stored hash")
	}

	ch, err := p.network.Channel(tx.Channel())
	if err != nil {
		return errors.Wrapf(err, "failed to get channel [%s:%s]", tx.Network(), tx.Channel())
	}
	transactionService := ch.TransactionService()
	if !transactionService.Exists(k) {
		logger.Debugf("transaction [%s] not found", k)
	} else {
		logger.Debugf("transaction [%s] found, merge rws", k)
	}

	txRaw, err := transactionService.LoadTransaction(k)
	if err != nil {
		return errors.Wrapf(err, "failed to retrieve transaction [%s]", k)
	}

	//txRawHash := hash.Hashable(txRaw).String()
	//if string(txHash) != txRawHash {
	//	logger.Warningf("private transaction [%s] does not match hash [%s]!=[%s]", tx.ID(), txHash, txRawHash)
	//	// don't do anything here
	//	return nil
	//}

	storedTx, err := p.transactionManager.NewTransactionFromBytes(txRaw, fabric.WithChannel(tx.Channel()))
	if err != nil {
		return errors.WithMessagef(err, "failed to unmarshal transaction [%s]", k)
	}

	// TODO: Check Endorsement Policy

	// Append RW Set
	storedRWS, err := storedTx.GetRWSet()
	if err != nil {
		return errors.WithMessagef(err, "failed to get rws from transaction [%s]", k)
	}
	defer storedRWS.Done()
	rawRWS, err := storedRWS.Bytes()
	if err != nil {
		return errors.WithMessagef(err, "failed to get rws from transaction [%s]", k)
	}
	if err := rws.AppendRWSet(rawRWS); err != nil {
		return errors.WithMessagef(err, "failed to append rws from transaction [%s]", k)
	}

	return nil
}
