/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package server

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/gorilla/mux"
)

const (
	apiVersion = "/v1"
)

type ResponseErr struct {
	Reason string
}

type Config struct {
	MaxReqSize uint16
}

type HttpHandler struct {
	r *mux.Router
}

type ReqContext struct {
	ResponseWriter http.ResponseWriter
	Req            *http.Request
	Vars           map[string]string
	Query          interface{}
}

//go:generate counterfeiter -o mocks/request_handler.go -fake-name FakeRequestHandler . RequestHandler

type RequestHandler interface {
	// HandleRequest dispatches the request in the backend by parsing the given request context
	// and returning a status code and a response back to the client.
	HandleRequest(*ReqContext) (response interface{}, statusCode int)

	// ParsePayload parses the payload to handler specific form or returns an error
	ParsePayload([]byte) (interface{}, error)
}

func NewHttpHandler() *HttpHandler {
	return &HttpHandler{r: mux.NewRouter()}
}

func (h *HttpHandler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	h.r.ServeHTTP(w, req)
}

func (h *HttpHandler) RegisterURI(uri string, method string, rh RequestHandler) {
	f := func(backToClient http.ResponseWriter, req *http.Request) {
		h.handle(backToClient, req, rh)
	}

	h.r.HandleFunc(apiVersion+uri, f).Methods(method)
}

func (h *HttpHandler) handle(backToClient http.ResponseWriter, req *http.Request, rh RequestHandler) {
	if _, err := negotiateContentType(req); err != nil {
		sendErr(backToClient, http.StatusBadRequest, "bad content type", err)
		return
	}

	reqPayload, err := io.ReadAll(req.Body)
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
		Vars:           mux.Vars(req),
	}

	resultFromBackend, statusCode := rh.HandleRequest(reqCtx)

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

	if !isWebSocket(req.Header) {
		backToClient.Header().Set("Content-Type", "application/json")
		backToClient.WriteHeader(http.StatusOK)
		_, _ = backToClient.Write(response.Bytes())
	}
}

func isWebSocket(h http.Header) bool {
	upgrade, ok := h["Upgrade"]
	return ok && upgrade[0] == "websocket"
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

func negotiateContentType(req *http.Request) (string, error) {
	acceptReq := req.Header.Get("Accept")
	if len(acceptReq) == 0 {
		return "application/json", nil
	}

	options := strings.Split(acceptReq, ",")
	for _, opt := range options {
		if strings.Contains(opt, "application/json") ||
			strings.Contains(opt, "application/*") ||
			strings.Contains(opt, "*/*") {
			return "application/json", nil
		}
	}

	return "", errors.New("response Content-Type is application/json only")
}
