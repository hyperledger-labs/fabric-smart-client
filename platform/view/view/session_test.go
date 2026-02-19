/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package view

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestConstants(t *testing.T) {
	t.Parallel()
	require.Equal(t, 200, OK)
	require.Equal(t, 500, ERROR)
}

func TestMessage_String(t *testing.T) {
	t.Parallel()

	table := []struct {
		name        string
		msg         Message
		contains    []string
		notContains []string
	}{
		{
			name: "All Fields Populated",
			msg: Message{
				SessionID:    "session-1",
				ContextID:    "ctx-1",
				Caller:       "view-caller",
				FromEndpoint: "endpoint-1",
				FromPKID:     []byte("pk-id"),
				Status:       OK,
				Payload:      []byte("hello"),
				Ctx:          context.Background(),
			},
			contains: []string{
				"session:session-1",
				"context:ctx-1",
				"caller:view-caller",
				"endpoint:endpoint-1",
			},
		},
		{
			name: "Empty Fields",
			msg:  Message{},
			contains: []string{
				"session:",
				"context:",
				"caller:",
				"endpoint:",
			},
		},
		{
			name: "Does Not Include Payload Or Status",
			msg: Message{
				SessionID: "s1",
				Status:    ERROR,
				Payload:   []byte("secret-data"),
			},
			contains:    []string{"session:s1"},
			notContains: []string{"secret-data", "500"},
		},
	}

	for _, tc := range table {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			result := tc.msg.String()

			for _, s := range tc.contains {
				require.Contains(t, result, s)
			}
			for _, s := range tc.notContains {
				require.NotContains(t, result, s)
			}
		})
	}
}

func TestSessionInfo_String(t *testing.T) {
	t.Parallel()

	caller := Identity("caller-identity")

	table := []struct {
		name     string
		info     SessionInfo
		contains []string
	}{
		{
			name: "All Fields Populated",
			info: SessionInfo{
				ID:           "info-1",
				Caller:       caller,
				CallerViewID: "caller-view",
				Endpoint:     "ep-1",
				EndpointPKID: []byte("pk-1"),
				Closed:       false,
			},
			contains: []string{"info-1", caller.String(), "caller-view", "ep-1", "pk-1", "false"},
		},
		{
			name: "Closed Session",
			info: SessionInfo{
				ID:     "closed-session",
				Closed: true,
			},
			contains: []string{"closed-session", "true"},
		},
		{
			name:     "Empty Fields",
			info:     SessionInfo{},
			contains: []string{"session info", "false"},
		},
	}

	for _, tc := range table {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			result := tc.info.String()

			for _, s := range tc.contains {
				require.Contains(t, result, s)
			}
		})
	}
}
