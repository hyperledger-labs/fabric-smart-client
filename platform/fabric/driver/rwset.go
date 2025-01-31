/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package driver

import (
	"context"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger/fabric-protos-go/common"
)

type (
	GetStateOpt                 = driver.GetStateOpt
	RWSet                       = driver.RWSet
	RWSetPayloadHandlerProvider = func(network, channel string, v RWSetInspector) RWSetPayloadHandler
)

type RWSetInspector interface {
	GetRWSet(ctx context.Context, txID driver.TxID, rwset []byte) (RWSet, error)
	InspectRWSet(ctx context.Context, rwsetBytes []byte, namespaces ...driver.Namespace) (RWSet, error)
	NewRWSet(ctx context.Context, txID driver.TxID) (RWSet, error)
	RWSExists(ctx context.Context, txID driver.TxID) bool
	GetExistingRWSet(ctx context.Context, txID driver.TxID) (driver.RWSet, error)
}

type RWSetPayloadHandler interface {
	Load(payl *common.Payload, header *common.ChannelHeader) (RWSet, ProcessTransaction, error)
}

type RWSetLoader interface {
	AddHandlerProvider(headerType common.HeaderType, handlerProvider RWSetPayloadHandlerProvider) error
	GetRWSetFromEvn(ctx context.Context, txID driver.TxID) (RWSet, ProcessTransaction, error)
	GetRWSetFromETx(ctx context.Context, txID driver.TxID) (RWSet, ProcessTransaction, error)
	GetInspectingRWSetFromEvn(ctx context.Context, id driver.TxID, envelopeRaw []byte) (RWSet, ProcessTransaction, error)
}
