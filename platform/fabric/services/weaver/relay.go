/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package weaver

import (
	"time"

	"github.com/hyperledger-labs/weaver-dlt-interoperability/common/protos-go/common"
	"github.com/pkg/errors"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/weaver/relay"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/grpc"
)

type Relay struct {
	fns *fabric.NetworkService
}

func (r *Relay) RequestState() error {
	address := r.fns.ConfigService().GetString("weaver.relay.address")
	config := &relay.ClientConfig{
		ID: r.fns.Name(),
		RelayServer: &grpc.ConnectionConfig{
			Address:           address,
			ConnectionTimeout: 300 * time.Second,
			TLSEnabled:        false,
		},
	}
	c, err := relay.NewClient(config, nil, nil)
	if err != nil {
		return errors.WithMessagef(err, "failed getting relay client")
	}
	defer c.Close()

	ack, err := c.RequestState()
	if err != nil {
		return errors.WithMessagef(err, "failed requesting state")
	}
	if ack.Status == common.Ack_ERROR {
		return errors.Errorf("got error ack [%s]", ack.String())
	}
	return nil
}
