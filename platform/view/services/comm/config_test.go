/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package comm

import (
	"testing"

	"github.com/stretchr/testify/require"
)

type mockConfigServiceForTesting struct {
	ints map[string]int
	sets map[string]bool
}

func (m *mockConfigServiceForTesting) GetInt(key string) int {
	return m.ints[key]
}

func (m *mockConfigServiceForTesting) IsSet(key string) bool {
	return m.sets[key]
}

func TestNewConfig(t *testing.T) {
	t.Parallel()

	t.Run("default values", func(t *testing.T) {
		t.Parallel()
		cs := &mockConfigServiceForTesting{
			ints: make(map[string]int),
			sets: make(map[string]bool),
		}
		cfg := NewConfig(cs)
		require.Equal(t, DefaultIncomingMessagesBufferSize, cfg.incomingMessagesBufferSize)
		require.Equal(t, DefaultStreamReaderBufferSize, cfg.streamReaderBufferSize)
		require.Equal(t, DefaultMaxMessageSize, cfg.maxRecvMsgSize)
	})

	t.Run("custom values", func(t *testing.T) {
		t.Parallel()
		cs := &mockConfigServiceForTesting{
			ints: map[string]int{
				"fsc.p2p.incomingMessagesBufferSize": 2048,
				"fsc.p2p.streamReaderBufferSize":     8192,
				"fsc.p2p.maxRecvMsgSize":             5000,
			},
			sets: map[string]bool{
				"fsc.p2p.incomingMessagesBufferSize": true,
				"fsc.p2p.streamReaderBufferSize":     true,
				"fsc.p2p.maxRecvMsgSize":             true,
			},
		}
		cfg := NewConfig(cs)
		require.Equal(t, 2048, cfg.incomingMessagesBufferSize)
		require.Equal(t, 8192, cfg.streamReaderBufferSize)
		require.Equal(t, 5000, cfg.maxRecvMsgSize)
	})

	t.Run("clamping invalid values", func(t *testing.T) {
		t.Parallel()
		cs := &mockConfigServiceForTesting{
			ints: map[string]int{
				"fsc.p2p.incomingMessagesBufferSize": 0,
				"fsc.p2p.streamReaderBufferSize":     -5,
				"fsc.p2p.maxRecvMsgSize":             -100,
			},
			sets: map[string]bool{
				"fsc.p2p.incomingMessagesBufferSize": true,
				"fsc.p2p.streamReaderBufferSize":     true,
				"fsc.p2p.maxRecvMsgSize":             true,
			},
		}
		cfg := NewConfig(cs)
		// Should clamp to defaults
		require.Equal(t, DefaultIncomingMessagesBufferSize, cfg.incomingMessagesBufferSize)
		require.Equal(t, DefaultStreamReaderBufferSize, cfg.streamReaderBufferSize)
		require.Equal(t, DefaultMaxMessageSize, cfg.maxRecvMsgSize)
	})

	t.Run("zero maxRecvMsgSize is allowed", func(t *testing.T) {
		t.Parallel()
		cs := &mockConfigServiceForTesting{
			ints: map[string]int{
				"fsc.p2p.maxRecvMsgSize": 0,
			},
			sets: map[string]bool{
				"fsc.p2p.maxRecvMsgSize": true,
			},
		}
		cfg := NewConfig(cs)
		require.Equal(t, 0, cfg.maxRecvMsgSize)
	})
}
