/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package rwset

import (
	"context"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/proto"
	driver2 "github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/protoutil"
	"github.com/hyperledger/fabric-protos-go-apiv2/common"
)

type Loader struct {
	Network            string
	Channel            string
	EnvelopeService    driver.EnvelopeService
	TransactionService driver.EndorserTransactionService
	TransactionManager driver.TransactionManager

	Vault    driver.RWSetInspector
	handlers map[common.HeaderType]driver.RWSetPayloadHandler
}

func NewLoader(
	network string,
	channel string,
	envelopeService driver.EnvelopeService,
	transactionService driver.EndorserTransactionService,
	transactionManager driver.TransactionManager,
	vault driver.RWSetInspector,
) *Loader {
	return &Loader{
		Network:            network,
		Channel:            channel,
		EnvelopeService:    envelopeService,
		TransactionService: transactionService,
		TransactionManager: transactionManager,
		Vault:              vault,
		handlers:           map[common.HeaderType]driver.RWSetPayloadHandler{},
	}
}

func (c *Loader) AddHandlerProvider(headerType common.HeaderType, handlerProvider driver.RWSetPayloadHandlerProvider) error {
	if handler, ok := c.handlers[headerType]; ok {
		return errors.Errorf("handler %T already defined for header type %v", handler, headerType)
	}
	c.handlers[headerType] = handlerProvider(c.Network, c.Channel, c.Vault)
	return nil
}

func (c *Loader) GetRWSetFromEvn(ctx context.Context, txID driver2.TxID) (driver.RWSet, driver.ProcessTransaction, error) {
	logger.DebugfContext(ctx, "Get RWSet from evn")
	defer logger.DebugfContext(ctx, "Got RWSet from evn")

	if !c.EnvelopeService.Exists(ctx, txID) {
		return nil, nil, errors.Errorf("envelope does not exists for [%s]", txID)
	}

	rawEnv, err := c.EnvelopeService.LoadEnvelope(ctx, txID)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "cannot load envelope [%s]", txID)
	}
	logger.Debugf("unmarshal envelope [%s,%s]", c.Channel, txID)
	env := &common.Envelope{}
	err = proto.Unmarshal(rawEnv, env)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "failed unmarshalling envelope [%s]", txID)
	}
	logger.Debugf("unpack envelope [%s,%s]", c.Channel, txID)

	payl, err := protoutil.UnmarshalPayload(env.Payload)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "VSCC error: GetPayload failed")
	}

	chdr, err := protoutil.UnmarshalChannelHeader(payl.Header.ChannelHeader)
	if err != nil {
		return nil, nil, err
	}

	if handler, ok := c.handlers[common.HeaderType(chdr.Type)]; ok {
		return handler.Load(payl, chdr)
	}
	return nil, nil, errors.Errorf("header type not support, provided type %d", chdr.Type)
}

func (c *Loader) GetRWSetFromETx(ctx context.Context, txID driver2.TxID) (driver.RWSet, driver.ProcessTransaction, error) {
	logger.DebugfContext(ctx, "Get RWSet from etx")
	defer logger.DebugfContext(ctx, "Got RWSet from etx")

	if !c.TransactionService.Exists(ctx, txID) {
		return nil, nil, errors.Errorf("transaction does not exists for [%s]", txID)
	}

	raw, err := c.TransactionService.LoadTransaction(ctx, txID)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "cannot load etx [%s]", txID)
	}
	tx, err := c.TransactionManager.NewTransactionFromBytes(context.TODO(), c.Channel, raw)
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
	logger.DebugfContext(ctx, "unmarshal envelope [%s,%s]", c.Channel, txID)
	env := &common.Envelope{}
	err := proto.Unmarshal(envelopeRaw, env)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "failed unmarshalling envelope [%s]", txID)
	}
	logger.Debugf("unpack envelope [%s,%s]", c.Channel, txID)
	upe, err := UnpackEnvelope(c.Network, env)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "failed unpacking envelope [%s]", txID)
	}
	logger.Debugf("retrieve rws [%s,%s]", c.Channel, txID)

	rws, err := c.Vault.InspectRWSet(ctx, upe.Results)
	if err != nil {
		return nil, nil, err
	}

	return rws, upe, nil
}
