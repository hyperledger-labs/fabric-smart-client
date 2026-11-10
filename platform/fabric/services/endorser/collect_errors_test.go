/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package endorser

import (
	"context"
	"reflect"
	"testing"
	"testing/synctest"

	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/endorser/mock"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/comm/session"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

// collectFromBob runs the collect-endorsements view in fakeCtx for a transaction endorsed by
// bob, a remote party whose session answers on reply. It returns the view's error.
func collectFromBob(t *testing.T, fakeCtx *mock.Context, reply chan *view.Message) error {
	t.Helper()
	fakeCtx.IsMeReturns(false)

	fakeTx := &mock.Transaction{}
	fakeTx.ChannelReturns("ch1")
	fakeTx.NetworkReturns("net1")
	fakeRWS := &mock.RWSet{}
	fakeRWS.BytesReturns([]byte("results"), nil)
	fakeTx.GetRWSetReturns(fakeRWS, nil)
	fakeTx.BytesReturns([]byte("bytes"), nil)
	fakeTx.ResultsReturns([]byte("results"), nil)

	fakeFNS := &mock.FabricNetworkService{}
	fakeFNS.ConfigServiceReturns(genericConfig())
	fakeFNS.NameReturns("net1")
	fakeCH := &mock.Channel{}
	fakeCH.NameReturns("ch1")
	fakeCH.ChannelMembershipReturns(&mock.ChannelMembership{})
	fakeFNS.ChannelReturns(fakeCH, nil)
	fakeFNSP := &mock.FabricNetworkServiceProvider{}
	fakeFNSP.FabricNetworkServiceReturns(fakeFNS, nil)
	nsp := fabric.NewNetworkServiceProvider(fakeFNSP, nil)
	fakeCtx.GetServiceCalls(func(v any) (any, error) {
		if v == reflect.TypeFor[*fabric.NetworkServiceProvider]() {
			return nsp, nil
		}
		return nil, nil
	})

	fakeSession := &mock.Session{}
	fakeSession.ReceiveReturns(reply)
	fakeCtx.GetSessionReturns(fakeSession, nil)

	tx := &Transaction{Transaction: fabric.NewTransaction(fabric.NewNetworkService(nil, fakeFNS, "net1"), fakeTx)}
	_, err := NewCollectEndorsementsView(tx, []byte("bob")).Call(fakeCtx)
	return err
}

// viewContext returns a fake view context whose Context is ctx.
func viewContext(ctx context.Context) *mock.Context {
	c := &mock.Context{}
	c.ContextReturns(ctx)
	return c
}

// A caller that spreads endorsements over several endorsers tells "the endorser is
// unavailable, try the next one" from any other failure with errors.Is.
func TestCollectEndorsementsView_ErrorTypes(t *testing.T) {
	t.Parallel()

	t.Run("rejection", func(t *testing.T) {
		t.Parallel()
		for reason, want := range map[string]error{
			session.ReasonDraining:    session.ErrDraining,
			session.ReasonBusy:        session.ErrBusy,
			session.ReasonNodeBusy:    session.ErrBusy,
			session.ReasonUnknownView: session.ErrUnknownView,
		} {
			reply := make(chan *view.Message, 1)
			reply <- &view.Message{Status: view.ERROR, Payload: []byte(reason + ": no new work")}
			err := collectFromBob(t, viewContext(t.Context()), reply)
			require.ErrorIs(t, err, want, reason)
			require.ErrorIs(t, err, session.ErrRejected, reason)
			require.EqualError(t, err, "received error from remote ["+reason+": no new work]")
		}
	})

	t.Run("error reply that is not a rejection", func(t *testing.T) {
		t.Parallel()
		reply := make(chan *view.Message, 1)
		reply <- &view.Message{Status: view.ERROR, Payload: []byte("failed endorsing: bad input")}
		err := collectFromBob(t, viewContext(t.Context()), reply)
		require.EqualError(t, err, "failed endorsing: bad input")
		require.NotErrorIs(t, err, session.ErrRejected)
	})

	t.Run("session closed", func(t *testing.T) {
		t.Parallel()
		reply := make(chan *view.Message)
		close(reply)
		err := collectFromBob(t, viewContext(t.Context()), reply)
		require.ErrorIs(t, err, comm.ErrSessionClosed)
		require.Regexp(t, "^session to party .+ closed: session closed$", err.Error())
	})

	t.Run("no reply before the timeout", func(t *testing.T) {
		t.Parallel()
		// The bubble's clock passes the view's one-minute wait as soon as everything blocks.
		synctest.Test(t, func(t *testing.T) {
			err := collectFromBob(t, viewContext(t.Context()), make(chan *view.Message))
			require.ErrorIs(t, err, context.DeadlineExceeded)
			require.Regexp(t, "^Timeout from party .+: context deadline exceeded$", err.Error())
		})
	})

	t.Run("caller's context ends", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		err := collectFromBob(t, viewContext(ctx), make(chan *view.Message))
		require.ErrorIs(t, err, context.Canceled)
	})
}
