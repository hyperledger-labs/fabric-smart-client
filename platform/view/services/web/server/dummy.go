/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package server

import (
	"net/http"
)

type DummyServer struct{}

func NewDummyServer() *DummyServer {
	return &DummyServer{}
}

func (*DummyServer) RegisterHandler(_ string, _ http.Handler, _ bool) {
}

func (*DummyServer) Start() error {
	return nil
}

func (*DummyServer) Stop() error {
	return nil
}
