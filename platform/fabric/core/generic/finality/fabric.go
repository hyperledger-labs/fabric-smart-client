/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package finality

import (
	"context"
	"time"

	ab "github.com/hyperledger/fabric-protos-go/orderer"
	"go.uber.org/zap/zapcore"

	"github.com/pkg/errors"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/delivery"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/grpc"
)

type Network interface {
	Name() string
	PickPeer(driver.PeerFunctionType) *grpc.ConnectionConfig
	LocalMembership() driver.LocalMembership
	Channel(id string) (driver.Channel, error)
	IdentityProvider() driver.IdentityProvider
}

type Hasher interface {
	Hash(msg []byte) (hash []byte, err error)
}

type fabricFinality struct {
	channel             string
	network             Network
	hasher              Hasher
	waitForEventTimeout time.Duration
}

func NewFabricFinality(channel string, network Network, hasher Hasher, waitForEventTimeout time.Duration) (*fabricFinality, error) {
	if len(channel) == 0 {
		return nil, errors.Errorf("expected a channel, got empty string")
	}

	d := &fabricFinality{
		channel:             channel,
		network:             network,
		hasher:              hasher,
		waitForEventTimeout: waitForEventTimeout,
	}

	return d, nil
}

func (d *fabricFinality) IsFinal(txID string, address string) error {
	if logger.IsEnabledFor(zapcore.DebugLevel) {
		logger.Debugf("remote checking if transaction [%s] is final in channel [%s]", txID, d.channel)
	}
	var eventCh chan delivery.TxEvent
	var ctx context.Context
	var cancelFunc context.CancelFunc

	ch, err := d.network.Channel(d.channel)
	if err != nil {
		return errors.WithMessagef(err, "failed connecting to channel [%s]", d.channel)
	}
	client, err := ch.NewPeerClientForAddress(*d.network.PickPeer(driver.PeerForFinality))
	if err != nil {
		return errors.WithMessagef(err, "failed creating peer client for address [%s]", address)
	}
	defer client.Close()

	deliverClient, err := delivery.NewDeliverClient(client)
	if err != nil {
		return errors.WithMessagef(err, "failed creating deliver client for address [%s]", address)
	}

	ctx, cancelFunc = context.WithTimeout(context.Background(), d.waitForEventTimeout)
	defer cancelFunc()
	deliverStream, err := deliverClient.NewDeliverFiltered(ctx)
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
	err = delivery.DeliverSend(deliverStream, blockEnvelope)
	if err != nil {
		return err
	}
	eventCh = make(chan delivery.TxEvent, 1)
	go delivery.DeliverReceive(deliverStream, address, txID, eventCh)
	committed, _, _, err := delivery.DeliverWaitForResponse(ctx, eventCh, txID)
	if err != nil {
		return err
	}
	if !committed {
		return errors.New("not committed")
	}
	return nil
}
