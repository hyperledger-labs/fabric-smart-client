/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package server

import (
	"context"
	"encoding/json"
	"log"
	"reflect"

	"github.com/pkg/errors"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/server/protos"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/tracker"
)

type viewHandler struct {
	sp view.ServiceProvider
}

type viewCallFunc func(ctx context.Context, command *protos.Command) (interface{}, error)

func (vcf viewCallFunc) CallView(command *protos.Command) (interface{}, error) {
	return vcf(context.Background(), command)
}

type Dispatcher interface {
	WireViewCaller(vc ViewCaller)
}

func InstallViewHandler(sp view.ServiceProvider, server Server, d Dispatcher) {
	fh := &viewHandler{sp: sp}
	server.RegisterProcessor(reflect.TypeOf(&protos.Command_InitiateView{}), fh.initiateView)
	server.RegisterProcessor(reflect.TypeOf(&protos.Command_TrackView{}), fh.trackView)
	server.RegisterProcessor(reflect.TypeOf(&protos.Command_CallView{}), fh.callView)
	d.WireViewCaller(viewCallFunc(fh.callView))
}

func (s *viewHandler) initiateView(ctx context.Context, command *protos.Command) (interface{}, error) {
	header := command.Header
	initiateView := command.Payload.(*protos.Command_InitiateView).InitiateView

	channelID := header.ChannelId
	fid := initiateView.Fid
	input := initiateView.Input
	log.Printf("Initiate view %s on channel %s\n", fid, channelID)

	viewManager := view.GetManager(s.sp)
	f, err := viewManager.NewView(fid, input)
	if err != nil {
		return nil, errors.Errorf("failed instantiating view [%s] on channel [%s], err [%s]", fid, channelID, err)
	}
	contextID, err := s.RunView(viewManager, f)
	if err != nil {
		return nil, errors.Errorf("failed running view [%s] on channel [%s], err %s", fid, channelID, err)
	}
	return &protos.CommandResponse_InitiateViewResponse{InitiateViewResponse: &protos.InitiateViewResponse{
		Cid: contextID,
	}}, nil
}

func (s *viewHandler) trackView(ctx context.Context, command *protos.Command) (interface{}, error) {
	header := command.Header
	trackView := command.Payload.(*protos.Command_TrackView).TrackView

	channelID := header.ChannelId
	cid := trackView.Cid
	log.Printf("Track context %s on channel %s\n", cid, channelID)

	_, err := view.GetManager(s.sp).Context(cid)
	if err != nil {
		return nil, errors.Errorf("failed retrieving context %s on channel %s, err %s", cid, channelID, err)
	}
	viewTracker, err := tracker.GetViewTracker(s.sp)
	if err != nil {
		return nil, errors.Errorf("failed getting tracker for context %s on channel %s, err %s", cid, channelID, err)
	}
	payload, err := json.Marshal(viewTracker.ViewStatus())
	if err != nil {
		return nil, errors.Errorf("failed marshalling view status for context %s on channel %s, err %s", cid, channelID, err)
	}
	log.Printf("Context id '%s', status '%s'\n", cid, string(payload))

	return &protos.CommandResponse_TrackViewResponse{TrackViewResponse: &protos.TrackViewResponse{
		Payload: payload,
	}}, nil
}

func (s *viewHandler) callView(ctx context.Context, command *protos.Command) (interface{}, error) {
	header := command.Header
	callView := command.Payload.(*protos.Command_CallView).CallView

	channelID := header.ChannelId
	fid := callView.Fid
	input := callView.Input
	logger.Debugf("Call view [%s] on channel [%s] and on input [%v]", fid, channelID, string(input))

	viewManager := view.GetManager(s.sp)
	f, err := viewManager.NewView(fid, input)
	if err != nil {
		return nil, errors.Errorf("failed instantiating view [%s] on channel [%s], err [%s]", fid, channelID, err)
	}
	result, err := viewManager.InitiateView(f)
	if err != nil {
		return nil, errors.Errorf("failed running view [%s] on channel [%s], err %s", fid, channelID, err)
	}
	raw, ok := result.([]byte)
	if !ok {
		raw, err = json.Marshal(result)
		if err != nil {
			return nil, errors.Errorf("failed marshalling result produced by view %s on channel %s, err %s", fid, channelID, err)
		}
	}
	logger.Debugf("Finished call view [%s] on channel [%s] and on input [%v]", fid, channelID, string(input))
	return &protos.CommandResponse_CallViewResponse{CallViewResponse: &protos.CallViewResponse{
		Result: raw,
	}}, nil
}

func (s *viewHandler) RunView(manager *view.Manager, view view.View) (string, error) {
	context, err := manager.InitiateContext(view)
	if err != nil {
		return "", err
	}

	// Get the tracker
	viewTracker, err := tracker.GetViewTracker(s.sp)
	if err != nil {
		return "", err
	}

	// Run the view
	go s.runViewWithTracking(view, context, viewTracker)

	return context.ID(), nil
}

func (s *viewHandler) runViewWithTracking(view view.View, context *view.Context, tracker tracker.ViewTracker) {
	result, err := context.RunView(view)
	if err != nil {
		log.Printf("Failed view execution. Err [%s]\n", err)
		tracker.Error(err)
	} else {
		log.Printf("Successful view execution. Result [%s]\n", result)
		tracker.Done(result)
	}
}
