/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package rwset

import (
	stderrors "errors"
	"testing"

	"github.com/stretchr/testify/require"

	cdriver "github.com/hyperledger-labs/fabric-smart-client/platform/common/driver"
	rwsetfake "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/rwset/fake"
	rwsetmock "github.com/hyperledger-labs/fabric-smart-client/platform/fabric/core/generic/rwset/mock"
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

	custom := &rwsetmock.Processor{}
	require.NoError(t, pm.AddProcessor("ns1", custom))
	require.Same(t, custom, pm.processors["ns1"])

	defaultP := &rwsetmock.Processor{}
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
		channelProvider := &rwsetfake.ChannelProvider{ChannelErr: stderrors.New("channel-failed")}
		pm := NewProcessorManager(
			channelProvider,
			nil,
		)
		err := pm.ProcessByID(t.Context(), "ch1", "tx1")
		require.Error(t, err)
		require.ErrorContains(t, err, "failed getting channel")
	})

	t.Run("no entry found", func(t *testing.T) {
		t.Parallel()
		loader := &rwsetmock.RWSetLoader{}
		channelProvider := &rwsetfake.ChannelProvider{ChannelValue: newFakeChannel(false, false, loader)}
		pm := NewProcessorManager(
			channelProvider,
			nil,
		)
		require.NoError(t, pm.ProcessByID(t.Context(), "ch1", "tx1"))
		require.Equal(t, 0, loader.GetRWSetFromEvnCallCount())
		require.Equal(t, 0, loader.GetRWSetFromETxCallCount())
	})

	t.Run("extraction error", func(t *testing.T) {
		t.Parallel()
		loader := &rwsetmock.RWSetLoader{}
		loader.GetRWSetFromEvnReturns(nil, nil, stderrors.New("extract-failed"))
		channelProvider := &rwsetfake.ChannelProvider{ChannelValue: newFakeChannel(true, false, loader)}
		pm := NewProcessorManager(
			channelProvider,
			nil,
		)
		err := pm.ProcessByID(t.Context(), "ch1", "tx1")
		require.Error(t, err)
		require.ErrorContains(t, err, "failed extraction")
	})

	t.Run("custom and default processors are used", func(t *testing.T) {
		t.Parallel()
		rws := &rwsetfake.RWSet{NamespacesList: []cdriver.Namespace{"ns-custom", "ns-default"}}
		tx := &rwsetfake.ProcessTransaction{IDValue: "tx1", NetworkValue: "network", ChannelValue: "ch1", Function: "invoke", Args: []string{"a"}}
		custom := &rwsetmock.Processor{}
		defaultP := &rwsetmock.Processor{}
		loader := &rwsetmock.RWSetLoader{}
		loader.GetRWSetFromEvnReturns(rws, tx, nil)
		channelProvider := &rwsetfake.ChannelProvider{ChannelValue: newFakeChannel(true, false, loader)}

		pm := NewProcessorManager(
			channelProvider,
			defaultP,
		)
		require.NoError(t, pm.AddProcessor("ns-custom", custom))
		require.NoError(t, pm.ProcessByID(t.Context(), "ch1", "tx1"))

		require.Equal(t, 1, custom.ProcessCallCount())
		req, processTX, _, ns := custom.ProcessArgsForCall(0)
		require.Equal(t, "tx1", req.ID())
		require.Equal(t, "tx1", processTX.ID())
		require.Equal(t, "ns-custom", ns)

		require.Equal(t, 1, defaultP.ProcessCallCount())
		req, _, _, ns = defaultP.ProcessArgsForCall(0)
		require.Equal(t, "tx1", req.ID())
		require.Equal(t, "ns-default", ns)
		require.Equal(t, 1, rws.DoneCalls)
	})

	t.Run("etx path and processor error", func(t *testing.T) {
		t.Parallel()
		rws := &rwsetfake.RWSet{NamespacesList: []cdriver.Namespace{"ns1"}}
		tx := &rwsetfake.ProcessTransaction{IDValue: "tx1", NetworkValue: "network", ChannelValue: "ch1"}
		loader := &rwsetmock.RWSetLoader{}
		loader.GetRWSetFromETxReturns(rws, tx, nil)
		channelProvider := &rwsetfake.ChannelProvider{ChannelValue: newFakeChannel(false, true, loader)}
		processor := &rwsetmock.Processor{}
		processor.ProcessReturns(stderrors.New("process-failed"))
		pm := NewProcessorManager(
			channelProvider,
			processor,
		)
		err := pm.ProcessByID(t.Context(), "ch1", "tx1")
		require.ErrorContains(t, err, "process-failed")
		require.Equal(t, 1, rws.DoneCalls)
	})
}

func newFakeChannel(envExists, txExists bool, loader fdriver.RWSetLoader) *rwsetfake.Channel {
	return &rwsetfake.Channel{
		EnvelopeServiceValue:    &rwsetfake.EnvelopeService{ExistsValue: envExists},
		TransactionServiceValue: &rwsetfake.EndorserTransactionService{ExistsValue: txExists},
		RWSetLoaderValue:        loader,
	}
}
