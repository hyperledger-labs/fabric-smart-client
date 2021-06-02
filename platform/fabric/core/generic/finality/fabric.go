/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package finality

import (
	"context"
	"time"

	ab "github.com/hyperledger/fabric-protos-go/orderer"

	"github.com/pkg/errors"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/api"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/delivery"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/grpc"
)

type Network interface {
	Peers() []*grpc.ConnectionConfig
	LocalMembership() api.LocalMembership
	Comm(channel string) (api.Comm, error)
	Channel(id string) (api.Channel, error)
	IdentityProvider() api.IdentityProvider
}

type Hasher interface {
	Hash(msg []byte) (hash []byte, err error)
}

type fabricFinality struct {
	channel              string
	network              Network
	hasher               Hasher
	waitForEventTimeout  time.Duration
	peerConnectionConfig *grpc.ConnectionConfig
}

func NewFabricFinality(channel string, network Network, hasher Hasher, waitForEventTimeout time.Duration) (*fabricFinality, error) {
	if len(channel) == 0 {
		panic("expected a channel, got empty string")
	}

	d := &fabricFinality{
		channel:              channel,
		network:              network,
		hasher:               hasher,
		waitForEventTimeout:  waitForEventTimeout,
		peerConnectionConfig: network.Peers()[0],
	}

	return d, nil
}

func (d *fabricFinality) IsFinal(txID string, address string) error {
	var eventCh chan delivery.TxEvent
	var ctx context.Context
	var cancelFunc context.CancelFunc

	deliverClient, err := delivery.NewDeliverClient(d.peerConnectionConfig)
	if err != nil {
		return err
	}

	ctx, cancelFunc = context.WithTimeout(context.Background(), d.waitForEventTimeout)
	defer cancelFunc()
	deliverFiltered, err := deliverClient.NewDeliverFiltered(ctx)
	if err != nil {
		return err
	}

	blockEnvelope, err := delivery.CreateDeliverEnvelope(
		d.channel,
		d.network.LocalMembership().DefaultSigningIdentity(),
		deliverClient.Certificate(),
		d.hasher,
		&ab.SeekPosition{
			Type: &ab.SeekPosition_Newest{
				Newest: &ab.SeekNewest{},
			},
		},
	)
	if err != nil {
		return err
	}
	err = delivery.DeliverSend(deliverFiltered, address, blockEnvelope)
	if err != nil {
		return err
	}
	eventCh = make(chan delivery.TxEvent, 1)
	go delivery.DeliverReceive(deliverFiltered, address, txID, eventCh)
	committed, _, _, err := delivery.DeliverWaitForResponse(ctx, eventCh, txID)
	if err != nil {
		return err
	}
	if !committed {
		return errors.New("not committed")
	}
	return nil
}
