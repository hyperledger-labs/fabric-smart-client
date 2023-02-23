/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package web

import (
	"encoding/json"

	"github.com/hyperledger-labs/fabric-smart-client/platform/view"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/server/view/protos"
	"github.com/pkg/errors"
)

type viewHandler struct {
	logger logger
	sp     view.ServiceProvider
}

type viewCallFunc func(vid string, input []byte) (interface{}, error)

func (vcf viewCallFunc) CallView(vid string, input []byte) (interface{}, error) {
	return vcf(vid, input)
}

type dispatcher interface {
	WireViewCaller(vc ViewCaller)
}

func InstallViewHandler(l logger, sp view.ServiceProvider, d dispatcher) {
	fh := &viewHandler{logger: l, sp: sp}
	d.WireViewCaller(viewCallFunc(fh.callView))
}

func (s *viewHandler) callView(vid string, input []byte) (interface{}, error) {
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

func (s *viewHandler) RunView(manager *view.Manager, view view.View) (string, error) {
	context, err := manager.InitiateContext(view)
	if err != nil {
		return "", errors.WithMessagef(err, "failed to initiate context")
	}

	// Run the view
	go s.runView(view, context)

	return context.ID(), nil
}

func (s *viewHandler) runView(view view.View, context *view.Context) {
	result, err := context.RunView(view)
	if err != nil {
		s.logger.Errorf("Failed view execution. Err [%s]\n", err)
	} else {
		s.logger.Infof("Successful view execution. Result [%s]\n", result)
	}
}
