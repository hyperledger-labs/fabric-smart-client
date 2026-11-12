/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package endorser

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/errors"
	"github.com/hyperledger-labs/fabric-smart-client/pkg/utils/proto"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/driver"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/protos"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/endorser/mock"
)

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

// wireTxRawNoTransient is the JSON of another Fabric transaction without a channel key, the
// form a fixture's BytesNoTransient returns.
var wireTxRawNoTransient = []byte(`{"Type":3,"Raw":"eyJUVHhJRCI6InR4MiIsIlRDaGFubmVsIjoiY2gxIn0="}`)

// flatTxRaw is the JSON of a transaction of flatDriver without a channel key.
var flatTxRaw = []byte(`{"TTxID":"tx1","TChannel":"ch1"}`)

// flatDriver is the driver name of a test platform whose transactions are flat JSON.
const flatDriver = "test-flat"

func init() {
	protos.RegisterTransactionFormat(flatDriver, protos.TransactionFormat{
		ToPayload: func(raw []byte) ([]byte, *driver.ChannelKey, error) { return raw, nil, nil },
		ToJSON:    func(*protos.TransactionEnvelope) ([]byte, error) { return nil, errors.New("not implemented") },
	})
}

func TestWireBytes(t *testing.T) {
	t.Parallel()

	for driverName, raw := range map[string][]byte{"generic": wireTxRaw, flatDriver: flatTxRaw} {
		t.Run(driverName+" sends an envelope", func(t *testing.T) {
			t.Parallel()
			out, err := WireBytes(wireTx(driverName), raw)
			require.NoError(t, err)
			want, err := protos.EnvelopeFromJSONFor(raw, driverName)
			require.NoError(t, err)
			require.Equal(t, want, out)
			env := &protos.TransactionEnvelope{}
			require.NoError(t, proto.Unmarshal(out, env))
			require.Equal(t, driverName, env.GetPlatform())
		})
	}

	t.Run("an unknown driver sends the JSON unchanged", func(t *testing.T) {
		t.Parallel()
		out, err := WireBytes(wireTx("other"), wireTxRaw)
		require.NoError(t, err)
		require.Equal(t, wireTxRaw, out)
	})

	t.Run("JSON that does not convert fails", func(t *testing.T) {
		t.Parallel()
		_, err := WireBytes(wireTx("generic"), []byte(`{"Type":3,"Raw":"not base64"}`))
		require.Error(t, err)
	})
}
