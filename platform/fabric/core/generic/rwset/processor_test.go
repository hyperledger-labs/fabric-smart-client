/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package rwset

import (
	"context"
	stderrors "errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	cdriver "github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	fdriver "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
)

func TestRequestID(t *testing.T) {
	t.Parallel()
	req := &request{id: "tx1"}
	require.Equal(t, "tx1", req.ID())
}

func TestProcessorManagerConfigurationHelpers(t *testing.T) {
	t.Parallel()

	pm := NewProcessorManager(nil, nil)

	custom := &fakeProcessor{processFn: func(_ fdriver.Request, _ fdriver.ProcessTransaction, _ fdriver.RWSet, _ string) error {
		return nil
	}}
	require.NoError(t, pm.AddProcessor("ns1", custom))
	require.Same(t, custom, pm.processors["ns1"])

	defaultP := &fakeProcessor{processFn: func(_ fdriver.Request, _ fdriver.ProcessTransaction, _ fdriver.RWSet, _ string) error {
		return nil
	}}
	require.NoError(t, pm.SetDefaultProcessor(defaultP))
	require.Same(t, defaultP, pm.defaultProcessor)

	pm.channelProcessors["ch1"] = map[string]fdriver.Processor{}
	require.NoError(t, pm.AddChannelProcessor("ch1", "ns2", custom))
	require.Same(t, custom, pm.channelProcessors["ch1"]["ns2"])
}

func TestProcessorManagerProcessByID(t *testing.T) {
	t.Parallel()

	t.Run("channel provider error", func(t *testing.T) {
		t.Parallel()
		pm := NewProcessorManager(
			&fakeChannelProvider{
				channelFn: func(_ string) (fdriver.Channel, error) { return nil, stderrors.New("channel-failed") },
			},
			nil,
		)
		err := pm.ProcessByID(context.Background(), "ch1", "tx1")
		require.Error(t, err)
		require.ErrorContains(t, err, "failed getting channel")
	})

	t.Run("no entry found", func(t *testing.T) {
		t.Parallel()
		pm := NewProcessorManager(
			&fakeChannelProvider{
				channelFn: func(_ string) (fdriver.Channel, error) {
					return &fakeChannel{
						envService: &fakeEnvelopeService{existsFn: func(_ context.Context, _ string) bool { return false }},
						txService:  &fakeTransactionService{existsFn: func(_ context.Context, _ string) bool { return false }},
						loader: &fakeRWSetLoader{
							fromEnvFn: func(_ context.Context, _ cdriver.TxID) (fdriver.RWSet, fdriver.ProcessTransaction, error) {
								return nil, nil, nil
							},
							fromETxFn: func(_ context.Context, _ cdriver.TxID) (fdriver.RWSet, fdriver.ProcessTransaction, error) {
								return nil, nil, nil
							},
						},
					}, nil
				},
			},
			nil,
		)
		require.NoError(t, pm.ProcessByID(context.Background(), "ch1", "tx1"))
	})

	t.Run("extraction error", func(t *testing.T) {
		t.Parallel()
		pm := NewProcessorManager(
			&fakeChannelProvider{
				channelFn: func(_ string) (fdriver.Channel, error) {
					return &fakeChannel{
						envService: &fakeEnvelopeService{existsFn: func(_ context.Context, _ string) bool { return true }},
						txService:  &fakeTransactionService{existsFn: func(_ context.Context, _ string) bool { return false }},
						loader: &fakeRWSetLoader{
							fromEnvFn: func(_ context.Context, _ cdriver.TxID) (fdriver.RWSet, fdriver.ProcessTransaction, error) {
								return nil, nil, stderrors.New("extract-failed")
							},
							fromETxFn: func(_ context.Context, _ cdriver.TxID) (fdriver.RWSet, fdriver.ProcessTransaction, error) {
								return nil, nil, nil
							},
						},
					}, nil
				},
			},
			nil,
		)
		err := pm.ProcessByID(context.Background(), "ch1", "tx1")
		require.Error(t, err)
		require.ErrorContains(t, err, "failed extraction")
	})

	t.Run("custom and default processors are used", func(t *testing.T) {
		t.Parallel()
		rws := &fakeRWSet{namespaces: []cdriver.Namespace{"ns-custom", "ns-default"}}
		tx := &fakeProcessTransaction{id: "tx1", network: "network", channel: "ch1", fn: "invoke", args: []string{"a"}}

		var customCalls []string
		custom := &fakeProcessor{
			processFn: func(req fdriver.Request, processTX fdriver.ProcessTransaction, _ fdriver.RWSet, ns string) error {
				customCalls = append(customCalls, fmt.Sprintf("%s:%s", req.ID(), ns))
				require.Equal(t, "tx1", processTX.ID())
				return nil
			},
		}

		var defaultCalls []string
		defaultP := &fakeProcessor{
			processFn: func(req fdriver.Request, _ fdriver.ProcessTransaction, _ fdriver.RWSet, ns string) error {
				defaultCalls = append(defaultCalls, fmt.Sprintf("%s:%s", req.ID(), ns))
				return nil
			},
		}

		pm := NewProcessorManager(
			&fakeChannelProvider{
				channelFn: func(_ string) (fdriver.Channel, error) {
					return &fakeChannel{
						envService: &fakeEnvelopeService{existsFn: func(_ context.Context, _ string) bool { return true }},
						txService:  &fakeTransactionService{existsFn: func(_ context.Context, _ string) bool { return false }},
						loader: &fakeRWSetLoader{
							fromEnvFn: func(_ context.Context, _ cdriver.TxID) (fdriver.RWSet, fdriver.ProcessTransaction, error) {
								return rws, tx, nil
							},
							fromETxFn: func(_ context.Context, _ cdriver.TxID) (fdriver.RWSet, fdriver.ProcessTransaction, error) {
								return nil, nil, nil
							},
						},
					}, nil
				},
			},
			defaultP,
		)
		require.NoError(t, pm.AddProcessor("ns-custom", custom))
		require.NoError(t, pm.ProcessByID(context.Background(), "ch1", "tx1"))

		require.Equal(t, []string{"tx1:ns-custom"}, customCalls)
		require.Equal(t, []string{"tx1:ns-default"}, defaultCalls)
		require.Equal(t, 1, rws.doneCalls)
	})

	t.Run("etx path and processor error", func(t *testing.T) {
		t.Parallel()
		rws := &fakeRWSet{namespaces: []cdriver.Namespace{"ns1"}}
		tx := &fakeProcessTransaction{id: "tx1", network: "network", channel: "ch1"}
		pm := NewProcessorManager(
			&fakeChannelProvider{
				channelFn: func(_ string) (fdriver.Channel, error) {
					return &fakeChannel{
						envService: &fakeEnvelopeService{existsFn: func(_ context.Context, _ string) bool { return false }},
						txService:  &fakeTransactionService{existsFn: func(_ context.Context, _ string) bool { return true }},
						loader: &fakeRWSetLoader{
							fromEnvFn: func(_ context.Context, _ cdriver.TxID) (fdriver.RWSet, fdriver.ProcessTransaction, error) {
								return nil, nil, nil
							},
							fromETxFn: func(_ context.Context, _ cdriver.TxID) (fdriver.RWSet, fdriver.ProcessTransaction, error) {
								return rws, tx, nil
							},
						},
					}, nil
				},
			},
			&fakeProcessor{
				processFn: func(_ fdriver.Request, _ fdriver.ProcessTransaction, _ fdriver.RWSet, _ string) error {
					return stderrors.New("process-failed")
				},
			},
		)
		err := pm.ProcessByID(context.Background(), "ch1", "tx1")
		require.EqualError(t, err, "process-failed")
		require.Equal(t, 1, rws.doneCalls)
	})
}
