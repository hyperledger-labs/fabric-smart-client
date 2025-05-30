/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package committer

import (
	"context"
	"fmt"
	"sync"
	"time"

	errors2 "github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	committer2 "github.com/hyperledger-labs/fabric-smart-client/platform/common/core/generic/committer"
	driver2 "github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/logging"
	"github.com/hyperledger-labs/fabric-smart-client/platform/orion/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/events"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/tracing"
	"github.com/hyperledger-labs/orion-server/pkg/types"
	"github.com/pkg/errors"
	"go.opentelemetry.io/otel/trace"
)

const (
	txIdLabel tracing.LabelName = "tx_id"
)

var (
	logger = logging.MustGetLogger()
	// ErrDiscardTX this error can be used to signal that a valid transaction should be discarded anyway
	ErrDiscardTX = errors.New("discard tx")
	// ErrUnknownTX this erro can be used to signal that a transaction is unknown
	ErrUnknownTX = errors.New("unknown tx")
)

type (
	FinalityManager = committer2.FinalityManager[driver.ValidationCode]
	TxEvent         = driver2.FinalityEvent[driver.ValidationCode]
)

type Finality interface {
	IsFinal(txID string, address string) error
}

type Vault interface {
	Status(ctx context.Context, txID driver2.TxID) (driver.ValidationCode, string, error)
	SetStatus(ctx context.Context, txID driver2.TxID, code driver.ValidationCode) error
	Statuses(ctx context.Context, ids ...driver2.TxID) ([]driver.TxValidationStatus, error)
	DiscardTx(ctx context.Context, txID driver2.TxID, message string) error
	CommitTX(ctx context.Context, txID driver2.TxID, block driver2.BlockNum, indexInBloc driver2.TxNum) error
}

type ProcessorManager interface {
	ProcessByID(ctx context.Context, txID driver2.TxID) error
}

type committer struct {
	networkName         string
	vault               Vault
	finality            Finality
	pm                  ProcessorManager
	em                  driver.EnvelopeService
	waitForEventTimeout time.Duration
	TransactionFilters  *committer2.AggregatedTransactionFilter

	FinalityManager *FinalityManager
	quietNotifier   bool

	listeners      map[string][]chan TxEvent
	mutex          sync.Mutex
	pollingTimeout time.Duration

	eventsSubscriber events.Subscriber
	eventsPublisher  events.Publisher
	subscribers      *events.Subscribers

	// finality
	finalityNumRetries int
	finalitySleepTime  time.Duration
	tracer             trace.Tracer
}

func New(
	networkName string,
	pm ProcessorManager,
	em driver.EnvelopeService,
	vault Vault,
	finality Finality,
	waitForEventTimeout time.Duration,
	quiet bool,
	eventsPublisher events.Publisher,
	eventsSubscriber events.Subscriber,
	tracerProvider trace.TracerProvider,
	networkConfig driver.NetworkConfig,
	listenerManager driver.ListenerManager,
) (*committer, error) {
	d := &committer{
		networkName:         networkName,
		vault:               vault,
		waitForEventTimeout: waitForEventTimeout,
		quietNotifier:       quiet,
		listeners:           map[string][]chan TxEvent{},
		mutex:               sync.Mutex{},
		finality:            finality,
		pm:                  pm,
		em:                  em,
		pollingTimeout:      networkConfig.PollingTimeout(),
		FinalityManager:     committer2.NewFinalityManager[driver.ValidationCode](listenerManager, logger, vault, tracerProvider, networkConfig.FinalityEventQueueWorkers(), driver.Valid, driver.Invalid),
		eventsSubscriber:    eventsSubscriber,
		eventsPublisher:     eventsPublisher,
		subscribers:         events.NewSubscribers(),
		finalityNumRetries:  networkConfig.FinalityNumRetries(),
		finalitySleepTime:   networkConfig.FinalitySleepTime(),
		TransactionFilters:  committer2.NewAggregatedTransactionFilter(),
		tracer:              tracerProvider.Tracer("committer", tracing.WithMetricsOpts(tracing.MetricsOpts{Namespace: "orionsdk"})),
	}
	return d, nil
}

func (c *committer) Start(context context.Context) error {
	go c.FinalityManager.Run(context)
	return nil
}

// Commit commits the transactions in the block passed as argument
func (c *committer) Commit(block *types.AugmentedBlockHeader) error {
	ctx, span := c.tracer.Start(context.Background(), "commit_block")
	defer span.End()
	bn := block.Header.BaseHeader.Number
	for i, txID := range block.TxIds {
		span.AddEvent("process_tx")
		var event TxEvent
		event.TxID = txID
		event.Block = bn
		event.IndexInBlock = uint64(i)
		event.ValidationCode = convertValidationCode(block.Header.ValidationInfo[i].Flag)
		event.ValidationMessage = block.Header.ValidationInfo[i].ReasonIfInvalid

		discard := false
		switch block.Header.ValidationInfo[i].Flag {
		case types.Flag_VALID:
			span.AddEvent("commit_valid_tx")
			if err := c.CommitTX(ctx, txID, bn, driver.TxNum(i), &event); err != nil {
				if errors2.HasCause(err, ErrDiscardTX) {
					// in this case, we will discard the transaction
					event.ValidationCode = convertValidationCode(types.Flag_INVALID_INCORRECT_ENTRIES)
					event.ValidationMessage = err.Error()
					discard = true
				} else {
					return errors.Wrapf(err, "failed to commit tx %s", txID)
				}
			}
		default:
			span.AddEvent("discard_invalid_tx")
			discard = true
		}

		if discard {
			if err := c.DiscardTX(ctx, txID, bn, &event); err != nil {
				return errors.Wrapf(err, "failed to discard tx %s", txID)
			}
		}
		span.AddEvent("notify_tx_finality")
		c.notifyFinality(ctx, event)
	}
	return nil
}

func (c *committer) CommitTX(ctx context.Context, txID driver2.TxID, bn driver.BlockNum, index driver.TxNum, event *TxEvent) error {
	logger.DebugfContext(ctx, "transaction [%s] in block [%d] is valid for orion", txID, bn)

	// if is already committed, do nothing
	vc, _, err := c.vault.Status(ctx, txID)
	if err != nil {
		return errors.Wrapf(err, "failed to get status of tx %s", txID)
	}
	switch vc {
	case driver.Valid:
		logger.DebugfContext(ctx, "tx %s is already committed", txID)
		return nil
	case driver.Invalid:
		logger.DebugfContext(ctx, "tx %s is already invalid", txID)
		return errors.Errorf("tx %s is already invalid but it is marked as valid by orion", txID)
	case driver.Unknown:
		if !c.em.Exists(ctx, txID) {
			logger.Debugf("tx %s is unknown, check the transaction filters...", txID)
			return c.commitWithFilter(ctx, txID)
		}
		logger.DebugfContext(ctx, "tx %s is unknown, but it was found its envelope has been found, process it", txID)
	}

	// post process
	if err := c.pm.ProcessByID(ctx, txID); err != nil {
		return errors.Wrapf(err, "failed to process tx %s", txID)
	}

	// commit
	if err := c.vault.CommitTX(ctx, txID, bn, index); err != nil {
		return errors.WithMessagef(err, "failed to commit tx %s", txID)
	}

	return nil
}

func (c *committer) DiscardTX(ctx context.Context, txID string, blockNum uint64, event *TxEvent) error {
	logger.DebugfContext(ctx, "transaction [%s] in block [%d] is not valid for orion [%s], discard!", txID, blockNum, event.ValidationCode)

	vc, _, err := c.vault.Status(ctx, txID)
	if err != nil {
		return errors.Wrapf(err, "failed getting tx's status [%s]", txID)
	}
	switch vc {
	case driver.Valid:
		// TODO: this might be due the fact that there are transactions with the same tx-id, the first is valid, the others are all invalid
		logger.Warnf("transaction [%s] in block [%d] is marked as valid but for orion is invalid", txID, blockNum)
	case driver.Invalid:
		logger.Debugf("transaction [%s] in block [%d] is marked as invalid, skipping", txID, blockNum)
	default:
		event.Err = errors.Errorf("transaction [%s] status is not valid: %d", txID, event.ValidationCode)
		// rollback
		if err := c.vault.DiscardTx(ctx, txID, event.ValidationMessage); err != nil {
			return errors.WithMessagef(err, "failed to discard tx %s", txID)
		}
	}
	return nil
}

// IsFinal takes in input a transaction id and waits for its confirmation
// with the respect to the passed context that can be used to set a deadline
// for the waiting time.
func (c *committer) IsFinal(ctx context.Context, txID string) error {
	newCtx, span := c.tracer.Start(ctx, "finality_check", tracing.WithAttributes(tracing.String(txIdLabel, txID)))
	defer span.End()
	logger.Debugf("Is [%s] final?", txID)

	skipLoop := false
	for iter := 0; iter < c.finalityNumRetries; iter++ {
		span.AddEvent("retry")
		vd, _, err := c.vault.Status(ctx, txID)
		if err == nil {
			switch vd {
			case driver.Valid:
				logger.Debugf("Tx [%s] is valid", txID)
				return nil
			case driver.Invalid:
				logger.Debugf("Tx [%s] is not valid", txID)
				return errors.Errorf("transaction [%s] is not valid", txID)
			case driver.Busy:
				logger.Debugf("Tx [%s] is known", txID)
			case driver.Unknown:
				if c.em.Exists(ctx, txID) {
					logger.Debugf("found an envelope for [%s], consider it as known", txID)
					skipLoop = true
					break
				}

				// wait a bit to see if something changes
				if iter >= c.finalityNumRetries-1 {
					return errors2.Wrapf(ErrUnknownTX, "transaction [%s] is unknown", txID)
				}
				logger.Debugf("Tx [%s] is unknown with no deps, wait a bit and retry [%d]", txID, iter)
				time.Sleep(c.finalitySleepTime)
			default:
				panic(fmt.Sprintf("invalid status code, got %c", vd))
			}
		} else {
			logger.Errorf("Is [%s] final? Failed getting transaction status from vault", txID)
			return errors.WithMessagef(err, "failed getting transaction status from vault [%s]", txID)
		}
		if skipLoop {
			break
		}
	}

	// Listen to the event
	return c.listenToFinality(newCtx, txID, c.waitForEventTimeout)
}

func (c *committer) AddFinalityListener(txID string, listener driver.FinalityListener) error {
	return c.FinalityManager.AddListener(txID, listener)
}

func (c *committer) RemoveFinalityListener(txID string, listener driver.FinalityListener) error {
	c.FinalityManager.RemoveListener(txID, listener)
	return nil
}

func (c *committer) AddTransactionFilter(sr driver.TransactionFilter) error {
	c.TransactionFilters.Add(sr)
	return nil
}

func (c *committer) commitWithFilter(ctx context.Context, txID driver2.TxID) error {
	ok, err := c.TransactionFilters.Accept(txID, nil)
	if err != nil {
		return err
	}
	if ok {
		return c.vault.SetStatus(ctx, txID, driver.Valid)
	}
	return nil
}

func (c *committer) postFinality(ctx context.Context, txID string, vc driver.ValidationCode, message string) {
	c.FinalityManager.Post(TxEvent{
		Ctx:               ctx,
		TxID:              txID,
		ValidationCode:    vc,
		ValidationMessage: message,
	})
}

func (c *committer) addFinalityListener(txid string, ch chan TxEvent) {
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

func (c *committer) deleteFinalityListener(txid string, ch chan TxEvent) {
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

func (c *committer) notifyFinality(ctx context.Context, event TxEvent) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	logger.DebugfContext(ctx, "Post finality")
	c.postFinality(ctx, event.TxID, event.ValidationCode, event.ValidationMessage)

	if event.Err != nil && !c.quietNotifier {
		logger.Warningf("An error occurred for tx [%s], event: [%v]", event.TxID, event)
	}

	listeners := c.listeners[event.TxID]
	logger.Debugf("Notify the finality of [%s] to [%d] listeners, event: [%v]", event.TxID, len(listeners), event)

	for i, listener := range listeners {
		logger.DebugfContext(ctx, "Notify listener [%d]", i)
		listener <- event
	}
}

func (c *committer) listenToFinality(ctx context.Context, txID string, timeout time.Duration) error {
	logger.DebugfContext(ctx, "Listen to finality of [%s]", txID)

	// notice that adding the listener can happen after the event we are looking for has already happened
	// therefore we need to check more often before the timeout happens
	ch := make(chan TxEvent, 100)
	c.addFinalityListener(txID, ch)
	defer c.deleteFinalityListener(txID, ch)

	iterations := int(timeout.Milliseconds() / c.pollingTimeout.Milliseconds())
	if iterations == 0 {
		iterations = 1
	}
	for i := 0; i < iterations; i++ {
		logger.DebugfContext(ctx, "Start iteration [%d]", i)
		timeout := time.NewTimer(c.pollingTimeout)

		stop := false
		select {
		case <-ctx.Done():
			logger.DebugfContext(ctx, "Cancel context")
			timeout.Stop()
			stop = true
		case event := <-ch:
			logger.DebugfContext(ctx, "Got an answer to finality of [%s]: [%s]", txID, event.Err)
			timeout.Stop()
			return event.Err
		case <-timeout.C:
			timeout.Stop()
			logger.DebugfContext(ctx, "Got a timeout for finality of [%s], check the status", txID)
			vd, _, err := c.vault.Status(ctx, txID)
			if err == nil {
				switch vd {
				case driver.Valid:
					logger.Debugf("Listen to finality of [%s]. VALID", txID)
					return nil
				case driver.Invalid:
					logger.Debugf("Listen to finality of [%s]. NOT VALID", txID)
					return errors.Errorf("transaction [%s] is not valid", txID)
				}
			}
			logger.Debugf("Is [%s] final? not available yet, wait [err:%s, vc:%d]", txID, err, vd)
		}
		if stop {
			break
		}
	}
	logger.DebugfContext(ctx, "Is [%s] final? Failed to listen to transaction for timeout", txID)
	return errors.Errorf("failed to listen to transaction [%s] for timeout", txID)
}

func convertValidationCode(vc types.Flag) driver.ValidationCode {
	switch vc {
	case types.Flag_VALID:
		return driver.Valid
	default:
		return driver.Invalid
	}
}
