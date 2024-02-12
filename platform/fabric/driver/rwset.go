/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package driver

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/core"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/core/generic/vault"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger/fabric-protos-go/common"
)

type (
	GetStateOpt                 = driver.GetStateOpt
	RWSet                       = driver.RWSet
	RWSetPayloadHandlerProvider = func(network, channel string, v RWSetInspector) RWSetPayloadHandler
)

type RWSetInspector interface {
	GetRWSet(txid string, rwset []byte) (vault.TxInterceptor, error)
	InspectRWSet(rwsetBytes []byte, namespaces ...core.Namespace) (RWSet, error)
	GetExistingRWSet(txID core.TxID) (vault.TxInterceptor, error)
	NewRWSet(txid string) (vault.TxInterceptor, error)
	RWSExists(txid string) bool
}

type RWSetPayloadHandler interface {
	Load(payl *common.Payload, header *common.ChannelHeader) (RWSet, ProcessTransaction, error)
}

type RWSetLoader interface {
	AddHandlerProvider(headerType common.HeaderType, handlerProvider RWSetPayloadHandlerProvider) error
	GetRWSetFromEvn(txID string) (RWSet, ProcessTransaction, error)
	GetRWSetFromETx(txID string) (RWSet, ProcessTransaction, error)
	GetInspectingRWSetFromEvn(id string, envelopeRaw []byte) (RWSet, ProcessTransaction, error)
}
