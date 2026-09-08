/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package fabric

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/endorser/mock"
)

type mockProcessor struct {
	ProcessCount int
	LastError    error
	LastReq      Request
	LastTx       ProcessTransaction
	LastRws      *RWSet
	LastNs       string
}

func (m *mockProcessor) Process(req Request, tx ProcessTransaction, rws *RWSet, ns string) error {
	m.ProcessCount++
	m.LastReq = req
	m.LastTx = tx
	m.LastRws = rws
	m.LastNs = ns
	return m.LastError
}

type mockProcessorManagerInner struct {
	driver.ProcessorManager
	AddProcessorCount        int
	SetDefaultProcessorCount int
	AddChannelProcessorCount int
	LastError                error

	LastProcessor driver.Processor
}

func (m *mockProcessorManagerInner) AddProcessor(ns string, p driver.Processor) error {
	m.AddProcessorCount++
	m.LastProcessor = p
	return m.LastError
}

func (m *mockProcessorManagerInner) SetDefaultProcessor(p driver.Processor) error {
	m.SetDefaultProcessorCount++
	m.LastProcessor = p
	return m.LastError
}

func (m *mockProcessorManagerInner) AddChannelProcessor(channel, ns string, p driver.Processor) error {
	m.AddChannelProcessorCount++
	m.LastProcessor = p
	return m.LastError
}

type mockProcessTx struct {
	driver.ProcessTransaction
}

type mockReq struct {
	driver.Request
}

func TestProcessorManager(t *testing.T) {
	t.Parallel()

	mockPMI := &mockProcessorManagerInner{}
	pm := &ProcessorManager{pm: mockPMI}

	mp := &mockProcessor{}

	// AddProcessor
	require.NoError(t, pm.AddProcessor("ns1", mp))
	require.Equal(t, 1, mockPMI.AddProcessorCount)
	require.NotNil(t, mockPMI.LastProcessor)

	// Test inner processor wrapper
	innerProc := mockPMI.LastProcessor
	mockPMI.LastProcessor = nil

	mr := &mockReq{}
	mtx := &mockProcessTx{}
	mrws := &mock.RWSet{}

	mp.LastError = errors.New("proc err")
	err := innerProc.Process(mr, mtx, mrws, "ns2")
	require.ErrorContains(t, err, "proc err")
	require.Equal(t, 1, mp.ProcessCount)
	require.Equal(t, "ns2", mp.LastNs)
	require.NotNil(t, mp.LastReq)
	require.NotNil(t, mp.LastTx)
	require.NotNil(t, mp.LastRws)

	// SetDefaultProcessor
	require.NoError(t, pm.SetDefaultProcessor(mp))
	require.Equal(t, 1, mockPMI.SetDefaultProcessorCount)
	require.NotNil(t, mockPMI.LastProcessor)

	// AddChannelProcessor
	require.NoError(t, pm.AddChannelProcessor("ch1", "ns3", mp))
	require.Equal(t, 1, mockPMI.AddChannelProcessorCount)
	require.NotNil(t, mockPMI.LastProcessor)
}
