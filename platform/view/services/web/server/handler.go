/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package server

import (
	"bytes"
	"encoding/json"
	stderrors "errors"
	"io"
	"mime"
	"net/http"
	"strings"

	"github.com/gorilla/websocket"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
)

const (
	apiVersion = "/v1"
)

type ResponseErr struct {
	Reason string
}

//nolint:revive // var-naming: renaming this exported type is an API break; see follow-up
type HttpHandler struct {
	mux *http.ServeMux
}

type ReqContext struct {
	ResponseWriter http.ResponseWriter
	Req            *http.Request
	Query          any
}

//go:generate counterfeiter -o mock/request_handler.go -fake-name RequestHandler . RequestHandler

type RequestHandler interface {
	// HandleRequest dispatches the request in the backend by parsing the given request context
	// and returning a status code and a response back to the client.
	HandleRequest(*ReqContext) (response any, statusCode int)

	// ParsePayload parses the payload to handler specific form or returns an error
	ParsePayload([]byte) (any, error)
}

//nolint:revive // var-naming: renaming this exported func is an API break; see follow-up
func NewHttpHandler() *HttpHandler {
	return &HttpHandler{mux: http.NewServeMux()}
}

func (h *HttpHandler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	h.mux.ServeHTTP(w, req)
}

func (h *HttpHandler) RegisterURI(uri, method string, rh RequestHandler) {
	f := func(backToClient http.ResponseWriter, req *http.Request) {
		h.handle(backToClient, req, rh)
	}
	h.mux.HandleFunc(method+" "+apiVersion+uri, f)
}

func (*HttpHandler) handle(backToClient http.ResponseWriter, req *http.Request, rh RequestHandler) {
	if !acceptsJSON(req) {
		sendErr(backToClient, http.StatusBadRequest, "bad content type", nil)
		return
	}

	reqPayload, err := io.ReadAll(http.MaxBytesReader(backToClient, req.Body, maxMessageSize))
	if _, ok := stderrors.AsType[*http.MaxBytesError](err); ok {
		sendErr(backToClient, http.StatusRequestEntityTooLarge, "request too large", err)
		return
	}
	if err != nil {
		sendErr(backToClient, http.StatusBadRequest, "failed reading request", err)
		return
	}

	o, err := rh.ParsePayload(reqPayload)
	if err != nil {
		sendErr(backToClient, http.StatusBadRequest, "failed parsing request", err)
		return
	}

	reqCtx := &ReqContext{
		Query:          o,
		ResponseWriter: backToClient,
		Req:            req,
	}

	resultFromBackend, statusCode := rh.HandleRequest(reqCtx)
	// WriteHeader panics on a status code outside 100-999.
	if statusCode < 100 || statusCode > 999 {
		sendErr(backToClient, http.StatusInternalServerError, "invalid status code from backend",
			errors.Errorf("backend returned status code %d", statusCode))
		return
	}

	response := &bytes.Buffer{}

	encoder := json.NewEncoder(response)
	err = encoder.Encode(resultFromBackend)
	if err != nil {
		sendErr(backToClient, http.StatusInternalServerError, "failed encoding response from backend", err)
		return
	}

	if statusCode/100 != 2 {
		sendErr(backToClient, statusCode, response.String(), nil)
		return
	}

	// The view has taken over the connection of a websocket upgrade, so nothing is written.
	if !websocket.IsWebSocketUpgrade(req) {
		backToClient.Header().Set("Content-Type", "application/json")
		backToClient.WriteHeader(http.StatusOK)
		_, _ = backToClient.Write(response.Bytes())
	}
}

func sendErr(resp http.ResponseWriter, code int, errToClient string, errLogged error) {
	if errLogged != nil {
		logger.Warnf("failed processing request: %v", errLogged)
	}

	encoder := json.NewEncoder(resp)
	resp.Header().Set("Content-Type", "application/json")
	resp.WriteHeader(code)
	if err := encoder.Encode(&ResponseErr{Reason: errToClient}); err != nil {
		logger.Warnf("failed encoding response: %v", err)
	}
}

// acceptsJSON reports whether the request's Accept header admits application/json, the only
// content type the handler responds with. A missing header accepts anything.
func acceptsJSON(req *http.Request) bool {
	accept := req.Header.Get("Accept")
	if accept == "" {
		return true
	}
	for opt := range strings.SplitSeq(accept, ",") {
		mediaType, _, err := mime.ParseMediaType(opt)
		if err != nil {
			continue
		}
		switch mediaType {
		case "application/json", "application/*", "*/*":
			return true
		}
	}
	return false
}
