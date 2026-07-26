/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package config_test

import (
	"context"
	"testing"
	"time"

	cb "github.com/hyperledger/fabric-protos-go-apiv2/common"
	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/proto"
	channelconfig "github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/core/channel/config"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/core/channel/config/mock"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabricx/core/committer/queryservice"
)

// configEnvelopeWithSequence builds a well-formed *cb.Envelope wrapping a
// marshaled Payload wrapping a marshaled ConfigEnvelope carrying the given
// Config.Sequence -- the envelope-embedded sequence number that
// checkAndUpdate now cross-validates against the remote-reported Version.
func configEnvelopeWithSequence(t *testing.T, sequence uint64) *cb.Envelope {
	t.Helper()

	configEnvelope := &cb.ConfigEnvelope{
		Config: &cb.Config{Sequence: sequence},
	}
	configEnvelopeRaw, err := proto.Marshal(configEnvelope)
	require.NoError(t, err)

	payload := &cb.Payload{Data: configEnvelopeRaw}
	payloadRaw, err := proto.Marshal(payload)
	require.NoError(t, err)

	return &cb.Envelope{Payload: payloadRaw}
}

// TestMonitorDetectsGenuinelyDifferentConfigViaSequenceEvenWhenVersionIsStale
// proves the fix: ChannelConfigMonitor.checkAndUpdate no longer trusts the
// remote-reported ConfigTransactionInfo.Version field in isolation. It now
// also cross-validates the config sequence number embedded in the envelope's
// own content (Config.Sequence), so a malicious/compromised committer cannot
// suppress a genuinely new config merely by reporting a stale Version.
//
// This test replays the same attack the original PoC demonstrated: after one
// legitimate update (version 1 / sequence 1 -> envelope A), the fake service
// starts serving a distinct envelope B with a genuinely higher embedded
// Config.Sequence (as if the real channel config changed, e.g. new orderer
// endpoints or membership) while still reporting the same stale Version: 1.
// Unlike before the fix, the monitor now detects the advanced sequence number
// and applies the update anyway.
func TestMonitorDetectsGenuinelyDifferentConfigViaSequenceEvenWhenVersionIsStale(t *testing.T) {
	t.Parallel()

	config := &channelconfig.Config{
		PollInterval:      20 * time.Millisecond,
		MaxRetries:        1,
		InitialRetryDelay: 5 * time.Millisecond,
		MaxRetryDelay:     20 * time.Millisecond,
	}

	queryService := &mock.QueryService{}
	membershipService := &mock.MembershipService{}
	orderingService := &mock.OrderingService{}
	configService := &mock.ConfigService{}

	envelopeA := configEnvelopeWithSequence(t, 1)
	envelopeB := configEnvelopeWithSequence(t, 2)

	callCount := 0
	queryService.GetConfigTransactionCalls(func() (*queryservice.ConfigTransactionInfo, error) {
		callCount++
		if callCount == 1 {
			// Legitimate initial config, version 1, sequence 1.
			return &queryservice.ConfigTransactionInfo{Envelope: envelopeA, Version: 1}, nil
		}
		// From the second call onward, the "channel config" has genuinely
		// changed (envelopeB, sequence 2), but the compromised committer
		// keeps reporting the same stale version to try to suppress
		// propagation.
		return &queryservice.ConfigTransactionInfo{Envelope: envelopeB, Version: 1}, nil
	})

	membershipService.UpdateReturns(nil)
	membershipService.OrdererConfigReturns("etcdraft", nil, nil)
	orderingService.ConfigureReturns(nil)

	monitor, err := channelconfig.NewChannelConfigMonitor(
		config, queryService, membershipService,
		orderingService, configService, "testnet", "mychannel",
	)
	require.NoError(t, err)

	err = monitor.Start(context.Background())
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		return membershipService.UpdateCallCount() >= 2
	}, 2*time.Second, 10*time.Millisecond, "the sequence-advanced envelopeB must be applied despite the stale reported Version")

	err = monitor.Stop()
	require.NoError(t, err)

	// The first update applied envelope A; a later one applied envelope B,
	// proving the genuinely new content was detected via its embedded
	// sequence number rather than being silently dropped.
	require.Same(t, envelopeA, membershipService.UpdateArgsForCall(0))
	appliedLast := membershipService.UpdateArgsForCall(membershipService.UpdateCallCount() - 1)
	require.Same(t, envelopeB, appliedLast)
}

// TestMonitorSkipsUpdateWhenNeitherVersionNorSequenceAdvanced proves the
// complementary, non-regressed behavior: once a config has been applied, a
// subsequent poll that serves the exact same envelope/version (no genuine
// change at all) must still be recognized as "nothing new" and not trigger a
// redundant call to applyConfigUpdate.
func TestMonitorSkipsUpdateWhenNeitherVersionNorSequenceAdvanced(t *testing.T) {
	t.Parallel()

	config := &channelconfig.Config{
		PollInterval:      20 * time.Millisecond,
		MaxRetries:        1,
		InitialRetryDelay: 5 * time.Millisecond,
		MaxRetryDelay:     20 * time.Millisecond,
	}

	queryService := &mock.QueryService{}
	membershipService := &mock.MembershipService{}
	orderingService := &mock.OrderingService{}
	configService := &mock.ConfigService{}

	envelopeA := configEnvelopeWithSequence(t, 1)

	queryService.GetConfigTransactionReturns(&queryservice.ConfigTransactionInfo{Envelope: envelopeA, Version: 1}, nil)
	membershipService.UpdateReturns(nil)
	membershipService.OrdererConfigReturns("etcdraft", nil, nil)
	orderingService.ConfigureReturns(nil)

	monitor, err := channelconfig.NewChannelConfigMonitor(
		config, queryService, membershipService,
		orderingService, configService, "testnet", "mychannel",
	)
	require.NoError(t, err)

	err = monitor.Start(context.Background())
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		return queryService.GetConfigTransactionCallCount() >= 5
	}, 2*time.Second, 10*time.Millisecond)

	err = monitor.Stop()
	require.NoError(t, err)

	// Despite many polls, the identical version/sequence must only ever be
	// applied once.
	require.Equal(t, 1, membershipService.UpdateCallCount())
}
