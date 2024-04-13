/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package committer

import (
	"context"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/compose"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/proto"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/fabricutils"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/membership"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/rwset"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/transaction"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/events"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/flogging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/grpc"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/tracing"
	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/bccsp/factory"
	"github.com/hyperledger/fabric/common/channelconfig"
	"github.com/hyperledger/fabric/common/configtx"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/pkg/errors"
	"go.uber.org/zap/zapcore"
)

const (
	channelConfigKey = "CHANNEL_CONFIG_ENV_BYTES"
	peerNamespace    = "_configtx"
	ConfigTXPrefix   = "configtx_"
)

var (
	// TODO: introduced due to a race condition in idemix.
	commitConfigMutex = &sync.Mutex{}
	logger            = flogging.MustGetLogger("fabric-sdk.Committer")
	// ErrDiscardTX this error can be used to signal that a valid transaction should be discarded anyway
	ErrDiscardTX = errors.New("discard tx")
)

type FabricFinality interface {
	IsFinal(txID string, address string) error
}

type TransactionHandler = func(block *common.Block, i int, event *TxEvent, env *common.Envelope, chHdr *common.ChannelHeader) error

type OrderingService interface {
	SetConfigOrderers(o channelconfig.Orderer, orderers []*grpc.ConnectionConfig) error
}

type Service struct {
	ConfigService driver.ConfigService
	ChannelConfig driver.ChannelConfig

	Vault              driver.Vault
	EnvelopeService    driver.EnvelopeService
	StatusReporters    []driver.StatusReporter
	ProcessNamespaces  []string
	Ledger             driver.Ledger
	RWSetLoaderService driver.RWSetLoader
	ProcessorManager   driver.ProcessorManager
	MembershipService  *membership.Service
	OrderingService    OrderingService
	FabricFinality     FabricFinality
	Tracer             tracing.Tracer

	// events
	Subscribers      *events.Subscribers
	EventsSubscriber events.Subscriber
	EventsPublisher  events.Publisher

	WaitForEventTimeout time.Duration
	Handlers            map[common.HeaderType]TransactionHandler
	QuietNotifier       bool

	listeners      map[string][]chan TxEvent
	mutex          sync.Mutex
	pollingTimeout time.Duration
}

func NewService(
	ConfigService driver.ConfigService,
	channelConfig driver.ChannelConfig,
	vault driver.Vault,
	envelopeService driver.EnvelopeService,
	ledger driver.Ledger,
	RWSetLoaderService driver.RWSetLoader,
	processorManager driver.ProcessorManager,
	eventsSubscriber events.Subscriber,
	eventsPublisher events.Publisher,
	ChannelMembershipService *membership.Service,
	OrderingService OrderingService,
	fabricFinality FabricFinality,
	waitForEventTimeout time.Duration,
	quiet bool,
	metrics tracing.Tracer,
) *Service {
	s := &Service{
		ConfigService:       ConfigService,
		ChannelConfig:       channelConfig,
		Vault:               vault,
		EnvelopeService:     envelopeService,
		StatusReporters:     []driver.StatusReporter{},
		ProcessNamespaces:   []string{},
		Ledger:              ledger,
		RWSetLoaderService:  RWSetLoaderService,
		ProcessorManager:    processorManager,
		MembershipService:   ChannelMembershipService,
		OrderingService:     OrderingService,
		Subscribers:         events.NewSubscribers(),
		EventsSubscriber:    eventsSubscriber,
		EventsPublisher:     eventsPublisher,
		FabricFinality:      fabricFinality,
		WaitForEventTimeout: waitForEventTimeout,
		QuietNotifier:       quiet,
		Tracer:              metrics,
		listeners:           map[string][]chan TxEvent{},
		Handlers:            map[common.HeaderType]TransactionHandler{},
		pollingTimeout:      1 * time.Second,
	}
	s.Handlers[common.HeaderType_CONFIG] = s.HandleConfig
	s.Handlers[common.HeaderType_ENDORSER_TRANSACTION] = s.HandleEndorserTransaction
	return s
}

func (c *Service) Status(txID string) (driver.ValidationCode, string, error) {
	vc, message, err := c.Vault.Status(txID)
	if err != nil {
		logger.Errorf("failed to get status of [%s]: %s", txID, err)
		return driver.Unknown, "", err
	}
	if vc == driver.Unknown {
		// give it a second chance
		if c.EnvelopeService.Exists(txID) {
			if err := c.extractStoredEnvelopeToVault(txID); err != nil {
				return driver.Unknown, "", errors.WithMessagef(err, "failed to extract stored enveloper for [%s]", txID)
			}
			vc = driver.Busy
		} else {
			// check status reporter, if any
			for _, reporter := range c.StatusReporters {
				externalStatus, externalMessage, _, err := reporter.Status(txID)
				if err == nil && externalStatus != driver.Unknown {
					vc = externalStatus
					message = externalMessage
				}
			}
		}
	}
	return vc, message, nil
}

func (c *Service) ProcessNamespace(nss ...string) error {
	c.ProcessNamespaces = append(c.ProcessNamespaces, nss...)
	return nil
}

func (c *Service) AddStatusReporter(sr driver.StatusReporter) error {
	c.StatusReporters = append(c.StatusReporters, sr)
	return nil
}

func (c *Service) DiscardTx(txID string, message string) error {
	logger.Debugf("discarding transaction [%s] with message [%s]", txID, message)

	defer c.notifyTxStatus(txID, driver.Invalid, message)
	vc, _, err := c.Status(txID)
	if err != nil {
		return errors.WithMessagef(err, "failed getting tx's status in state db [%s]", txID)
	}
	if vc == driver.Unknown {
		// give it a second chance
		if c.EnvelopeService.Exists(txID) {
			if err := c.extractStoredEnvelopeToVault(txID); err != nil {
				return errors.WithMessagef(err, "failed to extract stored enveloper for [%s]", txID)
			}
		} else {
			// check status reporter, if any
			found := false
			for _, reporter := range c.StatusReporters {
				externalStatus, _, _, err := reporter.Status(txID)
				if err == nil && externalStatus != driver.Unknown {
					found = true
					break
				}
			}
			if !found {
				logger.Debugf("Discarding transaction [%s] skipped, tx is unknown", txID)
				return nil
			}
		}
	}

	if err := c.Vault.DiscardTx(txID, message); err != nil {
		logger.Errorf("failed discarding tx [%s] in vault: %s", txID, err)
	}
	return nil
}

func (c *Service) CommitTX(txID string, block uint64, indexInBlock int, envelope *common.Envelope) (err error) {
	logger.Debugf("Committing transaction [%s,%d,%d]", txID, block, indexInBlock)
	defer logger.Debugf("Committing transaction [%s,%d,%d] done [%s]", txID, block, indexInBlock, err)
	defer func() {
		if err == nil {
			c.notifyTxStatus(txID, driver.Valid, "")
		}
	}()

	vc, _, err := c.Status(txID)
	if err != nil {
		return errors.WithMessagef(err, "failed getting tx's status in state db [%s]", txID)
	}
	switch vc {
	case driver.Valid:
		// This should generate a panic
		logger.Debugf("[%s] is already valid", txID)
		return errors.Errorf("[%s] is already valid", txID)
	case driver.Invalid:
		// This should generate a panic
		logger.Debugf("[%s] is invalid", txID)
		return errors.Errorf("[%s] is invalid", txID)
	case driver.Unknown:
		return c.commitUnknown(txID, block, indexInBlock, envelope)
	case driver.Busy:
		return c.commit(txID, block, indexInBlock, envelope)
	default:
		return errors.Errorf("invalid status code [%d] for [%s]", vc, txID)
	}
}

func (c *Service) SubscribeTxStatusChanges(txID string, listener driver.TxStatusChangeListener) error {
	_, topic := compose.CreateTxTopic(c.ConfigService.NetworkName(), c.ChannelConfig.ID(), txID)
	l := &TxEventsListener{listener: listener}
	logger.Debugf("[%s] Subscribing to transaction status changes", txID)
	c.EventsSubscriber.Subscribe(topic, l)
	logger.Debugf("[%s] store mapping", txID)
	c.Subscribers.Set(topic, listener, l)
	logger.Debugf("[%s] Subscribing to transaction status changes done", txID)
	return nil
}

func (c *Service) UnsubscribeTxStatusChanges(txID string, listener driver.TxStatusChangeListener) error {
	_, topic := compose.CreateTxTopic(c.ConfigService.NetworkName(), c.ChannelConfig.ID(), txID)
	l, ok := c.Subscribers.Get(topic, listener)
	if !ok {
		return errors.Errorf("listener not found for txID [%s]", txID)
	}
	el, ok := l.(events.Listener)
	if !ok {
		return errors.Errorf("listener not found for txID [%s]", txID)
	}
	c.Subscribers.Delete(topic, listener)
	c.EventsSubscriber.Unsubscribe(topic, el)
	return nil
}

// CommitConfig is used to validate and apply configuration transactions for a Channel.
func (c *Service) CommitConfig(blockNumber uint64, raw []byte, env *common.Envelope) error {
	commitConfigMutex.Lock()
	defer commitConfigMutex.Unlock()

	c.MembershipService.ResourcesApplyLock.Lock()
	defer c.MembershipService.ResourcesApplyLock.Unlock()

	if env == nil {
		return errors.Errorf("Channel config found nil")
	}

	payload, err := protoutil.UnmarshalPayload(env.Payload)
	if err != nil {
		return errors.Wrapf(err, "cannot get payload from config transaction, block number [%d]", blockNumber)
	}

	ctx, err := configtx.UnmarshalConfigEnvelope(payload.Data)
	if err != nil {
		return errors.Wrapf(err, "error unmarshalling config which passed initial validity checks")
	}

	txid := ConfigTXPrefix + strconv.FormatUint(ctx.Config.Sequence, 10)
	vc, _, err := c.Vault.Status(txid)
	if err != nil {
		return errors.Wrapf(err, "failed getting tx's status [%s]", txid)
	}
	switch vc {
	case driver.Valid:
		logger.Infof("config block [%s] already committed, skip it.", txid)
		return nil
	case driver.Unknown:
		logger.Infof("config block [%s] not committed, commit it.", txid)
		// this is okay
	default:
		return errors.Errorf("invalid configtx's [%s] status [%d]", txid, vc)
	}

	var bundle *channelconfig.Bundle
	if c.MembershipService.Resources() == nil {
		// set up the genesis block
		bundle, err = channelconfig.NewBundle(c.ChannelConfig.ID(), ctx.Config, factory.GetDefault())
		if err != nil {
			return errors.Wrapf(err, "failed to build a new bundle")
		}
	} else {
		configTxValidator := c.MembershipService.Resources().ConfigtxValidator()
		err := configTxValidator.Validate(ctx)
		if err != nil {
			return errors.Wrapf(err, "failed to validate config transaction, block number [%d]", blockNumber)
		}

		bundle, err = channelconfig.NewBundle(configTxValidator.ChannelID(), ctx.Config, factory.GetDefault())
		if err != nil {
			return errors.Wrapf(err, "failed to create next bundle")
		}

		channelconfig.LogSanityChecks(bundle)
		if err := capabilitiesSupported(bundle); err != nil {
			return err
		}
	}

	if err := c.commitConfig(txid, blockNumber, ctx.Config.Sequence, raw); err != nil {
		return errors.Wrapf(err, "failed committing configtx to the vault")
	}

	return c.applyBundle(bundle)
}

// Commit commits the transactions in the block passed as argument
func (c *Service) Commit(block *common.Block) error {
	c.Tracer.StartAt("commit", time.Now())
	for i, tx := range block.Data.Data {

		env, _, chdr, err := fabricutils.UnmarshalTx(tx)
		if err != nil {
			logger.Errorf("[%s] unmarshal tx failed: %s", c.ChannelConfig.ID(), err)
			return err
		}

		var event TxEvent
		c.Tracer.AddEventAt("commit", "start", time.Now())
		handler, ok := c.Handlers[common.HeaderType(chdr.Type)]
		if ok {
			if err := handler(block, i, &event, env, chdr); err != nil {
				return err
			}
		} else {
			if logger.IsEnabledFor(zapcore.DebugLevel) {
				logger.Debugf("[%s] Received unhandled transaction type: %s", c.ChannelConfig.ID(), chdr.Type)
			}
		}
		c.Tracer.AddEventAt("commit", "end", time.Now())

		c.notify(event)
		if logger.IsEnabledFor(zapcore.DebugLevel) {
			logger.Debugf("commit transaction [%s] in filteredBlock [%d]", chdr.TxId, block.Header.Number)
		}
	}

	return nil
}

// IsFinal takes in input a transaction id and waits for its confirmation
// with the respect to the passed context that can be used to set a deadline
// for the waiting time.
func (c *Service) IsFinal(ctx context.Context, txID string) error {
	if logger.IsEnabledFor(zapcore.DebugLevel) {
		logger.Debugf("Is [%s] final?", txID)
	}

	for iter := 0; iter < c.ChannelConfig.CommitterFinalityNumRetries(); iter++ {
		vd, _, err := c.Status(txID)
		if err != nil {
			logger.Errorf("Is [%s] final? Failed getting transaction status from vault", txID)
			return errors.WithMessagef(err, "failed getting transaction status from vault [%s]", txID)
		}

		switch vd {
		case driver.Valid:
			logger.Debugf("Tx [%s] is valid", txID)
			return nil
		case driver.Invalid:
			logger.Debugf("Tx [%s] is not valid", txID)
			return errors.Errorf("transaction [%s] is not valid", txID)
		case driver.Busy:
			logger.Debugf("Tx [%s] is known", txID)
			continue
		case driver.Unknown:
			if iter <= 1 {
				logger.Debugf("Tx [%s] is unknown with no deps, wait a bit and retry [%d]", txID, iter)
				time.Sleep(c.ChannelConfig.CommitterFinalityUnknownTXTimeout())
			}

			if logger.IsEnabledFor(zapcore.DebugLevel) {
				logger.Debugf("Tx [%s] is unknown with no deps, remote check [%d][%s]", txID, iter, debug.Stack())
			}
			peer := c.ConfigService.PickPeer(driver.PeerForFinality).Address
			err := c.FabricFinality.IsFinal(txID, peer)
			if err == nil {
				logger.Debugf("Tx [%s] is final, remote check on [%s]", txID, peer)
				return nil
			}

			if vd, _, err2 := c.Status(txID); err2 == nil && vd == driver.Unknown {
				logger.Debugf("Tx [%s] is not final for remote [%s], return [%s], [%d][%s]", txID, peer, err, vd, err2)
				return err
			}
		default:
			return errors.Errorf("invalid status code, got [%c]", vd)
		}
	}

	logger.Debugf("Tx [%s] start listening...", txID)
	// Listen to the event
	return c.listenTo(ctx, txID, c.WaitForEventTimeout)
}

func (c *Service) GetProcessNamespace() []string {
	return c.ProcessNamespaces
}

func (c *Service) ReloadConfigTransactions() error {
	c.MembershipService.ResourcesApplyLock.Lock()
	defer c.MembershipService.ResourcesApplyLock.Unlock()

	qe, err := c.Vault.NewQueryExecutor()
	if err != nil {
		return errors.WithMessagef(err, "failed getting query executor")
	}
	defer qe.Done()

	logger.Infof("looking up the latest config block available")
	var sequence uint64 = 0
	for {
		txID := ConfigTXPrefix + strconv.FormatUint(sequence, 10)
		vc, _, err := c.Vault.Status(txID)
		if err != nil {
			return errors.WithMessagef(err, "failed getting tx's status [%s]", txID)
		}
		logger.Infof("check config block at txID [%s], status [%v]...", txID, vc)
		done := false
		switch vc {
		case driver.Valid:
			logger.Infof("config block available, txID [%s], loading...", txID)

			key, err := rwset.CreateCompositeKey(channelConfigKey, []string{strconv.FormatUint(sequence, 10)})
			if err != nil {
				return errors.Wrapf(err, "cannot create configtx rws key")
			}
			envelope, err := qe.GetState(peerNamespace, key)
			if err != nil {
				return errors.Wrapf(err, "failed setting configtx state in rws")
			}
			env, err := protoutil.UnmarshalEnvelope(envelope)
			if err != nil {
				return errors.Wrapf(err, "cannot get payload from config transaction [%s]", txID)
			}
			payload, err := protoutil.UnmarshalPayload(env.Payload)
			if err != nil {
				return errors.Wrapf(err, "cannot get payload from config transaction [%s]", txID)
			}
			ctx, err := configtx.UnmarshalConfigEnvelope(payload.Data)
			if err != nil {
				return errors.Wrapf(err, "error unmarshalling config which passed initial validity checks [%s]", txID)
			}

			var bundle *channelconfig.Bundle
			if c.MembershipService.Resources() == nil {
				// set up the genesis block
				bundle, err = channelconfig.NewBundle(c.ChannelConfig.ID(), ctx.Config, factory.GetDefault())
				if err != nil {
					return errors.Wrapf(err, "failed to build a new bundle")
				}
			} else {
				configTxValidator := c.MembershipService.Resources().ConfigtxValidator()
				err := configTxValidator.Validate(ctx)
				if err != nil {
					return errors.Wrapf(err, "failed to validate config transaction [%s]", txID)
				}

				bundle, err = channelconfig.NewBundle(configTxValidator.ChannelID(), ctx.Config, factory.GetDefault())
				if err != nil {
					return errors.Wrapf(err, "failed to create next bundle")
				}

				channelconfig.LogSanityChecks(bundle)
				if err := capabilitiesSupported(bundle); err != nil {
					return err
				}
			}

			if err := c.applyBundle(bundle); err != nil {
				return err
			}

			sequence = sequence + 1
			continue
		case driver.Unknown:
			if sequence == 0 {
				// Give a chance to 1, in certain setting the first block starts with 1
				sequence++
				continue
			}

			logger.Infof("config block at txID [%s] unavailable, stop loading", txID)
			done = true
		default:
			return errors.Errorf("invalid configtx's [%s] status [%d]", txID, vc)
		}
		if done {
			logger.Infof("loading config block done")
			break
		}
	}
	if sequence == 1 {
		logger.Infof("no config block available, must start from genesis")
		// no configuration block found
		return nil
	}
	logger.Infof("latest config block available at sequence [%d]", sequence-1)

	return nil
}

func (c *Service) addListener(txid string, ch chan TxEvent) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	ls, ok := c.listeners[txid]
	if !ok {
		ls = []chan TxEvent{}
		c.listeners[txid] = ls
	}
	ls = append(ls, ch)
	c.listeners[txid] = ls
}

func (c *Service) deleteListener(txid string, ch chan TxEvent) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	ls, ok := c.listeners[txid]
	if !ok {
		return
	}
	for i, l := range ls {
		if l == ch {
			ls = append(ls[:i], ls[i+1:]...)
			c.listeners[txid] = ls
			return
		}
	}
}

func (c *Service) notify(event TxEvent) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if event.Err != nil && !c.QuietNotifier {
		logger.Warningf("An error occurred for tx [%s], event: [%v]", event.TxID, event)
	}

	listeners := c.listeners[event.TxID]
	if logger.IsEnabledFor(zapcore.DebugLevel) {
		logger.Debugf("Notify the finality of [%s] to [%d] listeners, event: [%v]", event.TxID, len(listeners), event)
	}
	for _, listener := range listeners {
		listener <- event
	}
}

// notifyChaincodeListeners notifies the chaincode event to the registered chaincode listeners.
func (c *Service) notifyChaincodeListeners(event *ChaincodeEvent) {
	c.EventsPublisher.Publish(event)
}

func (c *Service) listenTo(ctx context.Context, txid string, timeout time.Duration) error {
	c.Tracer.Start("committer-listenTo-start")

	if logger.IsEnabledFor(zapcore.DebugLevel) {
		logger.Debugf("Listen to finality of [%s]", txid)
	}

	// notice that adding the listener can happen after the event we are looking for has already happened
	// therefore we need to check more often before the timeout happens
	ch := make(chan TxEvent, 100)
	c.addListener(txid, ch)
	defer c.deleteListener(txid, ch)

	iterations := int(timeout.Milliseconds() / c.pollingTimeout.Milliseconds())
	if iterations == 0 {
		iterations = 1
	}
	for i := 0; i < iterations; i++ {
		timeout := time.NewTimer(c.pollingTimeout)

		stop := false
		select {
		case <-ctx.Done():
			timeout.Stop()
			stop = true
		case event := <-ch:
			if logger.IsEnabledFor(zapcore.DebugLevel) {
				logger.Debugf("Got an answer to finality of [%s]: [%s]", txid, event.Err)
			}
			timeout.Stop()
			return event.Err
		case <-timeout.C:
			timeout.Stop()
			if logger.IsEnabledFor(zapcore.DebugLevel) {
				logger.Debugf("Got a timeout for finality of [%s], check the status", txid)
			}
			vd, _, err := c.Status(txid)
			if err == nil {
				switch vd {
				case driver.Valid:
					if logger.IsEnabledFor(zapcore.DebugLevel) {
						logger.Debugf("Listen to finality of [%s]. VALID", txid)
					}
					return nil
				case driver.Invalid:
					if logger.IsEnabledFor(zapcore.DebugLevel) {
						logger.Debugf("Listen to finality of [%s]. NOT VALID", txid)
					}
					return errors.Errorf("transaction [%s] is not valid", txid)
				}
			}
			if logger.IsEnabledFor(zapcore.DebugLevel) {
				logger.Debugf("Is [%s] final? not available yet, wait [err:%s, vc:%d]", txid, err, vd)
			}
		}
		if stop {
			break
		}
	}
	if logger.IsEnabledFor(zapcore.DebugLevel) {
		logger.Debugf("Is [%s] final? Failed to listen to transaction for timeout", txid)
	}
	c.Tracer.End("committer-listenTo-end")
	return errors.Errorf("failed to listen to transaction [%s] for timeout", txid)
}

func (c *Service) commitConfig(txID string, blockNumber uint64, seq uint64, envelope []byte) error {
	logger.Infof("[Channel: %s] commit config transaction number [bn:%d][seq:%d]", c.ChannelConfig.ID(), blockNumber, seq)

	rws, err := c.Vault.NewRWSet(txID)
	if err != nil {
		return errors.Wrapf(err, "cannot create rws for configtx")
	}
	defer rws.Done()

	key, err := rwset.CreateCompositeKey(channelConfigKey, []string{strconv.FormatUint(seq, 10)})
	if err != nil {
		return errors.Wrapf(err, "cannot create configtx rws key")
	}
	if err := rws.SetState(peerNamespace, key, envelope); err != nil {
		return errors.Wrapf(err, "failed setting configtx state in rws")
	}
	rws.Done()
	if err := c.CommitTX(txID, blockNumber, 0, nil); err != nil {
		if err2 := c.DiscardTx(txID, ""); err2 != nil {
			logger.Errorf("failed committing configtx rws [%s]", err2)
		}
		return errors.Wrapf(err, "failed committing configtx rws")
	}
	return nil
}

func (c *Service) commit(txID string, block uint64, indexInBlock int, envelope *common.Envelope) error {
	// This is a normal transaction, validated by Fabric.
	// Commit it cause Fabric says it is valid.
	logger.Debugf("[%s] committing", txID)

	// Match rwsets if envelope is not empty
	if envelope != nil {
		logger.Debugf("[%s] matching rwsets", txID)

		pt, headerType, err := transaction.NewProcessedTransactionFromEnvelope(envelope)
		if err != nil && headerType == -1 {
			logger.Errorf("[%s] failed to unmarshal envelope [%s]", txID, err)
			return err
		}
		if headerType == int32(common.HeaderType_ENDORSER_TRANSACTION) {
			if !c.Vault.RWSExists(txID) && c.EnvelopeService.Exists(txID) {
				// Then match rwsets
				if err := c.extractStoredEnvelopeToVault(txID); err != nil {
					return errors.WithMessagef(err, "failed to load stored enveloper into the vault")
				}
				if err := c.Vault.Match(txID, pt.Results()); err != nil {
					logger.Errorf("[%s] rwsets do not match [%s]", txID, err)
					return errors.Wrapf(ErrDiscardTX, "[%s] rwsets do not match [%s]", txID, err)
				}
			} else {
				// Store it
				envelopeRaw, err := proto.Marshal(envelope)
				if err != nil {
					return errors.WithMessagef(err, "failed to store unknown envelope for [%s]", txID)
				}
				if err := c.EnvelopeService.StoreEnvelope(txID, envelopeRaw); err != nil {
					return errors.WithMessagef(err, "failed to store unknown envelope for [%s]", txID)
				}
				rws, _, err := c.RWSetLoaderService.GetRWSetFromEvn(txID)
				if err != nil {
					return errors.WithMessagef(err, "failed to get rws from envelope [%s]", txID)
				}
				rws.Done()
			}
		}
	}

	// Post-Processes
	logger.Debugf("[%s] post process rwset", txID)

	if err := c.postProcessTx(txID); err != nil {
		// This should generate a panic
		return err
	}

	// Commit
	logger.Debugf("[%s] commit in vault", txID)
	if err := c.Vault.CommitTX(txID, block, indexInBlock); err != nil {
		// This should generate a panic
		return err
	}

	return nil
}

func (c *Service) commitUnknown(txID string, block uint64, indexInBlock int, envelope *common.Envelope) error {
	// if an envelope exists for the passed txID, then commit it
	if c.EnvelopeService.Exists(txID) {
		return c.commitStoredEnvelope(txID, block, indexInBlock)
	}

	var envelopeRaw []byte
	var err error
	if envelope != nil {
		// Store it
		envelopeRaw, err = proto.Marshal(envelope)
		if err != nil {
			return errors.WithMessagef(err, "failed to store unknown envelope for [%s]", txID)
		}
	} else {
		// fetch envelope and store it
		envelopeRaw, err = c.fetchEnvelope(txID)
		if err != nil {
			return errors.WithMessagef(err, "failed getting rwset for tx [%s]", txID)
		}
	}

	// shall we commit this unknown envelope
	if ok, err := c.filterUnknownEnvelope(txID, envelopeRaw); err != nil || !ok {
		logger.Debugf("[%s] unknown envelope will not be processed [%b,%s]", txID, ok, err)
		return nil
	}

	if err := c.EnvelopeService.StoreEnvelope(txID, envelopeRaw); err != nil {
		return errors.WithMessagef(err, "failed to store unknown envelope for [%s]", txID)
	}
	rws, _, err := c.RWSetLoaderService.GetRWSetFromEvn(txID)
	if err != nil {
		return errors.WithMessagef(err, "failed to get rws from envelope [%s]", txID)
	}
	rws.Done()
	return c.commit(txID, block, indexInBlock, envelope)
}

func (c *Service) commitStoredEnvelope(txID string, block uint64, indexInBlock int) error {
	logger.Debugf("found envelope for transaction [%s], committing it...", txID)
	if err := c.extractStoredEnvelopeToVault(txID); err != nil {
		return err
	}
	// commit
	return c.commit(txID, block, indexInBlock, nil)
}

func (c *Service) applyBundle(bundle *channelconfig.Bundle) error {
	c.MembershipService.ResourcesLock.Lock()
	defer c.MembershipService.ResourcesLock.Unlock()
	c.MembershipService.ChannelResources = bundle

	// update the list of orderers
	ordererConfig, exists := c.MembershipService.ChannelResources.OrdererConfig()
	if !exists {
		logger.Infof("no orderer configuration found in Channel config")
		return nil
	}
	logger.Debugf("[Channel: %s] Orderer config has changed, updating the list of orderers", c.ChannelConfig.ID())

	var newOrderers []*grpc.ConnectionConfig
	orgs := ordererConfig.Organizations()
	for _, org := range orgs {
		msp := org.MSP()
		var tlsRootCerts [][]byte
		tlsRootCerts = append(tlsRootCerts, msp.GetTLSRootCerts()...)
		tlsRootCerts = append(tlsRootCerts, msp.GetTLSIntermediateCerts()...)
		for _, endpoint := range org.Endpoints() {
			logger.Debugf("[Channel: %s] Adding orderer endpoint: [%s:%s:%s]", c.ChannelConfig.ID(), org.Name(), org.MSPID(), endpoint)
			// TODO: load from configuration
			newOrderers = append(newOrderers, &grpc.ConnectionConfig{
				Address:           endpoint,
				ConnectionTimeout: 10 * time.Second,
				TLSEnabled:        true,
				TLSRootCertBytes:  tlsRootCerts,
			})
		}
		// If the Orderer MSP config omits the Endpoints and there is only one orderer org, we try to get the addresses from another key in the channel config.
		if len(newOrderers) == 0 && len(orgs) == 1 {
			for _, endpoint := range bundle.ChannelConfig().OrdererAddresses() {
				logger.Debugf("[Channel: %s] Adding orderer address [%s:%s:%s]", c.ChannelConfig.ID(), org.Name(), org.MSPID(), endpoint)
				newOrderers = append(newOrderers, &grpc.ConnectionConfig{
					Address:           endpoint,
					ConnectionTimeout: 10 * time.Second,
					TLSEnabled:        true,
					TLSRootCertBytes:  tlsRootCerts,
				})
			}
		}
	}
	if len(newOrderers) != 0 {
		logger.Debugf("[Channel: %s] Updating the list of orderers: (%d) found", c.ChannelConfig.ID(), len(newOrderers))
		return c.OrderingService.SetConfigOrderers(ordererConfig, newOrderers)
	}
	logger.Infof("[Channel: %s] No orderers found in Channel config", c.ChannelConfig.ID())

	return nil
}

func (c *Service) fetchEnvelope(txID string) ([]byte, error) {
	pt, err := c.Ledger.GetTransactionByID(txID)
	if err != nil {
		return nil, errors.WithMessagef(err, "failed fetching tx [%s]", txID)
	}
	if !pt.IsValid() {
		return nil, errors.Errorf("fetched tx [%s] should have been valid, instead it is [%s]", txID, peer.TxValidationCode_name[pt.ValidationCode()])
	}
	return pt.Envelope(), nil
}

func (c *Service) filterUnknownEnvelope(txID string, envelope []byte) (bool, error) {
	rws, _, err := c.RWSetLoaderService.GetInspectingRWSetFromEvn(txID, envelope)
	if err != nil {
		return false, errors.WithMessagef(err, "failed to get rws from envelope [%s]", txID)
	}
	defer rws.Done()

	logger.Debugf("[%s] contains namespaces [%v] or `initialized` key", txID, rws.Namespaces())
	for _, ns := range rws.Namespaces() {
		for _, namespace := range c.ProcessNamespaces {
			if namespace == ns {
				logger.Debugf("[%s] contains namespaces [%v], select it", txID, rws.Namespaces())
				return true, nil
			}
		}

		// search a read dependency on a key containing "initialized"
		for pos := 0; pos < rws.NumReads(ns); pos++ {
			k, err := rws.GetReadKeyAt(ns, pos)
			if err != nil {
				return false, errors.WithMessagef(err, "Error reading key at [%d]", pos)
			}
			if strings.Contains(k, "initialized") {
				logger.Debugf("[%s] contains 'initialized' key [%v] in [%s], select it", txID, ns, rws.Namespaces())
				return true, nil
			}
		}
	}

	status, _, _ := c.Status(txID)
	return status == driver.Busy, nil
}

func (c *Service) extractStoredEnvelopeToVault(txID string) error {
	rws, _, err := c.RWSetLoaderService.GetRWSetFromEvn(txID)
	if err != nil {
		// If another replica of the same node created the RWSet
		rws, _, err = c.RWSetLoaderService.GetRWSetFromETx(txID)
		if err != nil {
			return errors.WithMessagef(err, "failed to extract rws from envelope and etx [%s]", txID)
		}
	}
	rws.Done()
	return nil
}

func (c *Service) postProcessTx(txID string) error {
	if err := c.ProcessorManager.ProcessByID(c.ChannelConfig.ID(), txID); err != nil {
		// This should generate a panic
		return err
	}
	return nil
}

func (c *Service) notifyTxStatus(txID string, vc driver.ValidationCode, message string) {
	// We publish two events here:
	// 1. The first will be caught by the listeners that are listening for any transaction id.
	// 2. The second will be caught by the listeners that are listening for the specific transaction id.
	sb, topic := compose.CreateTxTopic(c.ConfigService.NetworkName(), c.ChannelConfig.ID(), "")
	c.EventsPublisher.Publish(&driver.TransactionStatusChanged{
		ThisTopic:         topic,
		TxID:              txID,
		VC:                vc,
		ValidationMessage: message,
	})
	c.EventsPublisher.Publish(&driver.TransactionStatusChanged{
		ThisTopic:         compose.AppendAttributesOrPanic(sb, txID),
		TxID:              txID,
		VC:                vc,
		ValidationMessage: message,
	})
}

func capabilitiesSupported(res channelconfig.Resources) error {
	ac, ok := res.ApplicationConfig()
	if !ok {
		return errors.Errorf("[Channel %s] does not have application config so is incompatible", res.ConfigtxValidator().ChannelID())
	}

	if err := ac.Capabilities().Supported(); err != nil {
		return errors.Wrapf(err, "[Channel %s] incompatible", res.ConfigtxValidator().ChannelID())
	}

	if err := res.ChannelConfig().Capabilities().Supported(); err != nil {
		return errors.Wrapf(err, "[Channel %s] incompatible", res.ConfigtxValidator().ChannelID())
	}

	return nil
}

type TxEventsListener struct {
	listener driver.TxStatusChangeListener
}

func (l *TxEventsListener) OnReceive(event events.Event) {
	tsc := event.Message().(*driver.TransactionStatusChanged)
	if err := l.listener.OnStatusChange(tsc.TxID, int(tsc.VC), tsc.ValidationMessage); err != nil {
		logger.Errorf("failed to notify listener for tx [%s] with err [%s]", tsc.TxID, err)
	}
}
