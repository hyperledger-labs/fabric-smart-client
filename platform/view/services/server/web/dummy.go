/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package web

import (
	"net/http"
)

type DummyServer struct{}

func NewDummyServer() *DummyServer {
	return &DummyServer{}
}

func (d *DummyServer) RegisterHandler(s string, handler http.Handler, secure bool) {
}

func (d *DummyServer) Start() error {
	return nil
}

func (d *DummyServer) Stop() error {
	return nil
}
