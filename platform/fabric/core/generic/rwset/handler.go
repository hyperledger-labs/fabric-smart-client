/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package rwset

import (
	"fmt"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/core/generic/vault"
	vault2 "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/vault"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger/fabric-protos-go/common"
)

type endorserTransactionHandler struct {
	network, channel string
	v                driver.RWSetInspector
}

func NewEndorserTransactionHandler(network, channel string, v driver.RWSetInspector) driver.RWSetPayloadHandler {
	return &endorserTransactionHandler{network: network, channel: channel, v: v}
}

func (h *endorserTransactionHandler) Load(payl *common.Payload, chdr *common.ChannelHeader) (driver.RWSet, driver.ProcessTransaction, error) {
	upe, err := UnpackEnvelopeFromPayloadAndCHHeader(h.network, payl, chdr)
	if err != nil {
		return nil, nil, fmt.Errorf("failed unpacking envelope [%s]: %w", chdr.TxId, err)
	}

	logger.Debugf("retrieve rws [%s,%s]", h.channel, chdr.TxId)

	rws, err := h.v.GetRWSet(chdr.TxId, upe.Results)
	if err != nil {
		return nil, nil, err
	}
	return rws, upe, nil
}

type endorserTransactionReader struct {
	network   string
	populator vault.Populator
}

func NewEndorserTransactionReader(network string) *endorserTransactionReader {
	return &endorserTransactionReader{
		network:   network,
		populator: vault2.NewPopulator(),
	}
}

func (h *endorserTransactionReader) Read(payl *common.Payload, chdr *common.ChannelHeader) (vault.ReadWriteSet, error) {
	upe, err := UnpackEnvelopeFromPayloadAndCHHeader(h.network, payl, chdr)
	if err != nil {
		return vault.ReadWriteSet{}, fmt.Errorf("failed unpacking envelope [%s]: %w", chdr.TxId, err)
	}

	logger.Debugf("retrieve rws [%s,%s]", h.network, chdr.TxId)

	return h.populator.Populate(upe.Results)
}
