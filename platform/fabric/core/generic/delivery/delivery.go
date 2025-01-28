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

	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/utils"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/committer"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/services"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/grpc"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/tracing"
	"github.com/hyperledger/fabric-protos-go/common"
	ab "github.com/hyperledger/fabric-protos-go/orderer"
	pb "github.com/hyperledger/fabric-protos-go/peer"
	"github.com/pkg/errors"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap/zapcore"
)

var logger = logging.MustGetLogger("fabric-sdk.delivery")

var (
	ErrComm      = errors.New("communication issue")
	StartGenesis = &ab.SeekPosition{
		Type: &ab.SeekPosition_Oldest{
			Oldest: &ab.SeekOldest{},
		},
	}
)

type blockResponse struct {
	ctx   context.Context
	block *common.Block
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
	GetLastTxID() (string, error)
}

type Services interface {
	NewPeerClient(cc grpc.ConnectionConfig) (services.PeerClient, error)
}

type Delivery struct {
	channel             string
	channelConfig       driver.ChannelConfig
	hasher              Hasher
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
	stop                chan struct{}
}

func New(
	networkName string,
	channelConfig driver.ChannelConfig,
	hasher Hasher,
	LocalMembership driver.LocalMembership,
	ConfigService driver.ConfigService,
	PeerManager Services,
	Ledger driver.Ledger,
	callback driver.BlockCallback,
	vault Vault,
	waitForEventTimeout time.Duration,
	bufferSize int,
	tracerProvider trace.TracerProvider,
	_ metrics.Provider,
) (*Delivery, error) {
	if channelConfig == nil {
		return nil, errors.Errorf("expected channel config, got nil")
	}

	d := &Delivery{
		NetworkName:         networkName,
		channel:             channelConfig.ID(),
		channelConfig:       channelConfig,
		hasher:              hasher,
		LocalMembership:     LocalMembership,
		ConfigService:       ConfigService,
		Services:            PeerManager,
		Ledger:              Ledger,
		waitForEventTimeout: waitForEventTimeout,
		tracer: tracerProvider.Tracer("delivery", tracing.WithMetricsOpts(tracing.MetricsOpts{
			Namespace:  "fabricsdk",
			LabelNames: []tracing.LabelName{messageTypeLabel},
		})),
		callback:   callback,
		vault:      vault,
		bufferSize: max(bufferSize, 1),
		stop:       make(chan struct{}),
	}
	return d, nil
}

// Start runs the delivery service in a goroutine
func (d *Delivery) Start(ctx context.Context) {
	go utils.IgnoreErrorFunc(func() error {
		return d.Run(ctx)
	})
}

func (d *Delivery) Stop() {
	close(d.stop)
}

var ctr = atomic.Uint32{}

func (d *Delivery) Run(ctx context.Context) error {
	logger.Debugf("Running delivery service [%d]", ctr.Add(1))
	if ctx == nil {
		ctx = context.Background()
	}
	ch := make(chan blockResponse, d.bufferSize)
	go d.readBlocks(ch)
	return d.runReceiver(ctx, ch)
}

func (d *Delivery) readBlocks(ch <-chan blockResponse) {
	for {
		select {
		case b := <-ch:
			logger.Debugf("Invoking callback for block [%d]", b.block.Header)
			stop, err := d.callback(b.ctx, b.block)
			if err != nil {
				logger.Errorf("callback errored for block %d: %v", b.block.Header.Number, err)
			}
			if stop {
				logger.Infof("Stopping delivery at block [%d]", b.block.Header.Number)
				close(d.stop)
				return
			}
		case <-d.stop:
			logger.Infof("Stopping delivery service")
			return
		}
	}
}

func (d *Delivery) runReceiver(ctx context.Context, ch chan<- blockResponse) error {
	if ctx == nil || ch == nil {
		return errors.New("ctx and channel must be provided")
	}
	var df DeliverStream
	var err error
	waitTime := d.channelConfig.DeliverySleepAfterFailure()
	for {
		select {
		case <-d.stop:
			logger.Infof("Stopped receiver")
			return nil
		default:
			select {
			case <-d.stop:
				logger.Infof("Stopped receiver")
				return nil
			case <-ctx.Done():
				logger.Infof("Context done")
				// Time to cancel
				return errors.New("context done")
			default:
				deliveryCtx, span := d.tracer.Start(context.Background(), "block_delivery",
					tracing.WithAttributes(tracing.String(messageTypeLabel, unknown)))
				if df == nil {
					if logger.IsEnabledFor(zapcore.DebugLevel) {
						logger.Debugf("deliver service [%s], connecting...", d.NetworkName, d.channel)
					}
					span.AddEvent("connect")
					df, err = d.connect(ctx)
					if err != nil {
						logger.Errorf("failed connecting to delivery service [%s:%s] [%s]. Wait %.1fs before reconnecting", d.NetworkName, d.channel, err, waitTime.Seconds())
						time.Sleep(waitTime)
						if logger.IsEnabledFor(zapcore.DebugLevel) {
							logger.Debugf("reconnecting to delivery service [%s:%s]", d.NetworkName, d.channel)
						}
						span.RecordError(err)
						span.End()
						continue
					}
				}

				span.AddEvent("wait_message")
				resp, err := df.Recv()
				span.AddEvent("received_message")
				if err != nil {
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
						if logger.IsEnabledFor(zapcore.DebugLevel) {
							logger.Debugf("deliver service [%s:%s:%s], received nil block", d.client.Address(), d.NetworkName, d.channel)
						}
						span.RecordError(errors.New("nil block"))
						time.Sleep(waitTime)
						df = nil
					}

					if logger.IsEnabledFor(zapcore.DebugLevel) {
						logger.Debugf("delivery service [%s:%s:%s], commit block [%d]", d.client.Address(), d.NetworkName, d.channel, r.Block.Header.Number)
					}
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
					if r.Status == common.Status_NOT_FOUND {
						span.RecordError(errors.New("not found"))
						df = nil
						logger.Warnf("delivery service [%s:%s:%s] status [%s], wait a few seconds before retrying", d.client.Address(), d.NetworkName, d.channel, r.Status)
						time.Sleep(waitTime)
					} else {
						logger.Warnf("delivery service [%s:%s:%s] status [%s]", d.client.Address(), d.NetworkName, d.channel, r.Status)
					}
				default:
					span.SetAttributes(tracing.String(messageTypeLabel, other))
					df = nil
					logger.Errorf("delivery service [%s:%s:%s], got [%s]", d.client.Address(), d.NetworkName, d.channel, r)
				}
				span.End()
			}
		}
	}
}

func (d *Delivery) connect(ctx context.Context) (DeliverStream, error) {
	// first cleanup everything
	d.cleanup()

	peerConnConf := d.ConfigService.PickPeer(driver.PeerForDelivery)

	address := peerConnConf.Address
	if logger.IsEnabledFor(zapcore.DebugLevel) {
		logger.Debugf("connecting to deliver service at [%s] for [%s:%s]", address, d.NetworkName, d.channel)
	}
	var err error
	d.client, err = d.Services.NewPeerClient(*peerConnConf)
	if err != nil {
		return nil, errors.WithMessagef(err, "failed creating peer client for address [%s][%s:%s]", address, d.NetworkName, d.channel)
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
		d.LocalMembership.DefaultSigningIdentity(),
		deliverClient.Certificate(),
		d.hasher,
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
		blockNumber, err := d.Ledger.GetBlockNumberByTxID(lastTxID)
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
