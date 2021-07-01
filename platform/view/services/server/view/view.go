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

	"github.com/pkg/errors"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view"
	protos2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/server/view/protos"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/tracker"
)

type viewHandler struct {
	sp view.ServiceProvider
}

func InstallViewHandler(sp view.ServiceProvider, server Service) {
	fh := &viewHandler{sp: sp}
	server.RegisterProcessor(reflect.TypeOf(&protos2.Command_InitiateView{}), fh.initiateView)
	server.RegisterProcessor(reflect.TypeOf(&protos2.Command_TrackView{}), fh.trackView)
	server.RegisterProcessor(reflect.TypeOf(&protos2.Command_CallView{}), fh.callView)
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

func (s *viewHandler) trackView(ctx context.Context, command *protos2.Command) (interface{}, error) {
	trackView := command.Payload.(*protos2.Command_TrackView).TrackView

	cid := trackView.Cid
	log.Printf("Track context [%s]", cid)

	_, err := view.GetManager(s.sp).Context(cid)
	if err != nil {
		return nil, errors.Errorf("failed retrieving context [%s], err [%s]", cid, err)
	}
	viewTracker, err := tracker.GetViewTracker(s.sp)
	if err != nil {
		return nil, errors.Errorf("failed getting tracker for context [%s], err [%s]", cid, err)
	}
	payload, err := json.Marshal(viewTracker.ViewStatus())
	if err != nil {
		return nil, errors.Errorf("failed marshalling view status for context [%s], err [%s]", cid, err)
	}
	log.Printf("Context id '%s', status '%s'\n", cid, string(payload))

	return &protos2.CommandResponse_TrackViewResponse{TrackViewResponse: &protos2.TrackViewResponse{
		Payload: payload,
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
	logger.Debugf("Finished call view [%s] on channel [%s] and on input [%v]", fid, string(input))
	return &protos2.CommandResponse_CallViewResponse{CallViewResponse: &protos2.CallViewResponse{
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
