/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package web

type ViewCaller interface {
	CallView(fid string, input []byte) (interface{}, error)
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

	res, err := rd.vc.CallView(context.Vars["View"], context.Query.([]byte))
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
