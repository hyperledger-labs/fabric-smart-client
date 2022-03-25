/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package finality

import (
	"context"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric"
	"github.com/hyperledger-labs/fabric-smart-client/platform/orion"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/flogging"
	"github.com/pkg/errors"
	"reflect"

	"go.uber.org/zap/zapcore"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	view2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/server/view"
	protos2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/server/view/protos"
)

var logger = flogging.MustGetLogger("view-sdk.finality")

type Server interface {
	RegisterProcessor(typ reflect.Type, p view2.Processor)
}

type NetworkProvider interface {
	FabricNetworkService(network string) (driver.FabricNetworkService, error)
}

type finalityHandler struct {
	sp view.ServiceProvider
}

func InstallHandler(sp view.ServiceProvider, server Server) {
	fh := &finalityHandler{sp: sp}
	server.RegisterProcessor(reflect.TypeOf(&protos2.Command_IsTxFinal{}), fh.isTxFinal)
}

func (s *finalityHandler) isTxFinal(ctx context.Context, command *protos2.Command) (interface{}, error) {
	c := command.Payload.(*protos2.Command_IsTxFinal).IsTxFinal

	if logger.IsEnabledFor(zapcore.DebugLevel) {
		logger.Debugf("Answering: Is [%s] final on [%s:%s]?", c.Txid, c.Network, c.Channel)
	}

	checkOrion := true
	fns := fabric.GetFabricNetworkService(s.sp, c.Network)
	var isFinal func(string) error
	if fns != nil {
		ch, err := fns.Channel(c.Channel)
		if err == nil {
			checkOrion = false
			isFinal = ch.Finality().IsFinal
		} else {
			logger.Errorf("Failed to get channel [%s] on network [%s]: %s", c.Channel, c.Network, err)
		}
	}

	if checkOrion {
		ons := orion.GetOrionNetworkService(s.sp, c.Network)
		if ons == nil {
			return nil, errors.Errorf("no network service provider found for network [%s]", c.Network)
		}
		isFinal = ons.Finality().IsFinal
	}

	err := isFinal(c.Txid)
	if err != nil {
		if logger.IsEnabledFor(zapcore.DebugLevel) {
			logger.Debugf("Answering: Is [%s] final on [%s:%s]? No", c.Txid, c.Network, c.Channel)
		}
		return &protos2.CommandResponse_IsTxFinalResponse{IsTxFinalResponse: &protos2.IsTxFinalResponse{
			Payload: []byte(err.Error()),
		}}, nil
	}
	if logger.IsEnabledFor(zapcore.DebugLevel) {
		logger.Debugf("Answering: Is [%s] final on [%s:%s]? Yes", c.Txid, c.Network, c.Channel)
	}
	return &protos2.CommandResponse_IsTxFinalResponse{IsTxFinalResponse: &protos2.IsTxFinalResponse{}}, nil
}
