/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package committer

import (
	"context"
	stderrors "errors"
	"sync/atomic"
	"testing"

	"github.com/hyperledger/fabric-protos-go-apiv2/common"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace/noop"
	"google.golang.org/protobuf/proto"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	cdriver "github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/common/services/grpc"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/committer/fake"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/protoutil"
	fdriver "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	dbdriver "github.com/hyperledger-labs/fabric-smart-client/platform/view/services/storage/driver"
)

// configEnvelope builds a channel configuration transaction carrying the given
// sequence, which is what CommitConfig keys the vault entry on.
func configEnvelope(sequence uint64) *common.Envelope {
	payload := &common.Payload{
		Data: protoutil.MarshalOrPanic(&common.ConfigEnvelope{
			Config: &common.Config{
				Sequence:     sequence,
				ChannelGroup: &common.ConfigGroup{},
			},
		}),
	}
	return &common.Envelope{Payload: protoutil.MarshalOrPanic(payload)}
}

func TestHandleConfigWrapsCommitError(t *testing.T) {
	t.Parallel()

	c := &Committer{
		logger:        logger,
		ChannelConfig: &fake.ChannelConfig{IDValue: "ch1"},
	}

	_, err := c.HandleConfig(t.Context(), nil, CommitTx{
		BlkNum:   1,
		TxNum:    1,
		TxID:     "tx1",
		Raw:      []byte("raw"),
		Envelope: nil,
	})
	require.ErrorContains(t, err, "cannot commit config envelope for channel [ch1]")
}

func TestReloadConfigTransactions(t *testing.T) {
	t.Parallel()

	t.Run("query executor creation fails", func(t *testing.T) {
		t.Parallel()
		c := &Committer{
			logger: logger,
			tracer: noop.NewTracerProvider().Tracer("test"),
			Vault: &fake.Vault{
				Vault: nil,
			},
		}
		c.Vault = &fake.VaultWithQueryErr{Vault: fake.Vault{}, Err: stderrors.New("qe-failed")}

		err := c.ReloadConfigTransactions()
		require.ErrorContains(t, err, "failed getting query executor")
	})

	t.Run("no config blocks available returns nil", func(t *testing.T) {
		t.Parallel()
		c := &Committer{
			logger: logger,
			tracer: noop.NewTracerProvider().Tracer("test"),
			Vault: &fake.VaultWithQuery{
				StatusFn: func(context.Context, cdriver.TxID) (fdriver.ValidationCode, string, error) {
					return fdriver.Unknown, "", nil
				},
				QE: &fake.QueryExecutor{},
			},
		}

		err := c.ReloadConfigTransactions()
		require.NoError(t, err)
	})
}

func TestCommitConfig(t *testing.T) {
	t.Parallel()

	t.Run("nil envelope", func(t *testing.T) {
		t.Parallel()
		c := &Committer{
			logger: logger,
		}
		err := c.CommitConfig(t.Context(), 1, []byte("raw"), nil)
		require.ErrorContains(t, err, "envelope nil")
	})

	t.Run("status lookup error", func(t *testing.T) {
		t.Parallel()
		c := &Committer{
			logger: logger,
			Vault: &fake.Vault{
				StatusFn: func(context.Context, cdriver.TxID) (fdriver.ValidationCode, string, error) {
					return fdriver.Unknown, "", stderrors.New("status-failed")
				},
			},
		}
		err := c.CommitConfig(t.Context(), 1, []byte("raw"), configEnvelope(2))
		require.ErrorContains(t, err, "failed getting tx's status")
	})

	t.Run("already committed", func(t *testing.T) {
		t.Parallel()
		c := &Committer{
			logger: logger,
			Vault: &fake.Vault{
				StatusFn: func(context.Context, cdriver.TxID) (fdriver.ValidationCode, string, error) {
					return fdriver.Valid, "", nil
				},
			},
		}
		err := c.CommitConfig(t.Context(), 1, []byte("raw"), configEnvelope(3))
		require.NoError(t, err)
	})

	t.Run("invalid status", func(t *testing.T) {
		t.Parallel()
		c := &Committer{
			logger: logger,
			Vault: &fake.Vault{
				StatusFn: func(context.Context, cdriver.TxID) (fdriver.ValidationCode, string, error) {
					return fdriver.Busy, "", nil
				},
			},
		}
		err := c.CommitConfig(t.Context(), 1, []byte("raw"), configEnvelope(4))
		require.ErrorContains(t, err, "invalid configtx's")
	})

	// Regression test: CommitConfig used to panic (panic(err)) if
	// MembershipService.Update failed after the configtx was already
	// committed to the vault. A malformed/malicious config transaction that
	// the membership service rejects would crash the whole process instead
	// of returning an error. This confirms the failure now propagates as a
	// wrapped error.
	t.Run("membership update failure is returned as error, not a panic", func(t *testing.T) {
		t.Parallel()
		// CommitConfig checks Status once itself (must be Unknown to proceed
		// past the "already committed" guard), then applyConfigCommit->CommitTX
		// checks Status again internally (must be Busy to take the
		// already-exercised c.commitBusyTx()->CommitTxFn path instead of the
		// unrelated commitUnknown() path). A stateful fake mirrors that.
		var statusCalls int
		c := &Committer{
			logger:        logger,
			ChannelConfig: &fake.ChannelConfig{IDValue: "ch-update-fail"},
			Vault: &fake.Vault{
				NewRWSetFn: func(context.Context, cdriver.TxID) (fdriver.RWSet, error) {
					return &fake.RWSet{}, nil
				},
				StatusFn: func(context.Context, cdriver.TxID) (fdriver.ValidationCode, string, error) {
					statusCalls++
					if statusCalls == 1 {
						return fdriver.Unknown, "", nil
					}
					return fdriver.Busy, "", nil
				},
				CommitTxFn: func(context.Context, cdriver.TxID, cdriver.BlockNum, cdriver.TxNum) error {
					return nil
				},
			},
			ProcessorManager: &fake.ProcessorManager{
				ProcessByIDFn: func(context.Context, string, cdriver.TxID) error { return nil },
			},
			MembershipService: &fake.MembershipService{
				UpdateFn: func(*common.Envelope) error {
					return stderrors.New("membership-update-failed")
				},
			},
		}
		require.NotPanics(t, func() {
			err := c.CommitConfig(t.Context(), 1, []byte("raw"), configEnvelope(5))
			require.ErrorContains(t, err, "failed updating membership service for configtx")
			require.ErrorContains(t, err, "membership-update-failed")
		})
	})
}

// TestCommitConfigKeysOnTheConfigSequence is the regression test for config
// blocks being dropped after the first one.
//
// CommitConfig used to key the vault entry on the transaction's index within
// its block. A configuration block holds exactly one transaction, so that index
// is always 0 and every configuration block mapped to configtx_0. The channel's
// genesis block committed that key during start-up catch-up, so every later
// configuration block hit the "already committed" guard and returned before
// MembershipService.Update and applyConfigUpdates ever ran.
func TestCommitConfigKeysOnTheConfigSequence(t *testing.T) {
	t.Parallel()

	var committedTxID string
	updated := false
	// configtx_0 is already Valid, as it is on any node that has caught up with
	// the channel's genesis block. For configtx_1 the status is consulted
	// twice: CommitConfig's own "already committed" guard must see Unknown to
	// proceed, then applyConfigCommit -> Committer.CommitTX consults it again and
	// must see Busy, because Unknown there routes to commitUnknown() instead of
	// the commit() path that calls CommitTxFn. The existing "membership update
	// failure" sub-test documents the same mechanics.
	var seqOneStatusCalls int
	c := &Committer{
		logger:        logger,
		ChannelConfig: &fake.ChannelConfig{IDValue: "ch-seq"},
		Vault: &fake.Vault{
			StatusFn: func(_ context.Context, txID cdriver.TxID) (fdriver.ValidationCode, string, error) {
				if txID == ConfigTXPrefix+"0" {
					return fdriver.Valid, "", nil
				}
				seqOneStatusCalls++
				if seqOneStatusCalls == 1 {
					return fdriver.Unknown, "", nil
				}
				return fdriver.Busy, "", nil
			},
			NewRWSetFn: func(context.Context, cdriver.TxID) (fdriver.RWSet, error) {
				return &fake.RWSet{}, nil
			},
			CommitTxFn: func(_ context.Context, txID cdriver.TxID, _ cdriver.BlockNum, _ cdriver.TxNum) error {
				committedTxID = txID
				return nil
			},
		},
		ProcessorManager: &fake.ProcessorManager{
			ProcessByIDFn: func(context.Context, string, cdriver.TxID) error { return nil },
		},
		MembershipService: &fake.MembershipService{
			UpdateFn: func(*common.Envelope) error {
				updated = true
				return nil
			},
		},
		ConfigService:   &fake.ConfigService{NetworkNameValue: "net1"},
		OrderingService: &fake.OrderingService{},
	}

	require.NoError(t, c.CommitConfig(t.Context(), 12, []byte("raw"), configEnvelope(1)))
	require.Equal(t, ConfigTXPrefix+"1", committedTxID,
		"a config block at sequence 1 must not be keyed on the genesis entry")
	require.True(t, updated,
		"the membership service must be updated for a config block that has not been seen")
}

// TestCommitConfigSkipsAConfigItAlreadyHas asserts the guard still works for a
// genuine replay: the same sequence arriving twice is committed once.
func TestCommitConfigSkipsAConfigItAlreadyHas(t *testing.T) {
	t.Parallel()

	updated := false
	c := &Committer{
		logger:        logger,
		ChannelConfig: &fake.ChannelConfig{IDValue: "ch-replay"},
		Vault: &fake.Vault{
			StatusFn: func(context.Context, cdriver.TxID) (fdriver.ValidationCode, string, error) {
				return fdriver.Valid, "", nil
			},
		},
		MembershipService: &fake.MembershipService{
			UpdateFn: func(*common.Envelope) error {
				updated = true
				return nil
			},
		},
	}

	require.NoError(t, c.CommitConfig(t.Context(), 12, []byte("raw"), configEnvelope(4)))
	require.False(t, updated, "a config block already in the vault must not be reapplied")
}

// TestCommitConfigRejectsAnEnvelopeWithoutASequence covers an envelope whose
// sequence cannot be read. Keying on a guessed value would silently collide
// with a real entry, so this must fail rather than fall back to 0.
func TestCommitConfigRejectsAnEnvelopeWithoutASequence(t *testing.T) {
	t.Parallel()

	c := &Committer{
		logger:        logger,
		ChannelConfig: &fake.ChannelConfig{IDValue: "ch-bad"},
	}

	err := c.CommitConfig(t.Context(), 1, []byte("raw"), &common.Envelope{})
	require.ErrorContains(t, err, "cannot read the config sequence")
}

func TestApplyConfigUpdates(t *testing.T) {
	t.Parallel()

	t.Run("membership lookup error is returned", func(t *testing.T) {
		t.Parallel()
		c := &Committer{
			logger:        logger,
			ChannelConfig: &fake.ChannelConfig{IDValue: "ch2"},
			ConfigService: &fake.ConfigService{NetworkNameValue: "net1"},
			MembershipService: &fake.MembershipService{
				OrdererConfigFn: func(fdriver.ConfigService) (string, []*grpc.ConnectionConfig, error) {
					return "", nil, stderrors.New("orderer-config-failed")
				},
			},
			OrderingService: &fake.OrderingService{},
		}
		err := c.applyConfigUpdates()
		require.ErrorContains(t, err, "orderer-config-failed")
	})

	t.Run("empty endpoints returns nil", func(t *testing.T) {
		t.Parallel()
		c := &Committer{
			logger:        logger,
			ChannelConfig: &fake.ChannelConfig{IDValue: "ch3"},
			ConfigService: &fake.ConfigService{NetworkNameValue: "net1"},
			MembershipService: &fake.MembershipService{
				OrdererConfigFn: func(fdriver.ConfigService) (string, []*grpc.ConnectionConfig, error) {
					return "raft", nil, nil
				},
			},
			OrderingService: &fake.OrderingService{},
		}
		require.NoError(t, c.applyConfigUpdates())
	})

	t.Run("configure orderers", func(t *testing.T) {
		t.Parallel()
		ordering := &fake.OrderingService{}
		endpoints := []*grpc.ConnectionConfig{{Address: "orderer1:7050"}}
		c := &Committer{
			logger:        logger,
			ChannelConfig: &fake.ChannelConfig{IDValue: "ch4"},
			ConfigService: &fake.ConfigService{NetworkNameValue: "net1"},
			MembershipService: &fake.MembershipService{
				OrdererConfigFn: func(fdriver.ConfigService) (string, []*grpc.ConnectionConfig, error) {
					return "raft", endpoints, nil
				},
			},
			OrderingService: ordering,
		}
		require.NoError(t, c.applyConfigUpdates())
		require.Equal(t, "raft", ordering.ConsensusType)
		require.Equal(t, endpoints, ordering.Endpoints)
	})
}

func TestCommitConfigInternalSuccessPath(t *testing.T) {
	t.Parallel()

	rws := &fake.RWSet{}
	committed := false
	c := &Committer{
		logger:        logger,
		ChannelConfig: &fake.ChannelConfig{IDValue: "ch5"},
		Vault: &fake.Vault{
			NewRWSetFn: func(context.Context, cdriver.TxID) (fdriver.RWSet, error) {
				return rws, nil
			},
			StatusFn: func(context.Context, cdriver.TxID) (fdriver.ValidationCode, string, error) {
				return fdriver.Busy, "", nil
			},
			CommitTxFn: func(context.Context, cdriver.TxID, cdriver.BlockNum, cdriver.TxNum) error {
				committed = true
				return nil
			},
		},
		ProcessorManager: &fake.ProcessorManager{
			ProcessByIDFn: func(context.Context, string, cdriver.TxID) error { return nil },
		},
	}

	err := c.applyConfigCommit(t.Context(), "configtx_1", 8, 1, []byte("env"))
	require.NoError(t, err)
	require.True(t, committed)
	require.GreaterOrEqual(t, rws.DoneCount, 1)
}

func TestReloadConfigTransactionsAdditionalBranches(t *testing.T) {
	t.Parallel()

	t.Run("invalid status code from vault", func(t *testing.T) {
		t.Parallel()
		c := &Committer{
			logger: logger,
			tracer: noop.NewTracerProvider().Tracer("test"),
			Vault: &fake.VaultWithQuery{
				StatusFn: func(context.Context, cdriver.TxID) (fdriver.ValidationCode, string, error) {
					return fdriver.Invalid, "", nil
				},
				QE: &fake.QueryExecutor{},
			},
		}
		err := c.ReloadConfigTransactions()
		require.ErrorContains(t, err, "invalid configtx")
	})

	t.Run("loads valid config block and applies updates", func(t *testing.T) {
		t.Parallel()
		updateCalled := false
		configureCalled := false

		envRaw, err := proto.Marshal(&common.Envelope{Payload: []byte("payload")})
		require.NoError(t, err)

		ordering := &fake.OrderingService{
			ConfigureFn: func(string, []*grpc.ConnectionConfig) error {
				configureCalled = true
				return nil
			},
		}
		c := &Committer{
			logger:        logger,
			tracer:        noop.NewTracerProvider().Tracer("test"),
			ChannelConfig: &fake.ChannelConfig{IDValue: "cfg-channel"},
			ConfigService: &fake.ConfigService{NetworkNameValue: "cfg-net"},
			Vault: &fake.VaultWithQuery{
				StatusFn: func(_ context.Context, txID cdriver.TxID) (fdriver.ValidationCode, string, error) {
					if txID == "configtx_0" {
						return fdriver.Valid, "", nil
					}
					return fdriver.Unknown, "", nil
				},
				QE: &fake.QueryExecutor{
					GetStateFn: func(context.Context, cdriver.Namespace, cdriver.PKey) (*cdriver.VaultRead, error) {
						return &cdriver.VaultRead{Raw: envRaw}, nil
					},
				},
			},
			MembershipService: &fake.MembershipService{
				UpdateFn: func(*common.Envelope) error {
					updateCalled = true
					return nil
				},
				OrdererConfigFn: func(fdriver.ConfigService) (string, []*grpc.ConnectionConfig, error) {
					return "raft", []*grpc.ConnectionConfig{{Address: "orderer1:7050"}}, nil
				},
			},
			OrderingService: ordering,
		}

		err = c.ReloadConfigTransactions()
		require.NoError(t, err)
		require.True(t, updateCalled)
		require.True(t, configureCalled)
	})
}

func TestCommitConfigInternalErrorPaths(t *testing.T) {
	t.Parallel()

	t.Run("new rwset error", func(t *testing.T) {
		t.Parallel()
		c := &Committer{
			logger:        logger,
			ChannelConfig: &fake.ChannelConfig{IDValue: "cfg-ch-err-1"},
			Vault: &fake.Vault{
				NewRWSetFn: func(context.Context, cdriver.TxID) (fdriver.RWSet, error) {
					return nil, stderrors.New("new-rws-failed")
				},
			},
		}
		err := c.applyConfigCommit(t.Context(), "configtx_2", 3, 2, []byte("env"))
		require.ErrorContains(t, err, "cannot create rws for configtx")
	})

	t.Run("set state error", func(t *testing.T) {
		t.Parallel()
		c := &Committer{
			logger:        logger,
			ChannelConfig: &fake.ChannelConfig{IDValue: "cfg-ch-err-2"},
			Vault: &fake.Vault{
				NewRWSetFn: func(context.Context, cdriver.TxID) (fdriver.RWSet, error) {
					return &fake.RWSet{
						SetStateFn: func(cdriver.Namespace, cdriver.PKey, cdriver.RawValue) error {
							return stderrors.New("setstate-failed")
						},
					}, nil
				},
			},
		}
		err := c.applyConfigCommit(t.Context(), "configtx_3", 3, 3, []byte("env"))
		require.ErrorContains(t, err, "failed setting configtx state in rws")
	})

	t.Run("commit tx failure is wrapped", func(t *testing.T) {
		t.Parallel()
		c := &Committer{
			logger:        logger,
			ChannelConfig: &fake.ChannelConfig{IDValue: "cfg-ch"},
			Vault: &fake.Vault{
				NewRWSetFn: func(context.Context, cdriver.TxID) (fdriver.RWSet, error) {
					return &fake.RWSet{}, nil
				},
				StatusFn: func(context.Context, cdriver.TxID) (fdriver.ValidationCode, string, error) {
					return fdriver.Busy, "", nil
				},
				CommitTxFn: func(context.Context, cdriver.TxID, cdriver.BlockNum, cdriver.TxNum) error {
					return stderrors.New("vault-commit-failed")
				},
				DiscardTxFn: func(context.Context, cdriver.TxID, string) error {
					return nil
				},
			},
			ProcessorManager: &fake.ProcessorManager{
				ProcessByIDFn: func(context.Context, string, cdriver.TxID) error { return nil },
			},
		}
		err := c.applyConfigCommit(t.Context(), "configtx_4", 4, 4, []byte("env"))
		require.ErrorContains(t, err, "failed committing configtx rws")
	})
}

// configVault is a vault whose status reflects what CommitConfig has actually
// recorded, so that a second call sees what the first one left behind.
//
// The real vault behaves this way, and a stub that always reports Unknown would
// hide the very thing this file pins: the masking depends on a later attempt
// observing Valid and taking CommitConfig's early return.
//
// The status sequence mirrors the real one within a single call. CommitConfig
// checks once itself and must see Unknown to proceed; applyConfigCommit then calls
// NewRWSet, which the real vault uses to mark the txID Busy, and CommitTX checks
// again and must see Busy to take the commitBusyTx path.
type configVault struct {
	fake.Vault
	committed atomic.Bool
	busy      atomic.Bool
}

func (v *configVault) Status(context.Context, cdriver.TxID) (fdriver.ValidationCode, string, error) {
	switch {
	case v.committed.Load():
		return fdriver.Valid, "", nil
	case v.busy.Load():
		return fdriver.Busy, "", nil
	default:
		return fdriver.Unknown, "", nil
	}
}

func (v *configVault) NewRWSet(context.Context, cdriver.TxID) (fdriver.RWSet, error) {
	v.busy.Store(true)
	return &fake.RWSet{}, nil
}

func (v *configVault) CommitTX(context.Context, cdriver.TxID, cdriver.BlockNum, cdriver.TxNum) error {
	v.busy.Store(false)
	v.committed.Store(true)
	return nil
}

// TestCommitConfigAppliesBeforeRecording pins the ordering inside CommitConfig: the
// membership service and the orderer list are updated before the vault write.
//
// The ordering is load-bearing because CommitConfig returns early when the vault
// already holds the configuration. Writing first means a commit retried after the
// write succeeded but an update failed would take that early return and report
// success having applied nothing — the node then serves a stale membership service
// while believing it is current, which is the failure #1624 describes.
func TestCommitConfigAppliesBeforeRecording(t *testing.T) {
	t.Parallel()

	t.Run("a retry after a failed update still applies the configuration", func(t *testing.T) {
		t.Parallel()

		var configureCalls, updateCalls atomic.Int32
		vault := &configVault{}
		c := &Committer{
			logger:        logger,
			Vault:         vault,
			ChannelConfig: &fake.ChannelConfig{IDValue: "ch"},
			MembershipService: &fake.MembershipService{
				UpdateFn: func(*common.Envelope) error {
					updateCalls.Add(1)
					return nil
				},
				OrdererConfigFn: func(fdriver.ConfigService) (string, []*grpc.ConnectionConfig, error) {
					return "etcdraft", []*grpc.ConnectionConfig{{Address: "orderer:7050"}}, nil
				},
			},
			ProcessorManager: &fake.ProcessorManager{
				ProcessByIDFn: func(context.Context, string, cdriver.TxID) error { return nil },
			},
			OrderingService: &fake.OrderingService{
				ConfigureFn: func(string, []*grpc.ConnectionConfig) error {
					// Fail the first attempt transiently, as a dial would, then succeed.
					if configureCalls.Add(1) == 1 {
						return errors.Wrapf(dbdriver.SqlBusy, "orderer connection busy")
					}
					return nil
				},
			},
		}

		env := configEnvelope(11)

		// First attempt: the orderer update fails, so the whole commit must fail.
		require.Error(t, c.CommitConfig(t.Context(), 1, []byte("raw"), env),
			"a failed orderer update must fail the commit")

		// Second attempt, as the retry would make it: it must actually apply the
		// configuration rather than find a vault entry and skip.
		require.NoError(t, c.CommitConfig(t.Context(), 1, []byte("raw"), env))

		require.Equal(t, int32(2), configureCalls.Load(),
			"the orderer list must be configured on the retry, not skipped as already done")
		require.Equal(t, int32(2), updateCalls.Load(),
			"the membership service must be updated on the retry")
		require.True(t, vault.committed.Load(),
			"the configuration must be recorded once it is actually in force")
	})

	t.Run("nothing is recorded when the membership update fails", func(t *testing.T) {
		t.Parallel()

		vault := &configVault{}
		c := &Committer{
			logger:        logger,
			Vault:         vault,
			ChannelConfig: &fake.ChannelConfig{IDValue: "ch"},
			MembershipService: &fake.MembershipService{
				UpdateFn: func(*common.Envelope) error {
					return errors.New("initializing channelconfig failed")
				},
			},
		}

		err := c.CommitConfig(t.Context(), 1, []byte("raw"), configEnvelope(12))

		require.ErrorContains(t, err, "failed updating membership service")
		require.False(t, vault.committed.Load(),
			"a configuration that was never applied must not be recorded as committed")
	})
}
