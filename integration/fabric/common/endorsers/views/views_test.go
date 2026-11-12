/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package views

import (
	"context"
	"io"
	"net"
	"os"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/session"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
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

func TestReason(t *testing.T) {
	t.Parallel()
	dial := func(err error) error {
		return errors.Wrap(&net.OpError{Op: "dial", Net: "tcp", Err: err}, "failed to open websocket")
	}
	reply := func(payload string) error {
		return session.RejectionError(&view.Message{Status: view.ERROR, Payload: []byte(payload)})
	}
	for err, want := range map[error]string{
		// A node's rejection, as the collect view returns it: the text is the reply.
		reply("draining: node is draining"):                         Draining,
		reply("busy: too many concurrent responders for this peer"): Busy,
		errors.Wrap(comm.ErrSessionClosed, "collecting"):            SessionClosed,
		errors.Wrap(context.DeadlineExceeded, "collecting"):         Deadline,
		dial(os.ErrDeadlineExceeded):                                Deadline,
		dial(syscall.ECONNREFUSED):                                  Refused,
		errors.Wrap(&net.OpError{Op: "write", Net: "tcp", Err: syscall.EPIPE}, "failed to send meta message"):  ConnLost,
		errors.Wrap(&net.OpError{Op: "read", Net: "tcp", Err: syscall.ECONNRESET}, "failed to open websocket"): ConnLost,
		errors.Wrap(io.EOF, "failed to open websocket"):                                                        ConnLost,
		errors.Wrap(io.ErrUnexpectedEOF, "failed to open websocket"):                                           ConnLost,
		errors.Wrap(net.ErrClosed, "failed to send meta message"):                                              ConnLost,
		reply("unknown_view: no responder"):                                                                    "",
		errors.Wrap(session.ErrUnauthorized, "rejected"):                                                       "",
		errors.New("failed unmarshalling received proposal response"):                                          "",
		errors.Wrap(context.Canceled, "collecting"):                                                            "",
	} {
		require.Equal(t, want, reason(err), err.Error())
	}
}
