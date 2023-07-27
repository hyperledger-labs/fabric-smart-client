/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package web

import (
	"encoding/json"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/server/view/protos"
	view2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
	"github.com/pkg/errors"
)

type viewCallFunc func(context *ReqContext, vid string, input []byte) (interface{}, error)

func (vcf viewCallFunc) CallView(context *ReqContext, vid string, input []byte) (interface{}, error) {
	return vcf(context, vid, input)
}

type viewHandler struct {
	logger logger
	sp     view.ServiceProvider
}

func (s *viewHandler) CallView(context *ReqContext, vid string, input []byte) (interface{}, error) {
	s.logger.Debugf("Call view [%s] on input [%v]", vid, string(input))

	viewManager := view.GetManager(s.sp)
	f, err := viewManager.NewView(vid, input)
	if err != nil {
		return nil, errors.Errorf("failed instantiating view [%s], err [%s]", vid, err)
	}
	result, err := viewManager.InitiateView(f)
	if err != nil {
		return nil, errors.Errorf("failed running view [%s], err %s", vid, err)
	}
	raw, ok := result.([]byte)
	if !ok {
		raw, err = json.Marshal(result)
		if err != nil {
			return nil, errors.Errorf("failed marshalling result produced by view [%s], err [%s]", vid, err)
		}
	}
	s.logger.Debugf("Finished call view [%s] on input [%v]", vid, string(input))
	return &protos.CommandResponse_CallViewResponse{CallViewResponse: &protos.CallViewResponse{
		Result: raw,
	}}, nil
}

func (s *viewHandler) StreamCallView(context *ReqContext, vid string, input []byte) (interface{}, error) {
	s.logger.Debugf("Call view [%s] on input [%v]", vid, string(input))

	viewManager := view.GetManager(s.sp)
	f, err := viewManager.NewView(vid, input)
	if err != nil {
		return nil, errors.Errorf("failed instantiating view [%s], err [%s]", vid, err)
	}
	viewContext, err := viewManager.InitiateContext(f)
	if err != nil {
		return nil, errors.Errorf("failed running view [%s], err %s", vid, err)
	}
	mutable, ok := viewContext.Context.(view2.MutableContext)
	if !ok {
		return nil, errors.Errorf("expected a mutable contexdt")
	}
	webSocket, err := NewSocketConn(s.logger, context.ResponseWriter, context.Req)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create web socket")
	}
	if err := mutable.PutService(webSocket); err != nil {
		return nil, errors.Errorf("failed registering stream command server")
	}
	result, err := viewContext.RunView(f)
	if err != nil {
		return nil, errors.Errorf("failed running view [%s], err %s", vid, err)
	}
	raw, ok := result.([]byte)
	if !ok {
		raw, err = json.Marshal(result)
		if err != nil {
			return nil, errors.Errorf("failed marshalling result produced by view [%s], err [%s]", vid, err)
		}
	}
	s.logger.Debugf("Finished call view [%s] on input [%v]", vid, string(input))
	return &protos.CommandResponse_CallViewResponse{CallViewResponse: &protos.CallViewResponse{
		Result: raw,
	}}, nil
}

func InstallViewHandler(l logger, sp view.ServiceProvider, h *HttpHandler) {
	fh := &viewHandler{logger: l, sp: sp}

	d := &Dispatcher{Logger: l, Handler: h}
	d.WireViewCaller(viewCallFunc(fh.CallView))

	d = &Dispatcher{Logger: l, Handler: h}
	d.WireStreamViewCaller(viewCallFunc(fh.StreamCallView))
}
