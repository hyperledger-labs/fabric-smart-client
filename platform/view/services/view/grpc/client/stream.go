/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package client

import (
	"encoding/json"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/proto"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view/grpc/server/protos"
	"google.golang.org/grpc"
)

// Stream models a communication stream between a client and a view.
type Stream struct {
	scc  protos.ViewService_StreamCommandClient
	conn *grpc.ClientConn
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

// Result returns the result produced by the view.
func (c *Stream) Result() ([]byte, error) {
	defer utils.IgnoreErrorFunc(c.conn.Close)
	scr, err := c.scc.Recv()
	if err != nil {
		return nil, err
	}

	commandResp := &protos.CommandResponse{}
	err = proto.Unmarshal(scr.Response, commandResp)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to unmarshal command response")
	}
	if commandResp.GetErr() != nil {
		return nil, errors.Errorf("error from view during process command: %s", commandResp.GetErr().GetMessage())
	}
	cvr := commandResp.GetCallViewResponse()
	if cvr == nil {
		return nil, errors.Errorf("no call view response found")
	}
	return cvr.Result, nil
}
