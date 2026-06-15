/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package chaincode_test

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric"
	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/chaincode"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view/mock"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

func TestEndorseView(t *testing.T) {
	t.Parallel()

	t.Run("NewEndorseView", func(t *testing.T) {
		t.Parallel()
		v := chaincode.NewEndorseView("my-chaincode", "my-func", "arg1", "arg2")
		require.NotNil(t, v)
		require.Equal(t, "my-chaincode", v.ChaincodeName)
		require.Equal(t, "my-func", v.Function)
		require.Equal(t, []any{"arg1", "arg2"}, v.Args)
	})

	t.Run("Fluent configuration methods", func(t *testing.T) {
		t.Parallel()
		v := chaincode.NewEndorseView("my-chaincode", "my-func", "arg1")

		v.WithTransientEntry("key1", "val1")
		require.NotNil(t, v.TransientMap)
		require.Equal(t, "val1", v.TransientMap["key1"])

		v.WithNetwork("test-network")
		require.Equal(t, "test-network", v.Network)

		v.WithChannel("test-channel")
		require.Equal(t, "test-channel", v.Channel)

		v.WithEndorsersByMSPIDs("MSP1", "MSP2")
		require.Equal(t, []string{"MSP1", "MSP2"}, v.EndorsersMSPIDs)

		v.WithEndorsersFromMyOrg()
		require.True(t, v.EndorsersFromMyOrg)

		v.WithSignerIdentity(view.Identity("my-identity"))
		require.Equal(t, view.Identity("my-identity"), v.InvokerIdentity)

		txID := fabric.TxID{}
		v.WithTxID(txID)
		require.Equal(t, txID, v.TxID)

		v.WithNumRetries(3)
		require.True(t, v.SetNumRetries)
		require.Equal(t, uint(3), v.NumRetries)

		v.WithRetrySleep(5 * time.Second)
		require.True(t, v.SetRetrySleep)
		require.Equal(t, 5*time.Second, v.RetrySleep)
	})

	t.Run("Endorse errors", func(t *testing.T) {
		t.Parallel()

		// 1. empty chaincode
		v := chaincode.NewEndorseView("", "my-func", "arg1")
		_, err := v.Endorse(nil)
		require.ErrorContains(t, err, "no chaincode specified")

		// 2. view.Context GetService network error
		mockCtx := &mock.Context{}
		mockCtx.GetServiceReturns(nil, errors.New("network error"))

		v2 := chaincode.NewEndorseView("my-chaincode", "my-func", "arg1")
		_, err = v2.Call(mockCtx)
		require.ErrorContains(t, err, "network error")
	})

	t.Run("Endorse happy path", func(t *testing.T) {
		t.Parallel()

		mockCtx, mockInv, _ := setupMockContext(t)

		v := chaincode.NewEndorseView("my-chaincode", "my-func", "arg1").
			WithTransientEntry("k1", "v1").
			WithEndorsersByMSPIDs("Org1MSP").
			WithEndorsersFromMyOrg().
			WithSignerIdentity([]byte("id")).
			WithNumRetries(2).
			WithRetrySleep(time.Second)

		env, err := v.Endorse(mockCtx)
		require.NoError(t, err)
		require.NotNil(t, env) // it's wrapped in fabric.NewEnvelope

		require.Equal(t, 1, mockInv.EndorseCallCount())
		require.Equal(t, 2, mockInv.WithSignerIdentityCallCount())
		require.Equal(t, 1, mockInv.WithTransientEntryCallCount())
		require.Equal(t, 1, mockInv.WithEndorsersByMSPIDsCallCount())
		require.Equal(t, 1, mockInv.WithEndorsersFromMyOrgCallCount())
		require.Equal(t, 1, mockInv.WithNumRetriesCallCount())
		require.Equal(t, 1, mockInv.WithRetrySleepCallCount())
	})
}
