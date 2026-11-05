/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package endorser

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/proto"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/protos"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/endorser/mock"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

// versionedSession is a session at a fixed version of every component, or failing with err.
type versionedSession struct {
	mock.Session
	version uint32
	err     error
	asked   []string
}

func (s *versionedSession) Version(_ context.Context, component string) (uint32, bool, error) {
	s.asked = append(s.asked, component)
	return s.version, true, s.err
}

// wireTx returns a transaction on a network whose driver is named driverName.
func wireTx(driverName string) *Transaction {
	conf := &mock.ConfigService{}
	conf.DriverNameReturns(driverName)
	fns := &mock.FabricNetworkService{}
	fns.ConfigServiceReturns(conf)
	return &Transaction{Transaction: fabric.NewTransaction(fabric.NewNetworkService(nil, fns, "net"), &mock.Transaction{})}
}

// wireTxRaw is the JSON of a Fabric transaction without a channel key.
var wireTxRaw = []byte(`{"Type":3,"Raw":"eyJUVHhJRCI6InR4MSIsIlRDaGFubmVsIjoiY2gxIn0="}`)

// flatTxRaw is the JSON of a transaction of flatDriver without a channel key.
var flatTxRaw = []byte(`{"TTxID":"tx1","TChannel":"ch1"}`)

// flatDriver is the driver name of a test platform whose transactions are flat JSON.
const flatDriver = "test-flat"

func init() {
	protos.RegisterTransactionFormat(flatDriver, protos.TransactionFormat{
		Component: "test-flat",
		ToPayload: func(raw []byte) ([]byte, *driver.ChannelKey, error) { return raw, nil, nil },
		ToJSON:    func(*protos.TransactionEnvelope) ([]byte, error) { return nil, errors.New("not implemented") },
	})
}

func TestWireBytes(t *testing.T) {
	t.Parallel()

	for driverName, tc := range map[string]struct {
		component string
		raw       []byte
	}{"generic": {"fabric", wireTxRaw}, flatDriver: {"test-flat", flatTxRaw}} {
		component, raw := tc.component, tc.raw
		t.Run(driverName+" v1 sends the JSON unchanged", func(t *testing.T) {
			t.Parallel()
			s := &versionedSession{version: 1}
			out, err := WireBytes(t.Context(), s, wireTx(driverName), raw)
			require.NoError(t, err)
			require.Equal(t, raw, out)
			require.Equal(t, []string{component}, s.asked)
		})

		t.Run(driverName+" v2 sends an envelope", func(t *testing.T) {
			t.Parallel()
			s := &versionedSession{version: 2}
			out, err := WireBytes(t.Context(), s, wireTx(driverName), raw)
			require.NoError(t, err)
			want, err := protos.EnvelopeFromJSONFor(raw, driverName)
			require.NoError(t, err)
			require.Equal(t, want, out)
			env := &protos.TransactionEnvelope{}
			require.NoError(t, proto.Unmarshal(out, env))
			require.Equal(t, driverName, env.GetPlatform())
			require.Equal(t, []string{component}, s.asked)
		})
	}

	t.Run("a session without versions is at v1", func(t *testing.T) {
		t.Parallel()
		var s view.Session = &mock.Session{}
		out, err := WireBytes(t.Context(), s, wireTx("generic"), wireTxRaw)
		require.NoError(t, err)
		require.Equal(t, wireTxRaw, out)
	})

	t.Run("a Version error is returned", func(t *testing.T) {
		t.Parallel()
		boom := errors.New("boom")
		_, err := WireBytes(t.Context(), &versionedSession{version: 2, err: boom}, wireTx("generic"), wireTxRaw)
		require.ErrorIs(t, err, boom)
	})

	t.Run("an unknown driver sends the JSON unchanged, also at v2", func(t *testing.T) {
		t.Parallel()
		s := &versionedSession{version: 2}
		out, err := WireBytes(t.Context(), s, wireTx("other"), wireTxRaw)
		require.NoError(t, err)
		require.Equal(t, wireTxRaw, out)
		require.Empty(t, s.asked, "no version is negotiated")
	})
}
