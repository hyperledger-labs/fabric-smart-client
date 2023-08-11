/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package web

import (
	"encoding/json"
	"net/http"

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
	c  *client
	sp view.ServiceProvider
}

func (s *viewHandler) CallView(context *ReqContext, vid string, input []byte) (interface{}, error) {
	s.c.viewManager = view.GetManager(s.sp)
	result, err := s.c.CallView(vid, input)
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
	return &protos.CommandResponse_CallViewResponse{CallViewResponse: &protos.CallViewResponse{
		Result: raw,
	}}, nil
}

func (s *viewHandler) StreamCallView(context *ReqContext, vid string, input []byte) (interface{}, error) {
	s.c.viewManager = view.GetManager(s.sp)
	return nil, s.c.StreamCallView(vid, context.ResponseWriter, context.Req)
}

func InstallViewHandler(l logger, sp view.ServiceProvider, h *HttpHandler) {
	fh := &viewHandler{c: &client{logger: l, viewManager: nil}, sp: sp}

	d := &Dispatcher{Logger: l, Handler: h}
	d.WireViewCaller(viewCallFunc(fh.CallView))

	d = &Dispatcher{Logger: l, Handler: h}
	d.WireStreamViewCaller(viewCallFunc(fh.StreamCallView))
}

type ViewClient interface {
	StreamCallView(fid string, writer http.ResponseWriter, request *http.Request) error
	CallView(fid string, in []byte) (interface{}, error)
}

type client struct {
	logger
	viewManager *view.Manager
}

func NewViewClient(logger logger, viewManager *view.Manager) ViewClient {
	return &client{logger: logger, viewManager: viewManager}
}

func (s *client) CallView(vid string, input []byte) (interface{}, error) {
	s.logger.Debugf("Call view [%s] on input [%v]", vid, string(input))

	f, err := s.viewManager.NewView(vid, input)
	if err != nil {
		return nil, errors.Errorf("failed instantiating view [%s], err [%s]", vid, err)
	}
	raw, err := s.viewManager.InitiateView(f)
	if err == nil {
		s.logger.Debugf("Finished call view [%s] on input [%v]", vid, string(input))
	}
	return raw, err
}

func (s *client) StreamCallView(vid string, writer http.ResponseWriter, request *http.Request) error {
	s.logger.Debugf("Call view [%s]", vid)

	// we need to retrieve the input to the factory from the web socket
	stream, err := NewWSStream(s.logger, writer, request)
	if err != nil {
		return errors.Wrapf(err, "failed to create web socket")
	}
	input, err := stream.ReadInput()
	if err != nil {
		return errors.Wrapf(err, "failed to read input")
	}

	f, err := s.viewManager.NewView(vid, input)
	if err != nil {
		return errors.Errorf("failed instantiating view [%s], err [%s]", vid, err)
	}
	viewContext, err := s.viewManager.InitiateContext(f)
	if err != nil {
		return errors.Errorf("failed instantiating context for view [%s], err %s", vid, err)
	}

	// register the web socket
	mutable, ok := viewContext.Context.(view2.MutableContext)
	if !ok {
		return errors.Errorf("expected a mutable contexdt")
	}
	if err := mutable.PutService(stream); err != nil {
		return errors.Errorf("failed registering stream command server")
	}
	// run the view
	result, err := viewContext.RunView(f)
	if err != nil {
		return errors.Errorf("failed running view [%s], err %s", vid, err)
	}
	raw, ok := result.([]byte)
	if !ok {
		raw, err = json.Marshal(result)
		if err != nil {
			return errors.Errorf("failed marshalling result produced by view [%s], err [%s]", vid, err)
		}
	}
	s.logger.Debugf("Finished call view [%s] on input [%v]", vid, string(input))

	// write back the result
	return stream.WriteResult(raw)
}
