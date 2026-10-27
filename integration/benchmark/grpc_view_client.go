/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package benchmark

import (
	"context"
	"crypto/rand"
	"io"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/proto"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view/grpc/server/protos"
)

type ViewClient struct {
	SignF       func(msg []byte) ([]byte, error)
	Creator     []byte
	TLSCertHash []byte
	Client      protos.ViewServiceClient
}

func (vc *ViewClient) CallView(fid string, input []byte) (any, error) {
	return vc.CallViewWithContext(context.TODO(), fid, input)
}

func (vc *ViewClient) CallViewWithContext(ctx context.Context, fid string, input []byte) (any, error) {
	c := &protos.Command{Payload: &protos.Command_CallView{CallView: &protos.CallView{Fid: fid, Input: input}}}

	sc, err := vc.CreateSignedCommand(c)
	if err != nil {
		return nil, err
	}

	scr, err := vc.Client.ProcessCommand(ctx, sc)
	if err != nil {
		return nil, err
	}

	commandResp := &protos.CommandResponse{}
	err = proto.Unmarshal(scr.Response, commandResp)
	if err != nil {
		return nil, err
	}

	if commandResp.GetErr() != nil {
		return nil, errors.Errorf("error from view during process command: %s", commandResp.GetErr().GetMessage())
	}

	return commandResp.GetCallViewResponse().GetResult(), nil
}

func (vc *ViewClient) CreateSignedCommand(command *protos.Command) (*protos.SignedCommand, error) {
	nonce := make([]byte, 32)
	_, err := io.ReadFull(rand.Reader, nonce)
	if err != nil {
		return nil, err
	}

	ts := timestamppb.New(time.Now())
	if err := ts.CheckValid(); err != nil {
		return nil, err
	}

	command.Header = &protos.Header{
		Timestamp:   ts,
		Nonce:       nonce,
		Creator:     vc.Creator,
		TlsCertHash: vc.TLSCertHash,
	}

	raw, err := proto.Marshal(command)
	if err != nil {
		return nil, err
	}

	signature, err := vc.SignF(raw)
	if err != nil {
		return nil, err
	}

	return &protos.SignedCommand{
		Command:   raw,
		Signature: signature,
	}, nil
}
