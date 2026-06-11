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

	"github.com/hyperledger-labs/fabric-smart-client/platform/fabric/services/chaincode"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/services/view/mock"
	"github.com/hyperledger-labs/fabric-smart-client/platform/view/view"
)

func TestInvokeView(t *testing.T) {
	t.Parallel()

	t.Run("NewInvokeView", func(t *testing.T) {
		t.Parallel()
		v := chaincode.NewInvokeView("my-chaincode", "my-func", "arg1", "arg2")
		require.NotNil(t, v)
		require.Equal(t, "my-chaincode", v.ChaincodeName)
		require.Equal(t, "my-func", v.Function)
		require.Equal(t, []any{"arg1", "arg2"}, v.Args)
	})

	t.Run("Fluent configuration methods", func(t *testing.T) {
		t.Parallel()
		v := chaincode.NewInvokeView("my-chaincode", "my-func", "arg1")

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

		v.WithNumRetries(3)
		require.True(t, v.SetNumRetries)
		require.Equal(t, uint(3), v.NumRetries)

		v.WithRetrySleep(5 * time.Second)
		require.True(t, v.SetRetrySleep)
		require.Equal(t, 5*time.Second, v.RetrySleep)
	})

	t.Run("Invoke errors", func(t *testing.T) {
		t.Parallel()

		v := chaincode.NewInvokeView("", "my-func", "arg1")
		_, _, err := v.Invoke(nil)
		require.ErrorContains(t, err, "no chaincode specified")

		// view.Context GetService network error
		mockCtx := &mock.Context{}
		mockCtx.GetServiceReturns(nil, errors.New("network error"))

		v2 := chaincode.NewInvokeView("my-chaincode", "my-func", "arg1")
		_, err = v2.Call(mockCtx)
		require.ErrorContains(t, err, "network error")
	})

	t.Run("Invoke happy path", func(t *testing.T) {
		t.Parallel()

		mockCtx, mockInv, _ := setupMockContext()

		v := chaincode.NewInvokeView("my-chaincode", "my-func", "arg1").
			WithTransientEntry("k1", "v1").
			WithEndorsersByMSPIDs("Org1MSP").
			WithEndorsersFromMyOrg().
			WithSignerIdentity([]byte("id")).
			WithNumRetries(2).
			WithRetrySleep(time.Second)

		txid, res, err := v.Invoke(mockCtx)
		require.NoError(t, err)
		require.Equal(t, "mock-txid", txid)
		require.Equal(t, []byte("mock-result"), res)

		require.Equal(t, 1, mockInv.SubmitCallCount())
		require.Equal(t, 2, mockInv.WithSignerIdentityCallCount())
		require.Equal(t, 1, mockInv.WithTransientEntryCallCount())
		require.Equal(t, 1, mockInv.WithEndorsersByMSPIDsCallCount())
		require.Equal(t, 1, mockInv.WithEndorsersFromMyOrgCallCount())
		require.Equal(t, 1, mockInv.WithNumRetriesCallCount())
		require.Equal(t, 1, mockInv.WithRetrySleepCallCount())
	})

	t.Run("Call happy path", func(t *testing.T) {
		t.Parallel()

		mockCtx, mockInv, _ := setupMockContext()

		v := chaincode.NewInvokeView("my-chaincode", "my-func", "arg1")

		res, err := v.Call(mockCtx)
		require.NoError(t, err)

		resArr, ok := res.([]any)
		require.True(t, ok)
		require.Len(t, resArr, 2)
		require.Equal(t, "mock-txid", resArr[0])
		require.Equal(t, []byte("mock-result"), resArr[1])

		require.Equal(t, 1, mockInv.SubmitCallCount())
	})
}
