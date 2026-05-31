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
	fdriver "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/grpc"
)

type testQueryExecutor struct {
	cdriver.QueryExecutor
	doneErr     error
	getStateFn  func(context.Context, cdriver.Namespace, cdriver.PKey) (*cdriver.VaultRead, error)
	getStateMD  func(context.Context, cdriver.Namespace, cdriver.PKey) (cdriver.Metadata, cdriver.RawVersion, error)
	getStateRng func(context.Context, cdriver.Namespace, cdriver.PKey, cdriver.PKey) (cdriver.VersionedResultsIterator, error)
}

func (q *testQueryExecutor) GetState(ctx context.Context, namespace cdriver.Namespace, key cdriver.PKey) (*cdriver.VaultRead, error) {
	if q.getStateFn == nil {
		return nil, nil
	}
	return q.getStateFn(ctx, namespace, key)
}

func (q *testQueryExecutor) GetStateMetadata(ctx context.Context, namespace cdriver.Namespace, key cdriver.PKey) (cdriver.Metadata, cdriver.RawVersion, error) {
	if q.getStateMD == nil {
		return nil, nil, nil
	}
	return q.getStateMD(ctx, namespace, key)
}

func (q *testQueryExecutor) GetStateRange(ctx context.Context, namespace cdriver.Namespace, startKey, endKey cdriver.PKey) (cdriver.VersionedResultsIterator, error) {
	if q.getStateRng == nil {
		return nil, nil
	}
	return q.getStateRng(ctx, namespace, startKey, endKey)
}

func (q *testQueryExecutor) Done() error {
	return q.doneErr
}

type testMembershipService struct {
	fdriver.MembershipService
	ordererConfigFn func(fdriver.ConfigService) (string, []*grpc.ConnectionConfig, error)
	updateFn        func(*common.Envelope) error
}

func (m *testMembershipService) OrdererConfig(cs fdriver.ConfigService) (string, []*grpc.ConnectionConfig, error) {
	if m.ordererConfigFn == nil {
		return "", nil, nil
	}
	return m.ordererConfigFn(cs)
}

func (m *testMembershipService) Update(env *common.Envelope) error {
	if m.updateFn == nil {
		return nil
	}
	return m.updateFn(env)
}

type testOrderingService struct {
	consensusType string
	endpoints     []*grpc.ConnectionConfig
	err           error
	configureFn   func(string, []*grpc.ConnectionConfig) error
}

func (s *testOrderingService) Configure(consensusType string, orderers []*grpc.ConnectionConfig) error {
	s.consensusType = consensusType
	s.endpoints = orderers
	if s.configureFn != nil {
		return s.configureFn(consensusType, orderers)
	}
	return s.err
}

func TestHandleConfigWrapsCommitError(t *testing.T) {
	t.Parallel()

	c := &Committer{
		logger:        logger,
		ChannelConfig: &testChannelConfig{id: "ch1"},
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
			Vault: &testVault{
				Vault: nil,
			},
		}
		c.Vault = &testVaultWithQueryErr{testVault: testVault{}, err: stderrors.New("qe-failed")}

		err := c.ReloadConfigTransactions()
		require.ErrorContains(t, err, "failed getting query executor")
	})

	t.Run("no config blocks available returns nil", func(t *testing.T) {
		t.Parallel()
		c := &Committer{
			logger: logger,
			tracer: noop.NewTracerProvider().Tracer("test"),
			Vault: &testVaultWithQuery{
				testVault: testVault{
					statusFn: func(context.Context, cdriver.TxID) (fdriver.ValidationCode, string, error) {
						return fdriver.Unknown, "", nil
					},
				},
				qe: &testQueryExecutor{},
			},
		}

		err := c.ReloadConfigTransactions()
		require.NoError(t, err)
	})
}

type testVaultWithQueryErr struct {
	testVault
	err error
}

func (v *testVaultWithQueryErr) NewQueryExecutor(context.Context) (cdriver.QueryExecutor, error) {
	return nil, v.err
}

type testVaultWithQuery struct {
	testVault
	qe cdriver.QueryExecutor
}

func (v *testVaultWithQuery) NewQueryExecutor(context.Context) (cdriver.QueryExecutor, error) {
	return v.qe, nil
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
			Vault: &testVault{
				statusFn: func(context.Context, cdriver.TxID) (fdriver.ValidationCode, string, error) {
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
			Vault: &testVault{
				statusFn: func(context.Context, cdriver.TxID) (fdriver.ValidationCode, string, error) {
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
			Vault: &testVault{
				statusFn: func(context.Context, cdriver.TxID) (fdriver.ValidationCode, string, error) {
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
			ChannelConfig: &testChannelConfig{id: "ch2"},
			ConfigService: &testConfigService{networkName: "net1"},
			MembershipService: &testMembershipService{
				ordererConfigFn: func(fdriver.ConfigService) (string, []*grpc.ConnectionConfig, error) {
					return "", nil, stderrors.New("orderer-config-failed")
				},
			},
			OrderingService: &testOrderingService{},
		}
		err := c.applyConfigUpdates()
		require.ErrorContains(t, err, "orderer-config-failed")
	})

	t.Run("empty endpoints returns nil", func(t *testing.T) {
		t.Parallel()
		c := &Committer{
			logger:        logger,
			ChannelConfig: &testChannelConfig{id: "ch3"},
			ConfigService: &testConfigService{networkName: "net1"},
			MembershipService: &testMembershipService{
				ordererConfigFn: func(fdriver.ConfigService) (string, []*grpc.ConnectionConfig, error) {
					return "raft", nil, nil
				},
			},
			OrderingService: &testOrderingService{},
		}
		require.NoError(t, c.applyConfigUpdates())
	})

	t.Run("configure orderers", func(t *testing.T) {
		t.Parallel()
		ordering := &testOrderingService{}
		endpoints := []*grpc.ConnectionConfig{{Address: "orderer1:7050"}}
		c := &Committer{
			logger:        logger,
			ChannelConfig: &testChannelConfig{id: "ch4"},
			ConfigService: &testConfigService{networkName: "net1"},
			MembershipService: &testMembershipService{
				ordererConfigFn: func(fdriver.ConfigService) (string, []*grpc.ConnectionConfig, error) {
					return "raft", endpoints, nil
				},
			},
			OrderingService: ordering,
		}
		require.NoError(t, c.applyConfigUpdates())
		require.Equal(t, "raft", ordering.consensusType)
		require.Equal(t, endpoints, ordering.endpoints)
	})
}

func TestCommitConfigInternalSuccessPath(t *testing.T) {
	t.Parallel()

	rws := &testRWSet{}
	committed := false
	c := &Committer{
		logger:        logger,
		ChannelConfig: &testChannelConfig{id: "ch5"},
		Vault: &testVault{
			newRWSetFn: func(context.Context, cdriver.TxID) (fdriver.RWSet, error) {
				return rws, nil
			},
			statusFn: func(context.Context, cdriver.TxID) (fdriver.ValidationCode, string, error) {
				return fdriver.Busy, "", nil
			},
			commitTxFn: func(context.Context, cdriver.TxID, cdriver.BlockNum, cdriver.TxNum) error {
				committed = true
				return nil
			},
		},
		ProcessorManager: &testProcessorManager{
			processByIDFn: func(context.Context, string, cdriver.TxID) error { return nil },
		},
	}

	err := c.commitConfig(t.Context(), "configtx_1", 8, 1, []byte("env"))
	require.NoError(t, err)
	require.True(t, committed)
	require.GreaterOrEqual(t, rws.doneCount, 1)
}

func TestReloadConfigTransactionsAdditionalBranches(t *testing.T) {
	t.Parallel()

	t.Run("invalid status code from vault", func(t *testing.T) {
		t.Parallel()
		c := &Committer{
			logger: logger,
			tracer: noop.NewTracerProvider().Tracer("test"),
			Vault: &testVaultWithQuery{
				testVault: testVault{
					statusFn: func(context.Context, cdriver.TxID) (fdriver.ValidationCode, string, error) {
						return fdriver.Invalid, "", nil
					},
				},
				qe: &testQueryExecutor{},
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

		ordering := &testOrderingService{
			configureFn: func(string, []*grpc.ConnectionConfig) error {
				configureCalled = true
				return nil
			},
		}
		c := &Committer{
			logger:        logger,
			tracer:        noop.NewTracerProvider().Tracer("test"),
			ChannelConfig: &testChannelConfig{id: "cfg-channel"},
			ConfigService: &testConfigService{networkName: "cfg-net"},
			Vault: &testVaultWithQuery{
				testVault: testVault{
					statusFn: func(_ context.Context, txID cdriver.TxID) (fdriver.ValidationCode, string, error) {
						if txID == "configtx_0" {
							return fdriver.Valid, "", nil
						}
						return fdriver.Unknown, "", nil
					},
				},
				qe: &testQueryExecutor{
					getStateFn: func(context.Context, cdriver.Namespace, cdriver.PKey) (*cdriver.VaultRead, error) {
						return &cdriver.VaultRead{Raw: envRaw}, nil
					},
				},
			},
			MembershipService: &testMembershipService{
				updateFn: func(*common.Envelope) error {
					updateCalled = true
					return nil
				},
				ordererConfigFn: func(fdriver.ConfigService) (string, []*grpc.ConnectionConfig, error) {
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
			ChannelConfig: &testChannelConfig{id: "cfg-ch-err-1"},
			Vault: &testVault{
				newRWSetFn: func(context.Context, cdriver.TxID) (fdriver.RWSet, error) {
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
			ChannelConfig: &testChannelConfig{id: "cfg-ch-err-2"},
			Vault: &testVault{
				newRWSetFn: func(context.Context, cdriver.TxID) (fdriver.RWSet, error) {
					return &testRWSet{
						setStateFn: func(cdriver.Namespace, cdriver.PKey, cdriver.RawValue) error {
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
			ChannelConfig: &testChannelConfig{id: "cfg-ch"},
			Vault: &testVault{
				newRWSetFn: func(context.Context, cdriver.TxID) (fdriver.RWSet, error) {
					return &testRWSet{}, nil
				},
				statusFn: func(context.Context, cdriver.TxID) (fdriver.ValidationCode, string, error) {
					return fdriver.Busy, "", nil
				},
				commitTxFn: func(context.Context, cdriver.TxID, cdriver.BlockNum, cdriver.TxNum) error {
					return stderrors.New("vault-commit-failed")
				},
				discardTxFn: func(context.Context, cdriver.TxID, string) error {
					return nil
				},
			},
			ProcessorManager: &testProcessorManager{
				processByIDFn: func(context.Context, string, cdriver.TxID) error { return nil },
			},
		}
		err := c.commitConfig(t.Context(), "configtx_4", 4, 4, []byte("env"))
		require.ErrorContains(t, err, "failed committing configtx rws")
	})
}
