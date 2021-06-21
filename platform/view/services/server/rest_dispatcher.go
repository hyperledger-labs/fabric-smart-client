/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package server

import (
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/rest"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/server/protos"
)

type ViewCaller interface {
	CallView(command *protos.Command) (interface{}, error)
}

type RestDispatcher struct {
	vc      ViewCaller
	Handler *rest.HttpHandler
}

func (rd *RestDispatcher) HandleRequest(context *rest.ReqContext) (response interface{}, statusCode int) {
	logger.Infof("Received request from %s", context.Req.Host)

	if rd.vc == nil {
		logger.Errorf("ViewCaller has not been initialized yet")
		return &rest.ResponseErr{Reason: "internal error"}, 500
	}

	cmd := &protos.Command{
		Header: &protos.Header{
			ChannelId: context.Vars["Channel"],
		},
		Payload: &protos.Command_CallView{
			CallView: &protos.CallView{
				Fid:   context.Vars["View"],
				Input: context.Query.([]byte),
			},
		},
	}

	res, err := rd.vc.CallView(cmd)
	if err != nil {
		return &rest.ResponseErr{Reason: err.Error()}, 500
	}

	return res, 200
}

func (rd *RestDispatcher) ParsePayload(bytes []byte) (interface{}, error) {
	return bytes, nil
}

func (rd *RestDispatcher) WireViewCaller(vc ViewCaller) {
	rd.vc = vc
	rd.Handler.RegisterURI("/{Channel}/Views/{View}", "PUT", rd)
}
