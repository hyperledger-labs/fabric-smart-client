/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package finality

import (
	"context"

	driver2 "github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/committer"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/events"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/fabricutils"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/rwset"
	fdriver "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger/fabric-protos-go-apiv2/common"
	"github.com/pkg/errors"
	"go.opentelemetry.io/otel/trace/noop"
)

type ListenerManager[T events.EventInfo] interface {
	AddEventListener(txID string, e events.ListenerEntry[T]) error
	RemoveEventListener(txID string, e events.ListenerEntry[T]) error
}

type txInfo struct {
	txID    driver2.TxID
	status  fdriver.ValidationCode
	message string
}

func (i txInfo) ID() events.EventID {
	return i.txID
}

type deliveryListenerEntry struct {
	l fabric.FinalityListener
}

func (e *deliveryListenerEntry) Namespace() driver2.Namespace {
	return ""
}

func (e *deliveryListenerEntry) OnStatus(ctx context.Context, info txInfo) {
	e.l.OnStatus(ctx, info.txID, info.status, info.message)
}

func (e *deliveryListenerEntry) Equals(other events.ListenerEntry[txInfo]) bool {
	return other != nil && other.(*deliveryListenerEntry).l == e.l
}

func NewDeliveryFLM(logger logging.Logger, config events.DeliveryListenerManagerConfig, network string, ch *fabric.Channel) (*deliveryListenerManager, error) {
	mapper := &txInfoMapper{logger: logger, network: network}
	delivery := ch.Delivery()
	queryService := &DeliveryScanQueryByID[txInfo]{Logger: logger, Delivery: delivery, Mapper: mapper}
	flm, err := events.NewListenerManager[txInfo](logger, config, delivery, queryService, &noop.Tracer{}, mapper)
	if err != nil {
		return nil, err
	}
	return &deliveryListenerManager{flm}, nil
}

type deliveryListenerManager struct {
	flm ListenerManager[txInfo]
}

func (m *deliveryListenerManager) AddFinalityListener(txID string, listener fabric.FinalityListener) error {
	return m.flm.AddEventListener(txID, &deliveryListenerEntry{listener})
}

func (m *deliveryListenerManager) RemoveFinalityListener(txID string, listener fabric.FinalityListener) error {
	return m.flm.RemoveEventListener(txID, &deliveryListenerEntry{listener})
}

type txInfoMapper struct {
	network string
	logger  logging.Logger
}

func (m *txInfoMapper) MapTxData(ctx context.Context, tx []byte, block *common.BlockMetadata, blockNum driver2.BlockNum, txNum driver2.TxNum) (map[driver2.Namespace]txInfo, error) {
	_, payl, chdr, err := fabricutils.UnmarshalTx(tx)
	if err != nil {
		return nil, errors.Wrapf(err, "failed unmarshaling tx [%d:%d]", blockNum, txNum)
	}
	if common.HeaderType(chdr.Type) != common.HeaderType_ENDORSER_TRANSACTION {
		m.logger.Warnf("Type of TX [%d:%d] is [%d]. Skipping...", blockNum, txNum, chdr.Type)
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
	m.logger.Debugf("TX [%s] has %d namespaces", chdr.TxId, len(rwSet.WriteSet.Writes))
	for ns, write := range rwSet.WriteSet.Writes {
		m.logger.Debugf("TX [%s:%s] has %d writes", chdr.TxId, ns, len(write))
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
