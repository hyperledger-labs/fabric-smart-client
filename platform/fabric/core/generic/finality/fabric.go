/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package finality

import (
	"context"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/delivery"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/peer"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/flogging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/grpc"
	ab "github.com/hyperledger/fabric-protos-go/orderer"
	"github.com/pkg/errors"
	"go.uber.org/zap/zapcore"
)

var logger = flogging.MustGetLogger("fabric-sdk.core")

type PeerService interface {
	NewClient(cc grpc.ConnectionConfig) (peer.Client, error)
}

type Hasher interface {
	Hash(msg []byte) (hash []byte, err error)
}

type FabricFinality struct {
	Channel                string
	ConfigService          driver.ConfigService
	PeerService            PeerService
	DefaultSigningIdentity driver.SigningIdentity
	Hasher                 Hasher
	WaitForEventTimeout    time.Duration
}

func NewFabricFinality(
	channel string,
	ConfigService driver.ConfigService,
	peerService PeerService,
	defaultSigningIdentity driver.SigningIdentity,
	hasher Hasher,
	waitForEventTimeout time.Duration,
) (*FabricFinality, error) {
	if len(channel) == 0 {
		return nil, errors.Errorf("expected a channel, got empty string")
	}

	d := &FabricFinality{
		Channel:                channel,
		ConfigService:          ConfigService,
		PeerService:            peerService,
		DefaultSigningIdentity: defaultSigningIdentity,
		Hasher:                 hasher,
		WaitForEventTimeout:    waitForEventTimeout,
	}

	return d, nil
}

func (d *FabricFinality) IsFinal(txID string, address string) error {
	if logger.IsEnabledFor(zapcore.DebugLevel) {
		logger.Debugf("remote checking if transaction [%s] is final in channel [%s]", txID, d.Channel)
	}
	var eventCh chan delivery.TxEvent
	var ctx context.Context
	var cancelFunc context.CancelFunc

	client, err := d.PeerService.NewClient(*d.ConfigService.PickPeer(driver.PeerForFinality))
	if err != nil {
		return errors.WithMessagef(err, "failed creating peer client for address [%s]", address)
	}
	defer client.Close()

	deliverClient, err := delivery.NewDeliverClient(client)
	if err != nil {
		return errors.WithMessagef(err, "failed creating deliver client for address [%s]", address)
	}

	ctx, cancelFunc = context.WithTimeout(context.Background(), d.WaitForEventTimeout)
	defer cancelFunc()
	deliverStream, err := deliverClient.NewDeliverFiltered(ctx)
	if err != nil {
		return err
	}

	blockEnvelope, err := delivery.CreateDeliverEnvelope(
		d.Channel,
		d.DefaultSigningIdentity,
		deliverClient.Certificate(),
		d.Hasher,
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
