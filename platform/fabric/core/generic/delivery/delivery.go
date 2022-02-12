/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package delivery

import (
	"context"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/peer"
	"strings"
	"time"

	"github.com/hyperledger/fabric-protos-go/common"
	ab "github.com/hyperledger/fabric-protos-go/orderer"
	pb "github.com/hyperledger/fabric-protos-go/peer"
	"github.com/pkg/errors"
	"go.uber.org/zap/zapcore"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/committer"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/flogging"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	view2 "github.com/hyperledger-labs/fabric-smart-client/platform/view"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/grpc"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/hash"
)

var logger = flogging.MustGetLogger("fabric-sdk.delivery")

var (
	ErrComm = errors.New("communication issue")
)

type Callback func(block *common.Block) (bool, error)

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
	ctx                 context.Context
	channel             string
	sp                  view2.ServiceProvider
	network             Network
	waitForEventTimeout time.Duration
	callback            Callback
	vault               Vault
	client              peer.Client
}

func New(
	ctx context.Context,
	channel string,
	sp view2.ServiceProvider,
	network Network,
	callback Callback,
	vault Vault,
	waitForEventTimeout time.Duration,
) (*delivery, error) {
	if len(channel) == 0 {
		panic("expected a channel, got empty string")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	d := &delivery{
		ctx:                 ctx,
		channel:             channel,
		sp:                  sp,
		network:             network,
		waitForEventTimeout: waitForEventTimeout,
		callback:            callback,
		vault:               vault,
	}
	return d, nil
}

// Start runs the delivery service in a goroutine
func (d *delivery) Start() {
	go d.Run()
}

func (d *delivery) Run() error {
	var df DeliverStream
	var err error
	for {
		select {
		case <-d.ctx.Done():
			// Time to cancel
			return errors.New("context done")
		default:
			if df == nil {
				if logger.IsEnabledFor(zapcore.DebugLevel) {
					logger.Debugf("deliver service [%s:%s], connecting...", d.channel)
				}
				df, err = d.connect()
				if err != nil {
					logger.Errorf("failed connecting to delivery service [%s:%s] [%s]. Wait 10 sec before reconnecting", d.channel, err)
					time.Sleep(10 * time.Second)
					if logger.IsEnabledFor(zapcore.DebugLevel) {
						logger.Debugf("reconnecting to delivery service [%s:%s]", d.channel)
					}
					continue
				}
			}

			resp, err := df.Recv()
			if err != nil {
				df = nil
				logger.Errorf("delivery service [%s:%s], failed receiving response [%s]",
					d.client.Address(), d.channel,
					errors.WithMessagef(err, "error receiving deliver response from peer %s", d.client.Address()))
				continue
			}

			switch r := resp.Type.(type) {
			case *pb.DeliverResponse_Block:
				if r.Block == nil || r.Block.Data == nil || r.Block.Header == nil || r.Block.Metadata == nil {
					if logger.IsEnabledFor(zapcore.DebugLevel) {
						logger.Debugf("deliver service [%s:%s], received nil block", d.client.Address(), d.channel)
					}
					time.Sleep(10 * time.Second)
					df = nil
				}

				if logger.IsEnabledFor(zapcore.DebugLevel) {
					logger.Debugf("delivery service [%s:%s], commit block [%d]", d.client.Address(), d.channel, r.Block.Header.Number)
				}

				stop, err := d.callback(r.Block)
				if err != nil {
					switch errors.Cause(err) {
					case ErrComm:
						logger.Errorf("error occurred when processing filtered block [%s], retry", err)
						// retry
						time.Sleep(10 * time.Second)
						df = nil
					default:
						// Stop here
						logger.Errorf("error occurred when processing filtered block [%s]", err)
						return err
					}
				}
				if stop {
					return nil
				}
			case *pb.DeliverResponse_Status:
				if r.Status == common.Status_NOT_FOUND {
					df = nil
					logger.Warnf("delivery service [%s:%s] status [%s], wait a few seconds before retrying", d.client.Address(), d.channel, r.Status)
					time.Sleep(10 * time.Second)
				} else {
					logger.Warnf("delivery service [%s:%s] status [%s]", d.client.Address(), d.channel, r.Status)
				}
			default:
				df = nil
				logger.Errorf("delivery service [%s:%s], got [%s]", d.client.Address(), d.channel, r)
			}
		}
	}
}

func (d *delivery) connect() (DeliverStream, error) {
	// first cleanup everything
	d.cleanup()

	address := d.network.Peers()[0].Address
	if logger.IsEnabledFor(zapcore.DebugLevel) {
		logger.Debugf("connecting to deliver service at [%s] for channel [%s]", address, d.channel)
	}
	ch, err := d.network.Channel(d.channel)
	if err != nil {
		return nil, errors.WithMessagef(err, "failed connecting to channel [%s]", d.channel)
	}
	d.client, err = ch.NewPeerClientForAddress(*d.network.Peers()[0])
	if err != nil {
		return nil, errors.WithMessagef(err, "failed creating peer client for address [%s]", address)
	}
	deliverClient, err := NewDeliverClient(d.client)
	if err != nil {
		return nil, err
	}
	stream, err := deliverClient.NewDeliver(d.ctx)
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
		if logger.IsEnabledFor(zapcore.DebugLevel) {
			logger.Debugf("restarting from block [%d], tx [%s]", blockNumber, lastTxID)
		}
	} else {
		start.Type = &ab.SeekPosition_Oldest{
			Oldest: &ab.SeekOldest{},
		}
		if logger.IsEnabledFor(zapcore.DebugLevel) {
			logger.Debugf("starting from the beginning, no last transaction found")
		}
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
	err = DeliverSend(stream, blockEnvelope)
	if err != nil {
		return nil, errors.Errorf("failed sending seek envelope to [%s]: %s", address, err)
	}

	if logger.IsEnabledFor(zapcore.DebugLevel) {
		logger.Debugf("connected to deliver service at [%s]", address)
	}
	return stream, nil
}

func (d *delivery) cleanup() {
	if d.client != nil {
		d.client.Close()
	}
}
