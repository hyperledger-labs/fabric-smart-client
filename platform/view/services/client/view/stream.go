/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package view

import (
	"encoding/json"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/proto"
	protos2 "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/server/view/protos"
	"github.com/pkg/errors"
	"google.golang.org/grpc"
)

type Stream struct {
	scc  protos2.ViewService_StreamCommandClient
	conn *grpc.ClientConn
}

func (c *Stream) Send(m interface{}) error {
	raw, err := json.Marshal(m)
	if err != nil {
		return err
	}
	s := &protos2.CallViewResponse{
		Result: raw,
	}
	return c.SendProtoMsg(s)
}

func (c *Stream) Recv(m interface{}) error {
	s := &protos2.CallViewResponse{}
	if err := c.RecvProtoMsg(s); err != nil {
		return err
	}
	return json.Unmarshal(s.Result, m)
}

func (c *Stream) SendProtoMsg(m interface{}) error {
	return c.scc.SendMsg(m)
}

func (c *Stream) RecvProtoMsg(m interface{}) error {
	return c.scc.RecvMsg(m)
}

func (c *Stream) Result() ([]byte, error) {
	defer c.conn.Close()
	scr, err := c.scc.Recv()
	if err != nil {
		return nil, err
	}

	commandResp := &protos2.CommandResponse{}
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
