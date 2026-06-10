/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package committer

import (
	"context"
	stderrors "errors"
	"testing"

	"github.com/hyperledger/fabric-protos-go-apiv2/common"
	"github.com/hyperledger/fabric-protos-go-apiv2/peer"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	commoncommitter "github.com/hyperledger-labs/fabric-smart-client/platform/common/core/generic/committer"
	cdriver "github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/committer/fake"
	fdriver "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
)

func mustMarshalProto(t *testing.T, m proto.Message) []byte {
	t.Helper()
	raw, err := proto.Marshal(m)
	require.NoError(t, err)
	return raw
}

func buildEnvelopeWithChaincodeEvent(t *testing.T, event *peer.ChaincodeEvent) *common.Envelope {
	t.Helper()

	action := &peer.ChaincodeAction{Events: mustMarshalProto(t, event)}
	respPayload := &peer.ProposalResponsePayload{Extension: mustMarshalProto(t, action)}
	ccActionPayload := &peer.ChaincodeActionPayload{
		Action: &peer.ChaincodeEndorsedAction{
			ProposalResponsePayload: mustMarshalProto(t, respPayload),
		},
	}
	tx := &peer.Transaction{
		Actions: []*peer.TransactionAction{
			{Payload: mustMarshalProto(t, ccActionPayload)},
		},
	}
	payl := &common.Payload{Data: mustMarshalProto(t, tx)}
	return &common.Envelope{Payload: mustMarshalProto(t, payl)}
}

func TestMapFinalityEvent(t *testing.T) {
	t.Parallel()

	t.Run("missing tx filter metadata", func(t *testing.T) {
		t.Parallel()
		_, _, err := MapFinalityEvent(t.Context(), &common.BlockMetadata{Metadata: [][]byte{}}, 0, "tx1")
		require.ErrorContains(t, err, "block metadata lacks transaction filter")
	})

	t.Run("maps fabric validation code into event", func(t *testing.T) {
		t.Parallel()
		metadata := make([][]byte, int(common.BlockMetadataIndex_TRANSACTIONS_FILTER)+1)
		metadata[common.BlockMetadataIndex_TRANSACTIONS_FILTER] = []byte{uint8(peer.TxValidationCode_VALID)}

		code, event, err := MapFinalityEvent(t.Context(), &common.BlockMetadata{Metadata: metadata}, 0, "tx2")
		require.NoError(t, err)
		require.Equal(t, uint8(peer.TxValidationCode_VALID), code)
		require.Equal(t, "tx2", event.TxID)
		require.Equal(t, fdriver.Valid, event.ValidationCode)
		require.Equal(t, peer.TxValidationCode_name[int32(peer.TxValidationCode_VALID)], event.ValidationMessage)
	})
}

func TestMapValidationCodeAndConvertValidationCode(t *testing.T) {
	t.Parallel()

	validCode, validMsg := MapValidationCode(int32(peer.TxValidationCode_VALID))
	require.Equal(t, fdriver.Valid, validCode)
	require.Equal(t, peer.TxValidationCode_name[int32(peer.TxValidationCode_VALID)], validMsg)

	invalidCode, invalidMsg := MapValidationCode(int32(peer.TxValidationCode_INVALID_OTHER_REASON))
	require.Equal(t, fdriver.Invalid, invalidCode)
	require.Equal(t, peer.TxValidationCode_name[int32(peer.TxValidationCode_INVALID_OTHER_REASON)], invalidMsg)

	require.Equal(t, fdriver.Valid, convertValidationCode(int32(peer.TxValidationCode_VALID)))
	require.Equal(t, fdriver.Invalid, convertValidationCode(int32(peer.TxValidationCode_INVALID_ENDORSER_TRANSACTION)))
}

func TestGetChaincodeEventsReturnsReadError(t *testing.T) {
	t.Parallel()

	c := &Committer{logger: logger}
	err := c.GetChaincodeEvents(&common.Envelope{Payload: []byte("bad-payload")}, 1)
	require.ErrorContains(t, err, "error reading chaincode event")
}

func TestGetChaincodeEventsPublishesChaincodeEvent(t *testing.T) {
	t.Parallel()

	pub := &fake.Publisher{}
	c := &Committer{
		logger:          logger,
		EventsPublisher: pub,
	}
	env := buildEnvelopeWithChaincodeEvent(t, &peer.ChaincodeEvent{
		TxId:        "tx-chaincode",
		ChaincodeId: "mycc",
		EventName:   "created",
		Payload:     []byte("payload"),
	})
	err := c.GetChaincodeEvents(env, 9)
	require.NoError(t, err)
	require.Len(t, pub.Events, 1)

	evt, ok := pub.Events[0].(*ChaincodeEvent)
	require.True(t, ok)
	require.Equal(t, uint64(9), evt.BlockNumber)
	require.Equal(t, "tx-chaincode", evt.TransactionID)
	require.Equal(t, "mycc", evt.ChaincodeID)
	require.Equal(t, "created", evt.EventName)
	require.Equal(t, []byte("payload"), evt.Payload)
}

func TestCommitEndorserTransaction(t *testing.T) {
	t.Parallel()

	t.Run("already valid returns processed", func(t *testing.T) {
		t.Parallel()
		c := &Committer{
			logger: logger,
			Vault: &fake.Vault{
				StatusFn: func(context.Context, cdriver.TxID) (fdriver.ValidationCode, string, error) {
					return fdriver.Valid, "", nil
				},
			},
		}
		processed, err := c.CommitEndorserTransaction(t.Context(), "tx3", 10, 4, nil, &FinalityEvent{TxID: "tx3"})
		require.NoError(t, err)
		require.True(t, processed)
	})

	t.Run("already invalid returns processed", func(t *testing.T) {
		t.Parallel()
		c := &Committer{
			logger: logger,
			Vault: &fake.Vault{
				StatusFn: func(context.Context, cdriver.TxID) (fdriver.ValidationCode, string, error) {
					return fdriver.Invalid, "", nil
				},
			},
		}
		processed, err := c.CommitEndorserTransaction(t.Context(), "tx4", 10, 5, nil, &FinalityEvent{TxID: "tx4"})
		require.NoError(t, err)
		require.True(t, processed)
	})

	t.Run("status error propagates", func(t *testing.T) {
		t.Parallel()
		c := &Committer{
			logger: logger,
			Vault: &fake.Vault{
				StatusFn: func(context.Context, cdriver.TxID) (fdriver.ValidationCode, string, error) {
					return fdriver.Unknown, "", stderrors.New("status-failed")
				},
			},
		}
		_, err := c.CommitEndorserTransaction(t.Context(), "tx5", 10, 6, nil, &FinalityEvent{TxID: "tx5"})
		require.ErrorContains(t, err, "failed getting tx's status")
	})
}

func TestHandleEndorserTransaction(t *testing.T) {
	t.Parallel()

	t.Run("map finality event error", func(t *testing.T) {
		t.Parallel()
		c := &Committer{
			logger:        logger,
			ChannelConfig: &fake.ChannelConfig{IDValue: "ch-endorser"},
		}
		_, err := c.HandleEndorserTransaction(t.Context(), &common.BlockMetadata{Metadata: [][]byte{}}, CommitTx{TxID: "tx8"})
		require.ErrorContains(t, err, "block metadata lacks transaction filter")
	})

	t.Run("invalid transaction path", func(t *testing.T) {
		t.Parallel()
		metadata := make([][]byte, int(common.BlockMetadataIndex_TRANSACTIONS_FILTER)+1)
		metadata[common.BlockMetadataIndex_TRANSACTIONS_FILTER] = []byte{uint8(peer.TxValidationCode_INVALID_OTHER_REASON)}

		c := &Committer{
			logger:        logger,
			ChannelConfig: &fake.ChannelConfig{IDValue: "ch-endorser"},
			Vault: &fake.Vault{
				StatusFn: func(context.Context, cdriver.TxID) (fdriver.ValidationCode, string, error) {
					return fdriver.Valid, "", nil
				},
			},
		}

		event, err := c.HandleEndorserTransaction(t.Context(), &common.BlockMetadata{Metadata: metadata}, CommitTx{
			BlkNum: 1,
			TxNum:  0,
			TxID:   "tx9",
			Raw:    []byte("raw"),
		})
		require.NoError(t, err)
		require.NotNil(t, event)
		require.Equal(t, "tx9", event.TxID)
		require.Equal(t, fdriver.Invalid, event.ValidationCode)
	})
}

func TestDiscardEndorserTransactionKnownStatuses(t *testing.T) {
	t.Parallel()

	t.Run("valid status returns nil", func(t *testing.T) {
		t.Parallel()
		c := &Committer{
			logger: logger,
			Vault: &fake.Vault{
				StatusFn: func(context.Context, cdriver.TxID) (fdriver.ValidationCode, string, error) {
					return fdriver.Valid, "", nil
				},
			},
		}
		require.NoError(t, c.DiscardEndorserTransaction(t.Context(), "tx6", 11, []byte("raw"), &FinalityEvent{TxID: "tx6"}))
	})

	t.Run("invalid status returns nil", func(t *testing.T) {
		t.Parallel()
		c := &Committer{
			logger: logger,
			Vault: &fake.Vault{
				StatusFn: func(context.Context, cdriver.TxID) (fdriver.ValidationCode, string, error) {
					return fdriver.Invalid, "", nil
				},
			},
		}
		require.NoError(t, c.DiscardEndorserTransaction(t.Context(), "tx7", 12, []byte("raw"), &FinalityEvent{TxID: "tx7"}))
	})

	t.Run("unknown status stores unknown envelope and marks discarded", func(t *testing.T) {
		t.Parallel()
		stored := false
		setDiscarded := false
		done := &fake.RWSet{}
		c := &Committer{
			logger:            logger,
			ProcessNamespaces: []string{"tracked-ns"},
			Vault: &fake.Vault{
				StatusFn: func(context.Context, cdriver.TxID) (fdriver.ValidationCode, string, error) {
					return fdriver.Unknown, "", nil
				},
				SetDiscardedFn: func(context.Context, cdriver.TxID, string) error {
					setDiscarded = true
					return nil
				},
			},
			EnvelopeService: &fake.EnvelopeService{
				ExistsFn: func(context.Context, string) bool { return false },
				StoreEnvelopeFn: func(context.Context, string, any) error {
					stored = true
					return nil
				},
			},
			RWSetLoaderService: &fake.RWSetLoader{
				InspectFromEnvFn: func(context.Context, cdriver.TxID, []byte) (fdriver.RWSet, fdriver.ProcessTransaction, error) {
					return &fake.RWSet{NamespacesList: []cdriver.Namespace{"tracked-ns"}}, nil, nil
				},
				GetFromEnvelopeFn: func(context.Context, cdriver.TxID) (fdriver.RWSet, fdriver.ProcessTransaction, error) {
					return done, nil, nil
				},
			},
			TransactionFilters: commoncommitter.NewAggregatedTransactionFilter(),
		}
		event := &FinalityEvent{Ctx: t.Context(), TxID: "tx-unknown", ValidationCode: fdriver.Invalid, ValidationMessage: "bad"}
		require.NoError(t, c.DiscardEndorserTransaction(t.Context(), "tx-unknown", 13, []byte("raw"), event))
		require.True(t, stored)
		require.True(t, setDiscarded)
		require.GreaterOrEqual(t, done.DoneCount, 1)
	})

	t.Run("status lookup error propagates", func(t *testing.T) {
		t.Parallel()
		c := &Committer{
			logger: logger,
			Vault: &fake.Vault{
				StatusFn: func(context.Context, cdriver.TxID) (fdriver.ValidationCode, string, error) {
					return fdriver.Unknown, "", stderrors.New("status-failed")
				},
			},
		}
		err := c.DiscardEndorserTransaction(t.Context(), "tx-status-err", 1, []byte("raw"), &FinalityEvent{TxID: "tx-status-err"})
		require.ErrorContains(t, err, "failed getting tx's status")
	})

	t.Run("unknown status with inspect error returns failure", func(t *testing.T) {
		t.Parallel()
		c := &Committer{
			logger: logger,
			Vault: &fake.Vault{
				StatusFn: func(context.Context, cdriver.TxID) (fdriver.ValidationCode, string, error) {
					return fdriver.Unknown, "", nil
				},
			},
			EnvelopeService: &fake.EnvelopeService{
				ExistsFn: func(context.Context, string) bool { return false },
			},
			RWSetLoaderService: &fake.RWSetLoader{
				InspectFromEnvFn: func(context.Context, cdriver.TxID, []byte) (fdriver.RWSet, fdriver.ProcessTransaction, error) {
					return nil, nil, stderrors.New("inspect-failed")
				},
			},
			TransactionFilters: commoncommitter.NewAggregatedTransactionFilter(),
		}
		err := c.DiscardEndorserTransaction(t.Context(), "tx-inspect-err", 1, []byte("raw"), &FinalityEvent{
			Ctx:               t.Context(),
			TxID:              "tx-inspect-err",
			ValidationCode:    fdriver.Invalid,
			ValidationMessage: "bad",
		})
		require.ErrorContains(t, err, "failed to get rws from envelope")
	})
}

func TestHandleEndorserTransactionAdditionalBranches(t *testing.T) {
	t.Parallel()

	validMetadata := make([][]byte, int(common.BlockMetadataIndex_TRANSACTIONS_FILTER)+1)
	validMetadata[common.BlockMetadataIndex_TRANSACTIONS_FILTER] = []byte{uint8(peer.TxValidationCode_VALID)}

	t.Run("discardable commit error maps event to invalid", func(t *testing.T) {
		t.Parallel()
		c := &Committer{
			logger: logger,
			ChannelConfig: &fake.ChannelConfig{
				IDValue: "ch-endorser-discard",
			},
			Vault: &fake.Vault{
				StatusFn: func(context.Context, cdriver.TxID) (fdriver.ValidationCode, string, error) {
					return fdriver.Busy, "", nil
				},
				RWSExistsFn: func(context.Context, cdriver.TxID) bool { return false },
				MatchFn: func(context.Context, cdriver.TxID, []byte) error {
					return stderrors.New("mismatch")
				},
			},
			EnvelopeService: &fake.EnvelopeService{
				ExistsFn: func(context.Context, string) bool { return true },
			},
			RWSetLoaderService: &fake.RWSetLoader{
				GetFromEnvelopeFn: func(context.Context, cdriver.TxID) (fdriver.RWSet, fdriver.ProcessTransaction, error) {
					return &fake.RWSet{}, nil, nil
				},
			},
			TransactionManager: &fake.TransactionManager{
				NewProcessedFromPayloadFn: func([]byte) (fdriver.ProcessedTransaction, int32, error) {
					return &fake.ProcessedTransaction{ResultsBytes: []byte("rws")}, int32(common.HeaderType_ENDORSER_TRANSACTION), nil
				},
			},
		}

		event, err := c.HandleEndorserTransaction(t.Context(), &common.BlockMetadata{Metadata: validMetadata}, CommitTx{
			BlkNum:   1,
			TxNum:    0,
			TxID:     "tx-discardable",
			Envelope: &common.Envelope{Payload: []byte("payload")},
		})
		require.NoError(t, err)
		require.NotNil(t, event)
		require.Equal(t, fdriver.Invalid, event.ValidationCode)
		require.Contains(t, event.ValidationMessage, "rwsets do not match")
	})

	t.Run("commit failure returns error", func(t *testing.T) {
		t.Parallel()
		c := &Committer{
			logger: logger,
			ChannelConfig: &fake.ChannelConfig{
				IDValue: "ch-endorser-commit-fail",
			},
			Vault: &fake.Vault{
				StatusFn: func(context.Context, cdriver.TxID) (fdriver.ValidationCode, string, error) {
					return fdriver.Busy, "", nil
				},
				CommitTxFn: func(context.Context, cdriver.TxID, cdriver.BlockNum, cdriver.TxNum) error {
					return stderrors.New("vault-commit-failed")
				},
			},
			ProcessorManager: &fake.ProcessorManager{
				ProcessByIDFn: func(context.Context, string, cdriver.TxID) error { return nil },
			},
		}

		_, err := c.HandleEndorserTransaction(t.Context(), &common.BlockMetadata{Metadata: validMetadata}, CommitTx{
			BlkNum:   2,
			TxNum:    0,
			TxID:     "tx-commit-failed",
			Envelope: nil,
		})
		require.ErrorContains(t, err, "failed committing transaction [tx-commit-failed]")
	})

	t.Run("chaincode events publish failure is returned", func(t *testing.T) {
		t.Parallel()
		c := &Committer{
			logger: logger,
			ChannelConfig: &fake.ChannelConfig{
				IDValue: "ch-endorser-events-fail",
			},
			Vault: &fake.Vault{
				StatusFn: func(context.Context, cdriver.TxID) (fdriver.ValidationCode, string, error) {
					return fdriver.Busy, "", nil
				},
				CommitTxFn: func(context.Context, cdriver.TxID, cdriver.BlockNum, cdriver.TxNum) error { return nil },
			},
			ProcessorManager: &fake.ProcessorManager{
				ProcessByIDFn: func(context.Context, string, cdriver.TxID) error { return nil },
			},
			TransactionManager: &fake.TransactionManager{
				NewProcessedFromPayloadFn: func([]byte) (fdriver.ProcessedTransaction, int32, error) {
					return nil, int32(common.HeaderType_CONFIG), nil
				},
			},
		}

		_, err := c.HandleEndorserTransaction(t.Context(), &common.BlockMetadata{Metadata: validMetadata}, CommitTx{
			BlkNum:   3,
			TxNum:    0,
			TxID:     "tx-events-failed",
			Envelope: &common.Envelope{Payload: []byte("bad-payload")},
		})
		require.ErrorContains(t, err, "failed to publish chaincode events")
	})

	t.Run("valid path commits and publishes event", func(t *testing.T) {
		t.Parallel()
		pub := &fake.Publisher{}
		c := &Committer{
			logger:          logger,
			EventsPublisher: pub,
			ChannelConfig: &fake.ChannelConfig{
				IDValue: "ch-endorser-ok",
			},
			Vault: &fake.Vault{
				StatusFn: func(context.Context, cdriver.TxID) (fdriver.ValidationCode, string, error) {
					return fdriver.Busy, "", nil
				},
				CommitTxFn: func(context.Context, cdriver.TxID, cdriver.BlockNum, cdriver.TxNum) error { return nil },
			},
			ProcessorManager: &fake.ProcessorManager{
				ProcessByIDFn: func(context.Context, string, cdriver.TxID) error { return nil },
			},
			TransactionManager: &fake.TransactionManager{
				NewProcessedFromPayloadFn: func([]byte) (fdriver.ProcessedTransaction, int32, error) {
					return nil, int32(common.HeaderType_CONFIG), nil
				},
			},
		}

		env := buildEnvelopeWithChaincodeEvent(t, &peer.ChaincodeEvent{
			TxId:        "tx-ok",
			ChaincodeId: "mycc",
			EventName:   "created",
			Payload:     []byte("payload"),
		})
		event, err := c.HandleEndorserTransaction(t.Context(), &common.BlockMetadata{Metadata: validMetadata}, CommitTx{
			BlkNum:   4,
			TxNum:    0,
			TxID:     "tx-ok",
			Envelope: env,
		})
		require.NoError(t, err)
		require.NotNil(t, event)
		require.Equal(t, "tx-ok", event.TxID)
		require.Equal(t, fdriver.Valid, event.ValidationCode)
		require.NotEmpty(t, pub.Events)
	})
}
