/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package views

import (
	"context"
	"net"
	"os"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/session"
)

func names(es []Endorser) []string {
	out := make([]string, len(es))
	for i, e := range es {
		out[i] = e.Name
	}
	return out
}

func TestSelector(t *testing.T) {
	t.Parallel()
	endorsers := []Endorser{{Name: "e1"}, {Name: "e2"}, {Name: "e3"}}

	t.Run("rotates over the endorsers", func(t *testing.T) {
		t.Parallel()
		s := &selector{}
		require.Equal(t, []string{"e1", "e2", "e3"}, names(s.order(endorsers, time.Minute)))
		require.Equal(t, []string{"e2", "e3", "e1"}, names(s.order(endorsers, time.Minute)))
		require.Equal(t, []string{"e3", "e1", "e2"}, names(s.order(endorsers, time.Minute)))
		require.Equal(t, []string{"e1", "e2", "e3"}, names(s.order(endorsers, time.Minute)))
	})

	t.Run("asks an endorser that failed last until it succeeds", func(t *testing.T) {
		t.Parallel()
		s := &selector{}
		s.report("e1", errors.New("down"))
		require.Equal(t, []string{"e2", "e3", "e1"}, names(s.order(endorsers, time.Minute)))
		require.Equal(t, []string{"e2", "e3", "e1"}, names(s.order(endorsers, time.Minute)))
		s.report("e1", nil)
		require.Equal(t, []string{"e3", "e1", "e2"}, names(s.order(endorsers, time.Minute)))
	})

	t.Run("asks an endorser that failed in turn again once the cooldown passed", func(t *testing.T) {
		t.Parallel()
		s := &selector{}
		s.report("e1", errors.New("down"))
		require.Equal(t, []string{"e1", "e2", "e3"}, names(s.order(endorsers, 0)))
	})
}

func TestUnavailable(t *testing.T) {
	t.Parallel()
	dial := func(err error) error {
		return errors.Wrap(&net.OpError{Op: "read", Net: "tcp", Err: err}, "failed to open websocket")
	}
	for name, err := range map[string]error{
		"draining":          errors.Wrap(session.ErrDraining, "rejected"),
		"busy":              errors.Wrap(session.ErrBusy, "rejected"),
		"session closed":    errors.Wrap(comm.ErrSessionClosed, "collecting"),
		"deadline":          errors.Wrap(context.DeadlineExceeded, "collecting"),
		"handshake timeout": dial(os.ErrDeadlineExceeded),
		"refused":           dial(syscall.ECONNREFUSED),
	} {
		require.True(t, unavailable(err), name)
	}
	for name, err := range map[string]error{
		"unknown view":  errors.Wrap(session.ErrUnknownView, "rejected"),
		"unauthorized":  errors.Wrap(session.ErrUnauthorized, "rejected"),
		"bad response":  errors.New("failed unmarshalling received proposal response"),
		"canceled view": errors.Wrap(context.Canceled, "collecting"),
	} {
		require.False(t, unavailable(err), name)
	}
}
