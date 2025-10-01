/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package rwset

import (
	"context"
	"fmt"
	"strings"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	driver2 "github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/fabricutils"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	cb "github.com/hyperledger/fabric-protos-go-apiv2/common"
	"go.opentelemetry.io/otel/trace"
)

var logger = logging.MustGetLogger()

type Loader struct {
	Network            string
	Channel            string
	EnvelopeService    driver.EnvelopeService
	TransactionService driver.EndorserTransactionService
	TransactionManager driver.TransactionManager

	Vault    driver.RWSetInspector
	handlers map[cb.HeaderType]driver.RWSetPayloadHandler
}

func NewLoader(
	network string,
	channel string,
	envelopeService driver.EnvelopeService,
	transactionService driver.EndorserTransactionService,
	transactionManager driver.TransactionManager,
	vault driver.RWSetInspector,
) driver.RWSetLoader {
	return &Loader{
		Network:            network,
		Channel:            channel,
		EnvelopeService:    envelopeService,
		TransactionService: transactionService,
		TransactionManager: transactionManager,
		Vault:              vault,
		handlers:           map[cb.HeaderType]driver.RWSetPayloadHandler{},
	}
}

func (c *Loader) AddHandlerProvider(headerType cb.HeaderType, handlerProvider driver.RWSetPayloadHandlerProvider) error {
	if handler, ok := c.handlers[headerType]; ok {
		return fmt.Errorf("handler %T already defined for header type %v", handler, headerType)
	}

	c.handlers[headerType] = handlerProvider(c.Network, c.Channel, c.Vault)
	return nil
}

func (c *Loader) GetRWSetFromEvn(ctx context.Context, txID driver2.TxID) (driver.RWSet, driver.ProcessTransaction, error) {
	span := trace.SpanFromContext(ctx)
	span.AddEvent("start_get_rwset_from_evn")
	defer span.AddEvent("end_get_rwset_from_evn")

	if !c.EnvelopeService.Exists(ctx, txID) {
		return nil, nil, fmt.Errorf("envelope does not exists for [%s]", txID)
	}

	rawEnv, err := c.EnvelopeService.LoadEnvelope(ctx, txID)
	if err != nil {
		return nil, nil, fmt.Errorf("cannot load envelope [%s]: %w", txID, err)
	}

	_, payl, chdr, err := fabricutils.UnmarshalTx(rawEnv)
	if err != nil {
		return nil, nil, err
	}

	rws, err := c.Vault.NewRWSetFromBytes(ctx, chdr.TxId, payl.Data)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create new rws for [%s]: %w", chdr.TxId, err)
	}

	var function string
	if anyKeyContains(rws, "initialized") {
		function = "init"
	}

	logger.Debugf("retrieved processed transaction from env [%s,%s]", txID, function)
	pt := &processedTransaction{
		network:  c.Network,
		channel:  chdr.ChannelId,
		id:       chdr.TxId,
		function: function,
	}

	return rws, pt, nil
}

func (c *Loader) GetRWSetFromETx(ctx context.Context, txID driver2.TxID) (driver.RWSet, driver.ProcessTransaction, error) {
	span := trace.SpanFromContext(ctx)
	span.AddEvent("start_get_rwset_from_etx")
	defer span.AddEvent("end_get_rwset_from_etx")

	if !c.TransactionService.Exists(ctx, txID) {
		return nil, nil, fmt.Errorf("transaction does not exists for [%s]", txID)
	}

	raw, err := c.TransactionService.LoadTransaction(ctx, txID)
	if err != nil {
		return nil, nil, fmt.Errorf("cannot load etx [%s]: %w", txID, err)
	}

	tx, err := c.TransactionManager.NewTransactionFromBytes(ctx, c.Channel, raw)
	if err != nil {
		return nil, nil, err
	}

	rws, err := tx.GetRWSet()
	if err != nil {
		return nil, nil, err
	}

	return rws, tx, nil
}

func (c *Loader) GetInspectingRWSetFromEvn(ctx context.Context, txID driver2.TxID, envelopeRaw []byte) (driver.RWSet, driver.ProcessTransaction, error) {
	span := trace.SpanFromContext(ctx)
	span.AddEvent("start_get_inspecting_rwset_from_evn")
	defer span.AddEvent("end_get_inspecting_rwset_from_evn")

	logger.Debugf("retrieve rwset from envelope [%s,%s]", c.Channel, txID)

	_, payl, chdr, err := fabricutils.UnmarshalTx(envelopeRaw)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "cannot unmarshal envelope [%s]", txID)
	}

	rws, err := c.Vault.InspectRWSet(ctx, payl.Data)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "cannot inspect rwset for [%s]", txID)
	}

	var function string
	if anyKeyContains(rws, "initialized") {
		function = "init"
	}

	logger.Debugf("retrieved inspecting processed transaction from env [%s,%s]", txID, function)
	pt := &processedTransaction{
		network:  c.Network,
		channel:  chdr.ChannelId,
		id:       txID,
		function: function,
	}

	return rws, pt, nil
}

func anyKeyContains(rws driver.RWSet, substr string) bool {
	for _, ns := range rws.Namespaces() {
		for pos := range rws.NumReads(ns) {
			if k, err := rws.GetReadKeyAt(ns, pos); err == nil && strings.Contains(k, substr) {
				return true
			}
		}
	}
	return false
}

type processedTransaction struct {
	network  string
	channel  string
	id       string
	function string
	params   []string
}

func (pt *processedTransaction) Network() string {
	return pt.network
}

func (pt *processedTransaction) Channel() string {
	return pt.channel
}

func (pt *processedTransaction) ID() string {
	return pt.id
}

func (pt *processedTransaction) FunctionAndParameters() (string, []string) {
	return pt.function, pt.params
}
