/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package web

import (
	"encoding/json"
	"log"

	"github.com/pkg/errors"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view"
	protos2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/server/view/protos"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/tracker"
)

type viewHandler struct {
	logger logger
	sp     view.ServiceProvider
}

type viewCallFunc func(fid string, input []byte) (interface{}, error)

func (vcf viewCallFunc) CallView(fid string, input []byte) (interface{}, error) {
	return vcf(fid, input)
}

type dispatcher interface {
	WireViewCaller(vc ViewCaller)
}

func InstallViewHandler(l logger, sp view.ServiceProvider, d dispatcher) {
	fh := &viewHandler{logger: l, sp: sp}
	d.WireViewCaller(viewCallFunc(fh.callView))
}

func (s *viewHandler) callView(fid string, input []byte) (interface{}, error) {
	s.logger.Debugf("Call view [%s] on input [%v]", fid, string(input))

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
	s.logger.Debugf("Finished call view [%s] on channel [%s] and on input [%v]", fid, string(input))
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
