/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package committer

import (
	"context"
	"fmt"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/compose"
	errors2 "github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/proto"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/core/generic/committer"
	driver2 "github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/fabricutils"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/membership"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/rwset"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/events"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/grpc"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/metrics"
	"github.com/hyperledger/fabric-protos-go/common"
	"github.com/hyperledger/fabric-protos-go/peer"
	"github.com/hyperledger/fabric/bccsp/factory"
	"github.com/hyperledger/fabric/common/channelconfig"
	"github.com/hyperledger/fabric/common/configtx"
	"github.com/hyperledger/fabric/protoutil"
	"github.com/pkg/errors"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/sync/errgroup"
)

const (
	channelConfigKey = "CHANNEL_CONFIG_ENV_BYTES"
	peerNamespace    = "_configtx"
	ConfigTXPrefix   = "configtx_"
)

var (
	// TODO: introduced due to a race condition in idemix.
	commitConfigMutex = &sync.Mutex{}
	logger            = logging.MustGetLogger("fabric-sdk.Committer")
	// ErrDiscardTX this error can be used to signal that a valid transaction should be discarded anyway
	ErrDiscardTX = errors.New("discard tx")
)

type (
	FinalityEvent   = driver2.FinalityEvent[driver.ValidationCode]
	FinalityManager = committer.FinalityManager[driver.ValidationCode]
)

type FabricFinality interface {
	IsFinal(txID string, address string) error
}

type CommitTx struct {
	BlkNum driver2.BlockNum
	TxNum  driver2.TxNum
	TxID   driver2.TxID
	Type   common.HeaderType

	Raw      []byte
	Envelope *common.Envelope
}

type TransactionHandler = func(ctx context.Context, block *common.BlockMetadata, tx CommitTx) (*FinalityEvent, error)

type OrderingService interface {
	SetConfigOrderers(o channelconfig.Orderer, orderers []*grpc.ConnectionConfig) error
}

type Committer struct {
	ConfigService driver.ConfigService
	ChannelConfig driver.ChannelConfig

	Vault              driver.Vault
	EnvelopeService    driver.EnvelopeService
	TransactionFilters *committer.AggregatedTransactionFilter
	ProcessNamespaces  []string
	Ledger             driver.Ledger
	RWSetLoaderService driver.RWSetLoader
	ProcessorManager   driver.ProcessorManager
	MembershipService  *membership.Service
	OrderingService    OrderingService
	FabricFinality     FabricFinality
	metrics            *Metrics
	TransactionManager driver.TransactionManager
	DependencyResolver DependencyResolver

	events chan FinalityEvent

	logger committer.Logger

	// events
	FinalityManager *FinalityManager
	EventsPublisher events.Publisher

	Handlers      map[common.HeaderType]TransactionHandler
	QuietNotifier bool

	listeners      map[string][]chan FinalityEvent
	mutex          sync.Mutex
	pollingTimeout time.Duration
}

func New(
	configService driver.ConfigService,
	channelConfig driver.ChannelConfig,
	vault driver.Vault,
	envelopeService driver.EnvelopeService,
	ledger driver.Ledger,
	rwsetLoaderService driver.RWSetLoader,
	processorManager driver.ProcessorManager,
	eventsPublisher events.Publisher,
	channelMembershipService *membership.Service,
	orderingService OrderingService,
	fabricFinality FabricFinality,
	transactionManager driver.TransactionManager,
	dependencyResolver DependencyResolver,
	quiet bool,
	listenerManager driver.ListenerManager,
	tracerProvider trace.TracerProvider,
	metricsProvider metrics.Provider,
) *Committer {
	s := &Committer{
		ConfigService:      configService,
		ChannelConfig:      channelConfig,
		Vault:              vault,
		EnvelopeService:    envelopeService,
		TransactionFilters: committer.NewAggregatedTransactionFilter(),
		ProcessNamespaces:  []string{},
		Ledger:             ledger,
		RWSetLoaderService: rwsetLoaderService,
		ProcessorManager:   processorManager,
		MembershipService:  channelMembershipService,
		OrderingService:    orderingService,
		FinalityManager:    committer.NewFinalityManager[driver.ValidationCode](listenerManager, logger, vault, tracerProvider, channelConfig.FinalityEventQueueWorkers(), driver.Valid, driver.Invalid),
		EventsPublisher:    eventsPublisher,
		FabricFinality:     fabricFinality,
		TransactionManager: transactionManager,
		DependencyResolver: dependencyResolver,
		QuietNotifier:      quiet,
		metrics:            NewMetrics(tracerProvider, metricsProvider),
		logger:             logger.Named(fmt.Sprintf("[%s:%s]", configService.NetworkName(), channelConfig.ID())),
		listeners:          map[string][]chan FinalityEvent{},
		Handlers:           map[common.HeaderType]TransactionHandler{},
		pollingTimeout:     1 * time.Second,
		events:             make(chan FinalityEvent, 2000),
	}
	s.Handlers[common.HeaderType_CONFIG] = s.HandleConfig
	s.Handlers[common.HeaderType_ENDORSER_TRANSACTION] = s.HandleEndorserTransaction
	return s
}

func (c *Committer) Start(context context.Context) error {
	go c.FinalityManager.Run(context)
	go c.runEventNotifiers(context)
	return nil
}

func (c *Committer) runEventNotifiers(context context.Context) {
	for {
		select {
		case <-context.Done():
			return
		case event := <-c.events:
			c.metrics.EventQueueLength.Add(-1)
			start := time.Now()
			c.notifyFinality(event)
			c.metrics.NotifyFinalityDuration.Observe(time.Since(start).Seconds())

			start = time.Now()
			c.FinalityManager.Post(event)
			c.metrics.PostFinalityDuration.Observe(time.Since(start).Seconds())

			start = time.Now()
			var driverVC driver.ValidationCode
			if peer.TxValidationCode(event.ValidationCode) == peer.TxValidationCode_VALID {
				driverVC = driver.Valid
			} else {
				driverVC = driver.Invalid
			}
			c.notifyTxStatus(event.TxID, driverVC, event.ValidationMessage)
			c.metrics.NotifyStatusDuration.Observe(time.Since(start).Seconds())
		}
	}
}

func (c *Committer) Status(txID string) (driver.ValidationCode, string, error) {
	vc, message, err := c.Vault.Status(txID)
	if err != nil {
		c.logger.Errorf("failed to get status of [%s]: %s", txID, err)
		return driver.Unknown, "", err
	}
	if vc == driver.Unknown {
		// give it a second chance
		if c.EnvelopeService.Exists(txID) {
			if err := c.extractStoredEnvelopeToVault(txID); err != nil {
				return driver.Unknown, "", errors.WithMessagef(err, "failed to extract stored enveloper for [%s]", txID)
			}
			vc = driver.Busy
		}
	}
	return vc, message, nil
}

func (c *Committer) ProcessNamespace(nss ...string) error {
	c.ProcessNamespaces = append(c.ProcessNamespaces, nss...)
	return nil
}

func (c *Committer) AddTransactionFilter(sr driver.TransactionFilter) error {
	c.TransactionFilters.Add(sr)
	return nil
}

func (c *Committer) DiscardTx(txID string, message string) error {
	c.logger.Debugf("discarding transaction [%s] with message [%s]", txID, message)

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
			c.logger.Debugf("Discarding transaction [%s] skipped, tx is unknown", txID)
			if err := c.Vault.SetDiscarded(txID, message); err != nil {
				c.logger.Errorf("failed setting tx discarded [%s] in vault: %s", txID, err)
			}
			return nil
		}
	}

	if err := c.Vault.DiscardTx(txID, message); err != nil {
		c.logger.Errorf("failed discarding tx [%s] in vault: %s", txID, err)
	}
	return nil
}

func (c *Committer) CommitTX(ctx context.Context, txID string, block driver.BlockNum, indexInBlock driver.TxNum, envelope *common.Envelope) (err error) {
	c.logger.Debugf("Committing transaction [%s,%d,%d]", txID, block, indexInBlock)
	defer c.logger.Debugf("Committing transaction [%s,%d,%d] done [%s]", txID, block, indexInBlock, err)

	vc, _, err := c.Status(txID)
	if err != nil {
		return errors.WithMessagef(err, "failed getting tx's status in state db [%s]", txID)
	}
	switch vc {
	case driver.Valid:
		// This should generate a panic
		c.logger.Debugf("[%s] is already valid", txID)
		return errors.Errorf("[%s] is already valid", txID)
	case driver.Invalid:
		// This should generate a panic
		c.logger.Debugf("[%s] is invalid", txID)
		return errors.Errorf("[%s] is invalid", txID)
	case driver.Unknown:
		return c.commitUnknown(ctx, txID, block, indexInBlock, envelope)
	case driver.Busy:
		return c.commit(ctx, txID, block, indexInBlock, envelope)
	default:
		return errors.Errorf("invalid status code [%d] for [%s]", vc, txID)
	}
}

func (c *Committer) AddFinalityListener(txID string, listener driver.FinalityListener) error {
	return c.FinalityManager.AddListener(txID, listener)
}

func (c *Committer) RemoveFinalityListener(txID string, listener driver.FinalityListener) error {
	c.FinalityManager.RemoveListener(txID, listener)
	return nil
}

// CommitConfig is used to validate and apply configuration transactions for a Channel.
func (c *Committer) CommitConfig(blockNumber uint64, raw []byte, env *common.Envelope) error {
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

	txID := ConfigTXPrefix + strconv.FormatUint(ctx.Config.Sequence, 10)
	vc, _, err := c.Vault.Status(txID)
	if err != nil {
		return errors.Wrapf(err, "failed getting tx's status [%s]", txID)
	}
	switch vc {
	case driver.Valid:
		c.logger.Infof("config block [%s] already committed, skip it.", txID)
		return nil
	case driver.Unknown:
		c.logger.Infof("config block [%s] not committed, commit it.", txID)
		// this is okay
	default:
		return errors.Errorf("invalid configtx's [%s] status [%d]", txID, vc)
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

	if err := c.commitConfig(txID, blockNumber, ctx.Config.Sequence, raw); err != nil {
		return errors.Wrapf(err, "failed committing configtx to the vault")
	}

	return c.applyBundle(bundle)
}

// Commit commits the transactions in the block passed as argument
func (c *Committer) Commit(ctx context.Context, block *common.Block) error {
	newCtx, span := c.metrics.Commits.Start(ctx, "commit_block")
	defer span.End()

	txs, err := unmarshalTxs(block)
	if err != nil {
		return errors.Wrapf(err, "[%s] unmarshal tx failed", c.ChannelConfig.ID())
	}

	resolvedTxs := c.DependencyResolver.Resolve(txs)

	return c.commitTxs(newCtx, resolvedTxs, block.Metadata)
}

func (c *Committer) commitTxs(ctx context.Context, parallelizableTxGroups ParallelExecutable[SerialExecutable[CommitTx]], blockMetadata *common.BlockMetadata) error {
	start := time.Now()
	var eg errgroup.Group
	eg.SetLimit(c.ChannelConfig.CommitParallelism())
	for _, txGroup := range parallelizableTxGroups {
		txs := txGroup
		eg.Go(func() error {
			for _, tx := range txs {
				newCtx, span := c.metrics.Commits.Start(ctx, "commit_tx")
				span.AddEvent("create_finality_event")

				start := time.Now()
				if handler, ok := c.Handlers[tx.Type]; !ok {
					c.logger.Debugf("[%s] Received unhandled transaction type: %s", c.ChannelConfig.ID(), tx.Type)
					c.metrics.HandlerDuration.With("status", "not_found").Observe(time.Since(start).Seconds())
					span.End()
				} else if event, err := handler(newCtx, blockMetadata, tx); err != nil {
					span.End()
					c.metrics.HandlerDuration.With("status", "failure").Observe(time.Since(start).Seconds())
					return errors.Wrapf(err, "failed calling handler for tx [%s]", tx.TxID)
				} else {
					c.logger.Debugf("commit transaction [%s] in filteredBlock [%d]", event.TxID, tx.BlkNum)
					span.AddEvent("call_finality_notifiers")
					c.metrics.HandlerDuration.With("status", "successful").Observe(time.Since(start).Seconds())
					start := time.Now()
					c.events <- *event
					c.metrics.EventQueueDuration.Observe(time.Since(start).Seconds())
					c.metrics.EventQueueLength.Add(1)
					span.End()
				}
			}
			return nil
		})
	}
	err := eg.Wait()
	c.metrics.BlockCommitDuration.Observe(time.Since(start).Seconds())
	return err
}

func unmarshalTxs(block *common.Block) ([]CommitTx, error) {
	txs := make([]CommitTx, len(block.Data.Data))
	for i, tx := range block.Data.Data {
		env, _, chdr, err := fabricutils.UnmarshalTx(tx)
		if err != nil {
			return nil, errors.Wrapf(err, "unmarshal tx failed")
		}
		txs[i] = CommitTx{
			BlkNum:   block.Header.Number,
			TxNum:    uint64(i),
			TxID:     chdr.TxId,
			Type:     common.HeaderType(chdr.Type),
			Raw:      tx,
			Envelope: env,
		}
	}
	return txs, nil
}

// IsFinal takes in input a transaction id and waits for its confirmation
// with the respect to the passed context that can be used to set a deadline
// for the waiting time.
func (c *Committer) IsFinal(ctx context.Context, txID string) error {
	c.logger.Debugf("Is [%s] final?", txID)

	for iter := 0; iter < c.ChannelConfig.CommitterFinalityNumRetries(); iter++ {
		vd, _, err := c.Status(txID)
		if err != nil {
			c.logger.Errorf("Is [%s] final? Failed getting transaction status from vault", txID)
			return errors.WithMessagef(err, "failed getting transaction status from vault [%s]", txID)
		}

		switch vd {
		case driver.Valid:
			c.logger.Debugf("Tx [%s] is valid", txID)
			return nil
		case driver.Invalid:
			c.logger.Debugf("Tx [%s] is not valid", txID)
			return errors.Errorf("transaction [%s] is not valid", txID)
		case driver.Busy:
			c.logger.Debugf("Tx [%s] is known", txID)
			continue
		case driver.Unknown:
			if iter <= 1 {
				c.logger.Debugf("Tx [%s] is unknown with no deps, wait a bit and retry [%d]", txID, iter)
				time.Sleep(c.ChannelConfig.CommitterFinalityUnknownTXTimeout())
			}

			c.logger.Debugf("Tx [%s] is unknown with no deps, remote check [%d][%s]", txID, iter, debug.Stack())
			peerForFinality := c.ConfigService.PickPeer(driver.PeerForFinality).Address
			err := c.FabricFinality.IsFinal(txID, peerForFinality)
			if err == nil {
				c.logger.Debugf("Tx [%s] is final, remote check on [%s]", txID, peerForFinality)
				return nil
			}

			if vd, _, err2 := c.Status(txID); err2 == nil && vd == driver.Unknown {
				c.logger.Debugf("Tx [%s] is not final for remote [%s], return [%s], [%d][%s]", txID, peerForFinality, err, vd, err2)
				return err
			}
		default:
			return errors.Errorf("invalid status code, got [%c]", vd)
		}
	}

	c.logger.Debugf("Tx [%s] start listening...", txID)
	// Listen to the event
	return c.listenTo(ctx, txID, c.ChannelConfig.CommitterWaitForEventTimeout())
}

func (c *Committer) GetProcessNamespace() []string {
	return c.ProcessNamespaces
}

func (c *Committer) ReloadConfigTransactions() error {
	c.MembershipService.ResourcesApplyLock.Lock()
	defer c.MembershipService.ResourcesApplyLock.Unlock()

	qe, err := c.Vault.NewQueryExecutor()
	if err != nil {
		return errors.WithMessagef(err, "failed getting query executor")
	}
	defer qe.Done()

	c.logger.Infof("looking up the latest config block available")
	var sequence uint64 = 0
	for {
		txID := ConfigTXPrefix + strconv.FormatUint(sequence, 10)
		vc, _, err := c.Vault.Status(txID)
		if err != nil {
			return errors.WithMessagef(err, "failed getting tx's status [%s]", txID)
		}
		c.logger.Infof("check config block at txID [%s], status [%v]...", txID, vc)
		done := false
		switch vc {
		case driver.Valid:
			c.logger.Infof("config block available, txID [%s], loading...", txID)

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

			c.logger.Infof("config block at txID [%s] unavailable, stop loading", txID)
			done = true
		default:
			return errors.Errorf("invalid configtx's [%s] status [%d]", txID, vc)
		}
		if done {
			c.logger.Infof("loading config block done")
			break
		}
	}
	if sequence == 1 {
		c.logger.Infof("no config block available, must start from genesis")
		// no configuration block found
		return nil
	}
	c.logger.Infof("latest config block available at sequence [%d]", sequence-1)

	return nil
}

func (c *Committer) addListener(txID string, ch chan FinalityEvent) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	ls, ok := c.listeners[txID]
	if !ok {
		ls = []chan FinalityEvent{}
		c.listeners[txID] = ls
	}
	ls = append(ls, ch)
	c.listeners[txID] = ls
}

func (c *Committer) deleteListener(txID string, ch chan FinalityEvent) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	ls, ok := c.listeners[txID]
	if !ok {
		return
	}
	for i, l := range ls {
		if l == ch {
			ls = append(ls[:i], ls[i+1:]...)
			c.listeners[txID] = ls
			return
		}
	}
}

func (c *Committer) notifyFinality(event FinalityEvent) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if event.Err != nil && !c.QuietNotifier {
		c.logger.Warnf("An error occurred for tx [%s], event: [%v]", event.TxID, event)
	}

	trace.SpanFromContext(event.Ctx).AddEvent("notify_listeners")
	listeners := c.listeners[event.TxID]
	c.logger.Debugf("Notify the finality of [%s] to [%d] listeners, event: [%v]", event.TxID, len(listeners), event)
	for _, listener := range listeners {
		listener <- event
	}
}

// notifyChaincodeListeners notifies the chaincode event to the registered chaincode listeners.
func (c *Committer) notifyChaincodeListeners(event *ChaincodeEvent) {
	c.EventsPublisher.Publish(event)
}

func (c *Committer) listenTo(ctx context.Context, txID string, timeout time.Duration) error {
	_, span := c.metrics.Listens.Start(ctx, "committer-listenTo-start")
	defer span.End()

	c.logger.Debugf("Listen to finality of [%s]", txID)

	// notice that adding the listener can happen after the event we are looking for has already happened
	// therefore we need to check more often before the timeout happens
	ch := make(chan FinalityEvent, 100)
	c.addListener(txID, ch)
	defer c.deleteListener(txID, ch)

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
			span.AddEvent("receive_channel_result")
			span.AddLink(trace.LinkFromContext(event.Ctx))
			trace.SpanFromContext(event.Ctx).AddEvent("return_channel_result")
			c.logger.Debugf("Got an answer to finality of [%s]: [%s]", txID, event.Err)
			timeout.Stop()
			return event.Err
		case <-timeout.C:
			span.AddEvent("check_vault_status")
			timeout.Stop()
			c.logger.Debugf("Got a timeout for finality of [%s], check the status", txID)
			vd, _, err := c.Status(txID)
			if err == nil {
				switch vd {
				case driver.Valid:
					c.logger.Debugf("Listen to finality of [%s]. VALID", txID)
					return nil
				case driver.Invalid:
					c.logger.Debugf("Listen to finality of [%s]. NOT VALID", txID)
					return errors.Errorf("transaction [%s] is not valid", txID)
				}
			}
			c.logger.Debugf("Is [%s] final? not available yet, wait [err:%s, vc:%d]", txID, err, vd)
		}
		if stop {
			break
		}
	}
	c.logger.Debugf("Is [%s] final? Failed to listen to transaction for timeout", txID)
	return errors.Errorf("failed to listen to transaction [%s] for timeout", txID)
}

func (c *Committer) commitConfig(txID string, blockNumber uint64, seq uint64, envelope []byte) error {
	c.logger.Infof("[Channel: %s] commit config transaction number [bn:%d][seq:%d]", c.ChannelConfig.ID(), blockNumber, seq)

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
	if err := c.CommitTX(context.Background(), txID, blockNumber, 0, nil); err != nil {
		if err2 := c.DiscardTx(txID, err.Error()); err2 != nil {
			c.logger.Errorf("failed committing configtx rws [%s]", err2)
		}
		return errors.Wrapf(err, "failed committing configtx rws")
	}
	return nil
}

func (c *Committer) commit(ctx context.Context, txID string, block uint64, indexInBlock uint64, envelope *common.Envelope) error {
	span := trace.SpanFromContext(ctx)
	// This is a normal transaction, validated by Fabric.
	// Commit it cause Fabric says it is valid.
	c.logger.Debugf("[%s] committing", txID)

	// Match rwsets if envelope is not empty
	if envelope != nil {
		c.logger.Debugf("[%s] matching rwsets", txID)

		span.AddEvent("new_tx_from_payload")
		pt, headerType, err := c.TransactionManager.NewProcessedTransactionFromEnvelopePayload(envelope.Payload)
		if err != nil && headerType == -1 {
			c.logger.Errorf("[%s] failed to unmarshal envelope [%s]", txID, err)
			return err
		}
		if headerType == int32(common.HeaderType_ENDORSER_TRANSACTION) {
			if !c.Vault.RWSExists(txID) && c.EnvelopeService.Exists(txID) {
				// Then match rwsets
				span.AddEvent("extract_stored_env_to_vault")
				if err := c.extractStoredEnvelopeToVault(txID); err != nil {
					return errors.WithMessagef(err, "failed to load stored enveloper into the vault")
				}
				span.AddEvent("match_rwset")
				if err := c.Vault.Match(txID, pt.Results()); err != nil {
					c.logger.Errorf("[%s] rwsets do not match [%s]", txID, err)
					return errors2.Wrapf(ErrDiscardTX, "[%s] rwsets do not match [%s]", txID, err)
				}
			} else {
				// Store it
				envelopeRaw, err := proto.Marshal(envelope)
				if err != nil {
					return errors.WithMessagef(err, "failed to store unknown envelope for [%s]", txID)
				}
				span.AddEvent("store_env")
				if err := c.EnvelopeService.StoreEnvelope(txID, envelopeRaw); err != nil {
					return errors.WithMessagef(err, "failed to store unknown envelope for [%s]", txID)
				}
				span.AddEvent("get_rwset_from_evn")
				rws, _, err := c.RWSetLoaderService.GetRWSetFromEvn(txID)
				if err != nil {
					return errors.WithMessagef(err, "failed to get rws from envelope [%s]", txID)
				}
				rws.Done()
			}
		}
	}

	// Post-Processes
	c.logger.Debugf("[%s] post process rwset", txID)

	span.AddEvent("post_process_tx")
	if err := c.postProcessTx(txID); err != nil {
		// This should generate a panic
		return err
	}

	// Commit
	c.logger.Debugf("[%s] commit in vault", txID)
	span.AddEvent("commit_to_vault")
	if err := c.Vault.CommitTX(ctx, txID, block, indexInBlock); err != nil {
		// This should generate a panic
		return err
	}

	return nil
}

func (c *Committer) commitUnknown(ctx context.Context, txID string, block uint64, indexInBlock uint64, envelope *common.Envelope) error {
	// if an envelope exists for the passed txID, then commit it
	if c.EnvelopeService.Exists(txID) {
		return c.commitStoredEnvelope(ctx, txID, block, indexInBlock)
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
		c.logger.Debugf("[%s] unknown envelope will not be processed [%b,%s]", txID, ok, err)
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
	return c.commit(ctx, txID, block, indexInBlock, envelope)
}

func (c *Committer) commitStoredEnvelope(ctx context.Context, txID string, block uint64, indexInBlock uint64) error {
	c.logger.Debugf("found envelope for transaction [%s], committing it...", txID)
	if err := c.extractStoredEnvelopeToVault(txID); err != nil {
		return err
	}
	// commit
	return c.commit(ctx, txID, block, indexInBlock, nil)
}

func (c *Committer) applyBundle(bundle *channelconfig.Bundle) error {
	c.MembershipService.ResourcesLock.Lock()
	defer c.MembershipService.ResourcesLock.Unlock()
	c.MembershipService.ChannelResources = bundle

	// update the list of orderers
	ordererConfig, exists := c.MembershipService.ChannelResources.OrdererConfig()
	if !exists {
		c.logger.Infof("no orderer configuration found in Channel config")
		return nil
	}
	c.logger.Debugf("[Channel: %s] Orderer config has changed, updating the list of orderers", c.ChannelConfig.ID())

	tlsEnabled, isSet := c.ConfigService.OrderingTLSEnabled()
	if !isSet {
		tlsEnabled = c.ConfigService.TLSEnabled()
	}
	tlsClientSideAuth, isSet := c.ConfigService.OrderingTLSClientAuthRequired()
	if !isSet {
		tlsClientSideAuth = c.ConfigService.TLSClientAuthRequired()
	}
	connectionTimeout := c.ConfigService.ClientConnTimeout()

	var newOrderers []*grpc.ConnectionConfig
	orgs := ordererConfig.Organizations()
	for _, org := range orgs {
		msp := org.MSP()
		var tlsRootCerts [][]byte
		tlsRootCerts = append(tlsRootCerts, msp.GetTLSRootCerts()...)
		tlsRootCerts = append(tlsRootCerts, msp.GetTLSIntermediateCerts()...)
		for _, endpoint := range org.Endpoints() {
			if len(endpoint) == 0 {
				c.logger.Debugf("[Channel: %s] empty endpoint for %s, skipping", c.ChannelConfig.ID(), org.MSPID())
				continue
			}
			c.logger.Debugf("[Channel: %s] Adding orderer endpoint: [%s:%s:%s]", c.ChannelConfig.ID(), org.Name(), org.MSPID(), endpoint)
			newOrderers = append(newOrderers, &grpc.ConnectionConfig{
				Address:           endpoint,
				ConnectionTimeout: connectionTimeout,
				TLSEnabled:        tlsEnabled,
				TLSClientSideAuth: tlsClientSideAuth,
				TLSRootCertBytes:  tlsRootCerts,
			})
		}
		// If the Orderer MSP config omits the Endpoints and there is only one orderer org, we try to get the addresses from another key in the channel config.
		// This is only here for backwards compatibility and is deprecated in Fabric 3.
		// https://hyperledger-fabric.readthedocs.io/en/latest/upgrade_to_newest_version.html#define-ordering-node-endpoint-per-org
		addr := bundle.ChannelConfig().OrdererAddresses()
		if len(newOrderers) == 0 && len(orgs) == 1 && len(addr) > 0 {
			c.logger.Infof("falling back to OrdererAddresses field in channel config (deprecated, please refer to Fabric docs)")
			for _, endpoint := range addr {
				if len(endpoint) == 0 {
					c.logger.Debugf("[Channel: %s] empty orderer address, skipping", c.ChannelConfig.ID())
					continue
				}
				c.logger.Debugf("[Channel: %s] Adding orderer address [%s:%s:%s]", c.ChannelConfig.ID(), org.Name(), org.MSPID(), endpoint)
				newOrderers = append(newOrderers, &grpc.ConnectionConfig{
					Address:           endpoint,
					ConnectionTimeout: connectionTimeout,
					TLSEnabled:        tlsEnabled,
					TLSClientSideAuth: tlsClientSideAuth,
					TLSRootCertBytes:  tlsRootCerts,
				})
			}
		}
	}
	if len(newOrderers) != 0 {
		c.logger.Debugf("[Channel: %s] Updating the list of orderers: (%d) found", c.ChannelConfig.ID(), len(newOrderers))
		return c.OrderingService.SetConfigOrderers(ordererConfig, newOrderers)
	}
	c.logger.Infof("[Channel: %s] No orderers found in Channel config", c.ChannelConfig.ID())

	return nil
}

func (c *Committer) fetchEnvelope(txID string) ([]byte, error) {
	pt, err := c.Ledger.GetTransactionByID(txID)
	if err != nil {
		return nil, errors.WithMessagef(err, "failed fetching tx [%s]", txID)
	}
	if !pt.IsValid() {
		return nil, errors.Errorf("fetched tx [%s] should have been valid, instead it is [%s]", txID, peer.TxValidationCode_name[pt.ValidationCode()])
	}
	return pt.Envelope(), nil
}

func (c *Committer) filterUnknownEnvelope(txID string, envelope []byte) (bool, error) {
	rws, _, err := c.RWSetLoaderService.GetInspectingRWSetFromEvn(txID, envelope)
	if err != nil {
		return false, errors.WithMessagef(err, "failed to get rws from envelope [%s]", txID)
	}
	defer rws.Done()

	// check namespaces
	c.logger.Debugf("[%s] contains namespaces [%v] or `initialized` key", txID, rws.Namespaces())
	for _, ns := range rws.Namespaces() {
		for _, namespace := range c.ProcessNamespaces {
			if namespace == ns {
				c.logger.Debugf("[%s] contains namespaces [%v], select it", txID, rws.Namespaces())
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
				c.logger.Debugf("[%s] contains 'initialized' key [%v] in [%s], select it", txID, ns, rws.Namespaces())
				return true, nil
			}
		}
	}

	// check the filters
	if ok, err := c.TransactionFilters.Accept(txID, envelope); err != nil || ok {
		return ok, err
	}

	status, _, _ := c.Status(txID)
	return status == driver.Busy, nil
}

func (c *Committer) extractStoredEnvelopeToVault(txID string) error {
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

func (c *Committer) postProcessTx(txID string) error {
	if err := c.ProcessorManager.ProcessByID(c.ChannelConfig.ID(), txID); err != nil {
		// This should generate a panic
		return err
	}
	return nil
}

func (c *Committer) notifyTxStatus(txID string, vc driver.ValidationCode, message string) {
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
