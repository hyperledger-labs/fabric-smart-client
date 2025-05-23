/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package operations

import (
	"encoding/json"
	"net/http"

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	"github.com/pkg/errors"
)

type VersionInfoHandler struct {
	CommitSHA string `json:"CommitSHA,omitempty"`
	Version   string `json:"Version,omitempty"`
}

func (m *VersionInfoHandler) ServeHTTP(resp http.ResponseWriter, req *http.Request) {
	switch req.Method {
	case http.MethodGet:
		m.sendResponse(resp, http.StatusOK, m)
	default:
		err := errors.Errorf("invalid request method: %s", req.Method)
		m.sendResponse(resp, http.StatusBadRequest, err)
	}
}

type errorResponse struct {
	Error string `json:"Error"`
}

func (m *VersionInfoHandler) sendResponse(resp http.ResponseWriter, code int, payload interface{}) {
	if err, ok := payload.(error); ok {
		payload = &errorResponse{Error: err.Error()}
	}
	js, err := json.Marshal(payload)
	if err != nil {
		logger := logging.MustGetLogger()
		logger.Errorw("failed to encode payload", "error", err)
		resp.WriteHeader(http.StatusInternalServerError)
		return
	}
	resp.Header().Set("Content-Type", "application/json")
	resp.WriteHeader(code)
	_, _ = resp.Write(js)
}
