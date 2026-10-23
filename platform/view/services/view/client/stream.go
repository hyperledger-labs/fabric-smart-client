/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package client

import (
	"encoding/json"
	"io"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view/grpc/server/protos"
)

// Stream models a communication stream between a client and a view.
type Stream struct {
	scc    protos.ViewService_StreamCommandClient
	closer io.Closer
	// sc is the command the stream was opened with; the final response must answer it.
	sc *protos.SignedCommand
}

// Send sends the given message to the stream.
func (c *Stream) Send(m any) error {
	raw, err := json.Marshal(m)
	if err != nil {
		return err
	}
	s := &protos.CallViewResponse{
		Result: raw,
	}
	return c.SendProtoMsg(s)
}

// Recv receives a message from the stream.
func (c *Stream) Recv(m any) error {
	s := &protos.CallViewResponse{}
	if err := c.RecvProtoMsg(s); err != nil {
		return err
	}
	return json.Unmarshal(s.Result, m)
}

// SendProtoMsg sends the given protobuf message to the stream.
func (c *Stream) SendProtoMsg(m any) error {
	return c.scc.SendMsg(m)
}

// RecvProtoMsg receives a protobuf message from the stream.
func (c *Stream) RecvProtoMsg(m any) error {
	return c.scc.RecvMsg(m)
}

// Result receives the view's final response, checks that it answers the command the stream
// was opened with, and releases the stream.
func (c *Stream) Result() ([]byte, error) {
	defer func() {
		_ = c.scc.CloseSend()
		_ = c.closer.Close()
	}()
	scr, err := c.scc.Recv()
	if err != nil {
		return nil, err
	}
	commandResp, err := decodeResponse(c.sc, scr)
	if err != nil {
		return nil, err
	}
	cvr := commandResp.GetCallViewResponse()
	if cvr == nil {
		return nil, errors.New("no call view response found")
	}
	return cvr.Result, nil
}
