/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package delivery

import (
	"context"
	"strings"
	"time"

	"github.com/hyperledger/fabric-protos-go/common"
	ab "github.com/hyperledger/fabric-protos-go/orderer"
	pb "github.com/hyperledger/fabric-protos-go/peer"
	"github.com/pkg/errors"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/committer"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/flogging"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	view2 "github.com/hyperledger-labs/fabric-smart-client/platform/view"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/grpc"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/hash"
)

var logger = flogging.MustGetLogger("fabric-sdk.delivery")

// Committer models a filtered block committer
type Committer interface {
	// Commit commits the transaction in the passed block
	Commit(block *pb.FilteredBlock)
}

// Vault models a key-value store that can be updated by committing rwsets
type Vault interface {
	// GetLastTxID returns the last transaction id committed
	GetLastTxID() (string, error)
}

type Network interface {
	Channel(name string) (driver.Channel, error)
	Peers() []*grpc.ConnectionConfig
	LocalMembership() driver.LocalMembership
}

type delivery struct {
	channel              string
	sp                   view2.ServiceProvider
	network              Network
	waitForEventTimeout  time.Duration
	peerConnectionConfig *grpc.ConnectionConfig
	committer            Committer
	vault                Vault
}

func New(
	channel string,
	sp view2.ServiceProvider,
	network Network,
	committer Committer,
	vault Vault,
	waitForEventTimeout time.Duration,
) (*delivery, error) {
	if len(channel) == 0 {
		panic("expected a channel, got empty string")
	}
	d := &delivery{
		channel:              channel,
		sp:                   sp,
		network:              network,
		waitForEventTimeout:  waitForEventTimeout,
		peerConnectionConfig: network.Peers()[0],
		committer:            committer,
		vault:                vault,
	}
	return d, nil
}

func (d *delivery) Start() {
	go d.run()
}

func (d *delivery) run() {
	var df DeliverFiltered
	var err error
	for {
		address := d.peerConnectionConfig.Address
		logger.Debugf("deliver service [%s:%s], next event...", address, d.channel)
		if df == nil {
			logger.Debugf("deliver service [%s:%s], connecting...", address, d.channel)
			df, err = d.connect()
			if err != nil {
				logger.Errorf("failed connecting to delivery service [%s:%s] [%s]. Wait 10 sec before reconnecting", address, d.channel, err)
				time.Sleep(10 * time.Second)
				logger.Debugf("reconnecting to delivery service [%s:%s]", address, d.channel)
				continue
			}
		}

		resp, err := df.Recv()
		if err != nil {
			df = nil
			logger.Errorf("delivery service [%s:%s], failed receiving response [%s]", address, d.channel, errors.WithMessagef(err, "error receiving deliver response from peer %s", address))
			continue
		}

		switch r := resp.Type.(type) {
		case *pb.DeliverResponse_FilteredBlock:
			logger.Debugf("delivery service [%s:%s], commit block [%d]", address, d.channel, r.FilteredBlock.Number)

			d.committer.Commit(r.FilteredBlock)
		case *pb.DeliverResponse_Status:
			if r.Status == common.Status_NOT_FOUND {
				df = nil
				logger.Warnf("delivery service [%s:%s] status [%s], wait a few seconds before retrying", address, d.channel, r.Status)
				time.Sleep(10 * time.Second)
			} else {
				logger.Warnf("delivery service [%s:%s] status [%s]", address, d.channel, r.Status)
			}
		default:
			df = nil
			logger.Errorf("delivery service [%s:%s], got [%s]", address, d.channel, r)
		}
	}
}

func (d *delivery) connect() (DeliverFiltered, error) {
	address := d.peerConnectionConfig.Address
	logger.Debugf("connecting to deliver service at [%s] for channel [%s]", address, d.channel)

	var ctx context.Context
	//var cancelFunc context.CancelFunc

	deliverClient, err := NewDeliverClient(d.peerConnectionConfig)
	if err != nil {
		return nil, err
	}

	//ctx, cancelFunc = context.WithTimeout(context.Background(), d.waitForEventTimeout)
	//defer cancelFunc()
	ctx = context.Background()
	deliverFiltered, err := deliverClient.NewDeliverFiltered(ctx)
	if err != nil {
		return nil, err
	}

	lastTxID, err := d.vault.GetLastTxID()
	if err != nil {
		return nil, errors.WithMessagef(err, "failed getting last transaction committed/discarted from the vault")
	}

	start := &ab.SeekPosition{}
	if len(lastTxID) != 0 && !strings.HasPrefix(lastTxID, committer.ConfigTXPrefix) {
		// Retrieve block from Fabric
		ch, err := d.network.Channel(d.channel)
		if err != nil {
			return nil, errors.WithMessagef(err, "failed getting channeln [%s]", d.channel)
		}
		blockNumber, err := ch.GetBlockNumberByTxID(lastTxID)
		if err != nil {
			return nil, errors.WithMessagef(err, "failed getting block number for transaction [%s]", lastTxID)
		}
		start.Type = &ab.SeekPosition_Specified{
			Specified: &ab.SeekSpecified{
				Number: blockNumber,
			},
		}
		logger.Debugf("restarting from block [%d], tx [%s]", blockNumber, lastTxID)
	} else {
		start.Type = &ab.SeekPosition_Oldest{
			Oldest: &ab.SeekOldest{},
		}
		logger.Debugf("starting from the beginning, no last transaction found")
	}

	blockEnvelope, err := CreateDeliverEnvelope(
		d.channel,
		d.network.LocalMembership().DefaultSigningIdentity(),
		deliverClient.Certificate(),
		hash.GetHasher(d.sp),
		start,
	)
	if err != nil {
		return nil, err
	}
	err = DeliverSend(deliverFiltered, d.peerConnectionConfig.Address, blockEnvelope)
	if err != nil {
		return nil, err
	}

	logger.Debugf("connected to deliver service at [%s]", address)
	return deliverFiltered, nil
}
