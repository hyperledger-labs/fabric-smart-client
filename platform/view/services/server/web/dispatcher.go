/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package web

import "strings"

type ViewCaller interface {
	CallView(context *ReqContext, vid string, input []byte) (interface{}, error)
}

func newDispatcher(h *HttpHandler) *Dispatcher {
	return &Dispatcher{Logger: h.Logger, Handler: h}
}

type Dispatcher struct {
	vc      ViewCaller
	Logger  logger
	Handler *HttpHandler
}

func (rd *Dispatcher) HandleRequest(context *ReqContext) (response interface{}, statusCode int) {
	rd.Logger.Infof("Received request from %s", context.Req.Host)

	if rd.vc == nil {
		rd.Logger.Errorf("ViewCaller has not been initialized yet")
		return &ResponseErr{Reason: "internal error"}, 500
	}

	viewID := context.Vars["View"]
	escapedViewID := strings.Replace(viewID, "\n", "", -1)
	escapedViewID = strings.Replace(escapedViewID, "\r", "", -1)

	res, err := rd.vc.CallView(context, escapedViewID, context.Query.([]byte))
	if err != nil {
		return &ResponseErr{Reason: err.Error()}, 500
	}

	return res, 200
}

func (rd *Dispatcher) ParsePayload(bytes []byte) (interface{}, error) {
	return bytes, nil
}

func (rd *Dispatcher) WireViewCaller(vc ViewCaller) {
	rd.vc = vc
	rd.Handler.RegisterURI("/Views/{View}", "PUT", rd)
}

func (rd *Dispatcher) WireStreamViewCaller(vc ViewCaller) {
	rd.vc = vc
	rd.Handler.RegisterURI("/Views/Stream/{View}", "GET", rd)
}
