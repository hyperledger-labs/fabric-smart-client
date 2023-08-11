/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package view

import (
	"context"
	"encoding/json"
	"log"
	"reflect"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view"
	protos2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/server/view/protos"
	view2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
	"github.com/pkg/errors"
)

type viewHandler struct {
	sp view.ServiceProvider
}

func InstallViewHandler(sp view.ServiceProvider, server Service) {
	fh := &viewHandler{sp: sp}
	server.RegisterProcessor(reflect.TypeOf(&protos2.Command_InitiateView{}), fh.initiateView)
	server.RegisterProcessor(reflect.TypeOf(&protos2.Command_CallView{}), fh.callView)
	server.RegisterStreamer(reflect.TypeOf(&protos2.Command_CallView{}), fh.streamCallView)
}

func (s *viewHandler) initiateView(ctx context.Context, command *protos2.Command) (interface{}, error) {
	initiateView := command.Payload.(*protos2.Command_InitiateView).InitiateView

	fid := initiateView.Fid
	input := initiateView.Input
	log.Printf("Initiate view [%s]", fid)

	viewManager := view.GetManager(s.sp)
	f, err := viewManager.NewView(fid, input)
	if err != nil {
		return nil, errors.Errorf("failed instantiating view [%s], err [%s]", fid, err)
	}
	contextID, err := s.RunView(viewManager, f)
	if err != nil {
		return nil, errors.Errorf("failed running view [%s], err %s", fid, err)
	}
	return &protos2.CommandResponse_InitiateViewResponse{InitiateViewResponse: &protos2.InitiateViewResponse{
		Cid: contextID,
	}}, nil
}

func (s *viewHandler) callView(ctx context.Context, command *protos2.Command) (interface{}, error) {
	callView := command.Payload.(*protos2.Command_CallView).CallView

	fid := callView.Fid
	input := callView.Input
	logger.Debugf("Call view [%s] on input [%v]", fid, string(input))

	viewManager := view.GetManager(s.sp)
	f, err := viewManager.NewView(fid, input)
	if err != nil {
		return nil, errors.Errorf("failed instantiating view [%s], err [%s]", fid, err)
	}
	result, err := viewManager.InitiateView(f)
	if err != nil {
		return nil, errors.Errorf("failed running view [%s], err %s", fid, err)
	}
	raw, ok := result.([]byte)
	if !ok {
		raw, err = json.Marshal(result)
		if err != nil {
			return nil, errors.Errorf("failed marshalling result produced by view [%s], err [%s]", fid, err)
		}
	}
	logger.Debugf("Finished call view [%s] on input [%v]", fid, string(input))
	return &protos2.CommandResponse_CallViewResponse{CallViewResponse: &protos2.CallViewResponse{
		Result: raw,
	}}, nil
}

func (s *viewHandler) streamCallView(sc *protos2.SignedCommand, command *protos2.Command, commandServer protos2.ViewService_StreamCommandServer, marshaller Marshaller) error {
	callView := command.Payload.(*protos2.Command_CallView).CallView

	fid := callView.Fid
	input := callView.Input
	logger.Debugf("Stream call view [%s] on input [%v]", fid, string(input))

	viewManager := view.GetManager(s.sp)
	f, err := viewManager.NewView(fid, input)
	if err != nil {
		return errors.Errorf("failed instantiating view [%s], err [%s]", fid, err)
	}
	context, err := viewManager.InitiateContext(f)
	if err != nil {
		return errors.Errorf("failed running view [%s], err %s", fid, err)
	}
	mutable, ok := context.Context.(view2.MutableContext)
	if !ok {
		return errors.Errorf("expected a mutable contexdt")
	}
	if err := mutable.PutService(&Stream{scs: commandServer}); err != nil {
		return errors.Errorf("failed registering stream command server")
	}

	result, err := context.RunView(f)
	if err != nil {
		return errors.Errorf("failed running view [%s], err %s", fid, err)
	}
	raw, ok := result.([]byte)
	if !ok {
		raw, err = json.Marshal(result)
		if err != nil {
			return errors.Errorf("failed marshalling result produced by view [%s], err [%s]", fid, err)
		}
	}
	logger.Debugf("Finished stream call view [%s] on input [%v]", fid, string(input))
	logger.Debugf("Preparing response")
	cr, err := marshaller.MarshalCommandResponse(
		sc.Command,
		&protos2.CommandResponse_CallViewResponse{CallViewResponse: &protos2.CallViewResponse{
			Result: raw,
		}},
	)
	if err != nil {
		return errors.Wrapf(err, "failed to marshal command response for [%s]", fid)
	}
	logger.Debugf("Done with err [%s]", err)
	return commandServer.Send(cr)
}

func (s *viewHandler) RunView(manager *view.Manager, view view.View) (string, error) {
	context, err := manager.InitiateContext(view)
	if err != nil {
		return "", err
	}

	// Run the view
	go s.runView(view, context)

	return context.ID(), nil
}

func (s *viewHandler) runView(view view.View, context *view.Context) {
	result, err := context.RunView(view)
	if err != nil {
		logger.Errorf("Failed view execution. Err [%s]\n", err)
	} else {
		logger.Infof("Successful view execution. Result [%s]\n", result)
	}
}

type Stream struct {
	scs protos2.ViewService_StreamCommandServer
}

func (c *Stream) Send(m interface{}) error {
	raw, err := json.Marshal(m)
	if err != nil {
		return err
	}
	s := &protos2.CallViewResponse{
		Result: raw,
	}
	return c.SendProtoMsg(s)
}

func (c *Stream) Recv(m interface{}) error {
	s := &protos2.CallViewResponse{}
	if err := c.RecvProtoMsg(s); err != nil {
		return err
	}
	return json.Unmarshal(s.Result, m)
}

func (c *Stream) SendProtoMsg(m interface{}) error {
	return c.scs.SendMsg(m)
}

func (c *Stream) RecvProtoMsg(m interface{}) error {
	return c.scs.RecvMsg(m)
}
