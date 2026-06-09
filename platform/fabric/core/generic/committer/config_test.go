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
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace/noop"
	"google.golang.org/protobuf/proto"

	cdriver "github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/committer/fake"
	fdriver "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/grpc"
)

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
				Vault: fake.Vault{
					StatusFn: func(context.Context, cdriver.TxID) (fdriver.ValidationCode, string, error) {
						return fdriver.Unknown, "", nil
					},
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
		err := c.CommitConfig(t.Context(), 1, 1, []byte("raw"), nil)
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
		err := c.CommitConfig(t.Context(), 1, 2, []byte("raw"), &common.Envelope{})
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
		err := c.CommitConfig(t.Context(), 1, 3, []byte("raw"), &common.Envelope{})
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
		err := c.CommitConfig(t.Context(), 1, 4, []byte("raw"), &common.Envelope{})
		require.ErrorContains(t, err, "invalid configtx's")
	})
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

	err := c.commitConfig(t.Context(), "configtx_1", 8, 1, []byte("env"))
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
				Vault: fake.Vault{
					StatusFn: func(context.Context, cdriver.TxID) (fdriver.ValidationCode, string, error) {
						return fdriver.Invalid, "", nil
					},
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
				Vault: fake.Vault{
					StatusFn: func(_ context.Context, txID cdriver.TxID) (fdriver.ValidationCode, string, error) {
						if txID == "configtx_0" {
							return fdriver.Valid, "", nil
						}
						return fdriver.Unknown, "", nil
					},
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
		err := c.commitConfig(t.Context(), "configtx_2", 3, 2, []byte("env"))
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
		err := c.commitConfig(t.Context(), "configtx_3", 3, 3, []byte("env"))
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
		err := c.commitConfig(t.Context(), "configtx_4", 4, 4, []byte("env"))
		require.ErrorContains(t, err, "failed committing configtx rws")
	})
}
