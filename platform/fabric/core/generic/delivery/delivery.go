/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package delivery

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/committer"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/services"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/grpc"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/tracing"
	cb "github.com/hyperledger/fabric-protos-go-apiv2/common"
	ab "github.com/hyperledger/fabric-protos-go-apiv2/orderer"
	pb "github.com/hyperledger/fabric-protos-go-apiv2/peer"
	"go.opentelemetry.io/otel/trace"
)

var logger = logging.MustGetLogger()

var (
	StartGenesis = &ab.SeekPosition{
		Type: &ab.SeekPosition_Oldest{
			Oldest: &ab.SeekOldest{},
		},
	}
)

type blockResponse struct {
	ctx   context.Context
	block *cb.Block
}

type messageType = string

const (
	messageTypeLabel tracing.LabelName = "type"
	unknown          messageType       = "unknown"
	block            messageType       = "block"
	responseStatus   messageType       = "status"
	other            messageType       = "other"
)

// Vault models a key-value store that can be updated by committing rwsets
type Vault interface {
	// GetLastTxID returns the last transaction id committed
	GetLastTxID(ctx context.Context) (string, error)
	GetLastBlock(context.Context) (uint64, error)
}

type Services interface {
	NewPeerClient(cc grpc.ConnectionConfig) (services.PeerClient, error)
}

type Delivery struct {
	channel             string
	channelConfig       driver.ChannelConfig
	NetworkName         string
	LocalMembership     driver.LocalMembership
	ConfigService       driver.ConfigService
	Services            Services
	Ledger              driver.Ledger
	waitForEventTimeout time.Duration
	callback            driver.BlockCallback
	vault               Vault
	client              services.PeerClient
	tracer              trace.Tracer
	lastBlockReceived   uint64
	bufferSize          int
	stop                chan error
}

var (
	ctr = atomic.Uint32{}
)

func New(
	networkName string,
	channelConfig driver.ChannelConfig,
	LocalMembership driver.LocalMembership,
	ConfigService driver.ConfigService,
	PeerManager Services,
	Ledger driver.Ledger,
	callback driver.BlockCallback,
	vault Vault,
	waitForEventTimeout time.Duration,
	bufferSize int,
	tracerProvider tracing.Provider,
	_ metrics.Provider,
) (*Delivery, error) {
	if channelConfig == nil {
		return nil, errors.Errorf("expected channel config, got nil")
	}

	d := &Delivery{
		NetworkName:         networkName,
		channel:             channelConfig.ID(),
		channelConfig:       channelConfig,
		LocalMembership:     LocalMembership,
		ConfigService:       ConfigService,
		Services:            PeerManager,
		Ledger:              Ledger,
		waitForEventTimeout: waitForEventTimeout,
		tracer: tracerProvider.Tracer("delivery", tracing.WithMetricsOpts(tracing.MetricsOpts{
			LabelNames: []tracing.LabelName{messageTypeLabel},
		})),
		callback:   callback,
		vault:      vault,
		bufferSize: max(bufferSize, 1),
		stop:       make(chan error),
	}
	return d, nil
}

// Start runs the delivery service in a goroutine
func (d *Delivery) Start(ctx context.Context) {
	go utils.IgnoreErrorFunc(func() error {
		return d.Run(ctx)
	})
}

func (d *Delivery) Stop(err error) {
	logger.Debugf("stop delivery with error [%v]", err)
	if err != nil {
		d.stop <- err
	}
	close(d.stop)
}

func (d *Delivery) Run(ctx context.Context) error {
	logger.Debugf("Running delivery service [%d]", ctr.Add(1))
	if ctx == nil {
		ctx = context.Background()
	}
	ch := make(chan blockResponse, d.bufferSize)
	go d.readBlocks(ch)
	go d.runReceiver(ctx, ch)
	return d.untilStop()
}

func (d *Delivery) readBlocks(ch <-chan blockResponse) {
	for {
		select {
		case b := <-ch:
			logger.Debugf("Invoking callback for block [%d]", b.block.Header.Number)
			stop, err := d.callback(b.ctx, b.block)
			if err != nil {
				logger.Errorf("callback errored for block [%d], stop delivery: [%v]", b.block.Header.Number, err)
				d.Stop(err)
				return
			}
			if stop {
				logger.Debugf("stopping delivery at block [%d]", b.block.Header.Number)
				d.Stop(nil)
				return
			}
		case err := <-d.stop:
			logger.Debugf("stopping delivery service with err [%s]", err)
			return
		}
	}
}

func (d *Delivery) runReceiver(ctx context.Context, ch chan<- blockResponse) {
	if ctx == nil || ch == nil {
		return
	}
	var df DeliverStream
	var dfCancel context.CancelFunc
	var err error
	waitTime := d.channelConfig.DeliverySleepAfterFailure()
	counter := 0
	for {
		select {
		case <-d.stop:
			logger.Debugf("Stopped receiver")
			return
		default:
			select {
			case <-d.stop:
				logger.Debugf("Stopped receiver")
				if dfCancel != nil {
					dfCancel()
				}
				return
			case <-ctx.Done():
				logger.Debugf("Context done")
				// Time to cancel
				if dfCancel != nil {
					dfCancel()
				}
				d.Stop(errors.New("context done"))
			default:
				deliveryCtx, span := d.tracer.Start(context.Background(), "block_delivery", tracing.WithAttributes(tracing.String(messageTypeLabel, unknown)))
				if df == nil {
					logger.Debugf("deliver service [%s:%s], connecting...", d.NetworkName, d.channel)
					span.AddEvent("connect")
					df, dfCancel, err = d.connect(ctx)
					if err != nil {
						logger.Errorf("failed connecting to delivery service [%s:%s] [%s]. Wait %.1fs before reconnecting", d.NetworkName, d.channel, err, waitTime.Seconds())
						time.Sleep(waitTime)
						logger.Debugf("reconnecting to delivery service [%s:%s]", d.NetworkName, d.channel)
						span.RecordError(err)
						span.End()
						continue
					}
				}

				logger.Debugf("call receive, it is the [%d]-th time", counter)
				counter++
				span.AddEvent("wait_message")
				resp, err := df.Recv()
				span.AddEvent("received_message")
				if err != nil {
					if dfCancel != nil {
						dfCancel()
					}
					df = nil
					logger.Errorf("delivery service [%s:%s:%s], failed receiving response [%s]",
						d.client.Address(), d.NetworkName, d.channel,
						errors.WithMessagef(err, "error receiving deliver response from peer %s", d.client.Address()))
					span.RecordError(err)
					span.End()
					continue
				}

				switch r := resp.Type.(type) {
				case *pb.DeliverResponse_Block:
					span.SetAttributes(tracing.String(messageTypeLabel, block))
					if r.Block == nil || r.Block.Data == nil || r.Block.Header == nil || r.Block.Metadata == nil {
						logger.Debugf("deliver service [%s:%s:%s], received nil block", d.client.Address(), d.NetworkName, d.channel)
						span.RecordError(errors.New("nil block"))
						time.Sleep(waitTime)
						if dfCancel != nil {
							dfCancel()
						}
						df = nil
					}

					logger.Debugf("delivery service [%s:%s:%s], commit block [%d]", d.client.Address(), d.NetworkName, d.channel, r.Block.Header.Number)
					d.lastBlockReceived = r.Block.Header.Number

					span.AddEvent(fmt.Sprintf("push_%d_to_channel", r.Block.Header.Number))
					logger.Debugf("Pushing block [%d] to channel with current length %d", r.Block.Header.Number, len(ch))
					ch <- blockResponse{
						ctx:   deliveryCtx,
						block: r.Block,
					}
					logger.Debugf("Pushed block [%d] to channel", r.Block.Header.Number)
					span.AddEvent("pushed_to_channel")
				case *pb.DeliverResponse_Status:
					span.SetAttributes(tracing.String(messageTypeLabel, responseStatus))
					if r.Status == cb.Status_NOT_FOUND {
						span.RecordError(errors.New("not found"))
						df = nil
						if dfCancel != nil {
							dfCancel()
						}
						logger.Warnf("delivery service [%s:%s:%s] status [%s], wait a few seconds before retrying", d.client.Address(), d.NetworkName, d.channel, r.Status)
						time.Sleep(waitTime)
					} else {
						logger.Warnf("delivery service [%s:%s:%s] status [%s]", d.client.Address(), d.NetworkName, d.channel, r.Status)
					}
				default:
					span.SetAttributes(tracing.String(messageTypeLabel, other))
					df = nil
					if dfCancel != nil {
						dfCancel()
					}
					logger.Errorf("delivery service [%s:%s:%s], got [%s]", d.client.Address(), d.NetworkName, d.channel, r)
				}
				span.End()
			}
		}
	}
}

func (d *Delivery) untilStop() error {
	for err := range d.stop {
		logger.Debugf("stopping delivery service with error [%s]", err)
		return err
	}
	return nil
}

func (d *Delivery) connect(ctx context.Context) (DeliverStream, context.CancelFunc, error) {
	// first cleanup everything
	d.cleanup()

	peerConnConf := d.ConfigService.PickPeer(driver.PeerForDelivery)

	address := peerConnConf.Address
	logger.Debugf("connecting to deliver service at [%s] for [%s:%s]", address, d.NetworkName, d.channel)
	var err error
	d.client, err = d.Services.NewPeerClient(*peerConnConf)
	if err != nil {
		return nil, nil, errors.WithMessagef(err, "failed creating peer client for address [%s][%s:%s]", address, d.NetworkName, d.channel)
	}
	deliverClient, err := NewDeliverClient(d.client)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "failed to get deliver client")
	}
	newCtx, cancel := context.WithCancel(ctx)
	stream, err := deliverClient.NewDeliver(newCtx)
	if err != nil {
		cancel()
		return nil, nil, errors.Wrapf(err, "failed to get delivery stream")
	}

	blockEnvelope, err := CreateDeliverEnvelope(
		d.channel,
		d.LocalMembership.DefaultSigningIdentity(),
		deliverClient.Certificate(),
		d.GetStartPosition(newCtx),
	)
	if err != nil {
		cancel()
		return nil, nil, errors.Wrap(err, "failed to create deliver envelope")
	}
	err = DeliverSend(stream, blockEnvelope)
	if err != nil {
		cancel()
		return nil, nil, errors.Wrapf(err, "failed sending seek envelope to [%s]", address)
	}

	logger.Debugf("connected to deliver service at [%s]", address)
	return stream, cancel, nil
}

func (d *Delivery) GetStartPosition(ctx context.Context) *ab.SeekPosition {
	if d.lastBlockReceived != 0 {
		logger.Debugf("restarting from the last block received [%d]", d.lastBlockReceived)

		return &ab.SeekPosition{
			Type: &ab.SeekPosition_Specified{
				Specified: &ab.SeekSpecified{
					Number: d.lastBlockReceived,
				},
			},
		}
	}

	logger.Debugf("no last block received set [%d], check last TxID in the vault", d.lastBlockReceived)

	lastBlock, err := d.vault.GetLastBlock(ctx)
	if err == nil && lastBlock != 0 {
		return SeekPosition(lastBlock)
	}

	logger.Debugf("failed to get last block [%s], try with last tx", err)
	lastTxID, err := d.vault.GetLastTxID(ctx)
	if err != nil {
		logger.Errorf("failed getting last transaction committed/discarded from the vault [%s], restarting from genesis", err)
		return StartGenesis
	}

	if len(lastTxID) != 0 && !strings.HasPrefix(lastTxID, committer.ConfigTXPrefix) {
		// Retrieve block from Fabric
		blockNumber, err := d.Ledger.GetBlockNumberByTxID(lastTxID)
		if err != nil {
			logger.Errorf("failed getting block number for transaction [%s], restart from genesis: error: %v", lastTxID, err)
			return StartGenesis
		}
		logger.Debugf("restarting from block [%d], tx [%s]", blockNumber, lastTxID)

		return SeekPosition(blockNumber)
	}

	return StartGenesis
}

func SeekPosition(blockNumber uint64) *ab.SeekPosition {
	return &ab.SeekPosition{
		Type: &ab.SeekPosition_Specified{
			Specified: &ab.SeekSpecified{
				Number: blockNumber,
			},
		},
	}
}

func (d *Delivery) cleanup() {
	if d.client != nil {
		d.client.Close()
	}
}
