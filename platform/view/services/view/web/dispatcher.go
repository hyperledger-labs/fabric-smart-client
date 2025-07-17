/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package web

import (
	"strings"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/web/server"
)

var logger = logging.MustGetLogger()

type ViewCaller interface {
	CallView(context *server.ReqContext, vid string, input []byte) (interface{}, error)
}

func newDispatcher(h *server.HttpHandler) *Dispatcher {
	return &Dispatcher{Handler: h}
}

type Dispatcher struct {
	vc      ViewCaller
	Handler *server.HttpHandler
}

func (rd *Dispatcher) HandleRequest(reqctx *server.ReqContext) (response interface{}, statusCode int) {
	ctx := reqctx.Req.Context()
	logger.DebugfContext(ctx, "received request from %s", reqctx.Req.Host)

	if rd.vc == nil {
		logger.ErrorfContext(ctx, "viewCaller has not been initialized yet")
		return &server.ResponseErr{Reason: "internal error"}, 500
	}

	viewID := reqctx.Vars["View"]
	escapedViewID := strings.ReplaceAll(viewID, "\n", "")
	escapedViewID = strings.ReplaceAll(escapedViewID, "\r", "")

	res, err := rd.vc.CallView(reqctx, escapedViewID, reqctx.Query.([]byte))
	if err != nil {
		return &server.ResponseErr{Reason: err.Error()}, 500
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
