/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package rwset

import (
	"fmt"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger/fabric-protos-go/common"
)

func NewEndorserTransactionHandler(network, channel string, v driver.RWSetInspector) driver.RWSetPayloadHandler {
	return &endorserTransactionHandler{network: network, channel: channel, v: v}
}

type endorserTransactionHandler struct {
	network, channel string
	v                driver.RWSetInspector
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
