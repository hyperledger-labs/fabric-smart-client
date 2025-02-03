/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package finality

import (
	"context"

	driver2 "github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/committer"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/fabricutils"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/rwset"
	fdriver "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/pkg/errors"
	"go.opentelemetry.io/otel/trace/noop"
)

type txInfo struct {
	txID    driver2.TxID
	status  fdriver.ValidationCode
	message string
}

func (i txInfo) TxID() driver2.TxID {
	return i.txID
}

type deliveryListenerEntry struct {
	l fabric.FinalityListener
}

func (e *deliveryListenerEntry) OnStatus(ctx context.Context, info txInfo) {
	e.l.OnStatus(ctx, info.txID, info.status, info.message)
}

func (e *deliveryListenerEntry) Equals(other ListenerEntry[txInfo]) bool {
	return other != nil && other.(*deliveryListenerEntry).l == e.l
}

func NewDeliveryFLM(config DeliveryListenerManagerConfig, network string, ch *fabric.Channel) (*deliveryListenerManager, error) {
	flm, err := NewListenerManager[txInfo](config, ch.Delivery(), &noop.Tracer{}, &txInfoMapper{network: network})
	if err != nil {
		return nil, err
	}
	return &deliveryListenerManager{flm}, nil
}

type deliveryListenerManager struct {
	flm *listenerManager[txInfo]
}

func (m *deliveryListenerManager) AddFinalityListener(txID string, listener fabric.FinalityListener) error {
	return m.flm.AddFinalityListener(txID, &deliveryListenerEntry{listener})
}

func (m *deliveryListenerManager) RemoveFinalityListener(txID string, listener fabric.FinalityListener) error {
	return m.flm.RemoveFinalityListener(txID, &deliveryListenerEntry{listener})
}

type txInfoMapper struct {
	network string
}

func (m *txInfoMapper) MapTxData(ctx context.Context, tx []byte, block *common.BlockMetadata, blockNum driver2.BlockNum, txNum driver2.TxNum) (map[driver2.Namespace]txInfo, error) {
	_, payl, chdr, err := fabricutils.UnmarshalTx(tx)
	if err != nil {
		return nil, errors.Wrapf(err, "failed unmarshaling tx [%d:%d]", blockNum, txNum)
	}
	if common.HeaderType(chdr.Type) != common.HeaderType_ENDORSER_TRANSACTION {
		logger.Warnf("Type of TX [%d:%d] is [%d]. Skipping...", blockNum, txNum, chdr.Type)
		return nil, nil
	}
	rwSet, err := rwset.NewEndorserTransactionReader(m.network).Read(payl, chdr)
	if err != nil {
		return nil, errors.Wrapf(err, "failed extracting rwset")
	}
	_, finalityEvent, err := committer.MapFinalityEvent(ctx, block, txNum, chdr.TxId)
	if err != nil {
		return nil, errors.Wrapf(err, "failed mapping finality event")
	}

	txInfos := make(map[driver2.Namespace]txInfo, len(rwSet.WriteSet.Writes))
	logger.Debugf("TX [%s] has %d namespaces", chdr.TxId, len(rwSet.WriteSet.Writes))
	for ns, write := range rwSet.WriteSet.Writes {
		logger.Debugf("TX [%s:%s] has %d writes", chdr.TxId, ns, len(write))
		txInfos[ns] = txInfo{
			txID:    chdr.TxId,
			status:  finalityEvent.ValidationCode,
			message: finalityEvent.ValidationMessage,
		}
	}
	return txInfos, nil
}

func (m *txInfoMapper) MapProcessedTx(tx *fabric.ProcessedTransaction) ([]txInfo, error) {
	status, message := committer.MapValidationCode(tx.ValidationCode())
	return []txInfo{{
		txID:    tx.TxID(),
		status:  status,
		message: message,
	}}, nil
}
