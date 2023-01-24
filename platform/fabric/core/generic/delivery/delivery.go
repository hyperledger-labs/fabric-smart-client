/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package delivery

import (
	"context"
	"strings"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/committer"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/peer"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	view2 "github.com/hyperledger-labs/fabric-smart-client/platform/view"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/flogging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/grpc"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/hash"
	"github.com/hyperledger/fabric-protos-go/common"
	ab "github.com/hyperledger/fabric-protos-go/orderer"
	pb "github.com/hyperledger/fabric-protos-go/peer"
	"github.com/pkg/errors"
	"go.uber.org/zap/zapcore"
)

var logger = flogging.MustGetLogger("fabric-sdk.delivery")

var (
	ErrComm      = errors.New("communication issue")
	StartGenesis = &ab.SeekPosition{
		Type: &ab.SeekPosition_Oldest{
			Oldest: &ab.SeekOldest{},
		},
	}
)

// Callback is the callback function prototype to alert the rest of the stack about the availability of a new block.
// The function returns two argument a boolean to signal if delivery should be stopped, and an error
// to signal an issue during the processing of the block.
// In case of an error, the same block is re-processed after a delay.
type Callback func(block *common.Block) (bool, error)

// Vault models a key-value store that can be updated by committing rwsets
type Vault interface {
	// GetLastTxID returns the last transaction id committed
	GetLastTxID() (string, error)
}

type Network interface {
	Name() string
	Channel(name string) (driver.Channel, error)
	PickPeer(funcType driver.PeerFunctionType) *grpc.ConnectionConfig
	LocalMembership() driver.LocalMembership
}

type Delivery struct {
	channel             string
	sp                  view2.ServiceProvider
	network             Network
	waitForEventTimeout time.Duration
	callback            Callback
	vault               Vault
	client              peer.Client
	lastBlockReceived   uint64
	stop                chan bool
}

func New(channel string, sp view2.ServiceProvider, network Network, callback Callback, vault Vault, waitForEventTimeout time.Duration) (*Delivery, error) {
	if len(channel) == 0 {
		return nil, errors.Errorf("expected a channel, got empty string")
	}
	d := &Delivery{
		channel:             channel,
		sp:                  sp,
		network:             network,
		waitForEventTimeout: waitForEventTimeout,
		callback:            callback,
		vault:               vault,
		stop:                make(chan bool),
	}
	return d, nil
}

// Start runs the delivery service in a goroutine
func (d *Delivery) Start(ctx context.Context) {
	go d.Run(ctx)
}

func (d *Delivery) Stop() {
	d.stop <- true
}

func (d *Delivery) Run(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	var df DeliverStream
	var err error
	for {
		select {
		case <-d.stop:
			// Time to stop
			return nil
		case <-ctx.Done():
			// Time to cancel
			return errors.New("context done")
		default:
			if df == nil {
				if logger.IsEnabledFor(zapcore.DebugLevel) {
					logger.Debugf("deliver service [%s], connecting...", d.network.Name(), d.channel)
				}
				df, err = d.connect(ctx)
				if err != nil {
					logger.Errorf("failed connecting to delivery service [%s:%s] [%s]. Wait 10 sec before reconnecting", d.network.Name(), d.channel, err)
					time.Sleep(10 * time.Second)
					if logger.IsEnabledFor(zapcore.DebugLevel) {
						logger.Debugf("reconnecting to delivery service [%s:%s]", d.network.Name(), d.channel)
					}
					continue
				}
			}

			resp, err := df.Recv()
			if err != nil {
				df = nil
				logger.Errorf("delivery service [%s:%s:%s], failed receiving response [%s]",
					d.client.Address(), d.network.Name(), d.channel,
					errors.WithMessagef(err, "error receiving deliver response from peer %s", d.client.Address()))
				continue
			}

			switch r := resp.Type.(type) {
			case *pb.DeliverResponse_Block:
				if r.Block == nil || r.Block.Data == nil || r.Block.Header == nil || r.Block.Metadata == nil {
					if logger.IsEnabledFor(zapcore.DebugLevel) {
						logger.Debugf("deliver service [%s:%s:%s], received nil block", d.client.Address(), d.network.Name(), d.channel)
					}
					time.Sleep(10 * time.Second)
					df = nil
				}

				if logger.IsEnabledFor(zapcore.DebugLevel) {
					logger.Debugf("delivery service [%s:%s:%s], commit block [%d]", d.client.Address(), d.network.Name(), d.channel, r.Block.Header.Number)
				}
				d.lastBlockReceived = r.Block.Header.Number

				stop, err := d.callback(r.Block)
				if err != nil {
					logger.Errorf("error occurred when processing filtered block [%s], retry...", err)
					time.Sleep(10 * time.Second)
					df = nil
				}
				if stop {
					return nil
				}
			case *pb.DeliverResponse_Status:
				if r.Status == common.Status_NOT_FOUND {
					df = nil
					logger.Warnf("delivery service [%s:%s:%s] status [%s], wait a few seconds before retrying", d.client.Address(), d.network.Name(), d.channel, r.Status)
					time.Sleep(10 * time.Second)
				} else {
					logger.Warnf("delivery service [%s:%s:%s] status [%s]", d.client.Address(), d.network.Name(), d.channel, r.Status)
				}
			default:
				df = nil
				logger.Errorf("delivery service [%s:%s:%s], got [%s]", d.client.Address(), d.network.Name(), d.channel, r)
			}
		}
	}
}

func (d *Delivery) connect(ctx context.Context) (DeliverStream, error) {
	// first cleanup everything
	d.cleanup()

	peerConnConf := d.network.PickPeer(driver.PeerForDelivery)

	address := peerConnConf.Address
	if logger.IsEnabledFor(zapcore.DebugLevel) {
		logger.Debugf("connecting to deliver service at [%s] for [%s:%s]", address, d.network.Name(), d.channel)
	}
	ch, err := d.network.Channel(d.channel)
	if err != nil {
		return nil, errors.WithMessagef(err, "failed connecting to channel [%s:%s]", d.network.Name(), d.channel)
	}
	d.client, err = ch.NewPeerClientForAddress(*peerConnConf)
	if err != nil {
		return nil, errors.WithMessagef(err, "failed creating peer client for address [%s][%s:%s]", address, d.network.Name(), d.channel)
	}
	deliverClient, err := NewDeliverClient(d.client)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get deliver client")
	}
	stream, err := deliverClient.NewDeliver(ctx)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get delivery stream")
	}

	blockEnvelope, err := CreateDeliverEnvelope(
		d.channel,
		d.network.LocalMembership().DefaultSigningIdentity(),
		deliverClient.Certificate(),
		hash.GetHasher(d.sp),
		d.GetStartPosition(),
	)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create deliver envelope")
	}
	err = DeliverSend(stream, blockEnvelope)
	if err != nil {
		return nil, errors.Wrapf(err, "failed sending seek envelope to [%s]", address)
	}

	if logger.IsEnabledFor(zapcore.DebugLevel) {
		logger.Debugf("connected to deliver service at [%s]", address)
	}
	return stream, nil
}

func (d *Delivery) GetStartPosition() *ab.SeekPosition {
	if d.lastBlockReceived != 0 {
		if logger.IsEnabledFor(zapcore.DebugLevel) {
			logger.Debugf("restarting from the last block received [%d]", d.lastBlockReceived)
		}

		return &ab.SeekPosition{
			Type: &ab.SeekPosition_Specified{
				Specified: &ab.SeekSpecified{
					Number: d.lastBlockReceived,
				},
			},
		}
	}

	if logger.IsEnabledFor(zapcore.DebugLevel) {
		logger.Debugf("no last block received set [%d], check last TxID in the vault", d.lastBlockReceived)
	}

	lastTxID, err := d.vault.GetLastTxID()
	if err != nil {
		logger.Errorf("failed getting last transaction committed/discarded from the vault [%s], restarting from genesis", err)
		return StartGenesis
	}

	if len(lastTxID) != 0 && !strings.HasPrefix(lastTxID, committer.ConfigTXPrefix) {
		// Retrieve block from Fabric
		ch, err := d.network.Channel(d.channel)
		if err != nil {
			logger.Errorf("failed getting channel [%s], restarting from genesis: [%s]", d.channel, err)
			return StartGenesis
		}
		blockNumber, err := ch.GetBlockNumberByTxID(lastTxID)
		if err != nil {
			logger.Errorf("failed getting block number for transaction [%s], restart from genesis [%s]", lastTxID, err)
			return StartGenesis
		}
		if logger.IsEnabledFor(zapcore.DebugLevel) {
			logger.Debugf("restarting from block [%d], tx [%s]", blockNumber, lastTxID)
		}

		return &ab.SeekPosition{
			Type: &ab.SeekPosition_Specified{
				Specified: &ab.SeekSpecified{
					Number: blockNumber,
				},
			},
		}
	}

	return StartGenesis
}

func (d *Delivery) cleanup() {
	if d.client != nil {
		d.client.Close()
	}
}
