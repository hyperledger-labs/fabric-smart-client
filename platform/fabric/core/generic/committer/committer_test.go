/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package committer

import (
	"context"
	stderrors "errors"
	"testing"
	"time"

	"github.com/hyperledger/fabric-protos-go-apiv2/common"
	"github.com/hyperledger/fabric-protos-go-apiv2/peer"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace/noop"

	commoncommitter "github.com/hyperledger-labs/fabric-smart-client/platform/common/core/generic/committer"
	cdriver "github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	fdriver "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/events"
)

func TestProcessNamespaceAndGetProcessNamespace(t *testing.T) {
	t.Parallel()

	c := &Committer{}
	require.NoError(t, c.ProcessNamespace("ns1", "ns2"))
	require.Equal(t, []string{"ns1", "ns2"}, c.GetProcessNamespace())
}

func TestAddTransactionFilter(t *testing.T) {
	t.Parallel()

	c := &Committer{TransactionFilters: commoncommitter.NewAggregatedTransactionFilter()}
	require.NoError(t, c.AddTransactionFilter(&testFilter{accept: true}))

	ok, err := c.TransactionFilters.Accept("tx1", []byte("env"))
	require.NoError(t, err)
	require.True(t, ok)
}

func TestStatusUnknownWithoutStoredEnvelope(t *testing.T) {
	t.Parallel()

	c := &Committer{
		logger: logger,
		Vault: &testVault{
			statusFn: func(context.Context, cdriver.TxID) (fdriver.ValidationCode, string, error) {
				return fdriver.Unknown, "", nil
			},
		},
		EnvelopeService: &testEnvelopeService{
			existsFn: func(context.Context, string) bool { return false },
		},
	}

	vc, msg, err := c.Status(t.Context(), "tx1")
	require.NoError(t, err)
	require.Equal(t, fdriver.Unknown, vc)
	require.Empty(t, msg)
}

func TestStatusUnknownWithStoredEnvelopeBecomesBusy(t *testing.T) {
	t.Parallel()

	rws := &testRWSet{}
	c := &Committer{
		Vault: &testVault{
			statusFn: func(context.Context, cdriver.TxID) (fdriver.ValidationCode, string, error) {
				return fdriver.Unknown, "", nil
			},
		},
		EnvelopeService: &testEnvelopeService{
			existsFn: func(context.Context, string) bool { return true },
		},
		RWSetLoaderService: &testRWSetLoader{
			getFromEnvelopeFn: func(context.Context, cdriver.TxID) (fdriver.RWSet, fdriver.ProcessTransaction, error) {
				return rws, nil, nil
			},
		},
	}

	vc, _, err := c.Status(t.Context(), "tx1")
	require.NoError(t, err)
	require.Equal(t, fdriver.Busy, vc)
	require.Equal(t, 1, rws.doneCount)
}

func TestDiscardTxUnknownAndNoEnvelopeSetsDiscarded(t *testing.T) {
	t.Parallel()

	calledSetDiscarded := false
	calledDiscard := false
	c := &Committer{
		logger: logger,
		Vault: &testVault{
			statusFn: func(context.Context, cdriver.TxID) (fdriver.ValidationCode, string, error) {
				return fdriver.Unknown, "", nil
			},
			setDiscardedFn: func(context.Context, cdriver.TxID, string) error {
				calledSetDiscarded = true
				return nil
			},
			discardTxFn: func(context.Context, cdriver.TxID, string) error {
				calledDiscard = true
				return nil
			},
		},
		EnvelopeService: &testEnvelopeService{
			existsFn: func(context.Context, string) bool { return false },
		},
	}

	require.NoError(t, c.DiscardTx(t.Context(), "tx1", "invalid"))
	require.True(t, calledSetDiscarded)
	require.False(t, calledDiscard)
}

func TestFilterUnknownEnvelopeSelectsByNamespace(t *testing.T) {
	t.Parallel()

	rws := &testRWSet{
		namespaces: []cdriver.Namespace{"ns1"},
	}
	c := &Committer{
		logger:            logger,
		ProcessNamespaces: []string{"ns1"},
		RWSetLoaderService: &testRWSetLoader{
			inspectFromEnvFn: func(context.Context, cdriver.TxID, []byte) (fdriver.RWSet, fdriver.ProcessTransaction, error) {
				return rws, nil, nil
			},
		},
		TransactionFilters: commoncommitter.NewAggregatedTransactionFilter(),
	}

	ok, err := c.filterUnknownEnvelope(t.Context(), "tx1", []byte("env"))
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, 1, rws.doneCount)
}

func TestFilterUnknownEnvelopeSelectsByInitializedRead(t *testing.T) {
	t.Parallel()

	rws := &testRWSet{
		namespaces: []cdriver.Namespace{"nsX"},
		readKeys: map[cdriver.Namespace][]string{
			"nsX": {"asset_initialized_flag"},
		},
	}
	c := &Committer{
		logger:            logger,
		ProcessNamespaces: []string{"other"},
		RWSetLoaderService: &testRWSetLoader{
			inspectFromEnvFn: func(context.Context, cdriver.TxID, []byte) (fdriver.RWSet, fdriver.ProcessTransaction, error) {
				return rws, nil, nil
			},
		},
		TransactionFilters: commoncommitter.NewAggregatedTransactionFilter(),
	}

	ok, err := c.filterUnknownEnvelope(t.Context(), "tx2", []byte("env"))
	require.NoError(t, err)
	require.True(t, ok)
}

func TestFilterUnknownEnvelopeFallsBackToBusyStatus(t *testing.T) {
	t.Parallel()

	rws := &testRWSet{
		namespaces: []cdriver.Namespace{"nsX"},
		readKeys:   map[cdriver.Namespace][]string{"nsX": {}},
	}
	c := &Committer{
		logger: logger,
		Vault: &testVault{
			statusFn: func(context.Context, cdriver.TxID) (fdriver.ValidationCode, string, error) {
				return fdriver.Busy, "", nil
			},
		},
		RWSetLoaderService: &testRWSetLoader{
			inspectFromEnvFn: func(context.Context, cdriver.TxID, []byte) (fdriver.RWSet, fdriver.ProcessTransaction, error) {
				return rws, nil, nil
			},
		},
		TransactionFilters: commoncommitter.NewAggregatedTransactionFilter(),
	}

	ok, err := c.filterUnknownEnvelope(t.Context(), "tx3", []byte("env"))
	require.NoError(t, err)
	require.True(t, ok)
}

func TestExtractStoredEnvelopeToVaultFallsBackToETx(t *testing.T) {
	t.Parallel()

	doneFromETx := &testRWSet{}
	c := &Committer{
		RWSetLoaderService: &testRWSetLoader{
			getFromEnvelopeFn: func(context.Context, cdriver.TxID) (fdriver.RWSet, fdriver.ProcessTransaction, error) {
				return nil, nil, stderrors.New("from-envelope-failed")
			},
			getFromETxFn: func(context.Context, cdriver.TxID) (fdriver.RWSet, fdriver.ProcessTransaction, error) {
				return doneFromETx, nil, nil
			},
		},
	}

	require.NoError(t, c.extractStoredEnvelopeToVault(t.Context(), "tx4"))
	require.Equal(t, 1, doneFromETx.doneCount)
}

func TestPostProcessTx(t *testing.T) {
	t.Parallel()

	c := &Committer{
		ChannelConfig: &testChannelConfig{id: "chan1"},
		ProcessorManager: &testProcessorManager{
			processByIDFn: func(context.Context, string, cdriver.TxID) error {
				return nil
			},
		},
	}
	require.NoError(t, c.postProcessTx(t.Context(), "tx5"))
}

func TestFetchEnvelope(t *testing.T) {
	t.Parallel()

	t.Run("ledger error", func(t *testing.T) {
		t.Parallel()
		c := &Committer{
			Ledger: &testLedger{
				getByIDFn: func(string) (fdriver.ProcessedTransaction, error) {
					return nil, stderrors.New("ledger-down")
				},
			},
		}
		_, err := c.fetchEnvelope("tx6")
		require.ErrorContains(t, err, "failed fetching tx")
	})

	t.Run("invalid processed tx", func(t *testing.T) {
		t.Parallel()
		c := &Committer{
			Ledger: &testLedger{
				getByIDFn: func(string) (fdriver.ProcessedTransaction, error) {
					return &testProcessedTransaction{valid: false, validationCode: int32(peer.TxValidationCode_INVALID_OTHER_REASON)}, nil
				},
			},
		}
		_, err := c.fetchEnvelope("tx7")
		require.ErrorContains(t, err, "should have been valid")
	})

	t.Run("valid tx returns envelope", func(t *testing.T) {
		t.Parallel()
		expected := []byte("env-raw")
		c := &Committer{
			Ledger: &testLedger{
				getByIDFn: func(string) (fdriver.ProcessedTransaction, error) {
					return &testProcessedTransaction{valid: true, envelope: expected}, nil
				},
			},
		}
		got, err := c.fetchEnvelope("tx8")
		require.NoError(t, err)
		require.Equal(t, expected, got)
	})
}

func TestNotifyTxStatusPublishesTwoEvents(t *testing.T) {
	t.Parallel()

	publisher := &testPublisher{}
	c := &Committer{
		ConfigService:   &testConfigService{networkName: "net1"},
		ChannelConfig:   &testChannelConfig{id: "ch1"},
		EventsPublisher: publisher,
	}

	c.notifyTxStatus("tx9", fdriver.Valid, "ok")
	require.Len(t, publisher.events, 2)

	first, ok := publisher.events[0].(*fdriver.TransactionStatusChanged)
	require.True(t, ok)
	require.Equal(t, "tx9", first.TxID)
	require.Equal(t, fdriver.Valid, first.VC)
	require.Equal(t, "ok", first.ValidationMessage)

	second, ok := publisher.events[1].(*fdriver.TransactionStatusChanged)
	require.True(t, ok)
	require.Equal(t, "tx9", second.TxID)
	require.Equal(t, fdriver.Valid, second.VC)
	require.Equal(t, "ok", second.ValidationMessage)
	require.NotEqual(t, first.ThisTopic, second.ThisTopic)
}

func TestAddDeleteListenerAndNotifyFinality(t *testing.T) {
	t.Parallel()

	c := &Committer{
		logger:    logger,
		listeners: map[string][]chan FinalityEvent{},
	}
	ch := make(chan FinalityEvent, 1)

	c.addListener("tx10", ch)
	c.notifyFinality(FinalityEvent{Ctx: t.Context(), TxID: "tx10"})

	select {
	case event := <-ch:
		require.Equal(t, "tx10", event.TxID)
	default:
		t.Fatalf("expected finality event")
	}

	c.deleteListener("tx10", ch)
	require.Len(t, c.listeners["tx10"], 0)
}

func TestIsFinalForKnownStatuses(t *testing.T) {
	t.Parallel()

	t.Run("valid", func(t *testing.T) {
		t.Parallel()
		c := &Committer{
			logger:        logger,
			ChannelConfig: &testChannelConfig{finalityRetries: 1},
			Vault: &testVault{
				statusFn: func(context.Context, cdriver.TxID) (fdriver.ValidationCode, string, error) {
					return fdriver.Valid, "", nil
				},
			},
		}
		require.NoError(t, c.IsFinal(t.Context(), "tx11"))
	})

	t.Run("invalid", func(t *testing.T) {
		t.Parallel()
		c := &Committer{
			logger:        logger,
			ChannelConfig: &testChannelConfig{finalityRetries: 1},
			Vault: &testVault{
				statusFn: func(context.Context, cdriver.TxID) (fdriver.ValidationCode, string, error) {
					return fdriver.Invalid, "", nil
				},
			},
		}
		err := c.IsFinal(t.Context(), "tx12")
		require.ErrorContains(t, err, "is not valid")
	})
}

func TestListenToReturnsTimeoutWhenContextIsCanceled(t *testing.T) {
	t.Parallel()

	c := &Committer{
		logger:          logger,
		Vault:           &testVault{},
		EnvelopeService: &testEnvelopeService{},
		tracer:          noop.NewTracerProvider().Tracer("test"),
		listeners:       map[string][]chan FinalityEvent{},
		pollingTimeout:  5 * time.Millisecond,
	}

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	err := c.listenTo(ctx, "tx13", 20*time.Millisecond)
	require.ErrorContains(t, err, "failed to listen to transaction")
}

func TestCommitTXBusyPath(t *testing.T) {
	t.Parallel()

	committed := false
	c := &Committer{
		logger:        logger,
		ChannelConfig: &testChannelConfig{id: "testchannel"},
		Vault: &testVault{
			statusFn: func(context.Context, cdriver.TxID) (fdriver.ValidationCode, string, error) {
				return fdriver.Busy, "", nil
			},
			commitTxFn: func(context.Context, cdriver.TxID, cdriver.BlockNum, cdriver.TxNum) error {
				committed = true
				return nil
			},
		},
		ProcessorManager: &testProcessorManager{
			processByIDFn: func(context.Context, string, cdriver.TxID) error {
				return nil
			},
		},
	}

	require.NoError(t, c.CommitTX(t.Context(), "tx14", 7, 2, nil))
	require.True(t, committed)
}

func TestNewInitializesHandlersAndQueues(t *testing.T) {
	t.Parallel()

	c := New(
		&testConfigService{networkName: "net1"},
		&testChannelConfig{id: "ch1"},
		&testVault{},
		&testEnvelopeService{},
		&testLedger{},
		&testRWSetLoader{},
		&testProcessorManager{},
		&testPublisher{},
		nil,
		nil,
		nil,
		nil,
		NewSerialDependencyResolver(),
		false,
		nil,
		noop.NewTracerProvider(),
		&testMetricsProvider{},
	)
	require.NotNil(t, c)
	require.NotNil(t, c.Handlers[common.HeaderType_CONFIG])
	require.NotNil(t, c.Handlers[common.HeaderType_ENDORSER_TRANSACTION])
	require.NotNil(t, c.events)
	require.Equal(t, 2000, cap(c.events))
	require.Equal(t, 1*time.Second, c.pollingTimeout)
}

func TestAddAndRemoveFinalityListener(t *testing.T) {
	t.Parallel()

	lm := &testListenerManager{}
	fm := commoncommitter.NewFinalityManager[fdriver.ValidationCode](lm, logger, nil, noop.NewTracerProvider(), 1, fdriver.Valid, fdriver.Invalid)
	c := &Committer{FinalityManager: fm}

	listener := &testFinalityListener{}
	require.NoError(t, c.AddFinalityListener("tx20", listener))
	require.Equal(t, cdriver.TxID("tx20"), lm.addedTx)

	require.NoError(t, c.RemoveFinalityListener("tx20", listener))
	require.Equal(t, cdriver.TxID("tx20"), lm.removedTx)
}

func TestRunEventNotifiersProcessesQueue(t *testing.T) {
	t.Parallel()

	publisher := &testPublisher{publishedC: make(chan events.Event, 2)}
	lm := &testListenerManager{}
	c := &Committer{
		logger:          logger,
		ConfigService:   &testConfigService{networkName: "net-run"},
		ChannelConfig:   &testChannelConfig{id: "ch-run"},
		EventsPublisher: publisher,
		metrics:         NewMetrics(noop.NewTracerProvider(), &testMetricsProvider{}),
		FinalityManager: commoncommitter.NewFinalityManager[fdriver.ValidationCode](lm, logger, nil, noop.NewTracerProvider(), 1, fdriver.Valid, fdriver.Invalid),
		listeners:       map[string][]chan FinalityEvent{},
		events:          make(chan FinalityEvent, 1),
	}

	done := make(chan FinalityEvent, 1)
	c.addListener("tx21", done)

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	go c.runEventNotifiers(ctx)

	c.events <- FinalityEvent{
		Ctx:               t.Context(),
		TxID:              "tx21",
		ValidationCode:    fdriver.Valid,
		ValidationMessage: "ok",
	}

	select {
	case event := <-done:
		require.Equal(t, "tx21", event.TxID)
	case <-time.After(2 * time.Second):
		t.Fatalf("event not received")
	}

	for i := range 2 {
		select {
		case <-publisher.publishedC:
		case <-time.After(2 * time.Second):
			t.Fatalf("publisher did not receive event %d", i+1)
		}
	}
}

func TestCommitUnknownStoredEnvelopePath(t *testing.T) {
	t.Parallel()

	rws := &testRWSet{}
	committed := false
	c := &Committer{
		logger:        logger,
		ChannelConfig: &testChannelConfig{id: "ch-unknown"},
		Vault: &testVault{
			commitTxFn: func(context.Context, cdriver.TxID, cdriver.BlockNum, cdriver.TxNum) error {
				committed = true
				return nil
			},
		},
		EnvelopeService: &testEnvelopeService{
			existsFn: func(context.Context, string) bool { return true },
		},
		RWSetLoaderService: &testRWSetLoader{
			getFromEnvelopeFn: func(context.Context, cdriver.TxID) (fdriver.RWSet, fdriver.ProcessTransaction, error) {
				return rws, nil, nil
			},
		},
		ProcessorManager: &testProcessorManager{
			processByIDFn: func(context.Context, string, cdriver.TxID) error { return nil },
		},
	}

	err := c.commitUnknown(t.Context(), "tx22", 3, 1, nil)
	require.NoError(t, err)
	require.Equal(t, 1, rws.doneCount)
	require.True(t, committed)
}

func TestCommitUnknownAdditionalPaths(t *testing.T) {
	t.Parallel()

	t.Run("fetch envelope failure", func(t *testing.T) {
		t.Parallel()
		c := &Committer{
			logger: logger,
			EnvelopeService: &testEnvelopeService{
				existsFn: func(context.Context, string) bool { return false },
			},
			Ledger: &testLedger{
				getByIDFn: func(string) (fdriver.ProcessedTransaction, error) {
					return nil, stderrors.New("ledger-failed")
				},
			},
		}
		err := c.commitUnknown(t.Context(), "tx24", 1, 1, nil)
		require.ErrorContains(t, err, "failed getting rwset for tx [tx24]")
	})

	t.Run("filtered out unknown envelope", func(t *testing.T) {
		t.Parallel()
		stored := false
		c := &Committer{
			logger: logger,
			Vault: &testVault{
				statusFn: func(context.Context, cdriver.TxID) (fdriver.ValidationCode, string, error) {
					return fdriver.Unknown, "", nil
				},
			},
			EnvelopeService: &testEnvelopeService{
				existsFn: func(context.Context, string) bool { return false },
				storeEnvelope: func(context.Context, string, any) error {
					stored = true
					return nil
				},
			},
			RWSetLoaderService: &testRWSetLoader{
				inspectFromEnvFn: func(context.Context, cdriver.TxID, []byte) (fdriver.RWSet, fdriver.ProcessTransaction, error) {
					return &testRWSet{
						namespaces: []cdriver.Namespace{"ns-other"},
						readKeys:   map[cdriver.Namespace][]string{"ns-other": {}},
					}, nil, nil
				},
			},
			TransactionFilters: commoncommitter.NewAggregatedTransactionFilter(),
		}
		err := c.commitUnknown(t.Context(), "tx25", 1, 1, &common.Envelope{})
		require.NoError(t, err)
		require.False(t, stored)
	})

	t.Run("selected unknown envelope is stored and committed", func(t *testing.T) {
		t.Parallel()
		stored := false
		committed := false
		done := &testRWSet{}
		c := &Committer{
			logger:            logger,
			ChannelConfig:     &testChannelConfig{id: "ch-unknown2"},
			ProcessNamespaces: []string{"ns-include"},
			Vault: &testVault{
				commitTxFn: func(context.Context, cdriver.TxID, cdriver.BlockNum, cdriver.TxNum) error {
					committed = true
					return nil
				},
				rwsExistsFn: func(context.Context, cdriver.TxID) bool {
					return false
				},
			},
			EnvelopeService: &testEnvelopeService{
				existsFn: func(context.Context, string) bool { return false },
				storeEnvelope: func(context.Context, string, any) error {
					stored = true
					return nil
				},
			},
			RWSetLoaderService: &testRWSetLoader{
				inspectFromEnvFn: func(context.Context, cdriver.TxID, []byte) (fdriver.RWSet, fdriver.ProcessTransaction, error) {
					return &testRWSet{namespaces: []cdriver.Namespace{"ns-include"}}, nil, nil
				},
				getFromEnvelopeFn: func(context.Context, cdriver.TxID) (fdriver.RWSet, fdriver.ProcessTransaction, error) {
					return done, nil, nil
				},
			},
			TransactionManager: &testTransactionManager{
				newProcessedFromPayloadFn: func([]byte) (fdriver.ProcessedTransaction, int32, error) {
					return &testProcessedTransaction{results: []byte("rws")}, int32(common.HeaderType_ENDORSER_TRANSACTION), nil
				},
			},
			ProcessorManager: &testProcessorManager{
				processByIDFn: func(context.Context, string, cdriver.TxID) error { return nil },
			},
		}

		err := c.commitUnknown(t.Context(), "tx26", 2, 1, &common.Envelope{Payload: []byte("payload")})
		require.NoError(t, err)
		require.True(t, stored)
		require.True(t, committed)
		require.GreaterOrEqual(t, done.doneCount, 1)
	})
}

func TestStart(t *testing.T) {
	t.Parallel()

	c := &Committer{
		FinalityManager: commoncommitter.NewFinalityManager[fdriver.ValidationCode](&testListenerManager{}, logger, nil, noop.NewTracerProvider(), 1, fdriver.Valid, fdriver.Invalid),
		metrics:         NewMetrics(noop.NewTracerProvider(), &testMetricsProvider{}),
		listeners:       map[string][]chan FinalityEvent{},
		events:          make(chan FinalityEvent, 1),
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	require.NoError(t, c.Start(ctx))
}

func TestIsFinalUnknownRemotePaths(t *testing.T) {
	t.Parallel()

	t.Run("remote finality confirms", func(t *testing.T) {
		t.Parallel()
		c := &Committer{
			logger:        logger,
			ConfigService: &testConfigService{peerAddress: "peerA"},
			ChannelConfig: &testChannelConfig{finalityRetries: 1, finalityUnknownTimeout: 0},
			Vault: &testVault{
				statusFn: func(context.Context, cdriver.TxID) (fdriver.ValidationCode, string, error) {
					return fdriver.Unknown, "", nil
				},
			},
			EnvelopeService: &testEnvelopeService{
				existsFn: func(context.Context, string) bool { return false },
			},
			FabricFinality: &testFabricFinality{err: nil},
		}
		require.NoError(t, c.IsFinal(t.Context(), "tx27"))
	})

	t.Run("remote finality fails and tx is still unknown", func(t *testing.T) {
		t.Parallel()
		calls := 0
		c := &Committer{
			logger:        logger,
			ConfigService: &testConfigService{peerAddress: "peerB"},
			ChannelConfig: &testChannelConfig{finalityRetries: 1, finalityUnknownTimeout: 0},
			Vault: &testVault{
				statusFn: func(context.Context, cdriver.TxID) (fdriver.ValidationCode, string, error) {
					calls++
					return fdriver.Unknown, "", nil
				},
			},
			EnvelopeService: &testEnvelopeService{
				existsFn: func(context.Context, string) bool { return false },
			},
			FabricFinality: &testFabricFinality{err: stderrors.New("not-final")},
		}

		err := c.IsFinal(t.Context(), "tx28")
		require.ErrorContains(t, err, "not-final")
		require.GreaterOrEqual(t, calls, 2)
	})
}

func TestCommitReturnsUnmarshalError(t *testing.T) {
	t.Parallel()

	c := &Committer{
		logger:             logger,
		ChannelConfig:      &testChannelConfig{id: "ch-commit"},
		DependencyResolver: NewSerialDependencyResolver(),
		metrics:            NewMetrics(noop.NewTracerProvider(), &testMetricsProvider{}),
	}

	block := &common.Block{
		Header:   &common.BlockHeader{Number: 7},
		Data:     &common.BlockData{Data: [][]byte{[]byte("bad-tx")}},
		Metadata: &common.BlockMetadata{},
	}
	err := c.Commit(t.Context(), block)
	require.ErrorContains(t, err, "unmarshal tx failed")
}

func TestCommitTxs(t *testing.T) {
	t.Parallel()

	t.Run("successful handler enqueues event", func(t *testing.T) {
		t.Parallel()
		c := &Committer{
			logger:        logger,
			ChannelConfig: &testChannelConfig{id: "ch-ct", commitParallelism: 1},
			metrics:       NewMetrics(noop.NewTracerProvider(), &testMetricsProvider{}),
			events:        make(chan FinalityEvent, 1),
			Handlers: map[common.HeaderType]TransactionHandler{
				common.HeaderType_ENDORSER_TRANSACTION: func(ctx context.Context, _ *common.BlockMetadata, tx CommitTx) (*FinalityEvent, error) {
					return &FinalityEvent{Ctx: ctx, TxID: tx.TxID, ValidationCode: fdriver.Valid}, nil
				},
			},
		}

		err := c.commitTxs(
			t.Context(),
			ParallelExecutable[SerialExecutable[CommitTx]]{
				{{TxID: "tx31", BlkNum: 1, TxNum: 0, Type: common.HeaderType_ENDORSER_TRANSACTION}},
			},
			&common.BlockMetadata{},
		)
		require.NoError(t, err)
		require.Len(t, c.events, 1)
	})

	t.Run("handler not found is ignored", func(t *testing.T) {
		t.Parallel()
		c := &Committer{
			logger:        logger,
			ChannelConfig: &testChannelConfig{id: "ch-ct2", commitParallelism: 1},
			metrics:       NewMetrics(noop.NewTracerProvider(), &testMetricsProvider{}),
			events:        make(chan FinalityEvent, 1),
			Handlers:      map[common.HeaderType]TransactionHandler{},
		}

		err := c.commitTxs(
			t.Context(),
			ParallelExecutable[SerialExecutable[CommitTx]]{
				{{TxID: "tx32", BlkNum: 1, TxNum: 0, Type: common.HeaderType_MESSAGE}},
			},
			&common.BlockMetadata{},
		)
		require.NoError(t, err)
		require.Len(t, c.events, 0)
	})

	t.Run("handler error is returned", func(t *testing.T) {
		t.Parallel()
		c := &Committer{
			logger:        logger,
			ChannelConfig: &testChannelConfig{id: "ch-ct3", commitParallelism: 1},
			metrics:       NewMetrics(noop.NewTracerProvider(), &testMetricsProvider{}),
			events:        make(chan FinalityEvent, 1),
			Handlers: map[common.HeaderType]TransactionHandler{
				common.HeaderType_ENDORSER_TRANSACTION: func(context.Context, *common.BlockMetadata, CommitTx) (*FinalityEvent, error) {
					return nil, stderrors.New("handler-failed")
				},
			},
		}

		err := c.commitTxs(
			t.Context(),
			ParallelExecutable[SerialExecutable[CommitTx]]{
				{{TxID: "tx33", BlkNum: 1, TxNum: 0, Type: common.HeaderType_ENDORSER_TRANSACTION}},
			},
			&common.BlockMetadata{},
		)
		require.ErrorContains(t, err, "failed calling handler for tx [tx33]")
	})
}

func TestNotifyChaincodeListenersPublishes(t *testing.T) {
	t.Parallel()

	publisher := &testPublisher{}
	c := &Committer{
		logger:          logger,
		EventsPublisher: publisher,
	}

	event := &ChaincodeEvent{ChaincodeID: "cc1"}
	c.notifyChaincodeListeners(event)
	require.Len(t, publisher.events, 1)
	require.Same(t, event, publisher.events[0])
}

func TestDiscardTxAdditionalBranches(t *testing.T) {
	t.Parallel()

	t.Run("status lookup error", func(t *testing.T) {
		t.Parallel()
		c := &Committer{
			logger: logger,
			Vault: &testVault{
				statusFn: func(context.Context, cdriver.TxID) (fdriver.ValidationCode, string, error) {
					return fdriver.Unknown, "", stderrors.New("status-failed")
				},
			},
		}
		err := c.DiscardTx(t.Context(), "tx-discard-err", "bad")
		require.ErrorContains(t, err, "failed getting tx's status in state db")
	})

	t.Run("unknown with stored envelope extract failure", func(t *testing.T) {
		t.Parallel()
		c := &Committer{
			logger: logger,
			Vault: &testVault{
				statusFn: func(context.Context, cdriver.TxID) (fdriver.ValidationCode, string, error) {
					return fdriver.Unknown, "", nil
				},
			},
			EnvelopeService: &testEnvelopeService{
				existsFn: func(context.Context, string) bool { return true },
			},
			RWSetLoaderService: &testRWSetLoader{
				getFromEnvelopeFn: func(context.Context, cdriver.TxID) (fdriver.RWSet, fdriver.ProcessTransaction, error) {
					return nil, nil, stderrors.New("env-failed")
				},
				getFromETxFn: func(context.Context, cdriver.TxID) (fdriver.RWSet, fdriver.ProcessTransaction, error) {
					return nil, nil, stderrors.New("etx-failed")
				},
			},
		}
		err := c.DiscardTx(t.Context(), "tx-discard-extract", "bad")
		require.ErrorContains(t, err, "failed to extract stored enveloper")
	})

	t.Run("known status discards tx in vault", func(t *testing.T) {
		t.Parallel()
		discarded := false
		c := &Committer{
			logger: logger,
			Vault: &testVault{
				statusFn: func(context.Context, cdriver.TxID) (fdriver.ValidationCode, string, error) {
					return fdriver.Busy, "", nil
				},
				discardTxFn: func(context.Context, cdriver.TxID, string) error {
					discarded = true
					return nil
				},
			},
		}
		require.NoError(t, c.DiscardTx(t.Context(), "tx-discard", "invalid"))
		require.True(t, discarded)
	})
}

func TestCommitTXAdditionalBranches(t *testing.T) {
	t.Parallel()

	t.Run("status lookup error", func(t *testing.T) {
		t.Parallel()
		c := &Committer{
			logger: logger,
			Vault: &testVault{
				statusFn: func(context.Context, cdriver.TxID) (fdriver.ValidationCode, string, error) {
					return fdriver.Unknown, "", stderrors.New("status-failed")
				},
			},
		}
		err := c.CommitTX(t.Context(), "tx-commit-status", 1, 0, nil)
		require.ErrorContains(t, err, "failed getting tx's status in state db")
	})

	t.Run("already valid", func(t *testing.T) {
		t.Parallel()
		c := &Committer{
			logger: logger,
			Vault: &testVault{
				statusFn: func(context.Context, cdriver.TxID) (fdriver.ValidationCode, string, error) {
					return fdriver.Valid, "", nil
				},
			},
		}
		err := c.CommitTX(t.Context(), "tx-commit-valid", 1, 0, nil)
		require.ErrorContains(t, err, "already valid")
	})

	t.Run("already invalid", func(t *testing.T) {
		t.Parallel()
		c := &Committer{
			logger: logger,
			Vault: &testVault{
				statusFn: func(context.Context, cdriver.TxID) (fdriver.ValidationCode, string, error) {
					return fdriver.Invalid, "", nil
				},
			},
		}
		err := c.CommitTX(t.Context(), "tx-commit-invalid", 1, 0, nil)
		require.ErrorContains(t, err, "is invalid")
	})

	t.Run("unknown path delegates to commitUnknown", func(t *testing.T) {
		t.Parallel()
		c := &Committer{
			logger: logger,
			Vault: &testVault{
				statusFn: func(context.Context, cdriver.TxID) (fdriver.ValidationCode, string, error) {
					return fdriver.Unknown, "", nil
				},
			},
			EnvelopeService: &testEnvelopeService{
				existsFn: func(context.Context, string) bool { return false },
			},
			Ledger: &testLedger{
				getByIDFn: func(string) (fdriver.ProcessedTransaction, error) {
					return nil, stderrors.New("ledger-down")
				},
			},
		}
		err := c.CommitTX(t.Context(), "tx-commit-unknown", 1, 0, nil)
		require.ErrorContains(t, err, "failed getting rwset for tx [tx-commit-unknown]")
	})

	t.Run("invalid status code", func(t *testing.T) {
		t.Parallel()
		c := &Committer{
			logger: logger,
			Vault: &testVault{
				statusFn: func(context.Context, cdriver.TxID) (fdriver.ValidationCode, string, error) {
					return fdriver.ValidationCode(99), "", nil
				},
			},
		}
		err := c.CommitTX(t.Context(), "tx-commit-code", 1, 0, nil)
		require.ErrorContains(t, err, "invalid status code [99]")
	})
}

func TestCommitAdditionalBranches(t *testing.T) {
	t.Parallel()

	t.Run("processed transaction parse failure", func(t *testing.T) {
		t.Parallel()
		c := &Committer{
			logger: logger,
			TransactionManager: &testTransactionManager{
				newProcessedFromPayloadFn: func([]byte) (fdriver.ProcessedTransaction, int32, error) {
					return nil, -1, stderrors.New("parse-failed")
				},
			},
		}
		err := c.commit(t.Context(), "tx-commit-parse", 1, 0, &common.Envelope{Payload: []byte("payload")})
		require.ErrorContains(t, err, "parse-failed")
	})

	t.Run("rwset match failure discards tx", func(t *testing.T) {
		t.Parallel()
		c := &Committer{
			logger: logger,
			Vault: &testVault{
				rwsExistsFn: func(context.Context, cdriver.TxID) bool { return false },
				matchFn: func(context.Context, cdriver.TxID, []byte) error {
					return stderrors.New("mismatch")
				},
			},
			EnvelopeService: &testEnvelopeService{
				existsFn: func(context.Context, string) bool { return true },
			},
			RWSetLoaderService: &testRWSetLoader{
				getFromEnvelopeFn: func(context.Context, cdriver.TxID) (fdriver.RWSet, fdriver.ProcessTransaction, error) {
					return &testRWSet{}, nil, nil
				},
			},
			TransactionManager: &testTransactionManager{
				newProcessedFromPayloadFn: func([]byte) (fdriver.ProcessedTransaction, int32, error) {
					return &testProcessedTransaction{results: []byte("rws")}, int32(common.HeaderType_ENDORSER_TRANSACTION), nil
				},
			},
		}
		err := c.commit(t.Context(), "tx-commit-match", 1, 0, &common.Envelope{Payload: []byte("payload")})
		require.ErrorContains(t, err, "rwsets do not match")
	})

	t.Run("store envelope failure", func(t *testing.T) {
		t.Parallel()
		c := &Committer{
			logger: logger,
			Vault: &testVault{
				rwsExistsFn: func(context.Context, cdriver.TxID) bool { return true },
			},
			EnvelopeService: &testEnvelopeService{
				storeEnvelope: func(context.Context, string, any) error {
					return stderrors.New("store-failed")
				},
			},
			TransactionManager: &testTransactionManager{
				newProcessedFromPayloadFn: func([]byte) (fdriver.ProcessedTransaction, int32, error) {
					return &testProcessedTransaction{results: []byte("rws")}, int32(common.HeaderType_ENDORSER_TRANSACTION), nil
				},
			},
		}
		err := c.commit(t.Context(), "tx-commit-store", 1, 0, &common.Envelope{Payload: []byte("payload")})
		require.ErrorContains(t, err, "failed to store unknown envelope")
	})

	t.Run("rwset from envelope failure", func(t *testing.T) {
		t.Parallel()
		c := &Committer{
			logger: logger,
			Vault: &testVault{
				rwsExistsFn: func(context.Context, cdriver.TxID) bool { return true },
			},
			EnvelopeService: &testEnvelopeService{
				storeEnvelope: func(context.Context, string, any) error { return nil },
			},
			RWSetLoaderService: &testRWSetLoader{
				getFromEnvelopeFn: func(context.Context, cdriver.TxID) (fdriver.RWSet, fdriver.ProcessTransaction, error) {
					return nil, nil, stderrors.New("rws-failed")
				},
			},
			TransactionManager: &testTransactionManager{
				newProcessedFromPayloadFn: func([]byte) (fdriver.ProcessedTransaction, int32, error) {
					return &testProcessedTransaction{results: []byte("rws")}, int32(common.HeaderType_ENDORSER_TRANSACTION), nil
				},
			},
		}
		err := c.commit(t.Context(), "tx-commit-rws", 1, 0, &common.Envelope{Payload: []byte("payload")})
		require.ErrorContains(t, err, "failed to get rws from envelope")
	})

	t.Run("post process failure", func(t *testing.T) {
		t.Parallel()
		c := &Committer{
			logger:        logger,
			ChannelConfig: &testChannelConfig{id: "ch-commit-post"},
			ProcessorManager: &testProcessorManager{
				processByIDFn: func(context.Context, string, cdriver.TxID) error {
					return stderrors.New("process-failed")
				},
			},
			Vault: &testVault{},
		}
		err := c.commit(t.Context(), "tx-commit-post", 1, 0, nil)
		require.ErrorContains(t, err, "process-failed")
	})

	t.Run("vault commit failure", func(t *testing.T) {
		t.Parallel()
		c := &Committer{
			logger:        logger,
			ChannelConfig: &testChannelConfig{id: "ch-commit-vault"},
			ProcessorManager: &testProcessorManager{
				processByIDFn: func(context.Context, string, cdriver.TxID) error { return nil },
			},
			Vault: &testVault{
				commitTxFn: func(context.Context, cdriver.TxID, cdriver.BlockNum, cdriver.TxNum) error {
					return stderrors.New("vault-commit-failed")
				},
			},
		}
		err := c.commit(t.Context(), "tx-commit-vault", 1, 0, nil)
		require.ErrorContains(t, err, "vault-commit-failed")
	})
}

func TestListenToAdditionalBranches(t *testing.T) {
	t.Parallel()

	t.Run("timeout when status remains unavailable", func(t *testing.T) {
		t.Parallel()
		c := &Committer{
			logger:         logger,
			tracer:         noop.NewTracerProvider().Tracer("test"),
			listeners:      map[string][]chan FinalityEvent{},
			pollingTimeout: 10 * time.Millisecond,
			Vault: &testVault{
				statusFn: func(context.Context, cdriver.TxID) (fdriver.ValidationCode, string, error) {
					return fdriver.Unknown, "", nil
				},
			},
			EnvelopeService: &testEnvelopeService{},
		}

		err := c.listenTo(t.Context(), "tx-listen-timeout", 50*time.Millisecond)
		require.ErrorContains(t, err, "failed to listen to transaction")
	})

	t.Run("timeout check sees valid tx", func(t *testing.T) {
		t.Parallel()
		c := &Committer{
			logger:         logger,
			tracer:         noop.NewTracerProvider().Tracer("test"),
			listeners:      map[string][]chan FinalityEvent{},
			pollingTimeout: 5 * time.Millisecond,
			Vault: &testVault{
				statusFn: func(context.Context, cdriver.TxID) (fdriver.ValidationCode, string, error) {
					return fdriver.Valid, "", nil
				},
			},
			EnvelopeService: &testEnvelopeService{},
		}
		require.NoError(t, c.listenTo(t.Context(), "tx-listen-valid", 20*time.Millisecond))
	})

	t.Run("timeout check sees invalid tx", func(t *testing.T) {
		t.Parallel()
		c := &Committer{
			logger:         logger,
			tracer:         noop.NewTracerProvider().Tracer("test"),
			listeners:      map[string][]chan FinalityEvent{},
			pollingTimeout: 5 * time.Millisecond,
			Vault: &testVault{
				statusFn: func(context.Context, cdriver.TxID) (fdriver.ValidationCode, string, error) {
					return fdriver.Invalid, "", nil
				},
			},
			EnvelopeService: &testEnvelopeService{},
		}
		err := c.listenTo(t.Context(), "tx-listen-invalid", 20*time.Millisecond)
		require.ErrorContains(t, err, "is not valid")
	})
}

func TestIsFinalAdditionalBranches(t *testing.T) {
	t.Parallel()

	t.Run("status lookup error", func(t *testing.T) {
		t.Parallel()
		c := &Committer{
			logger:        logger,
			ChannelConfig: &testChannelConfig{finalityRetries: 1},
			Vault: &testVault{
				statusFn: func(context.Context, cdriver.TxID) (fdriver.ValidationCode, string, error) {
					return fdriver.Unknown, "", stderrors.New("status-down")
				},
			},
		}
		err := c.IsFinal(t.Context(), "tx-final-status")
		require.ErrorContains(t, err, "failed getting transaction status from vault")
	})

	t.Run("invalid status code", func(t *testing.T) {
		t.Parallel()
		c := &Committer{
			logger:        logger,
			ChannelConfig: &testChannelConfig{finalityRetries: 1},
			Vault: &testVault{
				statusFn: func(context.Context, cdriver.TxID) (fdriver.ValidationCode, string, error) {
					return fdriver.ValidationCode(99), "", nil
				},
			},
		}
		err := c.IsFinal(t.Context(), "tx-final-code")
		require.ErrorContains(t, err, "invalid status code")
	})

	t.Run("busy status falls back to listener", func(t *testing.T) {
		t.Parallel()
		c := &Committer{
			logger: logger,
			ChannelConfig: &testChannelConfig{
				finalityRetries:     1,
				waitForEventTimeout: 50 * time.Millisecond,
			},
			Vault: &testVault{
				statusFn: func(context.Context, cdriver.TxID) (fdriver.ValidationCode, string, error) {
					return fdriver.Busy, "", nil
				},
			},
			tracer:          noop.NewTracerProvider().Tracer("test"),
			listeners:       map[string][]chan FinalityEvent{},
			pollingTimeout:  10 * time.Millisecond,
			EnvelopeService: &testEnvelopeService{},
		}
		err := c.IsFinal(t.Context(), "tx-final-busy")
		require.ErrorContains(t, err, "failed to listen to transaction")
	})
}
