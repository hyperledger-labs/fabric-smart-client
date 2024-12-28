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
	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/pkg/errors"
	"go.opentelemetry.io/otel/trace/noop"
)

type txInfo struct {
	txID    driver2.TxID
	status  fabric.ValidationCode
	message string
}

func (i txInfo) TxID() driver2.TxID {
	return i.txID
}

type listenerEntry struct {
	l fabric.FinalityListener
}

func (e *listenerEntry) OnStatus(ctx context.Context, info txInfo) {
	e.l.OnStatus(ctx, info.txID, info.status, info.message)
}

func (e *listenerEntry) Equals(other ListenerEntry[txInfo]) bool {
	return other != nil && other.(*listenerEntry).l == e.l
}

func NewDeliveryBasedFLMWrapper(network string, ch *fabric.Channel) (*deliveryBasedFLMWrapper, error) {
	flm, err := NewDeliveryBasedFLM[txInfo](ch, &noop.Tracer{}, &txInfoMapper{network: network})
	if err != nil {
		return nil, err
	}
	return &deliveryBasedFLMWrapper{flm}, nil
}

type deliveryBasedFLMWrapper struct {
	flm *deliveryBasedFLM[txInfo]
}

func (m *deliveryBasedFLMWrapper) AddFinalityListener(txID string, listener fabric.FinalityListener) error {
	return m.flm.AddFinalityListener(txID, &listenerEntry{listener})
}

func (m *deliveryBasedFLMWrapper) RemoveFinalityListener(txID string, listener fabric.FinalityListener) error {
	return m.flm.RemoveFinalityListener(txID, &listenerEntry{listener})
}

type txInfoMapper struct {
	network string
}

func (m *txInfoMapper) Map(ctx context.Context, tx []byte, block *common.BlockMetadata, blockNum driver2.BlockNum, txNum driver2.TxNum) (map[driver2.Namespace]txInfo, error) {
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
	logger.Infof("TX [%s] has %d namespaces", chdr.TxId, len(rwSet.WriteSet.Writes))
	for ns, write := range rwSet.WriteSet.Writes {
		logger.Infof("TX [%s:%s] has %d writes", chdr.TxId, ns, len(write))
		txInfos[ns] = txInfo{
			txID:    chdr.TxId,
			status:  finalityEvent.ValidationCode,
			message: finalityEvent.ValidationMessage,
		}
	}
	return txInfos, nil
}
