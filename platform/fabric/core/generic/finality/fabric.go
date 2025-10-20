/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package finality

import (
	"context"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/delivery"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/services"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/grpc"
	ab "github.com/hyperledger/fabric-protos-go-apiv2/orderer"
)

type Services interface {
	NewPeerClient(cc grpc.ConnectionConfig) (services.PeerClient, error)
}

type FabricFinality struct {
	Logger                 logging.Logger
	Channel                string
	ConfigService          driver.ConfigService
	Services               Services
	DefaultSigningIdentity driver.SigningIdentity
	WaitForEventTimeout    time.Duration
	useFiltered            bool
}

func NewFabricFinality(
	logger logging.Logger,
	channel string,
	ConfigService driver.ConfigService,
	peerService Services,
	defaultSigningIdentity driver.SigningIdentity,
	waitForEventTimeout time.Duration,
	useFiltered bool,
) (*FabricFinality, error) {
	if len(channel) == 0 {
		return nil, errors.Errorf("expected a channel, got empty string")
	}

	d := &FabricFinality{
		Logger:                 logger,
		Channel:                channel,
		ConfigService:          ConfigService,
		Services:               peerService,
		DefaultSigningIdentity: defaultSigningIdentity,
		WaitForEventTimeout:    waitForEventTimeout,
		useFiltered:            useFiltered,
	}

	return d, nil
}

func (d *FabricFinality) IsFinal(txID string, address string) error {
	d.Logger.Debugf("remote checking if transaction [%s] is final in channel [%s]", txID, d.Channel)
	var eventCh chan delivery.TxEvent
	var ctx context.Context
	var cancelFunc context.CancelFunc

	client, err := d.Services.NewPeerClient(*d.ConfigService.PickPeer(driver.PeerForFinality))
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
	var deliverStream delivery.DeliverFiltered
	if d.useFiltered {
		deliverStream, err = deliverClient.NewDeliverFiltered(ctx)
	} else {
		deliverStream, err = deliverClient.NewDeliver(ctx)
	}
	if err != nil {
		return err
	}

	blockEnvelope, err := delivery.CreateDeliverEnvelope(
		d.Channel,
		d.DefaultSigningIdentity,
		deliverClient.Certificate(),
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
	go utils.IgnoreErrorFunc(func() error {
		return delivery.DeliverReceive(deliverStream, address, txID, eventCh)
	})
	committed, _, _, err := delivery.DeliverWaitForResponse(ctx, eventCh, txID)
	if err != nil {
		return err
	}
	if !committed {
		return errors.New("not committed")
	}
	return nil
}
